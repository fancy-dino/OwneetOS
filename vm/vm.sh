#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# OwneetOS VM manager. Every file it creates stays under vm/images/ and vm/run/.
# Usage: vm/vm.sh help

set -euo pipefail

VM_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IMAGES_DIR="$VM_DIR/images"
RUN_DIR="$VM_DIR/run"

DEFAULT_DISK_SIZE="32G"   # matches the minimum disk size in PROJECT_RULES.md section 3
DEFAULT_MEM_MB="4096"     # matches the minimum RAM target
DEFAULT_CPUS="4"

die() { echo "error: $*" >&2; exit 1; }
info() { echo "==> $*"; }

usage() {
    cat <<'EOF'
OwneetOS VM manager

  vm/vm.sh create  NAME [SIZE]        Create a VM with a blank disk (default 32G, thin-provisioned)
  vm/vm.sh start   NAME [options]     Boot a VM
        --iso FILE      attach FILE as a CD-ROM (boots from it while the disk is empty)
        --headless      no window; runs in the background (serial log in vm/run/NAME/)
        --mem MB        RAM in MB (default 4096)
        --cpus N        virtual CPUs (default 4)
        --no-net        no network card
  vm/vm.sh stop    NAME               Power off a running VM
  vm/vm.sh destroy NAME               Stop a VM and delete all of its files
  vm/vm.sh status  NAME               Show whether a VM is running
  vm/vm.sh list                       List VMs
  vm/vm.sh selftest                   Boot a throwaway UEFI VM, check it, delete it

Firmware can be overridden with OVMF_CODE=... and OVMF_VARS=... (UEFI without Secure Boot).
EOF
}

# --- checks -----------------------------------------------------------------

check_name() {
    [[ "${1:-}" =~ ^[a-z0-9][a-z0-9-]{0,31}$ ]] \
        || die "invalid VM name '${1:-}' (lowercase letters, digits and '-', max 32 chars)"
}

need_cmd() { command -v "$1" >/dev/null 2>&1 || die "'$1' not found. See vm/README.md for the required packages."; }

check_host() {
    need_cmd qemu-system-x86_64
    need_cmd qemu-img
    [[ -r /dev/kvm && -w /dev/kvm ]] || die "/dev/kvm is not accessible by $(id -un); KVM acceleration is required."
    if pgrep -x -f '.*(VBoxHeadless|VirtualBoxVM).*' >/dev/null 2>&1; then
        die "a VirtualBox VM is running. Close it first: VirtualBox and KVM cannot run VMs at the same time."
    fi
}

find_firmware() {
    if [[ -n "${OVMF_CODE:-}" || -n "${OVMF_VARS:-}" ]]; then
        [[ -f "${OVMF_CODE:-}" && -f "${OVMF_VARS:-}" ]] || die "OVMF_CODE and OVMF_VARS must both point to existing files."
        return
    fi
    # Non-Secure-Boot builds only (Secure Boot is always disabled in OwneetOS).
    local pairs=(
        "/usr/share/OVMF/OVMF_CODE_4M.fd:/usr/share/OVMF/OVMF_VARS_4M.fd"            # Debian, Ubuntu, Mint
        "/usr/share/edk2/x64/OVMF_CODE.4m.fd:/usr/share/edk2/x64/OVMF_VARS.4m.fd"    # Arch
        "/usr/share/edk2/ovmf/OVMF_CODE.fd:/usr/share/edk2/ovmf/OVMF_VARS.fd"        # Fedora
    )
    local p
    for p in "${pairs[@]}"; do
        if [[ -f "${p%%:*}" && -f "${p##*:}" ]]; then
            OVMF_CODE="${p%%:*}"; OVMF_VARS="${p##*:}"
            return
        fi
    done
    die "UEFI firmware (OVMF) not found. Install the 'ovmf' package or set OVMF_CODE / OVMF_VARS."
}

vm_image_dir() { echo "$IMAGES_DIR/$1"; }
vm_run_dir() { echo "$RUN_DIR/$1"; }

vm_pid() {
    local pidfile; pidfile="$(vm_run_dir "$1")/qemu.pid"
    [[ -f "$pidfile" ]] || return 1
    local pid; pid="$(cat "$pidfile")"
    [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null || return 1
    echo "$pid"
}

# --- commands ---------------------------------------------------------------

cmd_create() {
    local name="${1:-}" size="${2:-$DEFAULT_DISK_SIZE}"
    check_name "$name"
    need_cmd qemu-img
    find_firmware
    local dir; dir="$(vm_image_dir "$name")"
    [[ -e "$dir" ]] && die "VM '$name' already exists ($dir)."
    mkdir -p "$dir"
    qemu-img create -q -f qcow2 "$dir/disk.qcow2" "$size"
    cp "$OVMF_VARS" "$dir/OVMF_VARS.fd"
    info "created VM '$name' (disk $size, thin-provisioned) in vm/images/$name"
}

cmd_start() {
    local name="${1:-}"; shift || true
    check_name "$name"
    local iso="" headless=0 mem="$DEFAULT_MEM_MB" cpus="$DEFAULT_CPUS" net=1
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --iso)      iso="${2:-}"; shift 2 ;;
            --headless) headless=1; shift ;;
            --mem)      mem="${2:-}"; shift 2 ;;
            --cpus)     cpus="${2:-}"; shift 2 ;;
            --no-net)   net=0; shift ;;
            *) die "unknown option '$1' (see: vm/vm.sh help)" ;;
        esac
    done
    [[ "$mem" =~ ^[0-9]+$ && "$cpus" =~ ^[0-9]+$ ]] || die "--mem and --cpus need numbers."

    check_host
    find_firmware
    local img; img="$(vm_image_dir "$name")"
    [[ -f "$img/disk.qcow2" ]] || die "VM '$name' does not exist. Create it with: vm/vm.sh create $name"
    vm_pid "$name" >/dev/null && die "VM '$name' is already running."

    local run; run="$(vm_run_dir "$name")"
    mkdir -p "$run"
    rm -f "$run/serial.log" "$run/qemu.log" "$run/qmp.sock" "$run/qemu.pid"

    local args=(
        -name "owneet-$name"
        -machine q35,accel=kvm
        -cpu host -smp "$cpus" -m "$mem"
        -drive "if=pflash,format=raw,readonly=on,file=$OVMF_CODE"
        -drive "if=pflash,format=raw,file=$img/OVMF_VARS.fd"
        -drive "file=$img/disk.qcow2,if=virtio,format=qcow2,discard=unmap"
        -serial "file:$run/serial.log"
        -qmp "unix:$run/qmp.sock,server=on,wait=off"
        -pidfile "$run/qemu.pid"
    )
    if [[ -n "$iso" ]]; then
        [[ -f "$iso" ]] || die "ISO not found: $iso"
        args+=(-drive "file=$(realpath "$iso"),media=cdrom,readonly=on")
    fi
    if (( net )); then
        args+=(-nic "user,model=virtio-net-pci")
    else
        args+=(-nic none)
    fi

    if (( headless )); then
        args+=(-display none -daemonize)
        qemu-system-x86_64 "${args[@]}" 2>"$run/qemu.log" || die "QEMU failed to start, see vm/run/$name/qemu.log"
        info "VM '$name' running in the background (pid $(cat "$run/qemu.pid")); serial log: vm/run/$name/serial.log"
    else
        if ! qemu-system-x86_64 -display help 2>/dev/null | grep -qx 'gtk'; then
            die "QEMU cannot open windows on this host. Install its GUI support (Debian/Ubuntu/Mint: qemu-system-gui;
       Arch: qemu-ui-gtk), or use --headless."
        fi
        args+=(-vga virtio -display gtk)
        info "starting VM '$name' in a window; close the window to power it off"
        qemu-system-x86_64 "${args[@]}"
    fi
}

cmd_stop() {
    local name="${1:-}"
    check_name "$name"
    local pid
    if ! pid="$(vm_pid "$name")"; then
        info "VM '$name' is not running"
        return 0
    fi
    kill "$pid"
    local i
    for i in $(seq 1 50); do
        kill -0 "$pid" 2>/dev/null || break
        sleep 0.2
    done
    if kill -0 "$pid" 2>/dev/null; then
        kill -9 "$pid"
    fi
    rm -f "$(vm_run_dir "$name")/qmp.sock" "$(vm_run_dir "$name")/qemu.pid"
    info "VM '$name' stopped"
}

cmd_destroy() {
    local name="${1:-}"
    check_name "$name"
    cmd_stop "$name" >/dev/null
    local img run
    img="$(vm_image_dir "$name")"; run="$(vm_run_dir "$name")"
    [[ -e "$img" || -e "$run" ]] || die "VM '$name' does not exist."
    # Paths are built from a validated name under vm/, so nothing outside vm/ can be removed.
    rm -rf -- "$img" "$run"
    info "VM '$name' deleted"
}

cmd_status() {
    local name="${1:-}"
    check_name "$name"
    [[ -d "$(vm_image_dir "$name")" ]] || die "VM '$name' does not exist."
    local pid
    if pid="$(vm_pid "$name")"; then echo "$name: running (pid $pid)"; else echo "$name: stopped"; fi
}

cmd_list() {
    [[ -d "$IMAGES_DIR" ]] || { echo "no VMs"; return; }
    local d found=0
    for d in "$IMAGES_DIR"/*/; do
        [[ -d "$d" ]] || continue
        found=1
        local name; name="$(basename "$d")"
        local size; size="$(du -sh "$d" | cut -f1)"
        if vm_pid "$name" >/dev/null; then echo "$name  running  ($size on disk)"; else echo "$name  stopped  ($size on disk)"; fi
    done
    (( found )) || echo "no VMs"
}

cmd_selftest() {
    local name="selftest"
    [[ -e "$(vm_image_dir "$name")" ]] && cmd_destroy "$name" >/dev/null
    cmd_create "$name" 1G
    cmd_start "$name" --headless --no-net --mem 512 --cpus 1

    # With no bootable media, OVMF ends in its boot manager or UEFI shell; both print to serial.
    local log; log="$(vm_run_dir "$name")/serial.log"
    local ok=0 i
    for i in $(seq 1 60); do
        if grep -aqE 'UEFI Interactive Shell|Shell>|BdsDxe|Boot Manager' "$log" 2>/dev/null; then ok=1; break; fi
        sleep 1
    done
    local excerpt
    excerpt="$(sed -e 's/\x1b\[[0-9;=?]*[A-Za-z]//g' -e 's/\r//g' "$log" 2>/dev/null | grep -aE 'BdsDxe' | head -3 || true)"

    cmd_destroy "$name"
    if [[ -e "$(vm_image_dir "$name")" || -e "$(vm_run_dir "$name")" ]]; then
        die "selftest: files of VM '$name' were not removed"
    fi
    (( ok )) || die "selftest: UEFI firmware output not seen on the serial console within 60 s"
    info "selftest passed: UEFI firmware booted under KVM and the VM was removed cleanly"
    [[ -n "$excerpt" ]] && sed 's/^/    /' <<<"$excerpt"
    return 0
}

# --- main -------------------------------------------------------------------

cmd="${1:-help}"; shift || true
case "$cmd" in
    create)   cmd_create "$@" ;;
    start)    cmd_start "$@" ;;
    stop)     cmd_stop "$@" ;;
    destroy)  cmd_destroy "$@" ;;
    status)   cmd_status "$@" ;;
    list)     cmd_list ;;
    selftest) cmd_selftest ;;
    help|-h|--help) usage ;;
    *) usage; exit 1 ;;
esac
