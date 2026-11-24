// SPDX-License-Identifier: GPL-3.0-or-later
// OwneetOS: Steam games from Steam's local files ([[step-4.1-steam-library]]), on an invented Steam
// folder.
#include <QtTest/QtTest>

#include "model/gaming/Game.h"
#include "providers/SearchContext.h"
#include "providers/steam/SteamPlaytime.h"
#include "providers/steam/SteamProvider.h"

#include <QTemporaryDir>
#include <QtEndian>


namespace {
// Steam's binary KeyValues (versions without a table of keys)
QByteArray le32(quint32 v)
{
    QByteArray out(4, '\0');
    qToLittleEndian(v, out.data());
    return out;
}
QByteArray key(char type, const char* name) { return QByteArray(1, type) + QByteArray(name) + '\0'; }
QByteArray text(const char* name, const char* value) { return key(1, name) + QByteArray(value) + '\0'; }
QByteArray number(const char* name, quint32 value) { return key(2, name) + le32(value); }
QByteArray block(const char* name, const QByteArray& body) { return key(0, name) + body + char(8); }

// What Steam says of a game: rating on SteamOS, the token of one reason, when it was checked
QByteArray compat(quint32 rating, const char* token, quint32 checked)
{
    return block("steam_deck_compatibility", number("category", 1) + number("steamos_compatibility", rating)
        + number("test_timestamp", checked)
        + block("steamos_tests", block("0", number("display", 1) + text("token", token))));
}
// an entry of appinfo.vdf, version 28
QByteArray app(quint32 id, const char* name, const QByteArray& more)
{
    const QByteArray body = block("appinfo", block("common", text("name", name) + text("type", "Game") + more)) + char(8);
    const QByteArray head = QByteArray(4 + 4 + 8 + 20 + 4 + 20, '\0');
    return le32(id) + le32(quint32(head.size() + body.size())) + head + body;
}
} // namespace


class test_Steam : public QObject {
    Q_OBJECT

private:
    QTemporaryDir m_steam;
    QTemporaryDir m_disk;           // a second library, e.g. on another disk

    void write(const QString& path, const QString& text)
    {
        QVERIFY(QDir().mkpath(QFileInfo(path).path()));
        QFile file(path);
        QVERIFY(file.open(QIODevice::WriteOnly | QIODevice::Text));
        file.write(text.toUtf8());
    }
    void manifest(const QString& library, const char* appid, const char* name, const char* flags)
    {
        write(library + QStringLiteral("/steamapps/appmanifest_%1.acf").arg(QLatin1String(appid)),
              QStringLiteral("\"AppState\"\n{\n\t\"appid\"\t\t\"%1\"\n\t\"name\"\t\t\"%2\"\n\t\"StateFlags\"\t\t\"%3\"\n}\n")
                  .arg(QLatin1String(appid), QLatin1String(name), QLatin1String(flags)));
    }

private slots:
    void initTestCase()
    {
        const QString steam = m_steam.path();
        write(steam + QStringLiteral("/steamapps/libraryfolders.vdf"), QStringLiteral(
            "\"libraryfolders\"\n{\n"
            "\t\"0\"\n\t{\n\t\t\"path\"\t\t\"%1\"\n\t}\n"
            "\t\"1\"\n\t{\n\t\t\"path\"\t\t\"%2\"\n\t}\n"
            "\t\"2\"\n\t{\n\t\t\"path\"\t\t\"/nowhere/unplugged-disk\"\n\t}\n"   // not connected
            "}\n").arg(steam, m_disk.path()));
        manifest(steam, "10", "Copper Coast", "4");                     // installed
        manifest(steam, "20", "Half Downloaded", "1026");               // first download running
        manifest(steam, "30", "Needs An Update", "6");                  // installed, update waiting
        manifest(steam, "1493710", "Proton Experimental", "4");         // Valve's tools
        manifest(steam, "4183110", "Steam Linux Runtime 4.0", "4");
        manifest(steam, "228980", "Steamworks Common Redistributables", "4");
        manifest(m_disk.path(), "40", "Ember \\\"Run\\\"", "4");        // escaped quotes in the name

        // two accounts: the latest login counts (SteamID64 76561197960265728 + account id)
        write(steam + QStringLiteral("/config/loginusers.vdf"), QStringLiteral(
            "\"users\"\n{\n"
            "\t\"76561197960265829\"\n\t{\n\t\t\"AccountName\"\t\t\"old\"\n\t\t\"Timestamp\"\t\t\"1600000000\"\n\t}\n"
            "\t\"76561197960265828\"\n\t{\n\t\t\"AccountName\"\t\t\"new\"\n\t\t\"Timestamp\"\t\t\"1700000000\"\n\t}\n"
            "}\n"));
        // "valve" in lower case, as some clients write it; a comment line too
        write(steam + QStringLiteral("/userdata/100/config/localconfig.vdf"), QStringLiteral(
            "\"UserLocalConfigStore\"\n{\n\t\"Software\"\n\t{\n\t\t\"valve\"\n\t\t{\n\t\t\t\"Steam\"\n\t\t\t{\n"
            "\t\t\t\t// play time in minutes\n"
            "\t\t\t\t\"apps\"\n\t\t\t\t{\n"
            "\t\t\t\t\t\"10\"\n\t\t\t\t\t{\n\t\t\t\t\t\t\"LastPlayed\"\t\t\"1700000500\"\n\t\t\t\t\t\t\"Playtime\"\t\t\"90\"\n\t\t\t\t\t}\n"
            "\t\t\t\t\t\"999\"\n\t\t\t\t\t{\n\t\t\t\t\t\t\"Playtime\"\t\t\"5\"\n\t\t\t\t\t}\n"
            "\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n}\n"));
        // the other account's numbers must not be used
        write(steam + QStringLiteral("/userdata/101/config/localconfig.vdf"), QStringLiteral(
            "\"UserLocalConfigStore\"\n{\n\t\"Software\"\n\t{\n\t\t\"Valve\"\n\t\t{\n\t\t\t\"Steam\"\n\t\t\t{\n"
            "\t\t\t\t\"apps\"\n\t\t\t\t{\n\t\t\t\t\t\"10\"\n\t\t\t\t\t{\n\t\t\t\t\t\t\"Playtime\"\t\t\"7\"\n\t\t\t\t\t}\n"
            "\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n}\n"));
        qputenv("OWNEET_STEAM_DIR", steam.toUtf8());
    }

    void installed_games_only()
    {
        providers::SearchContext sctx;
        providers::steam::SteamProvider provider;
        provider.run(sctx);

        QVERIFY(sctx.game_by_uri(QStringLiteral("steam:10")));
        QVERIFY(sctx.game_by_uri(QStringLiteral("steam:30")));
        QVERIFY(sctx.game_by_uri(QStringLiteral("steam:40")));
        QVERIFY(!sctx.game_by_uri(QStringLiteral("steam:20")));
        QVERIFY(!sctx.game_by_uri(QStringLiteral("steam:1493710")));
        QVERIFY(!sctx.game_by_uri(QStringLiteral("steam:4183110")));
        QVERIFY(!sctx.game_by_uri(QStringLiteral("steam:228980")));
        QCOMPARE(sctx.game_by_uri(QStringLiteral("steam:40"))->title(), QStringLiteral("Ember \"Run\""));
    }

    void play_time_from_steam()
    {
        providers::SearchContext sctx;
        providers::steam::SteamProvider provider;
        provider.run(sctx);

        // the totals are made when the list is finished, as in the interface
        const auto [collections, games] = sctx.finalize(this);
        const auto by_title = [&games = games](const char* title) -> const model::Game* {
            for (const model::Game* game : games)
                if (game->title() == QLatin1String(title))
                    return game;
            return nullptr;
        };
        const model::Game* game = by_title("Copper Coast");
        QVERIFY(game);
        QCOMPARE(game->playTime(), 90 * 60);
        QCOMPARE(game->lastPlayed(), QDateTime::fromSecsSinceEpoch(1700000500));
        QVERIFY(by_title("Needs An Update"));
        QCOMPARE(by_title("Needs An Update")->playTime(), 0);
    }

    // 4.8: the rating on SteamOS, its reason and the month of Steam's check
    void unsupported_games()
    {
        QTemporaryDir steam;
        manifest(steam.path(), "51", "Installed Anti Cheat", "4");
        const auto write_binary = [&steam](const char* name, const QByteArray& data) {
            QVERIFY(QDir().mkpath(steam.path() + QStringLiteral("/appcache")));
            QFile file(steam.path() + QStringLiteral("/appcache/") + QLatin1String(name));
            QVERIFY(file.open(QIODevice::WriteOnly));
            file.write(data);
        };
        QByteArray ids;
        for (quint32 id = 51; id <= 55; id++)
            ids += number(QByteArray::number(id - 51).constData(), id);
        write_binary("packageinfo.vdf", le32(0x06565528) + le32(1) + le32(3001) + QByteArray(20 + 4 + 8, '\0')
            + block("3001", block("appids", ids)) + char(8) + le32(0xFFFFFFFF));
        write_binary("appinfo.vdf", le32(0x07564428) + le32(1)
            + app(51, "Installed Anti Cheat", compat(1, "#SteamOS_TestResult_UnsupportedAntiCheat_Other", 1665878400))
            + app(52, "No Reason", compat(1, "#SteamOS_TestResult_SteamOSDoesNotSupport", 0))
            + app(53, "Deck Only", compat(2, "#SteamOS_TestResult_UnsupportedAntiCheat_Other", 1665878400))
            + app(54, "Not Known", compat(0, "", 1665878400))
            + app(55, "No Data", QByteArray())
            + le32(0));
        qputenv("OWNEET_STEAM_DIR", steam.path().toUtf8());
        providers::SearchContext sctx;
        providers::steam::SteamProvider provider;
        provider.run(sctx);
        qputenv("OWNEET_STEAM_DIR", m_steam.path().toUtf8());

        const auto extra = [&sctx](const char* uri, const char* name) {
            const model::Game* game = sctx.game_by_uri(QLatin1String(uri));
            return game ? game->extraMap().value(QLatin1String(name)).toString() : QStringLiteral("no game");
        };
        QCOMPARE(extra("steam:51", "steam.unsupported"), QStringLiteral("anticheat"));
        QCOMPARE(extra("steam:51", "steam.unsupported.month"), QStringLiteral("2022-10"));
        QCOMPARE(extra("steam:52", "steam.unsupported"), QStringLiteral("other"));
        QCOMPARE(extra("steam:52", "steam.unsupported.month"), QString());
        QCOMPARE(extra("steam:53", "steam.unsupported"), QString());
        QCOMPARE(extra("steam:54", "steam.unsupported"), QString());
        QCOMPARE(extra("steam:55", "steam.unsupported"), QString());
    }

    void no_steam()
    {
        QTemporaryDir empty;
        qputenv("OWNEET_STEAM_DIR", empty.path().toUtf8());
        providers::SearchContext sctx;
        providers::steam::SteamProvider provider;
        provider.run(sctx);
        QVERIFY(!sctx.game_by_uri(QStringLiteral("steam:10")));
        qputenv("OWNEET_STEAM_DIR", m_steam.path().toUtf8());
    }
};


QTEST_MAIN(test_Steam)
#include "test_Steam.moc"
