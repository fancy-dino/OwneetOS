// SPDX-License-Identifier: GPL-3.0-or-later
// OwneetOS: play time and last played of Steam games, from Steam's own local files (roadmap 4.1,
// local data only: PROJECT_RULES.md section 12).

#pragma once

#include "utils/HashMap.h"

#include <QDateTime>
#include <QString>
#include <QVariantMap>


namespace providers {
namespace steam {

// Steam's text "KeyValues" files (.vdf, .acf): nested { "key" "value" } blocks. Keys are matched
// without regard to case, as Steam does ("Valve" / "valve" both appear): they come lower-cased.
QVariantMap read_vdf_file(const QString& path);
QVariantMap parse_vdf(const QString& text);

struct PlayStats {
    qint64 seconds = 0;
    QDateTime last_played;
};

// The account that used Steam last on this computer (loginusers.vdf: "MostRecent", or the latest
// "Timestamp" in newer clients), then its
// localconfig.vdf: Software → Valve → Steam → apps → APPID → Playtime (minutes) and LastPlayed
// (Unix time). Play time counts the whole account, other computers too. Empty when unknown.
HashMap<QString, PlayStats> read_playtime(const QString& steam_datadir);

} // namespace steam
} // namespace providers
