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

    property int index: 0
    property var items: []

    function openPicker() {
        const langs = Tr.languages;
        index = Math.max(0, langs.findIndex(l => l.tag === Tr.language));
        open();
        focusCurrent();
    }
    function focusCurrent() {
        const item = items[index];
        if (item) {
            item.forceActiveFocus();
            ensureVisible(item);
        }
    }

    Keys.onPressed: {
        if (event.isAutoRepeat && (api.keys.isAccept(event) || api.keys.isCancel(event)))
            return;
        if (api.keys.isAccept(event)) { Tr.setLanguage(Tr.languages[index].tag); close(); }
        else if (api.keys.isCancel(event)) close();
        else if (event.key === Qt.Key_Up && index > 0) { index--; focusCurrent(); }
        else if (event.key === Qt.Key_Down && index < Tr.languages.length - 1) { index++; focusCurrent(); }
        else return;
        event.accepted = true;
    }

    Column {
        width: parent.width
        spacing: Theme.px(8)
        Repeater {
            model: Tr.languages
            Rectangle {
                id: row
                width: parent.width
                height: name.implicitHeight + Theme.px(22)
                radius: Theme.radiusS + Theme.px(2)
                color: Theme.surface
                border.color: Theme.line
                border.width: Math.max(1, Theme.px(1))
                Component.onCompleted: { const list = root.items; list[index] = row; root.items = list; }
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
