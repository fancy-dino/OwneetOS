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
        file("EMULAZIONE/FunContainer/EMULATORI/FunPlayer.AppImage");                      // no exec bit: unpacked by the console
        file("EMULAZIONE/FunContainer/EMULATORI/BoxEmu/BoxEmu-2.1-linux.zip");
        file("EMULAZIONE/FunContainer/EMULATORI/BoxEmu/readme.txt");
        file("EMULAZIONE/FunContainer/EMULATORI/readme.txt");
        file("EMULAZIONE/FunContainer/EMULATORI/Tools/first", program(), true);
        file("EMULAZIONE/FunContainer/EMULATORI/Tools/second", program(), true);
        file("EMULAZIONE/FunContainer/EMULATORI/Tools/not-executable", program());
        file("EMULAZIONE/FunContainer/EMULATORI/FunPlayer.AppImage.home/inner.AppImage");  // an AppImage's own settings
        file("EMULAZIONE/FunContainer/giochi/Copper Coast.img");
        file("EMULAZIONE/FunContainer/giochi/Copper Coast.srm");
        file("EMULAZIONE/FunContainer/giochi/Copper Coast.jpg");
        file("EMULAZIONE/FunContainer/giochi/Ember Run.cue", "FILE \"Ember Run (Track 1).bin\" BINARY\nFILE \"Ember Run (Track 2).bin\" BINARY\n");
        file("EMULAZIONE/FunContainer/giochi/Ember Run (Track 1).bin", QByteArray(100000, 'a'));
        file("EMULAZIONE/FunContainer/giochi/Ember Run (Track 2).bin", QByteArray(100000, 'b'));
        file("EMULAZIONE/FunContainer/giochi/Lumen Drift.m3u", "Lumen Drift (Disc 1).cue\nLumen Drift (Disc 2).cue\n");
        file("EMULAZIONE/FunContainer/giochi/Lumen Drift (Disc 1).cue", "FILE \"Lumen Drift (Disc 1).bin\" BINARY\n");
        file("EMULAZIONE/FunContainer/giochi/Lumen Drift (Disc 1).bin", QByteArray(100000, 'c'));
        file("EMULAZIONE/FunContainer/giochi/Lumen Drift (Disc 2).cue", "FILE \"Lumen Drift (Disc 2).bin\" BINARY\n");
        file("EMULAZIONE/FunContainer/giochi/Lumen Drift (Disc 2).bin", QByteArray(100000, 'd'));
        file("EMULAZIONE/FunContainer/giochi/Moss Garden.one", QByteArray(100000, 'e'));
        file("EMULAZIONE/FunContainer/giochi/Moss Garden.two", QByteArray(100000, 'f'));
        file("EMULAZIONE/FunContainer/giochi/Night Convoy/disc.img");
        file("EMULAZIONE/FunContainer/giochi/Night Convoy/cover.jpg");
        file("EMULAZIONE/FunContainer/giochi/Quiet Orbit/data/a.dat");
        file("EMULAZIONE/TinyPad/Emulateurs/readme.txt");
        file("EMULAZIONE/TinyPad/Jeux/Saltwind.img");
        file("EMULAZIONE/OnlyGames/Games/a.img");
        file("Games/A game/Emulators/x.AppImage");          // in the games folder: a game, never looked into
        file("Games/A game/Games/level.dat");
        file("Beside/Emulators/x.AppImage");                // at the top of the disk: not in "Emulation"
        file("Beside/Games/a.img");
    }

    void folders()
    {
        const auto c = console("EMULAZIONE/FunContainer");
        QVERIFY(c.valid());
        QCOMPARE(c.name, QStringLiteral("FunContainer"));
        QVERIFY(console("EMULAZIONE/TinyPad").valid());
        QVERIFY(!console("EMULAZIONE/OnlyGames").valid());
        QVERIFY(!console("not there").valid());
        QStringList names;
        for (const auto& c2 : owneet::Emulators::consolesOf(m_disk.path()))
            names << c2.name;
        QCOMPARE(names, QStringList({ "FunContainer", "TinyPad" }));
        for (const char* base : {"Emulation", "emulazione", "Émulation", "E\u0301mulation", "Emulación", "EMULACION"})
            QVERIFY2(owneet::Emulators::isBase(QString::fromUtf8(base)), base);
        QVERIFY(!owneet::Emulators::isBase("Emulators"));
        QVERIFY(!owneet::Emulators::isBase("Games"));
    }

    void emulators()
    {
        QStringList names;
        for (const auto& e : owneet::Emulators::emulatorsOf(console("EMULAZIONE/FunContainer")))
            names << e.id + QChar('=') + e.name;
        QCOMPARE(names, QStringList({ "BoxEmu/BoxEmu-2.1-linux.zip=BoxEmu", "FunPlayer.AppImage=FunPlayer",
                                      "Tools/first=Tools: first", "Tools/second=Tools: second" }));
        QVERIFY(owneet::Emulators::emulatorsOf(console("EMULAZIONE/TinyPad")).isEmpty());
    }

    void titles()
    {
        QStringList names;
        for (const auto& t : owneet::Emulators::titlesOf(console("EMULAZIONE/FunContainer"))) {
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
        extra.insert("emu.console", m_disk.filePath("EMULAZIONE/FunContainer"));
        extra.insert("emu.path", m_disk.filePath("EMULAZIONE/FunContainer/giochi/Moss Garden.one"));
        const QStringList files { m_disk.filePath("EMULAZIONE/FunContainer/giochi/Moss Garden.one"), m_disk.filePath("EMULAZIONE/FunContainer/giochi/Moss Garden.two") };
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
        QCOMPARE(made.value("base").toString(), QStringLiteral("Emulazione"));
        QVERIFY(QFileInfo(disk.filePath("Emulazione/Old - Box/Emulatori")).isDir());
        QVERIFY(QFileInfo(disk.filePath("Emulazione/Old - Box/Giochi")).isDir());
        made = e.create("Old: Box?", disk.path(), "en");    // there already: shown, nothing made twice
        QCOMPARE(made.value("ok").toBool(), true);
        QCOMPARE(made.value("emulators").toString(), QStringLiteral("Emulatori"));
        QCOMPARE(QDir(disk.filePath("Emulazione/Old - Box")).entryList(QDir::Dirs | QDir::NoDotAndDotDot).size(), 2);
        QCOMPARE(e.create("..", disk.path(), "en").value("ok").toBool(), false);
        QCOMPARE(e.create("../../x", disk.path(), "en").value("name").toString(), QStringLiteral("x"));
        QCOMPARE(e.create("x", disk.filePath("not there"), "en").value("ok").toBool(), false);
    }
};

QTEST_MAIN(test_Emulators)
#include "test_Emulators.moc"
