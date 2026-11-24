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
    // The licence agreement Steam asks to accept at the first start, if any (appinfo → common →
    // eulas, the first one for every country): its id ("208650_eula_1"), name and official address
    QString eula_id;
    QString eula_name;
    QString eula_url;
    // What Steam says of the game on SteamOS ([[step-4.8-games-that-cannot-run]]; appinfo → common →
    // steam_deck_compatibility → steamos_compatibility, 1 = unsupported): empty when Steam does not
    // call it unsupported, otherwise why ("anticheat", or "other" when Steam gives no such reason),
    // and the month of Steam's check ("2024-11"), empty when not known
    QString unsupported;
    QString unsupported_month;
};

// The games of the account's licences: appcache/packageinfo.vdf (binary, versions 27 and 28)
// gives the packages of the account's licences, with their apps. Left out: package 0 (Spacewar,
// Team Fortress 2, Dota 2…, given to every account), except its games Steam is downloading or has
// installed (a manifest in one of `installdirs`, the libraries' steamapps folders: owner,
// 2026-11-21), and free weekends ("extended" →
// "freeweekend": a licence that has ended). A package's "status" is whether the store still
// offers it, not whether the account owns it: it is not used (owner's check, 2026-10-14: a
// free weekend of 2020 was listed, a game given for free and no longer offered was missing).
// appcache/appinfo.vdf (binary, versions 27 to 29) gives each app's type and name: only apps of
// type "game" are kept (not DLC, soundtracks, tools, demos). Empty when the files are missing or
// cannot be read.
std::vector<OwnedApp> read_owned_games(const QString& steam_datadir, const std::vector<QString>& installdirs);

} // namespace steam
} // namespace providers
