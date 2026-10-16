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

    // ---- Games on a disk ([[step-4.3-local-drm-free-games]]): the provider gives each one its
    // folder ("disk.dir"), its disk ("disk.root": the mount folder, "internal" for the console's own
    // disk) and the files it can start ("disk.files", relative to its folder)
    function onDisk(game) { return !!game && !!game.extra && !!game.extra["disk.dir"]; }
    function diskRoot(game) { return onDisk(game) ? String(game.extra["disk.root"] || "") : ""; }
    function diskFiles(game) { return onDisk(game) ? (game.extra["disk.files"] || []) : []; }
    // A Windows game on a disk (4.5): never named so to the user, it only needs what owneetd
    // downloads once
    function isWindows(game) { return onDisk(game) && game.extra["disk.windows"] === true; }
    // The same game after the games were read again (new objects): by app id, folder or title
    function sameGame(a, b) {
        if (!a || !b)
            return false;
        if (appid(a) !== "" || appid(b) !== "")
            return appid(a) === appid(b);
        if (onDisk(a) || onDisk(b))
            return onDisk(a) && onDisk(b) && a.extra["disk.dir"] === b.extra["disk.dir"];
        return a.title === b.title;
    }

    // Steam gives the play time and the last time played, not how many times: either counts
    function played(game) {
        return !!game && !!game.lastPlayed && !isNaN(game.lastPlayed.getTime()) && (game.playCount > 0 || game.playTime > 0);
    }
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

    // ---- Steam games owned but not installed ([[step-4.2c-steam-without-its-interface]])
    // The Steam provider lists every owned game with its app id (extra "steam.appid") and whether
    // it was installed when the games were read; while the interface runs, owneetd says what is
    // installed, downloading, or on a disk that is not connected (GET /v1/steam, "steam.changed").
    property var steam: null                    // owneetd's Steam status, null without owneetd
    property bool online: true                  // the console has an internet connection
    function appid(game) { return game && game.extra ? String(game.extra["steam.appid"] || "") : ""; }
    // What is installed, downloading (in which order) or away changes rarely; the bytes of a
    // download every second. Lists are sorted and filtered on the first only, so that a download
    // moving on does not rebuild them (covers follow the bytes by themselves).
    property var steamStates: null              // { games: {id}, downloads: {id: {state, order}}, away: {id: disk} }
    property string steamKey: ""
    onSteamChanged: {
        const st = steam;
        const ds = st ? (st.downloads || []) : [];
        const key = st ? JSON.stringify([st.games, ds.map(d => [d.appid, d.state]), st.away]) : "";
        if (key === steamKey)
            return;
        steamKey = key;
        if (!st) {
            steamStates = null;
            return;
        }
        const games = {}, downloads = {};
        (st.games || []).forEach(id => { games[id] = true; });
        ds.forEach((d, i) => { downloads[d.appid] = { state: d.state, order: i }; });
        steamStates = { games: games, downloads: downloads, away: st.away || {} };
    }
    readonly property var downloads: {          // app id → download, with its bytes now
        const out = {};
        ((steam && steam.downloads) || []).forEach(d => { out[d.appid] = d; });
        return out;
    }
    // "installed", "none", "downloading", "updating", "queued" (waiting for its turn), "paused"
    // (waiting, no internet), "away" (installed on a disk that is not connected)
    function steamState(game) {
        const id = appid(game);
        if (!id || !isSteam(game))
            return "installed";
        const st = steamStates;
        if (!st)
            return game.extra["steam.installed"] === false ? "none" : "installed";
        const d = st.downloads[id];
        if (d)
            return !online && d.state !== "updating" ? "paused" : d.state;
        if (st.games[id])
            return "installed";
        return st.away[id] !== undefined ? "away" : "none";
    }
    function download(game) { return downloads[appid(game)] || null; }
    function progress(game) {                   // 0…100
        const d = download(game);
        return d && d.total > 0 ? Math.min(100, Math.floor(d.done * 100 / d.total)) : 0;
    }
    function awayDisk(game) { return steamStates ? (steamStates.away[appid(game)] || "") : ""; }
    // Can be played now (an update running counts: Steam finishes it first)
    function installed(game) {
        if (game === null || game === undefined)
            return false;
        const s = steamState(game);
        return s === "installed" || s === "updating";
    }
    // The state line under a game's title: "" for an installed game
    function stateText(game) {
        const s = steamState(game);
        if (s === "installed")
            return "";
        if (s === "downloading" || s === "updating")
            return Tr.tr("steam.state." + s, { p: progress(game) });
        return Tr.tr("steam.state." + s);
    }

    // Sorting: "recent" (downloads first, in Steam's order; then installed games, last played
    // first; then the others by name), "played" (last played first only: the home screen),
    // "name", "time" (most played)
    function sorted(list, by) {
        const out = list.slice();
        const byName = (a, b) => a.sortBy.localeCompare(b.sortBy);
        const last = g => played(g) ? g.lastPlayed.getTime() : 0;
        const queued = g => steamStates ? steamStates.downloads[appid(g)] : undefined;
        const rank = g => queued(g) ? 0 : installed(g) ? 1 : 2;
        const order = g => { const d = queued(g); return d ? d.order : 0; };
        if (by === "name")
            out.sort(byName);
        else if (by === "time")
            out.sort((a, b) => b.playTime - a.playTime || byName(a, b));
        else if (by === "played")
            out.sort((a, b) => last(b) - last(a) || byName(a, b));
        else
            out.sort((a, b) => rank(a) - rank(b) || order(a) - order(b)
                                || (rank(a) === 1 ? last(b) - last(a) : 0) || byName(a, b));
        return out;
    }
    function recent(model) { return sorted(model.toVarArray(), "recent"); }

    // Sizes for people: "212 GB", "1.1 TB" (decimal units, as disks are sold)
    function size1(bytes) {                    // "2.4 GB": downloads, with one decimal
        return Number(bytes / 1e9).toLocaleString(Qt.locale(), "f", 1) + " GB";
    }
    function size(bytes) {
        const gb = bytes / 1e9;
        if (gb >= 1000)
            return Number(gb / 1000).toLocaleString(Qt.locale(), "f", 1) + " TB";
        return Number(Math.round(gb)).toLocaleString(Qt.locale(), "f", 0) + " GB";
    }
}
