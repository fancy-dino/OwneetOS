# OwneetOS — Project Rules

> **Source of truth** for every requirement and decision agreed so far.
> Anyone working on this project (humans and AI assistants) must read this file before starting
> and must not contradict it. If something is not covered here, **ask** — do not assume.
> New decisions are added here as they are made (see [Decision log](#12-decision-log)).
> This file may be edited by the project owner at any time; the owner's edits always win.

---

## 1. Vision

OwneetOS is a **lightweight, open-source Linux distribution** that turns any x86_64 PC into a
**game console**, offering the same features as a modern Xbox / PlayStation, and more
(the long-term goal is a "console-killer", built one step at a time).

- The end user may have **no technical background**: the product must be extremely user-friendly.
- The whole system must be usable **with a gamepad only** — from the very first boot, through
  installation and setup, to daily use. Mouse and keyboard must **never be required**
  (they may still work as an optional fallback).
- It must run on **low-budget hardware** while keeping realistic expectations.

## 2. Hard constraints

| #   | Constraint                                                                                                                                            |
| --- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| C1  | **Lightweight** is a must: no bloatware, minimal RAM/CPU/disk footprint. Target: system + UI idle under **500 MB RAM** (with Steam and Brave closed). |
| C2  | **Gamepad-only** operation everywhere (installer, first setup, apps, settings).                                                                       |
| C3  | Architecture: **x86_64 only**. ARM is out of scope.                                                                                                   |
| C4  | Base distro: **minimal Arch Linux**. No desktop environment, no display manager.                                                                      |
| C5  | **Fully open source**, license **GPLv3** (also required by the Pegasus fork).                                                                         |
| C6  | **English is mandatory** for code, comments, documentation, commit messages, issues and PRs. The UI itself is multilingual.                           |
| C7  | The OS name **OwneetOS** is final.                                                                                                                    |
| C8  | Prefer modular, independent components — within feasibility and without creating bloat.                                                               |
| C9  | Must work **offline** for local / DRM-free content; internet is needed only for streaming and stores.                                                 |

## 3. Target hardware

- CPU: x86_64, roughly the last ~10 years.
- RAM: **4 GB minimum**, 8 GB recommended.
- Disk: **32 GB** minimum.
- GPU: **Vulkan** required for the main (gamescope) session. GPUs without adequate Vulkan support
  (e.g. Intel iGPUs older than ~2015) automatically fall back to a reduced **cage** session
  (no overlay).

## 4. Architecture

| Layer                           | Choice                                                                                                         | Notes                                                                                                                                                                                                                 |
| ------------------------------- | -------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Boot                            | **systemd-boot** + **Plymouth**                                                                                | Silent boot with logo. Auto-detects Windows.                                                                                                                                                                          |
| Login                           | Autologin on TTY (**greetd** or systemd getty autologin)                                                       | No display manager.                                                                                                                                                                                                   |
| Session / compositor            | **gamescope** (main), **cage** (fallback)                                                                      | Inspired by ChimeraOS `gamescope-session`. Overlay via gamescope overlay layer.                                                                                                                                       |
| Frontend / home                 | **Fork of Pegasus Frontend** (`mmatyas/pegasus-frontend`, `master`, cloned with submodules) + custom QML theme | Qt 5.15 now; **Qt 6 port** planned for a later phase.                                                                                                                                                                 |
| Gamepad mappings                | **SDL2** + **SDL_GameControllerDB**                                                                            |                                                                                                                                                                                                                       |
| System daemon                   | Own component, **Go or Rust**, single static binary                                                            | Reads controllers via evdev; Guide button (home / quick menu); system-wide on-screen keyboard via uinput; local HTTP API for Wi-Fi (NetworkManager), Bluetooth (BlueZ), audio (PipeWire), power; process launch/kill. |
| Streaming apps                  | **Brave**, installed system-wide (AUR `brave-bin`)                                                             | Controlled by the app via **Chromium managed policies** + launch flags (kiosk/app mode, dedicated profile). Widevine enabled (downloaded by Brave at runtime, never redistributed).                                   |
| Browser extension               | **Manifest V3**, Gamepad API, spatial navigation (e.g. WICG polyfill), per-site content scripts                | Prefer TV interfaces where they exist (e.g. `youtube.com/tv`). Note: Netflix on Linux is limited to 720p (Widevine L3).                                                                                               |
| Local media                     | **mpv**                                                                                                        | Kodi rejected (too heavy).                                                                                                                                                                                            |
| Steam                           | System package (multilib), launched with `-gamepadui` for login/store                                          |                                                                                                                                                                                                                       |
| Other stores (later)            | **legendary** (Epic), **gogdl** / lgogdownloader (GOG)                                                         | Heroic rejected (Electron, heavy).                                                                                                                                                                                    |
| Windows games (later)           | **umu-launcher** + GE-Proton                                                                                   |                                                                                                                                                                                                                       |
| Emulation (later, nice-to-have) | **RetroArch**                                                                                                  |                                                                                                                                                                                                                       |

## 5. Distribution, build and updates

- The OS is distributed as an **ISO on GitHub Releases** (public repository).
  - GitHub limits release assets to **2 GB per file**: the ISO must stay under it.
  - Steam's runtime is downloaded on first launch, not shipped in the ISO.
- ISO built with **archiso**, automated with **GitHub Actions**, with checksums and signatures.
- Packages built with **PKGBUILD**s; updates served from an **own pacman repository `[owneet]`**
  hosted on GitHub. Updates are triggered from the Settings menu with the gamepad.
- Arch packages are **pinned to Arch Linux Archive snapshots**; the snapshot date is advanced only
  after testing.
- **btrfs + snapper**: automatic snapshots, rollback selectable from the boot menu.
- The only "hands-on" step for the user: writing the ISO to a USB stick (balenaEtcher or Ventoy),
  explained in an illustrated guide.

## 6. Installation, boot and disks

- **Own installer** in QML (same look as the OS), fully gamepad-driven. Calamares rejected
  (mouse-oriented).
- Choices kept simple: "Use the whole disk" (default) / "Install alongside Windows" (later wave).
- **Dual boot must be considered, but it is NOT required.** OwneetOS must work fully as the only
  OS on the machine. Design choices (partitioning, bootloader, installer flow) must not prevent
  adding dual boot later, but no Wave 1 feature may depend on it. When implemented:
  - BitLocker partitions cannot be resized from Linux: detect them and guide the user to free
    space from Windows. Never touch them automatically.
  - Handle RTC (UTC vs local time) automatically.
  - Settings: default OS selection and a **"Restart into Windows"** button.
- **Secure Boot is always disabled** — explained in the user guide. The guide and the installer must
  **warn** that some Windows games with anti-cheat (e.g. Battlefield 6, Valorant) require Secure Boot.
- **Automatic mounting** of all internal disks and removable drives (ext4, btrfs, NTFS, exFAT),
  at boot and on hot-plug, with automatic scanning for game libraries and media folders.
  - NTFS left dirty by Windows Fast Startup / hibernation → mount read-only and explain why.
  - Windows games on NTFS with Proton are unreliable → readable, but suggest moving them.

## 7. Controllers

- Wired and 2.4 GHz dongle controllers must work instantly.
- Drivers shipped in the image: kernel HID drivers, **xone** (Xbox wireless dongle).
- **First Bluetooth pairing must need zero input**: when no controller is connected, the system
  scans and **auto-pairs and trusts any device that identifies as a gamepad**.
- Large, animated, per-brand pairing instructions on screen (e.g. "Hold PS + Share").
- Keyboard / mouse remain an optional fallback.

## 8. Features

- **Unified game library** on the home screen (Steam + other stores + local DRM-free games together).
  Stores are opened **only for purchases**.
  - Steam: installed games are read from local Steam files (no API key). Owned-but-not-installed
    games show "Install", which opens Steam.
- **Online features**: rely on existing services (Steam friends/achievements inside
  Steam, Discord, etc.). **No own server.**
- **Chat**: pluggable modules, user picks the service. **Discord web first**, controllable by
  gamepad, optionally running **in the background** (voice keeps working during games;
  notifications and mute/push-to-talk via the overlay).
  - Use only official web clients. **No modded clients or self-bots** (ToS violations).
- No bundled third-party logos (Netflix, Spotify…): fetch icons at runtime or use generic ones.

## 9. UI / UX

- **Original** design: minimal, clean, modern.
- **User-customizable color palette**, implemented with design tokens (colors, radii, spacing).
- Baseline accessibility: high contrast, text scaling, color-blind-safe palettes.
- Open-source fonts only.
- **i18n via JSON message files** with interchangeable labels; as many languages as possible.
  Wave 1 ships Italian + English; adding a language must require no code changes.
- **Mockups are approved by the owner before UI code is written.**
- Approved direction (2026-10-03): `design/mockups/wave1-mockups.html` — deep navy ground, coral
  accent, minimal UI, Bricolage Grotesque (titles) + Lexend (UI text), 1280×720 design grid,
  5% horizontal TV safe area. Palettes (Dusk, Tide, Bloom, Daylight, High contrast) are
  **provisional** and may change.

### 9.1 Controller input map (global, binding for every screen)

Every screen and component must follow this map. A button may never have two meanings on the
same screen. New bindings are added here before they are implemented.

| Button (Xbox / PlayStation) | Meaning                                                                                                                  |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| D-pad / left stick          | Move selection (spatial navigation)                                                                                      |
| A / ✕                       | Confirm / select                                                                                                         |
| B / ○                       | Back / close                                                                                                             |
| X / □                       | Secondary action on the selected item (e.g. Details, Install)                                                            |
| Y / △                       | Page-level option (e.g. Sort, Search)                                                                                    |
| LB / RB (L1 / R1)           | Switch top-level section (Home, Library, Settings, …) — **reserved, never used for anything else**                       |
| LT / RT (L2 / R2)           | Switch filter / sub-tab inside the current page                                                                          |
| Menu / Options              | Options menu for the selected item                                                                                       |
| View / Create               | Reserved (screenshot in a later wave)                                                                                    |
| Guide / PS                  | **Owned by the system daemon**: home / quick menu from anywhere, including in games. Apps and themes must never bind it. |
| Right stick                 | Fast scroll in long lists                                                                                                |

While a game or app is running, the system intercepts **only** the Guide button; every other input
goes to the game.

## 10. Roadmap

The step-by-step plan lives in **[ROADMAP.md](ROADMAP.md)**. Work proceeds one step per prompt,
in order; the owner reviews each step before the next one starts.

### Wave 1 (agreed scope)

1. Bootable and installable ISO.
2. Controller pairing (first-boot flow).
3. Home with unified library (Steam installed games + local DRM-free games).
4. Brave with Netflix, YouTube, Spotify.
5. Basic settings: Wi-Fi, Bluetooth, audio, language, theme.

### Later waves

- Overlay / quick menu over games, chat modules (Discord first), dual boot, Epic/GOG,
  Windows games via umu/Proton, emulation, user profiles, screenshots/recording, play-time stats,
  parental controls, quick suspend/resume, Qt 6 port of the frontend.

### Nice-to-have

- Screen reader / TTS for menus (espeak-ng or Piper, offline) — evaluate if feasible early.
- Crash reporting **only with explicit opt-in consent**. No telemetry otherwise.
- Emulation (RetroArch).
- Support for other OSes is **out of scope** for now.

## 11. Development environment rules

- Never damage the developer's PC or installed OS (host: Linux Mint).
- Keep everything inside the project folder (sources, VM disks, caches, build artefacts).
- ISOs are built inside an **Arch "builder" VM** (QEMU/KVM), never on the host.
- Host packages needed: `qemu-system-x86`, `ovmf`, `qemu-utils` — **ask the owner at the moment
  of installation**, every time something must be installed on the host.
- VirtualBox and KVM cannot run VMs at the same time on recent kernels.
- Repository: public on GitHub (under the account connected to this VS Code instance for now);
  shared with another developer.

## 12. Decision log

| Date       | Decision                                                                              |
| ---------- | ------------------------------------------------------------------------------------- |
| 2026-10-03 | Project scope, architecture and Wave 1 agreed (sections 1–11).                        |
| 2026-10-03 | Secure Boot always disabled; anti-cheat warning in guide.                             |
| 2026-10-03 | Online features: Option A (no own server).                                            |
| 2026-10-03 | Public repo; English mandatory for all project text.                                  |
| 2026-10-03 | Next step: UI mockups for owner approval, then repo structure and builder VM.         |
| 2026-10-03 | Mockup direction approved; palettes provisional (section 9).                          |
| 2026-10-03 | Global controller input map adopted: LB/RB = sections, LT/RT = filters (section 9.1). |
| 2026-10-03 | ROADMAP.md created; work proceeds one step per prompt.                                |
| 2026-10-04 | Dual boot: considered in the design, **not required** (section 6).                    |
| 2026-10-04 | License GPL-3.0-or-later and Conventional Commits confirmed by the owner; step 0.4 approved. |
