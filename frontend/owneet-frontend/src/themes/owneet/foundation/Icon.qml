// SPDX-License-Identifier: GPL-3.0-or-later
// A small icon drawn for OwneetOS from an SVG path on a 24 x 24 grid (no icon fonts or images).
// Named icons: "heart", "download", "controller", "wifi" (with `level` 0-3: arcs above it are
// dimmed), "ethernet", "lock", "battery" (filled by `fill`, 0-1), the settings sections and the
// on-screen keyboard's keys. `crossed` draws a slash over the icon
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
        ethernet: "M5 9h14v8H5zM9 17v3M15 17v3M8 9V6h8v3",
        lock: "M7 11h10a2 2 0 0 1 2 2v5a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2v-5a2 2 0 0 1 2-2zM8 11V8a4 4 0 0 1 8 0v3",
        // Notifications
        bell: "M6 16V11a6 6 0 0 1 12 0v5l2 2H4zM10 20a2 2 0 0 0 4 0",
        alert: "M5 3h14a1 1 0 0 1 1 1v16a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1zM12 8v5M12 16.5v.5",
        // On-screen keyboard
        backspace: "M8 5h12a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H8l-6-7zM11 9l6 6M17 9l-6 6",
        shift: "M12 4 4 12h4v7h8v-7h4z",
        caps: "M12 3 4 11h4v5h8v-5h4zM8 20h8",
        left: "M15 6l-6 6 6 6",
        right: "M9 6l6 6-6 6",
        eye: "M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12zM12 9a3 3 0 1 0 0 6a3 3 0 1 0 0-6",
        // Settings sections
        sound: "M4 9h4l5-4v14l-5-4H4zM17 9a4 4 0 0 1 0 6",
        display: "M3 5h18v11H3zM8 20h8M12 16v4",
        palette: "M12 3a9 9 0 1 0 0 18a9 9 0 1 0 0-18M12 3v18",
        globe: "M12 3a9 9 0 1 0 0 18a9 9 0 1 0 0-18M3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18",
        disks: "M5 5h14a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2zM5 13h14a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-2a2 2 0 0 1 2-2z",
        gear: "M12 9a3 3 0 1 0 0 6a3 3 0 1 0 0-6M12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9l2.1 2.1M17 17l2.1 2.1M4.9 19.1 7 17M17 7l2.1-2.1",
        // Quick menu and power options
        power: "M12 3v8M6.3 6.8a8 8 0 1 0 11.4 0",
        suspend: "M20 14.5A8 8 0 1 1 9.5 4a6.5 6.5 0 0 0 10.5 10.5z",
        restart: "M4 12a8 8 0 1 0 2.4-5.7M4 4v4.5h4.5",
        cancel: "M6 6l12 12M18 6 6 18",
        // Add games (4.2): a store (no third-party logo) and a USB stick
        plus: "M12 5v14M5 12h14",
        store: "M4 9h16l-1.2 10a1 1 0 0 1-1 .9H6.2a1 1 0 0 1-1-.9zM8.5 9V7a3.5 3.5 0 0 1 7 0v2",
        usb: "M9 3h6v6H9zM7 9h10v8a4 4 0 0 1-4 4h-2a4 4 0 0 1-4-4zM11 5.5v1M13 5.5v1",
        // Steam games (4.2c): on a disk that is not connected, the notice before Steam's window,
        // uninstalling, ready
        unplug: "M9 8V4M15 8V4M7 8h10v3a5 5 0 0 1-10 0zM12 16v4M4 4l16 16",
        warning: "M12 4 2.8 19.5h18.4zM12 10v4.5M12 17v.5",
        trash: "M4 7h16M9 7V4.5h6V7M6.5 7l1 13h9l1-13",
        check: "M5 12.5l4.5 4.5L19 7",
        info: "M21 12a9 9 0 1 1-18 0a9 9 0 1 1 18 0zM12 11v6M12 7.5v.5",
        pointer: "M6 3v15l4.2-4 3 7 2.8-1.2-3-6.8H18z"
    })
    // Wi-Fi: three arcs and a dot, from the largest arc down
    readonly property var wifiArcs: ["M2 9a15 15 0 0 1 20 0", "M5 12.5a10 10 0 0 1 14 0", "M8.5 16a5 5 0 0 1 7 0"]
    onLevelChanged: requestPaint()
    implicitWidth: Theme.px(16)
    implicitHeight: implicitWidth
    onColorChanged: requestPaint()
    onNameChanged: requestPaint()
    onVisibleChanged: if (visible) requestPaint()   // e.g. the network icon, shown once owneetd answers
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
