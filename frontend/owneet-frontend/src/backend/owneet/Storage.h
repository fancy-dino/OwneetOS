// SPDX-License-Identifier: GPL-3.0-or-later
// The disks the console can store games on, with their free space ([[step-3.7-library-screen]], the
// library's disks). The disks are mounted by owneet-mount (owneet-base,
// [[step-6.5-automatic-disk-mounting]]), which says why one is read only.

#pragma once

#include <QObject>
#include <QTimer>
#include <QVariantList>


namespace owneet {
class Storage : public QObject {
    Q_OBJECT

public:
    explicit Storage(QObject* parent = nullptr);

    /// The system disk and the mounted disks (/run/media, /media, /mnt): for each one
    /// { key, name, free, total, readOnly, why, ntfs }, sizes in bytes; key is "internal" for the
    /// system disk, the mount folder for the others; why: owneet-mount's reason when read only
    /// ("windows": left dirty or hibernated by Windows, "windows-system": Windows's own partition,
    /// "live": an internal disk from the USB stick, "error"); ntfs: an NTFS file system. OWNEET_DISKS ("path[:path…]") adds folders that act as
    /// disks while they exist (previews: plugging a disk is creating its folder).
    Q_INVOKABLE QVariantList volumes() const;

signals:
    /// A disk was mounted or unmounted (e.g. a USB disk plugged in or out)
    void volumesChanged();

private:
    QTimer* m_settle;
};
} // namespace owneet
