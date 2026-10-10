// SPDX-License-Identifier: GPL-3.0-or-later
// A compact window in the middle of the screen (details, options, confirmations): a title, an
// optional text or content, and round buttons in a centred row, chosen with left / right
// (approved in demo 3.6). B closes it. Buttons: [{ text, style, action }], `defaultIndex` is the
// button selected first (e.g. Cancel in a confirmation), unless `initialItem` (an item of the
// window's own content, e.g. a text field) is set.
import QtQuick 2.15

NavArea {
    id: root
    property string title: ""
    property string text: ""
    property var buttons: []
    property int defaultIndex: 0
    property Item initialItem: null
    property real contentWidth: 0          // width the window's own content needs, if any
    readonly property var defaultPrompts: [{ buttons: ["a"], label: Tr.tr("prompt.select") }, { buttons: ["b"], label: Tr.tr("prompt.back") }]
    property var prompts: defaultPrompts
    default property alias content: column.data
    backFeedback: ""            // closing plays "sheet-close"

    anchors.fill: parent
    visible: false
    z: 150

    function open() {
        Nav.feedback("sheet-open");
        visible = true;
        forceActiveFocus();
        focusDefault();
    }
    // Also after the window's content changes (e.g. a password window becomes "Connecting…")
    function focusDefault() {
        forceActiveFocus();
        const buttons = Nav.navigables(row);
        const item = initialItem && initialItem.visible ? initialItem : buttons[defaultIndex] || buttons[0] || null;
        current = null;
        focusItem(item);
    }
    function close(quiet) {
        if (!visible)
            return;
        if (!quiet)
            Nav.feedback("sheet-close");
        visible = false;
        initialItem = null;             // the next window starts from the defaults
        prompts = Qt.binding(() => defaultPrompts);
    }
    onCancelled: close()
    // A runs the button's action; the action decides whether the window closes (quietly when
    // something else follows, e.g. another window or a game starting).
    // (the actions stay in `buttons`: a Repeater's modelData would drop the functions)
    onAccepted: {
        if (!item || item.buttonIndex === undefined)
            return;             // content with its own action (e.g. a password field)
        const b = buttons[item.buttonIndex];
        if (b && b.action)
            b.action();
    }

    Rectangle {
        anchors.fill: parent
        color: Qt.rgba(Theme.bg.r, Theme.bg.g, Theme.bg.b, 0.85)
    }
    Rectangle {
        id: box
        anchors.centerIn: parent
        anchors.verticalCenterOffset: -Theme.px(30)
        width: Math.max(Theme.px(520), Math.min(Theme.px(1000), Math.max(row.implicitRowWidth, contentWidth) + Theme.px(64)))
        height: column.height + row.height + Theme.px(20) + Theme.px(44)
        radius: Theme.radiusL
        color: Theme.bg
        border.color: Theme.line
        border.width: Math.max(1, Theme.px(1))
        // Title, text and the window's own content (hidden items take no room), then the buttons
        Column {
            id: column
            anchors { top: parent.top; topMargin: Theme.px(22); horizontalCenter: parent.horizontalCenter }
            width: box.width - Theme.px(48)
            spacing: Theme.px(14)
            Text {
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                text: root.title
                color: Theme.fg
                font.family: Theme.displayFont
                font.weight: Font.Bold
                font.pixelSize: Theme.fs(24)
                wrapMode: Text.Wrap
            }
            Text {
                visible: root.text !== ""
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                text: root.text
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(17)
                wrapMode: Text.Wrap
            }
        }
        Flow {
            id: row
            anchors { top: column.bottom; topMargin: Theme.px(20); horizontalCenter: parent.horizontalCenter }
            width: Math.min(column.width, implicitRowWidth)
            readonly property real implicitRowWidth: {
                const buttons = Nav.navigables(row);
                let w = 0;
                for (let i = 0; i < buttons.length; i++) w += buttons[i].width + spacing;
                return Math.max(0, w - spacing);
            }
            spacing: Theme.px(20)
            Repeater {
                model: root.buttons
                Button {
                    text: modelData.text
                    style: modelData.style || "secondary"
                    readonly property int buttonIndex: index
                }
            }
        }
    }
    PromptBar {
        anchors {
            left: parent.left; right: parent.right; bottom: parent.bottom
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; bottomMargin: Theme.safeBottom
        }
        prompts: root.prompts
    }
}
