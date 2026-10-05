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

## Choices

| Choice | Why |
|--------|-----|
| **UEFI only**, booted by systemd-boot | PROJECT_RULES.md section 4; target hardware (last ~10 years) is UEFI. |
| No cloud-init, VM guest tools (VMware, VirtualBox, Hyper-V, QEMU agent) or SSH server | Not needed on a console; less weight, no remote login surface. |
| `/etc/os-release` from `airootfs/` | OwneetOS identity. `pacman.conf` sets `NoExtract = etc/os-release` so Arch's symlink does not replace it; archiso appends `IMAGE_ID` and `IMAGE_VERSION`. |
| Silent boot: `quiet splash loglevel=3 rd.udev.log_level=3 systemd.show_status=false rd.systemd.show_status=false vt.global_cursor_default=0` | Only the OwneetOS splash is shown between firmware and console session; no text, no cursor. |
| `plymouth.ignore-serial-consoles` | Without it Plymouth falls back to its text splash because of the serial console below. |
| Boot menu hidden (`timeout 0`) | Holding a key (e.g. Space) while the PC starts shows the systemd-boot menu. |
| `kms` + `plymouth` in the initramfs hooks | Graphics drivers and the splash start as early as possible. |
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
