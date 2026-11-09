// SPDX-License-Identifier: GPL-3.0-or-later
#include <QtTest/QtTest>

#include "owneet/Media.h"

#include <QCryptographicHash>
#include <QDir>
#include <QTemporaryDir>
#include <QUrl>


// Local videos and music (5.6): a pretend disk (OWNEET_DISKS) with Videos and Music folders; the
// songs are tiny WAV files with RIFF INFO tags, read by FFmpeg's library like any song.
class test_Media : public QObject {
    Q_OBJECT

private:
    QTemporaryDir m_home;
    QTemporaryDir m_disk;

    QString file(const QString& rel, const QByteArray& content = QByteArray())
    {
        const QString path = m_disk.filePath(rel);
        QDir().mkpath(QFileInfo(path).path());
        QFile f(path);
        if (!f.open(QIODevice::WriteOnly))
            qFatal("cannot write %s", qPrintable(path));
        f.write(content);
        return path;
    }

    // One second of silence (8 kHz, 16 bit, mono) with the given INFO tags (INAM title, IART
    // artist, IPRD album, ITRK track, ICRD date)
    static QByteArray wav(const QList<QPair<QByteArray, QByteArray>>& tags)
    {
        const auto le32 = [](quint32 v) { QByteArray b(4, 0); for (int i = 0; i < 4; ++i) b[i] = char((v >> (8 * i)) & 0xFF); return b; };
        const auto le16 = [](quint16 v) { QByteArray b(2, 0); b[0] = char(v & 0xFF); b[1] = char(v >> 8); return b; };
        QByteArray fmt = le16(1) + le16(1) + le32(8000) + le32(16000) + le16(2) + le16(16);
        QByteArray info = "INFO";
        for (const auto& tag : tags) {
            QByteArray value = tag.second + '\0';
            info += tag.first + le32(quint32(value.size())) + value;
            if (value.size() % 2)
                info += '\0';
        }
        const QByteArray data(16000, '\0');
        QByteArray body = "WAVE" + QByteArray("fmt ") + le32(quint32(fmt.size())) + fmt;
        if (!tags.isEmpty())
            body += "LIST" + le32(quint32(info.size())) + info;
        body += "data" + le32(quint32(data.size())) + data;
        return "RIFF" + le32(quint32(body.size())) + body;
    }

private slots:
    void initTestCase()
    {
        qputenv("PEGASUS_HOME", m_home.path().toUtf8());
        qputenv("OWNEET_DISKS", m_disk.path().toUtf8());
        file("Videos/Harbour Tales/cover.jpg", "picture");
        file("Videos/Harbour Tales/Season 1/Episode 1.mkv");
        file("Videos/Harbour Tales/Season 1/Episode 2.mkv");
        file("Videos/Harbour Tales/Season 1/Episode 2.en.srt");
        file("Videos/Harbour Tales/Season 1/Episode 10.mkv");
        file("Videos/Notes/readme.txt");                       // nothing to show: not listed
        file("Videos/.trash/Old.mkv");                         // hidden
        file("Videos/Bike ride.mp4");
        file("Music/Ada/First/01 One.wav", wav({ { "INAM", "One" }, { "IART", "Ada" }, { "IPRD", "First" }, { "ITRK", "1" }, { "ICRD", "2023" } }));
        file("Music/Ada/First/02 Two.wav", wav({ { "INAM", "Two" }, { "IART", "Ada" }, { "IPRD", "First" }, { "ITRK", "2" } }));
        file("Music/Loose/track07.wav", wav({}));
        file("Music/Rain.wav", wav({}));
    }

    void folder_names()
    {
        QVERIFY(owneet::Media::folderNames(QStringLiteral("videos")).contains(QStringLiteral("video")));
        QVERIFY(owneet::Media::folderNames(QStringLiteral("music")).contains(QStringLiteral("musica")));
        QCOMPARE(owneet::Media::mediaFolders(QStringLiteral("videos")).size(), 1);
    }

    void language_codes()
    {
        QCOMPARE(owneet::Media::languageCode(QStringLiteral("eng")), QStringLiteral("en"));
        QCOMPARE(owneet::Media::languageCode(QStringLiteral("ITA")), QStringLiteral("it"));
        QCOMPARE(owneet::Media::languageCode(QStringLiteral("en-US")), QStringLiteral("en"));
        QCOMPARE(owneet::Media::languageCode(QStringLiteral("und")), QString());
        QCOMPARE(owneet::Media::languageCode(QStringLiteral("forced")), QString());
    }

    void top_and_folders()
    {
        owneet::Media media;
        const QVariantMap top = media.folder(QStringLiteral("videos"), QString());
        const QVariantList folders = top.value(QStringLiteral("folders")).toList();
        QCOMPARE(folders.size(), 1);
        QCOMPARE(folders[0].toMap().value(QStringLiteral("name")).toString(), QStringLiteral("Harbour Tales"));
        QCOMPARE(folders[0].toMap().value(QStringLiteral("folders")).toInt(), 1);
        QCOMPARE(top.value(QStringLiteral("files")).toList().size(), 1);

        // in the order a person expects; a season shows its series' picture
        const QString season = m_disk.filePath(QStringLiteral("Videos/Harbour Tales/Season 1"));
        const QVariantMap s = media.folder(QStringLiteral("videos"), season);
        QStringList names;
        for (const QVariant& f : s.value(QStringLiteral("files")).toList())
            names << f.toMap().value(QStringLiteral("name")).toString();
        QCOMPARE(names, QStringList({ "Episode 1", "Episode 2", "Episode 10" }));
        QCOMPARE(s.value(QStringLiteral("picture")).toString(),
                 QUrl::fromLocalFile(m_disk.filePath(QStringLiteral("Videos/Harbour Tales/cover.jpg"))).toString(QUrl::FullyEncoded));
        QCOMPARE(media.trail(season), QStringLiteral("Harbour Tales › Season 1"));
        QCOMPARE(media.totals(QStringLiteral("videos")).value(QStringLiteral("files")).toInt(), 4);

        // a subtitle file beside a video, by name
        const QVariantMap ep2 = media.details(m_disk.filePath(QStringLiteral("Videos/Harbour Tales/Season 1/Episode 2.mkv")));
        QCOMPARE(ep2.value(QStringLiteral("subs")).toStringList(), QStringList({ "en" }));
    }

    void saved_positions()
    {
        const QString video = m_disk.filePath(QStringLiteral("Videos/Harbour Tales/Season 1/Episode 2.mkv"));
        const QString md5 = QString::fromLatin1(QCryptographicHash::hash(video.toUtf8(), QCryptographicHash::Md5).toHex().toUpper());
        QCOMPARE(QFileInfo(owneet::Media::watchLaterFile(video)).fileName(), md5);
        QDir().mkpath(owneet::Media::watchLaterDir());
        QFile saved(owneet::Media::watchLaterFile(video));
        QVERIFY(saved.open(QIODevice::WriteOnly));
        saved.write(("# " + video + "\nstart=724.500000\naid=2\n").toUtf8());
        saved.close();
        QCOMPARE(owneet::Media::readSavedPosition(video), 724.5);

        owneet::Media media;
        QCOMPARE(media.details(video).value(QStringLiteral("position")).toDouble(), 724.5);
        const QVariantList cont = media.continueWatching();
        QCOMPARE(cont.size(), 1);
        QCOMPARE(cont[0].toMap().value(QStringLiteral("path")).toString(), video);
        QCOMPARE(cont[0].toMap().value(QStringLiteral("trail")).toString(), QStringLiteral("Harbour Tales › Season 1"));
        QVERIFY(saved.remove());
        media.refreshPositions();
        QCOMPARE(media.details(video).value(QStringLiteral("position")).toDouble(), 0.0);
    }

    void tags_with_ffmpeg()
    {
        const QString one = m_disk.filePath(QStringLiteral("Music/Ada/First/01 One.wav"));
        {
            owneet::Media media;
            media.folder(QStringLiteral("music"), QFileInfo(one).path());     // read in the background
            QTRY_VERIFY(media.details(one).value(QStringLiteral("known")).toBool());
            const QVariantMap d = media.details(one);
            QCOMPARE(d.value(QStringLiteral("title")).toString(), QStringLiteral("One"));
            QCOMPARE(d.value(QStringLiteral("artist")).toString(), QStringLiteral("Ada"));
            QCOMPARE(d.value(QStringLiteral("album")).toString(), QStringLiteral("First"));
            QCOMPARE(d.value(QStringLiteral("track")).toInt(), 1);
            QCOMPARE(d.value(QStringLiteral("year")).toInt(), 2023);
            QVERIFY(qAbs(d.value(QStringLiteral("duration")).toDouble() - 1.0) < 0.05);

            // the Artists view: the first reading, then the artists and albums from the tags
            media.readTags();
            QCOMPARE(media.tagsState(), QStringLiteral("reading"));
            QTRY_COMPARE(media.tagsState(), QStringLiteral("ready"));
            QCOMPARE(media.tagsRead(), 4);
            const QVariantList artists = media.artists();
            QCOMPARE(artists.size(), 2);
            QCOMPARE(artists[0].toMap().value(QStringLiteral("name")).toString(), QStringLiteral("Ada"));
            QCOMPARE(artists[1].toMap().value(QStringLiteral("name")).toString(), QString());   // unknown, last
            const QVariantList unknown = media.albums(QString());
            QCOMPARE(unknown.size(), 2);                       // "Loose" (its folder), and "" (the top)
            QStringList titles;
            for (const QVariant& a : unknown)
                titles << a.toMap().value(QStringLiteral("title")).toString();
            QVERIFY(titles.contains(QStringLiteral("Loose")) && titles.contains(QString()));
            const QVariantList songs = media.songs(QStringLiteral("Ada"), QStringLiteral("First"));
            QCOMPARE(songs.size(), 2);
            QCOMPARE(songs[0].toMap().value(QStringLiteral("path")).toString(), one);
        }
        // the list is kept on the console's disk: the next time, ready at once
        owneet::Media again;
        again.readTags();
        QCOMPARE(again.tagsState(), QStringLiteral("ready"));
        QCOMPARE(again.artists().size(), 2);
    }
};


QTEST_MAIN(test_Media)
#include "test_Media.moc"
