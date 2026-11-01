// SPDX-License-Identifier: GPL-3.0-or-later
#include "Storage.h"

#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QFileSystemWatcher>
#include <QHash>
#include <QSocketNotifier>
#include <QStorageInfo>
#include <QVariantMap>


namespace {
QStringList pretend_disks()
{
    return qEnvironmentVariable("OWNEET_DISKS").split(QChar(':'), Qt::SkipEmptyParts);
}

// Why owneet-mount mounted a disk read only, by its folder (/run/owneet-mount/<kernel name>: the
// folder, rw or ro, the reason)
QHash<QString, QString> mount_reasons()
{
    QHash<QString, QString> out;
    const QDir dir(QStringLiteral("/run/owneet-mount"));
    for (const QString& name : dir.entryList(QDir::Files)) {
        QFile file(dir.filePath(name));
        if (!file.open(QIODevice::ReadOnly | QIODevice::Text))
            continue;
        const QStringList lines = QString::fromUtf8(file.readAll()).split(QChar('\n'));
        if (lines.size() >= 3 && lines.at(1) == QLatin1String("ro"))
            out.insert(lines.at(0), lines.at(2));
    }
    return out;
}
} // namespace


namespace owneet {

Storage::Storage(QObject* parent)
    : QObject(parent)
    , m_settle(new QTimer(this))
{
    // A mount or unmount wakes whoever polls the mount table for "priority" data; a disk often
    // brings several at once (one per partition): they are told as one, a moment later
    m_settle->setSingleShot(true);
    m_settle->setInterval(700);
    connect(m_settle, &QTimer::timeout, this, &Storage::volumesChanged);
    auto* mounts = new QFile(QStringLiteral("/proc/self/mountinfo"), this);
    if (mounts->open(QIODevice::ReadOnly)) {
        auto* notifier = new QSocketNotifier(mounts->handle(), QSocketNotifier::Exception, this);
        connect(notifier, &QSocketNotifier::activated, m_settle, qOverload<>(&QTimer::start));
    }
    const QStringList pretend = pretend_disks();
    if (!pretend.isEmpty()) {
        auto* watcher = new QFileSystemWatcher(this);
        for (const QString& path : pretend)
            watcher->addPath(QFileInfo(QDir::cleanPath(path)).absolutePath());
        connect(watcher, &QFileSystemWatcher::directoryChanged, m_settle, qOverload<>(&QTimer::start));
    }
}

QVariantList Storage::volumes() const
{
    QVariantList out;
    const QHash<QString, QString> reasons = mount_reasons();
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
        volume.insert(QStringLiteral("why"), info.isReadOnly() ? reasons.value(path) : QString());
        volume.insert(QStringLiteral("ntfs"), info.fileSystemType().startsWith("ntfs"));
        if (system)
            out.prepend(volume);
        else
            out.append(volume);
    }
    for (const QString& path : pretend_disks()) {
        const QFileInfo finfo(QDir::cleanPath(path));
        if (!finfo.isDir())
            continue;
        const QStorageInfo info(finfo.filePath());
        QVariantMap volume;
        volume.insert(QStringLiteral("key"), finfo.filePath());
        volume.insert(QStringLiteral("name"), finfo.fileName());
        volume.insert(QStringLiteral("free"), static_cast<double>(info.bytesAvailable()));
        volume.insert(QStringLiteral("total"), static_cast<double>(info.bytesTotal()));
        volume.insert(QStringLiteral("readOnly"), true);
        volume.insert(QStringLiteral("why"), QString());
        volume.insert(QStringLiteral("ntfs"), false);
        out.append(volume);
    }
    return out;
}

} // namespace owneet
