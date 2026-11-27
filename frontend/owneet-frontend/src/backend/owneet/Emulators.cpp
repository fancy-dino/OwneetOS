// SPDX-License-Identifier: GPL-3.0-or-later
#include "Emulators.h"

#include "DiskGames.h"
#include "Log.h"
#include "Paths.h"
#include "model/gaming/Game.h"

#ifdef WITH_SDL_GAMEPAD
#include <SDL.h>
#endif

#include <QDir>
#include <QDirIterator>
#include <QFile>
#include <QFileInfo>
#include <QCryptographicHash>
#include <QHash>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QRegularExpression>
#include <QSaveFile>
#include <QSet>
#include <QStorageInfo>

#include <algorithm>


namespace {
constexpr int PROGRAM_DEPTH = 3;             // sub-folders of an emulator's folder
constexpr int MAX_ENTRIES = 4000;            // entries looked at in one emulator's folder
constexpr int MAX_FILES = 40;                // files offered for one game
constexpr qint64 MAX_LIST = 64 * 1024;       // a text file that names the files beside it
constexpr int GAME_DEPTH = 3;                // folders looked into for the game's own folder

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

// The processor an ELF program is made for, 0 when it is none
int program_machine(const QString& path)
{
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly))
        return 0;
    const QByteArray head = file.read(20);
    if (head.size() < 20 || !head.startsWith("\x7f" "ELF"))
        return 0;
    const int type = static_cast<unsigned char>(head.at(16));
    return type == 2 || type == 3 ? static_cast<unsigned char>(head.at(18)) : 0;
}

// For this PC: 64 bits or 32
bool fits_machine(int machine) { return machine == 0x3e || machine == 3; }

// An ELF program, not a library
bool may_be_program(const QFileInfo& info)
{
    return info.isExecutable() && !info.fileName().contains(QLatin1String(".so"));
}

bool is_program(const QFileInfo& info)
{
    return may_be_program(info) && fits_machine(program_machine(info.filePath()));
}

// A program for another kind of PC (ARM, RISC-V)
bool is_other_program(const QFileInfo& info)
{
    if (!may_be_program(info) || info.suffix().toLower() == QLatin1String("elf"))
        return false;
    const int machine = program_machine(info.filePath());
    return machine == 0xb7 || machine == 0x28 || machine == 0xf3;
}

// Whether what is listed can run here: an AppImage or a program made for another kind of processor cannot
bool fits(const QString& path)
{
    const QString suffix = QFileInfo(path).suffix().toLower();
    if (is_archive(path) || suffix == QLatin1String("sh"))
        return true;
    const int machine = program_machine(path);
    return machine == 0 || fits_machine(machine);
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

// `others`: programs for another kind of processor
void search_programs(const QString& dir, int depth, int& entries, QVector<Program>& out, QVector<Program>& others)
{
    for (const QFileInfo& info : QDir(dir).entryInfoList(QDir::Files | QDir::Dirs | QDir::NoDotAndDotDot, QDir::Name | QDir::DirsLast)) {
        if (++entries > MAX_ENTRIES)
            return;
        if (info.isDir()) {
            // a folder an AppImage made for itself ("<name>.AppImage.home") holds its settings
            if (depth < PROGRAM_DEPTH && !info.isSymLink() && !info.fileName().contains(QLatin1String(".appimage."), Qt::CaseInsensitive))
                search_programs(info.filePath(), depth + 1, entries, out, others);
            continue;
        }
        const int rank = program_rank(info);
        if (rank > 0)
            out.append({ info.filePath(), rank, depth });
        else if (is_other_program(info))
            others.append({ info.filePath(), 1, depth });
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

// A name as it is compared between a game's folder and the one in it: letters and digits only, without
// what is in brackets ("Ember Run (Europe)" is "Ember Run")
QString title_key(const QString& name)
{
    QString out;
    int brackets = 0;
    for (const QChar c : name.normalized(QString::NormalizationForm_C).toLower()) {
        if (c == QChar('(') || c == QChar('['))
            brackets++;
        else if (c == QChar(')') || c == QChar(']'))
            brackets = std::max(0, brackets - 1);
        else if (brackets == 0 && c.isLetterOrNumber())
            out.append(c);
    }
    return out;
}

QFileInfoList game_folders(const QString& dir)
{
    QFileInfoList list = QDir(dir).entryInfoList(QDir::Dirs | QDir::NoDotAndDotDot, QDir::Name);
    list.erase(std::remove_if(list.begin(), list.end(), [](const QFileInfo& i) {
        return i.fileName().startsWith(QChar('.')) || i.isSymLink();
    }), list.end());
    return list;
}

// What an emulator may be given for a game's folder. `sure`: the first one, without asking
struct Start {
    QStringList files;
    bool sure;
};

Start start_of(const QString& dir, int depth)
{
    QStringList files;
    for (const QFileInfo& file : game_files(dir))
        files.append(file.filePath());
    const QFileInfoList folders = game_folders(dir);
    // nothing but one folder with the game's own name (what an archive leaves): it is looked into. With
    // any other name the folder stays the game, as it is: it may be the part of a game that an emulator
    // looks for in the folder above.
    if (files.isEmpty() && folders.size() == 1 && depth < GAME_DEPTH) {
        const QString key = title_key(QFileInfo(dir).fileName());
        if (!key.isEmpty() && key == title_key(folders.first().fileName())) {
            const QString inner = folders.first().filePath();
            Start start = start_of(inner, depth + 1);
            // files beside folders: either may be the game, the user is asked once
            if (!start.files.contains(inner) && !game_folders(inner).isEmpty()) {
                start.files.append(inner);
                start.sure = false;
            }
            start.files.append(dir);
            return start;
        }
    }
    // nothing at its top, or too much to choose from: the folder itself is the game
    if (files.isEmpty() || files.size() > MAX_FILES)
        return { QStringList(dir), true };
    return { files, files.size() == 1 };
}

// A name as it is compared: lower case, an accent written in one way only (a disk may have it in two)
QString plain(const QString& name)
{
    return name.normalized(QString::NormalizationForm_C).toLower();
}

// ---- What the console knows of an emulator it recognises: its card (frontend/emulator-cards/README.md).
// The cards are kept out of git until the legal review (owner, 2026-11-26); without one, nothing changes.
QStringList card_dirs()
{
    const QString env = qEnvironmentVariable("OWNEET_EMULATOR_CARDS");
    if (!env.isEmpty())
        return env.split(QChar(':'), Qt::SkipEmptyParts);
    return { QStringLiteral("/usr/share/owneet-frontend/emulator-cards") };
}

QVector<QJsonObject> all_cards()
{
    QVector<QJsonObject> out;
    for (const QString& dir : card_dirs()) {
        for (const QFileInfo& info : QDir(dir).entryInfoList({ QStringLiteral("*.json") }, QDir::Files, QDir::Name)) {
            QFile file(info.filePath());
            if (!file.open(QIODevice::ReadOnly))
                continue;
            const QJsonObject card = QJsonDocument::fromJson(file.readAll()).object();
            if (card.isEmpty())
                Log::warning(LOGMSG("Not a card: `%1`").arg(info.filePath()));
            else
                out.append(card);
        }
    }
    return out;
}

bool matches(const QJsonArray& patterns, const QString& name)
{
    for (const QJsonValue& p : patterns) {
        const QRegularExpression re(QRegularExpression::wildcardToRegularExpression(p.toString()),
                                    QRegularExpression::CaseInsensitiveOption);
        if (re.match(name).hasMatch())
            return true;
    }
    return false;
}

// The card of an emulator (its id, relative to the folder of the emulators): by its program's name, or its folder's
QJsonObject card_of(const QString& id)
{
    if (id.isEmpty())
        return {};
    const QString file = QFileInfo(id).fileName();
    const QString folder = id.section(QChar('/'), 0, 0);
    for (const QJsonObject& card : all_cards()) {
        const QJsonArray patterns = card.value(QStringLiteral("match")).toArray();
        if (matches(patterns, file) || (folder != file && matches(patterns, folder)))
            return card;
    }
    return {};
}

QString home_path(const QString& path)
{
    return path.startsWith(QLatin1String("~/")) ? QDir::homePath() + path.mid(1) : path;
}

QStringList with(const QJsonArray& words, const QString& key, const QString& value)
{
    QStringList out;
    for (const QJsonValue& w : words)
        out.append(w.toString().replace(key, value));
    return out;
}

// The settings the console wrote: { path: sha1 }, so that only its own are ever written again
QString written_path()
{
    return paths::writableConfigDir() + QStringLiteral("/emulator-settings.json");
}

QString sha1(const QByteArray& data)
{
    return QString::fromLatin1(QCryptographicHash::hash(data, QCryptographicHash::Sha1).toHex());
}

// Every {pad.…} in a text with its value; false when one has none (no controller yet)
bool fill_pad(QString& text, const QHash<QString, QString>& pad)
{
    static const QRegularExpression placeholder(QStringLiteral("\\{(pad\\.[a-z]+)\\}"));
    QString out;
    int last = 0;
    auto it = placeholder.globalMatch(text);
    while (it.hasNext()) {
        const QRegularExpressionMatch m = it.next();
        if (!pad.contains(m.captured(1)))
            return false;
        out += text.mid(last, m.capturedStart() - last) + pad.value(m.captured(1));
        last = m.capturedEnd();
    }
    text = out + text.mid(last);
    return true;
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
    QVector<Emulator> out, others;          // others: shown only when there is nothing else
    const QDir base(console.emulators);
    for (const QFileInfo& info : base.entryInfoList(QDir::Files | QDir::Dirs | QDir::NoDotAndDotDot, QDir::Name)) {
        if (info.fileName().startsWith(QChar('.')))
            continue;
        if (info.isFile()) {
            if (program_rank(info) > 0)
                out.append({ info.fileName(), bare_name(info.fileName()), fits(info.filePath()) });
            else if (is_other_program(info))
                others.append({ info.fileName(), bare_name(info.fileName()), false });
            continue;
        }
        if (info.fileName().contains(QLatin1String(".appimage."), Qt::CaseInsensitive))
            continue;
        // a folder: the best kind in it, at the shallowest place
        const auto add = [&base, &info](QVector<Program> found, QVector<Emulator>& list) {
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
                list.append({ base.relativeFilePath(p.path), found.size() == 1 ? info.fileName() : info.fileName() + QStringLiteral(": ") + file, fits(p.path) });
            }
        };
        QVector<Program> found, foundOthers;
        int entries = 0;
        search_programs(info.filePath(), 0, entries, found, foundOthers);
        add(found, out);
        if (found.isEmpty())
            add(foundOthers, others);
    }
    return out.isEmpty() ? others : out;
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
            title.sure = false;
            continue;
        }
        byName.insert(name, out.size());
        out.append({ info.filePath(), name, QStringList(info.filePath()), true });
    }
    for (const QFileInfo& info : QDir(console.games).entryInfoList(QDir::Dirs | QDir::NoDotAndDotDot, QDir::Name)) {
        if (info.fileName().startsWith(QChar('.')))
            continue;
        const Start start = start_of(info.filePath(), 0);
        out.append({ info.filePath(), info.fileName(), start.files, start.sure });
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
            out.append(QVariantMap { { QStringLiteral("id"), e.id }, { QStringLiteral("name"), e.name }, { QStringLiteral("fits"), e.fits } });
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
    QStringList able;                       // the ones that can run on this PC
    for (const Emulator& e : all) {
        if (e.fits)
            able.append(e.id);
    }
    return able.contains(id) ? id : able.size() == 1 ? able.first() : QString();
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
    if (all.contains(name))
        return name;
    if (game->extraMap().value(QStringLiteral("emu.sure")).toBool())
        return all.first();
    // what the recognised emulator wants, when it says so
    const QJsonValue prefer = card_of(chosen(game_obj)).value(QStringLiteral("prefer"));
    if (prefer.toString() == QLatin1String("folder")) {
        for (const QString& f : all) {
            if (QFileInfo(f).isDir())
                return f;
        }
    } else if (prefer.isArray()) {
        for (const QString& f : all) {
            if (matches(prefer.toArray(), QFileInfo(f).fileName()))
                return f;
        }
    }
    return {};
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

bool Emulators::prepare(QObject* game_obj, const QString& pad) const
{
    auto* game = qobject_cast<model::Game*>(game_obj);
    const ConsoleDir console = consoleDir(console_path(game_obj));
    const QString id = chosen(game_obj), start = file(game_obj);
    if (!game || !console.valid() || id.isEmpty() || start.isEmpty())
        return false;
    const QString emulator = QDir(console.emulators).filePath(id);
    const QJsonObject card = card_of(id);
    writeSettings(card, pad);
    // how it starts: the user's choice, else the card's, else the game alone
    const bool own = choices_of(console.path).value(QStringLiteral("options")).toObject().contains(id);
    const QJsonArray words = card.value(QStringLiteral("start")).toArray();
    const QStringList cmd = !own && !words.isEmpty() ? command(emulator, with(words, QStringLiteral("{game}"), start), QString())
                                                     : command(emulator, splitOptions(options(game_obj, id)), start);
    // what the card asks of the emulator's environment (e.g. the same kind of controller driver as the console's)
    QStringList env;
    const QJsonObject vars = card.value(QStringLiteral("env")).toObject();
    const QHash<QString, QString> values = padValues(pad);
    for (auto it = vars.constBegin(); it != vars.constEnd(); ++it) {
        QString value = it.value().toString();
        if (fill_pad(value, values) && !it.key().contains(QChar('=')) && !it.key().contains(QChar(' ')) && !value.contains(QChar(' ')))
            env.append(it.key() + QChar('=') + value);
    }
    if (!env.isEmpty())
        env.prepend(QStringLiteral("/usr/bin/env"));
    game->setLaunchCmd((env + cmd).join(QChar(' '))).setLaunchWorkdir(QFileInfo(emulator).absolutePath());
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

QVariantMap Emulators::systemFile(QObject* game) const
{
    const ConsoleDir console = consoleDir(console_path(game));
    const QJsonObject sys = card_of(chosen(game)).value(QStringLiteral("systemFile")).toObject();
    const QString name = sys.value(QStringLiteral("name")).toString();
    if (!console.valid() || name.isEmpty())
        return { { QStringLiteral("needed"), false } };
    bool installed = false;
    for (const QJsonValue& p : sys.value(QStringLiteral("installed")).toArray())
        installed = installed || QFileInfo::exists(home_path(p.toString()));
    // copied by the user beside the emulator, or at the top of the console's folder
    QString found;
    for (const QString& dir : { console.emulators, console.path }) {
        const int depth = dir == console.path ? 0 : PROGRAM_DEPTH;
        QDirIterator it(dir, QDir::Files | QDir::Dirs | QDir::NoDotAndDotDot,
                        depth > 0 ? QDirIterator::Subdirectories : QDirIterator::NoIteratorFlags);
        int entries = 0;
        while (found.isEmpty() && it.hasNext() && ++entries <= MAX_ENTRIES) {
            it.next();
            if (it.fileInfo().isFile() && it.fileName().compare(name, Qt::CaseInsensitive) == 0
                && QDir(dir).relativeFilePath(it.filePath()).count(QChar('/')) <= depth)
                found = it.filePath();
        }
        if (!found.isEmpty())
            break;
    }
    return { { QStringLiteral("needed"), true }, { QStringLiteral("name"), name },
             { QStringLiteral("installed"), installed }, { QStringLiteral("found"), found } };
}

QVariantMap Emulators::installCommand(QObject* game) const
{
    const ConsoleDir console = consoleDir(console_path(game));
    const QString id = chosen(game);
    const QJsonArray words = card_of(id).value(QStringLiteral("systemFile")).toObject().value(QStringLiteral("install")).toArray();
    const QString found = systemFile(game).value(QStringLiteral("found")).toString();
    if (!console.valid() || words.isEmpty() || found.isEmpty())
        return {};
    const QString emulator = QDir(console.emulators).filePath(id);
    return { { QStringLiteral("command"), command(emulator, with(words, QStringLiteral("{file}"), found), QString()) },
             { QStringLiteral("workdir"), QFileInfo(emulator).absolutePath() } };
}

// The first controller as SDL sees it here: its name, and which of its own buttons, axes and hats are the standard
// layout's (an emulator that reads the controller's raw buttons needs them). Emulators through SDL see the same, with the
// same kind of driver ({pad.hidapi}).
QHash<QString, QString> Emulators::padValues(const QString& name)
{
    QHash<QString, QString> out;
    if (name.isEmpty())
        return out;
    out.insert(QStringLiteral("pad.name"), name);
    if (!s_testPad.isEmpty()) {
        for (auto it = s_testPad.cbegin(); it != s_testPad.cend(); ++it)
            out.insert(it.key(), it.value().toString());
        return out;
    }
#ifdef WITH_SDL_GAMEPAD
    for (int i = 0; i < SDL_NumJoysticks(); ++i) {
        if (!SDL_IsGameController(i))
            continue;
        SDL_GameController* pad = SDL_GameControllerOpen(i);         // the same, counted once more
        if (!pad)
            continue;
        char guid[33] = {};
        SDL_JoystickGetGUIDString(SDL_JoystickGetDeviceGUID(i), guid, sizeof(guid));
        // byte 14 of the GUID is 'h' (0x68) for SDL's own (HIDAPI) drivers
        out.insert(QStringLiteral("pad.hidapi"), QString::fromLatin1(guid).mid(28, 2) == QLatin1String("68") ? QStringLiteral("1") : QStringLiteral("0"));
        static const QVector<QPair<const char*, SDL_GameControllerButton>> buttons {
            { "a", SDL_CONTROLLER_BUTTON_A }, { "b", SDL_CONTROLLER_BUTTON_B }, { "x", SDL_CONTROLLER_BUTTON_X },
            { "y", SDL_CONTROLLER_BUTTON_Y }, { "back", SDL_CONTROLLER_BUTTON_BACK }, { "start", SDL_CONTROLLER_BUTTON_START },
            { "lb", SDL_CONTROLLER_BUTTON_LEFTSHOULDER }, { "rb", SDL_CONTROLLER_BUTTON_RIGHTSHOULDER },
            { "ls", SDL_CONTROLLER_BUTTON_LEFTSTICK }, { "rs", SDL_CONTROLLER_BUTTON_RIGHTSTICK },
            { "up", SDL_CONTROLLER_BUTTON_DPAD_UP }, { "down", SDL_CONTROLLER_BUTTON_DPAD_DOWN },
            { "left", SDL_CONTROLLER_BUTTON_DPAD_LEFT }, { "right", SDL_CONTROLLER_BUTTON_DPAD_RIGHT },
        };
        for (const auto& b : buttons) {
            const SDL_GameControllerButtonBind bind = SDL_GameControllerGetBindForButton(pad, b.second);
            // a button's own number, -1 when it is a hat's direction (then {pad.hat} says which hat)
            out.insert(QStringLiteral("pad.") + QLatin1String(b.first),
                       QString::number(bind.bindType == SDL_CONTROLLER_BINDTYPE_BUTTON ? bind.value.button : -1));
            if (bind.bindType == SDL_CONTROLLER_BINDTYPE_HAT)
                out.insert(QStringLiteral("pad.hat"), QString::number(bind.value.hat.hat));
        }
        static const QVector<QPair<const char*, SDL_GameControllerAxis>> axes {
            { "lx", SDL_CONTROLLER_AXIS_LEFTX }, { "ly", SDL_CONTROLLER_AXIS_LEFTY },
            { "rx", SDL_CONTROLLER_AXIS_RIGHTX }, { "ry", SDL_CONTROLLER_AXIS_RIGHTY },
        };
        for (const auto& a : axes) {
            const SDL_GameControllerButtonBind bind = SDL_GameControllerGetBindForAxis(pad, a.second);
            out.insert(QStringLiteral("pad.") + QLatin1String(a.first),
                       QString::number(bind.bindType == SDL_CONTROLLER_BINDTYPE_AXIS ? bind.value.axis : -1));
        }
        if (!out.contains(QStringLiteral("pad.hat")))
            out.insert(QStringLiteral("pad.hat"), QStringLiteral("-1"));
        SDL_GameControllerClose(pad);
        break;
    }
#endif
    return out;
}

QVariantMap Emulators::s_testPad;

void Emulators::setTestPad(const QVariantMap& values)
{
    s_testPad = values;
}

// An ini file's section: where its lines start and end ([start, end)), -1 when it has none
static QPair<int, int> section_span(const QString& file, const QString& name)
{
    const QString header = QChar('[') + name + QChar(']');
    int start = -1;
    int pos = 0;
    while (pos <= file.size()) {
        int eol = file.indexOf(QChar('\n'), pos);
        if (eol < 0)
            eol = file.size();
        const QString line = file.mid(pos, eol - pos).trimmed();
        if (start < 0 && line == header) {
            start = eol + 1;
        } else if (start >= 0 && line.startsWith(QChar('['))) {
            return { std::min<int>(start, file.size()), pos };
        }
        pos = eol + 1;
    }
    return start < 0 ? qMakePair(-1, -1) : qMakePair(std::min<int>(start, file.size()), file.size());
}

void Emulators::writeSettings(const QJsonObject& card, const QString& padName)
{
    const QJsonArray settings = card.value(QStringLiteral("settings")).toArray();
    if (settings.isEmpty())
        return;
    const QHash<QString, QString> pad = padValues(padName);
    QFile in(written_path());
    QJsonObject written = in.open(QIODevice::ReadOnly) ? QJsonDocument::fromJson(in.readAll()).object() : QJsonObject();
    in.close();
    bool changed = false;
    for (const QJsonValue& v : settings) {
        const QJsonObject s = v.toObject();
        const QString path = home_path(s.value(QStringLiteral("path")).toString());
        const QString section = s.value(QStringLiteral("section")).toString();
        QString text = s.value(QStringLiteral("text")).toString();
        if (path.isEmpty() || !QDir::isAbsolutePath(path) || !fill_pad(text, pad))
            continue;                       // no controller yet: written when there is one
        // only for a controller like this ("when": {"pad.up": "-1"}; "!" before the value: not that one)
        bool fits = true;
        const QJsonObject when = s.value(QStringLiteral("when")).toObject();
        for (auto it = when.begin(); fits && it != when.end(); ++it) {
            QString want = it.value().toString();
            const bool other = want.startsWith(QChar('!'));
            if (other)
                want.remove(0, 1);
            fits = (pad.value(it.key()) == want) != other;
        }
        if (!fits)
            continue;
        if (!section.isEmpty() && !text.endsWith(QChar('\n')))
            text += QChar('\n');
        // the emulator's own first settings, which it makes by itself: replaced as if there were none
        const QRegularExpression untouched(s.value(QStringLiteral("replaceWhen")).toString());
        const bool hasRule = !s.value(QStringLiteral("replaceWhen")).toString().isEmpty() && untouched.isValid();
        const QString key = section.isEmpty() ? path : path + QChar('#') + section;
        QFile now(path);
        QString current;
        const bool exists = now.exists();
        if (exists) {
            if (!now.open(QIODevice::ReadOnly))
                continue;
            current = QString::fromUtf8(now.readAll());
            now.close();
        }
        const QPair<int, int> span = section.isEmpty() ? qMakePair(0, current.size()) : section_span(current, section);
        const bool there = exists && span.first >= 0;
        // "add": false, a section only filled where it is, or in a new file: an emulator that loses all its settings on a
        // file it cannot read may keep that part of it in another form
        if (exists && !there && !section.isEmpty() && !s.value(QStringLiteral("add")).toBool(true))
            continue;
        const QString mine = there ? current.mid(span.first, span.second - span.first) : QString();
        if (there) {
            const bool own = sha1(mine.toUtf8()) == written.value(key).toString();
            // an empty file or section is nobody's choice: an emulator may make one at its first start (open item 34)
            const bool empty = mine.trimmed().isEmpty();
            if (!own && !empty && !(hasRule && untouched.match(mine).hasMatch()))
                continue;                   // the user's own: never touched
            if (mine == text)
                continue;
        }
        QString result;
        if (section.isEmpty())
            result = text;
        else if (there)
            result = current.left(span.first) + text + current.mid(span.second);
        else
            result = current + (current.isEmpty() || current.endsWith(QChar('\n')) ? QString() : QStringLiteral("\n"))
                   + QChar('[') + section + QStringLiteral("]\n") + text;
        QDir().mkpath(QFileInfo(path).absolutePath());
        QSaveFile out(path);
        if (!out.open(QIODevice::WriteOnly) || out.write(result.toUtf8()) < 0 || !out.commit()) {
            Log::warning(LOGMSG("Cannot write `%1`").arg(path));
            continue;
        }
        written.insert(key, sha1(text.toUtf8()));
        changed = true;
    }
    if (!changed)
        return;
    QDir().mkpath(paths::writableConfigDir());
    QSaveFile out(written_path());
    if (!out.open(QIODevice::WriteOnly) || out.write(QJsonDocument(written).toJson()) < 0 || !out.commit())
        Log::warning(LOGMSG("Cannot save `%1`").arg(written_path()));
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
