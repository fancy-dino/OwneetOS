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
// instead of art, Resume and Close; its tile shows a green dot. The Videos and Music tiles open
// their pages ([[step-5.6-local-videos-and-music]], approved demo design/demos/5.6-videos-and-music.html);
// the player open takes the hero like a game: what plays, its time, Resume and Close; its tile
// shows a green dot.
// The apps' places ([[step-5.7-apps-page]], approved demo design/demos/5.7-apps-page.html): the
// built-in apps first, then the web apps; with more than six apps five places and "All apps", which
// opens the Apps page (the five most used from the seventh installed app; foundation/apps.js).
// Still to come: real covers from Steam's local files (4.1).
import QtQuick 2.15
import "foundation"
import "foundation/apps.js" as Apps

NavArea {
    id: page
    property var games: []                     // all games, most recently played first
    readonly property var hero: games.length > 0 ? games[0] : null
    // with a game running, the hero shows it and the row starts from the most recent game
    readonly property var recent: running ? games.slice(0, 6) : games.slice(1, 7)
    property var notices: []                   // [{ text, game }]: Steam's downloads (4.2c)
    // The web apps of the catalogue (owneetd): { id, nameKey, icon, installed, busy, sizeMiB }
    property var webApps: []
    function webApp(id) { return webApps.find(a => a.id === id) || null; }
    // How many times each app was opened from the console ({ key: count }, kept by the shell)
    property var uses: ({})
    // The apps in the fixed order ({ key, installed }; generic web apps until owneetd answers), and
    // the home's places (foundation/apps.js, [[step-5.7-apps-page]]): with six apps or fewer every
    // app, a missing one in its place; beyond, five and "All apps"
    readonly property var appList: Apps.fixedOrder(webApps.length > 0 ? webApps
        : [{ id: "netflix", installed: true }, { id: "youtube", installed: true }, { id: "spotify", installed: true }])
    readonly property var homeKeysNow: Apps.homeOrder(appList, uses)
    // Changed only when the places change: the tiles stay (and stay selected) while an app is
    // installed or removed; when they move, the selection follows its app
    property var homeKeys: []
    onHomeKeysNowChanged: {
        if (homeKeysNow.join() === homeKeys.join()) return;
        const key = current && (current.kind === "app" || current.kind === "allApps") ? current.key : "";
        homeKeys = homeKeysNow;
        if (key !== "") Qt.callLater(() => reselectApp(key));
    }
    Component.onCompleted: homeKeys = homeKeysNow
    function reselectApp(key) {
        const tiles = Nav.navigables(sideGroup).filter(i => i.kind === "app" || i.kind === "allApps");
        const tile = tiles.find(i => i.key === key) || tiles.find(i => i.kind === "allApps") || tiles[0] || null;
        if (!tile) return;
        if (activeFocus) { focusItem(tile); return; }
        sideGroup.remembered = tile;             // a window over the page keeps the focus
        current = tile;
    }
    property var running: null                 // the running game, web app or player: { id, name, kind, started }
    property bool canResume: true
    // The player open (5.6): what plays ({ title, line, position, duration, picture, seed }), and
    // whether it is the Videos or the Music page's ("videos", "music")
    property var nowPlaying: null
    property string playerKind: ""
    readonly property bool mediaOpen: running !== null && running.kind === "media"
    // An open web app (5.1): its catalogue entry
    readonly property bool webOpen: running !== null && running.kind === "webapp"
    readonly property var runningWeb: webOpen ? (webApps.find(a => "web:" + a.id === running.id) || null) : null
    function isOpen(app) { return app !== null && running !== null && running.id === "web:" + app.id; }
    // The Pegasus game of the running one (same title), for its art and source
    readonly property var runningGame: {
        if (!running || webOpen || mediaOpen) return null;
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
    signal openMedia(string kind)              // the Videos ("videos") or Music ("music") tile
    signal openApps()                          // "All apps": the Apps page (5.7)
    // Back from the Videos or Music page ("videos", "music") or the Apps page ("allapps"): the
    // selection on its tile
    function focusApp(key) {
        const tile = Nav.navigables(sideGroup).find(i => (i.kind === "app" || i.kind === "allApps") && i.key === key);
        if (tile) focusItem(tile); else focusStart();
    }
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
        if (k === "app" || k === "all" || k === "allApps" || k === "notice" || k === "store")
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
        else if (k === "allApps") openApps();
        else if (k === "how") howToAdd();
        else if (k === "resume") resume(running);
        else if (k === "close") closeApp(running);
        else if (k === "pointer") resumeWithPointer(running);
        else if (k === "pointerOff") pointerOff();
        else if (k === "store" || item.steamStore) openStore();
        else if (k === "app" && item.webApp) openWebApp(item.webApp);
        else if (k === "notice" && page.notices.length > 0) openNotice(page.notices[0]);
        else if (k === "app" && item.mediaKind !== "") openMedia(item.mediaKind);
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
                seed: page.mediaOpen && page.nowPlaying ? page.nowPlaying.seed
                    : page.running ? page.running.name : (page.hero ? page.hero.title : "")
                // Art (4.4): a Steam game's wide picture, else its header, else the generated art; a
                // game on a disk shows its cover whole, on the right (owner 2026-10-16)
                readonly property string wide: game ? (game.assets.background || game.assets.banner || "") : ""
                readonly property string cover: game && GameInfo.onDisk(game) ? (game.assets.boxFront || "") : ""
                image: page.mediaOpen ? (page.nowPlaying ? page.nowPlaying.picture : "") : (wide || cover)
                fit: !page.mediaOpen && wide === "" && cover !== ""
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
                        : page.mediaOpen ? (page.nowPlaying ? page.nowPlaying.title : page.running.name)
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
                            : page.mediaOpen ? (page.nowPlaying ? page.nowPlaying.line : "")
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
                    Row {            // the player: where it is in what plays
                        visible: page.mediaOpen && page.nowPlaying !== null && page.nowPlaying.duration > 0
                        anchors.verticalCenter: parent.verticalCenter
                        spacing: Theme.px(10)
                        Rectangle {
                            anchors.verticalCenter: parent.verticalCenter
                            width: Theme.px(120); height: Theme.px(5); radius: height / 2
                            color: "#40FFFFFF"
                            Rectangle {
                                width: parent.width * (page.nowPlaying && page.nowPlaying.duration > 0
                                                       ? Math.min(1, page.nowPlaying.position / page.nowPlaying.duration) : 0)
                                height: parent.height; radius: height / 2
                                color: Theme.accent
                            }
                        }
                        Text {
                            anchors.verticalCenter: parent.verticalCenter
                            text: page.nowPlaying ? MediaText.clock(page.nowPlaying.position) + " / " + MediaText.clock(page.nowPlaying.duration) : ""
                            color: "#E6FFFFFF"
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(14.5)
                        }
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
                        visible: page.running !== null && page.canResume && !page.mediaOpen
                        property string kind: page.pointerOn ? "pointerOff" : "pointer"
                        text: Tr.tr(page.pointerOn ? "home.pointer.off" : "home.pointer")
                        style: "glass"
                    }
                    Button {
                        visible: page.running !== null
                        readonly property string kind: "close"
                        text: Tr.tr(page.webOpen || page.mediaOpen ? "home.closeapp" : "home.close")
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
                        // The home's places ([[step-5.7-apps-page]]): AppTile, "All apps" beyond six apps
                        model: page.homeKeys
                        AppTile {
                            width: apps.cell
                            key: modelData
                            webApp: key.startsWith("web:") ? page.webApp(key.slice(4)) : null
                            open: (webApp !== null && page.isOpen(webApp))
                                  || (mediaKind !== "" && page.mediaOpen && page.playerKind === mediaKind)
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
