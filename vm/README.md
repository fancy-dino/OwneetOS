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

```
vm/vm.sh create  NAME [SIZE]      blank VM, default 32G disk (thin-provisioned)
vm/vm.sh start   NAME [options]   --iso FILE, --headless, --mem MB, --cpus N, --no-net
vm/vm.sh stop    NAME
vm/vm.sh destroy NAME             stops the VM and deletes all of its files
vm/vm.sh status  NAME
vm/vm.sh list
vm/vm.sh selftest                 boots a throwaway UEFI VM, checks it, deletes it
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

```
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
  (passwordless sudo, SSH key only) and installs `archiso`, `base-devel`, `devtools`, `git`, `rsync`.
- **Access:** SSH on `127.0.0.1:2222` with a project key in `vm/images/.keys/`; `~/.ssh` is not used.
  The VM is not reachable from the local network.
- **Resources:** 8 GB RAM, 8 CPUs, 80 GB thin-provisioned disk.

### Running builds: `tools/build-in-vm`

```
tools/build-in-vm COMMAND [ARGS...]
tools/build-in-vm 'shell command line'
```

Copies the repository into the VM (`~/owneet`, with rsync; VM images and build output excluded),
runs the command there, and copies the VM's `out/` folder back to the project's `out/` folder.
Builds run on the VM's own disk: archiso does not work reliably on shared folders.
