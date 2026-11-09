// SPDX-License-Identifier: GPL-3.0-or-later
#include "Media.h"

#include "Log.h"
#include "Paths.h"

#include <QCryptographicHash>
#include <QDateTime>
#include <QDir>
#include <QDirIterator>
#include <QFile>
#include <QFileInfo>
#include <QJsonArray>
#include <QJsonDocument>
#include <QLocale>
#include <QMutex>
#include <QRegularExpression>
#include <QElapsedTimer>
#include <QStorageInfo>
#include <QThread>
#include <QTimer>
#include <QUrl>
#include <QWaitCondition>

#include <algorithm>
#include <cstdio>
#include <deque>
#include <map>
#include <tuple>

extern "C" {
#include <libavformat/avformat.h>
#include <libavutil/dict.h>
#include <libavutil/log.h>
}


namespace {
constexpr int LIST_VERSION = 1;
constexpr int MAX_ENTRIES = 100000;      // files and folders looked at in one kind's folders
constexpr int CONTINUE_MAX = 5;          // videos in "Continue watching"

const QStringList VIDEO_SUFFIXES {
    "mkv", "mp4", "m4v", "avi", "mov", "webm", "wmv", "mpg", "mpeg", "ts", "m2ts", "mts", "flv", "ogv", "3gp",
};
const QStringList SONG_SUFFIXES {
    "mp3", "flac", "ogg", "oga", "opus", "m4a", "aac", "wav", "wma", "ape", "wv", "aif", "aiff", "mka",
};
const QStringList SUBTITLE_SUFFIXES { "srt", "ass", "ssa", "vtt", "sub" };
// The pictures a folder may hold, in order of preference (any case; .jpg, .jpeg, .png, .webp)
const QStringList VIDEO_PICTURES { "cover", "poster", "folder" };
const QStringList MUSIC_PICTURES { "cover", "folder", "front", "album" };
const QStringList PICTURE_SUFFIXES { "jpg", "jpeg", "png", "webp" };

QString list_path() { return paths::homePath() + QStringLiteral("/.cache/owneet/media.json"); }
QString pictures_dir() { return paths::homePath() + QStringLiteral("/.cache/owneet/media-pictures"); }

// A picture for the interface: a file's address (spaces, "#" and the like encoded)
QString picture_url(const QString& path)
{
    return path.isEmpty() ? QString() : QUrl::fromLocalFile(path).toString(QUrl::FullyEncoded);
}

// "video", "song" or "" (anything else)
QString file_kind(const QFileInfo& finfo)
{
    const QString suffix = finfo.suffix().toLower();
    if (VIDEO_SUFFIXES.contains(suffix))
        return QStringLiteral("video");
    if (SONG_SUFFIXES.contains(suffix))
        return QStringLiteral("song");
    return QString();
}
QString item_kind(const QString& kind) { return kind == QLatin1String("videos") ? QStringLiteral("video") : QStringLiteral("song"); }

// Hidden files and folders, and the "._" files macOS leaves on USB sticks
bool hidden(const QString& name) { return name.startsWith(QChar('.')); }

// Names in the order a person expects: numbers by their value ("Episode 2" before "Episode 10"),
// letters without case. Written here because QCollator's numeric mode does nothing in the C
// locale, and the console's locale is not always another.
bool before(const QString& a, const QString& b)
{
    int i = 0, j = 0;
    while (i < a.size() && j < b.size()) {
        if (a.at(i).isDigit() && b.at(j).isDigit()) {
            const int si = i, sj = j;
            while (i < a.size() && a.at(i).isDigit())
                ++i;
            while (j < b.size() && b.at(j).isDigit())
                ++j;
            QStringView x = QStringView(a).mid(si, i - si), y = QStringView(b).mid(sj, j - sj);
            while (x.size() > 1 && x.front() == QChar('0'))
                x = x.mid(1);
            while (y.size() > 1 && y.front() == QChar('0'))
                y = y.mid(1);
            if (x.size() != y.size())
                return x.size() < y.size();
            if (const int c = x.compare(y))
                return c < 0;
            continue;
        }
        const QChar x = a.at(i).toCaseFolded(), y = b.at(j).toCaseFolded();
        if (x != y)
            return x < y;
        ++i;
        ++j;
    }
    if (a.size() - i != b.size() - j)
        return a.size() - i < b.size() - j;
    return a < b;                         // the same but for case: always one order
}

// The disks to look at, as the disk-games provider: the home folder ("internal") and the mounted
// disks; OWNEET_DISKS adds folders that act as disks (previews and tests: "path[:path…]")
std::vector<std::pair<QString, QString>> disk_roots()
{
    std::vector<std::pair<QString, QString>> roots;
    roots.emplace_back(paths::homePath(), QStringLiteral("internal"));
    for (const QStorageInfo& info : QStorageInfo::mountedVolumes()) {
        if (!info.isValid() || !info.isReady())
            continue;
        const QString path = info.rootPath();
        if (path.startsWith(QLatin1String("/run/media/")) || path.startsWith(QLatin1String("/media/"))
                || path.startsWith(QLatin1String("/mnt/")))
            roots.emplace_back(path, path);
    }
    const QStringList extra = qEnvironmentVariable("OWNEET_DISKS").split(QChar(':'), Qt::SkipEmptyParts);
    for (const QString& path : extra)
        roots.emplace_back(QDir::cleanPath(path), QDir::cleanPath(path));
    return roots;
}

// A disk's name on the cards: its label, the folder's name for a pretend disk, "" for the console's
QString disk_name(const QString& disk)
{
    if (disk == QLatin1String("internal") || disk.isEmpty())
        return QString();
    const QStorageInfo info(disk);
    if (info.isValid() && info.rootPath() == disk && !info.displayName().isEmpty())
        return info.displayName();
    return QFileInfo(disk).fileName();
}

// The picture a folder holds itself, or ""
QString own_picture(const QString& dir, bool video)
{
    const QStringList names = video ? VIDEO_PICTURES : MUSIC_PICTURES;
    QStringList filters;
    for (const QString& name : names)
        for (const QString& suffix : PICTURE_SUFFIXES)
            filters << name + QChar('.') + suffix;
    const QStringList found = QDir(dir).entryList(filters, QDir::Files);   // name filters ignore case
    for (const QString& name : names)
        for (const QString& file : found)
            if (QFileInfo(file).completeBaseName().compare(name, Qt::CaseInsensitive) == 0)
                return QDir(dir).filePath(file);
    return QString();
}

// The sub-folders and media files right inside a folder
std::pair<int, int> contents_of(const QString& dir, bool video)
{
    int folders = 0, files = 0;
    const QString wanted = video ? QStringLiteral("video") : QStringLiteral("song");
    for (const QFileInfo& e : QDir(dir).entryInfoList(QDir::Dirs | QDir::Files | QDir::NoDotAndDotDot)) {
        if (hidden(e.fileName()))
            continue;
        if (e.isDir())
            ++folders;
        else if (file_kind(e) == wanted)
            ++files;
    }
    return { folders, files };
}

// A subtitle file beside a video ("Film.srt", "Film.en.srt", "Film.eng.forced.srt"): its language
QString subtitle_language(const QString& video_base, const QString& subtitle_file)
{
    const QString rest = QFileInfo(subtitle_file).completeBaseName().mid(video_base.size() + 1);
    return owneet::Media::languageCode(rest.section(QChar('.'), 0, 0));
}

// ---- Reading a file's details with FFmpeg's library (in the worker thread)
QString dict_value(const AVDictionary* dict, const char* key)
{
    const AVDictionaryEntry* entry = dict ? av_dict_get(dict, key, nullptr, 0) : nullptr;
    return entry ? QString::fromUtf8(entry->value).trimmed() : QString();
}

double seconds_of(const AVFormatContext* ctx)
{
    if (ctx->duration > 0)
        return ctx->duration / double(AV_TIME_BASE);
    double longest = 0;
    for (unsigned i = 0; i < ctx->nb_streams; ++i) {
        const AVStream* st = ctx->streams[i];
        if (st->duration > 0 && st->time_base.den > 0)
            longest = std::max(longest, st->duration * av_q2d(st->time_base));
    }
    return longest;
}

int leading_number(const QString& text)          // "3/12" → 3
{
    static const QRegularExpression number(QStringLiteral("^\\s*(\\d+)"));
    const QRegularExpressionMatch m = number.match(text);
    return m.hasMatch() ? m.captured(1).toInt() : 0;
}

// A file of the console's list or a picture: written whole, then put in its place. QSaveFile is
// not used: it waits for the disk at every file (tens of ms each in a VM), which a cache does not need.
bool write_cache(const QString& path, const QByteArray& data)
{
    const QString part = path + QStringLiteral(".part");
    QFile out(part);
    if (!out.open(QIODevice::WriteOnly) || out.write(data) != data.size()) {
        out.remove();
        return false;
    }
    out.close();
    return std::rename(QFile::encodeName(part).constData(), QFile::encodeName(path).constData()) == 0;
}

// A song's own picture, saved on the console's disk for the interface to show
QString save_picture(const QString& path, const AVPacket& pkt, bool png)
{
    const QString dir = pictures_dir();
    QDir().mkpath(dir);
    const QString file = dir + QChar('/')
        + QString::fromLatin1(QCryptographicHash::hash(path.toUtf8(), QCryptographicHash::Sha1).toHex().left(20))
        + (png ? QStringLiteral(".png") : QStringLiteral(".jpg"));
    return write_cache(file, QByteArray(reinterpret_cast<const char*>(pkt.data), pkt.size)) ? file : QString();
}

// The details kept in the list: s (size), m (modified, ms), d (seconds), au / su (audio and
// subtitle languages), t (title), a (artist), aa (album artist), al (album), n (track), dn (disc),
// y (year), p (the song's own picture). A file that cannot be read keeps only s and m.
QJsonObject probe(const QString& path)
{
    QJsonObject out;
    const QFileInfo finfo(path);
    out.insert(QStringLiteral("s"), double(finfo.size()));
    out.insert(QStringLiteral("m"), double(finfo.lastModified().toMSecsSinceEpoch()));
    AVFormatContext* ctx = nullptr;
    if (avformat_open_input(&ctx, QFile::encodeName(path).constData(), nullptr, nullptr) < 0)
        return out;
    // the length is in the file's header for most formats; the others (e.g. an MP3 without a
    // Xing header) need its streams read
    double seconds = seconds_of(ctx);
    if (seconds <= 0 && avformat_find_stream_info(ctx, nullptr) >= 0)
        seconds = seconds_of(ctx);
    QJsonArray audio, subs;
    const AVDictionary* audio_tags = nullptr;
    QString picture;
    for (unsigned i = 0; i < ctx->nb_streams; ++i) {
        const AVStream* st = ctx->streams[i];
        if (st->disposition & AV_DISPOSITION_ATTACHED_PIC) {
            if (picture.isEmpty() && st->attached_pic.size > 0)
                picture = save_picture(path, st->attached_pic, st->codecpar->codec_id == AV_CODEC_ID_PNG);
            continue;
        }
        const QString language = owneet::Media::languageCode(dict_value(st->metadata, "language"));
        if (st->codecpar->codec_type == AVMEDIA_TYPE_AUDIO) {
            audio.append(language);
            if (!audio_tags)
                audio_tags = st->metadata;
        } else if (st->codecpar->codec_type == AVMEDIA_TYPE_SUBTITLE) {
            subs.append(language);
        }
    }
    // The tags: the file's, else its audio stream's (Ogg keeps them there)
    const auto tag = [&](std::initializer_list<const char*> keys) {
        for (const char* key : keys) {
            QString value = dict_value(ctx->metadata, key);
            if (value.isEmpty())
                value = dict_value(audio_tags, key);
            if (!value.isEmpty())
                return value;
        }
        return QString();
    };
    const auto put = [&](const char* key, const QString& value) {
        if (!value.isEmpty())
            out.insert(QLatin1String(key), value);
    };
    out.insert(QStringLiteral("d"), seconds);
    out.insert(QStringLiteral("au"), audio);
    out.insert(QStringLiteral("su"), subs);
    put("t", tag({"title"}));
    put("a", tag({"artist"}));
    put("aa", tag({"album_artist", "albumartist", "album artist"}));
    put("al", tag({"album"}));
    if (const int track = leading_number(tag({"track"})))
        out.insert(QStringLiteral("n"), track);
    if (const int disc = leading_number(tag({"disc"})))
        out.insert(QStringLiteral("dn"), disc);
    static const QRegularExpression year(QStringLiteral("\\b(\\d{4})\\b"));
    const QRegularExpressionMatch y = year.match(tag({"date", "year"}));
    if (y.hasMatch())
        out.insert(QStringLiteral("y"), y.captured(1).toInt());
    put("p", picture);
    avformat_close_input(&ctx);
    return out;
}
} // namespace


namespace owneet {

// Reads the files' details one at a time; the files of the folder on screen go first
class MediaWorker : public QThread {
public:
    explicit MediaWorker(Media* media) : m_media(media) {}
    ~MediaWorker() override
    {
        {
            QMutexLocker lock(&m_mutex);
            m_stop = true;
        }
        m_wake.wakeAll();
        wait();
    }
    void add(const QStringList& paths, bool first)
    {
        QMutexLocker lock(&m_mutex);
        if (first) {
            // moved to the front, in their order
            for (auto it = paths.crbegin(); it != paths.crend(); ++it) {
                m_queue.erase(std::remove(m_queue.begin(), m_queue.end(), *it), m_queue.end());
                m_queue.push_front(*it);
            }
        } else {
            m_queue.insert(m_queue.end(), paths.cbegin(), paths.cend());
        }
        m_wake.wakeOne();
    }

protected:
    void run() override
    {
        for (;;) {
            QString path;
            {
                QMutexLocker lock(&m_mutex);
                while (m_queue.empty() && !m_stop)
                    m_wake.wait(&m_mutex);
                if (m_stop)
                    return;
                path = m_queue.front();
                m_queue.pop_front();
            }
            QElapsedTimer clock;
            clock.start();
            const QJsonObject info = probe(path);
            const qint64 us = clock.nsecsElapsed() / 1000;
            Media* media = m_media;
            QMetaObject::invokeMethod(media, [media, path, info, us] { media->probed(path, info, us); }, Qt::QueuedConnection);
        }
    }

private:
    Media* const m_media;
    QMutex m_mutex;
    QWaitCondition m_wake;
    std::deque<QString> m_queue;
    bool m_stop = false;
};

struct Media::Group {
    QString artist;                      // "" unknown
    QString album;                       // "" none (songs at the top of a Music folder)
    int year = 0;
    QString folder;                      // the first song's folder (its picture)
    std::vector<QString> songs;
};

Media::Media(QObject* parent)
    : QObject(parent)
    , m_worker(new MediaWorker(this))
    , m_announce(new QTimer(this))
    , m_save(new QTimer(this))
{
    av_log_set_level(AV_LOG_QUIET);       // a file that cannot be read is only not shown with details
    loadList();
    m_announce->setSingleShot(true);
    m_announce->setInterval(250);
    connect(m_announce, &QTimer::timeout, this, [this] {
        ++m_details_revision;
        emit detailsRevisionChanged();
    });
    m_save->setSingleShot(true);
    m_save->setInterval(3000);
    connect(m_save, &QTimer::timeout, this, &Media::saveList);
    m_worker->start(QThread::LowPriority);
}

Media::~Media()
{
    delete m_worker;
    if (m_save->isActive())
        saveList();
}

const QStringList& Media::folderNames(const QString& kind)
{
    static const QStringList videos { QStringLiteral("videos"), QStringLiteral("video") };
    static const QStringList music { QStringLiteral("music"), QStringLiteral("musica") };
    return kind == QLatin1String("videos") ? videos : music;
}

QVariantList Media::mediaFolders(const QString& kind)
{
    QVariantList out;
    const QStringList& names = folderNames(kind);
    for (const auto& root : disk_roots()) {
        const QDir dir(root.first);
        for (const QString& name : dir.entryList(QDir::Dirs | QDir::NoDotAndDotDot)) {
            if (names.contains(name.toLower()))
                out.append(QVariantMap { { QStringLiteral("path"), dir.filePath(name) }, { QStringLiteral("disk"), root.second } });
        }
    }
    return out;
}

QString Media::watchLaterDir()
{
    return paths::homePath() + QStringLiteral("/.local/state/owneet/watch_later");
}

QString Media::watchLaterFile(const QString& path)
{
    return watchLaterDir() + QChar('/')
        + QString::fromLatin1(QCryptographicHash::hash(path.toUtf8(), QCryptographicHash::Md5).toHex().toUpper());
}

double Media::readSavedPosition(const QString& path)
{
    QFile file(watchLaterFile(path));
    if (!file.open(QIODevice::ReadOnly | QIODevice::Text))
        return 0;
    while (!file.atEnd()) {
        const QByteArray line = file.readLine().trimmed();
        if (line.startsWith("start="))
            return std::max(0.0, line.mid(6).toDouble());
    }
    return 0;
}

// ISO 639-2 (as in Matroska and MP4 files), bibliographic and terminology forms
static const QHash<QString, QString>& three_letter_codes()
{
    static const QHash<QString, QString> three {
        { "eng", "en" }, { "ita", "it" }, { "fre", "fr" }, { "fra", "fr" }, { "ger", "de" }, { "deu", "de" },
        { "spa", "es" }, { "por", "pt" }, { "dut", "nl" }, { "nld", "nl" }, { "swe", "sv" }, { "nor", "no" },
        { "nob", "no" }, { "nno", "no" }, { "dan", "da" }, { "fin", "fi" }, { "pol", "pl" }, { "cze", "cs" },
        { "ces", "cs" }, { "rus", "ru" }, { "ukr", "uk" }, { "gre", "el" }, { "ell", "el" }, { "tur", "tr" },
        { "ara", "ar" }, { "heb", "he" }, { "hin", "hi" }, { "jpn", "ja" }, { "kor", "ko" }, { "chi", "zh" },
        { "zho", "zh" }, { "hun", "hu" }, { "rum", "ro" }, { "ron", "ro" }, { "cat", "ca" }, { "hrv", "hr" },
        { "srp", "sr" }, { "slv", "sl" }, { "slo", "sk" }, { "slk", "sk" }, { "bul", "bg" }, { "tha", "th" },
        { "vie", "vi" }, { "ind", "id" }, { "may", "ms" }, { "msa", "ms" },
    };
    return three;
}

QString Media::languageCode(const QString& code)
{
    QString c = code.trimmed().toLower().section(QChar('-'), 0, 0).section(QChar('_'), 0, 0);
    if (c.size() == 3)
        return three_letter_codes().value(c);   // und, mis, zxx, mul and unknown codes: no language
    static const QRegularExpression two(QStringLiteral("^[a-z]{2}$"));
    return two.match(c).hasMatch() ? c : QString();
}

QVariantMap Media::languageCodes() const
{
    QVariantMap out;
    const auto& three = three_letter_codes();
    for (auto it = three.cbegin(); it != three.cend(); ++it)
        out.insert(it.key(), it.value());
    return out;
}

bool Media::fileExists(const QString& path) const
{
    return !path.isEmpty() && QFileInfo(path).isFile();
}

QString Media::nativeLanguageName(const QString& code) const
{
    const QLocale locale(code);
    if (code.isEmpty() || locale.language() == QLocale::C)
        return code;
    QString name = locale.nativeLanguageName();
    if (name.isEmpty())
        return code;
    name[0] = name[0].toUpper();
    return name;
}

const QStringList& Media::roots(const QString& kind)
{
    if (!m_roots.contains(kind)) {
        QStringList paths;
        for (const QVariant& v : mediaFolders(kind))
            paths << v.toMap().value(QStringLiteral("path")).toString();
        m_roots.insert(kind, paths);
    }
    return m_roots[kind];
}

QString Media::diskName(const QString& disk)
{
    if (!m_disk_names.contains(disk))
        m_disk_names.insert(disk, disk_name(disk));
    return m_disk_names.value(disk);
}

QString Media::diskOf(const QString& path) const
{
    QString best, disk;
    for (const auto& root : disk_roots()) {
        if ((path == root.first || path.startsWith(root.first + QChar('/'))) && root.first.size() > best.size()) {
            best = root.first;
            disk = root.second;
        }
    }
    return disk;
}

QVariantMap Media::fileEntry(const QString& path, const QString& kind, const QString& disk)
{
    const QFileInfo finfo(path);
    return {
        { QStringLiteral("path"), path },
        { QStringLiteral("name"), finfo.completeBaseName() },
        { QStringLiteral("disk"), disk },
        { QStringLiteral("diskName"), diskName(disk) },
        { QStringLiteral("kind"), kind },
    };
}

// A folder's picture: its own; a video folder without one uses its parent's, up to the Videos
// folder (a season shows its series' picture); a music folder without one, its first song's own
QString Media::folderPicture(const QString& dir, bool video)
{
    const QStringList& roots = this->roots(video ? QStringLiteral("videos") : QStringLiteral("music"));
    for (QString d = QDir::cleanPath(dir); !roots.contains(d) && d.size() > 1; d = QFileInfo(d).path()) {
        const QString own = own_picture(d, video);
        if (!own.isEmpty())
            return own;
        if (!video) {
            QStringList songs;
            for (const QFileInfo& e : QDir(d).entryInfoList(QDir::Files, QDir::Name))
                if (file_kind(e) == QLatin1String("song"))
                    songs << e.filePath();
            std::sort(songs.begin(), songs.end(), before);
            for (const QString& song : songs) {
                const QString picture = m_list.value(song).value(QStringLiteral("p")).toString();
                if (!picture.isEmpty())
                    return picture;
            }
            return QString();
        }
        if (!roots.isEmpty() && !std::any_of(roots.cbegin(), roots.cend(), [&](const QString& r) { return d.startsWith(r + QChar('/')); }))
            break;                        // outside the Videos folders
    }
    return QString();
}

QVariantMap Media::folder(const QString& kind, const QString& path)
{
    const bool video = kind == QLatin1String("videos");
    const QString wanted = item_kind(kind);
    QVariantMap out;
    std::vector<std::pair<QString, QString>> dirs;        // (folder, disk)
    QSet<QString> disks;
    if (path.isEmpty()) {
        for (const QVariant& v : mediaFolders(kind)) {
            const QVariantMap root = v.toMap();
            dirs.emplace_back(root.value(QStringLiteral("path")).toString(), root.value(QStringLiteral("disk")).toString());
            disks.insert(dirs.back().second);
        }
        out.insert(QStringLiteral("exists"), true);
    } else {
        const QString disk = diskOf(path);
        dirs.emplace_back(path, disk);
        out.insert(QStringLiteral("exists"), QFileInfo(path).isDir());
        out.insert(QStringLiteral("name"), QFileInfo(path).fileName());
        out.insert(QStringLiteral("disk"), disk);
        out.insert(QStringLiteral("diskName"), diskName(disk));
        out.insert(QStringLiteral("picture"), picture_url(folderPicture(path, video)));
    }
    out.insert(QStringLiteral("path"), path);

    std::vector<QVariantMap> folders, files;
    QStringList to_read;
    for (const auto& dir : dirs) {
        QStringList subtitles;
        QStringList videos;
        for (const QFileInfo& e : QDir(dir.first).entryInfoList(QDir::Dirs | QDir::Files | QDir::NoDotAndDotDot)) {
            if (hidden(e.fileName()))
                continue;
            if (e.isDir()) {
                const auto count = contents_of(e.filePath(), video);
                if (count.first + count.second == 0)
                    continue;             // nothing to show inside
                folders.push_back({
                    { QStringLiteral("path"), e.filePath() },
                    { QStringLiteral("name"), e.fileName() },
                    { QStringLiteral("disk"), dir.second },
                    { QStringLiteral("diskName"), diskName(dir.second) },
                    { QStringLiteral("picture"), picture_url(folderPicture(e.filePath(), video)) },
                    { QStringLiteral("folders"), count.first },
                    { QStringLiteral("files"), count.second },
                });
                disks.insert(dir.second);
            } else if (file_kind(e) == wanted) {
                files.push_back(fileEntry(e.filePath(), wanted, dir.second));
                const auto it = m_list.constFind(e.filePath());
                if (it == m_list.constEnd() || it->value(QStringLiteral("s")).toDouble() != double(e.size())
                        || it->value(QStringLiteral("m")).toDouble() != double(e.lastModified().toMSecsSinceEpoch()))
                    to_read << e.filePath();
                if (video)
                    videos << e.filePath();
            } else if (video && SUBTITLE_SUFFIXES.contains(e.suffix().toLower())) {
                subtitles << e.fileName();
            }
        }
        // the subtitle files beside each video, by name
        for (const QString& v : videos) {
            const QString base = QFileInfo(v).completeBaseName();
            QStringList languages;
            for (const QString& s : subtitles)
                if (s.startsWith(base + QChar('.'), Qt::CaseInsensitive))
                    languages << subtitle_language(base, s);
            m_side_subs.insert(v, languages);
        }
    }
    const auto by_name = [](const QVariantMap& a, const QVariantMap& b) {
        return before(a.value(QStringLiteral("name")).toString(), b.value(QStringLiteral("name")).toString());
    };
    std::sort(folders.begin(), folders.end(), by_name);
    std::sort(files.begin(), files.end(), by_name);
    out.insert(QStringLiteral("folders"), QVariantList(folders.cbegin(), folders.cend()));
    out.insert(QStringLiteral("files"), QVariantList(files.cbegin(), files.cend()));
    out.insert(QStringLiteral("disks"), disks.size());
    // the files on screen are read first, in their order
    std::sort(to_read.begin(), to_read.end(), before);
    queue(to_read, true);
    return out;
}

QVariantMap Media::details(const QString& path)
{
    QVariantMap out;
    const auto it = m_list.constFind(path);
    const bool video = file_kind(QFileInfo(path)) == QLatin1String("video");
    QStringList subs;
    if (it != m_list.constEnd() && it->contains(QStringLiteral("d"))) {
        const QJsonObject& o = *it;
        out.insert(QStringLiteral("known"), true);
        out.insert(QStringLiteral("duration"), o.value(QStringLiteral("d")).toDouble());
        out.insert(QStringLiteral("title"), o.value(QStringLiteral("t")).toString());
        out.insert(QStringLiteral("artist"), o.value(QStringLiteral("a")).toString());
        out.insert(QStringLiteral("album"), o.value(QStringLiteral("al")).toString());
        out.insert(QStringLiteral("track"), o.value(QStringLiteral("n")).toInt());
        out.insert(QStringLiteral("year"), o.value(QStringLiteral("y")).toInt());
        out.insert(QStringLiteral("picture"), picture_url(o.value(QStringLiteral("p")).toString()));
        out.insert(QStringLiteral("audio"), o.value(QStringLiteral("au")).toVariant());
        for (const QJsonValue& s : o.value(QStringLiteral("su")).toArray())
            subs << s.toString();
    } else {
        out.insert(QStringLiteral("known"), it != m_list.constEnd());   // read, but not readable
        out.insert(QStringLiteral("duration"), 0);
    }
    if (video) {
        if (!m_side_subs.contains(path)) {
            const QFileInfo finfo(path);
            QStringList languages;
            for (const QString& s : QDir(finfo.path()).entryList(QDir::Files))
                if (s.startsWith(finfo.completeBaseName() + QChar('.'), Qt::CaseInsensitive)
                        && SUBTITLE_SUFFIXES.contains(QFileInfo(s).suffix().toLower()))
                    languages << subtitle_language(finfo.completeBaseName(), s);
            m_side_subs.insert(path, languages);
        }
        subs << m_side_subs.value(path);
        if (!m_positions.contains(path))
            m_positions.insert(path, readSavedPosition(path));
        out.insert(QStringLiteral("position"), m_positions.value(path));
    }
    out.insert(QStringLiteral("subs"), subs);
    return out;
}

QString Media::trail(const QString& path) const
{
    for (const QString& kind : { QStringLiteral("videos"), QStringLiteral("music") }) {
        for (const QVariant& v : mediaFolders(kind)) {
            const QString root = v.toMap().value(QStringLiteral("path")).toString();
            if (path.startsWith(root + QChar('/')))
                return path.mid(root.size() + 1).split(QChar('/'), Qt::SkipEmptyParts).join(QStringLiteral(" › "));
        }
    }
    return QFileInfo(path).fileName();
}

QStringList Media::allFiles(const QString& kind)
{
    if (m_all.contains(kind))
        return m_all.value(kind);
    QStringList out;
    const QString wanted = item_kind(kind);
    int seen = 0;
    for (const QString& root : roots(kind)) {
        QDirIterator it(root, QDir::Files | QDir::Dirs | QDir::NoDotAndDotDot,
                        QDirIterator::Subdirectories);
        while (it.hasNext() && seen < MAX_ENTRIES) {
            it.next();
            ++seen;
            if (it.fileInfo().isFile() && file_kind(it.fileInfo()) == wanted && !it.filePath().contains(QStringLiteral("/.")))
                out << it.filePath();
        }
    }
    m_all.insert(kind, out);
    return out;
}

QVariantMap Media::totals(const QString& kind)
{
    return { { QStringLiteral("files"), allFiles(kind).size() } };
}

QVariantList Media::continueWatching()
{
    QVariantList out;
    const QStringList& roots = this->roots(QStringLiteral("videos"));
    QStringList to_read;
    // mpv writes the video's path on the first line (--write-filename-in-watch-later-config);
    // "redirect" files for folders and old files are skipped
    for (const QFileInfo& saved : QDir(watchLaterDir()).entryInfoList(QDir::Files, QDir::Time)) {
        if (out.size() >= CONTINUE_MAX)
            break;
        QFile file(saved.filePath());
        if (!file.open(QIODevice::ReadOnly | QIODevice::Text))
            continue;
        const QString first = QString::fromUtf8(file.readLine()).trimmed();
        if (!first.startsWith(QLatin1String("# ")))
            continue;
        const QString path = first.mid(2);
        if (watchLaterFile(path) != saved.filePath() || !QFileInfo(path).isFile()
                || !std::any_of(roots.cbegin(), roots.cend(), [&](const QString& r) { return path.startsWith(r + QChar('/')); }))
            continue;
        double start = 0;
        while (!file.atEnd()) {
            const QByteArray line = file.readLine().trimmed();
            if (line.startsWith("start="))
                start = line.mid(6).toDouble();
        }
        if (start <= 0)
            continue;
        m_positions.insert(path, start);
        QVariantMap entry = fileEntry(path, QStringLiteral("video"), diskOf(path));
        entry.insert(QStringLiteral("trail"), trail(QFileInfo(path).path()));
        entry.insert(QStringLiteral("picture"), picture_url(folderPicture(QFileInfo(path).path(), true)));
        out.append(entry);
        if (!known(path))
            to_read << path;
    }
    queue(to_read, true);
    return out;
}

void Media::refreshPositions()
{
    m_positions.clear();
    ++m_details_revision;
    emit detailsRevisionChanged();
}

bool Media::known(const QString& path) const
{
    const auto it = m_list.constFind(path);
    if (it == m_list.constEnd())
        return false;
    const QFileInfo finfo(path);
    return it->value(QStringLiteral("s")).toDouble() == double(finfo.size())
        && it->value(QStringLiteral("m")).toDouble() == double(finfo.lastModified().toMSecsSinceEpoch());
}

void Media::queue(const QStringList& paths, bool first)
{
    QStringList add;
    for (const QString& path : paths) {
        if (first || !m_queued.contains(path))
            add << path;
        m_queued.insert(path);
    }
    if (!add.isEmpty())
        m_worker->add(add, first);
}

void Media::probed(const QString& path, const QJsonObject& info, qint64 reading_us)
{
    m_queued.remove(path);
    m_list.insert(path, info);
    m_save->start();
    if (!m_announce->isActive())
        m_announce->start();
    if (!m_tags_pending.remove(path))
        return;
    ++m_tags_read;
    ++m_tags_probed;
    m_tags_reading_us += reading_us;
    emit tagsProgressChanged();
    if (!m_tags_pending.isEmpty())
        return;
    // from the reading's start to its end, and the time spent in the files alone (the rest is waiting)
    const qint64 ms = QDateTime::currentMSecsSinceEpoch() - m_tags_started;
    const double n = std::max(1, m_tags_probed);
    Log::info(LOGMSG("Media: read the tags of %1 songs in %2 ms (%3 ms a song; %4 ms a song in the files)")
                  .arg(m_tags_probed).arg(ms).arg(ms / n, 0, 'f', 1).arg(m_tags_reading_us / 1000.0 / n, 0, 'f', 1));
    m_tags_complete = true;
    m_tags_state = QStringLiteral("ready");
    saveList();
    emit tagsChanged();
}

void Media::readTags()
{
    const QStringList songs = allFiles(QStringLiteral("music"));
    QStringList unread;
    for (const QString& path : songs)
        if (!known(path))
            unread << path;
    m_tags_pending = QSet<QString>(unread.cbegin(), unread.cend());
    m_tags_total = songs.size();
    m_tags_read = songs.size() - unread.size();
    m_tags_started = QDateTime::currentMSecsSinceEpoch();
    m_tags_probed = 0;
    m_tags_reading_us = 0;
    queue(unread, false);
    // the first time with a spinner; afterwards the kept list at once, new songs added quietly
    m_tags_state = unread.isEmpty() || m_tags_complete ? QStringLiteral("ready") : QStringLiteral("reading");
    emit tagsChanged();
    emit tagsProgressChanged();
}

std::vector<Media::Group> Media::groups()
{
    const QStringList roots = this->roots(QStringLiteral("music"));
    std::map<std::pair<QString, QString>, Group> found;
    for (const QString& path : allFiles(QStringLiteral("music"))) {
        const auto it = m_list.constFind(path);
        if (it == m_list.constEnd() || !it->contains(QStringLiteral("d")))
            continue;                     // not read yet, or not readable
        const QJsonObject& o = *it;
        QString artist = o.value(QStringLiteral("aa")).toString();
        if (artist.isEmpty())
            artist = o.value(QStringLiteral("a")).toString();
        // a song without an album tag: its folder is the album (none at the top of a Music folder)
        const QString dir = QFileInfo(path).path();
        QString album = o.value(QStringLiteral("al")).toString();
        if (album.isEmpty() && !roots.contains(dir))
            album = QFileInfo(dir).fileName();
        Group& g = found[{ artist, album }];
        if (g.songs.empty()) {
            g.artist = artist;
            g.album = album;
            g.folder = dir;
        }
        if (g.year == 0)
            g.year = o.value(QStringLiteral("y")).toInt();
        g.songs.push_back(path);
    }
    std::vector<Group> out;
    for (auto& entry : found)
        out.push_back(std::move(entry.second));
    return out;
}

QVariantList Media::artists()
{
    std::map<QString, std::pair<int, int>> counts;       // albums, songs
    for (const Group& g : groups()) {
        auto& c = counts[g.artist];
        ++c.first;
        c.second += int(g.songs.size());
    }
    std::vector<QVariantMap> list;
    for (const auto& c : counts)
        list.push_back({ { QStringLiteral("key"), c.first }, { QStringLiteral("name"), c.first },
                         { QStringLiteral("albums"), c.second.first }, { QStringLiteral("songs"), c.second.second } });
    std::sort(list.begin(), list.end(), [](const QVariantMap& a, const QVariantMap& b) {
        const QString x = a.value(QStringLiteral("name")).toString(), y = b.value(QStringLiteral("name")).toString();
        if (x.isEmpty() != y.isEmpty())
            return y.isEmpty();           // the unknown artist last
        return before(x, y);
    });
    return QVariantList(list.cbegin(), list.cend());
}

QVariantList Media::albums(const QString& artist)
{
    std::vector<QVariantMap> list;
    for (const Group& g : groups()) {
        if (g.artist != artist)
            continue;
        list.push_back({ { QStringLiteral("key"), g.album }, { QStringLiteral("title"), g.album },
                         { QStringLiteral("year"), g.year }, { QStringLiteral("picture"), picture_url(folderPicture(g.folder, false)) },
                         { QStringLiteral("songs"), int(g.songs.size()) } });
    }
    std::sort(list.begin(), list.end(), [](const QVariantMap& a, const QVariantMap& b) {
        const int x = a.value(QStringLiteral("year")).toInt(), y = b.value(QStringLiteral("year")).toInt();
        if (x != y)
            return x != 0 && (y == 0 || x < y);
        return before(a.value(QStringLiteral("title")).toString(), b.value(QStringLiteral("title")).toString());
    });
    return QVariantList(list.cbegin(), list.cend());
}

QVariantList Media::songs(const QString& artist, const QString& album)
{
    for (Group& g : groups()) {
        if (g.artist != artist || g.album != album)
            continue;
        const auto order = [this](const QString& p) {
            const QJsonObject o = m_list.value(p);
            return std::make_tuple(o.value(QStringLiteral("dn")).toInt(), o.value(QStringLiteral("n")).toInt());
        };
        std::sort(g.songs.begin(), g.songs.end(), [&](const QString& a, const QString& b) {
            const auto x = order(a), y = order(b);
            return x != y ? x < y : before(QFileInfo(a).fileName(), QFileInfo(b).fileName());
        });
        QVariantList out;
        for (const QString& path : g.songs)
            out.append(fileEntry(path, QStringLiteral("song"), diskOf(path)));
        return out;
    }
    return {};
}

void Media::disksChanged()
{
    m_roots.clear();
    m_disk_names.clear();
    m_all.clear();
    m_side_subs.clear();
    m_positions.clear();
    ++m_revision;
    emit revisionChanged();
    if (m_tags_state != QLatin1String("none"))
        readTags();                       // a new disk's songs join the Artists view quietly
}

void Media::loadList()
{
    QFile file(list_path());
    if (!file.open(QIODevice::ReadOnly))
        return;
    const QJsonObject root = QJsonDocument::fromJson(file.readAll()).object();
    if (root.value(QStringLiteral("version")).toInt() != LIST_VERSION)
        return;
    m_tags_complete = root.value(QStringLiteral("tagsComplete")).toBool();
    const QJsonObject files = root.value(QStringLiteral("files")).toObject();
    for (auto it = files.constBegin(); it != files.constEnd(); ++it)
        m_list.insert(it.key(), it.value().toObject());
}

void Media::saveList()
{
    m_save->stop();
    QJsonObject files;
    for (auto it = m_list.constBegin(); it != m_list.constEnd(); ++it)
        files.insert(it.key(), it.value());
    const QJsonObject root {
        { QStringLiteral("version"), LIST_VERSION },
        { QStringLiteral("tagsComplete"), m_tags_complete },
        { QStringLiteral("files"), files },
    };
    QDir().mkpath(QFileInfo(list_path()).path());
    if (!write_cache(list_path(), QJsonDocument(root).toJson(QJsonDocument::Compact)))
        Log::warning(LOGMSG("Media: cannot save the list of the files' details"));
}

} // namespace owneet
