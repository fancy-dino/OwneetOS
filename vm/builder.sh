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

DISK_SIZE="80G"
MEM_MB="8192"
CPUS="8"
SSH_PORT="${OWNEET_BUILDER_SSH_PORT:-2222}"
SEED_PORT="${OWNEET_SEED_PORT:-8765}"
PROVISION_TIMEOUT_S=1800

# shellcheck source=lib/common.sh
source "$VM_DIR/lib/common.sh"
IMAGE="$CACHE_DIR/$IMAGE_NAME"

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

ssh_opts builder "$SSH_PORT"

# Arguments form the remote command line on purpose.
# shellcheck disable=SC2029
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

# --- commands ---------------------------------------------------------------

cmd_setup() {
    if [[ -d "$VM_DIR/images/$NAME" ]]; then
        die "the builder VM already exists. To rebuild it: vm/builder.sh destroy && vm/builder.sh setup"
    fi
    verified_download "$IMAGE_URL" "$IMAGE" "$IMAGE_SHA256" "$SIGNING_KEY" \
        "Arch Linux cloud image $IMAGE_VERSION (~550 MB)"
    ensure_ssh_key builder
    seed_start "$VM_DIR/run/$NAME-seed" "$SEED_PORT" "$VM_DIR/builder/user-data.in" builder owneet-builder

    "$VM" create "$NAME" "$DISK_SIZE" --from "$IMAGE"
    "$VM" start "$NAME" --headless --mem "$MEM_MB" --cpus "$CPUS" \
        --ssh-port "$SSH_PORT" --seed-url "http://10.0.2.2:$SEED_PORT/"

    info "waiting for SSH"
    wait_for_ssh 300 || die "the VM did not answer on SSH within 5 minutes; see vm/run/$NAME/serial.log"

    info "installing build tools in the VM (archiso, base-devel, devtools, git, rsync, shellcheck, nodejs, go, fontTools, ffmpeg); this takes a few minutes"
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

    seed_stop

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
    ssh-tty) shift; is_running || die "the builder VM is not running (vm/builder.sh start)"
             ssh -t "${SSH_OPTS[@]}" builder@127.0.0.1 "$@" ;;  # interactive command (passphrase prompts)
    destroy) "$VM" destroy "$NAME" ;;
    help|-h|--help) usage ;;
    *) usage; exit 1 ;;
esac
