// SPDX-License-Identifier: GPL-3.0-or-later
// A controller button, drawn for OwneetOS (no third-party logos or images). The Xbox-style and
// Nintendo-style sets show letters, the PlayStation-style set shows the four shapes; Theme.glyphs
// picks the set. On Nintendo pads SDL follows the printed labels (sdl2-compat's default
// SDL_GAMECONTROLLER_USE_BUTTON_LABELS), so "a" is the button labelled A there too.
// button: a, b, x, y, lb, rb, lt, rt, menu, guide.
import QtQuick 2.15

Item {
    id: root
    property string button: "a"
    property color color: Theme.fg
    readonly property string set: Theme.glyphs
    readonly property real unit: Theme.fs(26) / 26     // the glyph is drawn on a 26 x 26 grid

    readonly property var pills: ({
        xbox: { lb: ["LB", 34], rb: ["RB", 34], lt: ["LT", 32], rt: ["RT", 32] },
        ps: { lb: ["L1", 30], rb: ["R1", 30], lt: ["L2", 30], rt: ["R2", 30], menu: ["OPTIONS", 62] },
        nintendo: { lb: ["L", 28], rb: ["R", 28], lt: ["ZL", 32], rt: ["ZR", 32] }
    })
    readonly property var pill: pills[set][button] || null
    // Lines and shapes inside the circle (SVG path syntax, 26 x 26 grid)
    readonly property var paths: ({
        "ps.a": "M9.2 9.2l7.6 7.6M16.8 9.2l-7.6 7.6",
        "ps.x": "M9.8 8.8h6.4a1 1 0 0 1 1 1v6.4a1 1 0 0 1-1 1h-6.4a1 1 0 0 1-1-1v-6.4a1 1 0 0 1 1-1z",
        "ps.y": "M13 8.4l4.8 8.2H8.2z",
        "xbox.menu": "M8.5 10h9M8.5 13h9M8.5 16h9",
        "nintendo.menu": "M13 8.5v9M8.5 13h9",
        "guide": "M8 13.2 13 8.8l5 4.4M9.6 12v5.4h6.8V12"
    })
    readonly property string path: paths[button === "guide" ? "guide" : set + "." + button] || ""
    readonly property string letter: set !== "ps" && ["a", "b", "x", "y"].indexOf(button) >= 0
                                     ? button.toUpperCase() : ""

    implicitWidth: (pill ? pill[1] : 26) * unit
    implicitHeight: 26 * unit

    // Frame: a circle, or a rounded pill for shoulder buttons and labels
    Rectangle {
        x: unit; y: unit
        width: parent.width - 2 * unit; height: parent.height - 2 * unit
        radius: pill ? 7 * unit : width / 2
        color: "transparent"
        border.color: root.color
        border.width: 1.8 * unit
        antialiasing: true
    }
    Text {
        visible: text !== ""
        anchors.centerIn: parent
        anchors.verticalCenterOffset: 0.3 * unit
        text: pill ? pill[0] : letter
        color: root.color
        font.family: Theme.textFont
        font.weight: Font.DemiBold
        font.pixelSize: (pill ? 11.5 : 13) * unit
    }
    // PlayStation circle button: a ring
    Rectangle {
        visible: set === "ps" && button === "b"
        anchors.centerIn: parent
        width: 9.2 * unit; height: width; radius: width / 2
        color: "transparent"
        border.color: root.color
        border.width: 2 * unit
        antialiasing: true
    }
    Canvas {
        id: canvas
        visible: path !== ""
        anchors.fill: parent
        property string strokeColor: root.color
        onStrokeColorChanged: requestPaint()
        onWidthChanged: requestPaint()
        Connections {
            target: root
            function onPathChanged() { canvas.requestPaint(); }
        }
        onPaint: {
            const ctx = getContext("2d");
            ctx.reset();
            if (root.path === "")
                return;
            ctx.scale(width / 26, height / 26);
            ctx.strokeStyle = strokeColor;
            ctx.lineWidth = 2;
            ctx.lineCap = "round";
            ctx.lineJoin = "round";
            ctx.path = root.path;
            ctx.stroke();
        }
    }
}
