// SPDX-License-Identifier: GPL-3.0-or-later
// A game's cover: its own image from local files when there is one, otherwise generated art with
// the title. A badge shows where the game comes from (Steam, Local); a heart marks a favourite;
// a game owned but not installed is dimmed, with a download mark; a Steam game being downloaded
// shows its state and progress under its title (4.2c); so does a game being installed from its
// installer (4.6: a moving bar when the installer tells no percentage), and one that cannot be
// installed yet has an "info" mark.
import QtQuick 2.15

Item {
    id: root
    property var game: null
    readonly property bool navigable: true
    readonly property string kind: "game"
    property real radius: Theme.radiusM
    property bool running: false                // this game runs now
    signal activated()

    // Steam (4.2c): not installed, downloading, or on a disk that is not connected
    readonly property string steamState: game ? GameInfo.steamState(game) : "installed"
    readonly property bool installed: steamState === "installed" || steamState === "updating"
    readonly property string stateText: game ? GameInfo.stateText(game) : ""
    readonly property bool busy: steamState === "downloading" || steamState === "updating" || steamState === "queued"
                                 || steamState === "paused" || steamState === "installing"
    // installing with no percentage (Windows installers, 4.6): the bar keeps moving
    readonly property bool unknownProgress: steamState === "installing" && GameInfo.install(game) !== null
                                            && GameInfo.install(game).total <= 0
    // Art (4.4): the portrait cover (Steam's, or cover.jpg / .png of a game on a disk), else a
    // Steam game's wide header shown whole over the generated art (option A, owner 2026-10-16),
    // else the generated art
    readonly property string cover: game ? (game.assets.poster || game.assets.boxFront || game.assets.tile || "") : ""
    readonly property string image: cover || (game ? game.assets.banner : "") || ""

    GameArt {
        anchors.fill: parent
        radius: root.radius
        seed: root.game ? root.game.title : ""
        image: root.image
        fit: root.cover === "" && root.image !== ""
        fitY: 0.4
        shade: true
        opacity: root.installed ? 1 : (root.activeFocus ? 0.8 : 0.55)
    }
    Column {
        anchors { left: parent.left; right: parent.right; bottom: parent.bottom; margins: Theme.px(10) }
        spacing: Theme.px(6)
        Text {
            width: parent.width
            text: root.game ? root.game.title : ""
            color: "#FFFFFF"
            font.family: Theme.displayFont
            font.weight: Font.Bold
            font.pixelSize: Theme.fs(17)
            wrapMode: Text.Wrap
            maximumLineCount: 3
            elide: Text.ElideRight
            lineHeight: 0.95
        }
        Text {               // the state: "Not installed", "Downloading · 45%"…
            visible: root.stateText !== ""
            width: parent.width
            text: root.stateText
            color: "#FFFFFF"
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(12.5)
            wrapMode: Text.Wrap
            maximumLineCount: 2
            elide: Text.ElideRight
        }
        Rectangle {          // the download's progress
            visible: root.busy
            width: parent.width; height: Theme.px(5); radius: height / 2
            color: "#40FFFFFF"
            clip: true
            Rectangle {
                visible: !root.unknownProgress
                width: parent.width * (root.game ? GameInfo.progress(root.game) : 0) / 100
                height: parent.height; radius: height / 2
                color: "#FFFFFF"
                Behavior on width { NumberAnimation { duration: Theme.motionMs } }
            }
            Rectangle {      // no percentage: a piece that keeps moving
                id: moving
                visible: root.unknownProgress
                width: parent.width * 0.35; height: parent.height; radius: height / 2
                color: "#FFFFFF"
                NumberAnimation on x {
                    running: moving.visible && !Theme.reduceMotion
                    from: -moving.width; to: moving.parent.width
                    duration: 1400; loops: Animation.Infinite
                }
            }
        }
    }
    Rectangle {          // source badge
        x: Theme.px(8); y: Theme.px(8)
        width: badge.implicitWidth + Theme.px(18)
        height: badge.implicitHeight + Theme.px(4)
        radius: height / 2
        color: "#73000000"
        Text {
            id: badge
            anchors.centerIn: parent
            text: GameInfo.source(root.game)
            color: "#FFFFFF"
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(12.5)
        }
    }
    Rectangle {          // running now
        visible: root.running
        anchors { right: parent.right; top: parent.top; margins: Theme.px(8) }
        width: runningText.implicitWidth + Theme.px(18)
        height: runningText.implicitHeight + Theme.px(4)
        radius: height / 2
        color: "#4ADE80"
        Text {
            id: runningText
            anchors.centerIn: parent
            text: Tr.tr("game.running")
            color: "#062010"
            font.family: Theme.textFont
            font.weight: Font.DemiBold
            font.pixelSize: Theme.fs(12.5)
        }
    }
    Row {                // favourite, and "not installed" or "on a disk that is not connected"
        visible: !root.running
        anchors { right: parent.right; top: parent.top; margins: Theme.px(8) }
        spacing: Theme.px(5)
        Repeater {
            model: (root.game && root.game.favorite ? ["heart"] : [])
                   .concat(root.steamState === "none" ? ["download"] : root.steamState === "away" ? ["unplug"]
                           : root.steamState === "unusable" ? ["info"] : [])
            Rectangle {
                width: Theme.px(24); height: width; radius: width / 2
                color: "#80000000"
                Icon {
                    anchors.centerIn: parent
                    width: Theme.px(14); height: width
                    name: modelData
                    color: "#FFFFFF"
                }
            }
        }
    }
    FocusFrame { radius: root.radius }
}
