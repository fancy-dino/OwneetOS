// SPDX-License-Identifier: GPL-3.0-or-later
// Notification banners (roadmap 3.11, approved demo design/demos/3.11-notifications.html): at the
// top right, under the top bar; at most three at a time, the newest on top, each gone after 5 s.
// show({ title, detail, icon, crossed, kind }); kind "attention" (accent) or "info".
import QtQuick 2.15

Item {
    id: root
    width: Theme.px(400)
    height: column.height
    z: 190

    ListModel { id: shown }
    property int nextId: 0
    function show(n) {
        shown.insert(0, { uid: nextId++, title: n.title, detail: n.detail || "", icon: n.icon, crossed: n.crossed === true, kind: n.kind,
                          until: Date.now() + 5000 });
        while (shown.count > 3)
            shown.remove(shown.count - 1);
    }
    function clear() { shown.clear(); }
    function removeUid(uid) {
        for (let i = 0; i < shown.count; i++)
            if (shown.get(i).uid === uid) { shown.remove(i); return; }
    }

    Column {
        id: column
        width: parent.width
        spacing: Theme.px(10)
        Repeater {
            model: shown
            Rectangle {
                id: card
                width: column.width
                height: Math.max(iconBox.height, texts.height) + Theme.px(28)
                radius: Theme.radiusL
                color: Theme.surface
                border.color: model.kind === "attention" ? Theme.accent : Theme.line
                border.width: Math.max(1, Theme.px(1))
                opacity: 0
                x: Theme.reduceMotion ? 0 : Theme.px(40)
                Component.onCompleted: { opacity = 1; x = 0; }
                Behavior on opacity { NumberAnimation { duration: Theme.reduceMotion ? 0 : 220 } }
                Behavior on x { NumberAnimation { duration: Theme.reduceMotion ? 0 : 220; easing.type: Easing.OutCubic } }
                Rectangle {
                    id: iconBox
                    x: Theme.px(16)
                    anchors.verticalCenter: parent.verticalCenter
                    width: Theme.px(40); height: width
                    radius: Theme.radiusS
                    color: model.kind === "attention" ? Theme.accent : Theme.raised
                    Icon {
                        anchors.centerIn: parent
                        width: Theme.fs(22); height: width
                        name: model.icon
                        crossed: model.crossed
                        color: model.kind === "attention" ? Theme.onAccent : Theme.fg
                    }
                }
                Column {
                    id: texts
                    anchors { left: iconBox.right; leftMargin: Theme.px(14); right: parent.right; rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                    spacing: Theme.px(2)
                    Text {
                        width: parent.width
                        text: model.title
                        color: Theme.fg
                        font.family: Theme.textFont
                        font.weight: Font.Medium
                        font.pixelSize: Theme.fs(17)
                        wrapMode: Text.Wrap
                    }
                    Text {
                        visible: text !== ""
                        width: parent.width
                        text: model.detail
                        color: Theme.muted
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                        wrapMode: Text.Wrap
                    }
                }
                // from the clock: a QML Timer follows the frames drawn, not the real time
                Timer {
                    interval: 250; running: true; repeat: true
                    onTriggered: if (Date.now() >= model.until) root.removeUid(model.uid)
                }
            }
        }
    }
}
