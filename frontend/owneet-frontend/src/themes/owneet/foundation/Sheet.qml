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
        if (!visible)
            Nav.windowOpened();     // A just after it opens is ignored
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
    scrollTarget: flick
    onMoved: ensureVisible(item)
    onCancelled: close()

    function ensureVisible(item) {
        const y = item.mapToItem(body, 0, 0).y + body.y;
        const pad = body.y;
        if (y - pad < flick.contentY)
            flick.contentY = Math.max(0, y - pad);
        else if (y + item.height + pad > flick.contentY + flick.height)
            flick.contentY = Math.min(flick.contentHeight - flick.height, y + item.height + pad - flick.height);
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

        FitText {
            id: heading
            anchors { left: parent.left; right: parent.right; top: parent.top }
            anchors { leftMargin: Theme.px(24); rightMargin: Theme.px(24); topMargin: Theme.px(20) }
            text: root.title
            color: Theme.fg
            font.family: Theme.displayFont
            font.weight: Font.Bold
            font.pixelSize: Theme.fs(24)
        }
        Flickable {
            id: flick
            anchors {
                left: parent.left; right: parent.right; top: heading.bottom; bottom: parent.bottom
                leftMargin: Theme.px(2); rightMargin: Theme.px(2)
                topMargin: 0; bottomMargin: Theme.px(4)
            }
            clip: true
            interactive: false
            contentWidth: width
            contentHeight: body.y + body.height
            Behavior on contentY {
                enabled: !Theme.reduceMotion
                NumberAnimation { duration: Theme.motionMs; easing.type: Easing.OutCubic }
            }
            Item {
                id: body
                // inner padding leaves room for the focus ring and the lift of the selected row (3w c:
                // 8 px cut the ring of a wide row); the rows stay where they were
                x: Theme.px(22)
                y: Theme.px(14)
                width: flick.width - Theme.px(44)
                height: childrenRect.height + Theme.px(14)
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
