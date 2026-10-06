# iso/

The **archiso profile** that produces the OwneetOS bootable ISO. Derived from archiso's `baseline`
profile (archiso 91), reduced to what a console needs.

## Build and test

```text
tools/build-in-vm tools/build-iso      # builds out/owneetos-YYYY.MM.DD-x86_64.iso (+ .sha256)
vm/test.sh boot --headless             # boots the newest ISO in out/
vm/test.sh wait-serial 'login:'        # waits for the login prompt on the serial console
vm/test.sh screenshot                  # what the screen shows (vm/run/test/screen.png)
```

The build runs only inside the builder VM (it needs root). `tools/build-iso` fails if the ISO is
larger than 2 GiB, the GitHub Releases limit. The ISO version is the date of the last commit, so
rebuilding the same commit gives the same version.

## Package snapshot

Arch packages come from the **Arch Linux Archive snapshot of 2026-10-03** (`pacman.conf`).
Move the date forward only after testing, and record it here.

| Snapshot | Kernel | Recorded |
|----------|--------|----------|
| 2026-10-03 | 7.2.8-arch1-2 | 2026-10-04 |

## Measurements

| Date | ISO size | Boot to session (VM) | RAM without UI (VM) | Notes |
|------|----------|----------------------|---------------------|-------|
| 2026-10-04 | 471 MiB | — | — | base system only (step 1.1) |
| 2026-10-05 | 1664 MiB | ~13 s (systemd: 9.1 s) | ~235 MiB | hardware support set (step 1.5); NVIDIA user space alone is ~950 MiB installed |
| 2026-10-06 | 1664 MiB | — | ~1.3 GB used on the owner's desktop (RTX 4060), excluding the 1.5 GB image copied to RAM | real hardware, cage + placeholder on the NVIDIA GPU; before `copytoram=n` |

## Choices

| Choice | Why |
|--------|-----|
| **UEFI only**, booted by systemd-boot | PROJECT_RULES.md section 4; target hardware (last ~10 years) is UEFI. |
| No cloud-init, VM guest tools (VMware, VirtualBox, Hyper-V, QEMU agent) or SSH server | Not needed on a console; less weight, no remote login surface. |
| `/etc/os-release` from `airootfs/` | OwneetOS identity. `pacman.conf` sets `NoExtract = etc/os-release` so Arch's symlink does not replace it; archiso appends `IMAGE_ID` and `IMAGE_VERSION`. |
| `copytoram=n` | The live system reads from the USB stick on demand. By default archiso copies the whole image (~1.6 GB) to RAM first on PCs with plenty of RAM, which made the boot very slow from ordinary USB sticks. The stick must stay plugged in (it must for installing anyway). |
| Silent boot: `quiet splash loglevel=3 rd.udev.log_level=3 systemd.show_status=false rd.systemd.show_status=false vt.global_cursor_default=0` | Only the OwneetOS splash is shown between firmware and console session; no text, no cursor. |
| `plymouth.ignore-serial-consoles` | Without it Plymouth falls back to its text splash because of the serial console below. |
| Boot menu hidden (`timeout 0`) | Tapping Space repeatedly right after choosing the USB stick shows the systemd-boot menu. The menu editor (`e`) always uses the **US keyboard layout** (firmware limitation). |
| Boot entry **"OwneetOS (diagnostics)"** | Starts the cage session and writes a report (GPUs, NVIDIA choice, drivers, Vulkan, session, RAM, failed units, errors) on **tty9**: press Ctrl+Alt+F9, take a photo, Ctrl+Alt+F1 to go back. Leaves a root shell on tty9: acceptable on the live medium (physical access only); not for installed systems as is. |
| Initramfs hooks `base udev microcode plymouth modconf archiso block filesystems` | CPU microcode early; the splash runs on the firmware framebuffer. No `kms` hook: it would load nouveau before `owneet-gpu-select` can choose the NVIDIA driver. |
| `console=ttyS0,115200 console=tty0` | Kernel and login prompt also on the serial port, so the test VM can check the boot automatically. The screen stays the main console. |
| root locked | No password login for root. The system runs as the console user `owneet` (autologin on tty1, from `owneet-session`). |
| `vulkan-swrast` (software Vulkan) | Lets `vulkaninfo` and tests run anywhere. It never selects gamescope: only a hardware Vulkan device does. |

## Layout

| Path | Content |
|------|---------|
| `profiledef.sh` | ISO name, label, boot modes, image compression |
| `packages.x86_64` | packages installed in the live system |
| `pacman.conf` | repositories used for the build (pinned snapshot) |
| `efiboot/` | systemd-boot menu and boot entry |
| `airootfs/` | files copied into the live system |
