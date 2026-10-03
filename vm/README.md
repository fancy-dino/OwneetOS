# vm/

**Builder and test virtual machines** (QEMU/KVM, UEFI).

- `builder`: Arch Linux VM where ISOs and packages are built.
- `test`: boots a built ISO with blank virtual disks and controller passthrough.

Scripts and configuration are committed. **Disk images, firmware variable stores and run-time
files are never committed** (`vm/images/`, `vm/run/`; see `.gitignore`), and they always stay
inside this folder.

Roadmap: steps 0.6, 0.7, 0.8.
