// SPDX-License-Identifier: GPL-3.0-or-later

#include "SteamPlaytime.h"

#include <QFile>
#include <QTextStream>


namespace {
// One token: a quoted string (with \" and \\ escapes), "{" or "}"; comments start with //
struct Tokens {
    const QString& text;
    int pos = 0;

    // false at the end; `quoted` tells a string from a brace
    bool next(QString& out, bool& quoted)
    {
        while (pos < text.size()) {
            const QChar c = text.at(pos);
            if (c.isSpace()) {
                pos++;
            } else if (c == QLatin1Char('/') && pos + 1 < text.size() && text.at(pos + 1) == QLatin1Char('/')) {
                while (pos < text.size() && text.at(pos) != QLatin1Char('\n'))
                    pos++;
            } else {
                break;
            }
        }
        if (pos >= text.size())
            return false;

        const QChar c = text.at(pos);
        if (c == QLatin1Char('{') || c == QLatin1Char('}')) {
            out = c;
            quoted = false;
            pos++;
            return true;
        }
        out.clear();
        quoted = true;
        if (c != QLatin1Char('"')) {            // an unquoted word
            while (pos < text.size() && !text.at(pos).isSpace() && text.at(pos) != QLatin1Char('{')
                   && text.at(pos) != QLatin1Char('}') && text.at(pos) != QLatin1Char('"'))
                out += text.at(pos++);
            return true;
        }
        pos++;
        while (pos < text.size() && text.at(pos) != QLatin1Char('"')) {
            if (text.at(pos) == QLatin1Char('\\') && pos + 1 < text.size())
                pos++;
            out += text.at(pos++);
        }
        pos++;                                  // the closing quote
        return true;
    }
};

QVariantMap parse_block(Tokens& tokens)
{
    QVariantMap map;
    QString key;
    QString token;
    bool quoted = false;
    while (tokens.next(token, quoted)) {
        if (!quoted && token == QLatin1String("}"))
            break;
        if (!quoted)
            continue;                           // a brace where a key belongs: skip it
        key = token.toLower();
        if (!tokens.next(token, quoted))
            break;
        if (quoted)
            map.insert(key, token);
        else if (token == QLatin1String("{"))
            map.insert(key, parse_block(tokens));
    }
    return map;
}

QVariantMap child(const QVariantMap& map, const char* key)
{
    return map.value(QLatin1String(key)).toMap();
}

// SteamID64 → the account id that names its userdata folder
QString account_id(const QString& steam_id64)
{
    bool ok = false;
    const quint64 id = steam_id64.toULongLong(&ok);
    return ok ? QString::number(id & 0xFFFFFFFFu) : QString();
}
} // namespace


namespace providers {
namespace steam {

QVariantMap parse_vdf(const QString& text)
{
    Tokens tokens { text };
    return parse_block(tokens);
}

QVariantMap read_vdf_file(const QString& path)
{
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly | QIODevice::Text))
        return {};
    QTextStream stream(&file);
    stream.setCodec("UTF-8");
    return parse_vdf(stream.readAll());
}

HashMap<QString, PlayStats> read_playtime(const QString& steam_datadir)
{
    HashMap<QString, PlayStats> result;

    const QVariantMap users = child(read_vdf_file(steam_datadir + QLatin1String("config/loginusers.vdf")), "users");
    // "MostRecent" "1" in older clients; otherwise the latest login ("Timestamp")
    QString account;
    qint64 latest = -1;
    for (auto it = users.cbegin(); it != users.cend(); ++it) {
        const QVariantMap user = it.value().toMap();
        if (user.value(QLatin1String("mostrecent")).toString() == QLatin1String("1")) {
            account = account_id(it.key());
            break;
        }
        const qint64 stamp = user.value(QLatin1String("timestamp")).toLongLong();
        if (stamp > latest) {
            latest = stamp;
            account = account_id(it.key());
        }
    }
    if (account.isEmpty())
        return result;

    const QVariantMap config = read_vdf_file(
        steam_datadir + QLatin1String("userdata/") + account + QLatin1String("/config/localconfig.vdf"));
    const QVariantMap apps = child(child(child(child(child(config, "userlocalconfigstore"), "software"), "valve"), "steam"), "apps");
    for (auto it = apps.cbegin(); it != apps.cend(); ++it) {
        const QVariantMap app = it.value().toMap();
        PlayStats stats;
        stats.seconds = app.value(QLatin1String("playtime")).toLongLong() * 60;
        const qint64 last = app.value(QLatin1String("lastplayed")).toLongLong();
        if (last > 0)
            stats.last_played = QDateTime::fromSecsSinceEpoch(last);
        for (auto entry = app.cbegin(); entry != app.cend(); ++entry)
            if (entry.key().contains(QLatin1String("_eula_")))
                stats.eulas.append(entry.key());
        if (stats.seconds > 0 || stats.last_played.isValid() || !stats.eulas.isEmpty())
            result.emplace(it.key(), stats);
    }
    return result;
}

} // namespace steam
} // namespace providers
