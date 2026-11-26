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
    static QByteArray program(char machine = '\x3e')
    {
        QByteArray head("\x7f" "ELF", 4);
        head += QByteArray(12, '\0');
        head += QByteArray("\x03\x00", 2) + machine + '\0';
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
        // a folder in a folder, as an archive leaves it, with a tiny file beside the game's own folders
        file("EMULAZIONE/FunContainer/giochi/Paper Lantern/notes.txt");
        file("EMULAZIONE/FunContainer/giochi/Paper Lantern/Paper Lantern (Europe) [en]/info.dat", QByteArray(1500, '\0'));
        file("EMULAZIONE/FunContainer/giochi/Paper Lantern/Paper Lantern (Europe) [en]/DATA/main.bin", QByteArray(2000000, 'g'));
        file("EMULAZIONE/FunContainer/giochi/River Post/River Post/disc.img");
        file("EMULAZIONE/FunContainer/giochi/Stone Kite/CODE0001/disc.img");                // another name: as before
        file("EMULAZIONE/FunContainer/giochi/Tin Harbour/list.cue", "FILE \"tracks/one.bin\" BINARY\n");
        file("EMULAZIONE/FunContainer/giochi/Tin Harbour/tracks/one.bin", QByteArray(2000000, 'h'));
        file("EMULAZIONE/FunContainer/giochi/Umber Gate/info.dat", QByteArray(1500, '\0'));     // a file beside a folder: as before
        file("EMULAZIONE/FunContainer/giochi/Umber Gate/DATA/main.bin", QByteArray(2000000, 'j'));
        file("EMULAZIONE/FunContainer/giochi/Velvet Mile/main.exe", QByteArray(500000, 'k'));
        file("EMULAZIONE/FunContainer/giochi/Velvet Mile/data/big.pak", QByteArray(2000000, 'l'));
        file("EMULAZIONE/FunContainer/giochi/Willow Run/code/a.one", QByteArray(2000000, 'm'));
        file("EMULAZIONE/FunContainer/giochi/Willow Run/content/b.two");
        file("EMULAZIONE/FunContainer/giochi/Yellow Pier/Yellow Pier (Rev 1)/PART/a.one");   // the named one holds only another
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
            names << t.name + QChar('=') + files.join(QChar('|')) + (t.sure ? QString() : QStringLiteral("?"));
        }
        QCOMPARE(names, QStringList({ "Copper Coast=Copper Coast.img", "Ember Run=Ember Run.cue", "Lumen Drift=Lumen Drift.m3u",
                                      "Moss Garden=Moss Garden.one|Moss Garden.two?", "Night Convoy=disc.img",
                                      "Paper Lantern=info.dat|Paper Lantern (Europe) [en]|Paper Lantern?", "Quiet Orbit=Quiet Orbit",
                                      "River Post=disc.img|River Post", "Stone Kite=Stone Kite", "Tin Harbour=list.cue",
                                      "Umber Gate=info.dat", "Velvet Mile=main.exe", "Willow Run=Willow Run",
                                      "Yellow Pier=Yellow Pier (Rev 1)|Yellow Pier" }));
    }

    // made for another kind of processor: shown, never chosen by the console
    void otherProcessor()
    {
        const auto mark = [this](const QString& rel) {
            QStringList names;
            for (const auto& e : owneet::Emulators::emulatorsOf(console(rel)))
                names << e.id + (e.fits ? QString() : QStringLiteral("!"));
            return names;
        };
        file("EMULAZIONE/OddBox/Emulators/odd.AppImage", program('\xb7'));
        file("EMULAZIONE/OddBox/Games/a.img");
        QCOMPARE(mark("EMULAZIONE/OddBox"), QStringList({ "odd.AppImage!" }));
        model::Game game(QStringLiteral("a"));
        game.extraMapMut().insert("emu.console", m_disk.filePath("EMULAZIONE/OddBox"));
        owneet::Emulators e;
        QCOMPARE(e.chosen(&game), QStringLiteral("odd.AppImage"));          // the only one: said when it starts
        file("EMULAZIONE/OddBox/Emulators/right.AppImage", program());
        QCOMPARE(mark("EMULAZIONE/OddBox"), QStringList({ "odd.AppImage!", "right.AppImage" }));
        QCOMPARE(e.chosen(&game), QStringLiteral("right.AppImage"));        // the only one that can run
        file("EMULAZIONE/OddBox/Emulators/text.sh", "#!/bin/bash\n");
        QCOMPARE(e.chosen(&game), QString());                              // two: asked
        e.choose(&game, "odd.AppImage");
        QCOMPARE(e.chosen(&game), QString());
        // plain programs: only when there is nothing else
        file("EMULAZIONE/OddPad/Emulators/Tool/tool", program('\xb7'), true);
        file("EMULAZIONE/OddPad/Games/a.img");
        QCOMPARE(mark("EMULAZIONE/OddPad"), QStringList({ "Tool/tool!" }));
        file("EMULAZIONE/OddPad/Emulators/good", program(), true);
        QCOMPARE(mark("EMULAZIONE/OddPad"), QStringList({ "good" }));
    }

    // A recognised emulator (an invented card): how it starts, which file, its system file, its settings
    void cards()
    {
        QTemporaryDir cards;
        QFile card(cards.filePath("box.json"));
        QVERIFY(card.open(QIODevice::WriteOnly));
        card.write(R"({ "match": ["boxstation*"], "start": ["--full", "{game}"], "prefer": "folder",
            "systemFile": { "name": "SYSTEM.BIN", "installed": ["~/.boxstation/ready"], "install": ["--install", "{file}"] },
            "settings": [ { "path": "~/.boxstation/pad.ini", "text": "Device = {pad.name}\n" } ] })");
        card.close();
        qputenv("OWNEET_EMULATOR_CARDS", cards.path().toUtf8());
        file("EMULAZIONE/BigBox/Emulators/BoxStation-1.0.AppImage");
        file("EMULAZIONE/BigBox/Games/Night Convoy/Night Convoy/info.dat", QByteArray(1500, '\0'));
        file("EMULAZIONE/BigBox/Games/Night Convoy/Night Convoy/DATA/main.bin");
        const QString inner = m_disk.filePath("EMULAZIONE/BigBox/Games/Night Convoy/Night Convoy");
        model::Game game(QStringLiteral("Night Convoy"));
        QVariantMap& extra = game.extraMapMut();
        extra.insert("emu.console", m_disk.filePath("EMULAZIONE/BigBox"));
        extra.insert("emu.path", m_disk.filePath("EMULAZIONE/BigBox/Games/Night Convoy"));
        for (const auto& t : owneet::Emulators::titlesOf(console("EMULAZIONE/BigBox"))) {
            extra.insert("emu.files", t.files);
            extra.insert("emu.sure", t.sure);
        }
        owneet::Emulators e;
        QCOMPARE(e.file(&game), inner);                                       // the folder, as the card prefers
        // its system file: not there, then copied beside it (any case)
        QVariantMap sys = e.systemFile(&game);
        QCOMPARE(sys.value("needed").toBool(), true);
        QCOMPARE(sys.value("found").toString(), QString());
        QVERIFY(e.installCommand(&game).isEmpty());
        const QString copied = file("EMULAZIONE/BigBox/Emulators/files/system.bin");
        sys = e.systemFile(&game);
        QCOMPARE(sys.value("found").toString(), copied);
        QCOMPARE(sys.value("installed").toBool(), false);
        const auto words = [](const QStringList& cmd) { return QByteArray::fromBase64(cmd.at(1).toLatin1()).split('\0'); };
        QList<QByteArray> w = words(e.installCommand(&game).value("command").toStringList());
        QCOMPARE(w.mid(2), QList<QByteArray>({ "", "--install", copied.toUtf8() }));
        QDir().mkpath(m_home.filePath(".boxstation"));
        QFile(m_home.filePath(".boxstation/ready")).open(QIODevice::WriteOnly);
        QCOMPARE(e.systemFile(&game).value("installed").toBool(), true);
        // how it starts: the card's, the game in its place; the settings with the controller's name
        const QString ini = m_home.filePath(".boxstation/pad.ini");
        QVERIFY(e.prepare(&game, QString()));
        QVERIFY(!QFile::exists(ini));                                        // no controller yet: not written
        QVERIFY(e.prepare(&game, "Pad One"));
        w = words(game.launchCmd().split(QChar(' ')));
        QCOMPARE(w.mid(2), QList<QByteArray>({ "", "--full", inner.toUtf8() }));
        const auto read = [&ini]() { QFile f(ini); f.open(QIODevice::ReadOnly); return f.readAll(); };
        QCOMPARE(read(), QByteArray("Device = Pad One\n"));
        QVERIFY(e.prepare(&game, "Pad Two"));                                 // its own file: written again
        QCOMPARE(read(), QByteArray("Device = Pad Two\n"));
        {
            QFile f(ini);
            QVERIFY(f.open(QIODevice::WriteOnly));
            f.write("Device = the user's\n");
        }
        QVERIFY(e.prepare(&game, "Pad Three"));                               // the user's: never touched
        QCOMPARE(read(), QByteArray("Device = the user's\n"));
        // the user's own choice of how it starts wins
        e.setOptions(&game, "BoxStation-1.0.AppImage", "");
        QVERIFY(e.prepare(&game, "Pad One"));
        w = words(game.launchCmd().split(QChar(' ')));
        QCOMPARE(w.mid(2), QList<QByteArray>({ inner.toUtf8() }));
        qunsetenv("OWNEET_EMULATOR_CARDS");
        QVERIFY(!e.systemFile(&game).value("needed").toBool());               // no card: as before
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
        // several, the first without asking; another one can still be chosen
        extra.insert("emu.path", m_disk.filePath("EMULAZIONE/FunContainer/giochi/River Post"));
        extra.insert("emu.sure", true);
        QCOMPARE(e.file(&game), files.at(0));
        e.chooseFile(&game, files.at(1));
        QCOMPARE(e.file(&game), files.at(1));
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
