// SPDX-License-Identifier: GPL-3.0-or-later
// The disks the console can store games on, with their free space (roadmap 3.7, the library's
// disks). Automatic mounting and the notices about Windows disks come with step 6.5.

#pragma once

#include <QObject>
#include <QVariantList>


namespace owneet {
class Storage : public QObject {
    Q_OBJECT

public:
    explicit Storage(QObject* parent = nullptr);

    /// The system disk and the mounted disks (/run/media, /media, /mnt): for each one
    /// { key, name, free, total, readOnly }, sizes in bytes; key is "internal" for the system disk.
    Q_INVOKABLE QVariantList volumes() const;
};
} // namespace owneet
