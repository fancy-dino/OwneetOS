// SPDX-License-Identifier: GPL-3.0-or-later
// A local game's cover from a picture on a disk ([[step-6.9-cover-images-from-a-disk]], approved
// demo design/demos/6.9-cover-images.html). The pictures are read from the folder "Images"
// (Immagini, Bilder, Imágenes; any case of letters) at the top of every disk but the console's
// own, with its sub-folders: PNG and JPEG (WebP left out, owner 2026-11-17). The chosen one is
// copied into the game's folder as cover.<ext>, where the library looks for a game's cover, at
// most 1200 px on its longest side (a bigger one is saved smaller, owner 2026-11-17); an earlier
// cover.* goes. Reading the disks and copying take a while: they run on a thread of their own and
// answer with a signal.

#pragma once

#include <QObject>
#include <QVariantList>


namespace owneet {
class CoverImages : public QObject {
    Q_OBJECT
    /// A disk with pictures is connected (Options on a local game offers a cover then)
    Q_PROPERTY(bool available READ available NOTIFY availableChanged)

public:
    explicit CoverImages(QObject* parent = nullptr);

    bool available() const { return m_available; }

    /// The folder names a disk's pictures are in (lower case)
    static const QStringList& folderNames();

    /// Reads the disks' pictures again (a disk plugged in or out, the window opening), answered by
    /// found(token, pictures): [{ path, url, name, folder, disk }], folder the sub-folders' path inside
    /// the pictures' folder ("" at its top, "/" between two), disk the disk's name; sorted by disk,
    /// then by folder and name as people read them (2 before 10). It also updates `available`.
    Q_INVOKABLE int find();

    /// Copies a picture into a game's folder as cover.<ext> (its own extension, lower case) and
    /// removes an earlier cover.*: answered by used(token, ok, cover). A picture longer than 1200 px
    /// on a side, or turned by its camera, is saved smaller and upright. Refused (ok false) for a
    /// file that is not a PNG or JPEG picture, or a folder on a disk mounted read only.
    Q_INVOKABLE int use(const QString& picture, const QString& game_dir);

signals:
    void availableChanged();
    void found(int token, const QVariantList& pictures);
    void used(int token, bool ok, const QString& cover);

private:
    int m_next = 0;
    int m_latest = 0;
    bool m_available = false;
};
} // namespace owneet
