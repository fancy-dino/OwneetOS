# OwneetOS

OwneetOS is a lightweight, open-source Linux distribution that turns any x86_64 PC into a game
console. Everything — installation, first setup, games, streaming apps, settings — is controlled
with a gamepad. Mouse and keyboard are never required.

> **Status: pre-alpha.** Nothing is usable yet. The project is in its foundation phase; see
> [ROADMAP.md](ROADMAP.md) for where we are.

## Goals

- **Gamepad only**, from the first boot onwards.
- **Lightweight**: runs on low-budget hardware (4 GB RAM minimum, x86_64).
- **One library** for Steam and DRM-free games; streaming apps (Netflix, YouTube, Spotify) in a
  console-style interface.
- **Built for people with no technical background.**
- **Fully open source** (GPLv3).

## System requirements

| | Minimum |
|---|---|
| Processor | 64-bit x86 (Intel or AMD), roughly the last 10 years |
| Firmware | **UEFI** (PCs from about 2012). If your PC is set to "Legacy" or "CSM" boot, switch it to UEFI in the firmware settings. Secure Boot must be disabled. |
| Memory | 4 GB (8 GB recommended) |
| Storage | 32 GB |
| Graphics | Vulkan support for the full experience; older GPUs run a reduced mode |

## Documents

| File | What it is |
|------|------------|
| [PROJECT_RULES.md](PROJECT_RULES.md) | Every agreed requirement and decision. Source of truth. |
| [ROADMAP.md](ROADMAP.md) | Step-by-step plan and current status. |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to work on the project. |
| [CREDITS.md](CREDITS.md) | The projects OwneetOS is built on, their authors and licences. |
| [design/mockups/wave1-mockups.html](design/mockups/wave1-mockups.html) | Approved UI mockups (open in a browser). |

## Repository layout

| Folder | Contents |
|--------|----------|
| [`iso/`](iso/) | archiso profile that produces the bootable ISO. |
| [`packages/`](packages/) | PKGBUILDs for the `[owneet]` pacman repository. |
| [`daemon/`](daemon/) | `owneetd`, the system daemon (controllers, Guide button, Wi-Fi, Bluetooth, audio, power, app launching). |
| [`frontend/`](frontend/) | The console interface (`owneet-frontend`, based on Pegasus Frontend) and the OwneetOS theme. |
| [`extension/`](extension/) | Brave extension that makes streaming sites usable with a gamepad. |
| [`installer/`](installer/) | Gamepad-driven installer (UI and backend). |
| [`tools/`](tools/) | Developer scripts (build wrappers, helpers). |
| [`vm/`](vm/) | Builder and test virtual machines. Images stay local and are never committed. |
| [`docs/`](docs/) | Technical and user documentation. |
| [`design/`](design/) | Mockups and design assets. |

## Credits

OwneetOS stands on the shoulders of Arch Linux, the Linux kernel, systemd, gamescope, Mesa,
PipeWire, BlueZ, NetworkManager, SDL, Pegasus Frontend and many more free software projects.
[CREDITS.md](CREDITS.md) names them, their authors and their licences. Thank you to everyone who
built them.

## License

OwneetOS is free software, released under the [GNU General Public License v3.0 or later](LICENSE).
Third-party components keep their own licenses.
