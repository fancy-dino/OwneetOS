// SPDX-License-Identifier: GPL-3.0-or-later
// A settings row that only shows something (not selectable): an optional icon, a label, an
// optional second line, and a value on the right.
import QtQuick 2.15

Rectangle {
    id: root
    property string text: ""
    property string detail: ""
    property string value: ""
    property string icon: ""           // Icon name, e.g. "wifi"
    property bool crossed: false       // the icon crossed out (e.g. offline)
    default property alias extra: more.data
    implicitHeight: column.implicitHeight + Theme.px(24)
    radius: Theme.radiusS + Theme.px(2)
    color: "transparent"
    border.color: Theme.line
    border.width: Math.max(1, Theme.px(1))
    Column {
        id: column
        x: Theme.px(16)
        anchors.verticalCenter: parent.verticalCenter
        width: parent.width - Theme.px(32)
        spacing: Theme.px(4)
        Item {
            width: parent.width
            height: Math.max(label.implicitHeight, valueText.implicitHeight)
            Icon {
                id: icon
                visible: root.icon !== ""
                anchors.verticalCenter: label.verticalCenter
                width: visible ? Theme.fs(20) : 0; height: width
                name: root.icon
                crossed: root.crossed
                color: Theme.fg
            }
            Text {
                id: label
                x: icon.visible ? icon.width + Theme.px(10) : 0
                width: parent.width - x - valueText.implicitWidth - Theme.px(16)
                text: root.text
                color: Theme.fg
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(14.5)
                elide: Text.ElideRight
            }
            Text {
                id: valueText
                anchors.right: parent.right
                text: root.value
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(14.5)
            }
        }
        Text {
            visible: root.detail !== ""
            width: parent.width
            text: root.detail
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(12.5)
            wrapMode: Text.Wrap
        }
        Item {
            id: more
            width: parent.width
            height: childrenRect.height
            visible: children.length > 0
        }
    }
}
