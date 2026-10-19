// SPDX-License-Identifier: GPL-3.0-or-later
// The OwneetOS theme: the shell around the sections (top bar, sections on LB / RB, prompt bar,
// windows). Foundations: [[step-3.3-theme-foundations]] (design/demos/3.3-theme-foundations.html);
// home: [[step-3.6-home-screen]] (design/demos/3.6-home.html). Library (3.7) and Settings (3.9) are
// temporary pages for now.
import QtQuick 2.15
import "foundation"

FocusScope {
    id: root
    focus: true

    // ---- Preferences, saved in the theme's memory (moved to Settings in [[step-3.9-settings]])
    Binding { target: Theme; property: "width"; value: root.width }
    Binding { target: Theme; property: "height"; value: root.height }
    readonly property var prefs: [
        ["palette", "paletteKey"], ["textScale", "textScale"], ["glyphs", "glyphSetting"],
        ["reduceMotion", "reduceMotion"], ["showSafeArea", "showSafeArea"], ["uiSounds", "uiSounds"],
        ["uiSoundsVolume", "uiSoundsVolume"], ["keyboard", "keyboardSetting"]
    ]
    property string librarySort: "recent"
    onLibrarySortChanged: save("librarySort", librarySort)
    property bool loaded: false
    Component.onCompleted: {
        prefs.forEach(p => { if (api.memory.has(p[0])) Theme[p[1]] = api.memory.get(p[0]); });
        if (api.memory.has("librarySort")) librarySort = api.memory.get("librarySort");
        loaded = true;
        // The interface restarts when the console moves to another screen (and when it comes
        // back): it opens again on Settings → Display, where the move was asked
        // (kept for two minutes: going back to the previous screen restarts it once more)
        if (api.memory.has("rescanReturn")) {            // after reading the games again (4.2)
            const r = String(api.memory.get("rescanReturn")).split("|");
            api.memory.unset("rescanReturn");
            if (Date.now() - Number(r[0]) < 60000 && sections.indexOf(r[1]) >= 0) {
                section = r[1];
                library.startOnAddGames = r[2] === "add";
            }
        }
        if (api.memory.has("displayReturn")) {
            if (Date.now() - api.memory.get("displayReturn") < 120000) {
                settings.section = "display";
                section = "settings";
            } else {
                api.memory.unset("displayReturn");
            }
        }
        updatePad();
        // once per boot, not when the interface restarts: the start-up animation with its sound
        if (systemInfo.firstStartThisBoot()) {
            intro.show();
            sounds.playStartup();
            introFallback.start();
        }
        refreshGames();
        refreshDaemon();          // owneetd may have connected before the theme was loaded
        knownVolumes = volumeKeys();
        diskNotices();            // a disk plugged in or out made the games be read again (4.3)
    }
    function save(key, value) {
        if (loaded && api.memory.get(key) !== value)
            api.memory.set(key, value);
    }
    Connections {
        target: Theme
        function onPaletteKeyChanged() { save("palette", Theme.paletteKey); }
        function onTextScaleChanged() { save("textScale", Theme.textScale); }
        function onGlyphSettingChanged() { save("glyphs", Theme.glyphSetting); }
        function onReduceMotionChanged() { save("reduceMotion", Theme.reduceMotion); }
        function onShowSafeAreaChanged() { save("showSafeArea", Theme.showSafeArea); }
        function onUiSoundsChanged() { save("uiSounds", Theme.uiSounds); }
        function onUiSoundsVolumeChanged() { save("uiSoundsVolume", Theme.uiSoundsVolume); }
        function onKeyboardSettingChanged() { save("keyboard", Theme.keyboardSetting); root.applyKeyboard(); }
    }

    // ---- Controller: its name, and the button prompts that match it
    readonly property var pads: Internal.gamepad.devices
    property string padName: ""
    function updatePad() {
        const pad = pads.count > 0 ? pads.get(0) : null;
        padName = pad ? pad.name : "";
        // By the names SDL and the kernel give the controllers: PlayStation ("PS5 Controller",
        // "DualSense Wireless Controller", "Sony ... Wireless Controller"), Nintendo ("Nintendo
        // Switch Pro Controller", "Joy-Con"); everything else (Xbox, generic pads) gets the
        // Xbox-style letters.
        if (pad) {
            const n = pad.name;
            if (/xbox|microsoft/i.test(n))
                Theme.padKind = "xbox";
            else if (/nintendo|switch|joy-?con/i.test(n))
                Theme.padKind = "nintendo";
            else if (/sony|dualsense|dualshock|playstation|wireless controller|\bps[345]\b/i.test(n))
                Theme.padKind = "ps";
            else
                Theme.padKind = "xbox";
        }
    }
    Connections {
        target: root.pads
        function onCountChanged() { root.updatePad(); }
    }
    Connections {             // the name can arrive after the controller
        target: root.pads.count > 0 ? root.pads.get(0) : null
        function onNameChanged() { root.updatePad(); }
    }
    Component.onDestruction: Theme.previewKey = ""

    // ---- Games, most recently played first (shared by the sections); the home screen shows the
    // installed ones only (every owned Steam game is in the library, 4.2c)
    property var games: []
    function refreshGames() { games = GameInfo.recent(api.allGames); }
    readonly property var homeGames: GameInfo.sorted(games.filter(g => GameInfo.installed(g)), "played")

    // ---- Steam's games as owneetd sees them (4.2c): installed, downloading, on a disk that is not
    // connected (GET /v1/steam and "steam.changed"). The console reads its games again when you
    // come back to the home screen and Steam has a game it does not list (e.g. a free game
    // installed from the store), or the licences changed after a visit to the store.
    readonly property bool online: network === null || (network.available !== false
        && ((network.ethernet && network.ethernet.connected) || (network.wifi && network.wifi.state === "connected")))
    Binding { target: GameInfo; property: "online"; value: root.online }
    property var steamBaseline: null          // app ids Steam had when the games were read
    property real steamLicencesSeen: 0
    property bool storeVisited: false
    property var steamAsked: ({})             // app id → { title, kind: install / uninstall / cancel }
    function steamIds(st) { return (st.games || []).concat((st.downloads || []).map(d => d.appid)); }
    function steamGame(appid) { return games.find(g => GameInfo.appid(g) === appid) || null; }
    function refreshSteam(check) {
        if (!owneetd.connected) return;
        owneetd.get("/v1/steam", (status, st) => {
            if (status !== 200) return;
            root.steamChanged(st);
            if (check) root.checkSteamGames();
        });
    }
    function checkSteamGames() {
        const st = GameInfo.steam;
        if (!st) return;
        const known = {};
        games.forEach(g => { const id = GameInfo.appid(g); if (id) known[id] = true; });
        const fresh = steamIds(st).filter(id => !steamBaseline[id] && !known[id]);
        const bought = storeVisited && (st.licences || 0) > steamLicencesSeen;
        if (fresh.length === 0 && !bought)
            return;
        console.log("Steam has games the console does not list yet: reading the games again");
        // reading the games again rebuilds the interface: it comes back where it was
        api.memory.set("rescanReturn", [Date.now(), root.section,
            root.section === "library" && library.addGamesSelected ? "add" : ""].join("|"));
        Internal.settings.reloadProviders();
    }
    // What changed: a download asked here has started, a game is ready, a game is gone
    function steamChanged(st) {
        const before = GameInfo.steam;
        GameInfo.steam = st;
        if (steamBaseline === null) {
            const base = {};
            steamIds(st).forEach(id => { base[id] = true; });
            steamBaseline = base;
            steamLicencesSeen = st.licences || 0;
        }
        if (!before) return;
        const has = (list, id) => (list || []).indexOf(id) >= 0;
        const downloading = s => (s.downloads || []).map(d => d.appid);
        const asked = Object.assign({}, steamAsked);
        Object.keys(asked).forEach(id => {
            const a = asked[id];
            if (a.kind === "install" && has(downloading(st), id)) {
                toast.show(Tr.tr("toast.steam.queued", { title: a.title }));
                delete asked[id];
            } else if (a.kind !== "install" && !has(st.games, id) && !has(downloading(st), id)) {
                toast.show(Tr.tr(a.kind === "cancel" ? "toast.steam.cancelled" : "toast.steam.uninstalled", { title: a.title }));
                delete asked[id];
            }
        });
        steamAsked = asked;
        downloading(before).forEach(id => {
            if (!has(downloading(st), id) && has(st.games, id)) {
                const g = steamGame(id);
                // not for the game starting or running (Steam updated it before starting it)
                const starting = g && ((steamPrep.visible && steamPrep.gameTitle === g.title)
                                       || (runningGame && runningGame.name === g.title));
                if (g && !starting) notify({ key: "notices.steam.ready", args: { name: g.title }, icon: "check", kind: "info",
                                action: "play", game: g.title });
            }
        });
    }
    // The downloads notice on the home screen: the game Steam works on, and how many wait
    readonly property var steamNotices: {
        const ds = GameInfo.steam && GameInfo.steam.downloads ? GameInfo.steam.downloads : [];
        const g = ds.length > 0 ? steamGame(ds[0].appid) : null;
        if (!g) return [];
        const p = ds[0].total > 0 ? Math.floor(ds[0].done * 100 / ds[0].total) : 0;
        const text = !online ? Tr.tr("home.notice.paused")
            : Tr.tr(ds[0].state === "updating" ? "home.notice.update" : "home.notice.download", { title: g.title, p: p })
              + (ds.length > 1 ? "  ·  " + Tr.tr("home.notice.more", { n: ds.length - 1 }) : "");
        return [{ text: text, game: g }];
    }
    Connections {
        target: api.allGames
        function onCountChanged() { root.refreshGames(); }
    }

    // ---- Sections (LB / RB)
    readonly property var sections: ["home", "library", "settings"]
    property string section: "home"
    readonly property Item page: section === "home" ? home : section === "library" ? library : settings
    function goSection(name) {
        if (name === section)
            return;
        section = name;
        Nav.feedback("section");
        page.focusStart();
    }
    readonly property bool modalOpen: palettes.visible || languages.visible || credits.visible || dialog.visible || osk.visible
                                      || notices.visible || quick.visible || controls.visible || steamPrep.visible
    function backToPage() { Qt.callLater(() => { if (!root.modalOpen) root.page.forceActiveFocus(); }); }

    // ---- Buttons the sections leave to the whole interface (section 9.1)
    Keys.onPressed: {
        const a = Nav.action(event);
        if (a === "page" && section === "home" && !modalOpen) {
            notices.openSheet();                    // Y: notifications (demo 3.11)
        } else if ((a === "section-prev" || a === "section-next") && !modalOpen) {
            const i = sections.indexOf(section) + (a === "section-next" ? 1 : -1);
            goSection(sections[(i + sections.length) % sections.length]);
        } else if (a.startsWith("section-") || a.startsWith("filter-") || a === "page") {
            Nav.feedback("edge");         // no filters or page options on these pages yet
        } else {
            return;
        }
        event.accepted = true;
    }

    // ---- owneetd (3.8): running apps, what is on screen, controllers, network
    property var apps: []                 // running games and apps: { id, name, kind, started }
    property string focusedApp: ""        // "home", an app id, or "" (cage)
    property string sessionMode: ""       // "gamescope" or "cage"
    property var controllers: []
    property var network: null
    readonly property var runningGame: {
        for (let i = 0; i < apps.length; i++)
            if (apps[i].kind === "game")
                return apps[i];
        return null;
    }
    function refreshApps() {
        owneetd.get("/v1/apps", (status, data) => {
            if (status === 200) { apps = data.apps || []; focusedApp = data.focus || ""; }
        });
    }
    function refreshControllers() {
        owneetd.get("/v1/controllers", (status, data) => { if (status === 200) controllers = data.controllers || []; });
    }
    function refreshDaemon() {
        owneetd.get("/v1/status", (status, data) => { if (status === 200) sessionMode = data.session_mode || ""; });
        owneetd.get("/v1/network", (status, data) => { if (status === 200) { network = data; lastNetwork = data; } });
        refreshApps();
        refreshControllers();
        applyKeyboard();          // the on-screen keyboard and physical keyboards follow the chosen layout
        refreshSteam(false);
        refreshWindows();
        refreshInstalls();
        owneetd.get("/v1/update", (status, data) => { if (status === 200) root.updateStatus = data; });
        checkDisplayPending();
    }
    Connections {
        target: owneetd
        function onConnectedChanged() { if (owneetd.connected) root.refreshDaemon(); }
        function onEvent(type, data) {
            root.noticeFor(type, data);
            if (type === "app.started" || type === "app.exited") {
                root.refreshApps();
                // a Steam game started: its licence agreement, if any, has been accepted
                if (type === "app.started" && String(data.id).startsWith("steam:"))
                    root.agreementAccepted(String(data.id).slice(6));
                if (type === "app.exited" && (data.id === "steam" || String(data.id).startsWith("steam:")))
                    root.refreshSteam(true);
                if (type === "app.exited")
                    refreshTimer.restart();       // play time and "last played" have changed
            } else if (type === "steam.launch_failed") {
                if (data && data.reason === "linux_version") root.askWindowsVersion(String(data.appid));
            } else if (type === "focus.changed") {
                if (data.focus === "home") root.refreshSteam(true);
                // Guide brings the home screen over a running game: the "launch" sound, the other way
                const fromGame = root.gameOnScreen;
                root.focusedApp = data.focus;
                if (data.focus === "home" && fromGame)
                    sounds.play("launch");
            } else if (type.startsWith("controller.")) {
                root.refreshControllers();
            } else if (type === "network.changed") {
                root.network = data;
            } else if (type.startsWith("bluetooth.")) {
                root.pairingEvent(type, data);
            } else if (type.startsWith("display.")) {
                root.displayEvent(type, data);
            } else if (type === "steam.changed") {
                root.steamChanged(data);
            } else if (type === "windows.changed") {
                root.windowsChanged(data);
            } else if (type === "installs.changed") {
                GameInfo.installs = (data && data.installs) || [];
            } else if (type === "install.done" || type === "install.failed") {
                root.installEnded(type, data);
            } else if (type === "update.changed") {
                root.updateStatus = data;
                root.showUpdate(data);
            }
        }
    }
    // Errors from owneetd come as "owneetd:CODE" and are translated (error.CODE)
    Connections {
        target: api
        function onEventLaunchError(msg) {
            const code = msg.startsWith("owneetd:") ? msg.slice(8) : "";
            steamPrep.close(true);
            if (code === "steam.needs_install") {
                root.showSteamNeedsInstall();
                return;
            }
            const key = "error." + code;
            toast.show(code && Tr.tr(key) !== key ? Tr.tr(key) : Tr.tr("error.launch"));
        }
    }
    function resume(app) {
        owneetd.post("/v1/apps/" + encodeURIComponent(app.id) + "/focus", {}, (status, data) => {
            if (status !== 200 && status !== 204)
                toast.show(Tr.tr("error.resume"));
        });
    }
    function confirmClose(app) {
        dialog.game = null;
        dialog.mode = "close";
        dialog.appId = app.id;
        dialog.contentWidth = 0;
        dialog.title = Tr.tr("close.title", { title: app.name });
        dialog.text = Tr.tr("close.text");
        dialog.buttons = [
            { text: Tr.tr("home.close"), action: () => {
                dialog.close();
                closingApps[app.id] = true;          // closed on purpose: not "closed unexpectedly"
                owneetd.post("/v1/apps/" + encodeURIComponent(app.id) + "/close", {}, (status) => {
                    if (status >= 300 || status === 0) toast.show(Tr.tr("error.close"));
                });
            } },
            { text: Tr.tr("prompt.cancel"), style: "primary", action: () => dialog.close() }
        ];
        dialog.defaultIndex = 1;             // Cancel first: no game closed by accident
        dialog.open();
    }

    // ---- Notifications (3.11, approved demo design/demos/3.11-notifications.html): banners at the top
    // right, the bell in the top bar, the list on Y from the home screen. "attention": something to
    // act on (notify-error sound); "info": news (notify). Kept until the console restarts (30 at most).
    property var noticeList: []
    property int unseen: 0
    property var quietPads: ({})              // controllers turned off from Settings: id → time
    property var closingApps: ({})            // games closed on purpose
    property var warnedBattery: ({})          // controllers already warned about their battery
    property real quietNetUntil: 0
    property bool lostNetwork: false          // a "lost" notice was given: "back online" is news
    property var lastNetwork: null
    function quietNetwork() { quietNetUntil = Date.now() + 15000; }
    // While a game is on screen nothing is drawn over it (the overlay comes in Wave 2) and no
    // sound plays: the notification waits in the list, the bell counts it
    readonly property bool gameOnScreen: runningGame !== null && focusedApp !== "home"
    function notify(n) {
        n.time = Date.now();
        noticeList = [n].concat(noticeList).slice(0, 30);
        if (notices.visible)
            return;
        unseen++;
        if (gameOnScreen)
            return;
        Nav.feedback(n.kind === "attention" ? "notify-error" : "notify");
        const detailKey = n.detailKey !== undefined ? n.detailKey : n.key + ".detail";
        banners.show({ title: Tr.tr(n.key, n.args), detail: detailKey ? Tr.tr(detailKey, n.args) : "",
                       icon: n.icon, crossed: n.crossed, kind: n.kind });
    }
    function controllerName(id) {           // a connected controller's name
        const c = controllers.find(c => c.id === id);
        return c ? c.name : "";
    }
    function batteryLow(c) {
        return c.battery !== undefined ? c.battery <= 15 : c.battery_level === "low" || c.battery_level === "critical";
    }
    function noticeFor(type, data) {
        if (type === "controller.added" && data && data.name) {
            if (Date.now() - (quietPads["paired:" + (data.id || "").toLowerCase()] || 0) < 15000)
                return;                         // just paired from Settings: "… paired" said it
            const pct = data.battery !== undefined;
            notify({ key: "notices.pad.connected", args: { name: data.name, battery: data.battery }, icon: "controller",
                     kind: "info", detailKey: pct ? "notices.pad.connected.detail" : "" });
        } else if (type === "controller.removed") {
            const name = controllerName(data.id);
            delete warnedBattery[data.id];
            if (!name || Date.now() - (quietPads[(data.id || "").toLowerCase()] || 0) < 15000)
                return;
            notify({ key: "notices.pad.off", args: { name: name }, icon: "controller", kind: "attention", action: "controllers" });
        } else if (type === "controller.battery" && data && data.id) {
            if (!batteryLow(data)) {
                if (data.battery === undefined || data.battery > 25)
                    delete warnedBattery[data.id];
                return;
            }
            if (warnedBattery[data.id])
                return;
            warnedBattery[data.id] = true;
            const pct = data.battery !== undefined;
            notify({ key: "notices.pad.low", args: { name: data.name || controllerName(data.id), battery: data.battery }, icon: "battery",
                     kind: "attention", action: "controllers",
                     detailKey: pct ? "notices.pad.low.detail" : "notices.pad.low.level.detail" });
        } else if (type === "bluetooth.paired" && data) {
            quietPads["paired:" + (data.address || "").toLowerCase()] = Date.now();
        } else if (type === "network.changed" && data) {
            const online = n => n && n.available !== false && ((n.ethernet && n.ethernet.connected) || (n.wifi && n.wifi.state === "connected"));
            const before = lastNetwork;
            lastNetwork = data;
            if (!before || Date.now() < quietNetUntil)
                return;
            if (online(before) && !online(data)) {
                // Wi-Fi turned off on purpose (Settings, or a switch on the computer) is not news
                if (before.wifi && before.wifi.state === "connected" && data.wifi && data.wifi.enabled === false)
                    return;
                lostNetwork = true;
                const wifi = before.wifi && before.wifi.state === "connected";
                notify({ key: wifi ? "notices.net.lost" : "notices.net.cable", args: { ssid: wifi ? before.wifi.ssid : "" },
                         icon: wifi ? "wifi" : "ethernet", crossed: true, kind: "attention", action: "network" });
            } else if (!online(before) && online(data) && lostNetwork) {
                lostNetwork = false;
                const wifi = data.wifi && data.wifi.state === "connected";
                notify({ key: wifi ? "notices.net.back" : "notices.net.back.cable", args: { ssid: wifi ? data.wifi.ssid : "" },
                         icon: wifi ? "wifi" : "ethernet", kind: "info", detailKey: "notices.net.back.detail" });
            }
        } else if (type === "app.exited" && data) {
            const app = apps.find(a => a.id === data.id);
            // "Close …?" open for it: it went by itself, as the user wanted; no notice
            const asked = dialog.visible && dialog.mode === "close" && dialog.appId === data.id;
            if (asked) dialog.close(true);
            const wanted = closingApps[data.id] || asked;
            delete closingApps[data.id];
            if (wanted || !app || app.kind !== "game" || ["exit-code", "signal", "core-dump"].indexOf(data.result) < 0)
                return;
            notify({ key: "notices.game.crash", args: { name: data.name || app.name }, icon: "alert", kind: "attention",
                     action: "play", game: data.name || app.name });
        }
    }
    function openNotice(n) {
        notices.close(true);
        if (n.action === "controllers" || n.action === "network") {
            settings.section = n.action;
            goSection("settings");
        } else if (n.action === "play") {
            const g = games.find(g => g.title === n.game);
            if (g) root.launch(g); else Nav.feedback("edge");
        }
    }

    // ---- Games: launch, details, options
    function launch(game, agreementShown) {
        // a game with only an installer (4.6): installed first, after the console's question
        if (GameInfo.hasInstaller(game)) { installStart(game); return; }
        // Steam (4.2c): a game not installed is installed first; one being downloaded or on a disk
        // that is not connected shows its details; Steam's first start needs the internet; a
        // licence agreement Steam asks to accept at the first start is shown here first
        if (GameInfo.isSteam(game)) {
            const s = GameInfo.steamState(game);
            if (s === "none") { askInstall(game); return; }
            if (s !== "installed" && s !== "updating") { showDetails(game); return; }
            if (!(runningGame && runningGame.name === game.title) && steamOffline("play")) return;
            if (!agreementShown && !runningGame && agreementPending(game)) { showAgreement(game); return; }
        }
        // a Windows game before what it needs is ready: its progress, or "connect" (4.5)
        if (GameInfo.isWindows(game) && !windowsReady && !(runningGame && runningGame.name === game.title)) {
            showWindowsNotReady(game);
            return;
        }
        // a game on a disk with several files to start: which one, asked once (4.3)
        if (GameInfo.onDisk(game) && diskGames.chosen(game) === "" && !(runningGame && runningGame.name === game.title)) {
            chooseFile(game, true);
            return;
        }
        // the game already running (same title, as on the home screen): back to it
        if (runningGame && owneetd.connected && runningGame.name === game.title) {
            Nav.feedback("launch");
            root.resume(runningGame);
            return;
        }
        if (runningGame && owneetd.connected) {     // one game at a time
            toast.show(Tr.tr("error.apps.game_running"));
            Nav.feedback("edge");
            return;
        }
        Nav.feedback("launch");
        if (GameInfo.isWindows(game))               // "Preparing {game}…" until its first picture (4.5)
            steamPrep.start(diskGames.firstStart(game), game.title, true);
        else
            toast.show(Tr.tr("toast.launch", { title: game.title }));
        if (GameInfo.isSteam(game) && owneetd.connected) {
            // Steam starts the game; while Steam itself starts, the console says so
            owneetd.get("/v1/steam", (status, st) => {
                // also when Steam runs: it stays hidden, e.g. while it prepares the game's shaders
                if (status === 200 && st.allowed) steamPrep.start(!st.prepared, game.title);
            });
        }
        game.launch();                    // through owneetd, [[step-3.8-frontend-daemon-bridge]]
        refreshTimer.restart();           // "last played" changes once the game has started
    }
    Timer { id: refreshTimer; interval: 3000; onTriggered: root.refreshGames() }
    // Details: Play an installed game, Install one that is not, otherwise Close; the facts follow
    // the download while the window is open (4.2c)
    function showDetails(game) {
        const s = GameInfo.steamState(game);
        dialog.game = game;
        dialog.mode = "details";
        dialog.contentWidth = 0;
        dialog.title = game.title;
        dialog.text = "";
        dialog.buttons = [s === "installed" || s === "updating"
            ? { text: Tr.tr("home.play"), style: "primary", action: () => { dialog.close(true); root.launch(game); } }
            : s === "none"
            ? { text: Tr.tr("options.install"), style: "primary", action: () => {
                    dialog.close(true);
                    if (GameInfo.hasInstaller(game)) root.installStart(game); else root.askInstall(game);
                } }
            : { text: Tr.tr("prompt.close"), style: "primary", action: () => dialog.close() }];
        if (GameInfo.hasInstaller(game) && GameInfo.usableInstaller(game))
            inspectInstaller(game, null);     // the space it needs, for the facts
        dialog.defaultIndex = 0;
        dialog.open();
    }
    function detailsFacts(game) {
        const facts = [[Tr.tr("details.source"), GameInfo.source(game)]];
        const s = GameInfo.steamState(game), d = GameInfo.download(game);
        if (GameInfo.isSteam(game))
            facts.push([Tr.tr("details.state"), s === "installed" ? Tr.tr("steam.state.installed") : GameInfo.stateText(game)]);
        if (GameInfo.hasInstaller(game)) {        // 4.6: its state, its installer, the space it needs
            facts.push([Tr.tr("details.state"), GameInfo.stateText(game)],
                       [Tr.tr("details.installer"), String(game.extra["disk.installer"]).split("/").pop()]);
            const info = installInfos[game.extra["disk.installer"]];
            if (info && GameInfo.usableInstaller(game))
                facts.push([Tr.tr("details.needs"), GameInfo.sizeAuto(info.need)]);
        }
        if (d && (s === "downloading" || s === "updating" || s === "paused"))
            facts.push([Tr.tr("details.done"), Tr.tr("details.done.value", { done: GameInfo.size1(d.done), total: GameInfo.size1(d.total) })]);
        if (d && (s === "downloading" || s === "updating") && d.rate > 0) {
            const left = (d.total - d.done) / d.rate;
            facts.push([Tr.tr("details.speed"), Number(d.rate / 1e6).toLocaleString(Qt.locale(), "f", 1) + " MB/s"],
                       [Tr.tr("details.left"), left < 60 ? Tr.tr("details.left.less") : Tr.tr("details.left.minutes", { n: Math.round(left / 60) })]);
        }
        if (s === "queued") {
            const first = (GameInfo.steam.downloads || [])[0];
            const g = first && first.appid !== GameInfo.appid(game) ? steamGame(first.appid) : null;
            if (g) facts.push([Tr.tr("details.done"), Tr.tr("details.queue", { title: g.title })]);
        }
        facts.push([Tr.tr("details.last"), GameInfo.lastPlayed(game)],
                   [Tr.tr("details.time"), GameInfo.played(game) ? GameInfo.playTime(game) : "\u2014"]);
        if (GameInfo.onDisk(game))
            facts.push([Tr.tr("details.disk"), diskName(GameInfo.diskRoot(game))]);
        return facts;
    }
    function showOptions(game) {
        const s = GameInfo.steamState(game), steam = GameInfo.isSteam(game);
        dialog.game = game;
        dialog.mode = "options";
        dialog.contentWidth = 0;
        dialog.title = game.title;
        dialog.text = "";
        dialog.buttons = (s === "installed"
            ? [{ text: Tr.tr("home.play"), style: "primary", action: () => { dialog.close(true); root.launch(game); } }]
            : s === "none"
            ? [{ text: Tr.tr("options.install"), style: "primary", action: () => {
                    dialog.close(true);
                    if (GameInfo.hasInstaller(game)) root.installStart(game); else root.askInstall(game);
                } }]
            : []).concat([
            { text: Tr.tr("home.details"), action: () => { dialog.close(true); root.showDetails(game); } },
            { text: Tr.tr(game.favorite ? "options.unfavorite" : "options.favorite"), action: () => {
                game.favorite = !game.favorite;
                library.favoritesRevision++;
                dialog.close();
                toast.show(Tr.tr(game.favorite ? "toast.favorite" : "toast.unfavorite"));
            } }
        ]);
        // a Steam game can always be uninstalled, and its download cancelled, without Steam's interface
        if (steam && (s === "installed" || s === "updating"))
            dialog.buttons = dialog.buttons.concat([{ text: Tr.tr("options.uninstall"), action: () => { dialog.close(true); root.askUninstall(game, false); } }]);
        if (steam && (s === "downloading" || s === "queued" || s === "paused"))
            dialog.buttons = dialog.buttons.concat([{ text: Tr.tr("options.cancel_install"), action: () => { dialog.close(true); root.askUninstall(game, true); } }]);
        if (GameInfo.diskFiles(game).length > 1)
            dialog.buttons = dialog.buttons.concat([{ text: Tr.tr("options.choose_file"), action: () => { dialog.close(true); root.chooseFile(game, false); } }]);
        // 4.6: an install stops; a game the console installed is uninstalled (its installer stays)
        if (GameInfo.hasInstaller(game) && (s === "installing" || s === "queued"))
            dialog.buttons = dialog.buttons.concat([{ text: Tr.tr("options.stop_install"), action: () => { dialog.close(true); root.askStopInstall(game); } }]);
        if (GameInfo.installedHere(game))
            dialog.buttons = dialog.buttons.concat([{ text: Tr.tr("options.uninstall"), action: () => { dialog.close(true); root.askRemoveInstalled(game); } }]);
        if (section !== "library")
            dialog.buttons = dialog.buttons.concat([{ text: Tr.tr("options.library"), action: () => {
                dialog.close(true);
                root.goSection("library");
                library.select(game);
            } }]);
        dialog.defaultIndex = 0;
        dialog.open();
    }
    function showHowToAdd() {
        dialog.game = null;
        dialog.mode = "how";
        dialog.contentWidth = 0;
        dialog.title = Tr.tr("how.title");
        dialog.text = Tr.tr("how.steam") + "\n\n" + Tr.tr("how.local");
        dialog.buttons = [{ text: Tr.tr("home.empty.store"), style: "primary", action: () => { dialog.close(true); root.openSteam("store"); } }];
        dialog.defaultIndex = 0;
        dialog.open();
    }
    function notYet() { toast.show(Tr.tr("toast.later")); }

    // ---- Windows games (4.5, approved demo design/demos/4.5-windows-games.html): what they need is
    // downloaded once by owneetd, only once the library has one (POST /v1/windows/needed), while
    // the console is online; its progress in the home screen's notices and Settings → System, a
    // banner when ready. GET /v1/windows and "windows.changed": { state, percent, needed }.
    property var windowsStatus: null
    readonly property bool windowsReady: windowsStatus !== null && windowsStatus.state === "ready"
    readonly property bool hasWindowsGames: games.some(g => GameInfo.isWindows(g)
        || ["inno", "nsis", "msi"].indexOf(GameInfo.installerKind(g)) >= 0)
    onHasWindowsGamesChanged: tellWindowsNeeded()
    function refreshWindows() {
        if (!owneetd.connected) return;
        owneetd.get("/v1/windows", (status, st) => {
            if (status !== 200) return;
            windowsStatus = st;
            tellWindowsNeeded();
        });
    }
    function tellWindowsNeeded() {
        if (hasWindowsGames && owneetd.connected && windowsStatus !== null && !windowsStatus.needed)
            owneetd.post("/v1/windows/needed", {});
    }
    function windowsChanged(st) {
        const before = windowsStatus;
        windowsStatus = st;
        if (before && before.state !== "ready" && st.state === "ready")
            notify({ key: "notices.windows.ready", args: {}, icon: "check", kind: "info" });
        if (dialog.visible && dialog.mode === "windowsWait" && dialog.game) {
            if (st.state === "ready") { const g = dialog.game; dialog.close(true); root.launch(g); }
            else showWindowsNotReady(dialog.game, true);
        }
    }
    function windowsText(st) {           // Settings → System and the home screen's notice
        if (!st) return "";
        if (st.state === "ready") return Tr.tr("windows.ready");
        if (st.state === "downloading") return Tr.tr("windows.getting", { p: st.percent });
        if (st.state === "failed") return Tr.tr("windows.failed");
        return Tr.tr("windows.waiting");
    }
    readonly property var windowsNotices: windowsStatus && windowsStatus.needed && windowsStatus.state !== "ready"
        ? [{ text: windowsStatus.state === "downloading" ? Tr.tr("windows.notice", { p: windowsStatus.percent }) : Tr.tr("windows.notice.wait"),
             game: null }]
        : []
    // A Windows game started before it can: the progress (online), or "connect to the internet"
    function showWindowsNotReady(game, refresh) {
        const st = windowsStatus;
        const offline = !online || !st || st.state === "waiting" || st.state === "failed";
        dialog.game = game;
        dialog.mode = "windowsWait";
        dialog.contentWidth = Theme.px(560);
        if (offline) {
            dialog.title = Tr.tr("windows.off.title", { title: game.title });
            dialog.text = Tr.tr("windows.off.text");
            dialog.buttons = [
                { text: Tr.tr("windows.off.network"), style: "primary", action: () => {
                    dialog.close(true);
                    settings.section = "network";
                    root.goSection("settings");
                } },
                { text: Tr.tr("prompt.cancel"), action: () => dialog.close() }];
        } else {
            dialog.title = Tr.tr("windows.wait.title");
            dialog.text = Tr.tr(GameInfo.hasInstaller(game) ? "windows.wait.text.install" : "windows.wait.text", { p: st.percent, title: game.title });
            dialog.buttons = [{ text: Tr.tr("prompt.ok"), style: "primary", action: () => dialog.close() }];
        }
        dialog.defaultIndex = 0;
        if (refresh) dialog.focusDefault(); else dialog.open();
    }

    // ---- Installers (4.6, approved demo design/demos/4.6-installers.html): a game with only an
    // installer is installed by owneetd on the console's disk, one at a time, after the console's
    // question with the game's licence ([[games-on-disk]]). GOG's Linux installers show their
    // percentage, Windows installers a moving bar; Windows installers wait for Windows games to be
    // ready. The installed copy replaces the game in the library (the games are read again
    // quietly); Options stop an install or uninstall a game the console installed.
    property var installInfos: ({})           // installer → { need, exact, free, licence }
    function refreshInstalls() {
        if (!owneetd.connected) return;
        owneetd.get("/v1/installs", (status, st) => { if (status === 200) GameInfo.installs = st.installs || []; });
    }
    function inspectInstaller(game, done) {
        const installer = game.extra["disk.installer"];
        owneetd.post("/v1/installs/inspect", { installer: installer, kind: GameInfo.installerKind(game) }, (status, info) => {
            if (status !== 200) {
                if (done) { toast.show(Tr.tr("error.installer")); Nav.feedback("error"); }
                return;
            }
            const infos = Object.assign({}, installInfos);
            infos[installer] = info;
            installInfos = infos;
            if (done) done(info);
        });
    }
    function installStart(game) {
        const s = GameInfo.steamState(game);
        if (s === "unusable") { showInstallUnusable(game); return; }
        if (s !== "none") { showDetails(game); return; }
        if (!owneetd.connected) { toast.show(Tr.tr("error.daemon.unavailable")); Nav.feedback("error"); return; }
        // a Windows installer runs with what Windows games need (4.5)
        if (GameInfo.installerKind(game) !== "sh" && !windowsReady) { showWindowsNotReady(game); return; }
        inspectInstaller(game, info => {
            if (info.free > 0 && info.need > info.free) showInstallSpace(game, info);
            else showInstallAsk(game, info, false);
        });
    }
    function showInstallAsk(game, info, back) {
        dialog.game = game;
        dialog.mode = "installAsk";
        dialog.contentWidth = Theme.px(560);
        dialog.icon = "";
        dialog.prompts = dialog.defaultPrompts;
        dialog.backAction = null;
        dialog.title = Tr.tr("installer.ask.title", { title: game.title });
        dialog.text = Tr.tr("installer.ask.text", { size: GameInfo.sizeAuto(info.need) });
        dialog.buttons = [
            { text: Tr.tr("installer.read"), action: () => root.showInstallLicence(game, info) },
            { text: Tr.tr("installer.go"), style: "primary", action: () => { dialog.close(true); root.requestInstall(game); } },
            { text: Tr.tr("prompt.cancel"), action: () => dialog.close() }];
        dialog.defaultIndex = 1;
        dialog.open(back);
    }
    // [Read]: the licence's text when the console found it (the right stick scrolls it), else
    // where to find it; B goes back to the question
    function showInstallLicence(game, info) {
        const back = () => { Nav.feedback("sheet-close"); root.showInstallAsk(game, info, true); };
        dialog.mode = "licence";
        dialog.licence = info.licence || "";
        dialog.contentWidth = Theme.px(680);
        dialog.title = Tr.tr("installer.licence.title", { title: game.title });
        dialog.text = info.licence ? "" : Tr.tr("installer.licence.missing");
        dialog.prompts = info.licence ? [{ buttons: ["rs"], label: Tr.tr("prompt.scroll") }, { buttons: ["a", "b"], label: Tr.tr("prompt.back") }]
                                      : [{ buttons: ["a", "b"], label: Tr.tr("prompt.back") }];
        dialog.backAction = back;
        dialog.buttons = [{ text: Tr.tr("prompt.back"), style: "primary", action: back }];
        dialog.defaultIndex = 0;
        licenceFlick.contentY = 0;
        dialog.open(true);
    }
    function showInstallSpace(game, info) {
        dialog.game = game;
        dialog.mode = "installSpace";
        dialog.contentWidth = Theme.px(560);
        dialog.icon = "";
        dialog.prompts = dialog.defaultPrompts;
        dialog.backAction = null;
        dialog.title = Tr.tr("installer.space.title", { title: game.title });
        dialog.text = Tr.tr("installer.space.text", { title: game.title, size: GameInfo.sizeAuto(info.need), free: GameInfo.sizeAuto(info.free) });
        dialog.buttons = [{ text: Tr.tr("prompt.ok"), style: "primary", action: () => dialog.close() }];
        dialog.defaultIndex = 0;
        dialog.open();
    }
    function showInstallUnusable(game) {
        dialog.game = game;
        dialog.mode = "installUnusable";
        dialog.contentWidth = Theme.px(560);
        dialog.icon = "";
        dialog.prompts = dialog.defaultPrompts;
        dialog.backAction = null;
        dialog.title = Tr.tr("installer.unusable.title", { title: game.title });
        dialog.text = Tr.tr(GameInfo.installerKind(game) === "galaxy" ? "installer.unusable.galaxy" : "installer.unusable.unknown");
        dialog.buttons = [{ text: Tr.tr("prompt.ok"), style: "primary", action: () => dialog.close() }];
        dialog.defaultIndex = 0;
        dialog.open();
    }
    function requestInstall(game) {
        const req = diskGames.installRequest(game, Tr.language);
        const waiting = (GameInfo.installs || []).length > 0;     // another one in progress: this one waits
        Nav.feedback("launch");
        owneetd.post("/v1/installs", req, (status, data) => {
            if (status === 202) {
                if (waiting) toast.show(Tr.tr("toast.install.queued", { title: game.title }));
                return;
            }
            const code = data && data.error ? data.error.code : "";
            if (code === "installs.no_space") { root.inspectInstaller(game, info => root.showInstallSpace(game, info)); return; }
            toast.show(Tr.tr(code === "installs.exists" ? "error.install.exists" : "error.installer"));
            Nav.feedback("error");
        });
    }
    function askStopInstall(game) {
        const inst = GameInfo.install(game);
        if (!inst) return;
        dialog.game = null;
        dialog.mode = "installStop";
        dialog.contentWidth = 0;
        dialog.icon = "";
        dialog.prompts = dialog.defaultPrompts;
        dialog.backAction = null;
        dialog.title = Tr.tr("installer.stop.title", { title: game.title });
        dialog.text = Tr.tr("installer.stop.text");
        dialog.buttons = [
            { text: Tr.tr("installer.stop.go"), style: "primary", action: () => {
                dialog.close(true);
                owneetd.post("/v1/installs/stop", { dest: inst.dest }, status => {
                    toast.show(Tr.tr(status === 204 ? "toast.install.stopped" : "error.installer"));
                });
            } },
            { text: Tr.tr("installer.stop.keep"), action: () => dialog.close() }];
        dialog.defaultIndex = 0;
        dialog.open();
    }
    // Uninstalling a game the console installed: its folder on the console's disk goes, its
    // installer stays where it is (it can be installed again); its saves in its prefix stay
    function askRemoveInstalled(game) {
        const dir = String(game.extra["disk.dir"]);
        const from = String(game.extra["disk.installed.from"] || "");
        const vol = storage.volumes().find(v => v.key !== "internal" && from.startsWith(v.key + "/"));
        dialog.game = null;
        dialog.mode = "installRemove";
        dialog.contentWidth = Theme.px(560);
        dialog.icon = "";
        dialog.prompts = dialog.defaultPrompts;
        dialog.backAction = null;
        dialog.title = Tr.tr("installer.uninstall.title", { title: game.title });
        dialog.text = Tr.tr("installer.uninstall.text", { size: GameInfo.sizeAuto(diskGames.folderSize(dir)),
                                                           disk: vol ? diskName(vol.key) : diskName("internal") });
        dialog.buttons = [
            { text: Tr.tr("uninstall.go"), style: "primary", action: () => {
                dialog.close(true);
                owneetd.post("/v1/installs/uninstall", { dest: dir }, status => {
                    if (status !== 204) { toast.show(Tr.tr("error.installer")); Nav.feedback("error"); return; }
                    toast.show(Tr.tr("toast.install.removed", { title: game.title }));
                    root.readGamesQuietly();
                });
            } },
            { text: Tr.tr("prompt.cancel"), action: () => dialog.close() }];
        dialog.defaultIndex = 1;             // Cancel first: nothing removed by accident
        dialog.open();
    }
    // The end of an install: the installed copy replaces the game (read again quietly), a notice
    // says so; or why it stopped
    function installEnded(type, data) {
        if (!data) return;
        if (type === "install.done") {
            notify({ key: "notices.install.ready", args: { name: data.title }, icon: "check", kind: "info", action: "play", game: data.title });
            readGamesQuietly();
        } else if (data.reason === "source_gone") {
            notify({ key: "notices.install.stopped", args: { name: data.title }, icon: "usb", kind: "info" });
        } else if (data.reason === "no_space") {
            notify({ key: "notices.install.space", args: { name: data.title }, icon: "alert", kind: "attention" });
        } else if (data.reason === "windows") {
            toast.show(Tr.tr("error.install.windows"));
        } else if (data.reason !== "cancelled") {
            notify({ key: "notices.install.failed", args: { name: data.title }, icon: "alert", kind: "attention" });
        }
    }
    function readGamesQuietly() {
        disksPending = true;
        checkDisks();
    }
    // The home screen's notice of the install in progress (and how many wait)
    readonly property var installNotices: {
        const list = GameInfo.installs || [];
        const cur = list.find(i => i.state === "installing");
        if (!cur) return [];
        const text = (cur.total > 0 ? Tr.tr("home.notice.install", { title: cur.title, p: Math.floor(cur.done * 100 / cur.total) })
                                    : Tr.tr("home.notice.install.busy", { title: cur.title }))
                     + (list.length > 1 ? "  ·  " + Tr.tr("home.notice.more", { n: list.length - 1 }) : "");
        return [{ text: text, game: games.find(g => g.title === cur.title) || null }];
    }

    // ---- Games on a disk (4.3, approved demo design/demos/4.3-games-on-disk.html)
    // "Which file starts {title}?": every file the console could start, with its folder; the
    // choice is remembered (diskGames) and changed from Options
    function chooseFile(game, thenStart) {
        const files = GameInfo.diskFiles(game);
        dialog.game = game;
        dialog.mode = "chooseFile";
        dialog.files = files;
        dialog.thenStart = thenStart;
        dialog.contentWidth = Theme.px(560);
        dialog.title = Tr.tr("choose.title", { title: game.title });
        dialog.text = Tr.tr("choose.text");
        dialog.buttons = [];
        dialog.initialItem = fileRepeater.itemAt(Math.max(0, files.indexOf(diskGames.chosen(game))));
        dialog.open();
    }
    function fileChosen(file) {
        const game = dialog.game, thenStart = dialog.thenStart;
        if (!diskGames.choose(game, file)) {
            dialog.close();
            return;
        }
        dialog.close(true);
        if (thenStart)
            root.launch(game);
        else
            toast.show(Tr.tr("toast.chosen", { title: game.title, file: file.split("/").pop() }));
    }
    // A disk's name for people: its label, or "Internal disk" for the console's own
    function diskName(root) {
        if (root === "internal")
            return Tr.tr("disk.internal");
        const v = storage.volumes().find(v => v.key === root);
        return v && v.name ? v.name : root.split("/").pop();
    }
    // A disk plugged in or out: its games come and go and a notice says so; disks without a Games
    // folder change nothing. The games are read again quietly: the interface stays on screen and
    // the games change once the new ones are ready (diskGames.quietReading). While a game runs,
    // the reading waits for the game's end. The notice is kept in the theme's memory, in case
    // something else reads the games again meanwhile and rebuilds the interface.
    property var knownVolumes: []
    property bool disksPending: false
    onRunningGameChanged: if (disksPending && !runningGame) checkDisks()
    function volumeKeys() { return storage.volumes().map(v => v.key).filter(k => k !== "internal"); }
    function checkDisks() {
        const now = volumeKeys();
        const plugged = now.filter(k => knownVolumes.indexOf(k) < 0 && diskGames.hasGames(k));
        const unplugged = knownVolumes.filter(k => now.indexOf(k) < 0 && games.some(g => GameInfo.diskRoot(g) === k));
        knownVolumes = now;
        if (plugged.length > 0 || unplugged.length > 0) {
            const prev = api.memory.has("diskNotice") ? JSON.parse(api.memory.get("diskNotice")) : { plugged: [], unplugged: [] };
            api.memory.set("diskNotice", JSON.stringify({ time: Date.now(), plugged: prev.plugged.concat(plugged),
                                                          unplugged: prev.unplugged.concat(unplugged) }));
            disksPending = true;
        }
        if (!disksPending || runningGame)
            return;
        disksPending = false;
        console.log("Disks changed: reading the games again, quietly");
        // a window about a game of the disk that is gone closes
        if (dialog.visible && dialog.game && unplugged.indexOf(GameInfo.diskRoot(dialog.game)) >= 0)
            dialog.close(true);
        diskGames.quietReading = true;
        Internal.settings.reloadProviders();
    }
    function diskNotices() {
        if (!api.memory.has("diskNotice"))
            return;
        const d = JSON.parse(api.memory.get("diskNotice"));
        api.memory.unset("diskNotice");
        if (Date.now() - d.time > 600000)
            return;
        d.plugged.forEach(k => {
            const n = games.filter(g => GameInfo.diskRoot(g) === k).length;
            if (n > 0)
                notify({ key: "notices.disk.plugged", args: { n: n }, icon: "usb", kind: "info" });
        });
        if (d.unplugged.length > 0)
            notify({ key: "notices.disk.unplugged", args: {}, icon: "usb", crossed: true, kind: "info" });
    }
    Connections {
        target: storage
        function onVolumesChanged() { root.checkDisks(); }
    }
    Connections {                         // the games read again quietly are ready
        target: api
        function onGamedataReady() {
            root.refreshGames();
            root.diskNotices();
        }
    }

    // ---- Updates from the USB stick (testers, 4.2b): check, install, restart
    property var updateStatus: null
    function checkUpdates() {
        dialog.game = null;
        dialog.mode = "update";
        dialog.contentWidth = Theme.px(520);
        dialog.title = Tr.tr("update.title");
        dialog.text = "";
        dialog.updateBusy = true;
        dialog.updateLines = [];
        dialog.updateNote = Tr.tr("update.checking");
        dialog.buttons = [];
        dialog.open();
        owneetd.post("/v1/update/check", {}, (status, data) => {
            if (status === 202) return;
            const code = data && data.error ? data.error.code : "";
            showUpdate({ state: "failed", message: Tr.tr(code === "update.busy" ? "update.busy" : "update.failed") });
        });
    }
    function showUpdate(st) {
        if (!dialog.visible || dialog.mode !== "update")
            return;
        const lines = (st.packages || []).map(p => p.name + "  " + p.from + "  →  " + p.to);
        const ok = { text: Tr.tr("prompt.ok"), style: "primary", action: () => dialog.close() };
        dialog.updateBusy = st.state === "installing";
        dialog.updateLines = st.state === "available" || st.state === "installing" ? lines : [];
        if (st.state === "available") {
            dialog.updateNote = Tr.tr("update.available");
            dialog.buttons = [
                { text: Tr.tr("update.install"), style: "primary", action: () => {
                    dialog.updateBusy = true;
                    dialog.updateNote = Tr.tr("update.installing");
                    dialog.buttons = [];
                    owneetd.post("/v1/update/apply", {}, (status) => {
                        if (status !== 202) showUpdate({ state: "failed", message: Tr.tr("update.failed") });
                    });
                } },
                { text: Tr.tr("prompt.cancel"), action: () => dialog.close() }];
            dialog.defaultIndex = 0;
        } else if (st.state === "installing") {
            dialog.updateNote = Tr.tr("update.installing");
            dialog.buttons = [];
        } else if (st.state === "done") {
            Nav.feedback("notify");
            dialog.updateNote = Tr.tr("update.done");
            dialog.buttons = [
                { text: Tr.tr("update.restart"), style: "primary", action: () => {
                    dialog.close(true);
                    owneetd.post("/v1/power/restart", {}, () => {});
                } },
                { text: Tr.tr("update.later"), action: () => dialog.close() }];
            dialog.defaultIndex = 0;
        } else {
            if (st.state === "failed") Nav.feedback("error");
            dialog.updateNote = Tr.tr(st.state === "none" ? "update.none" : st.state === "no_stick" ? "update.no_stick"
                                      : st.state === "not_tester" ? "update.not_tester" : "update.failed");
            dialog.buttons = [ok];
            dialog.defaultIndex = 0;
        }
        Qt.callLater(dialog.focusDefault);
    }

    // ---- Steam (4.2, 4.2c): its store, installing and uninstalling; owneetd starts Steam when
    // needed. Steam's window shows only for its store and its confirmations.
    function openSteam(page) {
        if (steamOffline(page === "store" ? "store" : "play")) return;
        withSteamNotice(() => {
            if (page === "store") root.storeVisited = true;
            steamRequest({ open: page }, null);
        });
    }
    // Asks owneetd; Steam starts if needed (its first start: "Preparing Steam")
    function steamRequest(body, done) {
        if (!owneetd.connected) {
            toast.show(Tr.tr("error.daemon.unavailable"));
            Nav.feedback("error");
            return;
        }
        owneetd.get("/v1/steam", (status, st) => {
            if (status !== 200 || !st.installed) {
                toast.show(Tr.tr("error.apps.unavailable"));
                Nav.feedback("error");
                return;
            }
            if (!st.allowed) {
                root.showSteamNeedsInstall();
                return;
            }
            Nav.feedback("launch");
            owneetd.post("/v1/steam", Object.assign({ language: Tr.language }, body), (status2, data) => {
                if (status2 === 202) {
                    if (!st.running) steamPrep.start(!st.prepared, "");
                    if (done) done();
                    return;
                }
                const code = data && data.error ? data.error.code : "";
                if (code === "steam.needs_install") { root.showSteamNeedsInstall(); return; }
                toast.show(Tr.tr(code && Tr.tr("error." + code) !== "error." + code ? "error." + code : "error.apps.failed"));
                Nav.feedback("error");
            });
        });
    }
    // Without internet, what needs it stops here with a clear message (owner, 2026-10-14):
    // installing, the store, and Steam's first start or sign-in ("play" when Steam is ready
    // needs nothing). True when blocked.
    function steamOffline(kind) {
        if (online)
            return false;
        const st = GameInfo.steam;
        const ready = st !== null && st.prepared && st.account;
        if (kind === "play" && ready)
            return false;
        if (kind === "play" || (kind === "store" && !ready))
            kind = "first";
        dialog.game = null;
        dialog.mode = "offline";
        dialog.contentWidth = 0;
        dialog.icon = "wifi";
        dialog.iconCrossed = true;
        dialog.iconAttention = true;
        dialog.title = Tr.tr("offline.title");
        dialog.text = Tr.tr("offline." + kind);
        dialog.buttons = [
            { text: Tr.tr("offline.network"), style: "primary", action: () => {
                dialog.close(true);
                settings.section = "network";
                root.goSection("settings");
            } },
            { text: Tr.tr("prompt.close"), action: () => dialog.close() }];
        dialog.defaultIndex = 0;
        Nav.feedback("notify-error");
        dialog.open(true);
        return true;
    }
    // Before Steam's window (installing, uninstalling, the store), on PCs where it may show display
    // errors (NVIDIA's driver) only: that they are harmless; every time until "Don't show again"
    // (owner, 2026-10-14). Since Steam's window is readable there (repaint overlay, 2026-10-15) it
    // no longer says what to press (owner, 2026-10-15).
    function withSteamNotice(next) {
        withSteamReturn(() => nvidiaNotice(next));
    }
    // How to come back from Steam (owner, 2026-10-15): at the first time Steam's window opens
    // after each start of the console, until "Don't show again". Hold Home comes back; a Steam
    // page that keeps loading is left by holding Home and closing Steam (Menu on its tile).
    function withSteamReturn(next) {
        if (api.memory.has("steamReturnNoticeHidden") || !systemInfo.onceThisBoot("steam-return-notice")) {
            next();
            return;
        }
        dialog.game = null;
        dialog.mode = "steamReturn";
        dialog.contentWidth = 0;
        dialog.icon = "controller";
        dialog.title = Tr.tr("steam.return.title");
        dialog.text = Tr.tr("steam.return.text");
        noticeNever.checked = false;
        dialog.buttons = [
            { text: Tr.tr("steam.notice.go"), style: "primary", action: () => {
                if (noticeNever.checked) api.memory.set("steamReturnNoticeHidden", true);
                dialog.close(true);
                next();
            } },
            { text: Tr.tr("prompt.cancel"), action: () => dialog.close() }];
        dialog.defaultIndex = 0;
        dialog.open();
    }
    function nvidiaNotice(next) {
        const nvidia = GameInfo.steam !== null && GameInfo.steam.nvidia;
        if (!nvidia || api.memory.has("steamConfirmNoticeHidden")) {
            next();
            return;
        }
        dialog.game = null;
        dialog.mode = "steamNotice";
        dialog.contentWidth = 0;
        dialog.icon = "warning";
        dialog.title = Tr.tr("steam.notice.title");
        dialog.text = Tr.tr("steam.notice.text");
        noticeNever.checked = false;
        dialog.buttons = [
            { text: Tr.tr("steam.notice.go"), style: "primary", action: () => {
                if (noticeNever.checked) api.memory.set("steamConfirmNoticeHidden", true);
                dialog.close(true);
                next();
            } },
            { text: Tr.tr("prompt.cancel"), action: () => dialog.close() }];
        dialog.defaultIndex = 0;
        dialog.open();
    }
    // A licence agreement Steam asks to accept at the first start (owner, 2026-10-14): Steam's
    // window may not be readable, and its selection starts on "Decline". The console shows the
    // agreement's official address as a code to read on a phone, and what to press; owneetd shows
    // Steam at once for that start (ask). Known from Steam's caches; remembered here once the game
    // has started (it was accepted).
    function agreementPending(game) {
        const id = GameInfo.appid(game);
        return id !== "" && !!game.extra["steam.eula.url"] && !api.memory.has("steamEulaDone." + id);
    }
    function agreementAccepted(appid) {
        const g = steamGame(appid);
        if (g && g.extra["steam.eula.url"] && !api.memory.has("steamEulaDone." + appid))
            api.memory.set("steamEulaDone." + appid, true);
    }
    function showAgreement(game) {
        const id = GameInfo.appid(game);
        dialog.game = null;
        dialog.mode = "agreement";
        // Steam's caches give "http://store.steampowered.com//eula/…": the secure address, one slash
        dialog.agreementUrl = String(game.extra["steam.eula.url"]).replace(/^http:/, "https:").replace(/([^:\/])\/\/+/g, "$1/");
        dialog.contentWidth = Theme.px(560);
        dialog.icon = "";
        dialog.title = Tr.tr("eula.title", { title: game.title });
        dialog.text = Tr.tr("eula.text");
        dialog.buttons = [
            { text: Tr.tr("steam.notice.go"), style: "primary", action: () => {
                dialog.close(true);
                root.withSteamReturn(() => owneetd.post("/v1/steam", { ask: id }, () => root.launch(game, true)));
            } },
            { text: Tr.tr("prompt.cancel"), action: () => dialog.close() }];
        dialog.defaultIndex = 0;
        dialog.open();
    }
    // A Steam game whose Linux version does not start (4.5, approved demo): owneetd read Steam's
    // "missing program" and kept Steam's error hidden. Try: owneetd closes Steam, sets the game's
    // version for Windows and starts it again; "Starting …" stays meanwhile
    function askWindowsVersion(appid) {
        const game = steamGame(appid);
        const title = game ? game.title : "Steam";
        steamPrep.close(true);
        dialog.game = null;
        dialog.mode = "steamBroken";
        dialog.contentWidth = 0;
        dialog.icon = "";
        dialog.title = Tr.tr("steam.broken.title", { title: title });
        dialog.text = Tr.tr("steam.broken.text");
        dialog.buttons = [
            { text: Tr.tr("steam.broken.try"), style: "primary", action: () => {
                dialog.close(true);
                Nav.feedback("launch");
                steamPrep.start(false, title);
                steamPrep.steamRestart = true;
                owneetd.post("/v1/steam", { windows: appid, name: title, language: Tr.language }, status => {
                    if (status === 202) return;
                    steamPrep.close(true);
                    toast.show(Tr.tr("error.apps.failed"));
                    Nav.feedback("error");
                });
            } },
            { text: Tr.tr("prompt.cancel"), action: () => dialog.close() }];
        dialog.defaultIndex = 0;
        dialog.open();
    }
    function askInstall(game) {
        if (steamOffline("install")) return;
        const id = GameInfo.appid(game);
        dialog.game = null;
        dialog.mode = "install";
        dialog.contentWidth = 0;
        dialog.icon = "download";
        dialog.title = Tr.tr("install.title", { title: game.title });
        dialog.text = Tr.tr("install.text");
        dialog.buttons = [
            { text: Tr.tr("install.go"), style: "primary", action: () => {
                dialog.close(true);
                root.withSteamNotice(() => root.steamRequest({ install: id }, () => {
                    const asked = Object.assign({}, root.steamAsked);
                    asked[id] = { title: game.title, kind: "install" };
                    root.steamAsked = asked;
                }));
            } },
            { text: Tr.tr("prompt.cancel"), action: () => dialog.close() }];
        dialog.defaultIndex = 0;
        dialog.open();
    }
    // Uninstalling, and cancelling a download: Steam's steam://uninstall, after the console asks
    function askUninstall(game, cancelling) {
        const id = GameInfo.appid(game);
        const k = cancelling ? "cancelinst" : "uninstall";
        dialog.game = null;
        dialog.mode = "uninstall";
        dialog.contentWidth = 0;
        dialog.icon = "trash";
        dialog.title = Tr.tr(k + ".title", { title: game.title });
        dialog.text = Tr.tr(k + ".text");
        dialog.buttons = [
            { text: Tr.tr(k + ".go"), action: () => {
                dialog.close(true);
                root.withSteamNotice(() => root.steamRequest({ uninstall: id }, () => {
                    const asked = Object.assign({}, root.steamAsked);
                    asked[id] = { title: game.title, kind: cancelling ? "cancel" : "uninstall" };
                    root.steamAsked = asked;
                }));
            } },
            { text: Tr.tr(cancelling ? "cancelinst.keep" : "prompt.cancel"), style: "primary", action: () => dialog.close() }];
        dialog.defaultIndex = 1;             // keep it: nothing removed by accident
        dialog.open();
    }
    // The live system keeps Steam's files in RAM: with less than 12 GB, install first
    function showSteamNeedsInstall() {
        dialog.game = null;
        dialog.mode = "steamInstall";
        dialog.contentWidth = 0;
        dialog.title = Tr.tr("steam.install.title");
        dialog.text = Tr.tr("steam.install.text");
        dialog.buttons = [{ text: Tr.tr("prompt.ok"), style: "primary", action: () => dialog.close() }];
        dialog.defaultIndex = 0;
        Nav.feedback("error");
        dialog.open(true);
    }
    // Closing Steam from the console (owner, 2026-10-15): Menu on the Steam tile of the home screen
    // while Steam runs, also to leave a Steam page that never finishes loading. Downloads stop; a
    // Steam game running closes too. Cancel first.
    function confirmCloseSteam() {
        const game = runningGame && String(runningGame.id).startsWith("steam:") ? runningGame : null;
        dialog.game = null;
        dialog.mode = "closeSteam";
        dialog.contentWidth = 0;
        dialog.title = Tr.tr("steam.close.title");
        dialog.text = Tr.tr("steam.close.text") + (game ? "\n\n" + Tr.tr("steam.close.game", { title: game.name }) : "");
        dialog.buttons = [
            { text: Tr.tr("steam.close.go"), action: () => {
                dialog.close();
                closingApps["steam"] = true;             // closed on purpose: no "closed unexpectedly"
                if (game) closingApps[game.id] = true;
                owneetd.post("/v1/apps/steam/close", {}, (status) => {
                    if (status >= 300 || status === 0) toast.show(Tr.tr("steam.close.failed"));
                });
            } },
            { text: Tr.tr("prompt.cancel"), style: "primary", action: () => dialog.close() }
        ];
        dialog.defaultIndex = 1;
        dialog.open();
    }
    // Library → Add games (approved demo 4.2): where games come from
    function showAddGames(selectDisk, quiet) {
        dialog.game = null;
        dialog.mode = "addGames";
        dialog.contentWidth = addChoices.implicitWidth;
        dialog.title = Tr.tr("add.title");
        dialog.text = "";
        dialog.buttons = [];
        dialog.backAction = null;
        dialog.initialItem = addRepeater.itemAt(selectDisk ? 1 : 0);
        dialog.prompts = Qt.binding(() => dialog.defaultPrompts);
        dialog.open(quiet);
    }
    function addFrom(source) {
        if (source === "steam") {
            dialog.close(true);
            root.openSteam("store");            // owned games are in the library already (4.2c)
        } else {
            showDiskGuide();
        }
    }
    // Read only: how to put games on a disk; B goes back to the choice
    function showDiskGuide() {
        dialog.mode = "diskGuide";
        dialog.contentWidth = Theme.px(720);
        dialog.title = Tr.tr("disk.title");
        dialog.text = "";
        dialog.initialItem = null;
        dialog.buttons = [{ text: Tr.tr("prompt.back"), style: "primary", action: () => { Nav.feedback("sheet-close"); root.showAddGames(true, true); } }];
        dialog.defaultIndex = 0;
        dialog.prompts = [{ buttons: ["a", "b"], label: Tr.tr("prompt.back") }];
        dialog.backAction = () => root.showAddGames(true, true);
        dialog.focusDefault();
    }

    // ---- Settings windows
    function choose(title, options, current, pick) {   // options: [[value, label]]
        dialog.game = null;
        dialog.mode = "choose";
        dialog.contentWidth = 0;
        dialog.title = title;
        dialog.text = "";
        dialog.buttons = options.map(o => ({ text: o[1], style: o[0] === current ? "primary" : "secondary",
                                             action: () => { dialog.close(); pick(o[0]); } }));
        dialog.defaultIndex = Math.max(0, options.findIndex(o => o[0] === current));
        dialog.open();
    }
    function chooseOutput(outputs, current) {
        choose(Tr.tr("sound.output"), outputs.map(o => [o.id, o.name]), current,
               id => owneetd.put("/v1/audio/output", { id: id }));
    }
    function chooseKeyboard(layouts, current) {
        const same = Tr.tr("language.keyboard.same", { layout: Tr.tr("keyboard." + keyboardFor(Tr.language, layouts)) });
        choose(Tr.tr("language.keyboard"), [["same", same]].concat(layouts.map(l => [l, Tr.tr("keyboard." + l)])), current,
               v => { Theme.keyboardSetting = v; });
    }
    // The on-screen keyboard's layout (owneetd): the chosen one, or the language's
    function keyboardFor(language, layouts) {
        const byLanguage = { en: "us", it: "it" };
        const l = byLanguage[language] || "us";
        return !layouts || layouts.indexOf(l) >= 0 ? l : "us";
    }
    function applyKeyboard() {
        if (!loaded) return;
        const layout = Theme.keyboardSetting === "same" ? keyboardFor(Tr.language) : Theme.keyboardSetting;
        owneetd.put("/v1/input/layout", { layout: layout });
    }
    Connections {
        target: i18n
        function onLanguageChanged() { root.applyKeyboard(); }
    }
    // Quick menu → Power options (3.13): Sleep, Restart, Shut down, Cancel (selected first)
    function showPowerOptions() {
        dialog.game = null;
        dialog.mode = "powerOptions";
        dialog.contentWidth = powerChoices.implicitWidth;
        dialog.title = Tr.tr("quick.power");
        dialog.text = runningGame ? Tr.tr("quick.power.game") : "";
        dialog.buttons = [];
        dialog.initialItem = choiceRepeater.itemAt(3);   // Cancel
        dialog.open();
    }
    function power(action) {
        quick.close(true);
        dialog.close(true);
        owneetd.post("/v1/power/" + action, {}, status => {
            if (status >= 300 || status === 0) toast.show(Tr.tr("error.power"));
        });
    }
    function confirmPower(action) {
        dialog.game = null;
        dialog.mode = "power";
        dialog.contentWidth = 0;
        dialog.title = Tr.tr("system.power.confirm." + action);
        dialog.text = runningGame ? Tr.tr("system.power.game") : "";
        dialog.buttons = [
            { text: Tr.tr("system.power." + action), action: () => {
                dialog.close();
                owneetd.post("/v1/power/" + action, {}, status => {
                    if (status >= 300 || status === 0) toast.show(Tr.tr("error.power"));
                });
            } },
            { text: Tr.tr("prompt.cancel"), style: "primary", action: () => dialog.close() }
        ];
        dialog.defaultIndex = 1;             // Cancel first
        dialog.open();
    }
    // Network (part 2): connect, with a password when the network needs one; options of a saved
    // network. The password is typed with a keyboard until the on-screen keyboard
    // ([[step-3.10-on-screen-keyboard]]).
    function connectNetwork(net, problem) {
        if (net.security === "enterprise" || net.security === "wep") {
            toast.show(Tr.tr("error.network.unsupported_security"));
            Nav.feedback("error");
            return;
        }
        const needsPassword = net.security !== "open" && (!net.known || problem !== undefined);
        dialog.game = null;
        dialog.mode = "connect";
        dialog.net = net;
        dialog.contentWidth = Theme.px(460);
        dialog.title = net.ssid;
        dialog.text = "";
        dialog.problem = problem ? Tr.tr("error." + problem) : "";
        dialog.needsPassword = needsPassword;
        dialog.connecting = false;
        dialog.prompts = dialog.defaultPrompts;
        passwordInput.text = "";
        showPassword.checked = false;
        if (!needsPassword) {
            if (problem === undefined) Nav.feedback("sheet-open");
            dialog.visible = true;
            startConnect(net, "");
            return;
        }
        dialog.buttons = [
            { text: Tr.tr("network.connect"), style: "primary", action: () => root.startConnect(net, passwordInput.text) },
            { text: Tr.tr("prompt.cancel"), action: () => dialog.close() }
        ];
        dialog.initialItem = passwordField;
        if (problem !== undefined) {         // opened again after a wrong password: no new sound
            dialog.visible = true;
            dialog.focusDefault();
        } else {
            dialog.open();
        }
        // the on-screen keyboard opens by itself under the field (demo 3.10)
        Qt.callLater(() => { if (dialog.visible && dialog.mode === "connect") osk.open(passwordInput, true); });
    }
    function startConnect(net, password) {
        osk.close(true);
        quietNetwork();                          // "Connected to …" comes as a toast
        dialog.connecting = true;
        dialog.problem = "";
        dialog.buttons = [];
        dialog.prompts = [{ buttons: ["b"], label: Tr.tr("prompt.close") }];   // it goes on in the background
        dialog.focusDefault();
        owneetd.post("/v1/network/wifi/connect", { ssid: net.ssid, password: password }, (status, data) => {
            const code = data && data.error ? data.error.code : "";
            const shown = dialog.visible && dialog.mode === "connect" && dialog.net === net;
            if (status === 204 || status === 200) {
                if (shown) dialog.close(true);
                Nav.feedback("notify");
                toast.show(Tr.tr("network.connected_to", { ssid: net.ssid }));
                return;
            }
            Nav.feedback("error");
            const again = ["network.wrong_password", "network.password_required", "network.invalid_password"];
            if (shown && again.indexOf(code) >= 0) {
                connectNetwork(Object.assign({}, net, { known: false }), code);
                return;
            }
            if (shown) dialog.close(true);
            const key = "error." + code;
            toast.show(code && Tr.tr(key) !== key ? Tr.tr(key, { ssid: net.ssid }) : Tr.tr("error.network.connect_failed", { ssid: net.ssid }));
        });
    }
    function networkOptions(net) {
        dialog.game = null;
        dialog.mode = "network";
        dialog.contentWidth = 0;
        dialog.title = net.ssid;
        dialog.text = "";
        const done = status => { if (status >= 300 || status === 0) toast.show(Tr.tr("error.network")); };
        dialog.buttons = (net.connected ? [{ text: Tr.tr("network.disconnect"), action: () => {
            dialog.close();
            root.quietNetwork();
            owneetd.post("/v1/network/wifi/disconnect", {}, done);
        } }] : []).concat([
            { text: Tr.tr("network.forget"), action: () => {
                dialog.close();
                root.quietNetwork();
                owneetd.remove("/v1/network/wifi/" + encodeURIComponent(net.ssid), status => {
                    if (status === 204 || status === 200) toast.show(Tr.tr("network.forgotten", { ssid: net.ssid }));
                    else done(status);
                });
            } },
            { text: Tr.tr("prompt.cancel"), style: "primary", action: () => dialog.close() }
        ]);
        dialog.defaultIndex = dialog.buttons.length - 1;      // Cancel first
        dialog.open();
    }

    // Controllers and Bluetooth (part 2). A Bluetooth controller can be turned off (disconnected:
    // it stays known and comes back when turned on) or forgotten; wired and adapter ones are
    // removed by unplugging or turning them off.
    function bluetoothAction(method, address, toastKey, name) {
        quietPads[address.toLowerCase()] = Date.now();     // turned off on purpose: no notification
        const path = "/v1/bluetooth/devices/" + address.toUpperCase();
        const done = status => {
            if (status === 204 || status === 200) toast.show(Tr.tr(toastKey, { name: name }));
            else toast.show(Tr.tr("error.bluetooth"));
        };
        if (method === "forget") owneetd.remove(path, done);
        else owneetd.post(path + "/" + method, {}, done);
    }
    function controllerOptions(pad) {
        dialog.game = null;
        dialog.mode = "controller";
        dialog.contentWidth = 0;
        dialog.title = pad.name;
        // a Bluetooth controller's id is its address
        const address = pad.device ? pad.device.address : /^([0-9a-f]{2}:){5}[0-9a-f]{2}$/i.test(pad.id) ? pad.id : "";
        if (pad.connection !== "bluetooth" || address === "") {
            dialog.text = Tr.tr(pad.connection === "usb" ? "pad.unplug" : pad.connection === "dongle" ? "pad.dongle" : "pad.other");
            dialog.buttons = [{ text: Tr.tr("prompt.ok"), style: "primary", action: () => dialog.close() }];
            dialog.defaultIndex = 0;
            dialog.open();
            return;
        }
        dialog.text = Tr.tr("pad.remove.text");
        dialog.buttons = [
            { text: Tr.tr("pad.off"), action: () => { dialog.close(); root.bluetoothAction("disconnect", address, "pad.turned_off", pad.name); } },
            { text: Tr.tr("pad.forget"), action: () => { dialog.close(); root.bluetoothAction("forget", address, "pad.forgotten", pad.name); } },
            { text: Tr.tr("prompt.cancel"), style: "primary", action: () => dialog.close() }
        ];
        dialog.defaultIndex = 2;             // Cancel first
        dialog.open();
    }
    function deviceOptions(device) {
        dialog.game = null;
        dialog.mode = "device";
        dialog.contentWidth = 0;
        dialog.title = device.name;
        dialog.text = "";
        dialog.buttons = (device.connected ? [{ text: Tr.tr("pad.off"), action: () => {
            dialog.close();
            root.bluetoothAction("disconnect", device.address, "pad.turned_off", device.name);
        } }] : []).concat([
            { text: Tr.tr("pad.forget"), action: () => { dialog.close(); root.bluetoothAction("forget", device.address, "pad.forgotten", device.name); } },
            { text: Tr.tr("prompt.cancel"), style: "primary", action: () => dialog.close() }
        ]);
        dialog.defaultIndex = dialog.buttons.length - 1;
        dialog.open();
    }
    // "Pair a new controller": owneetd pairs any gamepad in pairing mode for two minutes, also
    // with a controller already connected; the window closes when one is paired.
    function pairController() {
        dialog.game = null;
        dialog.mode = "pair";
        dialog.contentWidth = Theme.px(922);
        dialog.title = Tr.tr("pair.title");
        dialog.text = "";
        dialog.pairState = "searching";
        dialog.pairName = "";
        dialog.buttons = [{ text: Tr.tr("prompt.cancel"), style: "primary", action: () => dialog.close() }];
        dialog.defaultIndex = 0;
        dialog.prompts = pairPrompts;
        dialog.open();
        startPairing();
    }
    readonly property var pairPrompts: [{ buttons: ["a"], label: Tr.tr("prompt.cancel") }, { buttons: ["b"], label: Tr.tr("prompt.back") }]
    function startPairing() {
        dialog.pairState = "searching";
        owneetd.post("/v1/bluetooth/auto-pair", { enabled: true, seconds: 120 }, status => {
            if (status !== 204 && status !== 200 && dialog.visible && dialog.mode === "pair") {
                dialog.close(true);
                Nav.feedback("error");
                toast.show(Tr.tr("error.bluetooth"));
            }
        });
    }
    function pairingEvent(type, data) {
        if (!dialog.visible || dialog.mode !== "pair")
            return;
        if (type === "bluetooth.paired") {
            dialog.close(true);
            Nav.feedback("notify");
            toast.show(Tr.tr("pair.done", { name: data.name || Tr.tr("pair.controller") }));
        } else if (type === "bluetooth.pairing") {
            dialog.pairState = "pairing";
            dialog.pairName = data.name || Tr.tr("pair.controller");
        } else if (type === "bluetooth.pair_failed") {
            Nav.feedback("error");
            dialog.pairState = "failed";
            dialog.pairName = data.name || Tr.tr("pair.controller");
        } else if (type === "bluetooth.auto_pair" && data.enabled === false && dialog.pairState !== "") {
            dialog.pairState = "timeout";            // two minutes without a controller
            dialog.buttons = [
                { text: Tr.tr("pair.again"), style: "primary", action: () => {
                    dialog.buttons = [{ text: Tr.tr("prompt.cancel"), style: "primary", action: () => dialog.close() }];
                    dialog.prompts = root.pairPrompts;
                    dialog.focusDefault();
                    root.startPairing();
                } },
                { text: Tr.tr("prompt.cancel"), action: () => dialog.close() }
            ];
            dialog.prompts = dialog.defaultPrompts;
            dialog.focusDefault();
        }
    }

    // Display (part 3): the screen, its resolution and refresh rate. Every change asks "Keep this?"
    // and goes back after 15 s without an answer (owneetd keeps the time).
    function chooseDisplay(what, state) {
        const label = s => s.internal ? Tr.tr("display.internal")
                                       : (s.name || Tr.tr("display.screen.unnamed")) + " (" + s.connector + ")";
        const res = m => m.split("@")[0], rate = m => parseInt(m.split("@")[1]);
        const put = (path, body) => owneetd.put(path, body, (status, data) => {
            if (status === 204 || status === 200) return;
            const code = data && data.error ? data.error.code : "";
            if (api.memory.has("displayReturn")) api.memory.unset("displayReturn");   // nothing moved
            Nav.feedback("error");
            toast.show(Tr.tr(code === "display.apps_open" ? "error.display.apps_open" : "error.display"));
        });
        if (what === "screen") {
            const current = (state.screens.find(s => s.active) || {}).id;
            choose(Tr.tr("display.screen"), state.screens.map(s => [s.id, label(s)]), current, id => {
                if (id === current) return;
                // the new screen may stay black while it wakes up: say so first (owner, 2026-10-10)
                const target = state.screens.find(s => s.id === id);
                dialog.game = null;
                dialog.mode = "displayMove";
                dialog.contentWidth = 0;
                dialog.title = Tr.tr("display.move.title", { screen: label(target) });
                dialog.text = Tr.tr("display.move.text");
                dialog.buttons = [
                    { text: Tr.tr("display.move"), action: () => {
                        dialog.close(true);
                        api.memory.set("displayReturn", Date.now());   // back here after the restart
                        put("/v1/display/screen", { id: id });
                    } },
                    { text: Tr.tr("prompt.cancel"), style: "primary", action: () => dialog.close() }
                ];
                dialog.defaultIndex = 1;      // Cancel first
                dialog.open();
            });
            return;
        }
        const modes = state.modes;
        // the resolution in use: the chosen one, or the screen's preferred (listed first)
        const shown = state.mode !== "auto" ? state.mode : modes[0];
        if (what === "resolution") {
            const sizes = [];
            modes.forEach(m => { if (sizes.indexOf(res(m)) < 0) sizes.push(res(m)); });
            const options = [["auto", Tr.tr("display.auto")]].concat(sizes.map(r => [r, r.replace("x", " \u00d7 ")]));
            choose(Tr.tr("display.resolution"), options, state.mode === "auto" ? "auto" : res(state.mode), v => {
                if (v === "auto") { put("/v1/display/mode", { mode: "auto" }); return; }
                // keep the refresh rate when the new resolution has it, else its fastest
                const same = modes.find(m => m === v + "@" + rate(shown));
                const fastest = modes.filter(m => res(m) === v).sort((a, b) => rate(b) - rate(a))[0];
                const mode = same || fastest;
                if (mode !== state.mode) put("/v1/display/mode", { mode: mode });
            });
        } else {
            const rates = modes.filter(m => res(m) === res(shown)).sort((a, b) => rate(b) - rate(a));
            const options = [["auto", Tr.tr("display.auto")]].concat(rates.map(m => [m, rate(m) + " Hz"]));
            choose(Tr.tr("display.rate"), options, state.mode, v => { if (v !== state.mode) put("/v1/display/mode", { mode: v }); });
        }
    }
    // "Keep this screen / resolution?", with the seconds left; B goes back too
    function confirmDisplay(kind, seconds) {
        if (dialog.visible && dialog.mode === "displayConfirm") {
            dialog.deadline = Date.now() + seconds * 1000;
            dialog.seconds = seconds;
            if (!dialog.activeFocus) dialog.focusDefault();
            return;
        }
        if (dialog.visible)
            dialog.close(true);
        dialog.game = null;
        dialog.mode = "displayConfirm";
        dialog.answered = false;
        dialog.deadline = Date.now() + seconds * 1000;
        dialog.seconds = seconds;
        dialog.text = Tr.tr("display.keep.text", { n: seconds });
        dialog.contentWidth = Theme.px(640);   // fixed: the countdown must not resize the window
        dialog.title = Tr.tr("display.keep." + kind);
        dialog.buttons = [
            { text: Tr.tr("display.keep"), style: "primary", action: () => {
                dialog.answered = true;
                dialog.close();
                owneetd.post("/v1/display/confirm", {});
                if (api.memory.has("displayReturn")) api.memory.unset("displayReturn");
            } },
            { text: Tr.tr("display.go_back"), action: () => { dialog.close(); } }   // reverts on close
        ];
        dialog.defaultIndex = 0;
        dialog.prompts = [{ buttons: ["a"], label: Tr.tr("prompt.select") }, { buttons: ["b"], label: Tr.tr("display.go_back") }];
        dialog.open();
    }
    Timer {
        interval: 250; repeat: true
        running: dialog.visible && dialog.mode === "displayConfirm"
        // from the clock: a QML Timer follows the frames drawn, not the real time
        onTriggered: dialog.seconds = Math.max(0, Math.ceil((dialog.deadline - Date.now()) / 1000))
    }
    function displayEvent(type, data) {
        if (type === "display.pending") {
            root.confirmDisplay(data.kind, data.seconds);
        } else if (type === "display.reverted" || type === "display.confirmed") {
            if (dialog.visible && dialog.mode === "displayConfirm") {
                dialog.answered = true;
                dialog.close(true);
            }
            if (type === "display.reverted") toast.show(Tr.tr("display.reverted"));
        }
    }
    function checkDisplayPending() {          // e.g. the console has just moved to another screen
        owneetd.get("/v1/display", (status, data) => {
            if (status === 200 && data.pending) root.confirmDisplay(data.pending.kind, data.pending.seconds);
        });
    }

    function showSort() {
        const sorts = ["recent", "name", "time"];
        dialog.game = null;
        dialog.mode = "sort";
        dialog.contentWidth = 0;
        dialog.title = Tr.tr("library.sort.title");
        dialog.text = "";
        dialog.buttons = sorts.map(s => ({ text: Tr.tr("library.sort." + s), style: s === librarySort ? "primary" : "secondary",
                                           action: () => { root.librarySort = s; dialog.close(); } }));
        dialog.defaultIndex = sorts.indexOf(librarySort);
        dialog.open();
    }
    function showDisks(volumes) {
        dialog.game = null;
        dialog.mode = "disks";
        dialog.volumes = volumes;
        dialog.contentWidth = Theme.px(560);
        dialog.title = Tr.tr("disks.title");
        dialog.text = "";
        dialog.buttons = [{ text: Tr.tr("prompt.close"), style: "primary", action: () => dialog.close() }];
        dialog.defaultIndex = 0;
        dialog.open();
    }

    Rectangle {
        anchors.fill: parent
        color: Theme.bg
    }

    // ---- Top bar: wordmark, sections, controller, clock
    Item {
        id: top
        anchors {
            left: parent.left; right: parent.right; top: parent.top
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; topMargin: Theme.safeTop
        }
        height: Math.max(mark.height, tabs.height)

        Text {
            id: mark
            anchors.verticalCenter: parent.verticalCenter
            opacity: intro.visible ? 0 : 1      // the start-up animation brings it here
            textFormat: Text.StyledText
            text: "Owneet<font color=\"" + Theme.accent + "\">OS</font>"
            color: Theme.fg
            font.family: Theme.displayFont
            font.weight: Font.ExtraBold
            font.pixelSize: Theme.px(23)
            font.letterSpacing: -Theme.px(0.7)
        }
        Row {
            id: tabs
            anchors { left: mark.right; leftMargin: Theme.px(36); verticalCenter: parent.verticalCenter }
            spacing: Theme.px(6)
            Glyph { button: "lb"; anchors.verticalCenter: parent.verticalCenter; color: Theme.muted }
            Repeater {
                model: root.sections
                Rectangle {
                    readonly property bool current: modelData === root.section
                    width: tabText.implicitWidth + Theme.px(36)
                    height: tabText.implicitHeight + Theme.px(14)
                    radius: height / 2
                    color: current ? Theme.fg : "transparent"
                    Text {
                        id: tabText
                        anchors.centerIn: parent
                        text: Tr.tr("tab." + modelData)
                        color: parent.current ? Theme.bg : Theme.muted
                        font.family: Theme.textFont
                        font.weight: parent.current ? Font.Medium : Font.Normal
                        font.pixelSize: Theme.fs(14.5)
                    }
                }
            }
            Glyph { button: "rb"; anchors.verticalCenter: parent.verticalCenter; color: Theme.muted }
        }
        Row {
            anchors { right: parent.right; verticalCenter: parent.verticalCenter }
            spacing: Theme.px(20)
            Item {                         // notifications: the bell counts the ones not seen yet
                anchors.verticalCenter: parent.verticalCenter
                width: Theme.fs(20); height: width
                Icon {
                    anchors.fill: parent
                    name: "bell"
                    color: root.unseen > 0 ? Theme.fg : Theme.muted
                }
                Rectangle {
                    visible: root.unseen > 0
                    x: parent.width - width / 2; y: -height / 2 + Theme.px(2)
                    width: Math.max(height, countText.implicitWidth + Theme.px(10)); height: Theme.fs(18)
                    radius: height / 2
                    color: Theme.accent
                    Text {
                        id: countText
                        anchors.centerIn: parent
                        text: root.unseen > 9 ? "9+" : root.unseen
                        color: Theme.onAccent
                        font.family: Theme.textFont
                        font.weight: Font.DemiBold
                        font.pixelSize: Theme.fs(11)
                    }
                }
            }
            Row {                          // controller and its battery, when known
                visible: root.padName !== ""
                anchors.verticalCenter: parent.verticalCenter
                spacing: Theme.px(6)
                // A percentage when the driver gives one, otherwise a level (e.g. Nintendo pads)
                readonly property var battery: {
                    const levels = { full: 1, high: 0.75, normal: 0.5, low: 0.2, critical: 0.08 };
                    for (let i = 0; i < root.controllers.length; i++) {
                        const c = root.controllers[i];
                        if (c.battery !== undefined)
                            return { fill: c.battery / 100, percent: c.battery, low: c.battery <= 15 };
                        if (c.battery_level && levels[c.battery_level] !== undefined)
                            return { fill: levels[c.battery_level], percent: -1, low: c.battery_level === "low" || c.battery_level === "critical" };
                    }
                    return null;
                }
                Icon {
                    name: "controller"
                    color: Theme.muted
                    width: Theme.fs(20); height: width
                    anchors.verticalCenter: parent.verticalCenter
                }
                Icon {
                    visible: parent.battery !== null
                    name: "battery"
                    fill: parent.battery ? parent.battery.fill : 1
                    color: parent.battery && parent.battery.low ? Theme.accent : Theme.muted
                    width: Theme.fs(20); height: width
                    anchors.verticalCenter: parent.verticalCenter
                }
                Text {
                    visible: parent.battery !== null && parent.battery.percent >= 0
                    anchors.verticalCenter: parent.verticalCenter
                    text: parent.battery ? parent.battery.percent + "%" : ""
                    color: parent.battery && parent.battery.low ? Theme.accent : Theme.muted
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                }
            }
            Icon {                         // network: Wi-Fi with its signal, cable, or crossed when offline
                readonly property var net: root.network
                readonly property bool wired: net !== null && net.ethernet && net.ethernet.connected
                readonly property bool wifi: net !== null && net.wifi && net.wifi.state === "connected"
                readonly property bool offline: net !== null && net.available !== false && !wired && !wifi
                visible: net !== null
                name: wired || (offline && !(net.wifi && net.wifi.present)) ? "ethernet" : "wifi"
                crossed: offline
                opacity: offline ? 0.7 : 1
                level: wifi && net.wifi.strength !== undefined ? (net.wifi.strength >= 67 ? 3 : net.wifi.strength >= 34 ? 2 : 1) : 3
                color: Theme.muted
                width: Theme.fs(20); height: width
                anchors.verticalCenter: parent.verticalCenter
            }
            Text {
                id: clock
                anchors.verticalCenter: parent.verticalCenter
                color: Theme.fg
                font.family: Theme.textFont
                font.weight: Font.Medium
                font.pixelSize: Theme.fs(14.5)
                function tick() { text = Qt.formatTime(new Date(), "hh:mm"); }
                Component.onCompleted: tick()
                Timer { interval: 10000; running: true; repeat: true; onTriggered: clock.tick() }
            }
        }
    }

    // ---- Sections
    FocusScope {
        id: pages
        focus: true
        anchors {
            left: parent.left; right: parent.right; top: top.bottom; bottom: prompts.top
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; topMargin: Theme.px(22); bottomMargin: Theme.px(20)
        }
        HomePage {
            id: home
            anchors.fill: parent
            visible: root.section === "home"
            focus: visible
            games: root.homeGames
            notices: root.steamNotices.concat(root.installNotices).concat(root.windowsNotices)
            onOpenNotice: if (notice.game) root.showDetails(notice.game)
            steamRunning: root.apps.some(a => a.id === "steam")
            onSteamOptions: root.confirmCloseSteam()
            onLaunch: root.launch(game)
            onDetails: root.showDetails(game)
            onOptions: root.showOptions(game)
            onOpenLibrary: root.goSection("library")
            onHowToAdd: root.showHowToAdd()
            onOpenStore: root.openSteam("store")
            running: root.runningGame
            canResume: root.sessionMode === "gamescope"
            onResume: root.resume(app)
            onCloseApp: root.confirmClose(app)
            onNotYet: root.notYet()
            onQuickMenu: quick.openMenu()
        }
        LibraryPage {
            id: library
            anchors.fill: parent
            visible: root.section === "library"
            focus: visible
            games: root.games
            sort: root.librarySort
            onLaunch: root.launch(game)
            onDetails: root.showDetails(game)
            onOptions: root.showOptions(game)
            onChooseSort: root.showSort()
            onOpenDisks: root.showDisks(volumes)
            onAddGames: root.showAddGames(false)
            onQuickMenu: quick.openMenu()
        }
        SettingsPage {
            id: settings
            anchors.fill: parent
            visible: root.section === "settings"
            focus: visible
            onOpenPalettes: palettes.openPicker()
            onOpenLanguages: languages.openPicker()
            onChooseOutput: root.chooseOutput(outputs, current)
            onChooseKeyboard: root.chooseKeyboard(layouts, current)
            onConfirmPower: root.confirmPower(action)
            onOpenCredits: credits.openCredits()
            onNotYet: root.notYet()
            network: root.network
            windowsText: root.windowsStatus && root.windowsStatus.needed ? root.windowsText(root.windowsStatus) : ""
            controllers: root.controllers
            onConnectNetwork: root.connectNetwork(net)
            onNetworkOptions: root.networkOptions(net)
            onControllerOptions: root.controllerOptions(pad)
            onDeviceOptions: root.deviceOptions(device)
            onPairController: root.pairController()
            onFailed: toast.show(Tr.tr(message))
            onChooseDisplay: root.chooseDisplay(what, state)
            updatesFromStick: root.updateStatus !== null && root.updateStatus.tester === true
            onCheckUpdates: root.checkUpdates()
            onQuickMenu: quick.openMenu()
        }
    }
    // after the first layout; a window already open (e.g. "Keep this screen?") keeps the focus
    Timer { interval: 0; running: true; onTriggered: if (!root.modalOpen) root.page.focusStart() }

    PromptBar {
        id: prompts
        anchors {
            left: parent.left; right: parent.right; bottom: parent.bottom
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; bottomMargin: Theme.safeBottom
        }
        visible: !root.modalOpen
        prompts: root.page.prompts
        globalPrompts: [{ buttons: ["lb", "rb"], label: Tr.tr("prompt.sections") }, { buttons: ["guide"], label: Tr.tr("prompt.home") }]
    }

    // ---- TV safe area outline (Appearance → Show TV safe area)
    Rectangle {
        visible: Theme.showSafeArea
        anchors {
            fill: parent
            leftMargin: Theme.safeX; rightMargin: Theme.safeX
            topMargin: Theme.safeTop; bottomMargin: Theme.safeBottom
        }
        color: "transparent"
        border.color: Theme.muted
        border.width: Math.max(1, Theme.px(1))
        radius: Theme.px(8)
        opacity: 0.7
        z: 50
    }

    // ---- Windows
    Dialog {
        id: dialog
        property var game: null
        property string mode: ""
        property var volumes: []
        property var net: null              // connect: the network
        property bool needsPassword: false
        property bool connecting: false
        property string problem: ""         // connect: why the last try failed
        property string pairState: ""       // pair: searching, pairing, failed, timeout
        property string pairName: ""
        property bool answered: false       // displayConfirm: Keep or Go back was chosen
        property bool updateBusy: false     // update: checking or installing
        property var updateLines: []        // update: "name  from  →  to"
        property string updateNote: ""
        property string agreementUrl: ""    // agreement: the licence agreement's address (4.2c)
        property string licence: ""         // licence: an installer's licence text (4.6)
        scrollTarget: mode === "licence" && licence !== "" ? licenceFlick : null
        property string appId: ""           // close: the app it asks about
        property var files: []              // chooseFile: what the game on a disk can start (4.3)
        property bool thenStart: false      // chooseFile: the game starts once chosen
        property int seconds: 0             // displayConfirm: until it goes back
        property real deadline: 0
        onSecondsChanged: if (mode === "displayConfirm") text = Tr.tr("display.keep.text", { n: seconds })
        keyboardOpen: osk.visible
        onVisibleChanged: {
            if (visible) return;
            osk.close(true);
            if (mode === "displayConfirm" && !answered) {
                answered = true;
                owneetd.post("/v1/display/revert", {});
            }
            // paired, or given up: no more pairing (owneetd keeps it on while no controller is connected)
            if (mode === "pair" && pairState !== "timeout")
                owneetd.post("/v1/bluetooth/auto-pair", { enabled: false });
            pairState = "";
            if (quick.visible) Qt.callLater(quick.refocus);    // back to the quick menu
            else root.backToPage();
        }
        Column {                          // update: what the stick has, and what is happening (4.2b)
            visible: dialog.mode === "update"
            width: Theme.px(520)
            anchors.horizontalCenter: parent.horizontalCenter
            spacing: Theme.px(12)
            Row {
                anchors.horizontalCenter: parent.horizontalCenter
                spacing: Theme.px(10)
                Spinner { visible: dialog.updateBusy; anchors.verticalCenter: parent.verticalCenter }
                Text {
                    width: Math.min(implicitWidth, Theme.px(480))
                    text: dialog.updateNote
                    color: Theme.muted
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(17)
                    wrapMode: Text.Wrap
                }
            }
            Repeater {
                model: dialog.updateLines
                Text {
                    width: parent.width
                    horizontalAlignment: Text.AlignHCenter
                    text: modelData
                    color: Theme.fg
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                    elide: Text.ElideRight
                }
            }
        }
        Row {                             // addGames: Steam, or games on a disk (demo 4.2)
            id: addChoices
            visible: dialog.mode === "addGames"
            anchors.horizontalCenter: parent.horizontalCenter
            spacing: Theme.px(16)
            Repeater {
                id: addRepeater
                model: [["steam", "store", "add.steam"], ["disk", "usb", "add.disk"]]
                Rectangle {
                    readonly property bool navigable: true
                    signal activated()
                    onActivated: root.addFrom(modelData[0])
                    width: Theme.px(250)
                    height: Math.max(addColumn.height + Theme.px(36), Theme.px(150))
                    radius: Theme.radiusL
                    color: Theme.surface
                    border.color: Theme.line
                    border.width: Math.max(1, Theme.px(1))
                    Column {
                        id: addColumn
                        anchors { top: parent.top; topMargin: Theme.px(20); horizontalCenter: parent.horizontalCenter }
                        width: parent.width - Theme.px(32)
                        spacing: Theme.px(8)
                        Icon {
                            anchors.horizontalCenter: parent.horizontalCenter
                            width: Theme.fs(32); height: width
                            name: modelData[1]
                            color: Theme.fg
                        }
                        Text {
                            width: parent.width
                            horizontalAlignment: Text.AlignHCenter
                            text: Tr.tr(modelData[2])
                            color: Theme.fg
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(17)
                            wrapMode: Text.Wrap
                        }
                        Text {
                            width: parent.width
                            horizontalAlignment: Text.AlignHCenter
                            text: Tr.tr(modelData[2] + ".detail")
                            color: Theme.muted
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(12.5)
                            wrapMode: Text.Wrap
                        }
                    }
                    FocusFrame {}
                }
            }
        }
        Column {                          // diskGuide: the Games folder (demo 4.2)
            visible: dialog.mode === "diskGuide"
            width: Theme.px(720)
            anchors.horizontalCenter: parent.horizontalCenter
            spacing: Theme.px(14)
            Rectangle {
                width: parent.width
                height: tree.height + Theme.px(28)
                radius: Theme.radiusM
                color: Theme.surface
                border.color: Theme.line
                border.width: Math.max(1, Theme.px(1))
                Column {
                    id: tree
                    x: Theme.px(20); y: Theme.px(14)
                    width: parent.width - Theme.px(40)
                    spacing: Theme.px(3)
                    Repeater {
                        // [depth, name (a label key when it starts with "@"), folder?, comment key]
                        model: [[0, "@disk.tree.root", true, ""], [1, "@disk.tree.games", true, "disk.tree.c.games"],
                                [2, "Celeste/", true, "disk.tree.c.game"], [3, "bin/ …  Celeste", false, "disk.tree.c.any"],
                                [3, "cover.jpg", false, "disk.tree.c.cover"], [2, "Stardew Valley/", true, ""],
                                [3, "setup.exe", false, "disk.tree.c.setup"]]
                        Item {
                            width: tree.width
                            height: Math.max(treeName.implicitHeight, treeNote.implicitHeight)
                            Text {
                                id: treeName
                                x: modelData[0] * Theme.px(18)
                                text: modelData[1].startsWith("@") ? Tr.tr(modelData[1].slice(1)) : modelData[1]
                                color: modelData[2] ? Theme.fg : Theme.muted
                                font.family: Theme.textFont
                                font.weight: modelData[2] ? Font.Medium : Font.Normal
                                font.pixelSize: Theme.fs(14.5)
                            }
                            Text {
                                id: treeNote
                                x: Theme.px(300)
                                width: parent.width - x
                                visible: modelData[3] !== ""
                                text: "← " + Tr.tr(modelData[3])
                                color: Theme.accent
                                font.family: Theme.textFont
                                font.pixelSize: Theme.fs(12.5)
                                wrapMode: Text.Wrap
                            }
                        }
                    }
                }
            }
            Repeater {
                model: ["disk.step.plug", "disk.step.find", "disk.step.install", "disk.step.win"]
                Text {
                    width: parent.width
                    text: "•  " + Tr.tr(modelData)
                    color: Theme.fg
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                    wrapMode: Text.Wrap
                }
            }
        }
        Row {                             // powerOptions: four large choices (demo 3.13)
            id: powerChoices
            visible: dialog.mode === "powerOptions"
            anchors.horizontalCenter: parent.horizontalCenter
            spacing: Theme.px(14)
            Repeater {
                id: choiceRepeater
                model: ["suspend", "restart", "shutdown", "cancel"]
                Rectangle {
                    readonly property bool navigable: true
                    signal activated()
                    onActivated: modelData === "cancel" ? dialog.close(true) : root.power(modelData)
                    readonly property string feedbackKind: modelData === "cancel" ? "sheet-close" : "confirm"
                    width: Theme.px(128)
                    height: choiceColumn.height + Theme.px(30)
                    radius: Theme.radiusL
                    color: modelData === "cancel" ? Theme.raised : Theme.surface
                    border.color: Theme.line
                    border.width: Math.max(1, Theme.px(1))
                    Column {
                        id: choiceColumn
                        anchors.centerIn: parent
                        spacing: Theme.px(10)
                        Icon {
                            anchors.horizontalCenter: parent.horizontalCenter
                            width: Theme.fs(30); height: width
                            name: modelData === "shutdown" ? "power" : modelData
                            color: Theme.fg
                        }
                        Text {
                            anchors.horizontalCenter: parent.horizontalCenter
                            text: Tr.tr(modelData === "cancel" ? "prompt.cancel" : "system.power." + modelData)
                            color: Theme.fg
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(17)
                        }
                    }
                    FocusFrame {}
                }
            }
        }
        Column {                          // details: facts about the game (Steam: its download, 4.2c)
            visible: dialog.mode === "details" && dialog.game !== null
            anchors.horizontalCenter: parent.horizontalCenter
            spacing: Theme.px(6)
            Grid {
                anchors.horizontalCenter: parent.horizontalCenter
                columns: 2
                columnSpacing: Theme.px(18)
                rowSpacing: Theme.px(6)
                Repeater {
                    model: dialog.game && dialog.mode === "details" ? root.detailsFacts(dialog.game).reduce((a, f) => a.concat(f), []) : []
                    Text {
                        text: modelData
                        color: index % 2 === 0 ? Theme.muted : Theme.fg
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                    }
                }
            }
            Rectangle {                   // the download's progress
                readonly property string s: dialog.game ? GameInfo.steamState(dialog.game) : ""
                visible: s === "downloading" || s === "updating" || s === "queued" || s === "paused"
                anchors.horizontalCenter: parent.horizontalCenter
                width: Theme.px(420); height: Theme.px(8); radius: height / 2
                color: Theme.raised
                Rectangle {
                    width: parent.width * (dialog.game ? GameInfo.progress(dialog.game) : 0) / 100
                    height: parent.height; radius: height / 2
                    color: Theme.accent
                    Behavior on width { NumberAnimation { duration: Theme.motionMs } }
                }
            }
            Text {                        // on a disk that is not connected
                visible: dialog.game !== null && GameInfo.steamState(dialog.game) === "away"
                anchors.horizontalCenter: parent.horizontalCenter
                readonly property string disk: dialog.game ? GameInfo.awayDisk(dialog.game) : ""
                text: disk !== "" ? Tr.tr("details.away", { disk: disk }) : Tr.tr("details.away.unnamed")
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(14.5)
            }
        }
        Column {                          // chooseFile: the files a game on a disk can start (demo 4.3)
            visible: dialog.mode === "chooseFile"
            anchors.horizontalCenter: parent.horizontalCenter
            width: Theme.px(560)
            spacing: Theme.px(10)
            Repeater {
                id: fileRepeater
                model: dialog.mode === "chooseFile" ? dialog.files : []
                Rectangle {
                    readonly property bool navigable: true
                    readonly property bool current: dialog.game !== null && diskGames.chosen(dialog.game) === modelData
                    signal activated()
                    onActivated: root.fileChosen(modelData)
                    width: parent.width
                    height: fileColumn.height + Theme.px(24)
                    radius: Theme.radiusM
                    color: Theme.surface
                    border.color: current ? Theme.accent : Theme.line
                    border.width: Math.max(1, Theme.px(1))
                    Column {
                        id: fileColumn
                        anchors { left: parent.left; leftMargin: Theme.px(18); right: parent.right; rightMargin: Theme.px(18)
                                  verticalCenter: parent.verticalCenter }
                        spacing: Theme.px(2)
                        Text {
                            width: parent.width
                            text: modelData.split("/").pop()
                            color: Theme.fg
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(17)
                            elide: Text.ElideMiddle
                        }
                        Text {
                            readonly property string folder: modelData.indexOf("/") < 0 ? "" : modelData.slice(0, modelData.lastIndexOf("/"))
                            width: parent.width
                            text: folder === "" ? Tr.tr("choose.file.top") : Tr.tr("choose.file.in", { folder: folder })
                            color: Theme.muted
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(12.5)
                            elide: Text.ElideMiddle
                        }
                    }
                    FocusFrame {}
                }
            }
        }
        Rectangle {                       // licence: an installer's licence, scrolled (4.6)
            visible: dialog.mode === "licence" && dialog.licence !== ""
            anchors.horizontalCenter: parent.horizontalCenter
            width: Theme.px(680); height: Theme.px(300)
            radius: Theme.radiusM
            color: Theme.surface
            border.color: Theme.line
            Flickable {
                id: licenceFlick
                anchors { fill: parent; margins: Theme.px(16) }
                clip: true
                contentHeight: licenceText.implicitHeight
                boundsBehavior: Flickable.StopAtBounds
                Behavior on contentY { enabled: !Theme.reduceMotion; NumberAnimation { duration: Theme.motionMs; easing.type: Easing.OutCubic } }
                Text {
                    id: licenceText
                    width: licenceFlick.width
                    text: dialog.mode === "licence" ? dialog.licence : ""
                    textFormat: Text.PlainText
                    color: Theme.fg
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                    wrapMode: Text.Wrap
                }
            }
        }
        Column {                          // agreement: the licence agreement's address, as a code (4.2c)
            visible: dialog.mode === "agreement"
            anchors.horizontalCenter: parent.horizontalCenter
            spacing: Theme.px(8)
            QrCode {
                anchors.horizontalCenter: parent.horizontalCenter
                width: Theme.px(200); height: width
                text: dialog.mode === "agreement" ? dialog.agreementUrl : ""
            }
            Text {
                anchors.horizontalCenter: parent.horizontalCenter
                text: dialog.agreementUrl.replace(/^https?:\/\//, "")
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(12.5)
            }
        }
        SettingRow {                      // steamNotice: "Don't show again" (4.2c)
            id: noticeNever
            visible: dialog.mode === "steamNotice" || dialog.mode === "steamReturn"
            anchors.horizontalCenter: parent.horizontalCenter
            width: Theme.px(360)
            text: Tr.tr("steam.notice.never")
            onActivated: checked = !checked
        }
        Column {                          // connect: password, what went wrong, "Connecting…"
            visible: dialog.mode === "connect"
            anchors.horizontalCenter: parent.horizontalCenter
            width: Theme.px(460)
            spacing: Theme.px(14)
            FocusScope {
                id: passwordField
                // A opens the on-screen keyboard; a keyboard plugged in types here, Enter connects
                readonly property bool navigable: true
                signal activated()
                onActivated: osk.open(passwordInput, true)
                visible: dialog.needsPassword && !dialog.connecting
                width: parent.width
                height: fieldColumn.height + Theme.px(24)
                readonly property real radius: Theme.radiusS + Theme.px(2)
                Rectangle {
                    anchors.fill: parent
                    radius: parent.radius
                    color: Theme.surface
                    border.color: osk.visible && osk.target === passwordInput ? Theme.accent : Theme.line
                    border.width: Math.max(1, Theme.px(1))
                }
                Column {
                    id: fieldColumn
                    x: Theme.px(16); y: Theme.px(12)
                    width: parent.width - Theme.px(32)
                    spacing: Theme.px(4)
                    Text {
                        text: Tr.tr("network.password")
                        color: Theme.muted
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                    }
                    TextInput {
                        id: passwordInput
                        focus: true
                        width: parent.width
                        echoMode: showPassword.checked ? TextInput.Normal : TextInput.Password
                        passwordCharacter: "\u2022"
                        color: Theme.fg
                        selectionColor: Theme.accent
                        selectedTextColor: Theme.onAccent
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(17)
                        font.letterSpacing: showPassword.checked ? 0 : Theme.fs(17) * 0.1
                        maximumLength: 63
                        cursorVisible: passwordField.activeFocus || (osk.visible && osk.target === passwordInput)
                        Keys.onReturnPressed: if (dialog.buttons.length > 0) dialog.buttons[0].action()
                        Keys.onEnterPressed: if (dialog.buttons.length > 0) dialog.buttons[0].action()
                    }
                }
                FocusFrame { shown: passwordField.activeFocus }
            }
            SettingRow {                  // so that what was typed can be checked
                id: showPassword
                visible: passwordField.visible
                width: parent.width
                text: Tr.tr("network.password.show")
                onActivated: checked = !checked
            }
            Text {
                visible: dialog.problem !== "" && !dialog.connecting
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                text: dialog.problem
                color: Theme.accent
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(17)
                wrapMode: Text.Wrap
            }
            Row {
                visible: dialog.connecting
                anchors.horizontalCenter: parent.horizontalCenter
                spacing: Theme.px(10)
                Spinner { anchors.verticalCenter: parent.verticalCenter }
                Text {
                    text: Tr.tr("network.connecting")
                    color: Theme.muted
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(17)
                }
            }
        }
        Column {                          // pair: what is happening, how to pair each kind of controller
            visible: dialog.mode === "pair"
            anchors.horizontalCenter: parent.horizontalCenter
            width: Theme.px(922)
            spacing: Theme.px(16)
            Row {
                anchors.horizontalCenter: parent.horizontalCenter
                spacing: Theme.px(10)
                Spinner {
                    visible: dialog.pairState === "searching" || dialog.pairState === "pairing"
                    anchors.verticalCenter: parent.verticalCenter
                }
                Text {
                    text: dialog.pairState === "pairing" ? Tr.tr("pair.pairing", { name: dialog.pairName })
                        : dialog.pairState === "failed" ? Tr.tr("pair.failed", { name: dialog.pairName })
                        : dialog.pairState === "timeout" ? Tr.tr("pair.timeout")
                        : Tr.tr("pair.searching")
                    color: dialog.pairState === "failed" || dialog.pairState === "timeout" ? Theme.accent : Theme.muted
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(17)
                }
            }
            Row {
                anchors.horizontalCenter: parent.horizontalCenter
                spacing: Theme.px(14)
                Repeater {
                    model: ["xbox", "ps", "nintendo", "other"]
                    Rectangle {
                        width: Theme.px(220)
                        height: brandColumn.height + Theme.px(28)
                        radius: Theme.radiusM
                        color: Theme.surface
                        border.color: Theme.line
                        border.width: Math.max(1, Theme.px(1))
                        Column {
                            id: brandColumn
                            x: Theme.px(14); y: Theme.px(14)
                            width: parent.width - Theme.px(28)
                            spacing: Theme.px(10)
                            Text {
                                text: Tr.tr("pair." + modelData)
                                color: Theme.fg
                                font.family: Theme.textFont
                                font.weight: Font.Medium
                                font.pixelSize: Theme.fs(17)
                            }
                            Rectangle {           // no drawing for "other controllers"
                                visible: modelData !== "other"
                                width: parent.width
                                height: Theme.px(104)
                                radius: Theme.radiusS + Theme.px(2)
                                color: Theme.raised
                                PadDrawing {
                                    anchors.centerIn: parent
                                    width: Theme.px(170); height: Theme.px(100)
                                    kind: modelData
                                }
                            }
                            Text {
                                width: parent.width
                                text: Tr.tr("pair." + modelData + ".how")
                                color: Theme.muted
                                font.family: Theme.textFont
                                font.pixelSize: Theme.fs(14.5)
                                wrapMode: Text.Wrap
                            }
                        }
                    }
                }
            }
            Text {
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                text: Tr.tr("pair.wired")
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(14.5)
                wrapMode: Text.Wrap
            }
        }
        Column {                          // disks: free space of every disk
            visible: dialog.mode === "disks"
            anchors.horizontalCenter: parent.horizontalCenter
            width: Theme.px(560)
            spacing: Theme.px(12)
            Repeater {
                model: dialog.mode === "disks" ? dialog.volumes : []
                Rectangle {
                    width: parent.width
                    height: diskColumn.height + Theme.px(24)
                    radius: Theme.radiusM
                    color: Theme.surface
                    border.color: Theme.line
                    border.width: Math.max(1, Theme.px(1))
                    Column {
                        id: diskColumn
                        x: Theme.px(16); y: Theme.px(12)
                        width: parent.width - Theme.px(32)
                        spacing: Theme.px(6)
                        Item {
                            width: parent.width
                            height: Math.max(diskName.height, diskFree.height)
                            Text {
                                id: diskName
                                text: modelData.name || Tr.tr("disk.internal")
                                color: Theme.fg
                                font.family: Theme.textFont
                                font.pixelSize: Theme.fs(17)
                            }
                            Text {
                                id: diskFree
                                anchors { right: parent.right; baseline: diskName.baseline }
                                text: Tr.tr("disk.of", { free: GameInfo.size(modelData.free), total: GameInfo.size(modelData.total) })
                                color: Theme.muted
                                font.family: Theme.textFont
                                font.pixelSize: Theme.fs(14.5)
                            }
                        }
                        Rectangle {
                            width: parent.width; height: Theme.px(6); radius: height / 2
                            color: Theme.raised
                            Rectangle {
                                width: parent.width * Math.min(1, 1 - modelData.free / modelData.total)
                                height: parent.height; radius: height / 2
                                color: Theme.accent
                            }
                        }
                        Text {
                            visible: modelData.readOnly
                            width: parent.width
                            text: Tr.tr("disk.note.readonly")
                            color: Theme.muted
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(12.5)
                            wrapMode: Text.Wrap
                        }
                    }
                }
            }
        }
    }
    Toast { id: toast }

    Banners {
        id: banners
        anchors { right: parent.right; rightMargin: Theme.safeX; top: parent.top; topMargin: Theme.px(88) }
        visible: !notices.visible
    }
    NotificationsSheet {
        id: notices
        z: 101
        notices: root.noticeList
        onVisibleChanged: {
            if (visible) {
                root.unseen = 0;               // everything in the list has been seen
                banners.clear();
            } else {
                root.backToPage();
            }
        }
        onOpenAction: root.openNotice(notice)
        onRemoveAt: {
            root.noticeList = root.noticeList.filter((n, i) => i !== index);
            Qt.callLater(() => notices.focusIndex(index));
        }
        onClearAll: {
            root.noticeList = [];
            toast.show(Tr.tr("notices.cleared"));
            Qt.callLater(() => notices.focusIndex(0));
        }
    }

    // ---- Interface sounds (3.12): what the navigation reports, silent while a game is on screen
    UiSounds {
        id: sounds
        dir: systemInfo.soundsDir
        muted: root.gameOnScreen
    }
    Connections {
        target: Nav
        function onPlayed(kind) { sounds.play(kind); }
    }

    // ---- On-screen keyboard (3.10): its layout is Settings → Language → Keyboard layout
    Keyboard {
        id: osk
        layout: {
            const l = Theme.keyboardSetting === "same" ? root.keyboardFor(Tr.language) : Theme.keyboardSetting;
            return ["us", "it"].indexOf(l) >= 0 ? l : "us";
        }
        showPassword: showPassword.checked
        onTogglePassword: showPassword.checked = !showPassword.checked
        onDone: {
            if (dialog.visible && dialog.mode === "connect" && dialog.buttons.length > 0)
                dialog.buttons[0].action();      // Done = Connect
            else
                close(true);
        }
        onClosed: {
            close();
            if (dialog.visible)
                Qt.callLater(() => dialog.focusButton(0));
            else
                root.backToPage();
        }
    }

    PalettePicker {
        id: palettes
        z: 101
        onVisibleChanged: if (!visible) root.backToPage()
    }
    LanguagePicker {
        id: languages
        z: 101
        onVisibleChanged: if (!visible) root.backToPage()
    }
    CreditsSheet {
        id: credits
        z: 101
        onVisibleChanged: if (!visible) root.backToPage()
    }

    // ---- Steam starting (4.2): until Steam's or the game's window appears, or Steam stops
    SteamPreparing {
        id: steamPrep
        onVisibleChanged: if (!visible) root.backToPage()
    }
    Connections {
        target: owneetd
        function onEvent(type, data) {
            if (!steamPrep.visible || !data) return;
            const id = data.id || "";
            // Steam closed to set a game's version for Windows comes back for it (4.5)
            if (type === "app.started" && id === "steam") steamPrep.steamRestart = false;
            // started for a game, Steam stays hidden: wait for the game's window (owneetd keeps
            // Steam behind until then), or Steam, if it needs the user (focus.changed)
            const steamWindow = id === "steam" && steamPrep.gameTitle === "";
            if ((type === "app.window" && (steamWindow || id.startsWith("steam:")))
                    || (type === "app.exited" && ((id === "steam" && !steamPrep.steamRestart) || id.startsWith("steam:") || steamPrep.windows))
                    || (type === "focus.changed" && data.focus !== "home"))
                steamPrep.close(true);
        }
    }

    // ---- Quick menu (3.13): B where there is nothing to go back to
    QuickMenu {
        id: quick
        covered: dialog.visible || controls.visible
        onOpenPower: root.showPowerOptions()
        onOpenControls: controls.openControls()
        onVisibleChanged: if (!visible) root.backToPage()
    }
    ControlsSheet {
        id: controls
        z: 103
        onVisibleChanged: if (!visible && quick.visible) quick.refocus()
    }

    // ---- Start-up animation (3.13): with the start-up sound; without it (sounds off, or the
    // output not ready), on its own after a while
    Intro {
        id: intro
        target: mark
        pages: pages
    }
    Connections {
        target: sounds
        function onStartupStarted() { intro.go(); }
    }
    Timer {
        id: introFallback
        interval: Theme.uiSounds && systemInfo.soundsDir !== "" ? 4000 : 1200
        onTriggered: intro.go()
    }
}
