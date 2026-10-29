// SPDX-License-Identifier: GPL-3.0-or-later
// The home screen ([[step-3.6-home-screen]], approved demo design/demos/3.6-home.html): the last
// game played (or a welcome on a system with no games), the apps, notices, and the recently played
// games. Three navigation groups: hero, apps column, row of games.
// A game running behind the home screen (owneetd, 3.8) takes the hero: Resume (gamescope only:
// in cage the home screen is not shown over a game) and Close game.
// The web apps (Netflix, YouTube, Spotify) are optional ([[step-5.0.0-optional-web-apps]], approved
// demo design/demos/5.0.0-optional-apps.html): a missing one is dimmed with a download mark, A
// installs it, Menu opens its options (Open, Remove). An open web app takes the hero like a game
// ([[step-5.1-brave-in-the-os]], approved demo design/demos/5.1-web-app-running.html): its letter
// instead of art, Resume and Close; its tile shows a green dot.
// Still to come: real covers from Steam's local files (4.1).
import QtQuick 2.15
import "foundation"

NavArea {
    id: page
    property var games: []                     // all games, most recently played first
    readonly property var hero: games.length > 0 ? games[0] : null
    // with a game running, the hero shows it and the row starts from the most recent game
    readonly property var recent: running ? games.slice(0, 6) : games.slice(1, 7)
    property var notices: []                   // [{ text, game }]: Steam's downloads (4.2c)
    // The web apps of the catalogue (owneetd): { id, nameKey, icon, installed, busy, sizeMiB }
    property var webApps: []
    // Their ids, changed only when apps come or go: the tiles stay (and stay selected) while an
    // app is installed or removed
    property var webAppIds: []
    onWebAppsChanged: {
        const ids = webApps.map(a => a.id);
        if (ids.join() !== webAppIds.join()) webAppIds = ids;
    }
    function webApp(id) { return webApps.find(a => a.id === id) || null; }
    property var running: null                 // the running game or web app: { id, name, kind, started }
    property bool canResume: true
    // An open web app (5.1): its catalogue entry
    readonly property bool webOpen: running !== null && running.kind === "webapp"
    readonly property var runningWeb: webOpen ? (webApps.find(a => "web:" + a.id === running.id) || null) : null
    function isOpen(app) { return app !== null && running !== null && running.id === "web:" + app.id; }
    // The Pegasus game of the running one (same title), for its art and source
    readonly property var runningGame: {
        if (!running || webOpen) return null;
        for (let i = 0; i < games.length; i++)
            if (games[i].title === running.name) return games[i];
        return null;
    }
    property real now: Date.now()
    Timer { interval: 30000; running: page.running !== null; repeat: true; onTriggered: page.now = Date.now() }
    readonly property int playingMinutes: running && running.started
        ? Math.max(0, Math.floor((now - new Date(running.started).getTime()) / 60000)) : 0
    signal resume(var app)
    signal closeApp(var app)
    // The controller as a mouse (4.7, approved demo design/demos/4.7-controller-pointer.html): on
    // for the running game's window, with a "Pointer on" badge on its card
    property bool pointerOn: false
    signal resumeWithPointer(var app)
    signal pointerOff()

    signal launch(var game)
    signal details(var game)
    signal options(var game)
    signal openLibrary()
    signal howToAdd()
    signal notYet()                            // a feature of a later step
    signal quickMenu()                         // B: nothing to go back to here (3.13)
    signal openStore()                         // Steam's store (4.2)
    signal openWebApp(var app)                 // A on a web app: install it, or open it
    signal webAppOptions(var app)              // Menu on a web app
    signal openNotice(var notice)              // the downloads notice: the game's details (4.2c)
    property bool steamRunning: false           // Steam runs (hidden or not): Menu on its tile closes it
    signal steamOptions()
    onCancelled: quickMenu()
    backFeedback: ""                           // the quick menu plays its own sound

    // The prompt bar follows the selected item
    // Y opens the notifications from anywhere on the home screen (demo 3.11)
    readonly property var prompts: itemPrompts.concat([{ buttons: ["y"], label: Tr.tr("prompt.notifications") },
                                                        { buttons: ["b"], label: Tr.tr("prompt.menu") }])
    readonly property var itemPrompts: {
        const k = current ? current.kind : "";
        const P = (b, l) => ({ buttons: [b], label: Tr.tr(l) });
        if (k === "resume")
            return [P("a", "prompt.resume")];
        if (k === "close")
            return [P("a", "prompt.close")];
        if (k === "pointer" || k === "pointerOff")
            return [P("a", "prompt.select")];
        if (k === "game" || k === "play")
            return [P("a", "prompt.play"), P("x", "prompt.details"), P("menu", "prompt.options")];
        if (k === "details")
            return [P("a", "prompt.details"), P("menu", "prompt.options")];
        if (k === "app" && current.webApp)
            return current.webApp.busy ? [] : [P("a", isOpen(current.webApp) ? "prompt.resume"
                                                    : current.webApp.installed ? "prompt.open" : "prompt.install"),
                                               P("menu", "prompt.options")];
        if (k === "app" && current.steamStore && steamRunning)
            return [P("a", "prompt.open"), P("menu", "prompt.options")];
        if (k === "app" || k === "all" || k === "notice" || k === "store")
            return [P("a", "prompt.open")];
        return k ? [P("a", "prompt.select")] : [];
    }
    function gameOf(item) { return item ? (item.game || null) : null; }

    function focusStart() { focusItem(Nav.navigables(heroGroup)[0] || null); }
    // Selects the first item; a window over the page keeps the focus (e.g. "Close …?" while the
    // game ends by itself, FrankenPC 2026-10-16): the page takes it back when it gets it again
    function selectStart() {
        const item = Nav.navigables(heroGroup)[0] || null;
        if (!item || activeFocus) { focusStart(); return; }
        const group = Nav.groupOf(page, item);
        if (group) group.remembered = item;
        current = item;
    }
    // The games arrive after the page: keep a visible item selected (e.g. Play instead of the
    // welcome buttons)
    onHeroChanged: Qt.callLater(() => { if (!current || !current.visible) selectStart(); })
    onRunningChanged: Qt.callLater(() => { if (visible) selectStart(); })
    // While the boot splash is shown the page is disabled and nothing can be selected yet
    onActiveFocusChanged: {
        if (!activeFocus) return;
        if (!current || !current.visible) focusStart();
        else if (!current.activeFocus) focusItem(current);
    }

    onAccepted: {
        const k = item.kind;
        if (k === "game" || k === "play") launch(gameOf(item));
        else if (k === "details") details(gameOf(item));
        else if (k === "all") openLibrary();
        else if (k === "how") howToAdd();
        else if (k === "resume") resume(running);
        else if (k === "close") closeApp(running);
        else if (k === "pointer") resumeWithPointer(running);
        else if (k === "pointerOff") pointerOff();
        else if (k === "store" || item.steamStore) openStore();
        else if (k === "app" && item.webApp) openWebApp(item.webApp);
        else if (k === "notice" && page.notices.length > 0) openNotice(page.notices[0]);
        else notYet();                         // apps (later steps)
    }
    onOtherAction: {
        const game = gameOf(current);
        if (action === "secondary" && game && current.kind !== "details") { details(game); event.accepted = true; }
        else if (action === "options" && game) { options(game); event.accepted = true; }
        else if (action === "options" && current && current.webApp && !current.webApp.busy) {
            webAppOptions(current.webApp);
            event.accepted = true;
        }
        else if (action === "options" && current && current.steamStore && steamRunning) { steamOptions(); event.accepted = true; }
    }

    // ---- Top: hero (2/3) and side column (1/3)
    Item {
        id: top
        anchors { left: parent.left; right: parent.right; top: parent.top; bottom: rowBlock.top; bottomMargin: Theme.px(20) }

        NavGroup {
            id: heroGroup
            anchors { left: parent.left; top: parent.top; bottom: parent.bottom }
            width: (parent.width - Theme.px(20)) * 0.62

            GameArt {
                anchors.fill: parent
                readonly property var game: page.running ? page.runningGame : page.hero
                visible: (page.hero !== null || page.running !== null) && !page.webOpen
                radius: Theme.radiusL
                seed: page.running ? page.running.name : (page.hero ? page.hero.title : "")
                // Art (4.4): a Steam game's wide picture, else its header, else the generated art; a
                // game on a disk shows its cover whole, on the right (owner 2026-10-16)
                readonly property string wide: game ? (game.assets.background || game.assets.banner || "") : ""
                readonly property string cover: game && GameInfo.onDisk(game) ? (game.assets.boxFront || "") : ""
                image: wide || cover
                fit: wide === "" && cover !== ""
                fitX: 0.92
                fitMargin: Theme.px(22)
                scrim: true
            }
            Rectangle {                         // welcome card, when there are no games; an open web app
                anchors.fill: parent
                visible: (page.hero === null && page.running === null) || page.webOpen
                radius: Theme.radiusL
                color: Theme.surface
                border.color: Theme.line
                border.width: Math.max(1, Theme.px(1))
            }
            Column {
                // at the bottom over the art, in the middle of the welcome card
                x: Theme.px(34)
                width: parent.width - Theme.px(68)
                readonly property bool art: (page.hero !== null || page.running !== null) && !page.webOpen
                y: art ? parent.height - height - Theme.px(30) : (parent.height - height) / 2
                spacing: Theme.px(12)
                readonly property color ink: art ? "#FFFFFF" : Theme.fg

                Rectangle {                     // an open web app: its letter, no art
                    visible: page.webOpen
                    width: Theme.fs(64); height: width
                    radius: Theme.px(18)
                    color: Theme.raised
                    Text {
                        anchors.centerIn: parent
                        text: page.runningWeb ? page.runningWeb.icon : ""
                        color: Theme.accent
                        font.family: Theme.displayFont
                        font.weight: Font.ExtraBold
                        font.pixelSize: Theme.fs(34)
                    }
                }

                Row {
                    spacing: Theme.px(8)
                    Rectangle {                 // a game is running
                        visible: page.running !== null
                        anchors.verticalCenter: parent.verticalCenter
                        width: Theme.px(9); height: width; radius: width / 2
                        color: Theme.running
                    }
                    Text {
                        text: page.webOpen ? Tr.tr("home.open")
                            : page.running ? Tr.tr("home.playing")
                            : page.hero ? Tr.tr(GameInfo.played(page.hero) ? "home.continue" : "home.ready") : Tr.tr("home.welcome")
                        color: parent.parent.ink
                        opacity: 0.85
                        font.family: Theme.textFont
                        font.weight: Font.Medium
                        font.pixelSize: Theme.fs(12.5)
                        font.capitalization: Font.AllUppercase
                        font.letterSpacing: Theme.fs(12.5) * 0.12
                    }
                }
                Text {
                    width: parent.width
                    text: page.webOpen ? (page.runningWeb ? Tr.tr(page.runningWeb.nameKey) : page.running.name)
                        : page.running ? page.running.name : page.hero ? page.hero.title : Tr.tr("home.empty.title")
                    color: parent.ink
                    font.family: Theme.displayFont
                    font.weight: Font.Bold
                    font.pixelSize: Theme.fs(40)
                    wrapMode: Text.Wrap
                    maximumLineCount: 2
                    elide: Text.ElideRight
                }
                Text {
                    visible: page.hero === null && page.running === null
                    width: Math.min(parent.width, Theme.px(560))
                    text: Tr.tr("home.empty.text")
                    color: Theme.fg
                    opacity: 0.9
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                    wrapMode: Text.Wrap
                }
                Row {
                    visible: page.hero !== null || page.running !== null
                    readonly property var game: page.running ? page.runningGame : page.hero
                    spacing: Theme.px(18)
                    Rectangle {
                        visible: parent.game !== null
                        width: sourceText.implicitWidth + Theme.px(20); height: sourceText.implicitHeight + Theme.px(4)
                        radius: height / 2
                        color: "#59000000"
                        anchors.verticalCenter: parent.verticalCenter
                        Text {
                            id: sourceText
                            anchors.centerIn: parent
                            text: GameInfo.source(parent.parent.game)
                            color: "#FFFFFF"
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(14.5)
                        }
                    }
                    Text {
                        text: page.webOpen ? Tr.trn("home.open.time", page.playingMinutes)
                            : page.running ? Tr.trn("home.session", page.playingMinutes) : GameInfo.lastPlayed(page.hero)
                        color: page.webOpen ? Theme.muted : "#E6FFFFFF"
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                        anchors.verticalCenter: parent.verticalCenter
                    }
                    Text {
                        visible: parent.game !== null && GameInfo.played(parent.game)
                        text: GameInfo.playTime(parent.game)
                        color: "#E6FFFFFF"
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                        anchors.verticalCenter: parent.verticalCenter
                    }
                    Rectangle {      // the pointer is on for the running game (4.7)
                        visible: page.running !== null && page.pointerOn
                        width: pointerBadge.implicitWidth + Theme.px(34); height: pointerBadge.implicitHeight + Theme.px(6)
                        radius: height / 2
                        color: Theme.accent
                        anchors.verticalCenter: parent.verticalCenter
                        Icon {
                            x: Theme.px(9); anchors.verticalCenter: parent.verticalCenter
                            width: Theme.px(15); height: width
                            name: "pointer"
                            color: Theme.onAccent
                        }
                        Text {
                            id: pointerBadge
                            x: Theme.px(26); anchors.verticalCenter: parent.verticalCenter
                            text: Tr.tr("home.pointer.on")
                            color: Theme.onAccent
                            font.family: Theme.textFont
                            font.weight: Font.Medium
                            font.pixelSize: Theme.fs(12.5)
                        }
                    }
                }
                Row {
                    spacing: Theme.px(20)
                    topPadding: Theme.px(4)
                    Button {
                        visible: page.running !== null && page.canResume
                        readonly property string kind: "resume"
                        text: Tr.tr("home.resume")
                        style: page.webOpen ? "primary" : "light"
                    }
                    Button {
                        visible: page.running !== null && page.canResume
                        property string kind: page.pointerOn ? "pointerOff" : "pointer"
                        text: Tr.tr(page.pointerOn ? "home.pointer.off" : "home.pointer")
                        style: "glass"
                    }
                    Button {
                        visible: page.running !== null
                        readonly property string kind: "close"
                        text: Tr.tr(page.webOpen ? "home.closeapp" : "home.close")
                        style: page.webOpen ? (page.canResume ? "secondary" : "primary")
                                            : (page.canResume ? "glass" : "light")
                    }
                    Button {
                        visible: page.hero !== null && page.running === null
                        readonly property string kind: "play"
                        readonly property var game: page.hero
                        text: Tr.tr("home.play")
                        style: "light"
                    }
                    Button {
                        visible: page.hero !== null && page.running === null
                        readonly property string kind: "details"
                        readonly property var game: page.hero
                        text: Tr.tr("home.details")
                        style: "glass"
                    }
                    Button {
                        visible: page.hero === null && page.running === null
                        readonly property string kind: "store"
                        text: Tr.tr("home.empty.store")
                        style: "primary"
                    }
                    Button {
                        visible: page.hero === null && page.running === null
                        readonly property string kind: "how"
                        text: Tr.tr("home.empty.how")
                    }
                }
            }
        }

        NavGroup {
            id: sideGroup
            anchors { left: heroGroup.right; leftMargin: Theme.px(20); right: parent.right; top: parent.top; bottom: parent.bottom }
            Column {
                width: parent.width
                spacing: Theme.px(12)
                Label { text: Tr.tr("home.apps") }
                Grid {
                    id: apps
                    width: parent.width
                    columns: 3
                    spacing: Theme.px(12)
                    readonly property real cell: (width - 2 * spacing) / 3
                    Repeater {
                        // Generic tiles, no third-party logos. The web apps come from owneetd's
                        // catalogue (generic tiles until it answers); Videos, Music and the store stay.
                        model: (page.webAppIds.length > 0
                                ? page.webAppIds.map(id => ({ id: id }))
                                : [{ key: "app.netflix", icon: "N" }, { key: "app.youtube", icon: "Y" }, { key: "app.spotify", icon: "S" }])
                            .concat([{ key: "app.videos", icon: "▶" }, { key: "app.music", icon: "♪" }, { key: "app.store", icon: "$" }])
                        Rectangle {
                            readonly property bool navigable: true
                            readonly property string kind: "app"
                            readonly property bool steamStore: modelData.key === "app.store"
                            readonly property var webApp: modelData.id ? page.webApp(modelData.id) : null
                            readonly property string nameKey: webApp ? webApp.nameKey : (modelData.key || "")
                            readonly property bool missing: webApp !== null && !webApp.installed && !webApp.busy
                            readonly property real dim: missing ? (activeFocus ? 0.85 : 0.55) : 1
                            width: apps.cell
                            height: Math.max(Theme.px(78), appName.implicitHeight + icon.height + Theme.px(28))
                            radius: Theme.radiusM
                            color: Theme.surface
                            border.color: Theme.line
                            border.width: Math.max(1, Theme.px(1))
                            Rectangle {
                                id: icon
                                x: Theme.px(12); y: Theme.px(10)
                                width: Theme.fs(30); height: width
                                radius: Theme.px(9)
                                color: Theme.raised
                                opacity: parent.dim
                                Text {
                                    anchors.centerIn: parent
                                    text: parent.parent.webApp ? parent.parent.webApp.icon : modelData.icon
                                    color: Theme.accent
                                    font.family: Theme.displayFont
                                    font.weight: Font.Bold
                                    font.pixelSize: Theme.fs(15)
                                }
                            }
                            Rectangle {                 // not installed: download mark
                                visible: parent.missing
                                anchors { right: parent.right; top: parent.top; rightMargin: Theme.px(10); topMargin: Theme.px(10) }
                                width: Theme.fs(22); height: width; radius: width / 2
                                color: Theme.raised
                                Icon {
                                    anchors.centerIn: parent
                                    width: parent.width * 0.62; height: width
                                    name: "download"
                                    color: Theme.fg
                                }
                            }
                            Rectangle {                 // open (5.1): green dot, as the running game
                                visible: page.isOpen(parent.webApp) && !parent.webApp.busy
                                anchors { right: parent.right; top: parent.top; rightMargin: Theme.px(14); topMargin: Theme.px(16) }
                                width: Theme.px(9); height: width; radius: width / 2
                                color: Theme.running
                            }
                            Spinner {                   // installing or removing
                                visible: parent.webApp !== null && !!parent.webApp.busy
                                anchors { right: parent.right; top: parent.top; rightMargin: Theme.px(10); topMargin: Theme.px(10) }
                                width: Theme.fs(18); height: width
                            }
                            Text {
                                id: appName
                                anchors { left: parent.left; right: parent.right; bottom: parent.bottom; margins: Theme.px(12); bottomMargin: Theme.px(10) }
                                text: parent.webApp && parent.webApp.busy
                                      ? Tr.tr(parent.webApp.busy === "install" ? "apps.installing" : "apps.removing")
                                      : Tr.tr(parent.nameKey)
                                color: parent.webApp && parent.webApp.busy ? Theme.muted : Theme.fg
                                opacity: parent.dim
                                font.family: Theme.textFont
                                font.pixelSize: Theme.fs(14.5)
                                elide: Text.ElideRight
                            }
                            FocusFrame {}
                        }
                    }
                }
                Repeater {                      // Steam's downloads (4.2c), A opens the game's details
                    model: page.notices.length > 0 ? 1 : 0
                    Rectangle {
                        readonly property bool navigable: true
                        readonly property string kind: "notice"
                        width: parent.width
                        height: noticeText.implicitHeight + Theme.px(22)
                        radius: Theme.radiusM
                        color: Theme.surface
                        border.color: Theme.line
                        border.width: Math.max(1, Theme.px(1))
                        Rectangle {
                            x: Theme.px(14); anchors.verticalCenter: parent.verticalCenter
                            width: Theme.px(9); height: width; radius: width / 2; color: Theme.accent
                        }
                        Text {
                            id: noticeText
                            anchors { left: parent.left; right: parent.right; leftMargin: Theme.px(34); rightMargin: Theme.px(14); verticalCenter: parent.verticalCenter }
                            text: page.notices[0].text
                            color: Theme.fg
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(12.5)
                            wrapMode: Text.Wrap
                        }
                        FocusFrame {}
                    }
                }
            }
        }
    }

    // ---- Recently played
    NavGroup {
        id: rowBlock
        visible: page.recent.length > 0
        anchors { left: parent.left; right: parent.right; bottom: parent.bottom }
        height: visible ? rowHead.height + Theme.px(12) + row.height : 0

        Item {
            id: rowHead
            width: parent.width
            height: Math.max(rowLabel.height, rowNote.height)
            Label { id: rowLabel; text: Tr.tr("home.recent") }
            Text {
                id: rowNote
                anchors.right: parent.right
                text: Tr.tr("home.unified")
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(12.5)
            }
        }
        Row {
            id: row
            anchors { top: rowHead.bottom; topMargin: Theme.px(12) }
            spacing: Theme.px(14)
            readonly property real cell: (rowBlock.width - 6 * spacing) / 7
            Repeater {
                model: page.recent
                Cover {
                    width: row.cell
                    height: row.cell * 1.25
                    game: modelData
                    running: page.running !== null && page.running.name === modelData.title
                }
            }
            Rectangle {                         // "All games": the library
                readonly property bool navigable: true
                readonly property string kind: "all"
                width: row.cell
                height: row.cell * 1.25
                radius: Theme.radiusM
                color: "transparent"
                border.color: Theme.line
                border.width: Theme.px(2)
                Text {
                    anchors.centerIn: parent
                    width: parent.width - Theme.px(16)
                    horizontalAlignment: Text.AlignHCenter
                    text: Tr.tr("home.all")
                    color: Theme.muted
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                    wrapMode: Text.Wrap
                }
                FocusFrame {}
            }
        }
    }
}
