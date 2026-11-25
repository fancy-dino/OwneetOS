// SPDX-License-Identifier: GPL-3.0-or-later
#include "Emulators.h"

#include "DiskGames.h"
#include "Log.h"
#include "Paths.h"
#include "model/gaming/Game.h"

#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QHash>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QSaveFile>
#include <QSet>
#include <QStorageInfo>

#include <algorithm>


namespace {
constexpr int PROGRAM_DEPTH = 3;             // sub-folders of an emulator's folder
constexpr int MAX_ENTRIES = 4000;            // entries looked at in one emulator's folder
constexpr int MAX_FILES = 40;                // files offered for one game
constexpr qint64 MAX_LIST = 64 * 1024;       // a text file that names the files beside it

// ---- The user's choices: { "<console folder>": { "emulator": id, "told": [id], "options": { id: line },
// "files": { "<game path>": "<file>" } } }
QString choices_path()
{
    return paths::writableConfigDir() + QStringLiteral("/emulators.json");
}

QJsonObject read_choices()
{
    QFile file(choices_path());
    if (!file.open(QIODevice::ReadOnly))
        return {};
    return QJsonDocument::fromJson(file.readAll()).object();
}

QJsonObject choices_of(const QString& console)
{
    return read_choices().value(console).toObject();
}

void write_choices(const QString& console, const QJsonObject& choices)
{
    QJsonObject all = read_choices();
    all.insert(console, choices);
    QDir().mkpath(paths::writableConfigDir());
    QSaveFile out(choices_path());
    if (!out.open(QIODevice::WriteOnly) || out.write(QJsonDocument(all).toJson()) < 0 || !out.commit())
        Log::warning(LOGMSG("Cannot save what was chosen for `%1`").arg(console));
}

QString console_path(QObject* game_obj)
{
    const auto* game = qobject_cast<model::Game*>(game_obj);
    return game ? game->extraMap().value(QStringLiteral("emu.console")).toString() : QString();
}

bool is_archive(const QString& name)
{
    static const QStringList ends {
        QStringLiteral(".zip"), QStringLiteral(".7z"), QStringLiteral(".tar"), QStringLiteral(".tar.gz"), QStringLiteral(".tgz"),
        QStringLiteral(".tar.xz"), QStringLiteral(".txz"), QStringLiteral(".tar.zst"), QStringLiteral(".tar.bz2"),
    };
    const QString lower = name.toLower();
    return std::any_of(ends.cbegin(), ends.cend(), [&lower](const QString& e) { return lower.endsWith(e); });
}

// The name without what only says which kind of file it is
QString bare_name(const QString& name)
{
    QString out = name;
    if (is_archive(out)) {
        static const QStringList ends {
            QStringLiteral(".tar.gz"), QStringLiteral(".tar.xz"), QStringLiteral(".tar.zst"), QStringLiteral(".tar.bz2"),
            QStringLiteral(".zip"), QStringLiteral(".7z"), QStringLiteral(".tar"), QStringLiteral(".tgz"), QStringLiteral(".txz"),
        };
        for (const QString& e : ends) {
            if (out.toLower().endsWith(e))
                return out.left(out.length() - e.length());
        }
    }
    for (const char* e : {".appimage", ".sh", ".x86_64", ".bin"}) {
        if (out.toLower().endsWith(QLatin1String(e)))
            return out.left(out.length() - int(qstrlen(e)));
    }
    return out;
}

// An ELF program for this PC (64 bits or 32), not a library
bool is_program(const QFileInfo& info)
{
    if (!info.isExecutable() || info.fileName().contains(QLatin1String(".so")))
        return false;
    QFile file(info.filePath());
    if (!file.open(QIODevice::ReadOnly))
        return false;
    const QByteArray head = file.read(20);
    if (head.size() < 20 || !head.startsWith("\x7f" "ELF"))
        return false;
    const int type = static_cast<unsigned char>(head.at(16));
    const int machine = static_cast<unsigned char>(head.at(18));
    return (type == 2 || type == 3) && (machine == 0x3e || machine == 3);
}

// 0: nothing to start; higher is better: a program or a script, an archive, an AppImage
int program_rank(const QFileInfo& info)
{
    const QString suffix = info.suffix().toLower();
    if (suffix == QLatin1String("appimage"))
        return 3;
    if (is_archive(info.fileName()))
        return 2;
    if (suffix == QLatin1String("sh"))
        return 1;
    return is_program(info) ? 1 : 0;
}

struct Program {
    QString path;
    int rank;
    int depth;
};

void search_programs(const QString& dir, int depth, int& entries, QVector<Program>& out)
{
    for (const QFileInfo& info : QDir(dir).entryInfoList(QDir::Files | QDir::Dirs | QDir::NoDotAndDotDot, QDir::Name | QDir::DirsLast)) {
        if (++entries > MAX_ENTRIES)
            return;
        if (info.isDir()) {
            // a folder an AppImage made for itself ("<name>.AppImage.home") holds its settings
            if (depth < PROGRAM_DEPTH && !info.isSymLink() && !info.fileName().contains(QLatin1String(".appimage."), Qt::CaseInsensitive))
                search_programs(info.filePath(), depth + 1, entries, out);
            continue;
        }
        const int rank = program_rank(info);
        if (rank > 0)
            out.append({ info.filePath(), rank, depth });
    }
}

// What lies beside games and is never one: texts, pictures, saves
bool beside_game(const QString& name)
{
    static const QSet<QString> suffixes {
        QStringLiteral("txt"), QStringLiteral("nfo"), QStringLiteral("md"), QStringLiteral("pdf"), QStringLiteral("htm"), QStringLiteral("html"),
        QStringLiteral("url"), QStringLiteral("jpg"), QStringLiteral("jpeg"), QStringLiteral("png"), QStringLiteral("webp"), QStringLiteral("gif"),
        QStringLiteral("bmp"), QStringLiteral("ini"), QStringLiteral("cfg"), QStringLiteral("json"), QStringLiteral("xml"), QStringLiteral("log"),
        QStringLiteral("db"), QStringLiteral("sav"), QStringLiteral("save"), QStringLiteral("srm"), QStringLiteral("state"), QStringLiteral("auto"),
        QStringLiteral("bak"), QStringLiteral("tmp"), QStringLiteral("part"),
    };
    return name.startsWith(QChar('.')) || suffixes.contains(QFileInfo(name).suffix().toLower());
}

// The files of a folder that are games: a small text file that names files beside it (a disc's list of
// tracks, a list of discs) stands for them
QFileInfoList game_files(const QString& dir)
{
    QFileInfoList list;
    QSet<QString> names;
    for (const QFileInfo& info : QDir(dir).entryInfoList(QDir::Files | QDir::NoDotAndDotDot, QDir::Name)) {
        if (!beside_game(info.fileName())) {
            list.append(info);
            names.insert(info.fileName());
        }
    }
    QSet<QString> listed;
    for (const QFileInfo& info : list) {
        if (info.size() > MAX_LIST || info.size() == 0)
            continue;
        QFile file(info.filePath());
        if (!file.open(QIODevice::ReadOnly))
            continue;
        const QByteArray data = file.readAll();
        if (data.contains('\0'))
            continue;
        const QString text = QString::fromUtf8(data);
        for (const QString& name : qAsConst(names)) {
            if (name != info.fileName() && text.contains(name, Qt::CaseInsensitive))
                listed.insert(name);
        }
    }
    list.erase(std::remove_if(list.begin(), list.end(), [&listed](const QFileInfo& i) { return listed.contains(i.fileName()); }), list.end());
    return list;
}

// A name as it is compared: lower case, an accent written in one way only (a disk may have it in two)
QString plain(const QString& name)
{
    return name.normalized(QString::NormalizationForm_C).toLower();
}

QString folder_name(const QString& kind, const QString& language)
{
    if (kind == QLatin1String("base")) {
        const QString lang = language.left(2);
        return lang == QLatin1String("it") ? QStringLiteral("Emulazione") : lang == QLatin1String("fr") ? QStringLiteral("Émulation")
             : lang == QLatin1String("es") ? QStringLiteral("Emulación") : QStringLiteral("Emulation");
    }
    static const QHash<QString, QPair<QString, QString>> names {
        { QStringLiteral("en"), { QStringLiteral("Emulators"), QStringLiteral("Games") } },
        { QStringLiteral("it"), { QStringLiteral("Emulatori"), QStringLiteral("Giochi") } },
        { QStringLiteral("de"), { QStringLiteral("Emulatoren"), QStringLiteral("Spiele") } },
        { QStringLiteral("fr"), { QStringLiteral("Émulateurs"), QStringLiteral("Jeux") } },
        { QStringLiteral("es"), { QStringLiteral("Emuladores"), QStringLiteral("Juegos") } },
    };
    const auto pair = names.value(language.left(2), names.value(QStringLiteral("en")));
    return kind == QLatin1String("emulators") ? pair.first : pair.second;
}
} // namespace


namespace owneet {

Emulators::Emulators(QObject* parent)
    : QObject(parent)
{}

const QStringList& Emulators::baseNames()
{
    static const QStringList names {
        QStringLiteral("emulation"), QStringLiteral("emulazione"), QStringLiteral("émulation"), QStringLiteral("emulación"),
        QStringLiteral("emulacion"),
    };
    return names;
}

bool Emulators::isBase(const QString& name)
{
    return baseNames().contains(plain(name));
}

const QStringList& Emulators::emulatorsNames()
{
    static const QStringList names {
        QStringLiteral("emulators"), QStringLiteral("emulatori"), QStringLiteral("emulatoren"),
        QStringLiteral("émulateurs"), QStringLiteral("emulateurs"), QStringLiteral("emuladores"),
    };
    return names;
}

const QStringList& Emulators::gamesNames()
{
    static const QStringList names {
        QStringLiteral("games"), QStringLiteral("giochi"), QStringLiteral("spiele"), QStringLiteral("jeux"), QStringLiteral("juegos"),
    };
    return names;
}

Emulators::ConsoleDir Emulators::consoleDir(const QString& path)
{
    ConsoleDir out;
    const QDir dir(path);
    for (const QString& name : dir.entryList(QDir::Dirs | QDir::NoDotAndDotDot)) {
        const QString lower = plain(name);
        if (out.emulators.isEmpty() && emulatorsNames().contains(lower))
            out.emulators = dir.filePath(name);
        else if (out.games.isEmpty() && gamesNames().contains(lower))
            out.games = dir.filePath(name);
    }
    if (out.emulators.isEmpty() || out.games.isEmpty())
        return {};
    out.path = QDir::cleanPath(path);
    out.name = dir.dirName();
    return out;
}

QVector<Emulators::ConsoleDir> Emulators::consolesOf(const QString& root)
{
    QVector<ConsoleDir> out;
    const QDir top(root);
    for (const QString& base : top.entryList(QDir::Dirs | QDir::NoDotAndDotDot, QDir::Name)) {
        if (!isBase(base))
            continue;
        const QDir dir(top.filePath(base));
        for (const QString& name : dir.entryList(QDir::Dirs | QDir::NoDotAndDotDot, QDir::Name)) {
            const ConsoleDir console = consoleDir(dir.filePath(name));
            if (console.valid())
                out.append(console);
        }
    }
    return out;
}

QVector<Emulators::Emulator> Emulators::emulatorsOf(const ConsoleDir& console)
{
    QVector<Emulator> out;
    const QDir base(console.emulators);
    for (const QFileInfo& info : base.entryInfoList(QDir::Files | QDir::Dirs | QDir::NoDotAndDotDot, QDir::Name)) {
        if (info.fileName().startsWith(QChar('.')))
            continue;
        if (info.isFile()) {
            if (program_rank(info) > 0)
                out.append({ info.fileName(), bare_name(info.fileName()) });
            continue;
        }
        if (info.fileName().contains(QLatin1String(".appimage."), Qt::CaseInsensitive))
            continue;
        // a folder: the best kind in it, at the shallowest place
        QVector<Program> found;
        int entries = 0;
        search_programs(info.filePath(), 0, entries, found);
        int rank = 0, depth = 0;
        for (const Program& p : qAsConst(found)) {
            if (p.rank > rank || (p.rank == rank && p.depth < depth)) {
                rank = p.rank;
                depth = p.depth;
            }
        }
        found.erase(std::remove_if(found.begin(), found.end(), [rank, depth](const Program& p) { return p.rank != rank || p.depth != depth; }), found.end());
        for (const Program& p : qAsConst(found)) {
            const QString file = bare_name(QFileInfo(p.path).fileName());
            out.append({ base.relativeFilePath(p.path), found.size() == 1 ? info.fileName() : info.fileName() + QStringLiteral(": ") + file });
        }
    }
    return out;
}

QVector<Emulators::Title> Emulators::titlesOf(const ConsoleDir& console)
{
    QVector<Title> out;
    // files with the same name and another extension are one game, whose file is asked once
    QHash<QString, int> byName;
    for (const QFileInfo& info : game_files(console.games)) {
        const QString name = info.completeBaseName();
        if (byName.contains(name)) {
            Title& title = out[byName.value(name)];
            title.files.append(info.filePath());
            continue;
        }
        byName.insert(name, out.size());
        out.append({ info.filePath(), name, QStringList(info.filePath()) });
    }
    for (const QFileInfo& info : QDir(console.games).entryInfoList(QDir::Dirs | QDir::NoDotAndDotDot, QDir::Name)) {
        if (info.fileName().startsWith(QChar('.')))
            continue;
        QStringList files;
        for (const QFileInfo& file : game_files(info.filePath()))
            files.append(file.filePath());
        // nothing at its top, or too much to choose from: the folder itself is the game
        if (files.isEmpty() || files.size() > MAX_FILES)
            files = QStringList(info.filePath());
        out.append({ info.filePath(), info.fileName(), files });
    }
    return out;
}

QStringList Emulators::splitOptions(const QString& line)
{
    QStringList out;
    QString word;
    bool quoted = false, any = false;
    for (const QChar c : line) {
        if (c == QChar('"')) {
            quoted = !quoted;
            any = true;                     // "" is an empty option
        } else if (c.isSpace() && !quoted) {
            if (any || !word.isEmpty())
                out.append(word);
            word.clear();
            any = false;
        } else {
            word.append(c);
        }
    }
    if (any || !word.isEmpty())
        out.append(word);
    return out;
}

QStringList Emulators::command(const QString& emulator, const QStringList& options, const QString& file)
{
    // one word with everything in it: no quote or brace of the user's is read by anyone on the way
    QStringList words { DiskGames::gameDataDir(QStringLiteral("emulators"), emulator), emulator, file };
    words += options;
    const QByteArray data = words.join(QChar('\0')).toUtf8();
    return { QStringLiteral("/usr/lib/owneet/owneet-emulator-run"), QString::fromLatin1(data.toBase64()) };
}

QVariantList Emulators::list(QObject* game) const
{
    QVariantList out;
    const ConsoleDir console = consoleDir(console_path(game));
    if (console.valid()) {
        for (const Emulator& e : emulatorsOf(console))
            out.append(QVariantMap { { QStringLiteral("id"), e.id }, { QStringLiteral("name"), e.name } });
    }
    return out;
}

QString Emulators::folder(QObject* game) const
{
    return QFileInfo(consoleDir(console_path(game)).emulators).fileName();
}

QString Emulators::chosen(QObject* game) const
{
    const ConsoleDir console = consoleDir(console_path(game));
    if (!console.valid())
        return {};
    const QVector<Emulator> all = emulatorsOf(console);
    if (all.size() == 1)
        return all.first().id;
    const QString id = choices_of(console.path).value(QStringLiteral("emulator")).toString();
    return std::any_of(all.cbegin(), all.cend(), [&id](const Emulator& e) { return e.id == id; }) ? id : QString();
}

void Emulators::choose(QObject* game, const QString& id)
{
    const QString console = console_path(game);
    QJsonObject choices = choices_of(console);
    choices.insert(QStringLiteral("emulator"), id);
    write_choices(console, choices);
}

bool Emulators::told(QObject* game, const QString& id) const
{
    return choices_of(console_path(game)).value(QStringLiteral("told")).toArray().contains(id);
}

void Emulators::setTold(QObject* game, const QString& id)
{
    const QString console = console_path(game);
    QJsonObject choices = choices_of(console);
    QJsonArray ids = choices.value(QStringLiteral("told")).toArray();
    if (ids.contains(id))
        return;
    ids.append(id);
    choices.insert(QStringLiteral("told"), ids);
    write_choices(console, choices);
}

QString Emulators::options(QObject* game, const QString& id) const
{
    return choices_of(console_path(game)).value(QStringLiteral("options")).toObject().value(id).toString();
}

void Emulators::setOptions(QObject* game, const QString& id, const QString& line)
{
    const QString console = console_path(game);
    QJsonObject choices = choices_of(console);
    QJsonObject all = choices.value(QStringLiteral("options")).toObject();
    all.insert(id, line.trimmed());
    choices.insert(QStringLiteral("options"), all);
    write_choices(console, choices);
}

QStringList Emulators::files(QObject* game_obj) const
{
    const auto* game = qobject_cast<model::Game*>(game_obj);
    return game ? game->extraMap().value(QStringLiteral("emu.files")).toStringList() : QStringList();
}

QString Emulators::file(QObject* game_obj) const
{
    const auto* game = qobject_cast<model::Game*>(game_obj);
    const QStringList all = files(game_obj);
    if (!game || all.isEmpty())
        return {};
    if (all.size() == 1)
        return all.first();
    const QString path = game->extraMap().value(QStringLiteral("emu.path")).toString();
    const QString name = choices_of(console_path(game_obj)).value(QStringLiteral("files")).toObject().value(path).toString();
    return all.contains(name) ? name : QString();
}

void Emulators::chooseFile(QObject* game_obj, const QString& file)
{
    const auto* game = qobject_cast<model::Game*>(game_obj);
    if (!game || !files(game_obj).contains(file))
        return;
    const QString console = console_path(game_obj);
    QJsonObject choices = choices_of(console);
    QJsonObject all = choices.value(QStringLiteral("files")).toObject();
    all.insert(game->extraMap().value(QStringLiteral("emu.path")).toString(), file);
    choices.insert(QStringLiteral("files"), all);
    write_choices(console, choices);
}

bool Emulators::prepare(QObject* game_obj) const
{
    auto* game = qobject_cast<model::Game*>(game_obj);
    const ConsoleDir console = consoleDir(console_path(game_obj));
    const QString id = chosen(game_obj), start = file(game_obj);
    if (!game || !console.valid() || id.isEmpty() || start.isEmpty())
        return false;
    const QString emulator = QDir(console.emulators).filePath(id);
    const QStringList cmd = command(emulator, splitOptions(options(game_obj, id)), start);
    game->setLaunchCmd(cmd.join(QChar(' '))).setLaunchWorkdir(QFileInfo(emulator).absolutePath());
    return true;
}

QVariantMap Emulators::openCommand(QObject* game, const QString& id) const
{
    const ConsoleDir console = consoleDir(console_path(game));
    if (!console.valid() || id.isEmpty())
        return {};
    const QString emulator = QDir(console.emulators).filePath(id);
    return { { QStringLiteral("command"), command(emulator, {}, QString()) },
             { QStringLiteral("workdir"), QFileInfo(emulator).absolutePath() } };
}

bool Emulators::nameTaken(const QString& name, const QString& root) const
{
    const QDir top(root == QLatin1String("internal") ? paths::homePath() : root);
    for (const QString& base : top.entryList(QDir::Dirs | QDir::NoDotAndDotDot)) {
        if (!isBase(base))
            continue;
        for (const QString& other : QDir(top.filePath(base)).entryList(QDir::AllEntries | QDir::Hidden | QDir::NoDotAndDotDot)) {
            if (plain(other) == plain(name))
                return true;
        }
    }
    return false;
}

QVariantMap Emulators::create(const QString& name, const QString& root, const QString& language) const
{
    // a name every disk takes, also one written by Windows
    QString folder = DiskGames::installFolderName(name);
    for (const QChar c : QStringLiteral("/\\<>\"|?*"))
        folder.replace(c, QChar(' '));
    folder = folder.simplified();
    while (folder.startsWith(QChar('.')) || folder.endsWith(QChar('.')))
        folder = (folder.startsWith(QChar('.')) ? folder.mid(1) : folder.chopped(1)).trimmed();
    const QString top = root == QLatin1String("internal") ? paths::homePath() : root;
    const QVariantMap failed { { QStringLiteral("ok"), false } };
    if (folder.isEmpty() || !QFileInfo(top).isDir() || (root != QLatin1String("internal") && QStorageInfo(top).isReadOnly()))
        return failed;
    const QDir disk(top);
    QString base;
    for (const QString& dir : disk.entryList(QDir::Dirs | QDir::NoDotAndDotDot, QDir::Name)) {
        if (isBase(dir)) {
            base = dir;
            break;
        }
    }
    if (base.isEmpty())
        base = folder_name(QStringLiteral("base"), language);
    const QString path = disk.filePath(base) + QChar('/') + folder;
    ConsoleDir console = consoleDir(path);          // there already: nothing is made twice
    if (!console.valid()) {
        const QString emus = folder_name(QStringLiteral("emulators"), language), titles = folder_name(QStringLiteral("games"), language);
        if (!QDir().mkpath(path + QChar('/') + emus) || !QDir().mkpath(path + QChar('/') + titles)) {
            Log::warning(LOGMSG("Cannot make the folders in `%1`").arg(path));
            return failed;
        }
        console = consoleDir(path);
        if (!console.valid())
            return failed;
    }
    return { { QStringLiteral("ok"), true }, { QStringLiteral("name"), console.name }, { QStringLiteral("base"), base },
             { QStringLiteral("emulators"), QFileInfo(console.emulators).fileName() },
             { QStringLiteral("titles"), QFileInfo(console.games).fileName() } };
}

} // namespace owneet
