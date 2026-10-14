// SPDX-License-Identifier: GPL-3.0-or-later

#include "SteamOwned.h"

#include "Log.h"

#include <QFile>
#include <QSet>
#include <QVariantMap>
#include <QtEndian>


namespace {
const QString LOG_TAG = QStringLiteral("Steam");

// Steam's binary KeyValues: a type byte, a key (a C string, or in appinfo.vdf version 29 an index
// in the file's table of keys), then the value; a block ends with 0x08.
enum : char {
    KV_BLOCK = 0x00,
    KV_STRING = 0x01,
    KV_INT32 = 0x02,
    KV_UINT64 = 0x07,
    KV_END = 0x08,
};

class Reader {
public:
    explicit Reader(const QByteArray& data) : m_data(data) {}

    bool ok() const { return m_ok; }
    int pos() const { return m_pos; }
    void seek(int pos) { m_ok = m_ok && pos >= 0 && pos <= m_data.size(); m_pos = pos; }
    void skip(int n) { seek(m_pos + n); }

    quint32 u32()
    {
        if (!need(4))
            return 0;
        const quint32 v = qFromLittleEndian<quint32>(m_data.constData() + m_pos);
        m_pos += 4;
        return v;
    }
    qint64 i64()
    {
        if (!need(8))
            return 0;
        const qint64 v = qFromLittleEndian<qint64>(m_data.constData() + m_pos);
        m_pos += 8;
        return v;
    }
    char byte()
    {
        if (!need(1))
            return KV_END;
        return m_data.at(m_pos++);
    }
    QString cstring()
    {
        const int end = m_data.indexOf('\0', m_pos);
        if (end < 0) {
            m_ok = false;
            return {};
        }
        const QString s = QString::fromUtf8(m_data.constData() + m_pos, end - m_pos);
        m_pos = end + 1;
        return s;
    }

    // A block of keys and values, up to its end mark. Keys come lower-cased.
    QVariantMap block(const QStringList* keys, int depth = 0)
    {
        QVariantMap out;
        if (depth > 32) {
            m_ok = false;
            return out;
        }
        while (m_ok) {
            const char type = byte();
            if (type == KV_END)
                return out;
            QString key;
            if (keys) {
                const quint32 i = u32();
                if (i >= quint32(keys->size())) {
                    m_ok = false;
                    return out;
                }
                key = keys->at(int(i));
            } else {
                key = cstring();
            }
            key = key.toLower();
            switch (type) {
                case KV_BLOCK: out.insert(key, block(keys, depth + 1)); break;
                case KV_STRING: out.insert(key, cstring()); break;
                case KV_INT32: out.insert(key, qint32(u32())); break;
                case KV_UINT64: out.insert(key, i64()); break;
                default: m_ok = false; return out;     // a type Steam does not use in these files
            }
        }
        return out;
    }

private:
    const QByteArray& m_data;
    int m_pos = 0;
    bool m_ok = true;

    bool need(int n)
    {
        m_ok = m_ok && m_pos + n <= m_data.size();
        return m_ok;
    }
};

QByteArray read_file(const QString& path)
{
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly))
        return {};
    return file.readAll();
}

// The apps of the account's active licences (package 0 left out)
QSet<QString> licensed_apps(const QString& path)
{
    QSet<QString> apps;
    const QByteArray data = read_file(path);
    Reader r(data);
    const quint32 magic = r.u32();
    r.u32();                                    // universe
    if (!r.ok() || (magic != 0x06565527 && magic != 0x06565528)) {
        if (!data.isEmpty())
            Log::warning(LOG_TAG, LOGMSG("`%1`: unknown format").arg(path));
        return apps;
    }
    while (r.ok()) {
        const quint32 package = r.u32();
        if (package == 0xFFFFFFFF)
            break;
        r.skip(20 + 4 + (magic == 0x06565528 ? 8 : 0));   // hash, change number, token
        const QVariantMap root = r.block(nullptr);
        if (!r.ok())
            break;
        const QVariantMap info = root.isEmpty() ? QVariantMap() : root.first().toMap();
        if (package == 0 || info.value(QStringLiteral("status")).toInt() != 0)
            continue;
        const QVariantMap ids = info.value(QStringLiteral("appids")).toMap();
        for (const QVariant& id : ids)
            apps.insert(QString::number(id.toLongLong()));
    }
    if (!r.ok())
        Log::warning(LOG_TAG, LOGMSG("`%1` could not be read to the end").arg(path));
    return apps;
}
} // namespace


namespace providers {
namespace steam {

std::vector<OwnedApp> read_owned_games(const QString& steam_datadir)
{
    std::vector<OwnedApp> out;
    const QSet<QString> licensed = licensed_apps(steam_datadir + QLatin1String("appcache/packageinfo.vdf"));
    if (licensed.isEmpty())
        return out;

    const QString path = steam_datadir + QLatin1String("appcache/appinfo.vdf");
    const QByteArray data = read_file(path);
    Reader r(data);
    const quint32 magic = r.u32();
    r.u32();                                    // universe
    if (!r.ok() || magic < 0x07564427 || magic > 0x07564429) {
        Log::warning(LOG_TAG, LOGMSG("`%1`: missing or unknown format").arg(path));
        return out;
    }
    // version 29: the keys are kept once, in a table at the end of the file
    QStringList keys;
    const bool keyTable = magic == 0x07564429;
    if (keyTable) {
        const qint64 offset = r.i64();
        const int start = r.pos();
        if (offset <= 0 || offset >= data.size()) {
            Log::warning(LOG_TAG, LOGMSG("`%1`: no table of keys").arg(path));
            return out;
        }
        r.seek(int(offset));
        const quint32 count = r.u32();
        for (quint32 i = 0; i < count && r.ok(); i++)
            keys.append(r.cstring());
        r.seek(start);
    }
    while (r.ok()) {
        const quint32 appid = r.u32();
        if (appid == 0)
            break;
        const quint32 size = r.u32();
        const int next = r.pos() + int(size);
        const QString id = QString::number(appid);
        if (licensed.contains(id)) {
            // state, last updated, token, text hash, change number, binary hash (versions 28+)
            r.skip(4 + 4 + 8 + 20 + 4 + (magic >= 0x07564428 ? 20 : 0));
            const QVariantMap root = r.block(keyTable ? &keys : nullptr);
            const QVariantMap app = root.isEmpty() ? QVariantMap() : root.first().toMap();
            const QVariantMap common = app.value(QStringLiteral("common")).toMap();
            const QString name = common.value(QStringLiteral("name")).toString();
            if (r.ok() && common.value(QStringLiteral("type")).toString().compare(QLatin1String("game"), Qt::CaseInsensitive) == 0
                    && !name.isEmpty())
                out.push_back({id, name});
        }
        r.seek(next);
    }
    if (!r.ok())
        Log::warning(LOG_TAG, LOGMSG("`%1` could not be read to the end").arg(path));
    Log::info(LOG_TAG, LOGMSG("%1 games owned (%2 apps in the licences)")
        .arg(QString::number(out.size()), QString::number(licensed.size())));
    return out;
}

} // namespace steam
} // namespace providers
