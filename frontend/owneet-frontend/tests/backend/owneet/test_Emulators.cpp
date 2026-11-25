// SPDX-License-Identifier: GPL-3.0-or-later
#include <QtTest/QtTest>

#include "model/gaming/Game.h"
#include "owneet/Emulators.h"

#include <QDir>
#include <QTemporaryDir>


// The user's own emulators (4.9): pretend folders, everything in them invented.
class test_Emulators : public QObject {
    Q_OBJECT

private:
    QTemporaryDir m_home;
    QTemporaryDir m_disk;

    QString file(const QString& rel, const QByteArray& content = "x", bool exec = false)
    {
        const QString path = m_disk.filePath(rel);
        QDir().mkpath(QFileInfo(path).path());
        QFile f(path);
        if (!f.open(QIODevice::WriteOnly))
            qFatal("cannot write %s", qPrintable(path));
        f.write(content);
        if (exec)
            f.setPermissions(f.permissions() | QFileDevice::ExeOwner | QFileDevice::ExeUser);
        return path;
    }
    static QByteArray program()
    {
        QByteArray head("\x7f" "ELF", 4);
        head += QByteArray(12, '\0');
        head += QByteArray("\x03\x00\x3e\x00", 4);
        return head + QByteArray(64, '\0');
    }
    owneet::Emulators::ConsoleDir console(const QString& rel) { return owneet::Emulators::consoleDir(m_disk.filePath(rel)); }

private slots:
    void initTestCase()
    {
        qputenv("HOME", m_home.path().toUtf8());
        qputenv("PEGASUS_HOME", m_home.path().toUtf8());
        file("FunContainer/EMULATORI/FunPlayer.AppImage");                      // no exec bit: unpacked by the console
        file("FunContainer/EMULATORI/BoxEmu/BoxEmu-2.1-linux.zip");
        file("FunContainer/EMULATORI/BoxEmu/readme.txt");
        file("FunContainer/EMULATORI/readme.txt");
        file("FunContainer/EMULATORI/Tools/first", program(), true);
        file("FunContainer/EMULATORI/Tools/second", program(), true);
        file("FunContainer/EMULATORI/Tools/not-executable", program());
        file("FunContainer/EMULATORI/FunPlayer.AppImage.home/inner.AppImage");  // an AppImage's own settings
        file("FunContainer/giochi/Copper Coast.img");
        file("FunContainer/giochi/Copper Coast.srm");
        file("FunContainer/giochi/Copper Coast.jpg");
        file("FunContainer/giochi/Ember Run.cue", "FILE \"Ember Run (Track 1).bin\" BINARY\nFILE \"Ember Run (Track 2).bin\" BINARY\n");
        file("FunContainer/giochi/Ember Run (Track 1).bin", QByteArray(100000, 'a'));
        file("FunContainer/giochi/Ember Run (Track 2).bin", QByteArray(100000, 'b'));
        file("FunContainer/giochi/Lumen Drift.m3u", "Lumen Drift (Disc 1).cue\nLumen Drift (Disc 2).cue\n");
        file("FunContainer/giochi/Lumen Drift (Disc 1).cue", "FILE \"Lumen Drift (Disc 1).bin\" BINARY\n");
        file("FunContainer/giochi/Lumen Drift (Disc 1).bin", QByteArray(100000, 'c'));
        file("FunContainer/giochi/Lumen Drift (Disc 2).cue", "FILE \"Lumen Drift (Disc 2).bin\" BINARY\n");
        file("FunContainer/giochi/Lumen Drift (Disc 2).bin", QByteArray(100000, 'd'));
        file("FunContainer/giochi/Moss Garden.one", QByteArray(100000, 'e'));
        file("FunContainer/giochi/Moss Garden.two", QByteArray(100000, 'f'));
        file("FunContainer/giochi/Night Convoy/disc.img");
        file("FunContainer/giochi/Night Convoy/cover.jpg");
        file("FunContainer/giochi/Quiet Orbit/data/a.dat");
        file("TinyPad/Emulateurs/readme.txt");
        file("TinyPad/Jeux/Saltwind.img");
        file("OnlyGames/Games/a.img");
        file("A game/Emulators/x.AppImage");
        file("A game/Games/level.dat");
        file("A game/start.sh", "#!/bin/sh\n");
        file("Installed/Emulators/x.AppImage");
        file("Installed/Games/level.dat");
        file("Installed/.owneet-installed", "{}");
    }

    void folders()
    {
        const auto c = console("FunContainer");
        QVERIFY(c.valid());
        QCOMPARE(c.name, QStringLiteral("FunContainer"));
        QVERIFY(console("TinyPad").valid());
        QVERIFY(!console("OnlyGames").valid());
        QVERIFY(!console("A game").valid());            // a game of its own: something to start at its top
        QVERIFY(!console("Installed").valid());
        QVERIFY(!console("not there").valid());
        QVERIFY(owneet::Emulators::atTop(m_disk.path()));
        QVERIFY(!owneet::Emulators::atTop(m_disk.filePath("OnlyGames")));
    }

    void emulators()
    {
        QStringList names;
        for (const auto& e : owneet::Emulators::emulatorsOf(console("FunContainer")))
            names << e.id + QChar('=') + e.name;
        QCOMPARE(names, QStringList({ "BoxEmu/BoxEmu-2.1-linux.zip=BoxEmu", "FunPlayer.AppImage=FunPlayer",
                                      "Tools/first=Tools: first", "Tools/second=Tools: second" }));
        QVERIFY(owneet::Emulators::emulatorsOf(console("TinyPad")).isEmpty());
    }

    void titles()
    {
        QStringList names;
        for (const auto& t : owneet::Emulators::titlesOf(console("FunContainer"))) {
            QStringList files;
            for (const QString& f : t.files)
                files << QFileInfo(f).fileName();
            names << t.name + QChar('=') + files.join(QChar('|'));
        }
        QCOMPARE(names, QStringList({ "Copper Coast=Copper Coast.img", "Ember Run=Ember Run.cue", "Lumen Drift=Lumen Drift.m3u",
                                      "Moss Garden=Moss Garden.one|Moss Garden.two", "Night Convoy=disc.img", "Quiet Orbit=Quiet Orbit" }));
    }

    void options()
    {
        using E = owneet::Emulators;
        QCOMPARE(E::splitOptions(""), QStringList());
        QCOMPARE(E::splitOptions("  -g  "), QStringList({ "-g" }));
        QCOMPARE(E::splitOptions("-batch -fullscreen"), QStringList({ "-batch", "-fullscreen" }));
        QCOMPARE(E::splitOptions("--config=\"My file.cfg\" \"two words\" a;b {file.path}"),
                 QStringList({ "--config=My file.cfg", "two words", "a;b", "{file.path}" }));
        QCOMPARE(E::splitOptions("-x \"\""), QStringList({ "-x", "" }));
        // everything in one word without spaces, quotes or braces
        const QStringList cmd = E::command("/a \"b\"/c.AppImage", { "{file.path}", "it's" }, "/games/Copper Coast.img");
        QCOMPARE(cmd.size(), 2);
        const QList<QByteArray> words = QByteArray::fromBase64(cmd.at(1).toLatin1()).split('\0');
        QCOMPARE(words.size(), 5);
        QCOMPARE(words.at(1), QByteArray("/a \"b\"/c.AppImage"));
        QCOMPARE(words.at(2), QByteArray("/games/Copper Coast.img"));
        QCOMPARE(words.at(3), QByteArray("{file.path}"));
        QCOMPARE(words.at(4), QByteArray("it's"));
        QVERIFY(!cmd.at(1).contains(QChar(' ')));
    }

    void choices()
    {
        model::Game game(QStringLiteral("Moss Garden"));
        QVariantMap& extra = game.extraMapMut();
        extra.insert("emu.console", m_disk.filePath("FunContainer"));
        extra.insert("emu.path", m_disk.filePath("FunContainer/giochi/Moss Garden.one"));
        const QStringList files { m_disk.filePath("FunContainer/giochi/Moss Garden.one"), m_disk.filePath("FunContainer/giochi/Moss Garden.two") };
        extra.insert("emu.files", files);
        owneet::Emulators e;
        QCOMPARE(e.list(&game).size(), 4);
        QCOMPARE(e.folder(&game), QStringLiteral("EMULATORI"));
        QCOMPARE(e.chosen(&game), QString());
        QVERIFY(!e.prepare(&game));
        e.choose(&game, "FunPlayer.AppImage");
        QCOMPARE(e.chosen(&game), QStringLiteral("FunPlayer.AppImage"));
        QVERIFY(!e.told(&game, "FunPlayer.AppImage"));
        e.setTold(&game, "FunPlayer.AppImage");
        QVERIFY(e.told(&game, "FunPlayer.AppImage"));
        QVERIFY(!e.told(&game, "Tools/first"));
        QCOMPARE(e.file(&game), QString());             // two files: asked
        QVERIFY(!e.prepare(&game));
        e.chooseFile(&game, files.at(1));
        QCOMPARE(e.file(&game), files.at(1));
        e.setOptions(&game, "FunPlayer.AppImage", " -g ");
        QCOMPARE(e.options(&game, "FunPlayer.AppImage"), QStringLiteral("-g"));
        QVERIFY(e.prepare(&game));
        QVERIFY(game.launchCmd().startsWith("/usr/lib/owneet/owneet-emulator-run "));
        const QList<QByteArray> words = QByteArray::fromBase64(game.launchCmd().section(QChar(' '), 1).toLatin1()).split('\0');
        QCOMPARE(words.at(2), files.at(1).toUtf8());
        QCOMPARE(words.at(3), QByteArray("-g"));
        // the emulator chosen is gone: asked again
        e.choose(&game, "gone.AppImage");
        QCOMPARE(e.chosen(&game), QString());
        // opened by itself: no game, no options
        const QVariantMap open = e.openCommand(&game, "FunPlayer.AppImage");
        const QList<QByteArray> alone = QByteArray::fromBase64(open.value("command").toStringList().at(1).toLatin1()).split('\0');
        QCOMPARE(alone.size(), 3);
        QCOMPARE(alone.at(2), QByteArray());
    }

    void create()
    {
        owneet::Emulators e;
        QTemporaryDir disk;
        QVariantMap made = e.create("Old: Box?", disk.path(), "it");
        QCOMPARE(made.value("ok").toBool(), true);
        QCOMPARE(made.value("name").toString(), QStringLiteral("Old - Box"));
        QCOMPARE(made.value("games").toString(), QStringLiteral("Giochi"));
        QVERIFY(QFileInfo(disk.filePath("Giochi/Old - Box/Emulatori")).isDir());
        QVERIFY(QFileInfo(disk.filePath("Giochi/Old - Box/Giochi")).isDir());
        made = e.create("Old: Box?", disk.path(), "en");    // there already: shown, nothing made twice
        QCOMPARE(made.value("ok").toBool(), true);
        QCOMPARE(made.value("emulators").toString(), QStringLiteral("Emulatori"));
        QCOMPARE(QDir(disk.filePath("Giochi/Old - Box")).entryList(QDir::Dirs | QDir::NoDotAndDotDot).size(), 2);
        QCOMPARE(e.create("..", disk.path(), "en").value("ok").toBool(), false);
        QCOMPARE(e.create("../../x", disk.path(), "en").value("name").toString(), QStringLiteral("x"));
        QCOMPARE(e.create("x", disk.filePath("not there"), "en").value("ok").toBool(), false);
    }
};

QTEST_MAIN(test_Emulators)
#include "test_Emulators.moc"
