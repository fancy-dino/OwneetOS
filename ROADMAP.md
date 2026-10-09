# OwneetOS — Roadmap

> Step-by-step plan for the whole project. Rules and decisions live in
> [PROJECT_RULES.md](PROJECT_RULES.md); if this file and the rules disagree, the rules win and this
> file gets fixed.

## How we work

1. **One step per prompt**, in the order below. A step marked *(multi-prompt)* is expected to need
   more than one prompt; it is still reviewed as one unit.
2. At the end of every step the assistant reports what was done, how it was verified, and anything
   that deviated from the plan. The step is marked done only after the **owner approves it**.
3. Steps marked **Owner input** stop and ask before acting (installing software on the host,
   creating remote resources, choices with long-term impact).
4. Any change of plan is recorded in the PROJECT_RULES.md decision log and reflected here, so this
   file always shows the current plan.
5. Steps may be split, merged or reordered when reality requires it. That is expected and is
   always done openly, never silently.

Status: `[x]` done · `[~]` in progress · `[ ]` to do · `[?]` needs a decision before it can start

---

## Phase 0 — Foundations

### [x] 0.1 Project rules

- **Deliverable:** `PROJECT_RULES.md`, `CLAUDE.md`.

### [x] 0.2 Wave 1 UI mockups

- **Deliverable:** `design/mockups/wave1-mockups.html` (first boot, home, library, settings).
- **Outcome:** direction approved; palettes provisional; input conflict fixed (LT/RT for filters).

### [x] 0.3 Roadmap and global controller input map

- **Deliverable:** this file; section 9.1 of the rules.

### [x] 0.4 Repository skeleton

- **Goal:** a clean, documented repository that another developer can join.
- **Deliverables:** `git init`; directory layout (`iso/`, `packages/`, `daemon/`, `frontend/`,
  `extension/`, `installer/`, `tools/`, `vm/`, `docs/`, `design/`); `README.md`, `LICENSE`
  (GPLv3), `CONTRIBUTING.md` (English only, commit style, one-step workflow), `.gitignore`
  (VM images, caches, build output), `.editorconfig`.
- **Done when:** the tree is committed locally and every folder has a short README stating its purpose.

### [x] 0.5 GitHub repository — **Owner input**

- **Goal:** public repository on the account connected to VS Code.
- **Deliverables:** remote repo, first push, branch protection on `main`, issue labels per phase.
- **Owner input:** confirm repository name and visibility before creation; invite the second developer.
- **Outcome:** published by the owner from VS Code at `github.com/fancy-dino/OwneetOS`; ruleset on
  `main` blocks deletion and force push. Issue labels postponed (created when `gh` is available or
  when the first issues are opened).

### [x] 0.6 Host virtualization tools — **Owner input**

- **Goal:** run VMs on the host without touching anything else.
- **Deliverables:** install `qemu-system-x86`, `ovmf`, `qemu-utils` (asked at the moment);
  `vm/` scripts that keep every disk image, firmware variable store and log inside the project folder.
- **Done when:** a throwaway UEFI VM boots and is removed cleanly.
- **Outcome:** packages installed by the owner (QEMU 8.2.2, OVMF 2024.02); `vm/vm.sh` created;
  `vm/vm.sh selftest` boots UEFI firmware under KVM in about 1 s and removes every file.
  `qemu-system-gui` added afterwards (VM windows, 3D acceleration for step 0.8); owner tested a VM in a window.

### [x] 0.7 Arch builder VM *(multi-prompt)*

- **Goal:** an Arch Linux VM where ISOs and packages are built (archiso needs root; it gets it only inside the VM).
- **Deliverables:** builder VM from the official Arch cloud image, provisioned by script
  (cloud-init), project folder shared into the VM (virtiofs or 9p), `tools/build-in-vm` wrapper.
- **Done when:** one command on the host runs a build inside the VM and returns the output to the project folder.
- **Outcome:** `vm/builder.sh` (setup / start / stop / ssh / destroy) and `tools/build-in-vm`.
  Official Arch cloud image 20261001.604814, verified by SHA-256 and arch-boxes signature; files are
  copied with rsync instead of a shared folder (archiso needs a native filesystem). Verified: a command
  run through `tools/build-in-vm` writes `out/` back on the host; failures return a non-zero exit code;
  cold start in ~9 s; clean ACPI shutdown. The builder itself follows current Arch (not pinned):
  pinning applies to the ISO contents (step 1.1).

### [x] 0.8 Test VM harness

- **Goal:** boot any built ISO the same way every time.
- **Deliverables:** UEFI test VM script with blank virtual disks (single disk, multi-disk,
  "Windows-like" NTFS disk for later), controller passthrough (USB passthrough or evdev), serial
  log capture, snapshot/reset.
- **Done when:** the stock Arch ISO boots in the harness and a host gamepad reaches the guest.
- **Progress:** `vm/test.sh` + shared `vm/lib/common.sh`. Stock Arch ISO 2026.10.01 (verified by
  checksum and release signature) boots in UEFI mode in ~45 s with 4 GB RAM, 4 CPUs, 32G + 64G disks;
  `reset` returns to blank disks; `--gl` gives a virgl GPU (OpenGL only with QEMU 8.2, no Vulkan).
  Gamepad passthrough verified with the owner's Xbox One controller over Bluetooth: the guest sees
  it with the same name and USB IDs (`js0`), and received A/B/X/Y, LB/RB, both triggers, both sticks
  (full range) and the D-pad.
  NTFS test disk moved to step 6.5, where it is first needed.

### [x] 0.9 CI skeleton

- **Goal:** GitHub Actions that lint and build what exists.
- **Deliverables:** workflow for linting (shell, Markdown, later Go/Rust and QML), placeholder ISO job;
  extend the `main` ruleset to require pull requests and passing checks (done by the owner on the website).
- **Progress:** `tools/lint` (shellcheck, SPDX headers, markdownlint 0.23.3) passes in the builder VM;
  `.github/workflows/ci.yml` runs it on every push and pull request, plus a placeholder ISO job.
  Actions pinned to commit SHAs. Builder provisioning now also installs `shellcheck`, `nodejs`, `npm`.
- **Done when:** CI is green on `main`.

---

## Phase 1 — Minimal bootable OS (no UI yet)

### [x] 1.1 archiso profile

- **Deliverables:** `iso/` profile derived from Arch `baseline`, minimal package list, pinned
  Arch Linux Archive snapshot date, OwneetOS branding in `os-release`.
- **Done when:** the ISO builds in the builder VM and boots to a TTY in the test VM.
- **Outcome:** `iso/` profile (UEFI only, systemd-boot; packages `base`, `linux`, `mkinitcpio`,
  `mkinitcpio-archiso`; Arch Linux Archive snapshot 2026-10-03) and `tools/build-iso`. Build takes
  about 2 minutes; ISO is 471 MiB. In the test VM the systemd-boot menu shows "OwneetOS" and the
  system reaches `OwneetOS 7.2.8-arch1-2 (ttyS0)` / `owneet login:`. Added `vm/test.sh wait-serial`
  and `vm/test.sh log` for automated boot checks.

### [x] 1.2 Own package repository `[owneet]`

- **Deliverables:** `packages/` with PKGBUILD layout, build script (clean chroot in the builder
  VM), repository database, package signing key, repo consumed by the ISO build.
- **Owner input:** where the signing key is stored and who holds it.
- **Done when:** a dummy `owneet-base` package is built, signed and installed into the ISO.
- **Progress:** `tools/build-packages` builds `packages/*` in a clean chroot on the ISO's Arch
  snapshot and creates `out/repo/x86_64/owneet.db`; `tools/build-iso` adds the local `[owneet]`
  repository to a copy of the profile. `owneet-base` 0.0.1 is built and installed in the ISO, which
  still boots to the login prompt. `tools/keys/create-signing-key` written and tested with a
  throwaway key. Real key created by the owner (`B3BD…456E`). `owneet-keyring` package added
  (built before `owneet-base`, dependency order). `tools/keys/install-working-key` and
  `tools/sign-repo` written; negative test passed (repository signed with a fake key: ISO build
  refused at database sync); stale cached packages fixed. The owner signed the repository with
  `tools/sign-repo`: all signatures valid (subkey `61D8…3C37`); the ISO built from it verified them
  (`SigLevel = Required DatabaseRequired`), contains `owneet-base` and `owneet-keyring`, and boots
  to the login prompt.

### [x] 1.3 Console session

- **Deliverables:** autologin user, session launcher that starts **gamescope** with a
  placeholder fullscreen app; Vulkan capability check with automatic **cage** fallback;
  restart-on-crash.
- **Done when:** the ISO boots straight into a fullscreen placeholder in both modes (forced fallback tested).
- **Outcome:** `owneet-session` package (user `owneet`, getty autologin, Vulkan check, gamescope or
  cage, restart on crash, fallback, messages only in the journal). Verified in the test VM with
  screenshots: auto mode picks cage (software-only Vulkan) and shows the fullscreen placeholder;
  forced gamescope fails on the VM's software Vulkan ("not a valid physical device") and falls back
  to cage after 2 attempts; with the placeholder quitting every 5 s the session restarts it, and
  after 3 quick failures shows a plain error message and retries after 60 s. **Not verifiable in
  the VM:** gamescope actually running. It needs a hardware Vulkan driver (step 1.5) and is tested
  on the owner's PC (RTX 4060 + Intel UHD 770) booting the live ISO at the end of step 1.5.
  New tools: `tools/update-checksums`, `vm/test.sh screenshot`, `--kargs`, `--journal`.

### [x] 1.4 Boot experience

- **Deliverables:** systemd-boot config, Plymouth theme with the OwneetOS mark, quiet kernel
  parameters, no text on screen in normal boots.
- **Done when:** power-on to placeholder shows only the logo.
- **Outcome:** `owneet-branding` package (wordmark generated from Bricolage Grotesque by
  `tools/branding/make-wordmark`; Plymouth theme `owneet`: wordmark + coral spinner on deep navy),
  silent kernel/systemd parameters, hidden boot menu, silent autologin. Recorded the boot in the test
  VM (screenshot every 0.4 s): after the VM firmware, only the splash (2.4–9.3 s), then a dark
  screen without text (about 4 s in the VM, software rendering), then the console session.
  Problems found and fixed: Plymouth fell back to its text splash (serial console; theme images
  linked instead of copied); keeping the splash on screen until the UI (`--retain-splash`) delayed
  autologin by 30 s, so it was removed. **Left for step 3.2:** a seamless splash-to-UI handover
  (no dark gap). The owner approved the splash with the wordmark changed to "Owneet" + coral "OS".

### [x] 1.5 Hardware support set

- **Deliverables:** firmware, Mesa + Vulkan drivers, `nvidia-open` with automatic detection,
  `xone` (built into `[owneet]`), PipeWire, NetworkManager, BlueZ, udev rules for controllers.
- **Done when:** ISO size is measured and stays **under 2 GB**; idle RAM is measured and recorded.
  First real-hardware boot of the live ISO on the owner's NiPoGi E3B mini PC (AMD/Intel path), and
  later on the owner's desktop (RTX 4060, NVIDIA path), confirming gamescope (step 1.3).
- **Real hardware, 2026-10-06:** the live ISO boots on the owner's NiPoGi E3B mini PC (AMD Ryzen
  7430U, 12 threads, integrated Radeon graphics, 24 GB RAM): boot splash shown, then the placeholder with **"console session: gamescope"**, so the
  Vulkan check and gamescope work on real hardware. Still to test: the NVIDIA path (owner's desktop).
  Note: gamescope does not allow switching to other virtual terminals (Ctrl+Alt+F9), so the debug
  shell is reachable only in the cage session: diagnostics use
  `systemd.debug_shell owneet.session=cage`. To do: a "diagnostics" boot menu entry with these
  arguments, so testers do not have to type them. **Done:** boot entry "OwneetOS (diagnostics)"
  with an automatic report on tty9 (`owneet-diagnostics`), verified in the VM.
  Owner's desktop (RTX 4060 + Intel UHD 770): normal boot shows "console session: gamescope";
  diagnostics report: `blacklist nouveau` (Turing+ detected), `nvidia` + `nvidia_drm` loaded (plus
  `i915`/`xe` for the Intel iGPU), Vulkan lists the RTX 4060 as discrete GPU. Memory: 2824 MiB used,
  of which 1540 MiB were the live image copied to RAM by archiso (`copytoram` defaults to on with
  plenty of RAM): this also explained the slow boot from USB. Fixed with `copytoram=n`. The RAM target
  (500 MB) is measured on the installed system with the real UI. Observed: the live boot from USB is slow (USB stick speed,
  LZMA-compressed root, first NVIDIA initialisation; an installed system will not have the first two),
  and the boot menu editor uses the US keyboard layout (firmware limitation).
- **Progress:** `owneet-hardware` meta-package (firmware, microcode, Mesa + Vulkan for AMD/Intel,
  `nvidia-open` + `nvidia-utils`, PipeWire, NetworkManager, BlueZ, controller rules) and
  `owneet-gpu-select` (nvidia-open for Turing+, nouveau for older NVIDIA). Measured in the VM:
  ISO **1664 MiB** (under 2 GB, but NVIDIA's user space alone is ~950 MiB installed: the budget left
  for Brave and the UI is tight; options if needed: trim NVIDIA files, or a separate NVIDIA ISO),
  boot to session ~13 s, system RAM without the placeholder UI ~235 MiB, no failed units,
  NetworkManager and Bluetooth enabled. New test tool: `vm/test.sh --debug-shell` + `run`.
  **xone is not included:** the dongle firmware is Microsoft's and cannot be redistributed; it needs
  a separate decision (download on the user's PC at first use).

### [x] 1.6 ISO build in CI

- **Deliverables:** GitHub Actions job building the ISO (privileged container), checksums, build artefact.
- **Done when:** a CI-built ISO boots in the test VM.
- **Progress:** job `ISO` builds packages and ISO in a privileged, pinned Arch Linux container
  (`tools/ci/build-iso`, `OWNEET_NO_CHROOT=1`), only when ISO inputs changed, on demand or weekly;
  artifact kept 7 days (unsigned development build). Simulated with Podman in the builder VM with
  the same image: ISO built (1664 MiB) in 2 min 50 s with a warm package cache.
  **First run on GitHub (2026-10-06): green.** Job `ISO` took about 10 min (2.5 min freeing disk
  space, 7.5 min packages + ISO); artifact `owneetos-dev-iso-14`, 1664 MiB. Downloaded, checksum
  verified, booted in the test VM: console session starts; diagnostics report clean (boot 8.5 s,
  no failed units). **Since 2026-10-07 the ISO is no longer uploaded** (GPL source obligations,
  PROJECT_RULES.md section 12): CI builds it as a check only.

### [~] 1.7 Xbox wireless dongle (xone) (software done; real dongle test open)

- **Deliverables:** xone kernel module built for the ISO's kernel (rebuilt with every kernel
  change), firmware download helper (Microsoft's firmware, fetched on the user's PC with consent at
  first dongle use), first-boot notice that setup needs a Bluetooth or cable controller.
- **Done when:** a dongle-connected Xbox controller works after the firmware download, on real
  hardware.
- **Progress:** `owneet-xone` (xone 0.5.8, compiles cleanly against kernel 7.2.8; 8 modules, without
  `xone_wired`; `softdep mt76x2u pre: xone_dongle` instead of upstream's blacklist of `xpad` and
  `mt76x2u`), `owneet-xone-firmware` (status / install with explicit consent / udev flag
  `/run/owneet/xone-firmware-needed`). Verified in the test VM: driver loads, `xpad` untouched,
  install refused without consent, with consent the 4 firmware files are downloaded from
  Microsoft, checksum-verified and installed. CI build now pins its container to the ISO's Arch
  snapshot (kernel headers must match the ISO kernel). **Open:** test with a real dongle (the owner
  has none for now). The consent dialog and the first-boot notice are UI work: step 6.1.

---

## Phase 2 — System daemon (`owneetd`)

### [x] 2.1 Language decision and API design — **Owner input**

- **Goal:** choose **Go or Rust** and design the local API before writing the daemon.
- **Deliverables:** short comparison (RAM, binary size, D-Bus/evdev libraries, contributor
  friendliness), API specification (endpoints + event stream), security model (local only,
  per-session token), process split (system service vs user service).
- **Done when:** owner approves the language and the API document.

### [x] 2.2 Daemon skeleton and packaging

- **Deliverables:** project layout, config file, logging, systemd units, PKGBUILD, CI build + tests.
- **Outcome:** Go module in `daemon/` (standard library only): `owneetd` serves `GET /v1/status`
  and the `GET /v1/events` stream on `$XDG_RUNTIME_DIR/owneetd.sock` (0600); `owneetctl` client;
  optional `/etc/owneet/owneetd.json`; logs to the journal; 9 unit tests. Package `owneetd`
  (static binary, user service enabled by preset); `owneet-session` writes the session mode for
  `/v1/status`. `tools/lint` and CI run gofmt, go vet, go test. Verified in the test VM: service
  active and enabled for `owneet`, status answers (version, OwneetOS version, session mode), JSON
  error on unknown endpoints, another user is refused by the socket, ~10.6 MB RAM. Test tooling:
  the VM debug shell moved to a virtio console (reliable, unlike the emulated serial port).

### [x] 2.3 Controller input

- **Deliverables:** evdev discovery and hot-plug, SDL_GameControllerDB mapping, Guide button
  detection, controller battery level, event stream to clients.
- **Done when:** a virtual gamepad (uinput) in the test VM produces the expected events in automated tests.
- **Progress:** `internal/gamepad` (scan + inotify hot-plug, no grab), Guide from `BTN_MODE` or
  SDL_GameControllerDB (pinned, shipped in `owneetd`), `KEY_HOMEPAGE` companion devices, battery
  from sysfs power_supply; `GET /v1/controllers`, events `controller.added/removed/battery` and
  `guide.pressed`. Integration tests create real virtual controllers through `/dev/uinput` (Xbox-like
  pad: hot-plug, Guide, removal; generic pad: Guide found through the database): pass in the builder
  VM, run by `tools/lint` and CI. **Real controller (2026-10-07):** the owner's Xbox Wireless
  Controller (Bluetooth, passed to the test VM) is listed as brand `xbox`, connection `bluetooth`,
  Guide available; 3 presses of the Xbox button gave exactly 3 `guide.pressed` events, other
  buttons none. (In the VM the id is the virtio path: the Bluetooth address is not passed through.)

### [x] 2.4 Virtual input (uinput)

- **Deliverables:** virtual keyboard and mouse devices for the system-wide on-screen keyboard and
  for apps that need key presses.
- **Outcome:** `internal/vinput`: virtual keyboard and mouse created at startup; text typed with
  layout tables for US and Italian (accented letters, AltGr symbols; unsupported characters
  rejected before anything is typed); special keys with modifiers; pointer move, scroll, click.
  API: `/v1/input/text`, `/key`, `/pointer`, `/click`, `/layout`. `/dev/uinput` granted to the
  console session by a udev `uaccess` rule (owneetd stays unprivileged), `uinput` loaded at boot.
  Integration test reads back the key events of the real virtual keyboard (Italian "a@" + Enter).

### [x] 2.5 Bluetooth

- **Deliverables:** BlueZ over D-Bus: list, pair, trust, forget; **automatic gamepad pairing
  mode** (accept any device that identifies as a gamepad, no input needed).
- **Done when:** a real controller pairs with zero input on real hardware (VM Bluetooth is unreliable).
- **Progress:** `internal/bluetooth`: BlueZ over D-Bus (`godbus/dbus` v5.2.2, BSD-2-Clause, vendored
  with `golang.org/x/sys`; licences shipped in `/usr/share/licenses/owneetd/`). Auto-pair turns on
  by itself whenever no controller is connected (any connection) and off as soon as one is: adapter
  powered, scan, every device that identifies as a gamepad (class of device, LE appearance or BlueZ
  icon) is paired, trusted and connected. Pairing agent `NoInputNoOutput`: accepts only gamepads
  while auto-pair is on, re-registers when bluetoothd (re)starts; BlueZ is never D-Bus-activated
  (no journal noise without an adapter). API `GET /v1/bluetooth`, `POST /v1/bluetooth/auto-pair`,
  connect / disconnect / forget; events `bluetooth.*`. Unit tests with a fake BlueZ (policy, agent,
  no adapter) and API tests; race detector clean. **Test VM (no adapter):** `adapter: false`, clean
  log, no failed units; owneetd ~18 MB RSS (9.7 MB anonymous + 8.2 MB binary pages). The
  diagnostics report (tty9) shows owneetd's controllers and Bluetooth state and its log, and
  refreshes with Enter.
- **Outcome on real hardware (2026-10-07, NiPoGi E3B, owner's Xbox Wireless Controller over
  Bluetooth LE):** live ISO, no controller connected → auto-pair on at boot; controller put in
  pairing mode → paired, trusted and connected in 5 s with no input (`pairing gamepad` 18:04:05,
  `controller connected … guide=true` 18:04:10), auto-pair off; controller switched off and on →
  reconnected by itself in 4 s. Fixed after the test: the report used `bluetoothctl`, which fails
  without a terminal (showed "no adapter"; now it asks owneetd); Bluetooth LE controllers were named
  `bluez-hog-device` (now BlueZ's device name is used); `poweroff` from the diagnostics shell waited
  90 s for the shell to exit (now 3 s; normal shutdown was not affected). Approved by the owner;
  the re-test of these fixes on real hardware is postponed.

### [x] 2.6 Network

- **Deliverables:** NetworkManager over D-Bus: scan, connect (with password from the OSK), forget, status.
- **Outcome:** `internal/network`: status (state, connectivity, Wi-Fi, Ethernet), visible networks
  grouped by name with security (open, WPA, WPA3; WEP and enterprise reported as unsupported),
  scan, connect with password check before NetworkManager and a precise "wrong password" error,
  disconnect, forget, Wi-Fi on/off; events `network.*`, `wifi.scan_done`. A new password replaces a
  saved network only if it works. Networks are saved system-wide thanks to a narrow polkit rule
  (`settings.modify.system` for `owneet` in the active local session only; `owneet` is not an
  administrator). The diagnostics report shows the network status.
- **Test VM with simulated Wi-Fi** (`mac80211_hwsim` + NetworkManager hotspot, `vm/README.md`):
  scan finds the network; short password → `invalid_password`; wrong password → `wrong_password`
  in 1 s, nothing left saved; right password → connected in 3 s, saved in a root-only file;
  disconnect and reconnect without password; forget; Wi-Fi off → `wifi_disabled`; WPA3-only
  network: wrong and right password both handled. Passwords never appear in the journal. owneetd
  ~17.7 MB RSS (unchanged). Unit tests: security classification, password rules, network list.
  Real-hardware Wi-Fi is tested with the settings UI (step 3.9), or earlier on request. Approved
  by the owner (system-wide saved networks with the polkit rule).

### [x] 2.7 Audio

- **Deliverables:** PipeWire: volume, mute, output selection (speakers, HDMI, headset).
- **Outcome:** `internal/audio`: PipeWire through its PulseAudio protocol (`jfreymuth/pulse`, MIT,
  vendored, as planned in the daemon design). Outputs with name and kind (speakers, headphones,
  HDMI, Bluetooth, USB), default output, volume (mixer scale, 0–100) and mute; changing the output
  moves sounds already playing; changes made by any program produce `audio.changed`; reconnects
  when PipeWire restarts. The diagnostics report shows the audio status.
- **Test VM** (`vm/test.sh boot --audio`: built-in + USB virtual sound cards): both outputs listed
  (`usb`, `speakers`); volume 25 and mute applied (checked with `pactl`); output switched while a
  sound played, and the sound moved; volume changed by another program → event; PipeWire restarted
  → reconnected in 26 ms, settings kept. owneetd ~18 MB RSS; PipeWire starts at login (pipewire,
  pipewire-pulse, WirePlumber ~40 MB, needed anyway by the UI's sounds). HDMI and Bluetooth outputs
  are checked on real hardware with the settings UI (step 3.9).

### [x] 2.8 Power

- **Deliverables:** shut down, restart, suspend.
- **Outcome:** `internal/power`: systemd-logind over D-Bus (no new dependency). `GET /v1/power`
  (what is possible), `POST /v1/power/shutdown`, `/restart`, `/suspend`; events
  `power.suspending`, `power.resumed`, `power.shutting_down`. logind already allows these to the
  active local session: no polkit rule; owneetd never asks for a password. Hibernation not offered.
- **Test VM:** all three possible for `owneet`; suspend → QEMU reports the machine suspended, woken
  through QMP → `power.suspending` and `power.resumed` events, network back; restart → the system
  boots again; shutdown → VM off in 2 s. The PC's power button keeps logind's default for now
  (decided with the UI). Suspend/resume on real hardware is checked with the UI (step 3.9).

### [x] 2.9 Process manager and Guide button

- **Deliverables:** launch / track / close games and apps, return to home on Guide, force-close a
  frozen game, focus handling inside gamescope.
- **Done when:** a test app is launched, Guide brings back the home screen, and the app can be closed.
- **Decisions (owner, 2026-10-07):** Guide toggles home ↔ game in gamescope, the game keeps
  running; in cage, holding Guide 2 s closes the game, a short press does nothing.
- **Outcome:** `internal/apps`: each app in its own transient systemd user service (closing stops
  all its processes; SIGKILL after 10 s; force-close at once; found again after an owneetd
  restart); one game at a time. gamescope now runs with `--steam`: owneetd tags windows with app
  ids and chooses what is on screen (`jezek/xgb`, BSD-3-Clause, vendored). `owneet-session-app`
  publishes the session's display and the home screen's process. API `/v1/apps` (launch, list
  with focus, focus, close, kill); events `app.started`, `app.exited`, `focus.changed`,
  `guide.released`. Found while testing: gamescope ignores focus requests without `--steam`, and
  in that mode shows only tagged windows; gamescope cannot use the VM's screen with software
  Vulkan (test hook `owneet.debug.gamescope_backend=headless` added, VM README corrected).
- **Test VM** (virtual gamepad through `/dev/uinput`, `mpv` test pattern as the app): gamescope
  (headless): home on screen at boot; launched game comes to the front; Guide → home, Guide → game;
  owneetd restarted with the game open → still on screen, Guide still toggles; second game refused;
  missing program reported; close (SIGTERM), an app ignoring SIGTERM killed after 11 s, force-close
  at once. cage: short press does nothing, holding 2.5 s closes the game. Open: the same on a real
  screen (owner).
- **Real hardware (2026-10-08, owner's desktop, RTX 4060):** the placeholder shows "console session:
  gamescope", but flickering black rectangles appear (already before this step; not on the AMD
  NiPoGi). Test ISO with one variant per boot entry: drawing with the CPU or without `--steam`
  → rectangles; drawing through Vulkan or gamescope `--force-composition` → none. Cause: direct
  scanout of the app's buffer with the NVIDIA driver. Fix: `owneet-session` forces composition
  when the NVIDIA driver is loaded; AMD and Intel keep direct scanout. Test hooks
  `owneet.debug.placeholder_vo`, `owneet.debug.gamescope_composite`, `owneet.debug.gamescope_steam`.
  **Verified by the owner on the desktop:** no rectangles, gamescope session. (The second monitor
  still shows a frozen splash frame: known issue, step 3.9.)

---

## Phase 3 — Frontend (`owneet-frontend`, a Pegasus Frontend fork)

Legal requirements for the whole phase (checked 2026-10-08, PROJECT_RULES.md section 12):
Pegasus is GPL-3.0-or-later with additional terms: a significantly modified version must not use
"Pegasus Frontend", "Pegasus Launcher", "Pegasus" or Pegasus's logos as its title or logo (the fork
is `owneet-frontend`, the product is OwneetOS; copyright notices are kept and Pegasus is credited).
Pegasus's default theme (`pegasus-theme-grid`) is CC BY-NC-SA 4.0 and is never shipped. Pegasus's
Roboto fonts and button images are not used (own fonts and glyphs). Pegasus's online metadata
downloads (Steam store, GOG API, Play Store) are disabled: local data only.

Every screen step (home, library, settings, on-screen keyboard, notifications, and the pairing and
setup screens of phase 6) starts with an **interactive demo** approved by the owner before its
code is written (PROJECT_RULES.md section 9).

### [x] 3.1 Pegasus study and fork

- **Deliverables:** clone `mmatyas/pegasus-frontend` (master, with submodules), fork on GitHub as
  `owneet-frontend`, `docs/frontend-architecture.md` describing build system, data providers, theme
  API and the extension points we need, and what must be removed or disabled for the legal
  requirements above.
- **Done when:** the owner has a clear picture of what we change in C++ and what stays in QML.
- **Outcome:** owner's choice: the fork lives **inside this repository** (`frontend/owneet-frontend`,
  no separate GitHub repository). Pegasus `5d58223` imported unmodified (without the CC BY-NC-SA
  grid theme, the unlicensed translations and upstream CI files), then a minimal own theme as the
  built-in default. Builds against the ISO's snapshot with Qt 5.15 (Qt 6 not possible yet: the
  frontend still needs Qt 5-only parts); 20/23 upstream tests pass headless, the 3 QML scene tests
  need a GPU context (same upstream). About +30 MiB in the ISO. `docs/frontend-architecture.md`:
  build, structure, theme API, providers (kept / compiled out), launching (Pegasus unloads its UI
  during games and runs them as children: replaced by owneetd launching, UI kept loaded), the
  owneetd client in C++ (QML cannot reach a Unix socket), and the list of C++ changes.

### [x] 3.2 Build and package the frontend

- **Deliverables:** reproducible build in the builder VM against Qt 5.15, PKGBUILD in `[owneet]`,
  frontend replaces the placeholder in the session.
- **Done when:** the unmodified frontend, with a minimal theme of our own (never the CC BY-NC-SA
  grid theme), runs in gamescope in the test VM and is driven by a gamepad.
- **Also:** seamless handover from the boot splash to the UI, without the dark gap left in 1.4.
- **Outcome:** package `owneet-frontend` (built in the clean chroot; 20 upstream tests pass with
  the offscreen platform and Qt Quick's software renderer, flaky `test_Playtime` left out; LTO off:
  Qt's two-pass resource compiler fails with LTO objects); the session starts it instead of the
  placeholder (kept as fallback). Fork changes: OwneetOS names (executable, window title, config
  folder, log), no Pegasus logo/icons/splash images, loading screen identical to the boot splash,
  Pegasus's menu hidden, only local game data (8 providers not built, no Steam store download),
  Guide not passed to the interface, window tags itself for gamescope and shows the home screen if
  nobody chose (works with owneetd stopped), **interface kept loaded while a game runs** (Pegasus
  unloaded it and blocked until the game ended: black screen in cage), no endless button repeat
  when input pauses, SDL no longer swallows SIGTERM (the interface could not be stopped), mouse
  pointer hidden until the mouse is used, window born at screen size. ISO 1720 MiB.
- **Test VM:** cage: list of test games, D-pad moves the selection, A starts the game once, the
  game shows, closing it returns to the list and input works again. gamescope (headless):
  interface on screen and tagged, also with owneetd stopped. RAM in the VM (software rendering):
  owneet-frontend ~170 MB, whole live system 566 MiB — to be measured on real hardware (C1).
- **Real hardware (2026-10-08, owner's desktop, RTX 4060):** the interface shows after the boot
  splash; the dark gap between them is about a tenth of a second (the ~4 s in the VM come from
  software rendering): no helper needed, handover done. Live system RAM "used": 1142 MiB of 48 GB
  — breakdown per process requested (constraint C1: 500 MB).
- **Desktop diagnostics (cage):** "Timeout waiting session to become active": while the report on
  tty9 is on screen the console session is not active and cage cannot start (reproduced in the VM;
  back on tty1 it starts). This also explains the "endless splash" of 1.5. `owneet-session` now
  waits until the console is on screen instead of counting failed starts (no false fallback, no
  error screen). RAM on the desktop: programs ~60 MB, kernel ~340 MB, ~800 MB not attributed
  (typical of GPU drivers: NVIDIA with GSP firmware plus the Intel GPU); the NiPoGi (AMD) used
  ~235 MiB without the UI in 1.5. C1 is measured on the target low-budget hardware.
  **Verified by the owner (desktop, diagnostics entry):** report opened at once → "the console is
  not on screen: waiting", no failed start; back on tty1 → cage starts, interface running. RAM in
  cage there: owneet-frontend 230, cage 176, Xwayland 160 MiB RSS (including the shared NVIDIA
  libraries; anonymous memory +250 MB in total), 1427 MiB used. To measure in gamescope on low-cost
  hardware and optimise if needed (step 7.4).
- **Open:** (1) In cage, the mouse pointer shows until the first input (drawn by cage).
  (2) Games are still started by the frontend itself; through owneetd in 3.8.

### [x] 3.3 Theme foundations

- **Deliverables:** design tokens (palettes from section 9), bundled fonts, 1280×720 scaling grid,
  safe area, focus ring, prompt bar with Xbox / PlayStation glyph sets.
- **Demo:** `design/demos/3.3-theme-foundations.html`, approved on 2026-10-08 after one review
  (the "Foundations" tab exists only in the demo; 21 palettes in a picker with live preview, Tide
  made vivid "minty" cyan, Ember, Espresso, Harbor and others added; language chosen from a list;
  Guide prompt "Home"; text sizes S/M/L/XL = 90/100/120/140 %, text only, columns scroll while the
  prompt bar and safe area never move; glyphs drawn for OwneetOS, no third-party logos, chosen
  automatically from the connected controller).
- **Outcome:** theme `owneet` (built-in default, replaces `owneet-minimal`) with its foundations
  in `src/themes/owneet/foundation/` (docs/frontend-architecture.md section 4): `Theme` tokens and
  the 21 palettes, focus ring with lift, glyphs and prompt bar, picker window, settings rows.
  Bricolage Grotesque and Lexend bundled as static instances (`tools/branding/make-fonts`, OFL-1.1
  licence texts installed); Pegasus's Roboto fonts and button images removed. Until the home
  screen (3.6) the theme shows a plain game list; **Y opens a temporary Appearance window**
  (palette, text size, button prompts, reduce motion, TV safe area outline), saved in the theme's
  memory, which moves to Settings in 3.9. Language choice comes with 3.4. Checked in the builder VM
  (frontend under Xvfb, screenshots of every option) and in the test VM (choice kept when the
  frontend restarts). Owner test (2026-10-08): everything shown correctly, picker and options work;
  the choice is lost on reboot because the live USB has no persistent storage (checked again on the
  installed system, phase 6). Approved by the owner on 2026-10-08, with one addition: a
  **Nintendo-style** glyph set (L/R, ZL/ZR, "+"; letters as printed, since sdl2-compat follows the
  printed labels on Nintendo pads), chosen automatically for Switch controllers. Its hardware test
  (Switch Pro Controller) is deferred to the next hardware test, together with other checks.

### [x] 3.4 Internationalisation

- **Deliverables:** JSON message files, loader, **English as the default language** and fallback,
  `en` + `it` complete,
  `docs/translating.md` for contributors.
- **Done when:** a new language works by adding one JSON file, with no code change.
- **Outcome:** `owneet::I18n` in the frontend (C++, unit test `test_I18n`): one JSON file per
  language (`frontend/i18n/`, installed in `/usr/share/owneet-frontend/i18n/`; a file in
  `~/.config/owneet-frontend/i18n/` overrides it), `{placeholders}`, plural forms with six rule
  families (one-other, french, none, east-slavic, polish, czech), numbers in the language's style.
  English on first boot and for missing messages; the choice is saved in the frontend's settings.
  The theme uses the `Tr` singleton; all its text is in `en.json` and `it.json` (46 messages,
  palette names included). Language picker as in demo 3.3 (Appearance → Language, A applies at
  once). `tools/i18n-check`, part of `tools/lint`, checks the files. Checked in the builder VM
  (switch to Italian, kept after a restart). Limits: no right-to-left; non-Latin scripts need
  extra fonts, added with the first such language.

### [~] 3.5 Navigation and input map (awaiting owner review)

- **Deliverables:** spatial navigation and the global input map (rules section 9.1) implemented once and shared by every screen.
- **Outcome:** in C++, the gamepad and keyboard map of section 9.1 (LB/RB = sections, LT/RT =
  filters, Y = page option, X = secondary, Menu = options; keyboard fallback keys), the right
  stick as fast scrolling (new `scroll-up/down` keys; it no longer moves the selection like the
  left stick), repeat timing of demo 3.3 (360 ms, then 140 ms; scrolling 90 ms), and a fixed map
  (key bindings no longer saved or read from the settings file). In QML (`foundation/`): `Nav`
  turns key events into actions (held buttons act once; directions and scrolling repeat), finds
  the nearest item in a direction (spatial navigation, as in the demo) and handles lists;
  `NavArea` gives any screen or window spatial navigation, A and B; `Nav.feedback(kind)` reports
  move, edge, confirm, back, section, tab, toggles and windows for the sounds of 3.12. Windows
  (`Sheet`) and the pickers now use them. Buttons with no use on a screen do nothing (LB/RB/LT/RT
  report "edge"). New `tools/frontend-preview` runs the frontend in the builder VM with test games
  and a virtual controller, following a step list (`tools/preview-steps/`), and saves
  screenshots: checked there with keyboard, D-pad, right stick and buttons.

### [ ] 3.6 Home screen

- **Deliverables:** home as in the approved mockup (continue playing, apps, jump back in, notices).

### [ ] 3.7 Library screen

- **Deliverables:** unified grid, filters on LT/RT, sort on Y, disks with free space.

### [ ] 3.8 Frontend ↔ daemon bridge

- **Deliverables:** QML client for the `owneetd` API and event stream (XMLHttpRequest or a small
  C++ plugin in the fork, decided in 3.1).

### [ ] 3.9 Settings *(multi-prompt)*

- **Deliverables:** Appearance (palette, text size, reduce motion, persisted), Network, Controllers
  and Bluetooth, Audio, Display (resolution, refresh rate via gamescope; with several screens,
  which one the console uses), Language, Storage (read-only), System (version, restart, shut down).
- **Known issue to fix here (2026-10-08, owner's desktop with two monitors on two GPUs):**
  gamescope uses one screen; a screen on the other GPU keeps showing the frozen boot splash.
  Other screens are **not** turned off by default (owner's decision): they must show something
  sensible instead of the frozen splash. Using a second screen for something (e.g. chat, guides)
  is a possible later feature.

### [ ] 3.10 On-screen keyboard

- **Deliverables:** gamepad OSK for frontend text fields (Wi-Fi passwords, search), layouts per language.

### [ ] 3.11 Notifications

- **Deliverables:** notice area on home + transient toasts fed by daemon events.

### [ ] 3.12 Interface sounds

- **Deliverables:** sounds for navigation and feedback, as listed in `design/sounds.md`, played
  from **placeholders** (`frontend/sounds/`, Kenney CC0, generated by
  `tools/sounds/make-placeholders`) until the owner's final sounds arrive (7.7); a sound service in the theme
  (Qt `SoundEffect`, preloaded, no delay), played by the shared navigation components (3.5) so every
  screen gets them; Settings → Sound: "Interface sounds" on/off and volume; silent while a game is
  on screen. Priority-2 sounds are wired when their features arrive (notifications, controllers,
  game launch, on-screen keyboard).

---

## Phase 4 — Games

### [?] 4.1 Steam library *(investigation first)*

- **Deliverables:** installed Steam games read from local files (`libraryfolders.vdf`,
  `appmanifest_*.acf`) across all disks; play time from local Steam data; check whether Pegasus's
  built-in Steam provider can be reused (without its Steam store downloads: local data only).
- **Open point:** listing games that are **owned but not installed** may not be possible from local
  files alone. If it isn't, report options to the owner (e.g. show only installed games plus a
  "Get more games" store tile) and update the rules.

### [ ] 4.2 Steam integration

- **Deliverables:** Steam login and store in `-gamepadui` mode, game launch via the daemon, return
  to home on exit, Steam Store tile.

### [ ] 4.3 Local DRM-free games

- **Deliverables:** folder convention + metadata file, automatic scan of mounted disks, artwork.

### [ ] 4.4 Artwork

- **Deliverables:** cover download and cache for Steam games, generated fallback covers.

---

## Phase 5 — Streaming apps and media

### [ ] 5.1 Brave in the OS

- **Deliverables:** `brave-bin` built into `[owneet]`, managed policies (forced extension, no
  first-run, Widevine on, locked settings), dedicated profile, kiosk launch through the daemon.
- **Done when:** a test page opens fullscreen and Guide returns to home.

### [ ] 5.2 Extension core

- **Before shipping:** check the extension's behaviour and any user-agent change against the terms
  of service of each streaming service (PROJECT_RULES.md section 12).

- **Deliverables:** Manifest V3 extension: Gamepad API loop, spatial navigation, B = back, exit to home, OSK for web forms.

### [ ] 5.3 YouTube

- **Deliverables:** TV interface, playback controls on the gamepad.

### [ ] 5.4 Netflix

- **Deliverables:** Widevine verified, browse / play / pause / seek on the gamepad.

### [ ] 5.5 Spotify

- **Deliverables:** web player navigation, playback continues in the background.

### [ ] 5.6 Local videos and music

- **Deliverables:** mpv with gamepad bindings, Videos and Music browsing in the frontend from mounted disks.

---

## Phase 6 — First boot, installer, disks

### [ ] 6.1 First-boot flow

- **Deliverables:** controller (auto-pairing, as in the mockup) → language → network → ready.
  The keyboard layout follows the chosen language (console and on-screen keyboard), and can be
  changed in Settings.
  Includes the notice that the first setup needs a Bluetooth or cable controller, and the consent
  dialog for the Xbox dongle firmware (`owneet-xone-firmware install --accept-microsoft-terms`
  when `/run/owneet/xone-firmware-needed` exists).

### [ ] 6.2 Live vs installed mode

- **Deliverables:** detection, "Install OwneetOS" entry shown only in live mode.

### [?] 6.3 Installer backend *(multi-prompt)*

- **Deliverables:** whole-disk install: partitioning, btrfs subvolumes, snapper, bootloader, user,
  first-boot setup. Tested on virtual disks only, never on the host.
- **Open point:** **systemd-boot cannot boot btrfs snapshots by itself.** Rollback from the boot
  menu needs either GRUB + grub-btrfs or Limine + snapshot sync, or a different rollback design.
  Decision to be taken with the owner at the start of this step.

### [ ] 6.4 Installer UI

- **Deliverables:** QML installer in the OwneetOS style, gamepad-only, clear warnings and a final confirmation before erasing anything.

### [ ] 6.5 Automatic disk mounting

- **Deliverables:** udisks2 policy + daemon logic for internal and removable disks (ext4, btrfs,
  NTFS, exFAT), read-only NTFS when Windows left it hibernated, with an explanation to the user.

---

## Phase 7 — Updates and first release (Wave 1 complete)

### [ ] 7.1 Updates

- **Deliverables:** update check and install from Settings, snapshot before every update,
  rollback path (as decided in 6.3), archive snapshot date advanced only after tests.

### [ ] 7.2 Release pipeline

- **Deliverables:** tagged release builds the ISO in CI, checks the 2 GB limit, signs, writes
  checksums and release notes, publishes to GitHub Releases.
- **Legal (PROJECT_RULES.md section 12):** a **source archive** for every release (PKGBUILDs and
  exact sources of every package in the ISO and the repository), licence list in the ISO; only
  then may CI publish ISOs and packages again.

### [ ] 7.3 User guide

- **Deliverables:** illustrated guide: download, write the USB stick (balenaEtcher / Ventoy),
  disable Secure Boot (per-brand pointers), anti-cheat warning for Windows games, first boot.

### [ ] 7.4 Hardware testing and v0.1.0

- **Deliverables:** test matrix (Intel / AMD / NVIDIA, old iGPU fallback, 4 GB RAM machine), bug
  fixing, RAM and boot-time measurements against the targets, release **v0.1.0**.

### [ ] 7.5 Legal review before the public launch

- **Deliverables:** trademark search for "OwneetOS" (EUIPO, Italian register); licence notices in
  the UI ("About → Licences"); review of licences, third-party terms and privacy by someone
  experienced in open-source licensing (PROJECT_RULES.md section 12).

### [ ] 7.6 Final logo and brand

- **Deliverables:** the final OwneetOS logo and wordmark (the current wordmark is provisional),
  checked together with the trademark search of 7.5; boot splash, interface and documents updated.

### [ ] 7.7 Final interface sounds *(Owner delivery)*

- **Deliverables:** the owner's final sounds (`design/sounds.md`: names, format, lengths, licences)
  replace the placeholders of `frontend/sounds/`; `CREDITS.md` updated with their authors and
  licences. After the legal review (7.5), before the public launch.

---

## Wave 2 — to be detailed into steps when Wave 1 is done

- **8. Overlay and quick menu:** gamescope overlay layer, quick menu over games, volume, battery, screenshots on View/Create.
- **9. Chat modules:** module interface, Discord web in the background, notifications in the overlay, mute and push-to-talk.
- **10. Dual boot (optional, not required):** "Install alongside Windows", BitLocker detection and guidance, RTC handling, default OS, "Restart into Windows".
- **11. More stores and Windows games:** Epic (legendary), GOG (gogdl), umu-launcher + GE-Proton.
- **12. Console features:** user profiles, parental controls, play-time statistics, quick suspend and resume.
- **13. Custom accent colour** with automatic contrast checking.

## Backlog — nice-to-have, order to be decided

- Screen reader / TTS for menus (espeak-ng or Piper, offline).
- Crash reports with explicit opt-in consent.
- Emulation (RetroArch).
- Qt 6 port of the frontend.
- Community translations.
- Gameplay recording.
