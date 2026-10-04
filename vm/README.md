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
| KVM | `/dev/kvm` readable and writable by your user | same |

On the owner's machine these were installed with
`sudo apt install --no-install-recommends qemu-system-x86 ovmf qemu-utils`.

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
