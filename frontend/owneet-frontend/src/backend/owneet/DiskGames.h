// SPDX-License-Identifier: GPL-3.0-or-later
// Games on a disk ([[step-4.3-local-drm-free-games]], approved demo design/demos/4.3-games-on-disk.html):
// the "Games" folders, and the file each game starts with. The disk-games provider lists the
// games; when a game has several files to start, the theme asks "Which file starts {title}?"
// through `diskGames` and Options changes the choice, which is kept in the console's settings.

#pragma once

#include <QObject>
#include <QStringList>
#include <QVariantMap>

namespace model { class Game; }


namespace owneet {
class DiskGames : public QObject {
    Q_OBJECT
    // Set by the theme before it reads the games again because a disk came or went: the interface
    // stays on screen and the games are replaced only once the new ones are ready (no loading
    // screen); cleared when they are
    Q_PROPERTY(bool quietReading READ quietReading WRITE setQuietReading NOTIFY quietReadingChanged)

public:
    explicit DiskGames(QObject* parent = nullptr);

    /// The names of the games folder at the top of a disk and in the home folder, lower case:
    /// "games" and its translations ([[games-on-disk]])
    static const QStringList& folderNames();
    /// The file the user chose for a game folder (relative to it), or "" when none was chosen
    static QString savedChoice(const QString& game_dir);
    /// Makes `game` start `file` (relative to its folder `game_dir`): its command and working
    /// folder, and its extra "disk.start"
    static void applyChoice(model::Game& game, const QString& game_dir, const QString& file);
    /// A Windows game's own prefix, in the console's disk ([[step-4.5-windows-games-proton]])
    static QString windowsPrefix(const QString& game_dir);
    /// A game's own folder on the console's disk: ~/.local/share/owneet/KIND/<name>-<hash>
    static QString gameDataDir(const QString& kind, const QString& game_dir);

    /// Whether a mounted disk has a games folder at its top (the theme reads the games again only then)
    Q_INVOKABLE bool hasGames(const QString& root) const;
    /// The file a game on a disk starts with, relative to its folder; "" when the user must choose
    Q_INVOKABLE QString chosen(QObject* game) const;
    /// The user's choice, one of the game's "disk.files": applied at once and remembered
    Q_INVOKABLE bool choose(QObject* game, const QString& file);
    /// Whether a Windows game has never started (its prefix is made at its first start: longer)
    Q_INVOKABLE bool firstStart(QObject* game) const;
    /// What owneetd needs to install a game that has only an installer ([[step-4.6-installers]]):
    /// {installer, kind, dest, prefix, title}. dest is the game's folder on the console's disk, in
    /// the home folder's games folder (made in the console's language when there is none), under
    /// the same name; or the game's own folder when its installer is already there
    Q_INVOKABLE QVariantMap installRequest(QObject* game, const QString& language) const;
    /// The bytes a game's folder takes (what uninstalling it frees)
    Q_INVOKABLE double folderSize(const QString& path) const;

    bool quietReading() const { return m_quiet_reading; }
    void setQuietReading(bool quiet);

signals:
    void quietReadingChanged();

private:
    bool m_quiet_reading = false;
};
} // namespace owneet
