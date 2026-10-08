// SPDX-License-Identifier: GPL-3.0-or-later
// A settings row: a label on the left and, on the right, a switch (kind "switch") or the current
// value with an arrow that opens a picker (kind "picker").
import QtQuick 2.15

Rectangle {
    id: root
    property string text: ""
    property string kind: "switch"
    property bool checked: false
    property string value: ""
    property var swatches: []          // small color strip shown before the value
    signal activated()

    implicitHeight: Math.max(label.implicitHeight, Theme.fs(24)) + Theme.px(22)
    radius: Theme.radiusS + Theme.px(2)
    color: Theme.surface
    border.color: Theme.line
    border.width: Math.max(1, Theme.px(1))

    Text {
        id: label
        anchors { left: parent.left; leftMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
        text: root.text
        color: Theme.fg
        font.family: Theme.textFont
        font.pixelSize: Theme.fs(14.5)
    }
    Row {
        visible: root.kind === "picker"
        anchors { right: parent.right; rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
        spacing: Theme.px(10)
        Row {
            anchors.verticalCenter: parent.verticalCenter
            Repeater {
                model: root.swatches
                Rectangle { width: Theme.px(12); height: Theme.px(16); color: modelData }
            }
        }
        Text {
            anchors.verticalCenter: parent.verticalCenter
            text: root.value + "  ›"
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(14.5)
        }
    }
    Rectangle {          // switch
        visible: root.kind === "switch"
        anchors { right: parent.right; rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
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
