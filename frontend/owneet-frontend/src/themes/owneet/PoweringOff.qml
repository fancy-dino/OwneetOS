// SPDX-License-Identifier: GPL-3.0-or-later
// "Shutting down" at once ([[boot-experience]], approved demo design/demos/7.6b-shutting-down.html,
// owner 2026-11-09): when Shut down or Restart is chosen, the console shows at once the boot splash's
// own picture (black, the logo in the splash's grey, the spinner at 92% of the screen's height and
// "Shutting down" under it) while owneetd closes Steam and the games; then the system's splash takes
// the screen with the same picture. It stays until the session ends: no time of its own
// ([[waiting-for-events]]). Every button and the mouse stop here.
import QtQuick 2.15
import QtQuick.Window 2.15
import "foundation"

Item {
    id: root
    // the whole screen, also outside the display area the theme is drawn in (as the splash)
    x: -Math.round(Theme.areaX * Window.width)
    y: -Math.round(Theme.areaY * Window.height)
    width: Window.width
    height: Window.height
    visible: false
    z: 400

    function show() {
        Nav.inputPaused = true;
        visible = true;
    }
    // the power-off was refused: the console goes on
    function hide() {
        visible = false;
        Nav.inputPaused = false;
    }

    MouseArea {
        anchors.fill: parent
        hoverEnabled: true
        acceptedButtons: Qt.AllButtons
        onWheel: wheel.accepted = true
    }
    Rectangle {
        anchors.fill: parent
        color: Theme.splashBg
    }
    // the splash's picture: 800 px wide in the middle of the screen, unscaled (Plymouth's watermark)
    Item {
        width: LogoData.splashWidth
        height: LogoData.splashHeight
        x: Math.round((root.width - width) / 2)
        y: Math.round((root.height - height) / 2)
        Logo {
            layout: "above"
            size: LogoData.splashWidth
            letters: Theme.splashLogo
            os: Theme.splashLogo
            symbol: Theme.splashLogo
        }
    }
    // the splash's spinner (owneet-branding: a white arc over a faint ring, 48 px), centred at 92%
    Canvas {
        id: spinner
        width: 48; height: 48
        x: Math.round((root.width - width) / 2)
        y: Math.round(root.height * 0.92 - height / 2)
        property color ink: Theme.splashInk
        onInkChanged: requestPaint()
        onPaint: {
            const ctx = getContext("2d");
            ctx.reset();
            ctx.lineWidth = 4;
            ctx.strokeStyle = Qt.rgba(ink.r, ink.g, ink.b, 0.18);
            ctx.beginPath();
            ctx.arc(24, 24, 18, 0, 2 * Math.PI);
            ctx.stroke();
            ctx.strokeStyle = ink;
            ctx.lineCap = "round";
            ctx.beginPath();
            ctx.arc(24, 24, 18, -Math.PI / 2, 0);
            ctx.stroke();
        }
        RotationAnimation on rotation {
            running: root.visible
            from: 0; to: 360; duration: 1000
            loops: Animation.Infinite
        }
    }
    // the message under the spinner, in the splash's font
    Text {
        anchors.horizontalCenter: parent.horizontalCenter
        y: spinner.y + spinner.height + 8
        text: Tr.tr("splash.shutting_down")
        color: Theme.splashInk
        font.family: "Adwaita Sans"
        font.pixelSize: 16
    }
}
