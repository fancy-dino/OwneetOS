// SPDX-License-Identifier: GPL-3.0-or-later
// A small icon drawn for OwneetOS from an SVG path on a 24 x 24 grid (no icon fonts or images).
// Named icons: "heart", "download", "controller".
import QtQuick 2.15

Canvas {
    id: root
    property string name: ""
    property color color: Theme.fg
    property bool filled: name === "heart"
    readonly property var paths: ({
        heart: "M12 20s-7-4.4-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.6-7 10-7 10Z",
        download: "M12 4v11M7 10l5 5 5-5M5 20h14",
        controller: "M7 8h10a5 5 0 0 1 4.8 6.3l-.9 3.2a2 2 0 0 1-3.3.9L15 16H9l-2.6 2.4a2 2 0 0 1-3.3-.9l-.9-3.2A5 5 0 0 1 7 8Z"
    })
    implicitWidth: Theme.px(16)
    implicitHeight: implicitWidth
    onColorChanged: requestPaint()
    onNameChanged: requestPaint()
    onWidthChanged: requestPaint()
    onPaint: {
        const ctx = getContext("2d");
        ctx.reset();
        if (!paths[name])
            return;
        ctx.scale(width / 24, height / 24);
        ctx.path = paths[name];
        if (filled) {
            ctx.fillStyle = color;
            ctx.fill();
        } else {
            ctx.strokeStyle = color;
            ctx.lineWidth = 2.4;
            ctx.lineCap = "round";
            ctx.lineJoin = "round";
            ctx.stroke();
        }
    }
}
