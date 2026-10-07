# owneetd — design (draft for roadmap step 2.1)

Status: **draft, awaiting the owner's decisions** (language, transport, process split).
Nothing here is implemented yet.

`owneetd` is the OwneetOS system daemon (PROJECT_RULES.md section 4): it owns the controllers and
the Guide button, provides the on-screen keyboard's virtual input, manages Wi-Fi, Bluetooth, audio
and power, and launches and closes games and apps. The console UI (Pegasus fork) and later the
installer and the overlay talk to it.

## 1. Language: Go or Rust

Both produce a single binary, both are memory-safe, both have what `owneetd` needs. The table
compares what matters for this daemon.

| | Go | Rust |
|---|---|---|
| Static single binary | Trivial (`CGO_ENABLED=0`) | Possible (musl target); harder as soon as a C library is involved |
| Idle memory (typical daemon of this kind) | ~10–20 MB | ~2–6 MB |
| Binary size | ~8–15 MB | ~2–6 MB |
| D-Bus (NetworkManager, BlueZ, logind) | `godbus/dbus` (mature, pure Go) | `zbus` (excellent, pure Rust) |
| Bluetooth (BlueZ) | Plain D-Bus calls (BlueZ's API is small); `go-bluetooth` exists but is less maintained | `bluer`, the BlueZ project's own Rust library |
| Controllers (evdev) and virtual input (uinput) | `holoplot/go-evdev`, `bendahl/uinput` (pure Go) | `evdev` crate (input and virtual devices, pure Rust) |
| Audio (PipeWire) | PulseAudio protocol via `pipewire-pulse`: `jfreymuth/pulse` (pure Go, volume + events) | PulseAudio/PipeWire bindings wrap C libraries (breaks the pure static build), or a pure client written by us |
| HTTP server | Standard library | `hyper`/`axum` (more dependencies) |
| Build time in CI | Seconds | Minutes |
| Learning curve for new contributors | Low | High |

**Assessment.** Rust uses less memory and has the best Bluetooth library; Go is simpler to build,
to keep static, and to contribute to, and its pure-Go audio client avoids C bindings. The memory
difference (~10–15 MB) is about 2–3 % of the 500 MB idle target (C1). **Recommendation: Go**,
mainly for build simplicity and for the second developer and future contributors. Rust remains a
good choice if the owner prefers the lowest possible footprint.

## 2. Process split and privileges

`owneetd` runs as a **systemd user service of the console user `owneet`**, inside the console
session, **not as root**:

| Needs | How it works without root |
|---|---|
| Read controllers (evdev) | Console user is in the `input` group; logind already grants access to gamepads |
| Virtual keyboard/mouse (uinput) | udev rule giving the console session access to `/dev/uinput` |
| Wi-Fi (NetworkManager) | NetworkManager lets the active local session manage connections (polkit) |
| Bluetooth (BlueZ) | D-Bus policy / polkit rule for the console user |
| Audio (PipeWire) | PipeWire runs in the user session anyway: the daemon must be there too |
| Power (logind) | logind lets the active local session shut down, restart, suspend |
| Launch games and apps | Must run in the session, with the compositor's environment |

Operations that really need root later (installer, system updates, disk mounting policy) get their
own small privileged helpers with a narrow D-Bus/polkit interface, decided in their steps. A bug in
`owneetd` therefore cannot touch the system beyond what the console user can do.

## 3. Transport and security

**HTTP + JSON over a Unix socket** at `$XDG_RUNTIME_DIR/owneetd.sock` (`/run/user/1000/owneetd.sock`),
permissions `0600`, owned by `owneet`. No TCP port.

- Why not a localhost TCP port (as first written in the rules): any web page open in Brave could
  send requests to `http://127.0.0.1:PORT` (CSRF) and, for example, power the console off or
  connect to a Wi-Fi network. A Unix socket cannot be reached by web pages at all.
- The console UI is our Pegasus fork: a small C++ QML plugin (`QLocalSocket`) talks to the socket
  (decided in step 3.8, `docs/frontend-architecture.md`).
- `owneetctl`, a tiny command-line client, is used by scripts and tests (`owneetctl get /v1/status`).
- Only processes of the console user can connect. No tokens needed.

## 4. API v1 (draft)

Conventions: JSON bodies; `/v1` prefix; errors as `{"error": {"code": "wifi.wrong_password",
"message": "…"}}` with a proper HTTP status. **`code` is stable and is what the UI translates**
(no UI text comes from the daemon — PROJECT_RULES.md section 9). `message` is English, for logs.

### Status

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/status` | Daemon version, OwneetOS version, session mode (gamescope/cage) |

### Controllers

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/controllers` | Connected controllers: id, name, brand, connection (usb/bluetooth/dongle), battery |

### Bluetooth

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/bluetooth` | Adapter present/powered, known and connected devices |
| POST | `/v1/bluetooth/auto-pair` | `{"enabled": true, "seconds": 120}` — scan, pair and trust any gamepad, no input (section 7) |
| POST | `/v1/bluetooth/devices/{address}/connect` · `/disconnect` | |
| DELETE | `/v1/bluetooth/devices/{address}` | Forget a device |

### Network

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/network` | Connectivity (none/limited/full), active connection, signal |
| POST | `/v1/network/wifi/scan` | Start a scan (results arrive as an event and via GET below) |
| GET | `/v1/network/wifi` | Visible networks: SSID, signal, security, known |
| POST | `/v1/network/wifi/connect` | `{"ssid": "…", "password": "…"}` |
| DELETE | `/v1/network/wifi/{ssid}` | Forget a network |

### Audio

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/audio` | Outputs (speakers, HDMI, headset…), default output, volume, mute |
| PUT | `/v1/audio/volume` | `{"volume": 0–100}` |
| PUT | `/v1/audio/mute` | `{"muted": true}` |
| PUT | `/v1/audio/output` | `{"id": "…"}` — default output |

### Power

| Method | Path | Purpose |
|---|---|---|
| POST | `/v1/power/shutdown` · `/restart` · `/suspend` | |

### Apps and games

| Method | Path | Purpose |
|---|---|---|
| POST | `/v1/apps/launch` | `{"id": "…", "command": ["…"], "kind": "game"/"app"}` — start inside the session |
| GET | `/v1/apps` | Running apps and games |
| POST | `/v1/apps/{id}/close` | Ask to close (SIGTERM / window close) |
| POST | `/v1/apps/{id}/kill` | Force-close a frozen game |

### Virtual input (on-screen keyboard)

| Method | Path | Purpose |
|---|---|---|
| POST | `/v1/input/text` | `{"text": "…"}` — typed into the focused app |
| POST | `/v1/input/key` | `{"key": "enter"}` and other special keys |

### Event stream

`GET /v1/events` keeps the connection open and sends one JSON object per event (Server-Sent
Events format). Event types (initial list):

`guide.pressed`, `controller.added`, `controller.removed`, `controller.battery`,
`bluetooth.changed`, `bluetooth.paired`, `network.changed`, `wifi.scan_done`, `audio.changed`,
`app.started`, `app.exited`, `xone.firmware_needed`, `power.changed`.

## 5. Open points for later steps

- Focus switching inside gamescope (bring the UI back on Guide) — step 2.9.
- Which privileged helpers exist and their interfaces — installer (6.3), updates (7.1), disks (6.5).
- Configuration file format and location — step 2.2.
