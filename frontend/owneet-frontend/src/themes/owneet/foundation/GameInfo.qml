// SPDX-License-Identifier: GPL-3.0-or-later
// Texts about a game, shared by every screen: where it comes from, when it was last played, how
// long it has been played.
pragma Singleton
import QtQuick 2.15

QtObject {
    // The Steam provider puts its games in the collection "Steam"; everything else is local.
    function isSteam(game) {
        if (!game)
            return false;
        for (let i = 0; i < game.collections.count; i++)
            if (game.collections.get(i).name === "Steam")
                return true;
        return false;
    }
    function source(game) { return game ? (isSteam(game) ? "Steam" : Tr.tr("game.source.local")) : ""; }

    function played(game) { return game && game.lastPlayed && !isNaN(game.lastPlayed.getTime()) && game.playCount > 0; }
    function lastPlayed(game) {
        if (!played(game))
            return Tr.tr("game.played.never");
        const day = d => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
        const days = Math.round((day(new Date()) - day(game.lastPlayed)) / 86400000);
        if (days <= 0)
            return Tr.tr("game.played.today");
        if (days === 1)
            return Tr.tr("game.played.yesterday");
        return Tr.trn("game.played.days", days);
    }
    function playTime(game) {
        const minutes = game ? Math.floor(game.playTime / 60) : 0;
        if (minutes < 60)
            return Tr.trn("game.time.minutes", minutes);
        return Tr.tr("game.time.hours", { h: Math.floor(minutes / 60), m: minutes % 60 });
    }

    // Owned but not installed games (Steam) arrive with the Steam integration (4.x): until then
    // every game in the list is installed.
    function installed(game) { return game !== null && game !== undefined; }

    // Sorting: "recent" (last played, most recent first), "name", "time" (most played)
    function sorted(list, by) {
        const out = list.slice();
        const byName = (a, b) => a.sortBy.localeCompare(b.sortBy);
        const last = g => played(g) ? g.lastPlayed.getTime() : 0;
        if (by === "name")
            out.sort(byName);
        else if (by === "time")
            out.sort((a, b) => b.playTime - a.playTime || byName(a, b));
        else
            out.sort((a, b) => last(b) - last(a) || byName(a, b));
        return out;
    }
    function recent(model) { return sorted(model.toVarArray(), "recent"); }

    // Sizes for people: "212 GB", "1.1 TB" (decimal units, as disks are sold)
    function size(bytes) {
        const gb = bytes / 1e9;
        if (gb >= 1000)
            return Number(gb / 1000).toLocaleString(Qt.locale(), "f", 1) + " TB";
        return Number(Math.round(gb)).toLocaleString(Qt.locale(), "f", 0) + " GB";
    }
}
