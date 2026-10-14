# installer/

The **gamepad-driven installer**:

- a QML interface in the OwneetOS style;
- a backend that partitions the disk, creates the btrfs subvolumes, sets up snapshots and the
  bootloader, and configures the first boot.

Wave 1 offers "Use the whole disk". Dual boot is considered in the design but is not required
([[installer-and-firmware]]). Tested only on virtual disks.

Plan: [[phase-6-first-boot-installer-disks]].
