# owneet-frontend — architecture (roadmap step 3.1)

The OwneetOS console interface is **owneet-frontend**, a modified version of
[Pegasus Frontend](https://pegasus-frontend.org) by Mátyás Mustoha and contributors
(GPL-3.0-or-later with additional terms). This document describes how Pegasus is built, how it
works, and what OwneetOS changes in C++ and in QML. Legal requirements: PROJECT_RULES.md
section 12 and the introduction of roadmap phase 3.

## 1. Where the code lives

- `frontend/owneet-frontend/`: the Pegasus source, imported **unmodified** in one commit
  (upstream `master`, commit `5d58223e58f84afb31d5d14a67438841b981a6fa`, 2026-10-02), with
  `thirdparty/SortFilterProxyModel` (MIT, commit `2061f8136ba372fd06c1928a610258b7d88cb144`).
  Every OwneetOS change is a separate, later commit, so it can always be compared with upstream.
- **Not imported:** `src/themes/pegasus-theme-grid` (CC BY-NC-SA 4.0, not free), `lang/`
  (translations of Pegasus's own menus, no declared licence), upstream CI files.
- The OwneetOS theme (roadmap 3.3 onwards) is QML inside the fork, under `src/themes/`.
- Third-party code: excluded from the project's shellcheck, SPDX and markdownlint checks.

**Updating from upstream:** list the upstream commits since the imported one, read them, and
apply the useful ones with `git am --directory=frontend/owneet-frontend` (or by hand), one commit
each, noting the upstream commit id in the message.

## 2. Build

| | |
|---|---|
| Build system | CMake ≥ 3.16 (a qmake project also exists; we use CMake) |
| Toolkit | **Qt 5.15** (Arch: 5.15.19 with KDE's patch collection): Core, Qml, Quick, QuickCompiler, Multimedia, Sql, Svg, LinguistTools |
| Gamepads | SDL2 (Arch: `sdl2-compat` on SDL3), option `PEGASUS_USE_SDL2_GAMEPAD` (on) |
| Battery info | SDL2, option `PEGASUS_USE_SDL2_POWER` (on) |
| Result | one executable (`pegasus-fe`, 3.5 MB), QML compiled into it |
| Time | about 35 s in the builder VM (4 CPUs) |

Checked on 2026-10-08 in a chroot pinned to the ISO's Arch snapshot: the build succeeds; 20 of
the 23 upstream tests pass with `QT_QPA_PLATFORM=offscreen`; 3 QML scene tests (`KeyEditor`,
`Blurhash`, `SortFilter`) abort without a GPU context, exactly as on unmodified upstream (to be
run in the test VM, step 3.2).

**Qt 6 is not possible yet.** Pegasus's CMake looks for Qt 6 first, but the frontend still needs
Qt 5-only parts (`QuickCompiler`, the Qt 5 translation macros): with Qt 6 installed, CMake mixes
both versions and stops. The Qt 6 port stays a later step, as planned (PROJECT_RULES.md section 4).

**Image size:** the frontend adds about 95 MiB installed (Qt 5 base, declarative, multimedia,
SVG and a few small libraries), roughly 30 MiB in the compressed ISO (now 1671 MiB of 2 GB).

## 3. How Pegasus works

```text
main.cpp ── Backend ──┬── FrontendLayer ── QML engine: main.qml (menus) + the theme (theme.qml)
                      ├── ProviderManager ── providers ── game list (Collections, Games)
                      ├── ProcessLauncher ── QProcess: starts the game as a child process
                      └── model::Api ── exposed to QML as `api`
```

- **Backend (C++, about 20 000 lines):** settings (`AppSettings`, an INI-like file), paths
  (`Paths`: `~/.config/pegasus-frontend`), data providers, game launching, gamepads, power.
- **Frontend (QML, about 5 600 lines):** Pegasus's own screens: main menu (Start button),
  settings (game folders, data providers, key and gamepad editors), help, reboot/shutdown dialogs.
- **Theme (QML):** the screen the user sees. Pegasus loads `theme.qml` from a theme folder (or the
  built-in theme) and gives it the `api` object.

### Theme API (what the OwneetOS theme can use)

| `api.…` | What |
|---|---|
| `collections`, `allGames` | Lists (models) of collections and games |
| game: `title`, `summary`, `description`, `releaseYear`, `players`, `rating`, `assets` (cover, logo, background, screenshots, videos…) | Game data |
| game: `playCount`, `playTime`, `lastPlayed`, `favorite` | Play statistics and favourites, stored locally |
| game: `launch()` | Start the game |
| `keys` | Key bindings: `isAccept(event)`, `isCancel`, `isDetails`, `isFilters`, `isNextPage`, `isPrevPage`, `isPageUp`, `isPageDown`, `isMenu`, and the directions |
| `device` | Battery level (SDL) |
| `memory` | Small key/value store for the theme (persisted) |

Pegasus's own menus also get `Internal` (a separate, private object): settings, gamepad list,
`system.quit/reboot/shutdown/suspend`. The OwneetOS theme uses owneetd for these instead (3.8).

### Game data providers (chosen at compile time, `WITH_COMPAT_*`)

| Provider | What it reads | OwneetOS |
|---|---|---|
| `pegasus_metadata` | `metadata.pegasus.txt` files in game folders (title, launch command, assets) | **keep**: local DRM-free games |
| `pegasus_media`, `pegasus_playtime`, `pegasus_favorites` | media folders, play time, favourites | **keep** |
| `steam` | local Steam library files, **plus** `store.steampowered.com/api/appdetails` | **keep, without the store download** (local data only; covers from Steam's local cache) — reviewed in 4.1 |
| `gog` | GOG Galaxy files, **plus** `api.gog.com` | **compile out** (GOG comes later with our own integration) |
| `android_apps` | Android apps, **plus** Play Store pages | **compile out** |
| `es2`, `launchbox`, `logiqx`, `lutris`, `playnite`, `skraper` | other frontends' and launchers' data | **compile out** (not needed; less code, less risk) |

### Game launching today

`game.launch()` → `ProcessLauncher` runs the launch command as a **child process** of Pegasus.
Before starting it, Pegasus **unloads the whole QML interface** (`FrontendLayer::teardown`) to
free memory, and reloads it when the game ends.

This does not fit OwneetOS: owneetd starts games in their own systemd services (closing a game
closes all its processes) and the **Guide button switches between the home screen and the
running game**, so the home screen must stay loaded while a game runs.

### Input

SDL2 reads the gamepads and turns buttons into key events for QML (A = accept, B = back,
X = details, Y = filters, L1/R1 = previous/next page, L2/R2 = page up/down, Start = Pegasus's main
menu). **Guide is also turned into a key** (not bound to anything by default).

OwneetOS (3.2, 3.5): Guide is removed; the keys keep Pegasus's names but follow the OwneetOS
input map (PROJECT_RULES.md section 9.1): `prevPage/nextPage` = LB/RB (sections),
`pageUp/pageDown` = LT/RT (filters), `filters` = Y (page option), `details` = X, `menu` = Menu;
new `scrollUp/scrollDown` = right stick (fast scrolling). The theme never reads them directly: it
uses `Nav.action(event)`.

## 4. What OwneetOS changes

### In C++ (small, targeted changes)

Done in step 3.2: names, providers and Steam download, Guide removed, window tag, Pegasus's
screens hidden, and also: the interface stays loaded during games (no teardown, no blocking wait), button
repeat stopped when input pauses, SDL's SIGTERM handler disabled, pointer hidden until the mouse
is used. Still to do: owneetd client, launching and power through owneetd (3.8), input map (3.5).

| Change | Why | Step |
|---|---|---|
| Names: window title, application and organisation name, executable `owneet-frontend`, config folder `~/.config/owneet-frontend`; Pegasus logo and icons not installed | Pegasus's trademark terms (PROJECT_RULES.md section 12) | 3.2 |
| Build only the providers listed above; remove the Steam store download | Local data only, offline (section 12, C9) | 3.2 |
| Pegasus's Roboto fonts and button images removed; Bricolage Grotesque and Lexend bundled (OFL-1.1, static instances from `tools/branding/make-fonts`) | Own fonts and glyphs (section 12) | 3.3 |
| **owneetd client**: a small C++ object exposed to QML (`owneetd.get/post/put/delete` and an event stream) | QML's `XMLHttpRequest` cannot reach a Unix socket; owneetd listens only there (no TCP port, by design) | 3.8 |
| **Game launching through owneetd** (`POST /v1/apps/launch`) instead of `QProcess`; **no interface teardown**; play time updated from `app.exited` | Guide toggles home ↔ game; games in their own services | 3.8 / 4.x |
| Power actions (`Internal.system.reboot/shutdown/suspend`) through owneetd | One place for power, with its events | 3.8 |
| **Guide removed** from the gamepad → key mapping | Guide belongs to the system (section 9.1) | 3.2 |
| Gamepad → key mapping changed to the OwneetOS input map (LB/RB = sections, LT/RT = filters, Y = page option, Menu = options, right stick = fast scrolling), fixed (not saved in the settings) | Section 9.1 | 3.5 |
| The window tags itself for gamescope (`STEAM_GAME` with owneetd's home app id) | Shown even if owneetd is not running (daemon-design.md) | 3.2 |
| Pegasus's own screens (main menu on Start, settings, help, editors) not shown | Replaced by OwneetOS screens | 3.2 / 3.9 |

The theme API, the data model, the providers we keep, play statistics, favourites, the SQLite
cache and the QML engine stay as they are.

### In QML (everything the user sees)

The **OwneetOS theme** (`src/themes/owneet/`, the built-in default): design tokens and palettes (3.3), translations with JSON message files
(3.4), navigation (3.5), home (3.6), library (3.7), settings (3.9), on-screen keyboard (3.10),
notifications (3.11). Each screen starts with an interactive demo approved by the owner.

**Foundations (3.3)**, in `src/themes/owneet/foundation/`, used by every screen:

| Component | What |
|---|---|
| `Theme` (singleton) | Palettes (eight color tokens each), fonts, sizes on the 1280×720 grid (`px()`), text sizes that follow the "Text size" option (`fs()`), TV safe area, reduce motion, button glyph set |
| `FocusFrame` | Focus ring (background gap + accent ring) and lift of the selected item |
| `Glyph`, `Prompt`, `PromptBar` | Button glyphs drawn in QML (Xbox- and Nintendo-style letters, PlayStation-style shapes, chosen from the connected controller's name), the prompt bar |
| `Sheet`, `PalettePicker` | Window over the screen with its own prompt bar; palette picker with live preview |
| `Label`, `Choice`, `SettingRow` | Section label, segmented choice, settings row (switch or picker) |
| `Tr` (singleton), `LanguagePicker` | Translated text (3.4), language list |
| `Nav` (singleton), `NavArea`, `NavGroup` | Input map as actions, spatial navigation (with groups that remember their selection, 3.6), list keys, feedback for sounds (3.5) |
| `Button`, `Dialog`, `Toast` | Round buttons, compact centred windows with a row of buttons, short messages (3.6) |
| `GameInfo` (singleton), `Cover`, `GameArt` | Texts about a game, covers, art generated from the title (3.6) |

**Translations (3.4):** `owneet::I18n` (`src/backend/owneet/`, exposed to QML as `i18n`) loads one
JSON file per language from `/usr/share/owneet-frontend/i18n/` (repository: `frontend/i18n/`) and
`~/.config/owneet-frontend/i18n/`; English is the default and the fallback. The theme uses it
through the `Tr` singleton. How to translate: [translating.md](translating.md).

Appearance choices are saved in the theme's memory (`api.memory`). Until Settings exists (3.9),
they are opened from the temporary Settings section (`SettingsPage.qml` → `AppearanceSheet.qml`).

**Shell and sections (3.6):** `theme.qml` is the shell: top bar (wordmark, sections with the LB / RB
glyphs, controller, clock), the sections on LB / RB, the prompt bar (each section gives its
prompts, following the selected item), the windows (`Dialog`: details, options, confirmations;
`Sheet`: pickers) and short messages (`Toast`). Sections: `HomePage.qml` (hero, apps, notices,
recently played, in three `NavGroup`s), `LibraryPage.qml` (a plain list until 3.7),
`SettingsPage.qml` (Appearance only until 3.9). `GameInfo` gives the texts about a game (source,
last played, play time) and the order "most recently played first"; `Cover` and `GameArt` draw a
game's own image from local files or, without one, art generated from its title.

### Not changed / not needed

The qmake project, Android/Windows/macOS code paths, and the tests of removed providers stay in
the tree (unused) unless they get in the way; removing them would make upstream updates harder.

## 5. Checks for step 3.2

- The frontend runs in gamescope in the test VM with the minimal theme, driven by a gamepad.
- The 3 QML tests run in the test VM (GPU context).
- RAM of the frontend at idle (constraint C1: system + UI under 500 MB).
- Seamless handover from the boot splash to the interface.
