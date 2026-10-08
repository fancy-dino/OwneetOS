// SPDX-License-Identifier: GPL-3.0-or-later
// Appearance options (palette, text size, button prompts, reduce motion, TV safe area, language).
// Temporary home, opened with Y: these options move to Settings → Appearance in step 3.9.
import QtQuick 2.15
import "foundation"

Sheet {
    id: root
    title: Tr.tr("appearance.title")
    prompts: [{ buttons: ["a"], label: Tr.tr("prompt.select") }, { buttons: ["b"], label: Tr.tr("prompt.back") }]
    sideMargin: 340
    topMargin: 70

    signal openPalettes()
    signal openLanguages()

    property Item last: null    // selection to come back to after a picker

    function openSheet(fromTop, quiet) {
        open(quiet);
        focusItem(fromTop || !last ? paletteRow : last);
        if (fromTop)
            flickable.contentY = 0;
    }
    onMoved: last = item

    Column {
        width: parent.width
        spacing: Theme.px(12)

        SettingRow {
            id: paletteRow
            width: parent.width
            kind: "picker"
            text: Tr.tr("appearance.palette")
            value: Tr.tr("palette." + Theme.paletteKey)
            swatches: { const c = Theme.palette(Theme.paletteKey).c; return [c[0], c[2], c[6], c[4]]; }
            onActivated: root.openPalettes()
        }
        Label { text: Tr.tr("appearance.text_size") }
        Row {
            id: sizes
            width: parent.width
            spacing: Theme.px(6)
            readonly property var items: [s0, s1, s2, s3]
            readonly property real cell: (width - spacing * 3) / 4
            Choice { id: s0; width: sizes.cell; text: "S"; chosen: Theme.textScale === 0.9; onActivated: Theme.textScale = 0.9 }
            Choice { id: s1; width: sizes.cell; text: "M"; chosen: Theme.textScale === 1.0; onActivated: Theme.textScale = 1.0 }
            Choice { id: s2; width: sizes.cell; text: "L"; chosen: Theme.textScale === 1.2; onActivated: Theme.textScale = 1.2 }
            Choice { id: s3; width: sizes.cell; text: "XL"; chosen: Theme.textScale === 1.4; onActivated: Theme.textScale = 1.4 }
        }
        Label { text: Tr.tr("appearance.button_prompts") }
        Row {
            id: glyphSets
            width: parent.width
            spacing: Theme.px(6)
            readonly property var items: [g0, g1, g2, g3]
            readonly property real cell: (width - spacing * 3) / 4
            Choice { id: g0; width: glyphSets.cell; text: Tr.tr("appearance.auto"); chosen: Theme.glyphSetting === "auto"; onActivated: Theme.glyphSetting = "auto" }
            Choice { id: g1; width: glyphSets.cell; text: "Xbox"; chosen: Theme.glyphSetting === "xbox"; onActivated: Theme.glyphSetting = "xbox" }
            Choice { id: g2; width: glyphSets.cell; text: "PlayStation"; chosen: Theme.glyphSetting === "ps"; onActivated: Theme.glyphSetting = "ps" }
            Choice { id: g3; width: glyphSets.cell; text: "Nintendo"; chosen: Theme.glyphSetting === "nintendo"; onActivated: Theme.glyphSetting = "nintendo" }
        }
        SettingRow {
            id: motionRow
            width: parent.width
            text: Tr.tr("appearance.reduce_motion")
            checked: Theme.reduceMotion
            onActivated: Theme.reduceMotion = !Theme.reduceMotion
        }
        SettingRow {
            id: safeRow
            width: parent.width
            text: Tr.tr("appearance.safe_area")
            checked: Theme.showSafeArea
            onActivated: Theme.showSafeArea = !Theme.showSafeArea
        }
        SettingRow {
            id: languageRow
            width: parent.width
            kind: "picker"
            text: Tr.tr("appearance.language")
            value: Tr.languageName(Tr.language)
            onActivated: root.openLanguages()
        }
    }
}
