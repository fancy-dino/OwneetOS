# Credits

OwneetOS is a small layer on top of an enormous amount of work by other people. Almost everything
that makes it boot, draw, play sound, talk to controllers and run games was written by the free
software community, often by volunteers, over decades. **Thank you.**

This page names the projects OwneetOS is built on and how they are licensed. The complete list of
every package in an OwneetOS image, with its licence, is generated from the system itself (roadmap
step 3.9, "About → Credits and licences"); every package's licence texts are installed in
`/usr/share/licenses`.

Licences are given as [SPDX](https://spdx.org/licenses/) identifiers, as declared by each package.
"And contributors" is meant literally: these projects are the work of many people.

## Foundations

| Project | Who | Licence | What it does in OwneetOS |
|---|---|---|---|
| [Arch Linux](https://archlinux.org) | The Arch Linux developers, package maintainers and community | various | The base distribution: packages, `pacman`, the Arch Linux Archive snapshots we pin to |
| [archiso](https://gitlab.archlinux.org/archlinux/archiso) | Arch Linux contributors | GPL-3.0-or-later | Builds the OwneetOS ISO |
| [Linux](https://kernel.org) | Linus Torvalds and thousands of contributors | GPL-2.0-only | The kernel, and its drivers for controllers, GPUs, Wi-Fi and Bluetooth |
| [linux-firmware](https://gitlab.com/kernel-firmware/linux-firmware) | Hardware vendors and kernel-firmware maintainers | various, redistributable | Firmware for GPUs, Wi-Fi, Bluetooth and more |
| [systemd](https://systemd.io) | systemd contributors | LGPL-2.1-or-later and others | Boot (systemd-boot), services, login, power, the user services that run apps |
| [Plymouth](https://www.freedesktop.org/wiki/Software/Plymouth/) | Plymouth contributors | GPL-2.0-or-later | The boot splash |
| [pacman](https://archlinux.org/pacman/) | pacman contributors | GPL-2.0-or-later | Package manager and the `[owneet]` repository format |

## Graphics and the console session

| Project | Who | Licence | What it does in OwneetOS |
|---|---|---|---|
| [gamescope](https://github.com/ValveSoftware/gamescope) | Valve and contributors | BSD-2-Clause, BSD-3-Clause and others | The main compositor of the console session |
| [cage](https://www.hjdskes.nl/projects/cage/) | Jente Hidskes and contributors | MIT | The reduced session for GPUs without Vulkan |
| [wlroots](https://gitlab.freedesktop.org/wlroots/wlroots) | wlroots contributors | MIT | Compositor library used by cage |
| [Mesa](https://www.mesa3d.org) | Mesa contributors | MIT, BSD-3-Clause, SGI-B-2.0 | OpenGL and Vulkan drivers for AMD and Intel GPUs |
| [Vulkan loader](https://www.vulkan.org) | Khronos Group and LunarG contributors | Apache-2.0 | Loads Vulkan drivers |
| [NVIDIA open GPU kernel modules](https://github.com/NVIDIA/open-gpu-kernel-modules) and driver | NVIDIA | MIT and GPL-2.0-only (kernel modules); NVIDIA driver licence (user space) | NVIDIA GPU support, redistributed unmodified as its licence allows |
| [mpv](https://mpv.io) | mpv contributors | GPL-2.0-or-later and LGPL-2.1-or-later | Local media playback and the placeholder screen |
| [FFmpeg](https://ffmpeg.org) | FFmpeg contributors | GPL-3.0-only (as packaged) | Audio and video decoding |

## Sound, network, Bluetooth, system services

| Project | Who | Licence | What it does in OwneetOS |
|---|---|---|---|
| [PipeWire](https://pipewire.org) | Wim Taymans and contributors | MIT and LGPL-2.1-or-later | Sound |
| [WirePlumber](https://pipewire.pages.freedesktop.org/wireplumber/) | Collabora and contributors | MIT | Sound session policy (outputs, remembered volumes) |
| [NetworkManager](https://networkmanager.dev) | NetworkManager contributors | GPL-2.0-or-later and LGPL-2.1-or-later | Wi-Fi and wired network |
| [BlueZ](http://www.bluez.org) | BlueZ contributors | GPL-2.0-only | Bluetooth, including controller pairing |
| [polkit](https://github.com/polkit-org/polkit) | polkit contributors | LGPL-2.0-or-later | Permissions of the console user |
| [dbus-broker](https://github.com/bus1/dbus-broker) | dbus-broker contributors | Apache-2.0 | The D-Bus message bus |

## Controllers

| Project | Who | Licence | What it does in OwneetOS |
|---|---|---|---|
| [SDL](https://www.libsdl.org) | Sam Lantinga and contributors | Zlib | Game controller support for games and the frontend |
| [SDL_GameControllerDB](https://github.com/mdqinc/SDL_GameControllerDB) | Gabriel Jacobo and contributors | Zlib | Button layouts of hundreds of controllers (owneetd uses it to find the Guide button) |
| [xone](https://github.com/dlundqvist/xone) | medusalix (original author), dlundqvist (maintainer) and contributors | GPL-2.0-or-later | Driver for the Xbox wireless dongle |

## The OwneetOS daemon (`owneetd`)

| Project | Who | Licence | What it does in OwneetOS |
|---|---|---|---|
| [Go](https://go.dev) | The Go Authors | BSD-3-Clause | The language and standard library of `owneetd` |
| [godbus/dbus](https://github.com/godbus/dbus) | Georg Reinke and contributors | BSD-2-Clause | D-Bus: Bluetooth, network, power, apps |
| [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys) | The Go Authors | BSD-3-Clause | Low-level system calls (used by godbus) |
| [jfreymuth/pulse](https://github.com/jfreymuth/pulse) | Johann Freymuth | MIT | Talks to PipeWire for volume and outputs |
| [jezek/xgb](https://github.com/jezek/xgb) | The XGB Authors (originally by Andrew Gallant), maintained by jezek | BSD-3-Clause | Talks to gamescope to choose what is on screen |

## The console interface (from roadmap phase 3)

| Project | Who | Licence | What it does in OwneetOS |
|---|---|---|---|
| [Pegasus Frontend](https://pegasus-frontend.org) | Mátyás Mustoha and contributors | GPL-3.0-or-later, with additional terms | The base of the OwneetOS interface (`owneet-frontend`) |
| [SortFilterProxyModel](https://github.com/oKcerG/SortFilterProxyModel) | Pierre-Yves Siret | MIT | Sorting and filtering of lists in the interface |
| [Qt](https://www.qt.io) | The Qt Company, KDE and Qt Project contributors | LGPL-3.0 / GPL-3.0 | The toolkit the interface is written with |

OwneetOS's interface is a modified version of Pegasus Frontend. As its licence requires, it does
not use the names "Pegasus Frontend", "Pegasus Launcher" or "Pegasus" as its title, nor Pegasus's
logos; Pegasus's copyright notices are kept in its source code.

## Fonts and design

| Project | Who | Licence | Use |
|---|---|---|---|
| [Bricolage Grotesque](https://github.com/ateliertriay/bricolage) | Mathieu Triay | OFL-1.1 | Titles; the OwneetOS wordmark is drawn from its outlines |
| [Lexend](https://www.lexend.com) | Bonnie Shaver-Troup, Thomas Jockin and the Lexend team | OFL-1.1 | Interface text |

Both fonts are bundled in `owneet-frontend` as static instances generated from the variable fonts
by `tools/branding/make-fonts` (Qt 5 cannot select variation axes); their licence texts are
installed in `/usr/share/licenses/owneet-frontend/`.

## Inspiration

OwneetOS learned a lot from projects that walked this road first:

- [ChimeraOS](https://chimeraos.org) and its `gamescope-session`: how a console session around
  gamescope is put together.
- [SteamOS](https://store.steampowered.com/steamos) and Valve's work on gamescope: the model of a
  shell that chooses what gamescope shows.
- [OpenGamepadUI](https://github.com/ShadowBlip/OpenGamepadUI): showing that a console interface
  other than Steam can drive gamescope.
- [Pegasus Frontend](https://pegasus-frontend.org): a fast, gamepad-first frontend we build on.

## Development tools

[QEMU](https://www.qemu.org) and [EDK II / OVMF](https://github.com/tianocore/edk2) (virtual
machines for building and testing), [fontTools](https://github.com/fonttools/fonttools) (wordmark and font instances),
[ShellCheck](https://www.shellcheck.net) and [markdownlint](https://github.com/DavidAnson/markdownlint)
(checks), [GitHub Actions](https://github.com/features/actions) (CI).

## Trademarks

Names such as Xbox, PlayStation, Steam, NVIDIA, AMD, Intel, Netflix, YouTube and Spotify are
trademarks of their respective owners. They are used only to say what works with OwneetOS.
OwneetOS is not affiliated with or endorsed by them, nor by any of the projects listed here.
"Based on Arch Linux" is said with gratitude; OwneetOS is not an official Arch Linux project.

## Missing someone?

If your work is in OwneetOS and is missing here, or credited wrongly, please open an issue: we
will fix it.
