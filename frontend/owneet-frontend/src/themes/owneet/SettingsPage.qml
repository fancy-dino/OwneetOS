// SPDX-License-Identifier: GPL-3.0-or-later
// Settings section: for now only Appearance (palette, text size, button prompts, reduce motion,
// TV safe area, language). The full settings, with their own demo, come in step 3.9.
import QtQuick 2.15
import "foundation"

NavArea {
    id: page
    signal openAppearance()
    readonly property var prompts: [{ buttons: ["a"], label: Tr.tr("prompt.open") }]
    function focusStart() { focusItem(appearanceRow); }
    onActiveFocusChanged: if (activeFocus && !current) focusStart()

    Text {
        id: heading
        text: Tr.tr("tab.settings")
        color: Theme.fg
        font.family: Theme.displayFont
        font.weight: Font.Bold
        font.pixelSize: Theme.fs(40)
    }
    Column {
        anchors { top: heading.bottom; topMargin: Theme.px(20) }
        width: Math.min(parent.width, Theme.px(600))
        spacing: Theme.px(14)
        SettingRow {
            id: appearanceRow
            width: parent.width
            kind: "picker"
            text: Tr.tr("appearance.title")
            value: Tr.tr("palette." + Theme.paletteKey)
            onActivated: page.openAppearance()
        }
        Text {
            width: parent.width
            text: Tr.tr("settings.later")
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(14.5)
            wrapMode: Text.Wrap
        }
    }
}
