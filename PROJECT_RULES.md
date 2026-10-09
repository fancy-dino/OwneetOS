# OwneetOS — Project Rules

> **Source of truth** for every requirement and decision agreed so far.
> Anyone working on this project (humans and AI assistants) must read this file before starting
> and must not contradict it. If something is not covered here, **ask** — do not assume.
> New decisions are added here as they are made (see [Decision log](#13-decision-log)).
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
- Firmware: **UEFI required** (PCs from about 2012 onwards). Legacy BIOS boot is not supported.
  - PCs from about 2012–2019 often offer both modes: if set to "Legacy" / "CSM", the user switches
    to UEFI in the firmware settings. The user guide explains this in the same step as disabling
    Secure Boot, and the system requirements state it clearly.
  - Dual boot (later wave): a Windows installed in Legacy mode cannot be started from the
    OwneetOS boot menu; the installer must detect it and explain it.
- RAM: **4 GB minimum**, 8 GB recommended.
- Disk: **32 GB** minimum.
- GPU: **Vulkan** required for the main (gamescope) session. GPUs without adequate Vulkan support
  (e.g. Intel iGPUs older than ~2015) automatically fall back to a reduced **cage** session
  (no overlay).

## 4. Architecture

| Layer                           | Choice                                                                                                         | Notes                                                                                                                                                                                                                 |
| ------------------------------- | -------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Boot                            | **systemd-boot** + **Plymouth**                                                                                | Silent boot with logo. Auto-detects Windows.                                                                                                                                                                          |
| Login                           | **systemd getty autologin** on tty1 for the fixed console user **`owneet`**                                    | No display manager. People's profiles live in the console UI, not in Linux accounts.                                                                                                                                  |
| Session / compositor            | **gamescope** (main), **cage** (fallback)                                                                      | Inspired by ChimeraOS `gamescope-session`. Overlay via gamescope overlay layer.                                                                                                                                       |
| Frontend / home                 | **Fork of Pegasus Frontend** (`mmatyas/pegasus-frontend`, `master`, cloned with submodules) + custom QML theme | Qt 5.15 now; **Qt 6 port** planned for a later phase.                                                                                                                                                                 |
| Gamepad mappings                | **SDL2** + **SDL_GameControllerDB**                                                                            |                                                                                                                                                                                                                       |
| System daemon                   | **`owneetd`**, own component in **Go**, single static binary, **systemd user service of `owneet` (not root)** | Reads controllers via evdev; Guide button (home / quick menu); system-wide on-screen keyboard via uinput; **HTTP + JSON over a Unix socket** (`$XDG_RUNTIME_DIR/owneetd.sock`, no TCP port) for Wi-Fi (NetworkManager), Bluetooth (BlueZ), audio (PipeWire), power; process launch/kill. Design: `docs/daemon-design.md`. Root-only tasks (install, updates, disks) go to small separate privileged helpers. |
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
    If the ISO cannot stay under it, a free hosting service other than GitHub may be used for the
    ISO (to be decided when needed).
  - Steam's runtime is downloaded on first launch, not shipped in the ISO.
- ISO built with **archiso**, automated with **GitHub Actions**, with checksums and signatures.
- Packages built with **PKGBUILD**s; updates served from an **own pacman repository `[owneet]`**
  hosted on GitHub. Updates are triggered from the Settings menu with the gamepad.
- Arch packages are **pinned to Arch Linux Archive snapshots**; the snapshot date is advanced only
  after testing.
- **Package signing** (two-level OpenPGP key):
  - **Primary key** ("vault"): never used to sign packages; kept **offline by the owner**, with a
    backup outside the repository and outside everyday PCs. It certifies and revokes signing subkeys.
  - **Signing subkey** ("working stamp"): signs packages and the repository; **expires** (e.g. 2 years)
    and is renewed with the primary key. Held by the owner and the second developer; from roadmap
    step 7.2 also stored as a protected GitHub secret for release builds.
  - OwneetOS systems trust the primary key, so a compromised or expired subkey can be replaced
    without any action from users.
  - The key's identity uses a **dedicated project e-mail address** (public: it ships in every ISO),
    never a personal one.
  - The primary key's backup lives in **two places**: a dedicated USB stick and the owner's
    password manager.
  - Private keys never enter the repository or CI logs, and the assistant never reads or copies
    private key files. The owner created the key with `tools/keys/create-signing-key`.
  - **Who signs:** every signature needs the passphrase, typed by a key holder
    (`tools/keys/install-working-key` once per builder VM, then `tools/sign-repo`). From roadmap
    step 7.2, release builds are signed by CI with the subkey stored as a protected GitHub secret.
  - **Development builds** may use the local `[owneet]` repository unsigned (packages are built and
    consumed inside the same builder VM); `tools/build-iso` prints a warning. A signed repository
    is always verified (`SigLevel = Required DatabaseRequired`). **Anything distributed to users
    must be signed.**
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
  - The dongle's **firmware is Microsoft's and cannot be redistributed**: OwneetOS downloads it on
    the user's PC, with the user's consent, the first time a dongle is plugged in (internet needed).
  - First-boot warning: the first setup needs a **Bluetooth or USB-cable controller**; a wireless
    Xbox controller can be used over its USB cable until the dongle is ready.
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
  **English is the default language** (first boot, and fallback for any missing label); Italian is
  one of the additional languages.
- **Mockups are approved by the owner before UI code is written.**
- **Interactive demo before each screen:** before the code of each screen is written (controller
  pairing, home, library, settings, on-screen keyboard, …), the owner gets an interactive demo of
  that screen (HTML, usable with keyboard and gamepad in a browser) to adjust details first. The
  screen is implemented only after the owner approves its demo.
- Approved direction (2026-10-03): `design/mockups/wave1-mockups.html` — deep navy ground, coral
  accent, minimal UI, Bricolage Grotesque (titles) + Lexend (UI text), 1280×720 design grid,
  5% horizontal TV safe area. Palettes are **provisional** and may change.
- **Palettes (2026-10-08, demo 3.3):** about twenty, chosen in a picker window with live preview
  (Settings → Appearance shows only the current one): dark (Dusk, Tide in vivid "minty" cyan,
  Bloom, Ember, Espresso, Harbor navy/beige, Forest, Lagoon, Amethyst, Crimson, Synthwave, Amber,
  Olive, Graphite), light (Daylight, Arctic, Sand, Latte, Sakura), accessibility (high contrast
  dark and light). Every palette passes a contrast check: text ≥ 7:1, secondary text and text on
  the accent ≥ 4.5:1, accent against the background ≥ 3:1.
- **Pickers instead of button rows** for choices that can grow (palette, language): one compact
  row showing the current value opens a window with the full list.
- **Wordmark (provisional, the final logo comes before the public launch, roadmap 7.6):** "Owneet" (light) + "OS" (coral), Bricolage Grotesque 750 / width 80, generated as
  outlines by `tools/branding/make-wordmark` (`packages/owneet-branding/owneetos-wordmark.svg`).

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
| Guide / PS                  | **Owned by the system daemon**: home / quick menu from anywhere, including in games. Apps and themes must never bind it. Prompt label: "Home". |
| Right stick                 | Fast scroll in long lists                                                                                                |

While a game or app is running, the system intercepts **only** the Guide button; every other input
goes to the game.

Keyboard fallback (optional, same meanings): arrows = D-pad, Enter / Space = A, Esc / Backspace = B,
X = X, Y = Y, Q / E = LB / RB, Z / C = LT / RT, M = Menu, Page Up / Page Down = right stick.
The input map is fixed: it is not saved in the frontend's settings, so a change reaches every
system. A held direction repeats after 360 ms every 140 ms; other buttons act once per press.

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
- Repository: **[github.com/fancy-dino/OwneetOS](https://github.com/fancy-dino/OwneetOS)** (public), shared with another developer.
  - `main` is protected by a ruleset: no deletion, no force push, pull request with an approval
    and passing CI checks (`Lint`, `ISO`) required. **The owner (repository admin) is on the bypass
    list** and may push directly; every other contributor goes through pull requests. CI runs on
    every push anyway.
  - There is no GitHub CLI (`gh`) on the host and the assistant's shell has no GitHub credentials:
    the assistant commits locally, the **owner pushes** ("Sync Changes" in VS Code). Other GitHub
    actions are done by the owner through VS Code or the website, until `gh` is installed (with
    the owner's approval).

## 12. Legal compliance

These rules are not legal advice. Before the first public release meant for end users (v1.0), the
project is reviewed by someone experienced in open-source licensing.

- **Source code for distributed binaries (GPL and similar licences).** Whoever distributes compiled
  GPL software (kernel, xone, many Arch packages, OwneetOS's own packages) must provide the
  corresponding source code. Every published ISO and package repository comes with a **source
  archive** (PKGBUILDs and the exact sources of every package it contains) or a valid written
  offer. Until the release pipeline does this (roadmap step 7.2), **CI does not publish downloadable
  ISOs or packages**: ISOs built in CI are checks only; test ISOs are built locally.
- **Licence notices:** every package keeps its licence files (`/usr/share/licenses`); the ISO and
  the UI ("About → Licences", later) list the licences of what OwneetOS ships.
- **Proprietary but redistributable components** (NVIDIA driver, device firmware, CPU microcode)
  are shipped only as their licences allow, unmodified, with their licence texts.
- **Never redistributed:** Widevine, the Xbox wireless dongle firmware, Steam's runtime, games and
  store content. They are downloaded by the user's own system, with consent where required.
- **Trademarks:** third-party names (Xbox, PlayStation, Netflix, Steam…) only to say what works
  with OwneetOS; no third-party logos; never imply endorsement. "Based on Arch Linux" is allowed;
  the Arch logo is not used. The name **OwneetOS** gets a trademark search (EUIPO, Italian register)
  before the public launch.
- **Credits:** the projects OwneetOS is built on are credited by name, author and licence in
  `CREDITS.md` (shipped in the image at `/usr/share/doc/owneetos/CREDITS.md`, shown in the UI
  under "About → Credits and licences" with the full list of installed packages). Every new
  third-party component is added there when it is introduced.
- **Licences of assets:** only free licences that allow commercial use and redistribution
  (e.g. GPL, LGPL, MIT, BSD, Apache, Zlib, OFL, CC0, CC BY, CC BY-SA). Assets under "NonCommercial"
  or "NoDerivatives" terms (e.g. CC BY-NC-SA) are never shipped.
- **Pegasus Frontend fork:** Pegasus's licence adds trademark terms (GPL section 7): the modified
  frontend is named `owneet-frontend`, never uses "Pegasus" or Pegasus's logos as its title or logo,
  keeps Pegasus's copyright notices and credits it. Its CC BY-NC-SA default theme is not used.
- **Third-party services:** only official clients and web apps; no circumvention of DRM or content
  protection; the Brave extension and any user-agent change are checked against each service's
  terms of service before they ship (roadmap step 5.2).
- **Local data first:** game metadata comes from local files; no scraping or undocumented store
  APIs (Pegasus's Steam store, GOG API and Play Store downloads are disabled).
- **Privacy:** no telemetry. Any collection of personal data (e.g. opt-in crash reports) needs
  explicit consent and a privacy notice compliant with the GDPR.

## 13. Decision log

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
| 2026-10-04 | Repository published at github.com/fancy-dino/OwneetOS; `main` ruleset active (step 0.5). |
| 2026-10-04 | `main` ruleset: PR + approval + green CI for contributors; owner bypasses and pushes directly (option A). |
| 2026-10-04 | ISO boots UEFI only (no legacy BIOS); no SSH server, cloud-init or VM guest tools in the ISO (step 1.1). |
| 2026-10-04 | UEFI firmware is a system requirement (PCs from ~2012); Legacy/CSM switch explained in the user guide (section 3). |
| 2026-10-04 | Package signing: two-level key (primary offline with the owner, expiring signing subkey for owner + second developer), dedicated project e-mail (section 5). |
| 2026-10-04 | Signing key created (primary `B3BD F4E3 E477 2D3F 7E86 1A87 8D02 23BC EE51 456E`, `owneet@proton.me`). Key holders sign with their passphrase; development builds may be unsigned; distributed builds must be signed (section 5). |
| 2026-10-05 | Console session: fixed Linux user `owneet` with getty autologin on tty1; gamescope when a hardware Vulkan device exists, otherwise cage; gamescope failing twice falls back to cage (step 1.3). |
| 2026-10-05 | Boot: silent kernel/systemd, hidden systemd-boot menu (hold a key to show it), Plymouth splash `owneet` with the generated wordmark (step 1.4). |
| 2026-10-05 | Boot splash approved by the owner with the wordmark "Owneet" + coral "OS" (as in the mockups page header) instead of "owneet." (section 9). |
| 2026-10-05 | Hardware support (`owneet-hardware`): NVIDIA Turing and newer use `nvidia-open`; older NVIDIA GPUs use nouveau, chosen at boot by `owneet-gpu-select` (no `kms` initramfs hook). ISO 1664 MiB (step 1.5). |
| 2026-10-05 | xone (Xbox wireless dongle) moved out of step 1.5: its firmware is Microsoft's and cannot be redistributed. |
| 2026-10-06 | xone: driver in the image, Microsoft firmware downloaded on the user's PC at first dongle use with consent; first setup needs a Bluetooth or cable controller (section 7). |
| 2026-10-06 | ISO size: if 2 GB is exceeded, consider a free hosting service other than GitHub for the ISO (section 5, decided later). |
| 2026-10-06 | Step 1.5 verified on real hardware (NiPoGi E3B AMD, owner's desktop RTX 4060). Live ISO boots with `copytoram=n` (no copy of the image to RAM: faster boot from USB). |
| 2026-10-06 | xone packaged without `xone_wired` and without blacklisting `xpad`/`mt76x2u` (wired controllers and USB Wi-Fi adapters keep working); CI builds use the ISO's Arch snapshot for build dependencies too (step 1.7). |
| 2026-10-07 | `owneetd`: Go; HTTP + JSON over a Unix socket instead of a localhost TCP port (web pages in Brave cannot reach it); runs as a user service of `owneet`, not root (step 2.1). |
| 2026-10-07 | Legal compliance rules adopted (section 12). CI stops publishing the ISO as a download until releases ship a GPL source archive (option A). |
| 2026-10-07 | Bluetooth: owneetd auto-pairs gamepads whenever no controller is connected; its pairing agent accepts only gamepads, only while auto-pair is on. Verified on real hardware (step 2.5). |
| 2026-10-07 | Wi-Fi networks are saved system-wide: a polkit rule allows `settings.modify.system` to `owneet` in the active local session only; `owneet` is not an administrator (step 2.6). |
| 2026-10-07 | Third-party Go modules are vendored in `daemon/vendor/` (builds need no network; licences shipped with the package). |
| 2026-10-07 | Guide button (step 2.9): in the gamescope session it toggles between the home screen and the running game, which keeps running behind the home screen; in the reduced (cage) session, where the home screen cannot be shown over a game, holding Guide for 2 seconds closes the game and a short press does nothing. |
| 2026-10-07 | gamescope runs with `--steam`: owneetd tags each window with an app id and chooses what is on screen (as Steam does on SteamOS); each game or app runs in its own systemd user service, so closing it stops all of its processes. |
| 2026-10-08 | With the NVIDIA driver, gamescope always composites (direct scanout showed flickering black rectangles on the owner's RTX 4060). |
| 2026-10-08 | Several screens: the console uses one (choice in Settings, step 3.9); the others are not turned off by default, a use for a second screen may come later. |
| 2026-10-08 | Legal check before phase 3: the frontend fork is named `owneet-frontend` (Pegasus's trademark terms); Pegasus's CC BY-NC-SA theme, logo, Roboto fonts and button images are not used; its online metadata downloads are disabled. `CREDITS.md` credits every project OwneetOS builds on and ships in the image (section 12). Step 2.9 approved. |
| 2026-10-08 | Each screen gets an interactive demo approved by the owner before its code is written (section 9). |
| 2026-10-08 | Demo 3.3 feedback: wordmark/logo provisional (final logo before the launch, roadmap 7.6); about twenty palettes (Tide in vivid "minty" cyan; new Ember, Espresso, Harbor and more) in a picker with live preview; language chosen from a list; the Guide prompt reads "Home" (section 9). |
| 2026-10-08 | Step 3.3 approved. Button prompts also have a Nintendo-style set (chosen automatically for Switch controllers). Small hardware checks are batched with the next larger test instead of a new USB stick each. |
| 2026-10-08 | English is the default UI language and the fallback for missing labels; Italian is one of the additional languages (section 9). |
| 2026-10-08 | Step 3.4 approved. Input map implemented once for every screen (step 3.5): keyboard fallback keys, fixed map, repeat timing (section 9.1). |
| 2026-10-08 | Interface sounds planned (roadmap 3.12, list in `design/sounds.md`): WAV files provided by the owner. |
| 2026-10-09 | The owner's final sounds are delivered before the public launch, after the legal review (roadmap 7.7); until then the interface uses CC0 placeholders by Kenney (`frontend/sounds/`). |
