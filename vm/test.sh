#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# OwneetOS test VM: boots an ISO on blank virtual disks, optionally with the host's gamepads.
# Usage: vm/test.sh help

set -euo pipefail

VM_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$VM_DIR/.." && pwd)"
VM="$VM_DIR/vm.sh"
NAME="test"

# Stock Arch Linux ISO, pinned. Used to check the harness until OwneetOS builds its own ISO.
ARCH_ISO_VERSION="2026.10.01"
ARCH_ISO_SHA256="684ded26c63240ff4a41e8c25ee84ea6da233f557364821f13d12c2b0a9059a5"
ARCH_ISO_NAME="archlinux-$ARCH_ISO_VERSION-x86_64.iso"
ARCH_ISO_URL="https://fastly.mirror.pkgbuild.com/iso/$ARCH_ISO_VERSION/$ARCH_ISO_NAME"
# Pierre Schmitz <pierre@archlinux.org>, Arch Linux release manager (signs the ISOs).
ARCH_ISO_KEY="3E80CA1A8B89F69CBA57D98A76A5EF9054449A5C"

# Minimum hardware from PROJECT_RULES.md section 3.
MEM_MB="4096"
CPUS="4"
SYSTEM_DISK="32G"
DATA_DISK="64G"
SSH_PORT="${OWNEET_TEST_SSH_PORT:-2223}"
SEED_PORT="${OWNEET_TEST_SEED_PORT:-8766}"

# shellcheck source=lib/common.sh
source "$VM_DIR/lib/common.sh"
ARCH_ISO="$CACHE_DIR/$ARCH_ISO_NAME"

usage() {
    cat <<EOF
OwneetOS test VM (${MEM_MB} MB RAM, ${CPUS} CPUs, UEFI, Secure Boot off)

  vm/test.sh fetch-arch-iso        Download and verify the stock Arch ISO (harness checks)
  vm/test.sh create [LAYOUT]       Create the test VM with blank disks and save the state 'blank'
                                     single  one ${SYSTEM_DISK} disk (default)
                                     multi   ${SYSTEM_DISK} system disk + ${DATA_DISK} data disk
  vm/test.sh boot [ISO] [options]  Boot ISO (default: newest out/*.iso, else the stock Arch ISO)
        --headless                   no window, runs in the background
        --gamepad auto|none|PATH     pass host gamepads to the VM (default: auto = all detected)
        --gl                         3D-accelerated GPU (needed for gamescope)
        --ssh                        live Arch ISO only: allow root SSH with the project test key
        --kargs "ARGS"               extra kernel command line (e.g. owneet.session=cage)
        --journal                    send the system journal to the serial log (vm/test.sh log)
        --debug-shell                root shell on the serial console, for vm/test.sh run (tests only)
  vm/test.sh gamepads              List gamepads connected to this computer
  vm/test.sh ssh [CMD]             Shell in the VM (after boot --ssh), or run CMD
  vm/test.sh wait-serial PATTERN [SECONDS]
                                   Wait until the serial console shows PATTERN (regex, default 180 s)
  vm/test.sh log                   Print the serial console log (escape codes removed)
  vm/test.sh screenshot [FILE]     Save the VM screen as PNG (default: vm/run/test/screen.png)
  vm/test.sh run 'CMD' [SECONDS]   Run CMD in the VM's debug shell and print its output (boot --debug-shell)
  vm/test.sh keys COMBO            Press keys in the VM, e.g. ctrl-alt-f9
  vm/test.sh stop                  Power the VM off
  vm/test.sh reset                 Bring every disk back to the blank state
  vm/test.sh destroy               Delete the test VM

While the VM runs, passed-through gamepads are grabbed by it and stop working on the host.
SSH listens on 127.0.0.1:${SSH_PORT} (override with OWNEET_TEST_SSH_PORT).
EOF
}

ssh_opts test "$SSH_PORT"
# Arguments form the remote command line on purpose.
# shellcheck disable=SC2029
vm_ssh() { ssh "${SSH_OPTS[@]}" root@127.0.0.1 "$@"; }

wait_for_ssh() {
    local timeout="$1" i
    for (( i = 0; i < timeout; i += 2 )); do
        if vm_ssh true 2>/dev/null; then return 0; fi
        sleep 2
    done
    return 1
}

default_iso() {
    local newest
    newest="$(find "$ROOT/out" -maxdepth 1 -name '*.iso' -printf '%T@ %p\n' 2>/dev/null | sort -rn | head -1 | cut -d' ' -f2- || true)"
    if [[ -n "$newest" ]]; then echo "$newest"; return; fi
    [[ -f "$ARCH_ISO" ]] || die "no ISO given, none in out/, and the stock Arch ISO is missing (vm/test.sh fetch-arch-iso)"
    echo "$ARCH_ISO"
}

cmd_create() {
    local layout="${1:-single}"
    case "$layout" in single|multi) ;; *) die "unknown layout '$layout' (single or multi)" ;; esac
    "$VM" create "$NAME" "$SYSTEM_DISK"
    if [[ "$layout" == multi ]]; then "$VM" add-disk "$NAME" "$DATA_DISK"; fi
    "$VM" snapshot "$NAME" blank
}

cmd_boot() {
    local iso="" headless=0 gamepad="auto" gl=0 ssh=0 kargs=""
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --headless) headless=1; shift ;;
            --gamepad)  gamepad="${2:-}"; shift 2 ;;
            --gl)       gl=1; shift ;;
            --ssh)      ssh=1; shift ;;
            --kargs)    kargs+=" ${2:-}"; shift 2 ;;
            --debug-shell) kargs+=" systemd.debug_shell=hvc0 systemd.mask=serial-getty@hvc0.service"; shift ;;
            --journal)  kargs+=" console=ttyS0,115200 systemd.journald.forward_to_console=1 systemd.journald.max_level_console=info"; shift ;;
            -*) die "unknown option '$1' (see: vm/test.sh help)" ;;
            *) iso="$1"; shift ;;
        esac
    done
    [[ -d "$VM_DIR/images/$NAME" ]] || die "the test VM does not exist. Create it with: vm/test.sh create"
    [[ -n "$iso" ]] || iso="$(default_iso)"

    local args=(--iso "$iso" --mem "$MEM_MB" --cpus "$CPUS")
    if (( headless )); then args+=(--headless); fi
    if (( gl )); then args+=(--gl); fi
    if [[ -n "${kargs# }" ]]; then args+=(--kernel-args "${kargs# }"); info "extra kernel arguments:${kargs}"; fi

    case "$gamepad" in
        none) ;;
        auto)
            local found=0 dev pname access
            while IFS=$'\t' read -r dev pname access; do
                [[ -n "$dev" ]] || continue
                if [[ "$access" == ok ]]; then
                    args+=(--evdev "$dev"); found=1
                    info "gamepad passed to the VM: $pname ($dev)"
                else
                    info "gamepad skipped, no access: $pname ($dev)"
                fi
            done < <(list_gamepads)
            if (( ! found )); then info "no gamepad detected on this computer; booting without one"; fi
            ;;
        *) args+=(--evdev "$gamepad") ;;
    esac

    if (( ssh )); then
        ensure_ssh_key test
        seed_start "$VM_DIR/run/$NAME-seed" "$SEED_PORT" "$VM_DIR/test/user-data.in" test owneet-test
        args+=(--ssh-port "$SSH_PORT" --seed-url "http://10.0.2.2:$SEED_PORT/")
    fi

    info "booting $(basename "$iso")"
    "$VM" start "$NAME" "${args[@]}"

    if (( ssh && headless )); then
        info "waiting for SSH into the live system"
        wait_for_ssh 240 || die "no SSH answer within 4 minutes; see vm/run/$NAME/serial.log"
        seed_stop
        info "live system reachable: vm/test.sh ssh"
    fi
}

serial_log() { sed -e 's/\x1b\[[0-9;=?]*[A-Za-z]//g' -e 's/\r//g' "$VM_DIR/run/$NAME/serial.log" 2>/dev/null; }

cmd_wait_serial() {
    local pattern="${1:-}" timeout="${2:-180}" i
    [[ -n "$pattern" ]] || die "usage: vm/test.sh wait-serial PATTERN [SECONDS]"
    for (( i = 0; i < timeout; i++ )); do
        if serial_log | grep -aqE -- "$pattern"; then
            info "serial console shows: $(serial_log | grep -aE -- "$pattern" | head -1)"
            return 0
        fi
        sleep 1
    done
    die "'$pattern' did not appear on the serial console within $timeout s (vm/test.sh log)"
}

cmd_run() {
    local command="${1:-}" timeout="${2:-60}"
    [[ -n "$command" ]] || die "usage: vm/test.sh run 'CMD' [SECONDS]"
    local sock="$VM_DIR/run/$NAME/console.sock"
    [[ -S "$sock" ]] || die "the test VM is not running"
    python3 - "$sock" "$VM_DIR/run/$NAME/console.log" "$command" "$timeout" <<'PY'
import re, socket, sys, time, uuid
sock_path, log_path, command, timeout = sys.argv[1], sys.argv[2], sys.argv[3], float(sys.argv[4])
tag = uuid.uuid4().hex[:8]
start = len(open(log_path, "rb").read())
s = socket.socket(socket.AF_UNIX)
s.connect(sock_path)
# The marker is split in the typed command so that only the command's own output matches it.
data = f"{command}; echo __OWNEET_END_\"\"{tag}__ $?\r".encode()
# The emulated UART has a 16-byte FIFO: send in small chunks or characters get lost.
for i in range(0, len(data), 8):
    s.sendall(data[i:i + 8])
    time.sleep(0.01)
# The emulated serial port occasionally duplicates a character, so match the marker loosely;
# the echoed command line never matches (its marker contains "").
end_re = re.compile(r"_+O+W+N+E+T+_+E+N+D+_+[0-9a-f]+_+ (\d+)")
deadline = time.time() + timeout
while time.time() < deadline:
    text = open(log_path, "rb").read()[start:].decode(errors="replace")
    # Remove terminal control sequences (CSI, and OSC such as systemd's "]3008;" context marks).
    text = re.sub(r"\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b\[[0-9;?]*[A-Za-z]|\r", "", text)
    m = end_re.search(text)
    if m:
        lines = text[:m.start()].split("\n")
        body = "\n".join(l for l in lines[1:] if "__OWNEET_END_" not in l).strip("\n")
        if body:
            print(body)
        sys.exit(int(m.group(1)))
    time.sleep(0.3)
sys.exit("error: no answer from the debug shell (was the VM booted with --debug-shell?)")
PY
}

case "${1:-help}" in
    fetch-arch-iso) verified_download "$ARCH_ISO_URL" "$ARCH_ISO" "$ARCH_ISO_SHA256" "$ARCH_ISO_KEY" \
                        "Arch Linux ISO $ARCH_ISO_VERSION (~1.6 GB)" ;;
    create)   shift; cmd_create "$@" ;;
    boot)     shift; cmd_boot "$@" ;;
    gamepads) out="$(list_gamepads)"
              if [[ -z "$out" ]]; then echo "no gamepads detected"; else
                  printf 'DEVICE\tNAME\tACCESS\n%s\n' "$out" | column -t -s $'\t'; fi ;;
    ssh)      shift; if [[ $# -gt 0 ]]; then vm_ssh "$@"; else ssh -t "${SSH_OPTS[@]}" root@127.0.0.1; fi ;;
    wait-serial) shift; cmd_wait_serial "$@" ;;
    screenshot) "$VM" screenshot "$NAME" "${2:-$VM_DIR/run/$NAME/screen.png}" ;;
    log)      serial_log ;;
    run)      shift; cmd_run "$@" ;;
    keys)     "$VM" keys "$NAME" "${2:-}" ;;
    stop)     "$VM" stop "$NAME" --force ;;
    reset)    "$VM" stop "$NAME" --force >/dev/null; "$VM" revert "$NAME" blank ;;
    destroy)  "$VM" destroy "$NAME" ;;
    help|-h|--help) usage ;;
    *) usage; exit 1 ;;
esac
