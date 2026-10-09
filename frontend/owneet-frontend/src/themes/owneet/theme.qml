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
        ["reduceMotion", "reduceMotion"], ["showSafeArea", "showSafeArea"]
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
    readonly property bool modalOpen: appearance.visible || palettes.visible || languages.visible || dialog.visible
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
            Icon {                         // network: Wi-Fi with its signal, or cable
                readonly property var net: root.network
                readonly property bool wired: net !== null && net.ethernet && net.ethernet.connected
                readonly property bool wifi: net !== null && net.wifi && net.wifi.state === "connected"
                visible: wired || wifi
                name: wired ? "ethernet" : "wifi"
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
            onOpenAppearance: appearance.openSheet(true)
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
        onVisibleChanged: if (!visible) root.backToPage()
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

    AppearanceSheet {
        id: appearance
        z: 100
        onVisibleChanged: if (!visible && !palettes.visible && !languages.visible) root.backToPage()
        onOpenPalettes: { close(true); palettes.openPicker(); }
        onOpenLanguages: { close(true); languages.openPicker(); }
    }
    PalettePicker {
        id: palettes
        z: 101
        onVisibleChanged: if (!visible) appearance.openSheet(false, true)
    }
    LanguagePicker {
        id: languages
        z: 101
        onVisibleChanged: if (!visible) appearance.openSheet(false, true)
    }
}
