// SPDX-License-Identifier: GPL-3.0-or-later
// A game's cover: its own image from local files when there is one, otherwise generated art with
// the title. A badge shows where the game comes from (Steam, Local); a heart marks a favourite;
// a game owned but not installed is dimmed, with a download mark.
import QtQuick 2.15

Item {
    id: root
    property var game: null
    readonly property bool navigable: true
    readonly property string kind: "game"
    property real radius: Theme.radiusM
    signal activated()

    readonly property bool installed: GameInfo.installed(game)
    readonly property string image: game ? (game.assets.poster || game.assets.boxFront || game.assets.tile || "") : ""

    GameArt {
        anchors.fill: parent
        radius: root.radius
        seed: root.game ? root.game.title : ""
        image: root.image
        shade: true
        opacity: root.installed ? 1 : (root.activeFocus ? 0.8 : 0.55)
    }
    Text {
        anchors { left: parent.left; right: parent.right; bottom: parent.bottom; margins: Theme.px(10) }
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
    Rectangle {          // favourite, or "not installed"
        visible: !root.installed || (root.game && root.game.favorite)
        anchors { right: parent.right; top: parent.top; margins: Theme.px(8) }
        width: Theme.px(24); height: width; radius: width / 2
        color: "#80000000"
        Icon {
            anchors.centerIn: parent
            width: Theme.px(14); height: width
            name: root.installed ? "heart" : "download"
            color: "#FFFFFF"
        }
    }
    FocusFrame { radius: root.radius }
}
