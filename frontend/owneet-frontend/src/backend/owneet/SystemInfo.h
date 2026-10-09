// SPDX-License-Identifier: GPL-3.0-or-later
// Facts about the installed system for Settings → System (roadmap 3.9): the OwneetOS version,
// the credited projects (CREDITS.md) and every installed package with its licence.

#pragma once

#include <QObject>
#include <QVariantList>


namespace owneet {
class SystemInfo : public QObject {
    Q_OBJECT
    Q_PROPERTY(QString version READ version CONSTANT)

public:
    explicit SystemInfo(QObject* parent = nullptr);

    /// IMAGE_VERSION of /etc/os-release (the ISO's date), or "" when unknown
    QString version() const;
    /// The tables of CREDITS.md ($OWNEET_CREDITS, or /usr/share/doc/owneetos/CREDITS.md):
    /// [{ name, who, licence }]
    Q_INVOKABLE QVariantList credits() const;
    /// The installed packages, from pacman's local database: [{ name, version, licence }]
    Q_INVOKABLE QVariantList packages() const;
};
} // namespace owneet
