// SPDX-License-Identifier: GPL-3.0-or-later
// The user's own emulators ([[step-4.9-the-users-own-emulators]], approved demo
// design/demos/4.9-users-own-emulators.html, [[users-own-emulators]]): a folder named by the user as
// they like (the console's name) with two folders in it, the emulators and the games. The user brings
// both; OwneetOS ships, downloads, links and names no emulator, and configures none: it starts a
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
    };
    struct Title {
        QString path;                   // the file, or the game's folder
        QString name;
        QStringList files;              // what the emulator may be given: absolute paths
    };
    struct ConsoleDir {
        QString path;
        QString name;                   // the user's name for it
        QString emulators;              // its two folders, absolute paths
        QString games;
        bool valid() const { return !path.isEmpty(); }
    };

    explicit Emulators(QObject* parent = nullptr);

    /// The names of the two folders, lower case, in the console's languages
    static const QStringList& emulatorsNames();
    static const QStringList& gamesNames();
    /// A folder with the two in it and nothing to start of its own (a game for the PC that happens to
    /// have folders with these names stays a game); not valid otherwise
    static ConsoleDir consoleDir(const QString& path);
    static QVector<Emulator> emulatorsOf(const ConsoleDir& console);
    static QVector<Title> titlesOf(const ConsoleDir& console);
    /// How a line of options is split: by spaces, double quotes keep spaces together, anywhere
    static QStringList splitOptions(const QString& line);
    /// The command that starts an emulator (with a game, unless `file` is empty)
    static QStringList command(const QString& emulator, const QStringList& options, const QString& file);
    /// Whether a disk has such a folder at its top (the games folder apart)
    static bool atTop(const QString& root);

    // ---- For the theme. `game`: a game whose extra "emu.console" is its console's folder
    /// [{ id, name }]: read from the disk now
    Q_INVOKABLE QVariantList list(QObject* game) const;
    /// The name of the folder of its console's emulators
    Q_INVOKABLE QString folder(QObject* game) const;
    /// The id of the emulator chosen for the console, or of the only one; "" when the user must choose
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
    /// Makes a console's folders in a disk's games folder (made in the console's language when there
    /// is none): { ok, games, emulators, titles } (folder names), or { ok: false }
    Q_INVOKABLE QVariantMap create(const QString& name, const QString& root, const QString& language) const;
};
} // namespace owneet
