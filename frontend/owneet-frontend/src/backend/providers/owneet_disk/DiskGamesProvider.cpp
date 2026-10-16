// SPDX-License-Identifier: GPL-3.0-or-later
//
// Games on a disk ([[step-4.3-local-drm-free-games]], [[games-on-disk]]): a folder named "Games"
// (or its translation, owneet::DiskGames::folderNames) at the top of a mounted disk or in the home
// folder; one folder per game, named after it, with any sub-folders. In each game folder:
//
// - what can start on Linux: ELF programs for x86 with the exec bit, shell scripts (run with bash)
//   and AppImages, searched in the sub-folders too; never uninstallers, configurators,
//   redistributables, crash reporters, installers (makeself / MojoSetup scripts), nor the support
//   and documentation folders;
// - the file to start: the user's choice, else game.txt's "start", else start.sh at the top of
//   the folder (GOG's games), else the only file found; otherwise the theme asks once;
// - the title: game.txt's "title", else the folder's name as the user wrote it (never rewritten);
// - the cover: cover.jpg / cover.png (cover.jpeg, cover.webp) at the top of the folder.
//
// game.txt holds "key: value" lines (title, start, description; "#" starts a comment).
//
// A folder with nothing to start on Linux but programs for Windows is a Windows game
// ([[step-4.5-windows-games-proton]]): it starts with GOG's goggame-<id>.info when there is one
// (its primary play task: file, arguments, working folder; paths matched without case, as
// Windows does), else with the user's choice among its .exe files (the same rules). Folders with
// only an installer (step 4.6) are not listed yet. The extras the theme reads: "disk.dir" (the
// game folder), "disk.root" (the disk's mount folder, "internal" for the home folder),
// "disk.files" (what can start, relative to the game folder), "disk.start" (the file it starts
// with, "" until chosen), "disk.windows" (a Windows game) with "disk.gog.start", "disk.gog.args"
// and "disk.gog.workdir" (GOG's play task).

#include "DiskGamesProvider.h"

#include "Log.h"
#include "Paths.h"
#include "model/gaming/Assets.h"
#include "model/gaming/Collection.h"
#include "model/gaming/Game.h"
#include "owneet/DiskGames.h"
#include "providers/SearchContext.h"
#include "types/AssetType.h"

#include <QDir>
#include <QDirIterator>
#include <QFile>
#include <QFileInfo>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QStorageInfo>
#include <QTextStream>

#include <algorithm>


namespace {
constexpr int MAX_DEPTH = 5;             // sub-folders searched under a game's folder
constexpr int MAX_ENTRIES = 20000;       // files and folders looked at in one game's folder

struct GameTxt {
    QString title;
    QString start;
    QString description;
};

struct Found {
    QStringList linux_files;             // relative to the game folder
    QStringList windows_files;           // the same, programs for Windows
    int installers = 0;
};

// GOG's play task (goggame-<id>.info): the file that starts the game, with its arguments
struct GogTask {
    QString path;                        // relative to the game folder, as found on the disk
    QString args;
    QString workdir;                     // relative to the game folder, "" for the file's folder
};

// The disks to look at: the home folder ("internal") and the mounted disks, as Storage lists
// them; OWNEET_DISKS adds folders that act as disks (previews and tests: "path[:path…]")
std::vector<std::pair<QString, QString>> disk_roots()
{
    std::vector<std::pair<QString, QString>> roots;
    roots.emplace_back(paths::homePath(), QStringLiteral("internal"));
    for (const QStorageInfo& info : QStorageInfo::mountedVolumes()) {
        if (!info.isValid() || !info.isReady())
            continue;
        const QString path = info.rootPath();
        if (path.startsWith(QLatin1String("/run/media/")) || path.startsWith(QLatin1String("/media/"))
                || path.startsWith(QLatin1String("/mnt/")))
            roots.emplace_back(path, path);
    }
    const QStringList extra = qEnvironmentVariable("OWNEET_DISKS").split(QChar(':'), Qt::SkipEmptyParts);
    for (const QString& path : extra)
        roots.emplace_back(QDir::cleanPath(path), QDir::cleanPath(path));
    return roots;
}

GameTxt read_game_txt(const QString& game_dir)
{
    GameTxt out;
    QFile file(game_dir + QStringLiteral("/game.txt"));
    if (!file.open(QIODevice::ReadOnly | QIODevice::Text))
        return out;
    QTextStream stream(&file);
    stream.setCodec("UTF-8");
    while (!stream.atEnd()) {
        const QString line = stream.readLine().trimmed();
        if (line.isEmpty() || line.startsWith(QChar('#')))
            continue;
        const int colon = line.indexOf(QChar(':'));
        if (colon <= 0)
            continue;
        const QString key = line.left(colon).trimmed().toLower();
        const QString value = line.mid(colon + 1).trimmed();
        if (key == QLatin1String("title"))
            out.title = value;
        else if (key == QLatin1String("start"))
            out.start = QDir::cleanPath(value);
        else if (key == QLatin1String("description"))
            out.description = value;
    }
    return out;
}

// Folders that never hold what starts the game
bool skipped_folder(const QString& name)
{
    static const QStringList names {
        QStringLiteral("support"), QStringLiteral("__support"), QStringLiteral("docs"), QStringLiteral("doc"),
        QStringLiteral("manual"), QStringLiteral("manuals"), QStringLiteral("licenses"), QStringLiteral("licences"),
        QStringLiteral("redist"), QStringLiteral("_redist"), QStringLiteral("redistributables"),
        QStringLiteral("_commonredist"), QStringLiteral("commonredist"), QStringLiteral("directx"),
        QStringLiteral("vcredist"), QStringLiteral("dotnet"), QStringLiteral("__installer"),
        QStringLiteral("__redist"),
    };
    const QString lower = name.toLower();
    // Unity's data folder ("<game>_Data") holds libraries and assets only
    return name.startsWith(QChar('.')) || names.contains(lower) || lower.endsWith(QLatin1String("_data"));
}

// Helpers that are never offered: uninstallers, configurators, redistributables, crash reporters
bool helper_name(const QString& base)
{
    static const QStringList parts {
        QStringLiteral("unins"), QStringLiteral("setup"), QStringLiteral("install"), QStringLiteral("config"),
        QStringLiteral("redist"), QStringLiteral("directx"), QStringLiteral("dxsetup"),
        QStringLiteral("crashhandler"), QStringLiteral("crashreport"), QStringLiteral("crash_report"),
        QStringLiteral("help"),
    };
    const QString lower = base.toLower();
    return std::any_of(parts.cbegin(), parts.cend(), [&lower](const QString& p) { return lower.contains(p); });
}

// An ELF program for x86 (64 or 32 bits), not a library
bool is_x86_program(const QString& path)
{
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly))
        return false;
    const QByteArray head = file.read(20);
    if (head.size() < 20 || !head.startsWith("\x7f" "ELF"))
        return false;
    const auto byte = [&head](int i) { return static_cast<unsigned char>(head.at(i)); };
    const bool little = byte(5) == 1;
    const auto half = [&](int i) { return little ? byte(i) | (byte(i + 1) << 8) : (byte(i) << 8) | byte(i + 1); };
    const int type = half(16);           // 2: program, 3: program or library (PIE)
    const int machine = half(18);        // 0x3e: x86-64, 3: i386
    return (type == 2 || type == 3) && (machine == 0x3e || machine == 3);
}

// A self-extracting installer (GOG's .sh, MojoSetup, makeself): step 4.6
bool is_installer_script(const QString& path)
{
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly))
        return false;
    const QByteArray head = file.read(64 * 1024).toLower();
    return head.contains("makeself") || head.contains("mojosetup");
}

void search_folder(const QString& game_dir, const QString& dir, int depth, int& entries, Found& found)
{
    const QFileInfoList list = QDir(dir).entryInfoList(
        QDir::Files | QDir::Dirs | QDir::NoDotAndDotDot, QDir::Name | QDir::DirsLast);
    const QDir base(game_dir);
    for (const QFileInfo& finfo : list) {
        if (++entries > MAX_ENTRIES)
            return;
        if (finfo.isDir()) {
            if (depth < MAX_DEPTH && !finfo.isSymLink() && !skipped_folder(finfo.fileName()))
                search_folder(game_dir, finfo.filePath(), depth + 1, entries, found);
            continue;
        }
        const QString name = finfo.fileName();
        const QString suffix = finfo.suffix().toLower();
        const bool helper = helper_name(finfo.completeBaseName());
        if (suffix == QLatin1String("msi")) {
            found.installers++;
            continue;
        }
        if (suffix == QLatin1String("exe")) {
            if (!helper)
                found.windows_files.append(base.relativeFilePath(finfo.filePath()));
            continue;
        }
        bool starts = false;
        if (suffix == QLatin1String("sh")) {
            if (is_installer_script(finfo.filePath())) {
                found.installers++;
                continue;
            }
            starts = true;
        } else if (suffix == QLatin1String("appimage")) {
            starts = finfo.isExecutable();
        } else if (suffix.isEmpty() || suffix == QLatin1String("x86_64") || suffix == QLatin1String("x86")
                   || suffix == QLatin1String("amd64") || suffix == QLatin1String("bin")) {
            starts = finfo.isExecutable() && is_x86_program(finfo.filePath());
        }
        if (starts && !helper)
            found.linux_files.append(base.relativeFilePath(finfo.filePath()));
    }
}

// A Windows path (backslashes, any case) as the file on the disk, relative to the game folder;
// "" when it is not there
QString find_windows_path(const QString& game_dir, QString path)
{
    path.replace(QChar('\\'), QChar('/'));
    QString found;
    QDir dir(game_dir);
    for (const QString& part : path.split(QChar('/'), Qt::SkipEmptyParts)) {
        if (part == QLatin1String("."))
            continue;
        QString match;
        for (const QString& name : dir.entryList(QDir::AllEntries | QDir::NoDotAndDotDot | QDir::Hidden)) {
            if (name.compare(part, Qt::CaseInsensitive) == 0) {
                match = name;
                break;
            }
        }
        if (match.isEmpty())
            return {};
        found = found.isEmpty() ? match : found + QChar('/') + match;
        dir.setPath(dir.filePath(match));
    }
    return found;
}

// GOG's primary play task, from the goggame-<id>.info files at the top of the game folder
GogTask read_gog_task(const QString& game_dir)
{
    const QStringList infos = QDir(game_dir).entryList({QStringLiteral("goggame-*.info")}, QDir::Files);
    for (const QString& info : infos) {
        QFile file(game_dir + QChar('/') + info);
        if (!file.open(QIODevice::ReadOnly))
            continue;
        const QJsonArray tasks = QJsonDocument::fromJson(file.readAll()).object().value(QStringLiteral("playTasks")).toArray();
        for (const QJsonValue& value : tasks) {
            const QJsonObject task = value.toObject();
            if (!task.value(QStringLiteral("isPrimary")).toBool()
                    || task.value(QStringLiteral("type")).toString() != QLatin1String("FileTask"))
                continue;
            GogTask out;
            out.path = find_windows_path(game_dir, task.value(QStringLiteral("path")).toString());
            if (out.path.isEmpty())
                continue;
            out.args = task.value(QStringLiteral("arguments")).toString();
            const QString workdir = task.value(QStringLiteral("workingDir")).toString();
            if (!workdir.isEmpty())
                out.workdir = find_windows_path(game_dir, workdir);
            return out;
        }
    }
    return {};
}

QString find_cover(const QString& game_dir)
{
    const QStringList names = QDir(game_dir).entryList(QDir::Files);
    for (const char* wanted : {"cover.jpg", "cover.png", "cover.jpeg", "cover.webp"}) {
        for (const QString& name : names) {
            if (name.toLower() == QLatin1String(wanted))
                return game_dir + QChar('/') + name;
        }
    }
    return {};
}
} // namespace


namespace providers {
namespace owneet_disk {

DiskGamesProvider::DiskGamesProvider(QObject* parent)
    : Provider(QLatin1String("owneet_disk"), QStringLiteral("Games on disks"), PROVIDER_FLAG_NONE, parent)
{}

Provider& DiskGamesProvider::run(SearchContext& sctx)
{
    model::Collection* collection = nullptr;
    for (const auto& root : disk_roots()) {
        const QStringList top = QDir(root.first).entryList(QDir::Dirs | QDir::NoDotAndDotDot);
        for (const QString& folder : top) {
            if (!owneet::DiskGames::folderNames().contains(folder.toLower()))
                continue;
            const QString games_dir = root.first + QChar('/') + folder;
            Log::info(display_name(), LOGMSG("Looking for games in `%1`").arg(games_dir));
            const QStringList game_folders = QDir(games_dir).entryList(QDir::Dirs | QDir::NoDotAndDotDot);
            for (const QString& game_folder : game_folders) {
                const QString game_dir = games_dir + QChar('/') + game_folder;
                Found found;
                int entries = 0;
                search_folder(game_dir, game_dir, 0, entries, found);
                // game.txt can name the file to start, even one the search does not find (Windows
                // programs wait for step 4.5)
                const GameTxt txt = read_game_txt(game_dir);
                const QFileInfo txt_file(QDir(game_dir), txt.start);
                const bool txt_start = !txt.start.isEmpty() && txt_file.isFile()
                    && txt_file.suffix().toLower() != QLatin1String("exe") && txt_file.suffix().toLower() != QLatin1String("msi");
                if (txt_start && !found.linux_files.contains(txt.start))
                    found.linux_files.prepend(txt.start);
                // Nothing for Linux: a Windows game, started as GOG says when it can
                const bool windows = found.linux_files.isEmpty() && !found.windows_files.isEmpty();
                const GogTask gog = windows ? read_gog_task(game_dir) : GogTask();
                QStringList& files = windows ? found.windows_files : found.linux_files;
                if (!gog.path.isEmpty())
                    files = QStringList { gog.path };
                if (files.isEmpty()) {
                    Log::info(display_name(), LOGMSG("`%1`: nothing to start (%2 installers): not listed yet")
                        .arg(game_folder).arg(found.installers));
                    continue;
                }
                // The files at the top of the folder first
                std::stable_sort(files.begin(), files.end(), [](const QString& a, const QString& b) {
                    return !a.contains(QChar('/')) && b.contains(QChar('/'));
                });

                // The user's choice first (Options can always change it), then game.txt's
                QString start = owneet::DiskGames::savedChoice(game_dir);
                if (!files.contains(start))
                    start.clear();
                if (start.isEmpty() && txt_start)
                    start = txt.start;
                if (start.isEmpty() && files.contains(QStringLiteral("start.sh")))
                    start = QStringLiteral("start.sh");
                if (start.isEmpty() && files.size() == 1)
                    start = files.first();

                if (!collection)
                    collection = sctx.get_or_create_collection(QStringLiteral("Games on disks"));
                model::Game* game = sctx.create_game_for(*collection);
                sctx.game_add_filepath(*game, game_dir);
                sctx.game_add_to(*game, *collection);
                const QString title = txt.title.isEmpty() ? game_folder : txt.title;
                game->setTitle(title).setSortBy(title);
                if (!txt.description.isEmpty())
                    game->setDescription(txt.description);
                const QString cover = find_cover(game_dir);
                if (!cover.isEmpty())
                    game->assetsMut().add_file(AssetType::BOX_FRONT, cover);
                QVariantMap& extra = game->extraMapMut();
                extra.insert(QStringLiteral("disk.dir"), game_dir);
                extra.insert(QStringLiteral("disk.root"), root.second);
                extra.insert(QStringLiteral("disk.files"), files);
                if (windows) {
                    extra.insert(QStringLiteral("disk.windows"), true);
                    if (!gog.path.isEmpty()) {
                        extra.insert(QStringLiteral("disk.gog.start"), gog.path);
                        extra.insert(QStringLiteral("disk.gog.args"), gog.args);
                        extra.insert(QStringLiteral("disk.gog.workdir"), gog.workdir);
                    }
                }
                owneet::DiskGames::applyChoice(*game, game_dir, start);
                Log::info(display_name(), LOGMSG("`%1`%2: %3 file(s) to start, starts `%4`%5")
                    .arg(title, windows ? QStringLiteral(" (Windows)") : QString())
                    .arg(files.size()).arg(start.isEmpty() ? QStringLiteral("(asks)") : start,
                                           gog.path.isEmpty() ? QString() : QStringLiteral(" (GOG's play task)")));
            }
        }
    }
    return *this;
}

} // namespace owneet_disk
} // namespace providers
