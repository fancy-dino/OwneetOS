// SPDX-License-Identifier: GPL-3.0-or-later
// A small icon drawn for OwneetOS from an SVG path on a 24 x 24 grid (no icon fonts or images).
// Named icons: "heart", "download", "controller", "wifi" (with `level` 0-3: arcs above it are
// dimmed), "ethernet", "battery" (filled by `fill`, 0-1). `crossed` draws a slash over the icon
// (e.g. offline).
import QtQuick 2.15

Canvas {
    id: root
    property string name: ""
    property color color: Theme.fg
    property bool filled: name === "heart"
    property int level: 3
    property real fill: 1
    property bool crossed: false
    onFillChanged: requestPaint()
    onCrossedChanged: requestPaint()
    function slash(ctx) {
        if (!crossed)
            return;
        ctx.globalAlpha = 1;
        ctx.strokeStyle = color;
        ctx.lineWidth = 2.2;
        ctx.lineCap = "round";
        ctx.beginPath();
        ctx.moveTo(3, 3);
        ctx.lineTo(21, 21);
        ctx.stroke();
    }
    readonly property var paths: ({
        heart: "M12 20s-7-4.4-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.6-7 10-7 10Z",
        download: "M12 4v11M7 10l5 5 5-5M5 20h14",
        controller: "M7 8h10a5 5 0 0 1 4.8 6.3l-.9 3.2a2 2 0 0 1-3.3.9L15 16H9l-2.6 2.4a2 2 0 0 1-3.3-.9l-.9-3.2A5 5 0 0 1 7 8Z",
        ethernet: "M5 9h14v8H5zM9 17v3M15 17v3M8 9V6h8v3"
    })
    // Wi-Fi: three arcs and a dot, from the largest arc down
    readonly property var wifiArcs: ["M2 9a15 15 0 0 1 20 0", "M5 12.5a10 10 0 0 1 14 0", "M8.5 16a5 5 0 0 1 7 0"]
    onLevelChanged: requestPaint()
    implicitWidth: Theme.px(16)
    implicitHeight: implicitWidth
    onColorChanged: requestPaint()
    onNameChanged: requestPaint()
    onWidthChanged: requestPaint()
    onPaint: {
        const ctx = getContext("2d");
        ctx.reset();
        ctx.scale(width / 24, height / 24);
        if (name === "battery") {
            ctx.strokeStyle = color;
            ctx.fillStyle = color;
            ctx.lineWidth = 1.8;
            ctx.beginPath();
            ctx.roundedRect(2.5, 7, 17, 10, 2, 2);
            ctx.stroke();
            ctx.fillRect(20.5, 10, 2, 4);                       // terminal
            ctx.fillRect(4.5, 9, 13 * Math.max(0.08, Math.min(1, fill)), 6);
            return;
        }
        if (name === "wifi") {
            ctx.lineWidth = 2.2;
            ctx.lineCap = "round";
            for (let i = 0; i < 3; i++) {
                ctx.strokeStyle = color;
                ctx.globalAlpha = 3 - i <= level ? 1 : 0.3;
                ctx.beginPath();
                ctx.path = wifiArcs[i];
                ctx.stroke();
            }
            ctx.globalAlpha = 1;
            ctx.fillStyle = color;
            ctx.beginPath();
            ctx.arc(12, 19.5, 1.4, 0, Math.PI * 2);
            ctx.fill();
            slash(ctx);
            return;
        }
        if (!paths[name])
            return;
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
        slash(ctx);
    }
}
