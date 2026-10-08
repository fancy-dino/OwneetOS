// SPDX-License-Identifier: GPL-3.0-or-later
// A window over the current screen (pickers, options), with its own prompt bar; the screen
// below is dimmed. It is a NavArea: the selection moves among its navigable items, B closes it.
// Its content scrolls to keep the selection visible.
import QtQuick 2.15

NavArea {
    id: root
    property string title: ""
    property var prompts: []
    property string note: ""
    // Margins of the window on the 1280 x 720 grid (left/right, top, bottom)
    property real sideMargin: 64
    property real topMargin: 56
    property real bottomMargin: 76
    default property alias content: body.data
    property alias flickable: flick

    anchors.fill: parent
    visible: false
    // `quiet`: no sound, when one window hands over to another (e.g. Appearance → palettes)
    function open(quiet) {
        if (!visible && !quiet)
            Nav.feedback("sheet-open");
        visible = true;
        forceActiveFocus();
    }
    function close(quiet) {
        if (!visible)
            return;
        if (!quiet)
            Nav.feedback("sheet-close");
        visible = false;
    }
    backFeedback: ""            // closing plays "sheet-close"
    onMoved: ensureVisible(item)
    onCancelled: close()

    function ensureVisible(item) {
        const p = item.mapToItem(body, 0, 0);
        const pad = Theme.px(12);
        if (p.y - pad < flick.contentY)
            flick.contentY = Math.max(0, p.y - pad);
        else if (p.y + item.height + pad > flick.contentY + flick.height)
            flick.contentY = Math.min(body.height - flick.height, p.y + item.height + pad - flick.height);
    }

    Rectangle {          // dims the screen below; in the palette's own background color, so the
        anchors.fill: parent // prompt bar stays readable with light palettes too
        color: Qt.rgba(Theme.bg.r, Theme.bg.g, Theme.bg.b, 0.85)
    }
    Rectangle {
        id: sheet
        anchors {
            fill: parent
            leftMargin: Theme.px(root.sideMargin); rightMargin: Theme.px(root.sideMargin)
            topMargin: Theme.px(root.topMargin); bottomMargin: Theme.px(root.bottomMargin)
        }
        color: Theme.bg
        border.color: Theme.line
        border.width: Math.max(1, Theme.px(1))
        radius: Theme.radiusL

        Text {
            id: heading
            anchors { left: parent.left; right: parent.right; top: parent.top }
            anchors { leftMargin: Theme.px(24); rightMargin: Theme.px(24); topMargin: Theme.px(20) }
            text: root.title
            color: Theme.fg
            font.family: Theme.displayFont
            font.weight: Font.Bold
            font.pixelSize: Theme.fs(24)
            elide: Text.ElideRight
        }
        Flickable {
            id: flick
            anchors {
                left: parent.left; right: parent.right; top: heading.bottom; bottom: parent.bottom
                leftMargin: Theme.px(16); rightMargin: Theme.px(16)
                topMargin: Theme.px(6); bottomMargin: Theme.px(10)
            }
            clip: true
            interactive: false
            contentWidth: width
            contentHeight: body.height
            Behavior on contentY {
                enabled: !Theme.reduceMotion
                NumberAnimation { duration: Theme.motionMs; easing.type: Easing.OutCubic }
            }
            Item {
                id: body
                // inner padding leaves room for the focus ring
                x: Theme.px(8)
                y: Theme.px(8)
                width: flick.width - Theme.px(16)
                height: childrenRect.height + Theme.px(16)
            }
        }
    }
    PromptBar {
        anchors {
            left: parent.left; right: parent.right; bottom: parent.bottom
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; bottomMargin: Theme.safeBottom
        }
        prompts: root.prompts
        note: root.note
    }
}
