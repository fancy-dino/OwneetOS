// SPDX-License-Identifier: GPL-3.0-or-later
#include "DiskGames.h"

#include "Log.h"
#include "Paths.h"
#include "model/gaming/Game.h"

#include <QCryptographicHash>
#include <QDir>
#include <QDirIterator>
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

// An ELF program (not a script)
bool is_elf(const QString& path)
{
    QFile file(path);
    return file.open(QIODevice::ReadOnly) && file.read(4) == QByteArray("\x7f" "ELF");
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
    // a known game's launcher (owner, 2026-11-12): the OwneetOS library it needs, and whether it
    // hands over to the game (the app then lasts while any of its programs runs)
    const QVariantMap launcher = game.extraMap().value(QStringLiteral("disk.launchers")).toMap().value(file).toMap();
    game.extraMapMut().insert(QStringLiteral("disk.handover"), launcher.value(QStringLiteral("hands-over")).toBool());
    const QString compat = compatLibrary(launcher.value(QStringLiteral("compat")).toString());
    // the library the game the launcher starts gets (the launcher's library hands it over)
    const QString gameCompat = compat.isEmpty() ? QString() : compatLibrary(launcher.value(QStringLiteral("game-compat")).toString());
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
    // A GOG DOS game starts with Linux's DOSBox through owneet-dos-run (a copy on the console's
    // disk, GOG's configuration files)
    if (extra.value(QStringLiteral("disk.dos")).toBool() && extra.value(QStringLiteral("disk.gog.start")).toString() == file) {
        QString cmd = QStringLiteral("/usr/lib/owneet/owneet-dos-run ") + quoted(gameDataDir(QStringLiteral("dos"), game_dir))
            + QChar(' ') + quoted(game_dir) + QChar(' ') + quoted(extra.value(QStringLiteral("disk.gog.workdir")).toString());
        const QString args = extra.value(QStringLiteral("disk.gog.args")).toString();
        if (!args.isEmpty())
            cmd += QChar(' ') + args;
        game.setLaunchCmd(cmd).setLaunchWorkdir(game_dir);
        return;
    }
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
    // A program starts with its own folder among the libraries' folders, as GOG's start.sh does
    // (The Witcher 2's libopenal-eon.so.1 is next to it; owner, 2026-11-11), unless the folder's
    // name has ":", the separator of that list
    QString cmd;
    if (suffix == QLatin1String("sh"))
        cmd = QStringLiteral("/usr/bin/bash ") + quoted(path);
    else if (suffix == QLatin1String("appimage"))
        cmd = QStringLiteral("/usr/bin/env APPIMAGE_EXTRACT_AND_RUN=1 ") + quoted(path);
    else if (is_elf(path) && !finfo.absolutePath().contains(QChar(':')))
        cmd = QStringLiteral("/usr/bin/env ")
            + (compat.isEmpty() ? QString() : quoted(QStringLiteral("LD_PRELOAD=") + compat) + QChar(' '))
            + (gameCompat.isEmpty() ? QString() : quoted(QStringLiteral("OWNEET_GAME_PRELOAD=") + gameCompat) + QChar(' '))
            + quoted(QStringLiteral("LD_LIBRARY_PATH=") + finfo.absolutePath()) + QChar(' ') + quoted(path);
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
    return gameDataDir(QStringLiteral("windows/prefixes"), game_dir);
}

QString DiskGames::gameDataDir(const QString& kind, const QString& game_dir)
{
    // one folder per game, in the console's own disk (the game's disk may be read only): a
    // readable name and a short hash of the game's folder
    QString slug = QFileInfo(game_dir).fileName().toLower();
    slug.replace(QRegularExpression(QStringLiteral("[^a-z0-9]+")), QStringLiteral("-"));
    slug = slug.left(40).remove(QRegularExpression(QStringLiteral("^-+|-+$")));
    const QString hash = QString::fromLatin1(QCryptographicHash::hash(game_dir.toUtf8(), QCryptographicHash::Sha1).toHex().left(10));
    return paths::homePath() + QStringLiteral("/.local/share/owneet/") + kind + QChar('/') + slug + QChar('-') + hash;
}

QString DiskGames::compatLibrary(const QString& name)
{
    static const QRegularExpression valid(QStringLiteral("^[a-z0-9-]+$"));
    if (!valid.match(name).hasMatch())
        return {};
    const QString path = QStringLiteral("/usr/lib32/owneet/libowneet-") + name + QStringLiteral(".so");
    if (!QFileInfo::exists(path)) {
        Log::warning(LOGMSG("OwneetOS's library `%1` is not installed").arg(path));
        return {};
    }
    return path;
}

QString DiskGames::installFolderName(const QString& name)
{
    static const QRegularExpression colons(QStringLiteral("\\s*:\\s*"));
    return QString(name).replace(colons, QStringLiteral(" - ")).trimmed();
}

QVariantMap DiskGames::installRequest(QObject* game_obj, const QString& language) const
{
    const auto* game = qobject_cast<model::Game*>(game_obj);
    if (!game)
        return {};
    const QVariantMap& extra = game->extraMap();
    const QString installer = extra.value(QStringLiteral("disk.installer")).toString();
    const QString kind = extra.value(QStringLiteral("disk.installer.kind")).toString();
    const QString game_dir = extra.value(QStringLiteral("disk.dir")).toString();
    if (installer.isEmpty() || game_dir.isEmpty())
        return {};
    QString dest;
    const QString folder = installFolderName(QFileInfo(game_dir).fileName());
    if (extra.value(QStringLiteral("disk.root")).toString() == QLatin1String("internal")) {
        // already on the console's disk: installed in place (owneetd first renames a folder whose
        // name has ":")
        dest = QFileInfo(game_dir).absolutePath() + QChar('/') + folder;
    } else {
        // the home folder's games folder, or a new one named in the console's language
        QString games_dir;
        const QDir home(paths::homePath());
        for (const QString& folder : home.entryList(QDir::Dirs | QDir::NoDotAndDotDot)) {
            if (folderNames().contains(folder.toLower())) {
                games_dir = home.filePath(folder);
                break;
            }
        }
        if (games_dir.isEmpty())
            games_dir = home.filePath(language.startsWith(QLatin1String("it")) ? QStringLiteral("Giochi") : QStringLiteral("Games"));
        dest = games_dir + QChar('/') + folder;
    }
    QVariantMap out {
        { QStringLiteral("installer"), installer },
        { QStringLiteral("kind"), kind },
        { QStringLiteral("dest"), dest },
        { QStringLiteral("title"), game->title() },
    };
    if (kind != QLatin1String("sh"))
        out.insert(QStringLiteral("prefix"), windowsPrefix(dest));
    return out;
}

double DiskGames::folderSize(const QString& path) const
{
    qint64 bytes = 0;
    QDirIterator it(path, QDir::Files | QDir::Hidden | QDir::NoSymLinks, QDirIterator::Subdirectories);
    while (it.hasNext()) {
        it.next();
        bytes += it.fileInfo().size();
    }
    return static_cast<double>(bytes);
}

double DiskGames::installerSize(const QString& installer, const QString& game_dir) const
{
    const QFileInfo info(installer);
    if (installer.isEmpty() || !info.isFile())
        return 0;
    if (QDir::cleanPath(info.absolutePath()) != QDir::cleanPath(game_dir))
        return folderSize(info.absolutePath());     // a folder of its own: it goes with the installer
    double bytes = static_cast<double>(info.size());
    const QDir dir = info.absoluteDir();
    const QStringList parts = dir.entryList({ info.completeBaseName() + QStringLiteral("-*.bin") }, QDir::Files);
    for (const QString& part : parts)
        bytes += static_cast<double>(QFileInfo(dir.filePath(part)).size());
    return bytes;
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
