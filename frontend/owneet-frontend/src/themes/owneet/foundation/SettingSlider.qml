// SPDX-License-Identifier: GPL-3.0-or-later
// A settings row with a slider (0-100): left / right change it by `step` while it is selected.
import QtQuick 2.15

Rectangle {
    id: root
    property string text: ""
    property int value: 50
    property int step: 5
    readonly property bool navigable: true
    readonly property string kind: "slider"
    signal moved(int value)                      // the user changed it

    implicitHeight: Math.max(label.implicitHeight, Theme.fs(24)) + Theme.px(22)
    radius: Theme.radiusS + Theme.px(2)
    color: Theme.surface
    border.color: Theme.line
    border.width: Math.max(1, Theme.px(1))

    Keys.onPressed: {
        const a = Nav.action(event);
        if (a !== "left" && a !== "right")
            return;
        const next = Math.max(0, Math.min(100, value + (a === "right" ? step : -step)));
        if (next === value) {
            Nav.feedback("edge");
        } else {
            value = next;
            Nav.feedback("confirm");
            moved(next);
        }
        event.accepted = true;
    }

    Text {
        id: label
        anchors { left: parent.left; leftMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
        text: root.text
        color: Theme.fg
        font.family: Theme.textFont
        font.pixelSize: Theme.fs(14.5)
    }
    Row {
        anchors { right: parent.right; rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
        spacing: Theme.px(14)
        Rectangle {
            anchors.verticalCenter: parent.verticalCenter
            width: Theme.px(220); height: Theme.px(8); radius: height / 2
            color: Theme.raised
            Rectangle { width: parent.width * root.value / 100; height: parent.height; radius: height / 2; color: Theme.accent }
            Rectangle {
                x: parent.width * root.value / 100 - width / 2
                anchors.verticalCenter: parent.verticalCenter
                width: Theme.px(18); height: width; radius: width / 2
                color: Theme.fg
            }
        }
        Text {
            anchors.verticalCenter: parent.verticalCenter
            width: Theme.fs(30)
            horizontalAlignment: Text.AlignRight
            text: root.value
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(14.5)
        }
    }
    FocusFrame {}
}
