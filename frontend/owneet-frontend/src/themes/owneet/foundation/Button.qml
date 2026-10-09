// SPDX-License-Identifier: GPL-3.0-or-later
// A round ("pill") button. Styles: "primary" (accent), "secondary" (raised), and the two used over
// game art, "light" (white) and "glass" (translucent white).
import QtQuick 2.15

Rectangle {
    id: root
    property string text: ""
    property string style: "secondary"
    readonly property bool navigable: true
    signal activated()

    implicitWidth: label.implicitWidth + Theme.px(52)
    implicitHeight: label.implicitHeight + Theme.px(22)
    radius: height / 2
    color: style === "primary" ? Theme.accent
         : style === "light" ? "#FFFFFF"
         : style === "glass" ? "#29FFFFFF"
         : Theme.raised

    Text {
        id: label
        anchors.centerIn: parent
        text: root.text
        color: root.style === "primary" ? Theme.onAccent
             : root.style === "light" ? "#10131C"
             : root.style === "glass" ? "#FFFFFF"
             : Theme.fg
        font.family: Theme.textFont
        font.weight: Font.Medium
        font.pixelSize: Theme.fs(17)
    }
    FocusFrame {}
}
