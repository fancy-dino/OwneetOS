// SPDX-License-Identifier: GPL-3.0-or-later
// The language picker: every installed language by its own name, the current one ticked.
// A switches the whole interface to the selected language, B goes back.
import QtQuick 2.15

Sheet {
    id: root
    title: Tr.tr("language.title")
    prompts: [{ buttons: ["a"], label: Tr.tr("prompt.select") }, { buttons: ["b"], label: Tr.tr("prompt.back") }]
    sideMargin: 360
    topMargin: 130
    bottomMargin: 130

    property var rows: ({})

    function openPicker() {
        open(true);
        focusItem(rows[Tr.language]);
    }
    onAccepted: {
        Tr.setLanguage(item.tag);
        close(true);
    }

    Column {
        width: parent.width
        spacing: Theme.px(8)
        Repeater {
            model: Tr.languages
            Rectangle {
                id: row
                readonly property string tag: modelData.tag
                readonly property bool navigable: true
                width: parent.width
                height: name.implicitHeight + Theme.px(22)
                radius: Theme.radiusS + Theme.px(2)
                color: Theme.surface
                border.color: Theme.line
                border.width: Math.max(1, Theme.px(1))
                Component.onCompleted: root.rows[modelData.tag] = row
                Text {
                    id: name
                    anchors { left: parent.left; leftMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                    text: modelData.name
                    color: Theme.fg
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(17)
                }
                Text {
                    visible: modelData.tag === Tr.language
                    anchors { right: parent.right; rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                    text: "✓"
                    color: Theme.accent
                    font.family: Theme.textFont
                    font.weight: Font.DemiBold
                    font.pixelSize: Theme.fs(17)
                }
                FocusFrame {}
            }
        }
    }
}
