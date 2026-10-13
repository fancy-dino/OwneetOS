# Installing OwneetOS on a test PC

For **testers only** (roadmap step 4.2a). The user installer, used with the controller and with the
console's look, is step 6.4. This one is a text screen used with a keyboard, and it leaves the
user ISO with the other test boot entries (roadmap 7.4).

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

## What the installed system looks like

| Part | Value |
|---|---|
| Partitions (GPT) | 1 GiB EFI system partition (FAT32, label `OWNEETBOOT`), the rest btrfs (label `OwneetOS`) |
| btrfs subvolumes | `@` (/), `@home`, `@log` (/var/log), `@pkg` (pacman's cache), `@snapshots` (ready for snapshots, 6.3) |
| Boot | systemd-boot, hidden menu (hold a key at start-up): OwneetOS, diagnostics, fallback start-up image |
| System | the same files as the USB stick that was tested; initramfs without the live hooks |
| pacman | the same Arch Linux Archive snapshot as the stick |
| Tester mark | `/etc/owneet/tester` (updates from a USB stick, step 4.2b) |
| Log | `/var/log/owneet-tester-install.log` |

## Updating a test PC (step 4.2b)

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

## If something goes wrong

- The installed system keeps the **diagnostics** boot entry (hold a key at start-up to see the
  menu); the OwneetOS USB stick still starts the live system.
- The OWNEETLOG stick collects the logs on the installed system too.
