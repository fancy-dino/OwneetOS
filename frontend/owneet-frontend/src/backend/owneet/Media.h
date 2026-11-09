// SPDX-License-Identifier: GPL-3.0-or-later
// Local videos and music ([[step-5.6-local-videos-and-music]], [[local-media]], approved demo
// design/demos/5.6-videos-and-music.html): a "Videos" or "Music" folder (or its translation) at the
// top of every disk and in the home folder, read together. A folder is listed at once from the
// disk; the details of its files (length, audio and subtitle languages, the songs' tags and own
// picture) are read with FFmpeg's library in the background, kept in a list on the console's disk
// (~/.cache/owneet/media.json) and announced with `detailsRevision`. The Artists view reads every
// song's tags once (`tagsState`); later visits use the list and read only new or changed songs.
// Where a video stopped comes from the player's saved positions: mpv's watch-later files, named
// after the MD5 of the video's path, on the console's disk.

#pragma once

#include <QHash>
#include <QJsonObject>
#include <QObject>
#include <QSet>
#include <QStringList>
#include <QVariantList>
#include <QVariantMap>

#include <vector>

class QTimer;


namespace owneet {
class MediaWorker;

class Media : public QObject {
    Q_OBJECT
    // The folders changed (a disk came or went): the pages list them again
    Q_PROPERTY(int revision READ revision NOTIFY revisionChanged)
    // Details or saved positions arrived: the items already shown update
    Q_PROPERTY(int detailsRevision READ detailsRevision NOTIFY detailsRevisionChanged)
    // The songs' tags for the Artists view: "none" (never asked), "reading" (the first time, with a
    // spinner), "ready"
    // (tagsChanged also when a later reading adds songs to the list)
    Q_PROPERTY(QString tagsState READ tagsState NOTIFY tagsChanged)
    Q_PROPERTY(int tagsRead READ tagsRead NOTIFY tagsProgressChanged)
    Q_PROPERTY(int tagsTotal READ tagsTotal NOTIFY tagsProgressChanged)

public:
    explicit Media(QObject* parent = nullptr);
    ~Media() override;

    /// The names of the media folders, lower case: "videos" or "music" and their translations
    static const QStringList& folderNames(const QString& kind);
    /// The media folders of `kind` on every disk: { path, disk } (disk: "internal" for the home
    /// folder, the mount folder for the others)
    static QVariantList mediaFolders(const QString& kind);
    /// The player's saved positions (mpv's watch-later folder)
    static QString watchLaterDir();
    /// Where mpv saves the position of the video at `path`: the MD5 of the path, upper case
    static QString watchLaterFile(const QString& path);
    /// The position saved for the video at `path`, in seconds (0: none)
    static double readSavedPosition(const QString& path);
    /// A language code as the console names it: ISO 639-1 ("en"), "" when unknown
    static QString languageCode(const QString& code);

    /// A folder of "videos" or "music"; "" is the top: every disk's media folder together.
    /// { path, name, disk, picture, exists, folders: [{ path, name, disk, picture, folders, files }],
    ///   files: [{ path, name, disk, kind }] } (kind: "video" or "song"); the files' details are
    /// read in the background (details())
    Q_INVOKABLE QVariantMap folder(const QString& kind, const QString& path);
    /// What is known of a file: { known, duration, title, artist, album, track, year, audio,
    /// subs, picture, position } (position: videos only)
    Q_INVOKABLE QVariantMap details(const QString& path);
    /// The folder names from the media folder down to `path`, joined by " › "
    Q_INVOKABLE QString trail(const QString& path) const;
    /// How many videos or songs the media folders hold, and how many folders are at their top
    Q_INVOKABLE QVariantMap totals(const QString& kind);
    /// The videos started and not finished, the latest first (at most 5): file entries
    Q_INVOKABLE QVariantList continueWatching();
    /// The saved positions are read again (the player closed)
    Q_INVOKABLE void refreshPositions();
    /// A language's own name, for codes the message files do not name
    Q_INVOKABLE QString nativeLanguageName(const QString& code) const;

    /// The Artists view: every song's tags are read once, in the background
    Q_INVOKABLE void readTags();
    /// [{ key, name, albums, songs }]: name "" is the unknown artist (shown last)
    Q_INVOKABLE QVariantList artists();
    /// An artist's albums: [{ key, title, year, picture, songs }]; title "" is "Other songs"
    Q_INVOKABLE QVariantList albums(const QString& artist);
    /// An album's songs, in their order: file entries
    Q_INVOKABLE QVariantList songs(const QString& artist, const QString& album);

    int revision() const { return m_revision; }
    int detailsRevision() const { return m_details_revision; }
    QString tagsState() const { return m_tags_state; }
    int tagsRead() const { return m_tags_read; }
    int tagsTotal() const { return m_tags_total; }

public slots:
    /// A disk was mounted or unmounted (Storage::volumesChanged)
    void disksChanged();

signals:
    void revisionChanged();
    void detailsRevisionChanged();
    void tagsChanged();
    void tagsProgressChanged();

private:
    friend class MediaWorker;
    struct Group;
    const QStringList& roots(const QString& kind);
    QVariantMap fileEntry(const QString& path, const QString& kind, const QString& disk);
    QString diskOf(const QString& path) const;
    QString diskName(const QString& disk);
    QString folderPicture(const QString& dir, bool video);
    QStringList allFiles(const QString& kind);
    bool known(const QString& path) const;
    void queue(const QStringList& paths, bool first);
    void probed(const QString& path, const QJsonObject& info, qint64 reading_us);
    std::vector<Group> groups();
    void loadList();
    void saveList();

    MediaWorker* m_worker;
    QTimer* m_announce;                    // details arrive in bursts: announced together
    QTimer* m_save;
    QHash<QString, QJsonObject> m_list;    // the details read, by path (the list on the disk)
    QSet<QString> m_queued;
    QHash<QString, double> m_positions;    // saved positions read so far, by path
    QHash<QString, QStringList> m_side_subs;   // subtitle files beside a video: their languages
    QHash<QString, QStringList> m_all;     // every file of a kind, while the disks stay the same
    QHash<QString, QStringList> m_roots;  // the media folders of a kind, while the disks stay the same
    QHash<QString, QString> m_disk_names;
    int m_revision = 0;
    int m_details_revision = 0;
    QString m_tags_state = QStringLiteral("none");
    int m_tags_read = 0;
    int m_tags_total = 0;
    bool m_tags_complete = false;          // a whole reading finished once (kept in the list)
    QSet<QString> m_tags_pending;
    qint64 m_tags_started = 0;
    int m_tags_probed = 0;
    qint64 m_tags_reading_us = 0;          // the time spent in the files themselves
};
} // namespace owneet
