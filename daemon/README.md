# daemon/

**`owneetd`**, the OwneetOS system daemon, and **`owneetctl`**, its command-line client. Go,
standard library only for now. Design and API: [`docs/daemon-design.md`](../docs/daemon-design.md).

- Runs as a **systemd user service** of the console user `owneet` (not root).
- Serves **HTTP + JSON on a Unix socket**: `$XDG_RUNTIME_DIR/owneetd.sock`, mode `0600`.
- Optional configuration: `/etc/owneet/owneetd.json` (`{"log_level": "debug"}`), or the
  environment variable `OWNEETD_LOG_LEVEL`.
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

## Build and test

Inside the builder VM (Go is installed there, not on the host):

```text
tools/build-in-vm 'cd daemon && go test ./...'
tools/build-in-vm tools/build-packages owneetd
```

`tools/lint` also runs `gofmt`, `go vet` and `go test` (locally and in CI).

Roadmap: phase 2.
