// SPDX-License-Identifier: GPL-3.0-or-later
// A generic controller outline drawn for OwneetOS (no third-party drawings or logos), seen from
// the front, with the buttons to hold for pairing marked where they really are, with a pulsing
// ring (approved in demo 3.9, review 1): kind "xbox" and "nintendo" mark the pairing / sync
// button on the top edge next to the cable port, "ps" marks PS and Create. Drawn on a 120 x 80
// grid.
import QtQuick 2.15

Item {
    id: root
    property string kind: "xbox"
    property color color: Theme.muted
    implicitWidth: Theme.px(170)
    implicitHeight: Theme.px(100)
    // the drawing keeps its proportions inside the item
    readonly property real unit: Math.min(width / 120, height / 80)
    readonly property real ox: (width - 120 * unit) / 2
    readonly property real oy: (height - 80 * unit) / 2
    readonly property var presses: ({ xbox: [[50, 10]], ps: [[60, 46], [35, 22]], nintendo: [[50, 10]] })[kind] || []

    Canvas {
        id: canvas
        anchors.fill: parent
        onPaint: {
            const ctx = getContext("2d");
            ctx.reset();
            ctx.translate(root.ox, root.oy);
            ctx.scale(root.unit, root.unit);
            ctx.strokeStyle = root.color;
            ctx.fillStyle = root.color;
            ctx.lineJoin = "round";
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.path = "M22 16Q60 5 98 16Q114 20 115 42Q118 70 104 74Q94 77 86 62L34 62Q26 77 16 74Q2 70 5 42Q6 20 22 16Z";
            ctx.stroke();
            ctx.globalAlpha = 0.5;                       // cable port
            ctx.fillRect(56, 7, 8, 4);
            ctx.globalAlpha = 1;
            ctx.lineWidth = 1.6;
            const circle = (x, y, r) => { ctx.beginPath(); ctx.arc(x, y, r, 0, Math.PI * 2); ctx.stroke(); };
            if (root.kind === "ps") {
                circle(44, 52, 6); circle(76, 52, 6);
                ctx.beginPath();
                ctx.roundedRect(40, 18, 40, 20, 4, 4);   // touchpad
                ctx.stroke();
            } else {
                circle(34, 34, 6); circle(74, 50, 6); circle(90, 34, 3); circle(46, 50, 3);
                if (root.kind === "xbox")
                    circle(60, 24, 4.5);                 // the Xbox button
            }
        }
    }
    onColorChanged: canvas.requestPaint()
    onKindChanged: canvas.requestPaint()
    onWidthChanged: canvas.requestPaint()
    onHeightChanged: canvas.requestPaint()

    Repeater {
        model: root.presses
        Item {
            x: root.ox + modelData[0] * root.unit
            y: root.oy + modelData[1] * root.unit
            readonly property real r: 4 * root.unit
            Rectangle {                                  // pulsing ring
                id: ring
                x: -parent.r; y: -parent.r
                width: parent.r * 2; height: width; radius: width / 2
                color: "transparent"
                border.color: Theme.accent
                border.width: Math.max(1, 2 * root.unit / 1.4)
                visible: !Theme.reduceMotion
                SequentialAnimation {
                    running: ring.visible && root.visible
                    loops: Animation.Infinite
                    ParallelAnimation {
                        NumberAnimation { target: ring; property: "scale"; from: 1; to: 2.6; duration: 1200 }
                        NumberAnimation { target: ring; property: "opacity"; from: 0.9; to: 0; duration: 1200 }
                    }
                }
            }
            Rectangle {                                  // the button
                x: -parent.r; y: -parent.r
                width: parent.r * 2; height: width; radius: width / 2
                color: Theme.accent
            }
        }
    }
}
