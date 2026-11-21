// SPDX-License-Identifier: GPL-3.0-or-later
// Settings → System → Credits and licences ([[legal-compliance]]): the authors first, side by side
// at the same level (approved demo 3.9b, as on the website), the projects OwneetOS is built on
// (CREDITS.md), then every installed package with its licence. The right stick (or up / down)
// scrolls the list.
import QtQuick 2.15
import "foundation"

Sheet {
    id: root
    title: Tr.tr("credits.title")
    prompts: [{ buttons: ["rs"], label: Tr.tr("prompt.scroll") }, { buttons: ["b"], label: Tr.tr("prompt.close") }]
    topMargin: 40
    bottomMargin: 76
    scrollTarget: list

    property var authors: []
    property var credits: []
    property var packages: []
    function openCredits() {
        authors = systemInfo.authors();
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
            height: root.flickable.height - y - Theme.px(36)
            clip: true
            interactive: false
            spacing: Theme.px(6)
            // the authors, the credited projects, a heading, then the packages
            model: (root.authors.length ? [{ authors: root.authors }] : [])
                .concat(root.credits.map(c => ({ a: c.name, b: c.who, c: c.licence })))
                .concat([{ heading: Tr.trn("credits.packages", root.packages.length) }])
                .concat(root.packages.map(p => ({ a: p.name, b: p.version, c: p.licence })))
            Behavior on contentY { enabled: !Theme.reduceMotion; NumberAnimation { duration: Theme.motionMs; easing.type: Easing.OutCubic } }
            delegate: Rectangle {
                readonly property bool cells: !modelData.heading && !modelData.authors
                width: list.width
                height: modelData.authors ? authorsRow.implicitHeight + Theme.px(6)
                        : modelData.heading ? headingText.implicitHeight + Theme.px(20)
                        : Math.max(cellA.implicitHeight, cellB.implicitHeight, cellC.implicitHeight) + Theme.px(14)
                radius: Theme.radiusS
                color: cells ? Theme.surface : "transparent"
                Row {                    // the authors, side by side
                    id: authorsRow
                    readonly property int count: modelData.authors ? modelData.authors.length : 0
                    visible: count > 0
                    width: parent.width
                    spacing: Theme.px(12)
                    Repeater {
                        model: modelData.authors || []
                        Rectangle {
                            width: (authorsRow.width - authorsRow.spacing * (authorsRow.count - 1)) / Math.max(1, authorsRow.count)
                            height: authorColumn.implicitHeight + Theme.px(24)
                            radius: Theme.radiusS
                            color: Theme.surface
                            Column {
                                id: authorColumn
                                x: Theme.px(14); width: parent.width - Theme.px(28)
                                anchors.verticalCenter: parent.verticalCenter
                                spacing: Theme.px(2)
                                Text {
                                    width: parent.width
                                    text: modelData.name
                                    color: Theme.fg
                                    font.family: Theme.textFont
                                    font.weight: Font.Medium
                                    font.pixelSize: Theme.fs(14.5)
                                    elide: Text.ElideRight
                                }
                                Text {
                                    width: parent.width
                                    text: modelData.github
                                    color: Theme.muted
                                    font.family: Theme.textFont
                                    font.pixelSize: Theme.fs(14.5)
                                    elide: Text.ElideRight
                                }
                            }
                        }
                    }
                }
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
                    visible: parent.cells
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
                    visible: parent.cells
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
                    visible: parent.cells
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
