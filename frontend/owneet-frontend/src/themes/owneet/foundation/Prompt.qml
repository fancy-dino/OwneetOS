// SPDX-License-Identifier: GPL-3.0-or-later
// One entry of the prompt bar: one or more button glyphs and what they do.
import QtQuick 2.15

Row {
    property var buttons: ["a"]
    property string label: ""
    spacing: Theme.px(8)

    Repeater {
        model: parent.buttons
        Glyph { button: modelData; anchors.verticalCenter: parent.verticalCenter }
    }
    Text {
        anchors.verticalCenter: parent.verticalCenter
        leftPadding: Theme.px(1)
        text: label
        color: Theme.muted
        font.family: Theme.textFont
        font.pixelSize: Theme.fs(14.5)
    }
}
