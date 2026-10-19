// SPDX-License-Identifier: GPL-3.0-or-later
// A QR code of a text (e.g. the address of a game's licence agreement, to read on a phone; 4.2c),
// drawn from owneet::SystemInfo's modules. Dark modules on a light ground, as phones read them:
// the two come from the palette (its text and background colours, the lighter as the ground),
// with the quiet zone of four modules around.
import QtQuick 2.15

Canvas {
    id: root
    property string text: ""
    readonly property var rows: text !== "" ? systemInfo.qrCode(text) : []
    readonly property bool lightText: Theme.fg.hslLightness > Theme.bg.hslLightness
    readonly property color ground: lightText ? Theme.fg : Theme.bg
    readonly property color ink: lightText ? Theme.bg : Theme.fg
    visible: rows.length > 0
    implicitWidth: Theme.px(180)
    implicitHeight: implicitWidth
    onRowsChanged: requestPaint()
    onGroundChanged: requestPaint()
    onWidthChanged: requestPaint()
    onPaint: {
        const ctx = getContext("2d");
        ctx.reset();
        const n = rows.length;
        if (n === 0)
            return;
        const cell = Math.floor(Math.min(width, height) / (n + 8));
        const size = cell * (n + 8);
        const x0 = Math.floor((width - size) / 2), y0 = Math.floor((height - size) / 2);
        ctx.fillStyle = ground;
        ctx.fillRect(x0, y0, size, size);
        ctx.fillStyle = ink;
        for (let y = 0; y < n; y++)
            for (let x = 0; x < n; x++)
                if (rows[y][x] === "1")
                    ctx.fillRect(x0 + (x + 4) * cell, y0 + (y + 4) * cell, cell, cell);
    }
}
