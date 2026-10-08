// SPDX-License-Identifier: GPL-3.0-or-later
// Minimal development theme of OwneetOS: lists the games and starts the selected one with
// A / Enter. It replaces Pegasus's grid theme (CC BY-NC-SA, not free) until the OwneetOS theme
// exists (roadmap 3.3).
import QtQuick 2.15

FocusScope {
    id: root
    focus: true

    readonly property real margin: width * 0.05 // 5% TV safe area

    Rectangle {
        anchors.fill: parent
        color: "#0E1424"
    }

    Text {
        id: title
        text: "OwneetOS"
        color: "#E7EAF3"
        font.pixelSize: root.height * 0.06
        font.bold: true
        anchors { top: parent.top; left: parent.left; margins: root.margin }
    }

    ListView {
        id: games
        anchors {
            top: title.bottom; bottom: parent.bottom; left: parent.left; right: parent.right
            margins: root.margin
        }
        model: api.allGames
        focus: true
        clip: true
        highlightMoveDuration: 0
        delegate: Text {
            readonly property var game: modelData
            text: modelData.title
            color: ListView.isCurrentItem ? "#FF8A5B" : "#E7EAF3"
            font.pixelSize: root.height * 0.04
        }
        Keys.onPressed: {
            if (api.keys.isAccept(event) && !event.isAutoRepeat && currentItem) {
                event.accepted = true;
                currentItem.game.launch();
            }
        }
    }

    Text {
        visible: games.count === 0
        text: "No games found"
        color: "#9AA3BA"
        font.pixelSize: root.height * 0.04
        anchors.centerIn: parent
    }
}
