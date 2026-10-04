#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# OwneetOS builder VM: an Arch Linux VM where ISOs and packages are built.
# Usage: vm/builder.sh help

set -euo pipefail

VM_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VM="$VM_DIR/vm.sh"
NAME="builder"

# Official Arch Linux cloud image, pinned. Bump all three values together.
IMAGE_VERSION="20261001.604814"
IMAGE_SHA256="360f0fa49db6813bdc8e35bed230a2dc2ae3567b7b5ab74719c0a706e4e34e87"
IMAGE_NAME="Arch-Linux-x86_64-cloudimg-$IMAGE_VERSION.qcow2"
IMAGE_URL="https://fastly.mirror.pkgbuild.com/images/v$IMAGE_VERSION/$IMAGE_NAME"
# arch-boxes <arch-boxes@archlinux.org>, the key that signs the official images.
SIGNING_KEY="1B9A16984A4E8CB448712D2AE0B78BF4326C6F8F"
KEYSERVER="hkps://keyserver.ubuntu.com"

DISK_SIZE="80G"
MEM_MB="8192"
CPUS="8"
SSH_PORT="${OWNEET_BUILDER_SSH_PORT:-2222}"
SEED_PORT="${OWNEET_SEED_PORT:-8765}"
PROVISION_TIMEOUT_S=1800

CACHE_DIR="$VM_DIR/images/.cache"
KEYS_DIR="$VM_DIR/images/.keys"
GPG_DIR="$VM_DIR/images/.gnupg"
SSH_KEY="$KEYS_DIR/builder_ed25519"
IMAGE="$CACHE_DIR/$IMAGE_NAME"

die() { echo "error: $*" >&2; exit 1; }
info() { echo "==> $*"; }

usage() {
    cat <<EOF
OwneetOS builder VM (Arch Linux, ${MEM_MB} MB RAM, ${CPUS} CPUs, ${DISK_SIZE} disk)

  vm/builder.sh setup      Download and verify the Arch image, create and provision the VM (once)
  vm/builder.sh start      Start the VM in the background and wait until SSH answers
  vm/builder.sh stop       Shut the VM down cleanly
  vm/builder.sh status     Show whether the VM is running
  vm/builder.sh ssh [CMD]  Open a shell in the VM, or run CMD
  vm/builder.sh destroy    Delete the VM (the downloaded image and keys are kept)

SSH listens on 127.0.0.1:${SSH_PORT} (override with OWNEET_BUILDER_SSH_PORT).
EOF
}

SSH_OPTS=(
    -i "$SSH_KEY" -p "$SSH_PORT"
    -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR
    -o ConnectTimeout=5 -o BatchMode=yes
)

vm_ssh() { ssh "${SSH_OPTS[@]}" builder@127.0.0.1 "$@"; }

is_running() { "$VM" status "$NAME" 2>/dev/null | grep -q running; }

wait_for_ssh() {
    local timeout="$1" i
    for (( i = 0; i < timeout; i += 2 )); do
        vm_ssh true 2>/dev/null && return 0
        sleep 2
    done
    return 1
}

# --- image download and verification ---------------------------------------

fetch_image() {
    if [[ -f "$IMAGE" ]]; then
        info "Arch image $IMAGE_VERSION already downloaded"
        return
    fi
    mkdir -p "$CACHE_DIR"
    mkdir -p -m 700 "$GPG_DIR"
    export GNUPGHOME="$GPG_DIR"   # project-local keyring; the user's keyring is never touched

    info "downloading Arch Linux cloud image $IMAGE_VERSION (~550 MB)"
    curl -fL --progress-bar -o "$IMAGE.part" "$IMAGE_URL"
    curl -fsSL -o "$IMAGE.sig" "$IMAGE_URL.sig"

    info "verifying checksum"
    echo "$IMAGE_SHA256  $IMAGE.part" | sha256sum --check --quiet \
        || { rm -f "$IMAGE.part"; die "checksum mismatch: the download is corrupted or tampered with"; }

    info "verifying signature (arch-boxes key $SIGNING_KEY)"
    if ! gpg --batch --quiet --list-keys "$SIGNING_KEY" >/dev/null 2>&1; then
        gpg --batch --quiet --keyserver "$KEYSERVER" --recv-keys "$SIGNING_KEY" \
            || die "could not fetch the signing key from $KEYSERVER"
    fi
    local status
    status="$(gpg --batch --status-fd 1 --verify "$IMAGE.sig" "$IMAGE.part" 2>/dev/null || true)"
    grep -q "^\[GNUPG:\] VALIDSIG .* $SIGNING_KEY\$" <<<"$status" \
        || { rm -f "$IMAGE.part"; die "signature check failed"; }

    mv "$IMAGE.part" "$IMAGE"
    info "image verified: $IMAGE_NAME"
}

# --- commands ---------------------------------------------------------------

cmd_setup() {
    if [[ -d "$VM_DIR/images/$NAME" ]]; then
        die "the builder VM already exists. To rebuild it: vm/builder.sh destroy && vm/builder.sh setup"
    fi
    command -v python3 >/dev/null || die "python3 is required (it serves the first-boot configuration)"

    fetch_image

    if [[ ! -f "$SSH_KEY" ]]; then
        mkdir -p -m 700 "$KEYS_DIR"
        ssh-keygen -q -t ed25519 -N "" -C "owneet-builder" -f "$SSH_KEY"
        info "created project SSH key in vm/images/.keys/ (your ~/.ssh is not used)"
    fi

    # First-boot configuration, served once over HTTP on 127.0.0.1 (seen by the VM as 10.0.2.2).
    local seed="$VM_DIR/run/$NAME-seed"
    rm -rf "$seed"; mkdir -p "$seed"
    sed "s|@SSH_KEY@|$(cat "$SSH_KEY.pub")|" "$VM_DIR/builder/user-data.in" > "$seed/user-data"
    printf 'instance-id: owneet-builder-%s\nlocal-hostname: owneet-builder\n' "$(date +%s)" > "$seed/meta-data"
    : > "$seed/vendor-data"

    python3 -m http.server --bind 127.0.0.1 --directory "$seed" "$SEED_PORT" >"$seed/server.log" 2>&1 &
    SEED_SERVER_PID=$!
    trap 'if [[ -n "${SEED_SERVER_PID:-}" ]]; then kill "$SEED_SERVER_PID" 2>/dev/null || true; fi' EXIT
    sleep 1
    kill -0 "$SEED_SERVER_PID" 2>/dev/null || die "could not start the seed server on port $SEED_PORT (set OWNEET_SEED_PORT)"

    "$VM" create "$NAME" "$DISK_SIZE" --from "$IMAGE"
    "$VM" start "$NAME" --headless --mem "$MEM_MB" --cpus "$CPUS" \
        --ssh-port "$SSH_PORT" --seed-url "http://10.0.2.2:$SEED_PORT/"

    info "waiting for SSH"
    wait_for_ssh 300 || die "the VM did not answer on SSH within 5 minutes; see vm/run/$NAME/serial.log"

    info "installing build tools in the VM (archiso, base-devel, devtools, git, rsync); this takes a few minutes"
    local waited=0
    while (( waited < PROVISION_TIMEOUT_S )); do
        if vm_ssh test -f /var/lib/owneet/provisioned 2>/dev/null; then break; fi
        if vm_ssh test -f /var/lib/owneet/failed 2>/dev/null; then
            die "provisioning failed in the VM. Log: vm/builder.sh ssh sudo tail -50 /var/log/cloud-init-output.log"
        fi
        sleep 15; waited=$(( waited + 15 ))
        if (( waited % 60 == 0 )); then info "still installing ($(( waited / 60 )) min)"; fi
    done
    (( waited < PROVISION_TIMEOUT_S )) || die "provisioning did not finish within $(( PROVISION_TIMEOUT_S / 60 )) minutes"

    kill "$SEED_SERVER_PID" 2>/dev/null || true
    SEED_SERVER_PID=""
    rm -rf "$seed"

    # A full system upgrade may have replaced the kernel: restart once so the VM runs it.
    info "restarting the VM on the updated system"
    "$VM" stop "$NAME" >/dev/null
    cmd_start >/dev/null
    info "builder ready: $(vm_ssh 'uname -r; pacman -Q archiso' | tr '\n' ' ')"
}

cmd_start() {
    [[ -d "$VM_DIR/images/$NAME" ]] || die "the builder VM does not exist. Create it with: vm/builder.sh setup"
    if ! is_running; then
        "$VM" start "$NAME" --headless --mem "$MEM_MB" --cpus "$CPUS" --ssh-port "$SSH_PORT"
    fi
    wait_for_ssh 180 || die "the builder VM did not answer on SSH; see vm/run/$NAME/serial.log"
    info "builder is up (ssh on 127.0.0.1:$SSH_PORT)"
}

case "${1:-help}" in
    setup)   cmd_setup ;;
    start)   cmd_start ;;
    stop)    "$VM" stop "$NAME" ;;
    status)  "$VM" status "$NAME" ;;
    ssh)     shift; is_running || die "the builder VM is not running (vm/builder.sh start)"
             if [[ $# -gt 0 ]]; then vm_ssh "$@"; else ssh -t "${SSH_OPTS[@]}" builder@127.0.0.1; fi ;;
    ssh-command) printf '%q ' ssh "${SSH_OPTS[@]}"; echo ;;  # used by tools/build-in-vm for rsync
    destroy) "$VM" destroy "$NAME" ;;
    help|-h|--help) usage ;;
    *) usage; exit 1 ;;
esac
