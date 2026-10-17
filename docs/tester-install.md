# Installing OwneetOS on a test PC

For **testers only** ([[step-4.2a-tester-installer]]). The user installer, used with the controller
and with the console's look, is [[step-6.4-installer-ui]]. This one is a text screen used with a
keyboard, and it leaves the user ISO with the other test boot entries
([[step-7.4-hardware-testing-and-v0.1.0]]).

## Before you start

- The PC starts in **UEFI** mode (not "Legacy" / "CSM") and **Secure Boot is off**, in the PC's
  firmware settings.
- A **keyboard** is plugged in.
- Internet is not needed: the system is copied from the USB stick.
- The disk you choose is **erased completely**. Other disks are not touched.

## Installing

1. Start the PC from the OwneetOS USB stick and hold a key to show the boot menu.
2. Choose **"Install OwneetOS (testers: erases a disk)"**.
3. The screen lists the internal disks (the USB stick and other USB disks are never shown), with
   their size, model and current partitions. Type the number of the disk and press Enter.
4. Type `ERASE` and press Enter. The installation takes a few minutes.
5. Remove the USB stick and press Enter: the PC restarts into the installed OwneetOS.
   If the PC says that no boot device was found, choose **"Linux Boot Manager"** (or
   **"Fallback Linux Boot Manager"**: the same boot manager) as the first boot device in its
   firmware settings.

## What the installed system looks like

| Part | Value |
|---|---|
| Partitions (GPT) | 1 GiB EFI system partition (FAT32, label `OWNEETBOOT`), the rest btrfs (label `OwneetOS`) |
| btrfs subvolumes | `@` (/), `@home`, `@log` (/var/log), `@pkg` (pacman's cache), `@snapshots` (ready for snapshots, 6.3) |
| Boot | systemd-boot, hidden menu (hold a key at start-up): OwneetOS, diagnostics, fallback start-up image |
| System | the same files as the USB stick that was tested; initramfs without the live hooks |
| pacman | the same Arch Linux Archive snapshot as the stick |
| Tester mark | `/etc/owneet/tester` (updates from a USB stick, [[step-4.2b-updates-from-a-usb-stick]]) |
| Log | `/var/log/owneet-tester-install.log` |

## Updating a test PC ([[step-4.2b-updates-from-a-usb-stick]])

No new USB stick to write: the new OwneetOS packages travel on the **OWNEETLOG** stick.

1. On the developer's PC, with the OWNEETLOG stick plugged in:
   `tools/make-update-stick /media/$USER/OWNEETLOG` (copies `out/repo/x86_64` into the stick's
   `owneet-updates` folder). Unmount the stick.
2. Plug it into the test PC:
   - **at start-up**, newer packages are installed before the console starts (a message on the
     start-up screen); this also fixes a console that no longer starts;
   - or from **Settings → System → Check for updates**: the list of newer packages, Install, then
     Restart.

Only OwneetOS's own packages come from the stick (unsigned, accepted only by systems installed
with this installer); Arch Linux packages come from the pinned snapshot online, when the new
packages need any. The result of the last check is in `/run/owneet/update.json`.

## Test switches

The session's test switches (the `owneet.debug.*` kernel options listed at the top of
`owneet-session`) can also be written, one per line, in a file **`owneet-debug.txt`** at the top of
the OWNEETLOG stick, e.g. `owneet.debug.gamescope_explicit_sync=off`: they apply at every start
while the stick is plugged in, with no boot menu to edit. The hardware report on the stick lists
the switches that were set.

The windows on screen (e.g. pictures that should not be there) are written to
`owneet-…windows.txt` every 10 s by the test tool `owneet-xprobe`, installed with owneetd (a copy at
the top of the stick is used first).

**With a single USB stick** (the OwneetOS one, live system only): without an OWNEETLOG stick the
logs go to the OwneetOS stick itself, in its small FAT boot partition (`ARCHISO_EFI`, about 7 MB
free; the boot files are not touched): the system log up to 4 MB, the report and the windows up to
1 MB each, no Steam logs. Afterwards, on the developer's PC, that partition shows up as
`ARCHISO_EFI` when the stick is plugged in, with the `owneet-…` files at its top.

## If something goes wrong

- The installed system keeps the **diagnostics** boot entry (hold a key at start-up to see the
  menu); the OwneetOS USB stick still starts the live system.
- The OWNEETLOG stick collects the logs on the installed system too.
