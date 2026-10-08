// SPDX-License-Identifier: GPL-3.0-or-later
// The color palette picker: every palette as a card, grouped (dark, light, accessibility).
// Moving the selection previews the palette on the whole interface; A applies it, B restores
// the previous one.
import QtQuick 2.15

Sheet {
    id: root
    title: Tr.tr("palette.title")
    prompts: [{ buttons: ["a"], label: Tr.tr("prompt.apply") }, { buttons: ["b"], label: Tr.tr("prompt.cancel") }]
    note: Tr.tr("palette.preview_note")

    signal applied(string key)
    readonly property int columns: 7
    readonly property var groups: [
        { key: "dark", label: "palette.group.dark" }, { key: "light", label: "palette.group.light" },
        { key: "access", label: "palette.group.access" }
    ]
    property var cards: ({})

    function openPicker() {
        open(true);
        focusItem(cards[Theme.paletteKey]);
    }
    onMoved: Theme.previewKey = item.pal.key
    onAccepted: {
        Theme.paletteKey = item.pal.key;
        Theme.previewKey = "";
        applied(Theme.paletteKey);
        close(true);
    }
    onCancelled: Theme.previewKey = ""

    Column {
        width: parent.width
        spacing: Theme.px(12)
        Repeater {
            model: root.groups
            Column {
                width: parent.width
                spacing: Theme.px(10)
                readonly property string groupKey: modelData.key
                Label { text: Tr.tr(modelData.label) }
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
                            readonly property bool navigable: true
                            width: grid.cell
                            height: cardColumn.height + Theme.px(16)
                            radius: Theme.radiusS + Theme.px(2)
                            color: pal.c[1]
                            border.color: chosen ? pal.c[6] : pal.c[3]
                            border.width: Math.max(1, Theme.px(chosen ? 2 : 1))
                            Component.onCompleted: root.cards[pal.key] = card
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
                                    text: Tr.tr("palette." + card.pal.key)
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
