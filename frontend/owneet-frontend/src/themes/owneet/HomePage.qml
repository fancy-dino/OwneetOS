// SPDX-License-Identifier: GPL-3.0-or-later
// The home screen (roadmap 3.6, approved demo design/demos/3.6-home.html): the last game played
// (or a welcome on a system with no games), the apps, notices, and the recently played games.
// Three navigation groups: hero, apps column, row of games.
// Still to come: the running game in the hero (Resume / Close game) with owneetd (3.8), notices
// (3.11), apps (phase 5), real covers from Steam's local files (4.1).
import QtQuick 2.15
import "foundation"

NavArea {
    id: page
    property var games: []                     // all games, most recently played first
    readonly property var hero: games.length > 0 ? games[0] : null
    readonly property var recent: games.slice(1, 7)
    property var notices: []                   // [{ text }], from owneetd later (3.11)

    signal launch(var game)
    signal details(var game)
    signal options(var game)
    signal openLibrary()
    signal howToAdd()
    signal notYet()                            // a feature of a later step

    // The prompt bar follows the selected item
    readonly property var prompts: {
        const k = current ? current.kind : "";
        const P = (b, l) => ({ buttons: [b], label: Tr.tr(l) });
        if (k === "game" || k === "play")
            return [P("a", "prompt.play"), P("x", "prompt.details"), P("menu", "prompt.options")];
        if (k === "details")
            return [P("a", "prompt.details"), P("menu", "prompt.options")];
        if (k === "app" || k === "all" || k === "notice" || k === "store")
            return [P("a", "prompt.open")];
        return k ? [P("a", "prompt.select")] : [];
    }
    function gameOf(item) { return item ? (item.game || null) : null; }

    function focusStart() { focusItem(Nav.navigables(heroGroup)[0] || null); }
    // The games arrive after the page: keep a visible item selected (e.g. Play instead of the
    // welcome buttons)
    onHeroChanged: Qt.callLater(() => { if (!current || !current.visible) focusStart(); })
    // While the boot splash is shown the page is disabled and nothing can be selected yet
    onActiveFocusChanged: if (activeFocus && (!current || !current.visible)) focusStart()

    onAccepted: {
        const k = item.kind;
        if (k === "game" || k === "play") launch(gameOf(item));
        else if (k === "details") details(gameOf(item));
        else if (k === "all") openLibrary();
        else if (k === "how") howToAdd();
        else notYet();                         // apps, notices, Steam Store
    }
    onOtherAction: {
        const game = gameOf(current);
        if (action === "secondary" && game && current.kind !== "details") { details(game); event.accepted = true; }
        else if (action === "options" && game) { options(game); event.accepted = true; }
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
                visible: page.hero !== null
                radius: Theme.radiusL
                seed: page.hero ? page.hero.title : ""
                image: page.hero ? (page.hero.assets.background || page.hero.assets.screenshot || page.hero.assets.banner || "") : ""
                scrim: true
            }
            Rectangle {                         // welcome card, when there are no games
                anchors.fill: parent
                visible: page.hero === null
                radius: Theme.radiusL
                color: Theme.surface
                border.color: Theme.line
                border.width: Math.max(1, Theme.px(1))
            }
            Column {
                // at the bottom over the art, in the middle of the welcome card
                x: Theme.px(34)
                width: parent.width - Theme.px(68)
                y: page.hero ? parent.height - height - Theme.px(30) : (parent.height - height) / 2
                spacing: Theme.px(12)
                readonly property color ink: page.hero ? "#FFFFFF" : Theme.fg

                Text {
                    text: page.hero ? Tr.tr(GameInfo.played(page.hero) ? "home.continue" : "home.ready") : Tr.tr("home.welcome")
                    color: parent.ink
                    opacity: 0.85
                    font.family: Theme.textFont
                    font.weight: Font.Medium
                    font.pixelSize: Theme.fs(12.5)
                    font.capitalization: Font.AllUppercase
                    font.letterSpacing: Theme.fs(12.5) * 0.12
                }
                Text {
                    width: parent.width
                    text: page.hero ? page.hero.title : Tr.tr("home.empty.title")
                    color: parent.ink
                    font.family: Theme.displayFont
                    font.weight: Font.Bold
                    font.pixelSize: Theme.fs(40)
                    wrapMode: Text.Wrap
                    maximumLineCount: 2
                    elide: Text.ElideRight
                }
                Text {
                    visible: page.hero === null
                    width: Math.min(parent.width, Theme.px(560))
                    text: Tr.tr("home.empty.text")
                    color: Theme.fg
                    opacity: 0.9
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                    wrapMode: Text.Wrap
                }
                Row {
                    visible: page.hero !== null
                    spacing: Theme.px(18)
                    Rectangle {
                        width: sourceText.implicitWidth + Theme.px(20); height: sourceText.implicitHeight + Theme.px(4)
                        radius: height / 2
                        color: "#59000000"
                        anchors.verticalCenter: parent.verticalCenter
                        Text {
                            id: sourceText
                            anchors.centerIn: parent
                            text: GameInfo.source(page.hero)
                            color: "#FFFFFF"
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(14.5)
                        }
                    }
                    Text {
                        text: GameInfo.lastPlayed(page.hero)
                        color: "#E6FFFFFF"
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                        anchors.verticalCenter: parent.verticalCenter
                    }
                    Text {
                        visible: GameInfo.played(page.hero)
                        text: GameInfo.playTime(page.hero)
                        color: "#E6FFFFFF"
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                        anchors.verticalCenter: parent.verticalCenter
                    }
                }
                Row {
                    spacing: Theme.px(20)
                    topPadding: Theme.px(4)
                    Button {
                        visible: page.hero !== null
                        readonly property string kind: "play"
                        readonly property var game: page.hero
                        text: Tr.tr("home.play")
                        style: "light"
                    }
                    Button {
                        visible: page.hero !== null
                        readonly property string kind: "details"
                        readonly property var game: page.hero
                        text: Tr.tr("home.details")
                        style: "glass"
                    }
                    Button {
                        visible: page.hero === null
                        readonly property string kind: "store"
                        text: Tr.tr("home.empty.store")
                        style: "primary"
                    }
                    Button {
                        visible: page.hero === null
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
                        // Generic tiles, no third-party logos (real icons fetched at runtime, phase 5)
                        model: [
                            { name: "Netflix", icon: "N" }, { name: "YouTube", icon: "Y" }, { name: "Spotify", icon: "S" },
                            { key: "app.videos", icon: "▶" }, { key: "app.music", icon: "♪" }, { key: "app.store", icon: "$" }
                        ]
                        Rectangle {
                            readonly property bool navigable: true
                            readonly property string kind: "app"
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
                                Text {
                                    anchors.centerIn: parent
                                    text: modelData.icon
                                    color: Theme.accent
                                    font.family: Theme.displayFont
                                    font.weight: Font.Bold
                                    font.pixelSize: Theme.fs(15)
                                }
                            }
                            Text {
                                id: appName
                                anchors { left: parent.left; right: parent.right; bottom: parent.bottom; margins: Theme.px(12); bottomMargin: Theme.px(10) }
                                text: modelData.key ? Tr.tr(modelData.key) : modelData.name
                                color: Theme.fg
                                font.family: Theme.textFont
                                font.pixelSize: Theme.fs(14.5)
                                elide: Text.ElideRight
                            }
                            FocusFrame {}
                        }
                    }
                }
                Repeater {                      // the latest notice (3.11), A opens, X dismisses
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
