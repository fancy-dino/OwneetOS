// SPDX-License-Identifier: GPL-3.0-or-later
#include "FileManager.h"

#include "DiskGames.h"
#include "Paths.h"

#include <QDateTime>
#include <QDir>
#include <QDirIterator>
#include <QFile>
#include <QFileInfo>
#include <QRegularExpression>
#include <QSet>
#include <QStorageInfo>
#include <QThread>
#include <QVariantMap>

#include <algorithm>
#include <fcntl.h>
#include <unistd.h>


namespace {
QString windowsDir(const QString& kind)
{
    return paths::homePath() + QStringLiteral("/.local/share/owneet/windows/") + kind;
}

// Steam's folder (OWNEET_STEAM_DIR in previews, as the Steam provider)
QString steamDir()
{
    const QString env = qEnvironmentVariable("OWNEET_STEAM_DIR");
    return QDir::cleanPath(env.isEmpty() ? paths::homePath() + QStringLiteral("/.local/share/Steam") : env);
}

// The disks games may be on: the home folder, then every mounted disk (as Storage), and the
// folders OWNEET_DISKS names (previews)
QList<QPair<QString, QString>> diskRoots()
{
    QList<QPair<QString, QString>> out { { paths::homePath(), QStringLiteral("internal") } };
    for (const QStorageInfo& info : QStorageInfo::mountedVolumes()) {
        const QString path = info.rootPath();
        if (info.isValid() && info.isReady() && (path.startsWith(QLatin1String("/run/media/"))
                || path.startsWith(QLatin1String("/media/")) || path.startsWith(QLatin1String("/mnt/"))))
            out.append({ path, path });
    }
    for (const QString& path : qEnvironmentVariable("OWNEET_DISKS").split(QChar(':'), Qt::SkipEmptyParts)) {
        const QString clean = QDir::cleanPath(path);
        if (QFileInfo(clean).isDir())
            out.append({ clean, clean });
    }
    return out;
}

// Steam's libraries (its own folder and libraryfolders.vdf's "path" entries): their steamapps folders
QStringList steamLibraries()
{
    QStringList libs { steamDir() + QStringLiteral("/steamapps") };
    QFile vdf(libs.first() + QStringLiteral("/libraryfolders.vdf"));
    if (vdf.open(QIODevice::ReadOnly)) {
        static const QRegularExpression path(QStringLiteral(R""("path"\s+"([^"]+)")""));
        auto it = path.globalMatch(QString::fromUtf8(vdf.readAll()));
        while (it.hasNext())
            libs.append(QDir::cleanPath(it.next().captured(1) + QStringLiteral("/steamapps")));
    }
    libs.removeDuplicates();
    libs.erase(std::remove_if(libs.begin(), libs.end(), [](const QString& p) { return !QFileInfo(p).isDir(); }), libs.end());
    return libs;
}

// The app ids Steam has installed (an appmanifest in any library)
QSet<QString> steamInstalled(const QStringList& libs)
{
    QSet<QString> ids;
    static const QRegularExpression manifest(QStringLiteral("^appmanifest_([0-9]+)\\.acf$"));
    for (const QString& lib : libs)
        for (const QString& name : QDir(lib).entryList({ QStringLiteral("appmanifest_*.acf") }, QDir::Files)) {
            const auto m = manifest.match(name);
            if (m.hasMatch())
                ids.insert(m.captured(1));
        }
    return ids;
}

const QStringList& steamKinds()
{
    static const QStringList kinds { QStringLiteral("shadercache"), QStringLiteral("compatdata"), QStringLiteral("downloading") };
    return kinds;
}

// The bytes a file or a folder holds (links are not followed)
qint64 sizeOf(const QString& path)
{
    const QFileInfo info(path);
    if (info.isSymLink() || !info.isDir())
        return info.isSymLink() ? 0 : info.size();
    qint64 bytes = 0;
    QDirIterator it(path, QDir::Files | QDir::Hidden | QDir::System | QDir::NoSymLinks, QDirIterator::Subdirectories);
    while (it.hasNext()) {
        it.next();
        bytes += it.fileInfo().size();
    }
    return bytes;
}

QVariantMap entry(const QFileInfo& info)
{
    return QVariantMap {
        { QStringLiteral("name"), info.fileName() },
        { QStringLiteral("path"), info.absoluteFilePath() },
        { QStringLiteral("folder"), info.isDir() && !info.isSymLink() },
        { QStringLiteral("size"), static_cast<double>(sizeOf(info.absoluteFilePath())) },
        { QStringLiteral("modified"), static_cast<double>(info.lastModified().toMSecsSinceEpoch()) },
    };
}

// Steam's leftovers: the shader caches, Windows data and unfinished downloads of games no longer
// installed (owner, 2026-11-13)
QVariantList steamLeftovers()
{
    QVariantList out;
    const QStringList libs = steamLibraries();
    const QSet<QString> installed = steamInstalled(libs);
    static const QRegularExpression appid(QStringLiteral("^[0-9]+$"));
    for (const QString& lib : libs)
        for (const QString& kind : steamKinds()) {
            const QDir dir(lib + QChar('/') + kind);
            for (const QFileInfo& info : dir.entryInfoList(QDir::Dirs | QDir::NoDotAndDotDot | QDir::NoSymLinks)) {
                if (!appid.match(info.fileName()).hasMatch() || installed.contains(info.fileName()) || info.fileName() == QLatin1String("0"))
                    continue;
                QVariantMap e = entry(info);
                e.insert(QStringLiteral("appid"), info.fileName());
                e.insert(QStringLiteral("steam"), kind == QLatin1String("shadercache") ? QStringLiteral("shaders")
                                                 : kind == QLatin1String("compatdata") ? QStringLiteral("data") : QStringLiteral("download"));
                out.append(e);
            }
        }
    return out;
}

bool inside(const QString& path, const QString& root)
{
    return path.startsWith(root + QChar('/')) && path.size() > root.size() + 1;
}
} // namespace


namespace owneet {
FileManager::FileManager(QObject* parent)
    : QObject(parent)
{}

QVariantList FileManager::places() const
{
    QVariantList out;
    for (const auto& root : diskRoots()) {
        for (const QString& folder : QDir(root.first).entryList(QDir::Dirs | QDir::NoDotAndDotDot)) {
            if (!DiskGames::folderNames().contains(folder.toLower()))
                continue;
            out.append(QVariantMap {
                { QStringLiteral("kind"), QStringLiteral("games") },
                { QStringLiteral("path"), root.first + QChar('/') + folder },
                { QStringLiteral("root"), root.second },
            });
        }
    }
    for (const QString& kind : { QStringLiteral("windows"), QStringLiteral("logs") }) {
        const QString path = windowsDir(kind == QLatin1String("windows") ? QStringLiteral("prefixes") : QStringLiteral("logs"));
        if (QFileInfo(path).isDir())
            out.append(QVariantMap { { QStringLiteral("kind"), kind }, { QStringLiteral("path"), path },
                                     { QStringLiteral("root"), QStringLiteral("internal") } });
    }
    if (QFileInfo(steamDir()).isDir())
        out.append(QVariantMap { { QStringLiteral("kind"), QStringLiteral("steam") }, { QStringLiteral("path"), steamDir() },
                                 { QStringLiteral("root"), QStringLiteral("internal") } });
    return out;
}

QStringList FileManager::roots() const
{
    QStringList out;
    for (const QVariant& p : places()) {
        const QVariantMap place = p.toMap();
        if (place.value(QStringLiteral("kind")).toString() != QLatin1String("steam"))
            out.append(QDir::cleanPath(place.value(QStringLiteral("path")).toString()));
    }
    for (const QString& lib : steamLibraries())
        for (const QString& kind : steamKinds())
            out.append(QDir::cleanPath(lib + QChar('/') + kind));
    return out;
}

int FileManager::list(const QString& kind, const QString& path)
{
    const int token = ++m_next;
    const QString dir = QDir::cleanPath(path);
    QStringList allowed = roots();
    QThread* thread = QThread::create([this, token, kind, dir, allowed] {
        QVariantList entries;
        if (kind == QLatin1String("steam")) {
            entries = steamLeftovers();
        } else if (allowed.contains(dir) || std::any_of(allowed.cbegin(), allowed.cend(), [&dir](const QString& r) { return inside(dir, r); })) {
            QFileInfoList infos = QDir(dir).entryInfoList(QDir::Dirs | QDir::Files | QDir::NoDotAndDotDot | QDir::System);
            std::sort(infos.begin(), infos.end(), [](const QFileInfo& a, const QFileInfo& b) {
                const bool da = a.isDir() && !a.isSymLink(), db = b.isDir() && !b.isSymLink();
                return da != db ? da : a.fileName().compare(b.fileName(), Qt::CaseInsensitive) < 0;
            });
            for (const QFileInfo& info : infos)
                entries.append(entry(info));
        }
        QMetaObject::invokeMethod(this, [this, token, entries] { emit listed(token, entries); }, Qt::QueuedConnection);
    });
    connect(thread, &QThread::finished, thread, &QObject::deleteLater);
    thread->start(QThread::LowPriority);
    return token;
}

int FileManager::remove(const QString& path)
{
    const int token = ++m_next;
    const QString target = QDir::cleanPath(QFileInfo(path).absoluteFilePath());
    const QStringList allowed = roots();
    const bool ok_place = std::any_of(allowed.cbegin(), allowed.cend(), [&target](const QString& r) { return inside(target, r); });
    QThread* thread = QThread::create([this, token, target, ok_place] {
        const QFileInfo info(target);
        bool ok = false;
        qint64 freed = 0;
        // inside a place (never a place itself), on a disk that can be written, and there
        if (ok_place && (info.exists() || info.isSymLink()) && !QStorageInfo(info.absolutePath()).isReadOnly()) {
            freed = sizeOf(target);
            ok = info.isDir() && !info.isSymLink() ? QDir(target).removeRecursively() : QFile::remove(target);
            // the disk's free space told at once (btrfs frees at its next commit, 3o)
            const int fd = ::open(QFile::encodeName(info.absolutePath()).constData(), O_RDONLY | O_DIRECTORY | O_CLOEXEC);
            if (fd >= 0) {
                ::syncfs(fd);
                ::close(fd);
            }
        }
        QMetaObject::invokeMethod(this, [this, token, target, ok, freed] {
            emit removed(token, target, ok, static_cast<double>(freed));
        }, Qt::QueuedConnection);
    });
    connect(thread, &QThread::finished, thread, &QObject::deleteLater);
    thread->start(QThread::LowPriority);
    return token;
}

QString FileManager::windowsPrefix(const QString& game_dir) const
{
    return DiskGames::windowsPrefix(game_dir);
}
} // namespace owneet
