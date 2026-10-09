// SPDX-License-Identifier: GPL-3.0-or-later
// A short message above the prompt bar that fades away: Toast.show(text).
import QtQuick 2.15

Rectangle {
    id: root
    function show(message) {
        label.text = message;
        opacity = 1;
        timer.restart();
    }
    anchors { horizontalCenter: parent.horizontalCenter; bottom: parent.bottom; bottomMargin: Theme.px(74) }
    width: label.implicitWidth + Theme.px(36)
    height: label.implicitHeight + Theme.px(18)
    radius: height / 2
    color: Theme.raised
    border.color: Theme.line
    border.width: Math.max(1, Theme.px(1))
    opacity: 0
    visible: opacity > 0
    z: 200
    Behavior on opacity { NumberAnimation { duration: Theme.reduceMotion ? 0 : 200 } }
    Text {
        id: label
        anchors.centerIn: parent
        color: Theme.fg
        font.family: Theme.textFont
        font.pixelSize: Theme.fs(14.5)
    }
    Timer { id: timer; interval: 1800; onTriggered: root.opacity = 0 }
}
