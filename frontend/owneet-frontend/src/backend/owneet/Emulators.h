// SPDX-License-Identifier: GPL-3.0-or-later
// The user's own emulators ([[step-4.9-the-users-own-emulators]], approved demo
// design/demos/4.9-users-own-emulators.html, [[users-own-emulators]]): at the top of a disk, beside the
// games folder, a folder "Emulation" (owner, 2026-11-25: no game's folder is looked into); in it, a
// folder named by the user as they like (the console's name) with two folders in it, the emulators and
// the games. The user brings both; OwneetOS ships, downloads, links and names no emulator, and configures none: it starts a
// program of the user's with a file of the user's (owneet-emulator-run, packages/owneetd).
// The disk-games provider lists the games; the theme asks, through `emulators`, which emulator and
// which file, and how it starts; the answers are kept in the console's settings (emulators.json).

#pragma once

#include <QObject>
#include <QString>
#include <QStringList>
#include <QVariantList>
#include <QVariantMap>
#include <QVector>

namespace model { class Game; }


namespace owneet {
class Emulators : public QObject {
    Q_OBJECT

public:
    struct Emulator {
        QString id;                     // relative to the folder of the emulators
        QString name;                   // as the user wrote it: the folder's or the file's name
        bool fits;                      // made for this kind of processor
    };
    struct Title {
        QString path;                   // the file, or the game's folder
        QString name;
        QStringList files;              // what the emulator may be given: absolute paths
        bool sure;                      // the first of them, without asking (Options can still change it)
    };
    struct ConsoleDir {
        QString path;
        QString name;                   // the user's name for it
        QString emulators;              // its two folders, absolute paths
        QString games;
        bool valid() const { return !path.isEmpty(); }
    };

    explicit Emulators(QObject* parent = nullptr);

    /// The names of the folder at the top of a disk, and of the two, lower case, in the console's languages
    static const QStringList& baseNames();
    static bool isBase(const QString& name);
    static const QStringList& emulatorsNames();
    static const QStringList& gamesNames();
    /// A folder with the two in it; not valid otherwise
    static ConsoleDir consoleDir(const QString& path);
    static QVector<Emulator> emulatorsOf(const ConsoleDir& console);
    static QVector<Title> titlesOf(const ConsoleDir& console);
    /// How a line of options is split: by spaces, double quotes keep spaces together, anywhere
    static QStringList splitOptions(const QString& line);
    /// The command that starts an emulator (with a game, unless `file` is empty)
    static QStringList command(const QString& emulator, const QStringList& options, const QString& file);
    /// The consoles in a disk's "Emulation" folders
    static QVector<ConsoleDir> consolesOf(const QString& root);

    // ---- For the theme. `game`: a game whose extra "emu.console" is its console's folder
    /// [{ id, name, fits }]: read from the disk now; fits: made for this kind of processor
    Q_INVOKABLE QVariantList list(QObject* game) const;
    /// The name of the folder of its console's emulators
    Q_INVOKABLE QString folder(QObject* game) const;
    /// The id of the emulator chosen for the console, or of the only one (of those that can run here, when
    /// there are several); "" when the user must choose
    Q_INVOKABLE QString chosen(QObject* game) const;
    Q_INVOKABLE void choose(QObject* game, const QString& id);
    /// The user has been told, once, that this emulator is not part of OwneetOS
    Q_INVOKABLE bool told(QObject* game, const QString& id) const;
    Q_INVOKABLE void setTold(QObject* game, const QString& id);
    /// The options before the game ("How it starts"), "" = the game alone
    Q_INVOKABLE QString options(QObject* game, const QString& id) const;
    Q_INVOKABLE void setOptions(QObject* game, const QString& id, const QString& line);
    /// The files the game can be started with, and the one chosen ("" when the user must choose)
    Q_INVOKABLE QStringList files(QObject* game) const;
    Q_INVOKABLE QString file(QObject* game) const;
    Q_INVOKABLE void chooseFile(QObject* game, const QString& file);
    /// Makes the game's launch command, from the choices
    Q_INVOKABLE bool prepare(QObject* game) const;
    /// The command and the folder that open an emulator without a game: { command, workdir }
    Q_INVOKABLE QVariantMap openCommand(QObject* game, const QString& id) const;
    /// Whether a disk ("internal", or its folder) has a console of that name (in any case)
    Q_INVOKABLE bool nameTaken(const QString& name, const QString& root) const;
    /// Makes a console's folders in a disk's "Emulation" folder (made in the console's language when
    /// there is none): { ok, base, name, emulators, titles } (folder names), or { ok: false }
    Q_INVOKABLE QVariantMap create(const QString& name, const QString& root, const QString& language) const;
};
} // namespace owneet
