// SPDX-License-Identifier: GPL-3.0-or-later
#include "CoverImages.h"

#include "Log.h"

#include <QCollator>
#include <QDir>
#include <QDirIterator>
#include <QFile>
#include <QFileInfo>
#include <QImage>
#include <QImageReader>
#include <QImageWriter>
#include <QStorageInfo>
#include <QThread>
#include <QUrl>
#include <QVariantMap>

#include <algorithm>
#include <cstdio>
#include <fcntl.h>
#include <unistd.h>


namespace {
// PNG and JPEG (owner, 2026-11-17: WebP left out, it needs a plugin the console does not have)
const QStringList& formats()
{
    static const QStringList ext { QStringLiteral("png"), QStringLiteral("jpg"), QStringLiteral("jpeg") };
    return ext;
}

// A cover's longest side at most (owner, 2026-11-17): a bigger picture is saved smaller, so a
// photo of many megapixels never weighs on the library's memory
constexpr int COVER_MAX_SIDE = 1200;

// Writes the cover to `to`: the picture itself when it is small enough and needs no turning, else
// read at the size it is saved at (a big JPEG is never decoded whole) and turned as its camera says
bool writeCover(const QString& picture, const QString& ext, const QString& to)
{
    QImageReader reader(picture);
    reader.setAutoTransform(true);
    const QSize size = reader.size();
    if (!size.isValid())
        return false;
    const int side = std::max(size.width(), size.height());
    const bool turned = reader.transformation() != QImageIOHandler::TransformationNone;
    if (side <= COVER_MAX_SIDE && !turned)
        return QFile::copy(picture, to);
    if (side > COVER_MAX_SIDE)
        reader.setScaledSize(size.scaled(COVER_MAX_SIDE, COVER_MAX_SIDE, Qt::KeepAspectRatio));
    const QImage image = reader.read();
    if (image.isNull())
        return false;
    QImageWriter writer(to, ext == QLatin1String("png") ? QByteArray("png") : QByteArray("jpeg"));
    writer.setQuality(90);
    if (!writer.write(image))
        return false;
    Log::info(LOGMSG("Cover images: `%1` saved at %2 x %3 (it was %4 x %5)")
        .arg(QFileInfo(picture).fileName()).arg(image.width()).arg(image.height()).arg(size.width()).arg(size.height()));
    return true;
}

// Every disk but the console's own: the mounted disks (as Storage), and the folders OWNEET_DISKS
// names (previews), with their names
QList<QPair<QString, QString>> otherDisks()
{
    QList<QPair<QString, QString>> out;
    for (const QStorageInfo& info : QStorageInfo::mountedVolumes()) {
        const QString path = info.rootPath();
        if (info.isValid() && info.isReady() && (path.startsWith(QLatin1String("/run/media/"))
                || path.startsWith(QLatin1String("/media/")) || path.startsWith(QLatin1String("/mnt/"))))
            out.append({ path, info.displayName() });
    }
    for (const QString& path : qEnvironmentVariable("OWNEET_DISKS").split(QChar(':'), Qt::SkipEmptyParts)) {
        const QFileInfo info(QDir::cleanPath(path));
        if (info.isDir())
            out.append({ info.filePath(), info.fileName() });
    }
    return out;
}

// The pictures of every disk but the console's own, sorted (see CoverImages::find)
QVariantList readPictures()
{
    struct Picture { QString disk, folder, name, path; };
    QList<Picture> found;
    for (const auto& disk : otherDisks()) {
        for (const QFileInfo& top : QDir(disk.first).entryInfoList(QDir::Dirs | QDir::NoDotAndDotDot)) {
            if (!owneet::CoverImages::folderNames().contains(top.fileName().toLower()) || top.isSymLink())
                continue;
            const QDir base(top.filePath());
            // its sub-folders too (owner, 2026-11-17); never a link out of it; no hidden files
            QDirIterator it(top.filePath(), QDir::Files | QDir::NoDotAndDotDot, QDirIterator::Subdirectories);
            while (it.hasNext()) {
                it.next();
                const QFileInfo info = it.fileInfo();
                if (info.isSymLink() || !formats().contains(info.suffix().toLower()))
                    continue;
                const QString rel = base.relativeFilePath(info.absolutePath());
                found.append({ disk.second, rel == QLatin1String(".") ? QString() : rel, info.fileName(), info.filePath() });
            }
        }
    }
    QCollator collator;
    collator.setNumericMode(true);
    collator.setCaseSensitivity(Qt::CaseInsensitive);
    std::sort(found.begin(), found.end(), [&collator](const Picture& a, const Picture& b) {
        if (a.disk != b.disk)
            return collator.compare(a.disk, b.disk) < 0;
        const QString pa = a.folder.isEmpty() ? a.name : a.folder + QChar('/') + a.name;
        const QString pb = b.folder.isEmpty() ? b.name : b.folder + QChar('/') + b.name;
        return collator.compare(pa, pb) < 0;
    });
    QVariantList out;
    for (const Picture& p : found)
        out.append(QVariantMap { { QStringLiteral("path"), p.path }, { QStringLiteral("url"), QUrl::fromLocalFile(p.path).toString() },
                                 { QStringLiteral("name"), p.name },
                                 { QStringLiteral("folder"), p.folder }, { QStringLiteral("disk"), p.disk } });
    return out;
}

void syncDir(const QString& dir)
{
    const int fd = ::open(QFile::encodeName(dir).constData(), O_RDONLY | O_DIRECTORY | O_CLOEXEC);
    if (fd >= 0) {
        ::syncfs(fd);
        ::close(fd);
    }
}

// The copy (see CoverImages::use): the new cover is written apart first, so a failed copy keeps
// the cover the game had
QString copyCover(const QString& picture, const QString& game_dir)
{
    const QFileInfo from(picture);
    const QString ext = from.suffix().toLower();
    if (!from.isFile() || !formats().contains(ext) || !QImageReader(picture).canRead()) {
        Log::warning(LOGMSG("Cover images: `%1` is not a picture the console can use").arg(picture));
        return {};
    }
    if (!QFileInfo(game_dir).isDir() || QStorageInfo(game_dir).isReadOnly()) {
        Log::warning(LOGMSG("Cover images: `%1` cannot be written").arg(game_dir));
        return {};
    }
    const QDir dir(game_dir);
    const QString part = dir.filePath(QStringLiteral(".cover.owneet-part"));
    const QString cover = dir.filePath(QStringLiteral("cover.") + ext);
    QFile::remove(part);
    if (!writeCover(picture, ext, part)) {
        Log::warning(LOGMSG("Cover images: cannot copy `%1` into `%2`").arg(picture, game_dir));
        QFile::remove(part);
        return {};
    }
    // an earlier cover in any of the library's formats (any case of letters), but the one about to
    // be replaced
    static const QStringList covers { QStringLiteral("cover.jpg"), QStringLiteral("cover.png"), QStringLiteral("cover.jpeg"),
                                      QStringLiteral("cover.webp") };
    for (const QString& name : dir.entryList(QDir::Files | QDir::Hidden))
        if (covers.contains(name.toLower()) && name != QStringLiteral("cover.") + ext)
            QFile::remove(dir.filePath(name));
    if (std::rename(QFile::encodeName(part).constData(), QFile::encodeName(cover).constData()) != 0) {
        Log::warning(LOGMSG("Cover images: cannot write `%1`").arg(cover));
        QFile::remove(part);
        return {};
    }
    syncDir(game_dir);
    Log::info(LOGMSG("Cover images: `%1` is the cover of `%2` now").arg(from.fileName(), game_dir));
    return cover;
}
} // namespace


namespace owneet {

CoverImages::CoverImages(QObject* parent)
    : QObject(parent)
{}

const QStringList& CoverImages::folderNames()
{
    static const QStringList names {
        QStringLiteral("images"),
        QStringLiteral("immagini"),
        QStringLiteral("bilder"),
        QStringLiteral("imágenes"),
    };
    return names;
}

int CoverImages::find()
{
    const int token = ++m_next;
    QThread* thread = QThread::create([this, token] {
        const QVariantList pictures = readPictures();
        QMetaObject::invokeMethod(this, [this, token, pictures] {
            // the latest reading tells whether pictures are there (an older one may end later)
            if (token >= m_latest) {
                m_latest = token;
                const bool now = !pictures.isEmpty();
                if (now != m_available) {
                    m_available = now;
                    emit availableChanged();
                }
            }
            emit found(token, pictures);
        }, Qt::QueuedConnection);
    });
    connect(thread, &QThread::finished, thread, &QObject::deleteLater);
    thread->start(QThread::LowPriority);
    return token;
}

int CoverImages::use(const QString& picture, const QString& game_dir)
{
    const int token = ++m_next;
    QThread* thread = QThread::create([this, token, picture, game_dir] {
        const QString cover = copyCover(QDir::cleanPath(picture), QDir::cleanPath(game_dir));
        QMetaObject::invokeMethod(this, [this, token, cover] { emit used(token, !cover.isEmpty(), cover); }, Qt::QueuedConnection);
    });
    connect(thread, &QThread::finished, thread, &QObject::deleteLater);
    thread->start(QThread::LowPriority);
    return token;
}

} // namespace owneet
