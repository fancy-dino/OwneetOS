// SPDX-License-Identifier: GPL-3.0-or-later
// OwneetOS: every game of the Steam account, installed or not, from Steam's own binary caches
// ([[step-4.2c-steam-without-its-interface]], local data only: [[legal-compliance]]).

#pragma once

#include <QString>
#include <vector>


namespace providers {
namespace steam {

struct OwnedApp {
    QString appid;
    QString name;
};

// The games of the account's licences: appcache/packageinfo.vdf (binary, versions 27 and 28)
// gives the packages of the account's licences, with their apps. Left out: package 0 (Spacewar,
// Team Fortress 2, Dota 2…, given to every account) and free weekends ("extended" →
// "freeweekend": a licence that has ended). A package's "status" is whether the store still
// offers it, not whether the account owns it: it is not used (owner's check, 2026-10-14: a
// free weekend of 2020 was listed, a game given for free and no longer offered was missing).
// appcache/appinfo.vdf (binary, versions 27 to 29) gives each app's type and name: only apps of
// type "game" are kept (not DLC, soundtracks, tools, demos). Empty when the files are missing or
// cannot be read.
std::vector<OwnedApp> read_owned_games(const QString& steam_datadir);

} // namespace steam
} // namespace providers
