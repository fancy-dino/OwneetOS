// SPDX-License-Identifier: GPL-3.0-or-later
#include "Storage.h"

#include <QStorageInfo>
#include <QVariantMap>


namespace owneet {

Storage::Storage(QObject* parent)
    : QObject(parent)
{}

QVariantList Storage::volumes() const
{
    QVariantList out;
    for (const QStorageInfo& info : QStorageInfo::mountedVolumes()) {
        if (!info.isValid() || !info.isReady() || info.bytesTotal() <= 0)
            continue;
        const QString path = info.rootPath();
        const bool system = path == QLatin1String("/");
        if (!system && !path.startsWith(QLatin1String("/run/media/")) && !path.startsWith(QLatin1String("/media/"))
                && !path.startsWith(QLatin1String("/mnt/")))
            continue;

        QVariantMap volume;
        volume.insert(QStringLiteral("key"), system ? QStringLiteral("internal") : path);
        volume.insert(QStringLiteral("name"), system ? QString() : info.displayName());
        volume.insert(QStringLiteral("free"), static_cast<double>(info.bytesAvailable()));
        volume.insert(QStringLiteral("total"), static_cast<double>(info.bytesTotal()));
        volume.insert(QStringLiteral("readOnly"), info.isReadOnly());
        if (system)
            out.prepend(volume);
        else
            out.append(volume);
    }
    return out;
}

} // namespace owneet
