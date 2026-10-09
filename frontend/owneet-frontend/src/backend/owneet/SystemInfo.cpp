// SPDX-License-Identifier: GPL-3.0-or-later
#include "SystemInfo.h"

#include <QDir>
#include <QFile>
#include <QRegularExpression>
#include <QTextStream>
#include <QVariantMap>

#include <algorithm>


namespace owneet {

namespace {
QStringList read_lines(const QString& path)
{
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly | QIODevice::Text))
        return {};
    QTextStream stream(&file);
    stream.setCodec("UTF-8");
    return stream.readAll().split(QLatin1Char('\n'));
}

// "[Name](https://…) and driver" → "Name and driver"; `code` → code
QString plain(QString cell)
{
    static const QRegularExpression link(QStringLiteral("\\[([^\\]]*)\\]\\([^)]*\\)"));
    cell.replace(link, QStringLiteral("\\1"));
    cell.remove(QLatin1Char('`'));
    return cell.trimmed();
}
} // namespace


SystemInfo::SystemInfo(QObject* parent)
    : QObject(parent)
{}

QString SystemInfo::version() const
{
    for (const QString& line : read_lines(QStringLiteral("/etc/os-release"))) {
        if (line.startsWith(QLatin1String("IMAGE_VERSION=")))
            return line.mid(14).remove(QLatin1Char('"')).trimmed();
    }
    return QString();
}

QVariantList SystemInfo::credits() const
{
    QString path = qEnvironmentVariable("OWNEET_CREDITS");
    if (path.isEmpty())
        path = QStringLiteral("/usr/share/doc/owneetos/CREDITS.md");

    QVariantList out;
    for (const QString& line : read_lines(path)) {
        if (!line.startsWith(QLatin1String("| ")))
            continue;
        const QStringList cells = line.split(QLatin1Char('|'));
        if (cells.size() < 5)
            continue;
        const QString name = plain(cells.at(1));
        if (name.isEmpty() || name == QLatin1String("Project") || name.startsWith(QLatin1String("---")))
            continue;
        out.append(QVariantMap {
            {QStringLiteral("name"), name},
            {QStringLiteral("who"), plain(cells.at(2))},
            {QStringLiteral("licence"), plain(cells.at(3))},
        });
    }
    return out;
}

QVariantList SystemInfo::packages() const
{
    // Each package has /var/lib/pacman/local/NAME-VERSION/desc with %NAME%, %VERSION%, %LICENSE%
    struct Package { QString name, version, licence; };
    std::vector<Package> list;
    const QDir local(QStringLiteral("/var/lib/pacman/local"));
    for (const QString& dir : local.entryList(QDir::Dirs | QDir::NoDotAndDotDot)) {
        const QStringList lines = read_lines(local.filePath(dir + QStringLiteral("/desc")));
        Package p;
        QStringList licences;
        QString section;
        for (const QString& line : lines) {
            if (line.startsWith(QLatin1Char('%'))) {
                section = line;
                continue;
            }
            if (line.isEmpty())
                continue;
            if (section == QLatin1String("%NAME%")) p.name = line;
            else if (section == QLatin1String("%VERSION%")) p.version = line;
            else if (section == QLatin1String("%LICENSE%")) licences << line;
        }
        if (p.name.isEmpty())
            continue;
        p.licence = licences.join(QStringLiteral(", "));
        list.push_back(std::move(p));
    }
    std::sort(list.begin(), list.end(), [](const Package& a, const Package& b) { return a.name < b.name; });

    QVariantList out;
    for (const Package& p : list) {
        out.append(QVariantMap {
            {QStringLiteral("name"), p.name},
            {QStringLiteral("version"), p.version},
            {QStringLiteral("licence"), p.licence},
        });
    }
    return out;
}

} // namespace owneet
