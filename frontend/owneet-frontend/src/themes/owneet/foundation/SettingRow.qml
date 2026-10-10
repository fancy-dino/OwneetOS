// SPDX-License-Identifier: GPL-3.0-or-later
// A settings row: a label on the left and, on the right, a switch (kind "switch"), the current
// value with an arrow that opens a picker (kind "picker"), or an action with an optional state
// text and no arrow (kind "action", e.g. "Search again").
import QtQuick 2.15

Rectangle {
    id: root
    property string text: ""
    property string detail: ""         // a second, smaller line under the label
    property string kind: "switch"
    property bool checked: false
    property string value: ""
    property var swatches: []          // small color strip shown before the value
    readonly property bool navigable: true
    readonly property string feedbackKind: kind === "switch" ? (checked ? "toggle-off" : "toggle-on") : "confirm"
    signal activated()

    implicitHeight: Math.max(labels.implicitHeight, Theme.fs(24)) + Theme.px(22)
    radius: Theme.radiusS + Theme.px(2)
    color: Theme.surface
    border.color: Theme.line
    border.width: Math.max(1, Theme.px(1))

    Column {
        id: labels
        anchors { left: parent.left; leftMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
        width: parent.width * 0.6
        spacing: Theme.px(2)
        Text {
            id: label
            width: parent.width
            text: root.text
            color: Theme.fg
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(14.5)
            elide: Text.ElideRight
        }
        Text {
            visible: root.detail !== ""
            width: parent.width
            text: root.detail
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(12.5)
            wrapMode: Text.Wrap
        }
    }
    Row {
        visible: root.kind === "picker" || root.kind === "action"
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
            text: root.kind === "picker" ? root.value + "  ›" : root.value
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
