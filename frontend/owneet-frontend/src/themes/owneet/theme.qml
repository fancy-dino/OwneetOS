// SPDX-License-Identifier: GPL-3.0-or-later
// The OwneetOS theme: the shell around the sections (top bar, sections on LB / RB, prompt bar,
// windows). Foundations: step 3.3 (design/demos/3.3-theme-foundations.html); home: step 3.6
// (design/demos/3.6-home.html). Library (3.7) and Settings (3.9) are temporary pages for now.
import QtQuick 2.15
import "foundation"

FocusScope {
    id: root
    focus: true

    // ---- Preferences, saved in the theme's memory (moved to Settings in step 3.9)
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
        updatePad();
        refreshGames();
        refreshDaemon();          // owneetd may have connected before the theme was loaded
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

    // ---- Games, most recently played first (shared by the sections)
    property var games: []
    function refreshGames() { games = GameInfo.recent(api.allGames); }
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
    readonly property bool modalOpen: palettes.visible || languages.visible || credits.visible || dialog.visible
    function backToPage() { Qt.callLater(() => { if (!root.modalOpen) root.page.forceActiveFocus(); }); }

    // ---- Buttons the sections leave to the whole interface (section 9.1)
    Keys.onPressed: {
        const a = Nav.action(event);
        if ((a === "section-prev" || a === "section-next") && !modalOpen) {
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
        owneetd.get("/v1/network", (status, data) => { if (status === 200) network = data; });
        refreshApps();
        refreshControllers();
        applyKeyboard();          // the on-screen keyboard and physical keyboards follow the chosen layout
        checkDisplayPending();
    }
    Connections {
        target: owneetd
        function onConnectedChanged() { if (owneetd.connected) root.refreshDaemon(); }
        function onEvent(type, data) {
            if (type === "app.started" || type === "app.exited") {
                root.refreshApps();
                if (type === "app.exited")
                    refreshTimer.restart();       // play time and "last played" have changed
            } else if (type === "focus.changed") {
                root.focusedApp = data.focus;
            } else if (type.startsWith("controller.")) {
                root.refreshControllers();
            } else if (type === "network.changed") {
                root.network = data;
            } else if (type.startsWith("bluetooth.")) {
                root.pairingEvent(type, data);
            } else if (type.startsWith("display.")) {
                root.displayEvent(type, data);
            }
        }
    }
    // Errors from owneetd come as "owneetd:CODE" and are translated (error.CODE)
    Connections {
        target: api
        function onEventLaunchError(msg) {
            const code = msg.startsWith("owneetd:") ? msg.slice(8) : "";
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
        dialog.contentWidth = 0;
        dialog.title = Tr.tr("close.title", { title: app.name });
        dialog.text = Tr.tr("close.text");
        dialog.buttons = [
            { text: Tr.tr("home.close"), action: () => {
                dialog.close();
                owneetd.post("/v1/apps/" + encodeURIComponent(app.id) + "/close", {}, (status) => {
                    if (status >= 300 || status === 0) toast.show(Tr.tr("error.close"));
                });
            } },
            { text: Tr.tr("prompt.cancel"), style: "primary", action: () => dialog.close() }
        ];
        dialog.defaultIndex = 1;             // Cancel first: no game closed by accident
        dialog.open();
    }

    // ---- Games: launch, details, options
    function launch(game) {
        if (runningGame && owneetd.connected) {     // one game at a time
            toast.show(Tr.tr("error.apps.game_running"));
            Nav.feedback("edge");
            return;
        }
        Nav.feedback("launch");
        toast.show(Tr.tr("toast.launch", { title: game.title }));
        game.launch();                    // through owneetd from step 3.8
        refreshTimer.restart();           // "last played" changes once the game has started
    }
    Timer { id: refreshTimer; interval: 3000; onTriggered: root.refreshGames() }
    function showDetails(game) {
        dialog.game = game;
        dialog.mode = "details";
        dialog.contentWidth = 0;
        dialog.title = game.title;
        dialog.text = "";
        dialog.buttons = [{ text: Tr.tr("home.play"), style: "primary", action: () => { dialog.close(true); root.launch(game); } }];
        dialog.defaultIndex = 0;
        dialog.open();
    }
    function showOptions(game) {
        dialog.game = game;
        dialog.mode = "options";
        dialog.contentWidth = 0;
        dialog.title = game.title;
        dialog.text = "";
        dialog.buttons = [
            { text: Tr.tr("home.play"), style: "primary", action: () => { dialog.close(true); root.launch(game); } },
            { text: Tr.tr("home.details"), action: () => { dialog.close(true); root.showDetails(game); } },
            { text: Tr.tr(game.favorite ? "options.unfavorite" : "options.favorite"), action: () => {
                game.favorite = !game.favorite;
                library.favoritesRevision++;
                dialog.close();
                toast.show(Tr.tr(game.favorite ? "toast.favorite" : "toast.unfavorite"));
            } }
        ];
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
        dialog.buttons = [{ text: Tr.tr("home.empty.store"), style: "primary", action: () => { dialog.close(); root.notYet(); } }];
        dialog.defaultIndex = 0;
        dialog.open();
    }
    function notYet() { toast.show(Tr.tr("toast.later")); }

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
    // network. The password is typed with a keyboard until the on-screen keyboard (step 3.10).
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
    }
    function startConnect(net, password) {
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
            owneetd.post("/v1/network/wifi/disconnect", {}, done);
        } }] : []).concat([
            { text: Tr.tr("network.forget"), action: () => {
                dialog.close();
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
            Nav.feedback("error");
            toast.show(Tr.tr(code === "display.apps_open" ? "error.display.apps_open" : "error.display"));
        });
        if (what === "screen") {
            const current = (state.screens.find(s => s.active) || {}).id;
            choose(Tr.tr("display.screen"), state.screens.map(s => [s.id, label(s)]), current,
                   id => { if (id !== current) put("/v1/display/screen", { id: id }); });
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
        dialog.contentWidth = 0;
        dialog.title = Tr.tr("display.keep." + kind);
        dialog.buttons = [
            { text: Tr.tr("display.keep"), style: "primary", action: () => {
                dialog.answered = true;
                dialog.close();
                owneetd.post("/v1/display/confirm", {});
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
            games: root.games
            onLaunch: root.launch(game)
            onDetails: root.showDetails(game)
            onOptions: root.showOptions(game)
            onOpenLibrary: root.goSection("library")
            onHowToAdd: root.showHowToAdd()
            running: root.runningGame
            canResume: root.sessionMode === "gamescope"
            onResume: root.resume(app)
            onCloseApp: root.confirmClose(app)
            onNotYet: root.notYet()
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
            controllers: root.controllers
            onConnectNetwork: root.connectNetwork(net)
            onNetworkOptions: root.networkOptions(net)
            onControllerOptions: root.controllerOptions(pad)
            onDeviceOptions: root.deviceOptions(device)
            onPairController: root.pairController()
            onFailed: toast.show(Tr.tr(message))
            onChooseDisplay: root.chooseDisplay(what, state)
        }
    }
    Timer { interval: 0; running: true; onTriggered: home.focusStart() }   // after the first layout

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
        property int seconds: 0             // displayConfirm: until it goes back
        property real deadline: 0
        onSecondsChanged: if (mode === "displayConfirm") text = Tr.tr("display.keep.text", { n: seconds })
        onVisibleChanged: {
            if (visible) return;
            if (mode === "displayConfirm" && !answered) {
                answered = true;
                owneetd.post("/v1/display/revert", {});
            }
            // paired, or given up: no more pairing (owneetd keeps it on while no controller is connected)
            if (mode === "pair" && pairState !== "timeout")
                owneetd.post("/v1/bluetooth/auto-pair", { enabled: false });
            pairState = "";
            root.backToPage();
        }
        Column {                          // details: facts about the game
            visible: dialog.mode === "details" && dialog.game !== null
            anchors.horizontalCenter: parent.horizontalCenter
            spacing: Theme.px(6)
            Repeater {
                model: dialog.game ? [
                    [Tr.tr("details.source"), GameInfo.source(dialog.game)],
                    [Tr.tr("details.last"), GameInfo.lastPlayed(dialog.game)],
                    [Tr.tr("details.time"), GameInfo.played(dialog.game) ? GameInfo.playTime(dialog.game) : "\u2014"]
                ] : []
                Row {
                    spacing: Theme.px(18)
                    anchors.horizontalCenter: parent.horizontalCenter
                    Text { text: modelData[0]; color: Theme.muted; font.family: Theme.textFont; font.pixelSize: Theme.fs(14.5) }
                    Text { text: modelData[1]; color: Theme.fg; font.family: Theme.textFont; font.pixelSize: Theme.fs(14.5) }
                }
            }
        }
        Column {                          // connect: password, what went wrong, "Connecting…"
            visible: dialog.mode === "connect"
            anchors.horizontalCenter: parent.horizontalCenter
            width: Theme.px(460)
            spacing: Theme.px(14)
            FocusScope {
                id: passwordField
                // A keyboard types here; A (or Enter) connects
                readonly property bool navigable: true
                signal activated()
                onActivated: if (dialog.buttons.length > 0) dialog.buttons[0].action()
                visible: dialog.needsPassword && !dialog.connecting
                width: parent.width
                height: fieldColumn.height + Theme.px(24)
                readonly property real radius: Theme.radiusS + Theme.px(2)
                Rectangle {
                    anchors.fill: parent
                    radius: parent.radius
                    color: Theme.surface
                    border.color: Theme.line
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
                        echoMode: TextInput.Password
                        passwordCharacter: "\u2022"
                        color: Theme.fg
                        selectionColor: Theme.accent
                        selectedTextColor: Theme.onAccent
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(17)
                        font.letterSpacing: Theme.fs(17) * 0.1
                        maximumLength: 63
                        cursorVisible: passwordField.activeFocus
                        Text {
                            visible: parent.text === ""
                            text: Tr.tr("network.password.hint")
                            color: Theme.muted
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(12.5)
                            anchors.verticalCenter: parent.verticalCenter
                        }
                    }
                }
                FocusFrame { shown: passwordField.activeFocus }
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
}
