// SPDX-License-Identifier: GPL-3.0-or-later
#include "DiskGames.h"

#include "Log.h"
#include "Paths.h"
#include "model/gaming/Game.h"

#include <QCryptographicHash>
#include <QDir>
#include <QFileInfo>
#include <QJsonDocument>
#include <QJsonObject>
#include <QRegularExpression>
#include <QSaveFile>


namespace {
// The user's choices: { "<game folder>": "<file, relative to it>" }
QString choices_path()
{
    return paths::writableConfigDir() + QStringLiteral("/disk-games.json");
}

QJsonObject read_choices()
{
    QFile file(choices_path());
    if (!file.open(QIODevice::ReadOnly))
        return {};
    return QJsonDocument::fromJson(file.readAll()).object();
}

// A path as one argument of a launch command (the tokenizer understands both quotes)
QString quoted(const QString& path)
{
    const QChar quote = path.contains(QChar('"')) ? QChar('\'') : QChar('"');
    return quote + path + quote;
}
} // namespace


namespace owneet {

DiskGames::DiskGames(QObject* parent)
    : QObject(parent)
{}

const QStringList& DiskGames::folderNames()
{
    static const QStringList names {
        QStringLiteral("games"),
        QStringLiteral("giochi"),
    };
    return names;
}

QString DiskGames::savedChoice(const QString& game_dir)
{
    return read_choices().value(game_dir).toString();
}

void DiskGames::applyChoice(model::Game& game, const QString& game_dir, const QString& file)
{
    game.extraMapMut().insert(QStringLiteral("disk.start"), file);
    if (file.isEmpty()) {
        game.setLaunchCmd(QString()).setLaunchWorkdir(QString());
        return;
    }
    const QFileInfo finfo(QDir(game_dir), file);
    const QString path = finfo.absoluteFilePath();
    const QString suffix = finfo.suffix().toLower();
    // A Windows game ([[step-4.5-windows-games-proton]]) starts through owneet-windows-run, with
    // the Proton owneetd has made ready and its own prefix; GOG's play task adds its arguments
    // and working folder
    const QVariantMap& extra = game.extraMap();
    if (extra.value(QStringLiteral("disk.windows")).toBool()) {
        const bool gog = extra.value(QStringLiteral("disk.gog.start")).toString() == file;
        QString cmd = QStringLiteral("/usr/lib/owneet/owneet-windows-run ") + quoted(windowsPrefix(game_dir))
            + QChar(' ') + quoted(path);
        const QString args = gog ? extra.value(QStringLiteral("disk.gog.args")).toString() : QString();
        if (!args.isEmpty())
            cmd += QChar(' ') + args;
        const QString workdir = gog ? extra.value(QStringLiteral("disk.gog.workdir")).toString() : QString();
        game.setLaunchCmd(cmd).setLaunchWorkdir(workdir.isEmpty() ? finfo.absolutePath() : QDir(game_dir).filePath(workdir));
        return;
    }
    // Scripts run with bash and AppImages unpack themselves (no FUSE in the system): neither
    // needs anything written on the disk, which is mounted read only
    QString cmd;
    if (suffix == QLatin1String("sh"))
        cmd = QStringLiteral("/usr/bin/bash ") + quoted(path);
    else if (suffix == QLatin1String("appimage"))
        cmd = QStringLiteral("/usr/bin/env APPIMAGE_EXTRACT_AND_RUN=1 ") + quoted(path);
    else
        cmd = quoted(path);
    game.setLaunchCmd(cmd).setLaunchWorkdir(finfo.absolutePath());
}

void DiskGames::setQuietReading(bool quiet)
{
    if (quiet == m_quiet_reading)
        return;
    m_quiet_reading = quiet;
    emit quietReadingChanged();
}

QString DiskGames::windowsPrefix(const QString& game_dir)
{
    // one prefix per game, in the console's own disk (the game's disk may be read only): a
    // readable name and a short hash of the game's folder
    QString slug = QFileInfo(game_dir).fileName().toLower();
    slug.replace(QRegularExpression(QStringLiteral("[^a-z0-9]+")), QStringLiteral("-"));
    slug = slug.left(40).remove(QRegularExpression(QStringLiteral("^-+|-+$")));
    const QString hash = QString::fromLatin1(QCryptographicHash::hash(game_dir.toUtf8(), QCryptographicHash::Sha1).toHex().left(10));
    return paths::homePath() + QStringLiteral("/.local/share/owneet/windows/prefixes/") + slug + QChar('-') + hash;
}

bool DiskGames::firstStart(QObject* game_obj) const
{
    const auto* game = qobject_cast<model::Game*>(game_obj);
    if (!game || !game->extraMap().value(QStringLiteral("disk.windows")).toBool())
        return false;
    return !QFileInfo::exists(windowsPrefix(game->extraMap().value(QStringLiteral("disk.dir")).toString()) + QStringLiteral("/pfx"));
}

bool DiskGames::hasGames(const QString& root) const
{
    const QStringList dirs = QDir(root).entryList(QDir::Dirs | QDir::NoDotAndDotDot);
    for (const QString& name : dirs) {
        if (folderNames().contains(name.toLower()))
            return true;
    }
    return false;
}

QString DiskGames::chosen(QObject* game_obj) const
{
    const auto* game = qobject_cast<model::Game*>(game_obj);
    return game ? game->extraMap().value(QStringLiteral("disk.start")).toString() : QString();
}

bool DiskGames::choose(QObject* game_obj, const QString& file)
{
    auto* game = qobject_cast<model::Game*>(game_obj);
    if (!game)
        return false;
    const QVariantMap& extra = game->extraMap();
    const QString game_dir = extra.value(QStringLiteral("disk.dir")).toString();
    if (game_dir.isEmpty() || !extra.value(QStringLiteral("disk.files")).toStringList().contains(file))
        return false;

    applyChoice(*game, game_dir, file);

    QJsonObject choices = read_choices();
    choices.insert(game_dir, file);
    QDir().mkpath(paths::writableConfigDir());
    QSaveFile out(choices_path());
    if (!out.open(QIODevice::WriteOnly)) {
        Log::warning(LOGMSG("Cannot save the file chosen for `%1`").arg(game_dir));
        return true;
    }
    out.write(QJsonDocument(choices).toJson());
    if (!out.commit())
        Log::warning(LOGMSG("Cannot save the file chosen for `%1`").arg(game_dir));
    return true;
}

} // namespace owneet
