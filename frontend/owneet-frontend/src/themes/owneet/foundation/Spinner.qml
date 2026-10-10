// SPDX-License-Identifier: GPL-3.0-or-later
// A small turning ring: something is in progress (searching, connecting).
import QtQuick 2.15

Item {
    id: root
    implicitWidth: Theme.fs(16)
    implicitHeight: implicitWidth
    Rectangle {
        anchors.fill: parent
        radius: width / 2
        color: "transparent"
        border.color: Theme.line
        border.width: Math.max(1, Theme.px(2))
    }
    Canvas {
        id: arc
        anchors.fill: parent
        readonly property color color: Theme.accent
        onColorChanged: requestPaint()
        onPaint: {
            const ctx = getContext("2d");
            ctx.reset();
            const w = Math.max(1, Theme.px(2));
            ctx.strokeStyle = color;
            ctx.lineWidth = w;
            ctx.beginPath();
            ctx.arc(width / 2, height / 2, width / 2 - w / 2, -Math.PI / 2, 0);
            ctx.stroke();
        }
        onWidthChanged: requestPaint()
        RotationAnimator on rotation {
            running: root.visible
            from: 0; to: 360
            duration: Theme.reduceMotion ? 3000 : 1000
            loops: Animation.Infinite
        }
    }
}
