// SPDX-License-Identifier: GPL-3.0-or-later
// Facts about the installed system for Settings → System (roadmap 3.9): the OwneetOS version,
// the credited projects (CREDITS.md) and every installed package with its licence; where the
// interface sounds are, and whether this is the interface's first start since boot (3.12).

#pragma once

#include <QObject>
#include <QVariantList>


namespace owneet {
class SystemInfo : public QObject {
    Q_OBJECT
    Q_PROPERTY(QString version READ version CONSTANT)
    Q_PROPERTY(QString soundsDir READ soundsDir CONSTANT)

public:
    explicit SystemInfo(QObject* parent = nullptr);

    /// IMAGE_VERSION of /etc/os-release (the ISO's date), or "" when unknown
    QString version() const;
    /// The tables of CREDITS.md ($OWNEET_CREDITS, or /usr/share/doc/owneetos/CREDITS.md):
    /// [{ name, who, licence }]
    Q_INVOKABLE QVariantList credits() const;
    /// The installed packages, from pacman's local database: [{ name, version, licence }]
    Q_INVOKABLE QVariantList packages() const;
    /// The interface sounds: $OWNEET_SOUNDS, or /usr/share/owneet-frontend/sounds
    QString soundsDir() const;
    /// True only the first time it is asked since the system started (a mark in
    /// $XDG_RUNTIME_DIR/owneet): the start-up sound plays once per boot, not after the interface
    /// restarts (e.g. when the console moves to another screen)
    Q_INVOKABLE bool firstStartThisBoot() const;
};
} // namespace owneet
