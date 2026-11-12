// SPDX-License-Identifier: GPL-3.0-or-later
// The prompt bar at the bottom of every screen: what the buttons do here (left) and the
// system-wide buttons (right). It sits inside the safe area and never moves.
// prompts: [{ buttons: ["a"], label: "Select" }, ...]
// When it does not fit (owner 2026-11-12, approved demo design/demos/3.3c-prompt-bar-fit.html): if
// less than twice the gap between prompts would stay between the two groups, the prompts on the
// right show their buttons only; the screen's own prompts keep their words.
import QtQuick 2.15

Item {
    id: root
    property var prompts: []
    property var globalPrompts: [{ buttons: ["guide"], label: Tr.tr("prompt.home") }]
    property string note: ""
    implicitHeight: Theme.fs(26)

    // The right group's width with its words (measured on an unseen copy), whether they are shown or not
    readonly property real rightFull: fullRight.width
    readonly property bool tight: rightFull > 0 && leftRow.width + rightFull + 2 * Theme.px(26) > width

    Row {
        id: fullRight
        opacity: 0
        spacing: Theme.px(26)
        Text {
            visible: root.note !== ""
            text: root.note
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(14.5)
        }
        Repeater {
            model: root.globalPrompts
            Prompt { buttons: modelData.buttons; label: modelData.label }
        }
    }

    Row {
        id: leftRow
        anchors { left: parent.left; verticalCenter: parent.verticalCenter }
        spacing: Theme.px(26)
        Repeater {
            model: root.prompts
            Prompt { buttons: modelData.buttons; label: modelData.label }
        }
    }
    Row {
        id: rightRow
        anchors { right: parent.right; verticalCenter: parent.verticalCenter }
        spacing: root.tight ? Theme.px(18) : Theme.px(26)
        Text {
            id: noteText
            visible: root.note !== ""
            anchors.verticalCenter: parent.verticalCenter
            text: root.note
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(14.5)
        }
        Repeater {
            model: root.globalPrompts
            Prompt { buttons: modelData.buttons; label: modelData.label; compact: root.tight }
        }
    }
}
