// SPDX-License-Identifier: GPL-3.0-or-later
// The start-up animation ([[step-3.13-start-up-animation-and-power-options]], approved demo
// design/demos/3.13-startup-power.html), once per boot with the start-up sound: it starts from the
// boot splash's own picture (the wordmark centred, at 46 % of the height, as Plymouth shows it,
// white on black for every palette since 2026-10-15), the letters of "Owneet" ripple, "OS" pops
// and takes the palette's accent with its glow, then the wordmark flies to its place in the top
// bar, taking the palette's text colour, while the black fades into the pages rising under it.
// "Reduce motion": a plain fade.
import QtQuick 2.15
import "foundation"

Item {
    id: root
    anchors.fill: parent
    z: 300
    visible: false

    property Item target: null          // the top bar's wordmark
    property Item pages: null           // they rise under the wordmark
    readonly property bool running: visible
    signal ripple()
    signal finished()

    // The splash's wordmark (packages/owneet-branding): rendered 420 pixels wide from
    // owneetos-wordmark.svg (viewBox 3521.8 x 844.5, baseline 752.3 from the top) and shown
    // unscaled by Plymouth, centred, its top at 46 % of the height left free
    readonly property real splashWidth: 420
    // the wordmark's colours: the splash's white, then the palette's (set by the animation)
    property color inkLetters: Theme.splashFg
    property color inkOS: Theme.splashFg
    // Shown at once, still (the splash's picture), until go(): the start-up sound plays then
    function show() {
        place();
        mark.opacity = 1;
        backdrop.opacity = 1;
        inkLetters = Theme.splashFg;
        inkOS = Theme.splashFg;
        if (pages) { pages.opacity = 0; pageShift.y = Theme.px(14); }
        visible = true;
    }
    // where the splash shows the wordmark (also when the size arrives after show())
    function place() {
        const s = splashWidth / mark.width;
        const top = (height - splashWidth * 844.5 / 3521.8) * 0.46;
        mark.scale = s;
        mark.x = (width - splashWidth) / 2;
        mark.y = top + splashWidth * 752.3 / 3521.8 - os.baselineOffset * s;
    }
    readonly property bool still: visible && !flight.running && !fade.running
    onWidthChanged: if (still) place()
    onHeightChanged: if (still) place()
    function go() {
        if (!visible || flight.running || fade.running)
            return;
        if (Theme.reduceMotion) {
            fade.start();
            return;
        }
        // where it lands: on the top bar's wordmark, baselines aligned (set before the start:
        // a running animation keeps the values it started with)
        const p = target ? target.mapToItem(root, 0, 0) : Qt.point(0, 0);
        toScale.to = target ? target.width / mark.width : 0.4;
        toX.to = p.x;
        toY.to = p.y + (target ? target.baselineOffset : 0) - os.baselineOffset * toScale.to;
        ripple();
        flight.start();
    }
    function done() {
        visible = false;
        if (pages) { pages.opacity = 1; pageShift.y = 0; }
        finished();
    }
    property Translate pageShift: Translate { y: 0 }
    onPagesChanged: if (pages) pages.transform = [pageShift]

    Rectangle {
        id: backdrop
        anchors.fill: parent
        color: Theme.splashBg
    }
    // A soft coral glow behind "OS", drawn once
    Canvas {
        id: glow
        width: root.splashWidth * 1.5; height: width
        x: mark.x + (os.x + os.width / 2) * mark.scale - width / 2
        y: mark.y + (os.y + os.height / 2) * mark.scale - height / 2
        opacity: 0
        readonly property color tint: Theme.accent
        onTintChanged: requestPaint()
        onPaint: {
            const ctx = getContext("2d");
            ctx.reset();
            const g = ctx.createRadialGradient(width / 2, height / 2, 0, width / 2, height / 2, width / 2);
            g.addColorStop(0, Qt.rgba(tint.r, tint.g, tint.b, 0.55));
            g.addColorStop(1, Qt.rgba(tint.r, tint.g, tint.b, 0));
            ctx.fillStyle = g;
            ctx.fillRect(0, 0, width, height);
        }
    }

    Item {
        id: mark
        transformOrigin: Item.TopLeft
        readonly property real size: 108    // about the splash's size: sharp there, scaled down in flight
        width: letters.width + os.width
        height: os.height
        onWidthChanged: if (root.still) root.place()     // the letters are laid out after show()
        Row {
            id: letters
            spacing: -mark.size * 0.03
            Repeater {
                model: ["O", "w", "n", "e", "e", "t"]
                Text {
                    id: letter
                    property real lift: 0
                    transform: Translate { y: -letter.lift }
                    text: modelData
                    color: root.inkLetters
                    font.family: Theme.displayFont
                    font.weight: Font.ExtraBold
                    font.pixelSize: mark.size
                    SequentialAnimation {
                        id: wave
                        PauseAnimation { duration: 80 + index * 55 }
                        NumberAnimation { target: letter; property: "lift"; to: mark.size * 0.16; duration: 210; easing.type: Easing.InOutSine }
                        NumberAnimation { target: letter; property: "lift"; to: 0; duration: 210; easing.type: Easing.InOutSine }
                    }
                    Connections {
                        target: root
                        function onRipple() { wave.restart(); }
                    }
                }
            }
        }
        Text {
            id: os
            x: letters.width - mark.size * 0.03
            text: "OS"
            color: root.inkOS
            font.family: Theme.displayFont
            font.weight: Font.ExtraBold
            font.pixelSize: mark.size
        }
    }

    // "OS" lights up, then the wordmark flies to the top bar while the pages come up
    SequentialAnimation {
        id: flight
        PauseAnimation { duration: 480 }
        ParallelAnimation {
            SequentialAnimation {
                ParallelAnimation {
                    NumberAnimation { target: os; property: "scale"; to: 1.16; duration: 200; easing.type: Easing.OutCubic }
                    ColorAnimation { target: root; property: "inkOS"; to: Theme.accent; duration: 200 }
                }
                NumberAnimation { target: os; property: "scale"; to: 1; duration: 320; easing.type: Easing.OutBack }
            }
            SequentialAnimation {
                NumberAnimation { target: glow; property: "opacity"; to: 1; duration: 300 }
                NumberAnimation { target: glow; property: "opacity"; to: 0; duration: 600 }
            }
        }
        ParallelAnimation {
            NumberAnimation { id: toX; target: mark; property: "x"; duration: 750; easing.type: Easing.InOutCubic }
            NumberAnimation { id: toY; target: mark; property: "y"; duration: 750; easing.type: Easing.InOutCubic }
            NumberAnimation { id: toScale; target: mark; property: "scale"; duration: 750; easing.type: Easing.InOutCubic }
            ColorAnimation { target: root; property: "inkLetters"; to: Theme.fg; duration: 750 }
            SequentialAnimation {
                PauseAnimation { duration: 200 }
                NumberAnimation { target: backdrop; property: "opacity"; to: 0; duration: 600 }
            }
            SequentialAnimation {
                PauseAnimation { duration: 150 }
                ParallelAnimation {
                    NumberAnimation { target: root.pages; property: "opacity"; to: 1; duration: 600 }
                    NumberAnimation { target: root.pageShift; property: "y"; to: 0; duration: 600; easing.type: Easing.OutCubic }
                }
            }
        }
        ScriptAction { script: root.done() }
    }
    SequentialAnimation {
        id: fade
        ParallelAnimation {
            NumberAnimation { target: backdrop; property: "opacity"; to: 0; duration: 600 }
            NumberAnimation { target: mark; property: "opacity"; to: 0; duration: 600 }
            NumberAnimation { target: root.pages; property: "opacity"; to: 1; duration: 600 }
        }
        ScriptAction { script: root.done() }
    }
}
