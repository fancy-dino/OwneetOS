// SPDX-License-Identifier: GPL-3.0-or-later
// The Notifications window ([[step-3.11-notifications]], approved demo
// design/demos/3.11-notifications.html), opened with Y on the home screen: newest first, with the
// time and, where there is one, where A takes you; X removes one, Y clears all, B closes.
import QtQuick 2.15
import "foundation"

Sheet {
    id: root
    title: Tr.tr("notices.title")
    topMargin: 40
    bottomMargin: 76

    property var notices: []        // from the shell: { key, args, detailKey, icon, crossed, kind, action, time }
    signal openAction(var notice)
    signal removeAt(int index)
    signal clearAll()

    readonly property var focusedNotice: current && current.notice !== undefined ? current.notice : null
    prompts: {
        const P = (b, l) => ({ buttons: [b], label: Tr.tr(l) });
        if (notices.length === 0)
            return [P("b", "prompt.close")];
        return (focusedNotice && focusedNotice.action ? [P("a", "prompt.open")] : [])
            .concat([P("x", "prompt.remove"), P("y", "prompt.clear_all"), P("b", "prompt.close")]);
    }

    function openSheet() {
        now = Date.now();
        open();
        Qt.callLater(() => focusIndex(0));
    }
    function focusIndex(i) {
        const rows = Nav.navigables(list);
        current = null;
        focusItem(rows[Math.min(i, rows.length - 1)] || null);
        if (!current)
            forceActiveFocus();
    }

    onAccepted: {
        if (item.notice && item.notice.action)
            openAction(item.notice);
    }
    onOtherAction: {
        if (action === "secondary" && current && current.notice !== undefined) {
            Nav.feedback("confirm");
            removeAt(current.index);
        } else if (action === "page" && notices.length > 0) {
            Nav.feedback("confirm");
            clearAll();
        } else {
            Nav.feedback("edge");
        }
        event.accepted = true;
    }

    // "now", "5 min ago"… (refreshed while the window is open)
    property real now: Date.now()
    Timer { interval: 30000; repeat: true; running: root.visible; onTriggered: root.now = Date.now() }
    function ago(t) {
        const m = Math.floor((now - t) / 60000);
        return m < 1 ? Tr.tr("notices.now") : m < 60 ? Tr.tr("notices.minutes", { n: m }) : Tr.tr("notices.hours", { n: Math.floor(m / 60) });
    }

    Column {
        id: list
        width: parent.width
        spacing: Theme.px(10)
        Repeater {
            model: root.notices
            Rectangle {
                readonly property bool navigable: true
                readonly property var notice: modelData
                readonly property int index: model.index
                readonly property bool attention: modelData.kind === "attention"
                width: list.width
                height: Math.max(iconBox.height, texts.height, side.height) + Theme.px(24)
                radius: Theme.radiusM
                color: Theme.surface
                border.color: attention ? Qt.tint(Theme.line, Qt.rgba(Theme.accent.r, Theme.accent.g, Theme.accent.b, 0.6)) : Theme.line
                border.width: Math.max(1, Theme.px(1))
                Rectangle {
                    id: iconBox
                    x: Theme.px(16)
                    anchors.verticalCenter: parent.verticalCenter
                    width: Theme.px(40); height: width
                    radius: Theme.radiusS
                    color: parent.attention ? Theme.accent : Theme.raised
                    Icon {
                        anchors.centerIn: parent
                        width: Theme.fs(22); height: width
                        name: modelData.icon
                        crossed: modelData.crossed === true
                        color: parent.parent.attention ? Theme.onAccent : Theme.fg
                    }
                }
                Column {
                    id: texts
                    anchors { left: iconBox.right; leftMargin: Theme.px(14); right: side.left; rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                    spacing: Theme.px(2)
                    Text {
                        width: parent.width
                        text: Tr.tr(modelData.key, modelData.args)
                        color: Theme.fg
                        font.family: Theme.textFont
                        font.weight: Font.Medium
                        font.pixelSize: Theme.fs(17)
                        wrapMode: Text.Wrap
                    }
                    Text {
                        readonly property string detailKey: modelData.detailKey !== undefined ? modelData.detailKey : modelData.key + ".detail"
                        visible: detailKey !== ""
                        width: parent.width
                        text: detailKey ? Tr.tr(detailKey, modelData.args) : ""
                        color: Theme.muted
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                        wrapMode: Text.Wrap
                    }
                }
                Column {
                    id: side
                    anchors { right: parent.right; rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                    spacing: Theme.px(4)
                    Text {
                        anchors.right: parent.right
                        text: root.ago(modelData.time)
                        color: Theme.muted
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                    }
                    Text {
                        visible: !!modelData.action
                        anchors.right: parent.right
                        text: modelData.action ? Tr.tr("notices.action." + modelData.action) + "  ›" : ""
                        color: Theme.fg
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                    }
                }
                FocusFrame {}
            }
        }
        Column {                           // nothing yet
            visible: root.notices.length === 0
            width: parent.width
            topPadding: Theme.px(120)
            spacing: Theme.px(6)
            Text {
                anchors.horizontalCenter: parent.horizontalCenter
                text: Tr.tr("notices.empty")
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(17)
            }
            Text {
                anchors.horizontalCenter: parent.horizontalCenter
                text: Tr.tr("notices.empty.detail")
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(14.5)
            }
        }
    }
}
