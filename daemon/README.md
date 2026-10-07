# daemon/

**`owneetd`**, the OwneetOS system daemon, and **`owneetctl`**, its command-line client. Go;
the only external module is `github.com/godbus/dbus/v5` (D-Bus, BSD-2-Clause), vendored in
`vendor/` with `golang.org/x/sys`. Update it in the builder VM with `go get`, `go mod tidy`,
`go mod vendor`, and copy `go.mod`, `go.sum` and `vendor/` back. Design and API: [`docs/daemon-design.md`](../docs/daemon-design.md).

- Runs as a **systemd user service** of the console user `owneet` (not root).
- Serves **HTTP + JSON on a Unix socket**: `$XDG_RUNTIME_DIR/owneetd.sock`, mode `0600`.
- Optional configuration: `/etc/owneet/owneetd.json`, e.g. `{"log_level": "debug",
  "keyboard_layout": "it", "bluetooth_autopair_without_controller": true}`, or the environment variable `OWNEETD_LOG_LEVEL`.
- Needs access to `/dev/uinput` for the virtual keyboard and mouse: granted to the console session
  by `70-owneet-uinput.rules` (package `owneetd`).
- Saves Wi-Fi networks system-wide through NetworkManager, allowed by the polkit rule
  `50-owneet-networkmanager.rules` (package `owneetd`).
- Logs go to the journal: `journalctl --user -u owneetd`.

## Layout

| Path | Content |
|------|---------|
| `cmd/owneetd` | the daemon |
| `cmd/owneetctl` | the client (`owneetctl get /v1/status`, `owneetctl events`) |
| `internal/api` | HTTP server, routes, JSON errors, event stream |
| `internal/events` | event broker |
| `internal/config` | configuration loading |
| `internal/client` | HTTP client over the Unix socket |
| `internal/gamepad` | controller discovery and hot-plug, Guide button, battery |
| `internal/evdev` | reading input devices (no grab: games keep their input) |
| `internal/uinput` | virtual input devices (test controllers, virtual keyboard and mouse) |
| `internal/vinput` | virtual keyboard and mouse, keyboard layouts (US, Italian) |
| `internal/sdldb` | SDL_GameControllerDB: Guide button of generic controllers |
| `internal/bluetooth` | BlueZ over D-Bus: devices, automatic gamepad pairing, pairing agent |
| `internal/network` | NetworkManager over D-Bus: status, Wi-Fi scan, connect, forget |
| `vendor/` | vendored Go modules (third-party code, not linted) |

## Build and test

Inside the builder VM (Go is installed there, not on the host):

```text
tools/build-in-vm 'cd daemon && go test ./...'
tools/build-in-vm tools/build-packages owneetd
```

`tools/lint` also runs `gofmt`, `go vet` and `go test` (locally and in CI), plus integration tests
that create **virtual controllers** through `/dev/uinput` (`go test -tags uinput`, run as root).

Roadmap: phase 2.
