// SPDX-License-Identifier: GPL-3.0-or-later
// The focus ring: a gap in the background color, then a ring in the accent color, around the
// selected item, which is also lifted a little (unless "Reduce motion" is on). Put it inside the
// item it marks: FocusFrame { shown: parent.activeFocus }
import QtQuick 2.15

Item {
    id: root
    property bool shown: parent ? parent.activeFocus : false
    property real radius: parent && parent.radius !== undefined ? parent.radius : Theme.radiusM
    anchors.fill: parent
    z: 10
    visible: shown

    Rectangle {           // gap, so that the ring stands out from neighbouring items
        anchors { fill: parent; margins: -Theme.px(3) }
        radius: root.radius + Theme.px(3)
        color: "transparent"
        border.color: Theme.bg
        border.width: Theme.px(3)
        antialiasing: true
    }
    Rectangle {           // ring
        anchors { fill: parent; margins: -Theme.px(6.5) }
        radius: root.radius + Theme.px(6.5)
        color: "transparent"
        border.color: Theme.accent
        border.width: Theme.px(3.5)
        antialiasing: true
    }

    // The lift: the marked item grows by 3.5%, at most 12 px on each side for wide items
    // (none with "Reduce motion")
    property real lift: shown && !Theme.reduceMotion && parent
                        ? Math.min(0.035, Theme.px(24) / Math.max(1, parent.width)) : 0
    Behavior on lift { NumberAnimation { duration: Theme.motionMs; easing.type: Easing.OutCubic } }
    Binding { target: root.parent; property: "scale"; value: 1 + root.lift }
    Binding { target: root.parent; property: "z"; value: root.shown ? 2 : 0 }
}
