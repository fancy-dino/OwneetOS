// SPDX-License-Identifier: GPL-3.0-or-later
// Settings → Display → "Display area" ([[screens-and-display]], owner 2026-11-12; approved demo
// design/demos/3.9c-display-area.html): for a TV that cuts the edges of the picture. On the whole
// screen, an arrow in each corner of the area; the D-pad makes the area narrower or wider
// (left / right) and shorter or taller (up / down), held it repeats; X goes back to the whole
// screen, A confirms, B cancels. The console's screens are then drawn inside the area
// (Theme.areaX / areaY: the part of each side left out; the frontend's main.qml places the theme).
import QtQuick 2.15
import QtQuick.Window 2.15
import "foundation"

FocusScope {
    id: root
    // The whole screen, also the part outside the display area the theme is drawn in
    x: -Math.round(Theme.areaX * Window.width)
    y: -Math.round(Theme.areaY * Window.height)
    width: Window.width
    height: Window.height
    visible: false
    z: 102

    property var toast: null           // the theme's, which stays after the window closes
    property real ax: 0                 // the area being chosen: the part of each side left out
    property real ay: 0
    readonly property real step: 0.0025 // a quarter of a percent of the screen, each side
    readonly property real most: 0.1    // at most a tenth of the screen on each side

    function open() {
        ax = Theme.areaX;
        ay = Theme.areaY;
        Nav.feedback("sheet-open");
        Nav.windowOpened();             // A just after it opens is ignored
        visible = true;
        forceActiveFocus();
    }
    function close(save) {
        if (!visible)
            return;
        visible = false;
        if (save) {
            Nav.feedback("confirm");
            Theme.areaX = ax;
            Theme.areaY = ay;
            if (toast) toast.show(Tr.tr("area.saved"));
        } else {
            Nav.feedback("sheet-close");
            if (toast) toast.show(Tr.tr("area.unchanged"));
        }
    }
    function change(horizontal, by) {
        const old = horizontal ? ax : ay;
        const v = Math.min(most, Math.max(0, Math.round((old + by * step) * 10000) / 10000));
        if (v === old) {
            Nav.feedback("edge");
            return;
        }
        if (horizontal) ax = v; else ay = v;
        Nav.feedback("move");
    }
    // 96%, 99,5 %: the part of the screen used, in the language's numbers
    function percent(side) {
        const v = Math.round((1 - 2 * side) * 1000) / 10;
        return Tr.tr("area.percent", { n: Number(v).toLocaleString(Qt.locale(), "f", v % 1 === 0 ? 0 : 1) });
    }

    Keys.onPressed: {
        const a = Nav.action(event);
        if (a === "left" || a === "right")
            change(true, a === "left" ? 1 : -1);
        else if (a === "up" || a === "down")
            change(false, a === "up" ? -1 : 1);
        else if (a === "secondary") {
            ax = 0;
            ay = 0;
            Nav.feedback("move");
        } else if (a === "accept")
            close(true);
        else if (a === "back")
            close(false);
        else
            return;
        event.accepted = true;
    }

    Rectangle {
        anchors.fill: parent
        color: Theme.bg
    }
    MouseArea {                         // nothing below it is reached
        anchors.fill: parent
        acceptedButtons: Qt.AllButtons
        onWheel: wheel.accepted = true
    }

    // The area: its edge, the four arrows (their tips are its corners), and the prompts
    Item {
        id: area
        x: Math.round(root.ax * root.width)
        y: Math.round(root.ay * root.height)
        width: root.width - 2 * x
        height: root.height - 2 * y
        Rectangle {
            anchors.fill: parent
            color: "transparent"
            border.color: Theme.fg
            border.width: Theme.px(3)
            radius: Theme.px(2)
        }
        Repeater {
            model: [[0, 0], [1, 0], [0, 1], [1, 1]]
            Canvas {
                readonly property bool atRight: modelData[0] === 1
                readonly property bool atBottom: modelData[1] === 1
                width: Theme.px(74); height: width
                x: atRight ? area.width - width : 0
                y: atBottom ? area.height - height : 0
                property color fill: Theme.accent
                onFillChanged: requestPaint()
                onWidthChanged: requestPaint()
                onPaint: {
                    const ctx = getContext("2d");
                    ctx.reset();
                    ctx.translate(atRight ? width : 0, atBottom ? height : 0);
                    ctx.scale((atRight ? -1 : 1) * width / 74, (atBottom ? -1 : 1) * height / 74);
                    ctx.fillStyle = fill;
                    ctx.path = "M0 0L46 0L34 12L64 42L42 64L12 34L0 46Z";
                    ctx.fill();
                }
            }
        }
        Row {
            anchors { horizontalCenter: parent.horizontalCenter; bottom: parent.bottom; bottomMargin: Theme.px(34) }
            spacing: Theme.px(30)
            Row {
                spacing: Theme.px(26)
                Prompt { anchors.verticalCenter: parent.verticalCenter; buttons: ["lr"]; label: Tr.tr("area.width") }
                Prompt { anchors.verticalCenter: parent.verticalCenter; buttons: ["ud"]; label: Tr.tr("area.height") }
                Prompt { anchors.verticalCenter: parent.verticalCenter; buttons: ["x"]; label: Tr.tr("area.full") }
            }
            Row {
                spacing: Theme.px(26)
                Prompt { anchors.verticalCenter: parent.verticalCenter; buttons: ["a"]; label: Tr.tr("area.confirm") }
                Prompt { anchors.verticalCenter: parent.verticalCenter; buttons: ["b"]; label: Tr.tr("prompt.cancel") }
            }
        }
    }

    // In the middle of the screen: what to do, and the area's size
    Column {
        anchors.centerIn: parent
        width: Math.min(parent.width - 2 * Theme.px(200), Theme.px(700))
        spacing: Theme.px(14)
        Text {
            width: parent.width
            horizontalAlignment: Text.AlignHCenter
            text: Tr.tr("area.title")
            color: Theme.fg
            font.family: Theme.displayFont
            font.weight: Font.Bold
            font.pixelSize: Theme.fs(40)
        }
        Text {
            width: parent.width
            horizontalAlignment: Text.AlignHCenter
            text: Tr.tr("area.text")
            color: Theme.fg
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(17)
            wrapMode: Text.Wrap
        }
        Row {
            anchors.horizontalCenter: parent.horizontalCenter
            spacing: Theme.px(28)
            Repeater {
                model: [["area.width", root.ax], ["area.height", root.ay]]
                Row {
                    spacing: Theme.fs(5)
                    Text {
                        text: Tr.tr(modelData[0])
                        color: Theme.muted
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                    }
                    Text {
                        text: root.percent(modelData[1])
                        color: Theme.fg
                        font.family: Theme.textFont
                        font.weight: Font.DemiBold
                        font.pixelSize: Theme.fs(14.5)
                    }
                }
            }
        }
    }
}
