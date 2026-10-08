// SPDX-License-Identifier: GPL-3.0-or-later
// The color palette picker: every palette as a card, grouped (dark, light, accessibility).
// Moving the selection previews the palette on the whole interface; A applies it, B restores
// the previous one.
import QtQuick 2.15

Sheet {
    id: root
    title: "Color palette"
    prompts: [{ buttons: ["a"], label: "Apply" }, { buttons: ["b"], label: "Cancel" }]
    note: "Moving the selection previews the palette"

    signal applied(string key)
    readonly property int columns: 7
    readonly property var groups: [
        { key: "dark", label: "Dark" }, { key: "light", label: "Light" }, { key: "access", label: "Accessibility" }
    ]
    // rows of palette indexes, for moving up and down across groups
    readonly property var rows: {
        const out = [];
        groups.forEach(g => {
            const idx = [];
            Theme.palettes.forEach((p, i) => { if (p.group === g.key) idx.push(i); });
            for (let i = 0; i < idx.length; i += columns)
                out.push(idx.slice(i, i + columns));
        });
        return out;
    }
    property int row: 0
    property int col: 0
    readonly property int current: rows[row] && col < rows[row].length ? rows[row][col] : 0
    property var cards: ({})

    function openPicker() {
        for (let r = 0; r < rows.length; r++) {
            const c = rows[r].indexOf(Theme.palettes.findIndex(p => p.key === Theme.paletteKey));
            if (c >= 0) { row = r; col = c; }
        }
        open();
        Theme.previewKey = Theme.palettes[current].key;
        focusCurrent();
    }
    function focusCurrent() {
        const card = cards[current];
        if (card) {
            card.forceActiveFocus();
            ensureVisible(card);
        }
        Theme.previewKey = Theme.palettes[current].key;
    }
    function finish(apply) {
        if (apply) {
            Theme.paletteKey = Theme.palettes[current].key;
            applied(Theme.paletteKey);
        }
        Theme.previewKey = "";
        close();
    }

    Keys.onPressed: {
        if (event.isAutoRepeat && (api.keys.isAccept(event) || api.keys.isCancel(event)))
            return;
        if (api.keys.isAccept(event)) { finish(true); }
        else if (api.keys.isCancel(event)) { finish(false); }
        else if (event.key === Qt.Key_Left && col > 0) { col--; focusCurrent(); }
        else if (event.key === Qt.Key_Right && col < rows[row].length - 1) { col++; focusCurrent(); }
        else if (event.key === Qt.Key_Up && row > 0) { row--; col = Math.min(col, rows[row].length - 1); focusCurrent(); }
        else if (event.key === Qt.Key_Down && row < rows.length - 1) { row++; col = Math.min(col, rows[row].length - 1); focusCurrent(); }
        else return;
        event.accepted = true;
    }

    Column {
        width: parent.width
        spacing: Theme.px(12)
        Repeater {
            model: root.groups
            Column {
                width: parent.width
                spacing: Theme.px(10)
                readonly property string groupKey: modelData.key
                Label { text: modelData.label }
                Grid {
                    id: grid
                    width: parent.width
                    columns: root.columns
                    spacing: Theme.px(10)
                    readonly property real cell: (width - spacing * (columns - 1)) / columns
                    Repeater {
                        model: Theme.palettes.filter(p => p.group === groupKey)
                        Rectangle {
                            id: card
                            readonly property var pal: modelData
                            readonly property bool chosen: pal.key === Theme.paletteKey
                            width: grid.cell
                            height: cardColumn.height + Theme.px(16)
                            radius: Theme.radiusS + Theme.px(2)
                            color: pal.c[1]
                            border.color: chosen ? pal.c[6] : pal.c[3]
                            border.width: Math.max(1, Theme.px(chosen ? 2 : 1))
                            Component.onCompleted: root.cards[Theme.palettes.findIndex(p => p.key === pal.key)] = card
                            Column {
                                id: cardColumn
                                x: Theme.px(8); y: Theme.px(8)
                                width: parent.width - Theme.px(16)
                                spacing: Theme.px(7)
                                Row {     // background, raised, accent, text
                                    width: parent.width
                                    height: Theme.px(34)
                                    Rectangle { width: parent.width * 0.4; height: parent.height; color: card.pal.c[0] }
                                    Rectangle { width: parent.width * 0.2; height: parent.height; color: card.pal.c[2] }
                                    Rectangle { width: parent.width * 0.2; height: parent.height; color: card.pal.c[6] }
                                    Rectangle { width: parent.width * 0.2; height: parent.height; color: card.pal.c[4] }
                                }
                                Text {
                                    width: parent.width
                                    horizontalAlignment: Text.AlignHCenter
                                    text: card.pal.name
                                    color: card.pal.c[4]
                                    font.family: Theme.textFont
                                    font.pixelSize: Theme.fs(12.5)
                                    wrapMode: Text.Wrap
                                }
                            }
                            FocusFrame {}
                        }
                    }
                }
            }
        }
    }
}
