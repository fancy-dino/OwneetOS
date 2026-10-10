# owneetd — design (draft for roadmap step 2.1)

Status: **approved by the owner on 2026-10-07**: Go, HTTP over a Unix socket, user service (not root).
The API list is a draft that grows step by step (phase 2).

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
| Wi-Fi (NetworkManager) | NetworkManager lets the active local session scan, connect and switch Wi-Fi (polkit). Saving a network for the whole system needs one polkit rule (checked in 2.6, see Network) |
| Bluetooth (BlueZ) | BlueZ's default D-Bus policy already lets any local user call it (checked in 2.5): no extra rule |
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
| GET | `/v1/status` | **Implemented (2.2).** Daemon version, OwneetOS version, session mode (gamescope/cage) |

### Controllers

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/controllers` | **Implemented (2.3).** Connected controllers: id, name, brand, connection (usb/bluetooth/dongle), Guide available, battery |

The **Guide button** is `BTN_MODE` when the kernel driver reports it (Xbox, PlayStation, Nintendo
and most others); otherwise the `guide` entry of SDL_GameControllerDB tells which button it is.
Some Bluetooth controllers send it as `KEY_HOMEPAGE` on a second input device with the same
unique id: that device is followed too. Devices are never grabbed.

### Bluetooth — implemented (2.5)

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/bluetooth` | `adapter`, `powered`, `discovering`, `auto_pair`, `devices` (address, name, paired, trusted, connected, gamepad, battery) |
| POST | `/v1/bluetooth/auto-pair` | `{"enabled": true, "seconds": 120}` (default 120, max 600) — scan, pair, trust and connect any gamepad, no input |
| POST | `/v1/bluetooth/devices/{address}/connect` · `/disconnect` | Connect or disconnect a known device |
| DELETE | `/v1/bluetooth/devices/{address}` | Forget (unpair) a device |

Errors: `bluetooth.unavailable` (503, no system bus), `bluetooth.no_adapter` (409),
`bluetooth.unknown_device` (404), `bluetooth.invalid_address` (400), `bluetooth.failed` (502, BlueZ
refused; the message says why).

**First pairing with zero input** (PROJECT_RULES.md section 7): whenever **no controller is
connected** (USB, dongle or Bluetooth), auto-pair turns on by itself — the adapter is powered, a scan
runs, and every device that **identifies as a gamepad** is paired, trusted and connected. It turns
off as soon as a controller is connected. The UI can also turn it on for a while (pairing a second
controller). Configuration: `bluetooth_autopair_without_controller` (default `true`).

A device is a gamepad when its Bluetooth Classic class of device is "peripheral / joystick or
gamepad", its Bluetooth LE appearance is joystick or gamepad, or BlueZ's icon is `input-gaming`.

owneetd registers a BlueZ **pairing agent** with the `NoInputNoOutput` capability (nobody can type
or compare a code). It accepts a pairing request **only from a gamepad while auto-pair is on** and
rejects everything else, so keyboards, phones or headsets are never paired without the user. The
agent is registered again when bluetoothd starts or restarts. Trusted devices reconnect by themselves.
Bluetooth controllers are listed in `/v1/controllers` with BlueZ's device name (Bluetooth LE
controllers otherwise appear as `bluez-hog-device`).

D-Bus library: `github.com/godbus/dbus/v5` (BSD-2-Clause), **vendored** in `daemon/vendor/` with
its dependency `golang.org/x/sys` (BSD-3-Clause): the package build needs no network and the source
archive is complete.

### Network — implemented (2.6)

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/network` | `available` (NetworkManager running), `state`, `connectivity` (full/limited/portal/none/unknown), `wifi` (present, enabled, hardware_enabled, state, ssid, strength), `ethernet` (present, connected) |
| GET | `/v1/network/wifi` | Visible networks: `ssid`, `strength`, `security` (open/wpa/wpa3/wep/enterprise), `known` (saved), `connected`; connected first, then saved, then by signal |
| POST | `/v1/network/wifi/scan` | Start a scan (202); `wifi.scan_done` follows |
| POST | `/v1/network/wifi/connect` | `{"ssid": "…", "password": "…"}`; waits for the result (up to 60 s) |
| POST | `/v1/network/wifi/disconnect` | Disconnect Wi-Fi |
| DELETE | `/v1/network/wifi/{ssid}` | Forget a saved network (SSID URL-encoded) |
| PUT | `/v1/network/wifi/enabled` | `{"enabled": false}` — Wi-Fi radio off/on |

Errors: `network.unavailable` (503), `network.no_wifi` / `network.wifi_disabled` (409),
`network.not_found` (404, not visible), `network.unknown_network` (404, not saved),
`network.unsupported_security` (422, WEP or enterprise), `network.password_required` (422),
`network.invalid_password` (400, WPA: 8–63 characters or 64 hex digits),
`network.wrong_password` (422), `network.timeout` (504), `network.connect_failed` (502).

- **Connecting:** without a password, a saved network is reused; with one, a new saved network is
  created and replaces the old one **only if it works** (a wrong password never leaves a broken
  saved network behind). WPA/WPA2 (and WPA2/WPA3 transition) networks use `wpa-psk`, WPA3-only
  networks `sae`. Hidden networks are not listed (later, if needed).
- **Wrong password:** NetworkManager has no secret agent to ask, so a refused password ends the
  attempt at once with reason `no-secrets`, read from the device's `StateChanged` signal.
- **Saved networks** are system-wide (`/etc/NetworkManager/system-connections`, readable by root
  only), so they also work before login and for system services. NetworkManager allows this only
  to administrators, so the package `owneetd` ships one polkit rule,
  `50-owneet-networkmanager.rules`: the action `settings.modify.system` is allowed to the user
  `owneet` only, from the active local session only. `owneet` is **not** an administrator.
- Passwords are never logged and never returned by the API.
- Tested in the test VM with simulated Wi-Fi (`mac80211_hwsim`, see `vm/README.md`).

### Audio — implemented (2.7)

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/audio` | `available`, default `output` (id), its `volume` (0–100) and `muted`; `outputs`: id, name, kind (speakers, headphones, hdmi, bluetooth, usb, other), default, volume, muted |
| PUT | `/v1/audio/volume` | `{"volume": 0–100}` — default output, all channels alike |
| PUT | `/v1/audio/mute` | `{"muted": true}` |
| PUT | `/v1/audio/output` | `{"id": "…"}` — default output; sounds already playing move to it |

Errors: `audio.unavailable` (503, no audio server), `audio.no_output` (409),
`audio.unknown_output` (404), `audio.failed` (502).

- **How:** PipeWire's PulseAudio protocol (`pipewire-pulse`, socket `$XDG_RUNTIME_DIR/pulse/native`)
  through `github.com/jfreymuth/pulse/proto` (MIT, pure Go, vendored). owneetd subscribes to
  output, server and card changes, so changes made by any program produce `audio.changed`; it
  reconnects when PipeWire restarts. Connecting starts PipeWire (socket activation) at login.
- **Volume** uses the same scale as desktop mixers (PulseAudio's, perceived loudness), so 50 here
  is 50 in any other tool. Above 100 is not offered.
- **Outputs** are the ones PipeWire/WirePlumber expose for the active card profiles; headphones
  plugged into a jack switch automatically (WirePlumber). Choosing among card profiles (for cards
  where HDMI and analog outputs exclude each other) is left for later, if real hardware needs it.
- WirePlumber remembers volume and default output across restarts.
- Tested in the test VM with two virtual sound cards (`vm/test.sh boot --audio`).

### Power — implemented (2.8)

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/power` | `can_shutdown`, `can_restart`, `can_suspend` |
| POST | `/v1/power/shutdown` · `/restart` · `/suspend` | 202 once systemd-logind accepted it |

Errors: `power.unavailable` (503), `power.unsupported` (409, e.g. no suspend support),
`power.not_allowed` (403, a password would be needed), `power.inhibited` (409, a program blocks
it), `power.failed` (502).

- systemd-logind allows these actions to the active local session without a password (checked
  in 2.8): no extra polkit rule. owneetd never asks for a password (`interactive=false`).
- Events from logind's signals: `power.suspending`, `power.resumed` (the UI can, for example,
  refresh the controller list), `power.shutting_down`.
- Hibernation is not offered (no swap, and resume times are long).
- Tested in the test VM: suspend (QEMU reports the machine suspended; woken through QMP), restart,
  shutdown.

### Apps and games — implemented (2.9)

| Method | Path | Purpose |
|---|---|---|
| POST | `/v1/apps/launch` | `{"id": "…", "name": "…", "kind": "game"/"app", "command": ["…"], "workdir": "/…"}` — start inside the session (201); `workdir` (optional, an existing absolute folder, default the home folder) is where the program starts (3.8) |
| GET | `/v1/apps` | Running apps and games, and `focus`: `home`, an app id, or empty (cage, or no session) |
| POST | `/v1/apps/{id}/focus` | Bring an app to the front ("Resume" on the home screen; gamescope only) |
| POST | `/v1/apps/{id}/close` | Close: SIGTERM to all its processes, SIGKILL after 10 seconds |
| POST | `/v1/apps/{id}/kill` | Force-close a frozen game at once (SIGKILL to all its processes) |

Errors: `apps.unavailable` (503), `apps.invalid` (400), `apps.program_not_found` (400),
`apps.no_session` (409), `apps.already_running` (409, same id), `apps.game_running` (409, one game
at a time; apps such as the browser may run next to it), `apps.unknown_app` (404),
`apps.focus_unsupported` (409, cage), `apps.no_window` (409), `apps.failed` (502).

- **Session:** `owneet-session` starts the console UI through `owneet-session-app`, which writes
  `$XDG_RUNTIME_DIR/owneet/session-env` (display variables, session mode, process id of the home
  screen). Apps get those variables, so they open in the console session.
- **Each app runs in its own transient systemd user service** (`owneet-app-ID.service`, created
  over the user's D-Bus): closing it stops every process it started (launchers, Proton, wine), its
  output goes to the journal, and apps still running are found again when owneetd restarts.
- **gamescope** runs with `--steam`. It then shows only windows tagged with an app id
  (`STEAM_GAME`) and lets the shell choose which app is on screen, as Steam does on SteamOS.
  owneetd watches new windows on gamescope's X server (`github.com/jezek/xgb`, BSD-3-Clause,
  vendored), tags the home screen and each app (by process: the home screen's process id, or the
  app's systemd unit from `/proc/PID/cgroup`), and sets `GAMESCOPECTRL_BASELAYER_APPID`: a new app
  comes to the front as soon as its window appears, with the home screen as fallback. App ids
  are derived from the app's id, so they survive an owneetd restart.
- **Guide** (owned by the system, PROJECT_RULES.md section 9.1): in gamescope it toggles between
  the home screen and the app last shown; the game keeps running behind the home screen. In cage,
  which shows only the newest window, holding Guide for 2 seconds closes the app on screen; a short
  press does nothing. `guide.pressed` and `guide.released` are still sent to clients.
- Events: `app.started`, `app.exited` (with systemd's result: `success`, `exit-code`, `signal`,
  `timeout`…), `focus.changed`.
- Tested in the test VM: gamescope with its headless backend (test hook
  `owneet.debug.gamescope_backend=headless`) and cage, a virtual gamepad for Guide, `mpv` as the
  test app.

### Display — implemented (3.9, part 3)

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/display` | `available` (gamescope session), `screens` (`id`, `connector`, `name`, `internal`, `active`), `modes` of the screen in use (`"1920x1080@60"`, preferred first; none for a laptop's built-in screen), `mode` (`auto` or one of them), `brightness` (0-100, only when the screen allows it), `pending` (`kind`, `seconds`) |
| PUT | `/v1/display/screen` | `{"id": "card1-HDMI-A-1"}` — move the console to that screen (gamescope restarts; refused while a game or app is open) |
| PUT | `/v1/display/mode` | `{"mode": "1920x1080@60"}` or `"auto"` — applied at once |
| POST | `/v1/display/confirm` · `/revert` | Keep the change waiting for confirmation, or go back now |
| PUT | `/v1/display/brightness` | `{"value": 0-100}` |

Errors: `display.unavailable` (503), `display.not_gamescope` (409), `display.unknown_screen`
(404), `display.unknown_mode` (400), `display.apps_open` (409), `display.not_pending` (409),
`display.no_brightness` (409), `display.failed` (502).

- **One screen at a time** (PROJECT_RULES.md, decision log 2026-10-10). gamescope drives one
  screen and turns off the others on its graphics card. The screen and its card are chosen when
  gamescope starts (`--prefer-output`, `--prefer-vk-device`): owneetd writes them to
  `~/.config/owneet/display.conf`, which `owneet-session` reads, and stops gamescope (a marker
  file tells `owneet-session` the stop was wanted). Screens are read from sysfs
  (`/sys/class/drm`: status, EDID names with hwdata's `pnp.ids`, the card's PCI ids).
- **Other graphics cards:** `owneet-screens-off@CARD.service` (root, `/usr/lib/owneet`) becomes DRM
  master of every card but the console's, turns their outputs off and holds them until stopped.
  owneetd starts and stops it over systemd's D-Bus API; a polkit rule allows only these units,
  only start and stop, only to the console user in the active local session.
- **Modes**, as Steam does on SteamOS: gamescope publishes the screen's modes in
  `GAMESCOPE_DISPLAY_MODE_LIST_EXTERNAL`; owneetd writes the choice to gamescope's
  `GAMESCOPE_MODE_SAVE_FILE` (`~/.config/owneet/gamescope-modes`, "Make Model:WxH@Hz") and sets
  `GAMESCOPE_DISPLAY_MODE_NUDGE`: gamescope applies it at once and keeps it for that screen.
- **"Keep this?":** a new screen or mode goes back after 15 seconds without `confirm`; for a new
  screen the time starts once the console is back on screen. Events `display.pending`
  (`kind`, `seconds`), `display.confirmed`, `display.reverted`, `display.changed` (the status, also
  on hot-plug).
- **Brightness:** a laptop's backlight through logind (`Session.SetBrightness`, allowed to the
  active session's user); monitors through DDC/CI with `ddcutil` (its udev rule gives the active
  session the I2C devices; monitors are found in the background, values are sent one at a time).
- **Keyboard layout:** `PUT /v1/input/layout` also sets the layout of physical keyboards:
  `GAMESCOPE_KEYBOARD_LAYOUT` at once, `XKB_DEFAULT_LAYOUT` (display.conf) when the compositor
  starts — the only way in cage.
- Tested in the test VM: gamescope (headless backend) recompiles its keymap after a layout change,
  `owneet-screens-off` turns the VM's screen off and gives it back, `owneet-session` restarts
  gamescope without counting a failure, the polkit rule refuses other units and verbs.

### Virtual input (on-screen keyboard) — implemented (2.4)

| Method | Path | Purpose |
|---|---|---|
| POST | `/v1/input/text` | `{"text": "…"}` — typed into the focused app. If one character is not available in the keyboard layout nothing is typed: `422 input.unsupported_character` |
| POST | `/v1/input/key` | `{"key": "enter", "modifiers": ["ctrl"]}` — keys: enter, backspace, tab, escape, space, delete, home, end, up, down, left, right, pageup, pagedown; modifiers: ctrl, shift, alt, meta |
| POST | `/v1/input/pointer` | `{"dx": 10, "dy": -5, "wheel": 0}` — move the pointer / scroll |
| POST | `/v1/input/click` | `{"button": "left"}` (left, right, middle) |
| GET / PUT | `/v1/input/layout` | `{"layout": "it"}` — keyboard layout used to type text (`us`, `it`) |

A virtual keyboard sends **keys, not characters**: the character depends on the XKB layout of the
console session. `owneetd` therefore keeps its own layout tables (US and Italian for Wave 1) and
must use the same layout as the session (setting `keyboard_layout`, later driven by the language
chosen at first boot). Access to `/dev/uinput` is granted to the console session by a udev
`uaccess` rule, so owneetd stays unprivileged.

### Event stream

`GET /v1/events` keeps the connection open and sends one JSON object per event (Server-Sent
Events format). Event types (initial list):

`guide.pressed`, `controller.added`, `controller.removed`, `controller.battery` (implemented, 2.3),
`bluetooth.auto_pair`, `bluetooth.pairing`, `bluetooth.paired`, `bluetooth.pair_failed`,
`bluetooth.forgotten` (implemented, 2.5), `network.changed`, `network.connecting`,
`network.connect_failed`, `network.forgotten`, `wifi.scan_done` (implemented, 2.6), `audio.changed`
(implemented, 2.7), `power.suspending`, `power.resumed`, `power.shutting_down` (implemented, 2.8), `app.started`,
`app.exited`, `focus.changed`, `guide.released` (implemented, 2.9), `input.layout_changed`
(implemented, 2.4), `display.changed`, `display.pending`, `display.confirmed`, `display.reverted`
(implemented, 3.9), `xone.firmware_needed` (later).

## 5. Open points for later steps

- The PC's power button keeps logind's default (shut down) for now; a console-like behaviour
  (e.g. short press = suspend) is decided with the UI.
- When Steam runs inside the session (store, login: phase 4) it also wants to choose what
  gamescope shows: decide then how owneetd and Steam share it.
- Which privileged helpers exist and their interfaces — installer (6.3), updates (7.1), disks (6.5).
- Configuration file format and location — step 2.2.
