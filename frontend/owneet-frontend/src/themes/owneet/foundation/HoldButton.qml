// SPDX-License-Identifier: GPL-3.0-or-later
// The final confirmation before a disk is erased ([[user-data]]): A held for three seconds fills a
// ring around the A glyph; let go before it is full and nothing happens (approved demo 6.4, used by
// the installer and by Settings → Disks, demo 6.7). `confirmed` comes once the ring is full.
import QtQuick 2.15

FocusScope {
    id: hold
    readonly property bool navigable: true
    readonly property bool isHold: true
    readonly property real radius: width / 2     // the focus ring follows the circle
    property real progress: 0
    property bool holding: false
    property int duration: 3000
    signal confirmed()
    width: Theme.px(132); height: width
    function cancel() {
        if (!holding && progress === 0)
            return;
        holding = false;
        holdTimer.stop();
        unwind.start();
        Nav.feedback("edge");
    }
    // A pressed fills the ring; let go before it is full, nothing happens
    Keys.onPressed: {
        if (!api.keys.isAccept(event))
            return;
        event.accepted = true;
        if (event.isAutoRepeat || holding)
            return;
        unwind.stop();
        progress = 0;
        holding = true;
        holdTimer.start();
    }
    Keys.onReleased: {
        if (!api.keys.isAccept(event) || event.isAutoRepeat)
            return;
        event.accepted = true;
        if (holding)
            cancel();
    }
    onActiveFocusChanged: if (!activeFocus && holding) cancel()
    Timer {
        id: holdTimer
        interval: 16; repeat: true
        property real began: 0
        onRunningChanged: if (running) began = Date.now()
        onTriggered: {
            hold.progress = Math.min(1, (Date.now() - began) / hold.duration);
            if (hold.progress >= 1) {
                stop();
                hold.holding = false;
                hold.progress = 0;
                hold.confirmed();
            }
        }
    }
    NumberAnimation { id: unwind; target: hold; property: "progress"; to: 0; duration: 250; easing.type: Easing.OutCubic }
    Rectangle {
        anchors.fill: parent
        radius: width / 2
        color: Theme.surface
        border.color: Theme.line
        border.width: Math.max(1, Theme.px(1))
    }
    Glyph {
        anchors.centerIn: parent
        button: "a"
        scale: 46 / 26
    }
    Canvas {                  // the ring that fills while A is held
        id: ring
        anchors.centerIn: parent
        width: parent.width + Theme.px(12); height: width
        readonly property real value: hold.progress
        onValueChanged: requestPaint()
        onPaint: {
            const ctx = getContext("2d");
            ctx.reset();
            if (value <= 0)
                return;
            const w = Theme.px(6);
            ctx.lineWidth = w;
            ctx.lineCap = "round";
            ctx.strokeStyle = Theme.accent;
            ctx.beginPath();
            ctx.arc(width / 2, height / 2, width / 2 - w / 2, -Math.PI / 2, -Math.PI / 2 + value * 2 * Math.PI);
            ctx.stroke();
        }
    }
    FocusFrame {}
}
