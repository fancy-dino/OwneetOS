// SPDX-License-Identifier: GPL-3.0-or-later
// The simplified file manager ([[step-6.11-simplified-file-manager]], approved demo
// design/demos/6.11-file-manager.html): the places it opens on, what is in a folder with each
// item's size, Steam's leftovers, and deleting. It only ever reads and deletes inside its places
// (the games folders, the Windows games' data and logs, Steam's leftovers), never a place itself,
// and never follows a link out of them. Sizes and deletions take a while: they run on a thread of
// their own and answer with a signal.

#pragma once

#include <QObject>
#include <QStringList>
#include <QVariantList>


namespace owneet {
class FileManager : public QObject {
    Q_OBJECT

public:
    explicit FileManager(QObject* parent = nullptr);

    /// The places: [{ kind, path, root }], kind "games" (a games folder: root is the disk's key,
    /// "internal" for the home folder, else its mount folder), "windows" (the Windows games'
    /// prefixes), "logs" (their logs), "steam" (Steam's leftovers; path is Steam's folder). Only
    /// the places that exist.
    Q_INVOKABLE QVariantList places() const;

    /// What is in a place or one of its folders (kind as in places(); "folder" for a folder inside
    /// a games folder), answered by listed(token, entries): [{ name, path, folder, size, modified }]
    /// (modified: ms since 1970), sorted folders first, then by name; files whose name starts with
    /// "." are left out. For "steam" (path: Steam's folder) the leftovers of the games no longer
    /// installed: [{ name: appid, path, folder: true, size, modified, steam: "shaders" | "data" |
    /// "download", appid }].
    Q_INVOKABLE int list(const QString& kind, const QString& path);

    /// Deletes a file or a folder inside a place, answered by removed(token, path, ok, freed bytes).
    /// Refused (ok false) outside the places, for a place itself, or on a disk mounted read only.
    Q_INVOKABLE int remove(const QString& path);

    /// The Windows data's folder of a game folder, as the console names it (DiskGames)
    Q_INVOKABLE QString windowsPrefix(const QString& game_dir) const;

signals:
    void listed(int token, const QVariantList& entries);
    void removed(int token, const QString& path, bool ok, double freed);

private:
    int m_next = 0;
    QStringList roots() const;
};
} // namespace owneet
