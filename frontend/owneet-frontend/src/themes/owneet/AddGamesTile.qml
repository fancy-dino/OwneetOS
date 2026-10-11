// SPDX-License-Identifier: GPL-3.0-or-later
// The library's last place (approved demo design/demos/4.2-library-add-games.html): a dashed
// outline with a "+", like "All games" on the home screen; A opens "Add games".
import QtQuick 2.15
import "foundation"

Item {
    id: root
    readonly property bool navigable: true
    readonly property bool addTile: true
    property real radius: Theme.radiusM
    signal activated()

    Canvas {
        id: outline
        anchors.fill: parent
        readonly property color stroke: Theme.line
        onStrokeChanged: requestPaint()
        onWidthChanged: requestPaint()
        onHeightChanged: requestPaint()
        onPaint: {
            const ctx = getContext("2d");
            ctx.reset();
            const w = Math.max(1, Theme.px(2)), r = root.radius;
            ctx.strokeStyle = stroke;
            ctx.lineWidth = w;
            ctx.setLineDash([3, 2.5]);            // in line widths
            ctx.beginPath();
            ctx.roundedRect(w / 2, w / 2, width - w, height - w, r, r);
            ctx.stroke();
        }
    }
    Column {
        anchors.centerIn: parent
        width: parent.width - Theme.px(20)
        spacing: Theme.px(12)
        Rectangle {
            anchors.horizontalCenter: parent.horizontalCenter
            width: Theme.px(46); height: width; radius: width / 2
            color: Theme.raised
            Icon {
                anchors.centerIn: parent
                width: Theme.px(22); height: width
                name: "plus"
                color: Theme.fg
            }
        }
        Text {
            width: parent.width
            horizontalAlignment: Text.AlignHCenter
            text: Tr.tr("library.add_games")
            color: Theme.fg
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(17)
            wrapMode: Text.Wrap
        }
    }
    FocusFrame { radius: root.radius }
}
