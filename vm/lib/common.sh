# SPDX-License-Identifier: GPL-3.0-or-later
#
# Shared helpers for vm/builder.sh and vm/test.sh. Source it; do not run it.
# Expects VM_DIR to be set by the caller.

CACHE_DIR="$VM_DIR/images/.cache"
KEYS_DIR="$VM_DIR/images/.keys"
GPG_DIR="$VM_DIR/images/.gnupg"
KEYSERVER="hkps://keyserver.ubuntu.com"

die() { echo "error: $*" >&2; exit 1; }
info() { echo "==> $*"; }

# verified_download URL DEST SHA256 KEY_FINGERPRINT [LABEL]
# Downloads URL and URL.sig, checks the SHA-256 and that the signature was made by
# KEY_FINGERPRINT (primary key). Uses a project-local keyring; the user's keyring is never touched.
verified_download() {
    local url="$1" dest="$2" sha="$3" key="$4" label="${5:-$(basename "$2")}"
    if [[ -f "$dest" ]]; then
        info "$label already downloaded"
        return
    fi
    mkdir -p "$(dirname "$dest")"
    mkdir -p -m 700 "$GPG_DIR"

    info "downloading $label"
    curl -fL --progress-bar -o "$dest.part" "$url"
    curl -fsSL -o "$dest.sig" "$url.sig"

    info "verifying checksum"
    echo "$sha  $dest.part" | sha256sum --check --quiet \
        || { rm -f "$dest.part"; die "checksum mismatch: the download is corrupted or tampered with"; }

    info "verifying signature (key $key)"
    if ! GNUPGHOME="$GPG_DIR" gpg --batch --quiet --list-keys "$key" >/dev/null 2>&1; then
        GNUPGHOME="$GPG_DIR" gpg --batch --quiet --keyserver "$KEYSERVER" --recv-keys "$key" \
            || die "could not fetch signing key $key from $KEYSERVER"
    fi
    local status
    status="$(GNUPGHOME="$GPG_DIR" gpg --batch --status-fd 1 --verify "$dest.sig" "$dest.part" 2>/dev/null || true)"
    grep -q "^\[GNUPG:\] VALIDSIG .* $key\$" <<<"$status" \
        || { rm -f "$dest.part"; die "signature check failed for $label"; }

    mv "$dest.part" "$dest"
    info "verified: $label"
}

# ensure_ssh_key NAME -> creates vm/images/.keys/NAME_ed25519 if missing
ensure_ssh_key() {
    local key="$KEYS_DIR/$1_ed25519"
    if [[ ! -f "$key" ]]; then
        mkdir -p -m 700 "$KEYS_DIR"
        ssh-keygen -q -t ed25519 -N "" -C "owneet-$1" -f "$key"
        info "created project SSH key vm/images/.keys/$1_ed25519 (your ~/.ssh is not used)"
    fi
}

# ssh_opts KEYNAME PORT -> fills the SSH_OPTS array
ssh_opts() {
    SSH_OPTS=(
        -i "$KEYS_DIR/$1_ed25519" -p "$2"
        -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR
        -o ConnectTimeout=5 -o BatchMode=yes
    )
}

# seed_start DIR PORT TEMPLATE KEYNAME INSTANCE
# Renders a cloud-init NoCloud seed from TEMPLATE (@SSH_KEY@ is replaced) and serves it on
# 127.0.0.1:PORT, seen by the VM as http://10.0.2.2:PORT/. Stop it with seed_stop.
SEED_SERVER_PID=""
SEED_DIR=""
seed_start() {
    local dir="$1" port="$2" template="$3" keyname="$4" instance="$5"
    command -v python3 >/dev/null || die "python3 is required (it serves the first-boot configuration)"
    rm -rf "$dir"; mkdir -p "$dir"
    sed "s|@SSH_KEY@|$(cat "$KEYS_DIR/${keyname}_ed25519.pub")|" "$template" > "$dir/user-data"
    printf 'instance-id: %s-%s\nlocal-hostname: %s\n' "$instance" "$(date +%s)" "$instance" > "$dir/meta-data"
    : > "$dir/vendor-data"
    python3 -m http.server --bind 127.0.0.1 --directory "$dir" "$port" >"$dir/server.log" 2>&1 &
    SEED_SERVER_PID=$!
    SEED_DIR="$dir"
    trap seed_stop EXIT
    sleep 1
    kill -0 "$SEED_SERVER_PID" 2>/dev/null || die "could not start the seed server on port $port"
}

seed_stop() {
    if [[ -n "${SEED_SERVER_PID:-}" ]]; then kill "$SEED_SERVER_PID" 2>/dev/null || true; fi
    if [[ -n "${SEED_DIR:-}" ]]; then rm -rf "$SEED_DIR"; fi
    SEED_SERVER_PID=""
    SEED_DIR=""
}

# list_gamepads -> one line per host gamepad: "EVDEV_PATH<TAB>NAME<TAB>ACCESS"
list_gamepads() {
    local ev props name access
    for ev in /dev/input/event*; do
        [[ -e "$ev" ]] || continue
        props="$(udevadm info -q property -n "$ev" 2>/dev/null || true)"
        grep -q '^ID_INPUT_JOYSTICK=1$' <<<"$props" || continue
        name="$(cat "/sys/class/input/$(basename "$ev")/device/name" 2>/dev/null || echo unknown)"
        if [[ -r "$ev" && -w "$ev" ]]; then access="ok"; else access="no-access"; fi
        printf '%s\t%s\t%s\n' "$ev" "$name" "$access"
    done
}
