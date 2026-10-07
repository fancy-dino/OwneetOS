# vm/

**Builder and test virtual machines** (QEMU/KVM, UEFI without Secure Boot).

- `builder`: Arch Linux VM where ISOs and packages are built (step 0.7).
- `test`: boots a built ISO with blank virtual disks and controller passthrough (step 0.8).

Scripts and configuration are committed. **Disk images, firmware variable stores and run-time
files are never committed** (`vm/images/`, `vm/run/`; see `.gitignore`), and they always stay
inside this folder.

## Host requirements

| What | Debian / Ubuntu / Mint | Arch |
|------|------------------------|------|
| QEMU | `qemu-system-x86`, `qemu-utils` | `qemu-base` |
| UEFI firmware | `ovmf` | `edk2-ovmf` |
| VM windows and 3D acceleration | `qemu-system-gui` | `qemu-ui-gtk`, `qemu-hw-display-virtio-gpu-gl` |
| KVM | `/dev/kvm` readable and writable by your user | same |

On the owner's machine these were installed with
`sudo apt install --no-install-recommends qemu-system-x86 ovmf qemu-utils`
(`qemu-system-gui` was missed at first: without it only `--headless` works).

VirtualBox and KVM cannot run VMs at the same time on recent kernels: close VirtualBox VMs first
(the script checks this).

## `vm.sh`

```text
vm/vm.sh create  NAME [SIZE]      blank VM, default 32G disk (thin-provisioned)
vm/vm.sh start   NAME [options]   --iso FILE, --headless, --mem MB, --cpus N, --no-net,
                                  --ssh-port N, --seed-url URL, --evdev PATH, --gl
vm/vm.sh stop    NAME
vm/vm.sh destroy NAME             stops the VM and deletes all of its files
vm/vm.sh add-disk NAME SIZE       extra blank disk (data-N.qcow2)
vm/vm.sh snapshot NAME TAG        save the state of every disk (VM stopped)
vm/vm.sh revert   NAME TAG        go back to a saved state
vm/vm.sh snapshots NAME
vm/vm.sh status  NAME
vm/vm.sh list
vm/vm.sh selftest                 boots a throwaway UEFI VM, checks it, deletes it
vm/vm.sh reset-firmware NAME      fresh UEFI variables if a VM no longer finds its disk
```

Defaults follow the minimum hardware in PROJECT_RULES.md section 3: 4 GB RAM, 32 GB disk.

### Files per VM

| Path | Content |
|------|---------|
| `vm/images/NAME/disk.qcow2` | virtual disk |
| `vm/images/NAME/OVMF_VARS.fd` | the VM's own UEFI variable store (boot entries) |
| `vm/run/NAME/serial.log` | serial console output (firmware, kernel, systemd) |
| `vm/run/NAME/qemu.log` | QEMU errors (headless mode) |
| `vm/run/NAME/qmp.sock`, `qemu.pid` | control socket and process id while running |

## Builder VM (`builder.sh`)

An Arch Linux VM where ISOs and packages are built. archiso needs root; it only ever gets it
inside this VM.

```text
vm/builder.sh setup      download + verify the Arch image, create and provision the VM (once)
vm/builder.sh start      start in the background, wait for SSH
vm/builder.sh stop       clean shutdown
vm/builder.sh status
vm/builder.sh ssh [CMD]  shell in the VM, or run CMD
vm/builder.sh destroy    delete the VM (downloaded image and keys are kept)
```

- **Base image:** official Arch Linux cloud image, version pinned in `builder.sh`. Verified by
  SHA-256 and by the signature of the `arch-boxes <arch-boxes@archlinux.org>` key
  (`1B9A 1698 4A4E 8CB4 4871  2D2A E0B7 8BF4 326C 6F8F`), using a project-local keyring in
  `vm/images/.gnupg/`. The image is never modified: the VM disk is a copy-on-write layer over it.
- **First boot:** configured by cloud-init ([`builder/user-data.in`](builder/user-data.in)),
  served once by a temporary HTTP server on `127.0.0.1`. It creates the `builder` user
  (passwordless sudo, SSH key only) and installs `archiso`, `base-devel`, `devtools`, `git`, `rsync`, `shellcheck`, `nodejs`, `npm`, `go`.
- **Access:** SSH on `127.0.0.1:2222` with a project key in `vm/images/.keys/`; `~/.ssh` is not used.
  The VM is not reachable from the local network.
- **Resources:** 8 GB RAM, 8 CPUs, 80 GB thin-provisioned disk.

### Running builds: `tools/build-in-vm`

```text
tools/build-in-vm COMMAND [ARGS...]
tools/build-in-vm 'shell command line'
```

Copies the repository into the VM (`~/owneet`, with rsync; VM images and build output excluded),
runs the command there, and copies the VM's `out/` folder back to the project's `out/` folder.
Builds run on the VM's own disk: archiso does not work reliably on shared folders.

## Test VM (`test.sh`)

Boots an ISO the same way every time: UEFI, Secure Boot off, 4 GB RAM, 4 CPUs (the minimum
hardware), blank virtual disks.

```text
vm/test.sh fetch-arch-iso        stock Arch ISO, verified (used until OwneetOS has its own ISO)
vm/test.sh create [single|multi] single: one 32G disk; multi: 32G + 64G data disk
vm/test.sh boot [ISO] [options]  --headless, --gamepad auto|none|PATH, --gl, --ssh, --kargs, --journal, --debug-shell
vm/test.sh run 'CMD'             run a command in the debug shell and print its output
vm/test.sh keys COMBO            press keys in the VM, e.g. ctrl-alt-f9
vm/test.sh gamepads              gamepads connected to this computer
vm/test.sh ssh [CMD]             root shell in the live system (after boot --ssh)
vm/test.sh stop | reset | destroy
```

- **Default ISO:** the newest `out/*.iso`, otherwise the stock Arch ISO.
- **Gamepads:** passed to the VM as the same input device (evdev passthrough,
  `virtio-input-host`), so USB and Bluetooth controllers both work and the VM sees real buttons and
  sticks. No host permission change is needed: the desktop session already gives the logged-in user
  access to connected gamepads. While the VM runs it grabs the gamepad and the host stops seeing it.
- **Blank state:** `create` saves a snapshot called `blank`; `reset` brings every disk back to it,
  so each test starts from empty disks.
- **3D:** `--gl` uses a virgl-accelerated GPU (needs `qemu-system-gui`). With QEMU 8.2 (Mint 22)
  this gives the guest accelerated **OpenGL only, not Vulkan** (Vulkan in a VM, "Venus", needs a
  newer QEMU). gamescope in the VM therefore runs on software Vulkan (lavapipe): fine for functional
  tests, not for performance. Performance is measured on real hardware (roadmap step 7.4).
- **Screenshots:** `vm/test.sh screenshot` saves the VM screen as PNG; not available with `--gl`
  in headless mode (QEMU keeps no copy of a 3D screen).
- **Extra kernel arguments:** `--kargs "..."` is passed through SMBIOS and appended by systemd-boot
  (e.g. `owneet.session=cage`); `--journal` sends the system journal to the serial log.
- **Debug shell:** `--debug-shell` starts a root shell on a virtio console (`hvc0`, kernel argument
  `systemd.debug_shell=hvc0`, with the login prompt on `hvc0` masked); `vm/test.sh run 'CMD'` runs a command there and prints its output.
  The virtio console has flow control, unlike the emulated serial port (which lost or duplicated
  characters). Test VMs only: nothing in the ISO enables it. Output also lands in
  `vm/run/NAME/console.log`; the serial port keeps kernel and journal messages (`serial.log`).
- **`--ssh`:** for the live Arch ISO only. A cloud-init seed puts the project test key
  (`vm/images/.keys/test_ed25519`) in root's `authorized_keys`, on `127.0.0.1:2223`.
- **Simulated Wi-Fi** (step 2.6): the kernel module `mac80211_hwsim` creates virtual Wi-Fi radios
  that see each other, and NetworkManager makes one of them a hotspot. NetworkManager's hotspot
  needs `dnsmasq`, which is not in the ISO: it is installed in the live system only (lost at
  power-off), from the ISO's Arch Linux Archive snapshot. In the debug shell:

  ```text
  echo 'Server = https://archive.archlinux.org/repos/2026/10/03/$repo/os/$arch' > /etc/pacman.d/mirrorlist
  pacman-key --init; pacman-key --populate archlinux; pacman -Sy --noconfirm dnsmasq
  modprobe mac80211_hwsim radios=2
  nmcli dev wifi hotspot ifname wlan1 ssid OwneetTest password testpass123
  ```

  owneetd uses `wlan0` (the first Wi-Fi adapter) and sees `OwneetTest`. For a WPA3-only network:
  `nmcli con modify Hotspot 802-11-wireless-security.key-mgmt sae && nmcli con up Hotspot`.
- **Not yet:** a "Windows-like" NTFS disk; it is created when automatic mounting is built (step 6.5).
- **Changing virtual hardware:** VMs keep firmware boot entries that point at PCI addresses. New
  devices get fixed PCI slots (the virtio console uses `0x10`) so existing VMs keep booting. If a VM
  still tries to boot from the network or stops in the UEFI shell, run `vm/vm.sh reset-firmware NAME`.
