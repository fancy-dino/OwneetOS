# OwneetOS

**Plug it. Play it. Own it!**

OwneetOS is a free and open source system that turns your PC into a game console. You only need a
controller: installing it, setting it up, playing, watching and changing the settings all work
with a controller. Mouse and keyboard are optional.

Website: <https://owneetos.org>

> **Status: almost here.** The first release is being tested on real PCs; the download opens with
> it, on the website.

## What it does

- **A game console from your PC**, with a lightweight system.
- **A controller is all you need.** Mouse and keyboard are optional.
- **One library for all your games**: Steam's, DRM-free games and games on a disk.
- **Games on a disk?** Plug it in and play, or copy them to the console.
- **Films, series, music**: stream them or play your own.
- **Safe updates.** Something wrong? Roll back to your version of choice.
- **Your style**: several palettes and languages.

And it belongs to everyone: completely free and open source, no forced telemetry, updates never
forced on you, a community driven project.

## System requirements

| | Minimum | Recommended |
|---|---|---|
| Processor | 64-bit (x86-64) | 64-bit (x86-64) |
| Firmware | UEFI, with Secure Boot off | UEFI, with Secure Boot off |
| Memory | 4 GB RAM | 8 GB RAM |
| Graphics | any card (without Vulkan the interface runs in a reduced mode) | with Vulkan support |
| Storage | 32 GB | 32 GB + the space your games need |
| To install | a USB stick of 4 GB or more (it is erased) | the same |
| Controller | Bluetooth, or with a cable | the same |

Installing erases the whole disk you choose; dual boot is currently not supported. The website's
[install guide](https://owneetos.org/guide) explains every step.

## Documents

| File | What it is |
|------|------------|
| [docs/setup.md](docs/setup.md) | **Start here** to work on the code: requirements and setting up the workspace with the code and the knowledge base. |
| `OwneetOS-knowledge-base` (separate repository, an Obsidian vault) | Every agreed requirement and decision, the step-by-step plan and its status, design documents. Source of truth. |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to work on the project. |
| [docs/translating.md](docs/translating.md) | How to translate the console into your language. |
| [docs/tester-install.md](docs/tester-install.md) | Installing a test build on a PC, for testers. |
| [CREDITS.md](CREDITS.md) | The projects OwneetOS is built on, their authors and licences. |
| [design/](design/) | Approved mockups and interactive demos of every screen (open them in a browser). |

## Repository layout

| Folder | Contents |
|--------|----------|
| [`iso/`](iso/) | archiso profile that produces the bootable ISO. |
| [`packages/`](packages/) | PKGBUILDs for the `[owneet]` pacman repository, the installer (`owneet-installer`) included. |
| [`daemon/`](daemon/) | `owneetd`, the system daemon (controllers, Guide button, Wi-Fi, Bluetooth, audio, power, disks, games and apps). |
| [`frontend/`](frontend/) | The console interface (`owneet-frontend`, based on Pegasus Frontend), the OwneetOS theme and its translations. |
| [`extension/`](extension/) | Brave extension that makes streaming sites usable with a controller. |
| [`compat/`](compat/) | OwneetOS's own compatibility libraries for old Linux games (`lib32-owneet-compat`). |
| [`installer/`](installer/) | Notes on the installer. |
| [`release-notes/`](release-notes/) | The notes of each release, in every language of the console. |
| [`tools/`](tools/) | Developer scripts (builds in the builder VM, previews, checks). |
| [`vm/`](vm/) | Builder and test virtual machines. Images stay local and are never committed. |
| [`docs/`](docs/) | Technical and user documentation. |

## Credits

OwneetOS stands on the shoulders of Arch Linux, the Linux kernel, systemd, gamescope, Mesa,
PipeWire, BlueZ, NetworkManager, SDL, Pegasus Frontend and many more free software projects.
[CREDITS.md](CREDITS.md) names them, their authors and their licences. Thank you to everyone who
built them.

Steam is a trademark of Valve Corporation, Xbox of Microsoft Corporation, PlayStation of Sony
Interactive Entertainment. They only say what works with OwneetOS, which is not affiliated with or
endorsed by them.

## License

OwneetOS is free software, released under the [GNU General Public License v3.0 or later](LICENSE).
Third-party components keep their own licenses.
