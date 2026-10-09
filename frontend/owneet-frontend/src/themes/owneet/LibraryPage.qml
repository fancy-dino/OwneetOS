// SPDX-License-Identifier: GPL-3.0-or-later
// Library section: for now a plain list of every game (A plays, X details, Menu options). The
// real library, with its grid and filters, comes in step 3.7 (its own demo first).
import QtQuick 2.15
import "foundation"

FocusScope {
    id: page
    property var games: []
    signal launch(var game)
    signal details(var game)
    signal options(var game)

    readonly property var prompts: list.count > 0
        ? [{ buttons: ["a"], label: Tr.tr("prompt.play") }, { buttons: ["x"], label: Tr.tr("prompt.details") },
           { buttons: ["menu"], label: Tr.tr("prompt.options") }]
        : []
    function focusStart() { list.forceActiveFocus(); }
    function select(game) {
        const i = games.indexOf(game);
        if (i >= 0) list.currentIndex = i;
    }

    Text {
        id: heading
        text: Tr.tr("tab.library")
        color: Theme.fg
        font.family: Theme.displayFont
        font.weight: Font.Bold
        font.pixelSize: Theme.fs(40)
    }
    Text {
        visible: list.count > 0
        anchors { left: heading.right; leftMargin: Theme.px(14); baseline: heading.baseline }
        text: Tr.trn("games.count", list.count)
        color: Theme.muted
        font.family: Theme.textFont
        font.pixelSize: Theme.fs(17)
    }
    ListView {
        id: list
        anchors {
            left: parent.left; right: parent.right; top: heading.bottom; bottom: parent.bottom
            leftMargin: -Theme.px(20); rightMargin: -Theme.px(20); topMargin: Theme.px(10)
        }
        model: page.games
        focus: true
        clip: true
        spacing: Theme.px(10)
        topMargin: Theme.px(8)
        bottomMargin: Theme.px(8)
        highlightRangeMode: ListView.ApplyRange
        preferredHighlightBegin: Theme.px(16)
        preferredHighlightEnd: height - Theme.px(16)
        highlightMoveDuration: Theme.motionMs
        keyNavigationEnabled: false             // Nav.listKeys moves the selection
        delegate: Rectangle {
            readonly property var game: modelData
            x: Theme.px(20)                      // room for the lift and the focus ring
            width: list.width - Theme.px(40)
            height: title.implicitHeight + Theme.px(28)
            radius: Theme.radiusS + Theme.px(2)
            color: Theme.surface
            border.color: Theme.line
            border.width: Math.max(1, Theme.px(1))
            Text {
                id: title
                anchors {
                    left: parent.left; right: source.left; verticalCenter: parent.verticalCenter
                    leftMargin: Theme.px(18); rightMargin: Theme.px(18)
                }
                text: modelData.title
                color: Theme.fg
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(17)
                elide: Text.ElideRight
            }
            Text {
                id: source
                anchors { right: parent.right; rightMargin: Theme.px(18); verticalCenter: parent.verticalCenter }
                text: GameInfo.source(modelData)
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(14.5)
            }
            FocusFrame { shown: parent.ListView.isCurrentItem && list.activeFocus }
        }
        Keys.onPressed: {
            const a = Nav.action(event);
            const game = currentItem ? currentItem.game : null;
            if (Nav.listKeys(list, a)) event.accepted = true;
            else if (a === "left" || a === "right") { Nav.feedback("edge"); event.accepted = true; }
            else if (a === "accept" && game) { Nav.feedback("confirm"); page.launch(game); event.accepted = true; }
            else if (a === "secondary" && game) { page.details(game); event.accepted = true; }
            else if (a === "options" && game) { page.options(game); event.accepted = true; }
        }
    }
    Text {
        visible: list.count === 0
        anchors.centerIn: list
        text: Tr.tr("games.empty")
        color: Theme.muted
        font.family: Theme.textFont
        font.pixelSize: Theme.fs(17)
    }
}
