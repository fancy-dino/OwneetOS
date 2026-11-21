// SPDX-License-Identifier: GPL-3.0-or-later
// Settings → Language → Time zone ([[step-6.1-first-boot-flow]], approved demo
// 6.1-settings-time): every city of the first start, by distance from UTC, each with its time
// now; the current one ticked. A sets its zone (owneetd: PUT /v1/time/zone) and keeps the city
// (the interface's memory "city", for the cities that share a zone), B goes back.
import QtQuick 2.15
import "foundation"

Sheet {
    id: root
    title: Tr.tr("first.time.choose")
    prompts: [{ buttons: ["a"], label: Tr.tr("prompt.select") }, { buttons: ["b"], label: Tr.tr("prompt.back") },
              { buttons: ["rs"], label: Tr.tr("prompt.scroll") }]
    sideMargin: 300
    topMargin: 60
    bottomMargin: 80

    property string current: ""          // the city shown as chosen
    property var toast: null
    property real now: Date.now()
    property var rows: ({})
    signal chosen(string city)

    function openPicker(city) {
        current = city;
        now = Date.now();
        flickable.contentY = 0;
        open();
        Qt.callLater(() => {
            cities.forceLayout();         // the rows' places, to scroll to the current one
            const row = rows[current];
            if (!row) { focusFirst(); return; }
            focusItem(row);
            // in the middle of the window, away from its edges
            const f = flickable, y = row.mapToItem(f.contentItem, 0, 0).y;
            f.contentY = Math.max(0, Math.min(f.contentHeight - f.height, y + row.height / 2 - f.height / 2));
        });
    }
    onAccepted: {
        const city = item.city;
        owneetd.put("/v1/time/zone", { zone: TimeZones.cityZones[city] }, status => {
            if (status === 200) {
                api.memory.set("city", city);
                if (root.toast) root.toast.show(Tr.tr("toast.time_zone", { city: Tr.tr("city." + city) }));
                root.chosen(city);
            } else if (root.toast) {
                root.toast.show(Tr.tr("first.time.failed"));
            }
        });
        close(true);
    }

    Column {
        id: cities
        width: parent.width
        spacing: Theme.px(8)
        Repeater {
            model: root.visible ? TimeZones.allCities() : []
            Rectangle {
                id: row
                readonly property string city: modelData
                readonly property bool navigable: true
                readonly property string zone: TimeZones.cityZones[modelData]
                width: parent.width
                height: name.implicitHeight + Theme.px(22)
                radius: Theme.radiusS + Theme.px(2)
                color: Theme.surface
                border.color: Theme.line
                border.width: Math.max(1, Theme.px(1))
                Component.onCompleted: root.rows[modelData] = row
                Text {
                    id: name
                    anchors { left: parent.left; leftMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                    text: Tr.tr("city." + modelData)
                    color: Theme.fg
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(17)
                }
                Row {
                    anchors { right: parent.right; rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                    spacing: Theme.px(14)
                    Text {
                        anchors.verticalCenter: parent.verticalCenter
                        text: TimeZones.clockAt(row.zone, root.now)
                        color: Theme.muted
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(17)
                    }
                    Text {
                        anchors.verticalCenter: parent.verticalCenter
                        width: Theme.fs(72)
                        text: TimeZones.utcLabel(row.zone)
                        color: Theme.muted
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                    }
                    Text {
                        anchors.verticalCenter: parent.verticalCenter
                        width: Theme.fs(18)
                        text: modelData === root.current ? "✓" : ""
                        color: Theme.accent
                        font.family: Theme.textFont
                        font.weight: Font.DemiBold
                        font.pixelSize: Theme.fs(17)
                    }
                }
                FocusFrame {}
            }
        }
    }
}
