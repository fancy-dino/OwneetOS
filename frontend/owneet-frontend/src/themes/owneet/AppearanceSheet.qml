// SPDX-License-Identifier: GPL-3.0-or-later
// Appearance options (palette, text size, button prompts, reduce motion, TV safe area).
// Temporary home, opened with Y: these options move to Settings → Appearance in step 3.9.
import QtQuick 2.15
import "foundation"

Sheet {
    id: root
    title: "Appearance"
    prompts: [{ buttons: ["a"], label: "Select" }, { buttons: ["b"], label: "Back" }]
    sideMargin: 340
    topMargin: 70

    signal openPalettes()

    readonly property var grid: [[paletteRow], sizes.items, glyphSets.items, [motionRow], [safeRow]]
    property int row: 0
    property int col: 0

    function openSheet(fromTop) {
        if (fromTop) { row = 0; col = 0; flickable.contentY = 0; }
        open();
        focusCurrent();
    }
    function focusCurrent() {
        const item = grid[row][col];
        item.forceActiveFocus();
        ensureVisible(item);
    }
    function moveRow(step) {
        const next = row + step;
        if (next < 0 || next >= grid.length)
            return;
        // keep the column whose center is nearest to the current one
        const from = grid[row][col].mapToItem(root, grid[row][col].width / 2, 0).x;
        let best = 0, dist = Infinity;
        grid[next].forEach((it, i) => {
            const d = Math.abs(it.mapToItem(root, it.width / 2, 0).x - from);
            if (d < dist) { dist = d; best = i; }
        });
        row = next; col = best;
        focusCurrent();
    }

    Keys.onPressed: {
        if (event.isAutoRepeat && (api.keys.isAccept(event) || api.keys.isCancel(event)))
            return;
        if (api.keys.isAccept(event)) grid[row][col].activated();
        else if (api.keys.isCancel(event)) close();
        else if (event.key === Qt.Key_Up) moveRow(-1);
        else if (event.key === Qt.Key_Down) moveRow(1);
        else if (event.key === Qt.Key_Left && col > 0) { col--; focusCurrent(); }
        else if (event.key === Qt.Key_Right && col < grid[row].length - 1) { col++; focusCurrent(); }
        else return;
        event.accepted = true;
    }

    Column {
        width: parent.width
        spacing: Theme.px(12)

        SettingRow {
            id: paletteRow
            width: parent.width
            kind: "picker"
            text: "Color palette"
            value: Theme.palette(Theme.paletteKey).name
            swatches: { const c = Theme.palette(Theme.paletteKey).c; return [c[0], c[2], c[6], c[4]]; }
            onActivated: root.openPalettes()
        }
        Label { text: "Text size" }
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
        Label { text: "Button prompts" }
        Row {
            id: glyphSets
            width: parent.width
            spacing: Theme.px(6)
            readonly property var items: [g0, g1, g2, g3]
            readonly property real cell: (width - spacing * 3) / 4
            Choice { id: g0; width: glyphSets.cell; text: "Auto"; chosen: Theme.glyphSetting === "auto"; onActivated: Theme.glyphSetting = "auto" }
            Choice { id: g1; width: glyphSets.cell; text: "Xbox"; chosen: Theme.glyphSetting === "xbox"; onActivated: Theme.glyphSetting = "xbox" }
            Choice { id: g2; width: glyphSets.cell; text: "PlayStation"; chosen: Theme.glyphSetting === "ps"; onActivated: Theme.glyphSetting = "ps" }
            Choice { id: g3; width: glyphSets.cell; text: "Nintendo"; chosen: Theme.glyphSetting === "nintendo"; onActivated: Theme.glyphSetting = "nintendo" }
        }
        SettingRow {
            id: motionRow
            width: parent.width
            text: "Reduce motion"
            checked: Theme.reduceMotion
            onActivated: Theme.reduceMotion = !Theme.reduceMotion
        }
        SettingRow {
            id: safeRow
            width: parent.width
            text: "Show TV safe area"
            checked: Theme.showSafeArea
            onActivated: Theme.showSafeArea = !Theme.showSafeArea
        }
    }
}
