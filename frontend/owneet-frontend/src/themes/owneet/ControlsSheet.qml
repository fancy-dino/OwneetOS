// SPDX-License-Identifier: GPL-3.0-or-later
// Quick menu → Controls and features (roadmap 3.13): a read-only guide to every button
// (PROJECT_RULES.md section 9.1), scrolled like Credits and licences (right stick, or up / down).
import QtQuick 2.15
import "foundation"

Sheet {
    id: root
    title: Tr.tr("controls.title")
    prompts: [{ buttons: ["rs"], label: Tr.tr("prompt.scroll") }, { buttons: ["b"], label: Tr.tr("prompt.close") }]
    topMargin: 40
    bottomMargin: 76
    scrollTarget: list

    function openControls() {
        list.contentY = list.originY;
        open();
    }

    // [section heading] then [buttons, message] rows; a button "kb:…" is a key of a keyboard
    readonly property var rows: [
        ["controls.everywhere"],
        [["dpad"], "controls.move"], [["a"], "controls.a"], [["b"], "controls.b"], [["lb", "rb"], "controls.sections"],
        [["guide"], "controls.guide"], [["b"], "controls.quick"], [["rs"], "controls.rs"],
        ["controls.pages"],
        [["x"], "controls.x"], [["y"], "controls.y"], [["lt", "rt"], "controls.filters"], [["menu"], "controls.menu"],
        ["controls.keyboard"],
        [["a"], "controls.k.a"], [["x"], "controls.k.x"], [["y"], "controls.k.y"], [["lt", "rt"], "controls.k.pages"],
        [["menu"], "controls.k.done"], [["b"], "controls.k.b"],
        ["controls.games"],
        [["guide"], "controls.game.home"], [["guide"], "controls.game.cage"],
        ["controls.physical"],
        [["kb:↑ ↓ ← →"], "controls.kb.move"], [["kb:Enter", "kb:Space"], "controls.kb.a"], [["kb:Esc", "kb:Backspace"], "controls.kb.b"],
        [["kb:Q", "kb:E"], "controls.kb.sections"], [["kb:Z", "kb:C"], "controls.kb.filters"], [["kb:X", "kb:Y", "kb:M"], "controls.kb.others"],
        [["kb:PgUp", "kb:PgDn"], "controls.kb.scroll"], [[], "controls.kb.type"]
    ]

    ListView {
        id: list
        width: parent.width
        height: root.flickable.height - Theme.px(24)
        clip: true
        interactive: false
        spacing: Theme.px(6)
        model: root.rows
        Behavior on contentY { enabled: !Theme.reduceMotion; NumberAnimation { duration: Theme.motionMs; easing.type: Easing.OutCubic } }
        delegate: Item {
            readonly property bool heading: modelData.length === 1
            width: list.width
            height: heading ? headingText.implicitHeight + Theme.px(22) : Math.max(keys.height, message.implicitHeight) + Theme.px(16)
            Text {
                id: headingText
                visible: parent.heading
                anchors { left: parent.left; bottom: parent.bottom; bottomMargin: Theme.px(6) }
                text: parent.heading ? Tr.tr(modelData[0]) : ""
                color: Theme.muted
                font.family: Theme.textFont
                font.weight: Font.Medium
                font.pixelSize: Theme.fs(12.5)
                font.capitalization: Font.AllUppercase
                font.letterSpacing: Theme.fs(12.5) * 0.09
            }
            Rectangle {
                visible: !parent.heading
                anchors.fill: parent
                radius: Theme.radiusS
                color: Theme.surface
                Row {
                    id: keys
                    x: Theme.px(12)
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: Theme.px(6)
                    Repeater {
                        model: parent.parent.parent.heading ? [] : modelData[0]
                        Item {
                            readonly property bool key: modelData.indexOf("kb:") === 0
                            width: key ? keyLabel.implicitWidth + Theme.px(16) : glyph.width
                            height: Theme.fs(26)
                            Glyph {
                                id: glyph
                                visible: !parent.key
                                button: parent.key ? "a" : modelData
                                anchors.verticalCenter: parent.verticalCenter
                            }
                            Rectangle {
                                visible: parent.key
                                anchors.fill: parent
                                radius: Theme.px(6)
                                color: "transparent"
                                border.color: Theme.line
                                border.width: Math.max(1, Theme.px(1))
                                Text {
                                    id: keyLabel
                                    anchors.centerIn: parent
                                    text: modelData.slice(3)
                                    color: Theme.muted
                                    font.family: Theme.textFont
                                    font.pixelSize: Theme.fs(12.5)
                                }
                            }
                        }
                    }
                }
                Text {
                    id: message
                    anchors { left: parent.left; leftMargin: Theme.px(190); right: parent.right; rightMargin: Theme.px(12); verticalCenter: parent.verticalCenter }
                    text: parent.parent.heading ? "" : Tr.tr(modelData[1])
                    color: Theme.fg
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                    wrapMode: Text.Wrap
                }
            }
        }
    }
}
