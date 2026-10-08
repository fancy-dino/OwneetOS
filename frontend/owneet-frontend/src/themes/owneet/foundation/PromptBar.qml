// SPDX-License-Identifier: GPL-3.0-or-later
// The prompt bar at the bottom of every screen: what the buttons do here (left) and the
// system-wide buttons (right). It sits inside the safe area and never moves.
// prompts: [{ buttons: ["a"], label: "Select" }, ...]
import QtQuick 2.15

Item {
    id: root
    property var prompts: []
    property var globalPrompts: [{ buttons: ["guide"], label: "Home" }]
    property string note: ""
    implicitHeight: Theme.fs(26)

    Row {
        anchors { left: parent.left; verticalCenter: parent.verticalCenter }
        spacing: Theme.px(26)
        Repeater {
            model: root.prompts
            Prompt { buttons: modelData.buttons; label: modelData.label }
        }
    }
    Row {
        anchors { right: parent.right; verticalCenter: parent.verticalCenter }
        spacing: Theme.px(26)
        Text {
            visible: root.note !== ""
            anchors.verticalCenter: parent.verticalCenter
            text: root.note
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(14.5)
        }
        Repeater {
            model: root.globalPrompts
            Prompt { buttons: modelData.buttons; label: modelData.label }
        }
    }
}
