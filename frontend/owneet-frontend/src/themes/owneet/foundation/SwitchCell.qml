// SPDX-License-Identifier: GPL-3.0-or-later
// A switch on its own, for a table of switches (approved demo 6.6c: "Copy" and "Remove the
// original" on each game): the same switch as SettingRow's, selectable by itself. Off
// (`available` false) it is dimmed and skipped by the selection.
import QtQuick 2.15

Rectangle {
    id: root
    property bool checked: false
    property bool available: true
    readonly property bool navigable: available
    readonly property string feedbackKind: checked ? "toggle-off" : "toggle-on"
    signal activated()

    implicitWidth: Theme.px(44) + 2 * Theme.px(6)
    implicitHeight: Theme.px(24) + 2 * Theme.px(6)
    radius: height / 2
    color: "transparent"
    opacity: available ? 1 : 0.35

    Rectangle {          // the switch, as in SettingRow
        anchors.centerIn: parent
        width: Theme.px(44); height: Theme.px(24); radius: height / 2
        color: root.checked ? Theme.accent : Theme.raised
        border.color: root.checked ? Theme.accent : Theme.line
        border.width: Math.max(1, Theme.px(1))
        Rectangle {
            width: Theme.px(16); height: width; radius: width / 2
            anchors.verticalCenter: parent.verticalCenter
            x: root.checked ? Theme.px(23) : Theme.px(3)
            color: root.checked ? Theme.onAccent : Theme.muted
            Behavior on x { NumberAnimation { duration: Theme.motionMs; easing.type: Easing.OutCubic } }
        }
    }
    FocusFrame {}
}
