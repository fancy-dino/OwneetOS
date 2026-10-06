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

### [~] 1.1 archiso profile (awaiting owner review)

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

### [x] 1.3 Console session (gamescope on real hardware: checked at the end of 1.5)

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

### [~] 1.5 Hardware support set (waiting for the real-hardware test)

- **Deliverables:** firmware, Mesa + Vulkan drivers, `nvidia-open` with automatic detection,
  `xone` (built into `[owneet]`), PipeWire, NetworkManager, BlueZ, udev rules for controllers.
- **Done when:** ISO size is measured and stays **under 2 GB**; idle RAM is measured and recorded.
  First real-hardware boot of the live ISO on the owner's NiPoGi E3B mini PC (AMD/Intel path), and
  later on the owner's desktop (RTX 4060, NVIDIA path), confirming gamescope (step 1.3).
- **Progress:** `owneet-hardware` meta-package (firmware, microcode, Mesa + Vulkan for AMD/Intel,
  `nvidia-open` + `nvidia-utils`, PipeWire, NetworkManager, BlueZ, controller rules) and
  `owneet-gpu-select` (nvidia-open for Turing+, nouveau for older NVIDIA). Measured in the VM:
  ISO **1664 MiB** (under 2 GB, but NVIDIA's user space alone is ~950 MiB installed: the budget left
  for Brave and the UI is tight; options if needed: trim NVIDIA files, or a separate NVIDIA ISO),
  boot to session ~13 s, system RAM without the placeholder UI ~235 MiB, no failed units,
  NetworkManager and Bluetooth enabled. New test tool: `vm/test.sh --debug-shell` + `run`.
  **xone is not included:** the dongle firmware is Microsoft's and cannot be redistributed; it needs
  a separate decision (download on the user's PC at first use).

### [ ] 1.6 ISO build in CI

- **Deliverables:** GitHub Actions job building the ISO (privileged container), checksums, build artefact.
- **Done when:** a CI-built ISO boots in the test VM.

### [ ] 1.7 Xbox wireless dongle (xone)

- **Deliverables:** xone kernel module built for the ISO's kernel (rebuilt with every kernel
  change), firmware download helper (Microsoft's firmware, fetched on the user's PC with consent at
  first dongle use), first-boot notice that setup needs a Bluetooth or cable controller.
- **Done when:** a dongle-connected Xbox controller works after the firmware download, on real
  hardware.

---

## Phase 2 — System daemon (`owneetd`)

### [?] 2.1 Language decision and API design — **Owner input**

- **Goal:** choose **Go or Rust** and design the local API before writing the daemon.
- **Deliverables:** short comparison (RAM, binary size, D-Bus/evdev libraries, contributor
  friendliness), API specification (endpoints + event stream), security model (local only,
  per-session token), process split (system service vs user service).
- **Done when:** owner approves the language and the API document.

### [ ] 2.2 Daemon skeleton and packaging

- **Deliverables:** project layout, config file, logging, systemd units, PKGBUILD, CI build + tests.

### [ ] 2.3 Controller input

- **Deliverables:** evdev discovery and hot-plug, SDL_GameControllerDB mapping, Guide button
  detection, controller battery level, event stream to clients.
- **Done when:** a virtual gamepad (uinput) in the test VM produces the expected events in automated tests.

### [ ] 2.4 Virtual input (uinput)

- **Deliverables:** virtual keyboard and mouse devices for the system-wide on-screen keyboard and
  for apps that need key presses.

### [ ] 2.5 Bluetooth

- **Deliverables:** BlueZ over D-Bus: list, pair, trust, forget; **automatic gamepad pairing
  mode** (accept any device that identifies as a gamepad, no input needed).
- **Done when:** a real controller pairs with zero input on real hardware (VM Bluetooth is unreliable).

### [ ] 2.6 Network

- **Deliverables:** NetworkManager over D-Bus: scan, connect (with password from the OSK), forget, status.

### [ ] 2.7 Audio

- **Deliverables:** PipeWire: volume, mute, output selection (speakers, HDMI, headset).

### [ ] 2.8 Power

- **Deliverables:** shut down, restart, suspend.

### [ ] 2.9 Process manager and Guide button

- **Deliverables:** launch / track / close games and apps, return to home on Guide, force-close a
  frozen game, focus handling inside gamescope.
- **Done when:** a test app is launched, Guide brings back the home screen, and the app can be closed.

---

## Phase 3 — Frontend (Pegasus fork)

### [ ] 3.1 Pegasus study and fork

- **Deliverables:** clone `mmatyas/pegasus-frontend` (master, with submodules), fork on GitHub,
  `docs/frontend-architecture.md` describing build system, data providers, theme API and the
  extension points we need.
- **Done when:** the owner has a clear picture of what we change in C++ and what stays in QML.

### [ ] 3.2 Build and package Pegasus

- **Deliverables:** reproducible build in the builder VM against Qt 5.15, PKGBUILD in `[owneet]`,
  frontend replaces the placeholder in the session.
- **Done when:** stock Pegasus runs in gamescope in the test VM and is driven by a gamepad.
- **Also:** seamless handover from the boot splash to the UI, without the dark gap left in 1.4.

### [ ] 3.3 Theme foundations

- **Deliverables:** design tokens (palettes from section 9), bundled fonts, 1280×720 scaling grid,
  safe area, focus ring, prompt bar with Xbox / PlayStation glyph sets.

### [ ] 3.4 Internationalisation

- **Deliverables:** JSON message files, loader, fallback to English, `en` + `it` complete,
  `docs/translating.md` for contributors.
- **Done when:** a new language works by adding one JSON file, with no code change.

### [ ] 3.5 Navigation and input map

- **Deliverables:** spatial navigation and the global input map (rules section 9.1) implemented once and shared by every screen.

### [ ] 3.6 Home screen

- **Deliverables:** home as in the approved mockup (continue playing, apps, jump back in, notices).

### [ ] 3.7 Library screen

- **Deliverables:** unified grid, filters on LT/RT, sort on Y, disks with free space.

### [ ] 3.8 Frontend ↔ daemon bridge

- **Deliverables:** QML client for the `owneetd` API and event stream (XMLHttpRequest or a small
  C++ plugin in the fork, decided in 3.1).

### [ ] 3.9 Settings *(multi-prompt)*

- **Deliverables:** Appearance (palette, text size, reduce motion, persisted), Network, Controllers
  and Bluetooth, Audio, Display (resolution, refresh rate via gamescope), Language, Storage (read-only),
  System (version, restart, shut down).

### [ ] 3.10 On-screen keyboard

- **Deliverables:** gamepad OSK for frontend text fields (Wi-Fi passwords, search), layouts per language.

### [ ] 3.11 Notifications

- **Deliverables:** notice area on home + transient toasts fed by daemon events.

---

## Phase 4 — Games

### [?] 4.1 Steam library *(investigation first)*

- **Deliverables:** installed Steam games read from local files (`libraryfolders.vdf`,
  `appmanifest_*.acf`) across all disks; play time from local Steam data; check whether Pegasus's
  built-in Steam provider can be reused.
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

### [ ] 7.3 User guide

- **Deliverables:** illustrated guide: download, write the USB stick (balenaEtcher / Ventoy),
  disable Secure Boot (per-brand pointers), anti-cheat warning for Windows games, first boot.

### [ ] 7.4 Hardware testing and v0.1.0

- **Deliverables:** test matrix (Intel / AMD / NVIDIA, old iGPU fallback, 4 GB RAM machine), bug
  fixing, RAM and boot-time measurements against the targets, release **v0.1.0**.

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
