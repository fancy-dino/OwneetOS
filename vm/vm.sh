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

  vm/vm.sh create  NAME [SIZE] [--from IMAGE]
                                      Create a VM with a blank disk (default 32G, thin-provisioned),
                                      or a copy-on-write layer over IMAGE (IMAGE is never modified)
  vm/vm.sh start   NAME [options]     Boot a VM
        --iso FILE      attach FILE as a CD-ROM (boots from it while the disk is empty)
        --headless      no window; runs in the background (serial log in vm/run/NAME/)
        --mem MB        RAM in MB (default 4096)
        --cpus N        virtual CPUs (default 4)
        --no-net        no network card
        --ssh-port N    forward host 127.0.0.1:N to the guest's SSH port
        --seed-url URL  cloud-init NoCloud seed URL (first-boot configuration)
        --evdev PATH    pass a host input device (e.g. a gamepad) to the guest; repeatable
        --gl            3D-accelerated virtual GPU (virgl), needed for gamescope
        --kernel-args S extra kernel command line, appended by systemd-boot (SMBIOS type 11)
  vm/vm.sh stop    NAME [--force]     Shut a VM down (ACPI, then forced after 60 s; --force: at once)
  vm/vm.sh destroy NAME               Stop a VM and delete all of its files
  vm/vm.sh add-disk NAME SIZE         Attach an extra blank disk (data-N.qcow2) to a VM
  vm/vm.sh snapshot NAME TAG          Save the state of all disks of a stopped VM
  vm/vm.sh revert   NAME TAG          Bring all disks back to a saved state (VM stopped)
  vm/vm.sh snapshots NAME             List saved states
  vm/vm.sh screenshot NAME FILE.png   Save what the VM's screen shows
  vm/vm.sh keys NAME COMBO            Press a key combination, e.g. ctrl-alt-f9 (QEMU key names)
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

qmp_screendump() {
    python3 - "$1" "$2" <<'PY' 2>/dev/null
import json, socket, sys
s = socket.socket(socket.AF_UNIX)
s.settimeout(10)
s.connect(sys.argv[1])
f = s.makefile("rw")
f.readline()  # greeting
for cmd in ({"execute": "qmp_capabilities"},
            {"execute": "screendump", "arguments": {"filename": sys.argv[2], "format": "png"}}):
    f.write(json.dumps(cmd) + "\n"); f.flush()
    while True:
        msg = json.loads(f.readline())
        if "error" in msg:
            sys.exit(1)
        if "return" in msg:
            break
PY
}

qmp_keys() {
    python3 - "$1" "$2" <<'PY' 2>/dev/null
import json, socket, sys
s = socket.socket(socket.AF_UNIX)
s.settimeout(10)
s.connect(sys.argv[1])
f = s.makefile("rw")
f.readline()  # greeting
keys = [{"type": "qcode", "data": k} for k in sys.argv[2].split("-")]
for cmd in ({"execute": "qmp_capabilities"}, {"execute": "send-key", "arguments": {"keys": keys}}):
    f.write(json.dumps(cmd) + "\n"); f.flush()
    while True:
        msg = json.loads(f.readline())
        if "error" in msg:
            sys.exit(1)
        if "return" in msg:
            break
PY
}

qmp_powerdown() {
    python3 - "$1" <<'PY' 2>/dev/null
import json, socket, sys
s = socket.socket(socket.AF_UNIX)
s.settimeout(5)
s.connect(sys.argv[1])
f = s.makefile("rw")
f.readline()  # greeting
for cmd in ("qmp_capabilities", "system_powerdown"):
    f.write(json.dumps({"execute": cmd}) + "\n"); f.flush()
    while True:
        msg = json.loads(f.readline())
        if "return" in msg or "error" in msg:
            break
PY
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
    local name="${1:-}"; shift || true
    check_name "$name"
    local size="$DEFAULT_DISK_SIZE" from=""
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --from) from="${2:-}"; shift 2 ;;
            -*) die "unknown option '$1' (see: vm/vm.sh help)" ;;
            *) size="$1"; shift ;;
        esac
    done
    need_cmd qemu-img
    find_firmware
    local dir; dir="$(vm_image_dir "$name")"
    if [[ -e "$dir" ]]; then die "VM '$name' already exists ($dir)."; fi
    mkdir -p "$dir"
    if [[ -n "$from" ]]; then
        [[ -f "$from" ]] || die "base image not found: $from"
        # Relative backing path, so the project folder can be moved without breaking the VM.
        local rel; rel="$(realpath --relative-to="$dir" "$from")"
        qemu-img create -q -f qcow2 -b "$rel" -F qcow2 "$dir/disk.qcow2" "$size"
    else
        qemu-img create -q -f qcow2 "$dir/disk.qcow2" "$size"
    fi
    cp "$OVMF_VARS" "$dir/OVMF_VARS.fd"
    info "created VM '$name' (disk $size, thin-provisioned${from:+, layered over $(basename "$from")}) in vm/images/$name"
}

cmd_start() {
    local name="${1:-}"; shift || true
    check_name "$name"
    local iso="" headless=0 mem="$DEFAULT_MEM_MB" cpus="$DEFAULT_CPUS" net=1 ssh_port="" seed_url="" gl=0
    local evdevs=() kargs=""
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --iso)      iso="${2:-}"; shift 2 ;;
            --headless) headless=1; shift ;;
            --mem)      mem="${2:-}"; shift 2 ;;
            --cpus)     cpus="${2:-}"; shift 2 ;;
            --no-net)   net=0; shift ;;
            --ssh-port) ssh_port="${2:-}"; shift 2 ;;
            --seed-url) seed_url="${2:-}"; shift 2 ;;
            --evdev)    evdevs+=("${2:-}"); shift 2 ;;
            --gl)       gl=1; shift ;;
            --kernel-args) kargs="${2:-}"; shift 2 ;;
            *) die "unknown option '$1' (see: vm/vm.sh help)" ;;
        esac
    done
    [[ "$mem" =~ ^[0-9]+$ && "$cpus" =~ ^[0-9]+$ ]] || die "--mem and --cpus need numbers."
    [[ -z "$ssh_port" || "$ssh_port" =~ ^[0-9]+$ ]] || die "--ssh-port needs a number."
    (( net )) || [[ -z "$ssh_port" && -z "$seed_url" ]] || die "--ssh-port and --seed-url need the network (drop --no-net)."

    check_host
    find_firmware
    local img; img="$(vm_image_dir "$name")"
    [[ -f "$img/disk.qcow2" ]] || die "VM '$name' does not exist. Create it with: vm/vm.sh create $name"
    if vm_pid "$name" >/dev/null; then die "VM '$name' is already running."; fi

    local run; run="$(vm_run_dir "$name")"
    mkdir -p "$run"
    rm -f "$run/serial.log" "$run/serial.sock" "$run/qemu.log" "$run/qmp.sock" "$run/qemu.pid"

    local args=(
        -name "owneet-$name"
        -machine "q35,accel=kvm"
        -cpu host -smp "$cpus" -m "$mem"
        -drive "if=pflash,format=raw,readonly=on,file=$OVMF_CODE"
        -drive "if=pflash,format=raw,file=$img/OVMF_VARS.fd"
        -drive "file=$img/disk.qcow2,if=virtio,format=qcow2,discard=unmap"
        # Serial console: everything is logged to serial.log; serial.sock accepts input (vm/test.sh run).
        -chardev "socket,id=ser0,path=$run/serial.sock,server=on,wait=off,logfile=$run/serial.log"
        -serial chardev:ser0
        -qmp "unix:$run/qmp.sock,server=on,wait=off"
        -pidfile "$run/qemu.pid"
    )
    local extra
    for extra in "$img"/data-*.qcow2; do
        if [[ -f "$extra" ]]; then args+=(-drive "file=$extra,if=virtio,format=qcow2,discard=unmap"); fi
    done
    if [[ -n "$kargs" ]]; then
        # systemd-boot appends this to the kernel command line (only with Secure Boot off).
        # QEMU needs commas doubled inside option values.
        args+=(-smbios "type=11,value=io.systemd.boot.kernel-cmdline-extra=${kargs//,/,,}")
    fi
    local dev
    for dev in "${evdevs[@]}"; do
        [[ -r "$dev" && -w "$dev" ]] || die "cannot open input device $dev (needs read and write access)"
        # The device is grabbed by the VM: while it runs, the host no longer sees its input.
        args+=(-device "virtio-input-host-pci,evdev=$(realpath "$dev")")
    done
    if [[ -n "$iso" ]]; then
        [[ -f "$iso" ]] || die "ISO not found: $iso"
        # -cdrom uses the machine's default CD-ROM drive (no second, empty drive).
        args+=(-cdrom "$(realpath "$iso")")
    fi
    if (( net )); then
        local nic="user,model=virtio-net-pci"
        # Bound to 127.0.0.1 only: the VM is never reachable from the local network.
        if [[ -n "$ssh_port" ]]; then nic+=",hostfwd=tcp:127.0.0.1:$ssh_port-:22"; fi
        args+=(-nic "$nic")
        if [[ -n "$seed_url" ]]; then args+=(-smbios "type=1,serial=ds=nocloud;s=$seed_url"); fi
    else
        args+=(-nic none)
    fi

    if (( headless )); then
        if (( gl )); then args+=(-device virtio-vga-gl -display egl-headless); else args+=(-display none); fi
        args+=(-daemonize)
        qemu-system-x86_64 "${args[@]}" 2>"$run/qemu.log" || die "QEMU failed to start, see vm/run/$name/qemu.log"
        info "VM '$name' running in the background (pid $(cat "$run/qemu.pid")); serial log: vm/run/$name/serial.log"
    else
        if ! qemu-system-x86_64 -display help 2>/dev/null | grep -qx 'gtk'; then
            die "QEMU cannot open windows on this host. Install its GUI support (Debian/Ubuntu/Mint: qemu-system-gui;
       Arch: qemu-ui-gtk), or use --headless."
        fi
        if (( gl )); then args+=(-device virtio-vga-gl -display "gtk,gl=on"); else args+=(-vga virtio -display gtk); fi
        info "starting VM '$name' in a window; close the window to power it off"
        qemu-system-x86_64 "${args[@]}"
    fi
}

cmd_stop() {
    local name="${1:-}" force=0
    if [[ "${2:-}" == "--force" ]]; then force=1; fi
    check_name "$name"
    local pid
    if ! pid="$(vm_pid "$name")"; then
        info "VM '$name' is not running"
        return 0
    fi
    # Ask the guest to shut down cleanly (ACPI power button) through QMP, then force if needed.
    local sock; sock="$(vm_run_dir "$name")/qmp.sock"
    if (( ! force )) && [[ -S "$sock" ]] && qmp_powerdown "$sock"; then
        for _ in $(seq 1 120); do
            kill -0 "$pid" 2>/dev/null || break
            sleep 0.5
        done
    fi
    if kill -0 "$pid" 2>/dev/null; then
        kill "$pid"
        for _ in $(seq 1 50); do
            kill -0 "$pid" 2>/dev/null || break
            sleep 0.2
        done
    fi
    if kill -0 "$pid" 2>/dev/null; then kill -9 "$pid"; fi
    rm -f "$(vm_run_dir "$name")/qmp.sock" "$(vm_run_dir "$name")/serial.sock" "$(vm_run_dir "$name")/qemu.pid"
    info "VM '$name' stopped"
}

cmd_destroy() {
    local name="${1:-}"
    check_name "$name"
    cmd_stop "$name" --force >/dev/null
    local img run
    img="$(vm_image_dir "$name")"; run="$(vm_run_dir "$name")"
    [[ -e "$img" || -e "$run" ]] || die "VM '$name' does not exist."
    # Paths are built from a validated name under vm/, so nothing outside vm/ can be removed.
    rm -rf -- "$img" "$run"
    info "VM '$name' deleted"
}

cmd_add_disk() {
    local name="${1:-}" size="${2:-}"
    check_name "$name"
    [[ "$size" =~ ^[0-9]+[KMGT]?$ ]] || die "usage: vm/vm.sh add-disk NAME SIZE (e.g. 64G)"
    local img; img="$(vm_image_dir "$name")"
    [[ -f "$img/disk.qcow2" ]] || die "VM '$name' does not exist."
    if vm_pid "$name" >/dev/null; then die "stop VM '$name' first."; fi
    local n=1
    while [[ -e "$img/data-$n.qcow2" ]]; do n=$(( n + 1 )); done
    qemu-img create -q -f qcow2 "$img/data-$n.qcow2" "$size"
    info "added disk data-$n ($size, thin-provisioned) to VM '$name'"
}

vm_disks() {
    local img; img="$(vm_image_dir "$1")"
    [[ -f "$img/disk.qcow2" ]] || die "VM '$1' does not exist."
    local d
    for d in "$img/disk.qcow2" "$img"/data-*.qcow2; do if [[ -f "$d" ]]; then echo "$d"; fi; done
    return 0
}

cmd_snapshot() {
    local name="${1:-}" tag="${2:-}" op="${3:-create}"
    check_name "$name"
    [[ "$tag" =~ ^[A-Za-z0-9._-]+$ ]] || die "invalid snapshot tag '$tag'"
    if vm_pid "$name" >/dev/null; then die "stop VM '$name' first."; fi
    local d flag
    case "$op" in create) flag=-c ;; revert) flag=-a ;; esac
    while read -r d; do
        qemu-img snapshot "$flag" "$tag" "$d" || die "snapshot '$tag' failed on $(basename "$d")"
    done < <(vm_disks "$name")
    if [[ "$op" == create ]]; then
        cp "$(vm_image_dir "$name")/OVMF_VARS.fd" "$(vm_image_dir "$name")/OVMF_VARS.$tag.fd"
        info "saved state '$tag' of VM '$name'"
    else
        local vars; vars="$(vm_image_dir "$name")/OVMF_VARS.$tag.fd"
        if [[ -f "$vars" ]]; then cp "$vars" "$(vm_image_dir "$name")/OVMF_VARS.fd"; fi
        info "VM '$name' is back to state '$tag'"
    fi
}

cmd_snapshots() {
    local name="${1:-}"
    check_name "$name"
    qemu-img snapshot -l "$(vm_image_dir "$name")/disk.qcow2"
}

cmd_screenshot() {
    local name="${1:-}" file="${2:-}"
    check_name "$name"
    [[ "$file" == *.png ]] || die "usage: vm/vm.sh screenshot NAME FILE.png"
    vm_pid "$name" >/dev/null || die "VM '$name' is not running."
    mkdir -p "$(dirname "$file")"
    qmp_screendump "$(vm_run_dir "$name")/qmp.sock" "$(realpath "$file")" \
        || die "screenshot failed (with --gl and no window, QEMU keeps no copy of the screen: boot without --gl)"
    info "screenshot saved: $file"
}

cmd_keys() {
    local name="${1:-}" combo="${2:-}"
    check_name "$name"
    [[ "$combo" =~ ^[a-z0-9_]+(-[a-z0-9_]+)*$ ]] || die "usage: vm/vm.sh keys NAME COMBO (e.g. ctrl-alt-f9)"
    vm_pid "$name" >/dev/null || die "VM '$name' is not running."
    qmp_keys "$(vm_run_dir "$name")/qmp.sock" "$combo" || die "could not send keys '$combo'"
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
    if [[ -e "$(vm_image_dir "$name")" ]]; then cmd_destroy "$name" >/dev/null; fi
    cmd_create "$name" 1G
    cmd_start "$name" --headless --no-net --mem 512 --cpus 1

    # With no bootable media, OVMF ends in its boot manager or UEFI shell; both print to serial.
    local log; log="$(vm_run_dir "$name")/serial.log"
    local ok=0
    for _ in $(seq 1 60); do
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
    local line
    while IFS= read -r line; do
        if [[ -n "$line" ]]; then echo "    $line"; fi
    done <<<"$excerpt"
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
    add-disk) cmd_add_disk "$@" ;;
    snapshot) cmd_snapshot "${1:-}" "${2:-}" create ;;
    revert)   cmd_snapshot "${1:-}" "${2:-}" revert ;;
    snapshots) cmd_snapshots "$@" ;;
    screenshot) cmd_screenshot "$@" ;;
    keys)     cmd_keys "$@" ;;
    help|-h|--help) usage ;;
    *) usage; exit 1 ;;
esac
