// SPDX-License-Identifier: GPL-3.0-or-later
// "Hold Home to return to the console", for a few seconds each time Steam's window comes on screen
// (owner, 2026-10-15): Steam may stay on a page that never finishes loading, and nothing in Steam
// says how to leave. A window of its own, marked as gamescope's external overlay: gamescope draws
// it over Steam's window (owneetd never takes it for an app). Gamescope session only.
import QtQuick 2.15
import QtQuick.Window 2.15
import "foundation"

Window {
    id: root
    flags: Qt.FramelessWindowHint | Qt.WindowStaysOnTopHint | Qt.WindowTransparentForInput | Qt.WindowDoesNotAcceptFocus
    color: "transparent"
    // only the bar, at the bottom in the middle: should transparency fail, it covers no more
    width: bar.width
    height: bar.height
    x: Math.round((Screen.width - width) / 2)
    y: Screen.height - height - Theme.safeBottom - Theme.px(8)
    visible: false
    Component.onCompleted: systemInfo.markOverlay(root)

    function flash() {
        visible = true;
        hideTimer.restart();
    }
    function hide() {
        hideTimer.stop();
        visible = false;
    }
    Timer { id: hideTimer; interval: 6000; onTriggered: root.visible = false }

    Rectangle {
        id: bar
        width: hintRow.width + Theme.px(36)
        height: hintRow.height + Theme.px(20)
        radius: height / 2
        color: Theme.surface
        border.color: Theme.line
        border.width: Math.max(1, Theme.px(1))
        Row {
            id: hintRow
            anchors.centerIn: parent
            spacing: Theme.px(10)
            Glyph { button: "guide"; anchors.verticalCenter: parent.verticalCenter }
            Text {
                anchors.verticalCenter: parent.verticalCenter
                text: Tr.tr("steam.hint")
                color: Theme.fg
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(17)
            }
        }
    }
}
