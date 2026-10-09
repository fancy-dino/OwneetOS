// SPDX-License-Identifier: GPL-3.0-or-later
// Settings → System → Credits and licences (PROJECT_RULES.md section 12): the projects OwneetOS is
// built on (CREDITS.md), then every installed package with its licence. The right stick (or
// up / down) scrolls the list.
import QtQuick 2.15
import "foundation"

Sheet {
    id: root
    title: Tr.tr("credits.title")
    prompts: [{ buttons: ["rs"], label: Tr.tr("prompt.scroll") }, { buttons: ["b"], label: Tr.tr("prompt.close") }]
    topMargin: 40
    bottomMargin: 76
    scrollTarget: list

    property var credits: []
    property var packages: []
    function openCredits() {
        credits = systemInfo.credits();
        packages = systemInfo.packages();
        list.contentY = list.originY;
        open();
    }

    Column {
        width: parent.width
        spacing: Theme.px(12)
        Text {
            width: parent.width
            text: Tr.tr("credits.text")
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(14.5)
            wrapMode: Text.Wrap
        }
        ListView {
            id: list
            width: parent.width
            height: root.flickable.height - y - Theme.px(24)
            clip: true
            interactive: false
            spacing: Theme.px(6)
            // the credited projects, a heading, then the packages
            model: root.credits.map(c => ({ a: c.name, b: c.who, c: c.licence }))
                .concat([{ heading: Tr.trn("credits.packages", root.packages.length) }])
                .concat(root.packages.map(p => ({ a: p.name, b: p.version, c: p.licence })))
            Behavior on contentY { enabled: !Theme.reduceMotion; NumberAnimation { duration: Theme.motionMs; easing.type: Easing.OutCubic } }
            delegate: Rectangle {
                width: list.width
                height: modelData.heading ? headingText.implicitHeight + Theme.px(20) : Math.max(cellA.implicitHeight, cellB.implicitHeight, cellC.implicitHeight) + Theme.px(14)
                radius: Theme.radiusS
                color: modelData.heading ? "transparent" : Theme.surface
                Text {
                    id: headingText
                    visible: !!modelData.heading
                    anchors { left: parent.left; bottom: parent.bottom; bottomMargin: Theme.px(4) }
                    text: modelData.heading || ""
                    color: Theme.muted
                    font.family: Theme.textFont
                    font.weight: Font.Medium
                    font.pixelSize: Theme.fs(12.5)
                    font.capitalization: Font.AllUppercase
                }
                Text {
                    id: cellA
                    visible: !modelData.heading
                    x: Theme.px(12); width: parent.width * 0.26
                    anchors.verticalCenter: parent.verticalCenter
                    text: modelData.a || ""
                    color: Theme.fg
                    font.family: Theme.textFont
                    font.weight: Font.Medium
                    font.pixelSize: Theme.fs(14.5)
                    wrapMode: Text.Wrap
                }
                Text {
                    id: cellB
                    visible: !modelData.heading
                    x: parent.width * 0.29; width: parent.width * 0.42
                    anchors.verticalCenter: parent.verticalCenter
                    text: modelData.b || ""
                    color: Theme.muted
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                    wrapMode: Text.Wrap
                }
                Text {
                    id: cellC
                    visible: !modelData.heading
                    x: parent.width * 0.74; width: parent.width * 0.25
                    anchors.verticalCenter: parent.verticalCenter
                    text: modelData.c || ""
                    color: Theme.fg
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                    wrapMode: Text.Wrap
                }
            }
        }
    }
}
