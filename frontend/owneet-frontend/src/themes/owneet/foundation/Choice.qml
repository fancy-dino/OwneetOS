// SPDX-License-Identifier: GPL-3.0-or-later
// One option of a segmented choice (e.g. text size S M L XL). The chosen one is filled.
import QtQuick 2.15

Rectangle {
    id: root
    property string text: ""
    property bool chosen: false
    signal activated()

    implicitHeight: label.implicitHeight + Theme.px(16)
    radius: Theme.radiusS
    color: chosen ? Theme.fg : Theme.surface
    border.color: chosen ? Theme.fg : Theme.line
    border.width: Math.max(1, Theme.px(1))

    Text {
        id: label
        anchors.centerIn: parent
        text: root.text
        color: root.chosen ? Theme.bg : Theme.muted
        font.family: Theme.textFont
        font.weight: root.chosen ? Font.Medium : Font.Normal
        font.pixelSize: Theme.fs(14.5)
    }
    FocusFrame {}
}
