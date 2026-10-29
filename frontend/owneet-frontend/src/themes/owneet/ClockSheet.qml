// SPDX-License-Identifier: GPL-3.0-or-later
// Date and time by hand ([[step-6.1-first-boot-flow]], approved demos 6.1-first-start and
// 6.1-settings-time): without the internet the console uses the PC's own clock, which may be
// wrong. Day, month, year, hours and minutes, one field each, in a zone's time: left / right
// choose the field, up / down change it (held: it repeats), A saves
// (owneetd: PUT /v1/time/now, in that zone), B goes back. Opened by the first start (the city
// shown or selected: the console's zone is not set yet) and by Settings → Language.
import QtQuick 2.15
import "foundation"

FocusScope {
    id: root
    anchors.fill: parent
    visible: false
    z: 101

    property var fields: [1, 1, 2026, 0, 0]     // day, month, year, hours, minutes
    property int at: 3                          // the field selected: the hours
    property var toast: null                    // the theme's, which stays after the window closes
    property string zone: ""                    // the zone of the fields (Europe/Rome)
    signal saved()

    // open([29, 10, 2026, 14, 30], "Europe/Rome"): the date and time now in that zone
    function open(now, zone) {
        fields = now;
        root.zone = zone;
        at = 3;
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
    readonly property var low: [1, 1, 2024, 0, 0]
    readonly property var high: [31, 12, 2100, 23, 59]
    function step(by) {
        const f = fields.slice();
        let v = f[at] + by;
        if (v > high[at]) v = low[at];
        if (v < low[at]) v = high[at];
        f[at] = v;
        // a day the month does not have becomes its last one
        const days = new Date(Date.UTC(f[2], f[1], 0)).getUTCDate();
        if (f[0] > days) f[0] = days;
        fields = f;
        Nav.feedback("move");
    }
    function save() {
        const two = n => (n < 10 ? "0" : "") + n;
        const date = fields[2] + "-" + two(fields[1]) + "-" + two(fields[0]);
        const time = two(fields[3]) + ":" + two(fields[4]);
        owneetd.put("/v1/time/now", { date: date, time: time, zone: zone }, status => {
            if (status === 200) {
                Nav.feedback("notify");
                if (root.toast) root.toast.show(Tr.tr("clock.saved"));
                root.saved();
            } else {
                Nav.feedback("error");
                if (root.toast) root.toast.show(Tr.tr("clock.failed"));
            }
        });
        close(true);
    }

    Keys.onPressed: {
        const a = Nav.action(event);
        if (a === "left" || a === "right") {
            const n = at + (a === "left" ? -1 : 1);
            if (n < 0 || n > 4) Nav.feedback("edge");
            else { at = n; Nav.feedback("move"); }
        } else if (a === "up" || a === "down") {
            step(a === "up" ? 1 : -1);
        } else if (a === "accept") {
            Nav.feedback("confirm");
            save();
        } else if (a === "back") {
            close();
        } else {
            return;
        }
        event.accepted = true;
    }

    Rectangle {          // dims the screen below
        anchors.fill: parent
        color: Qt.rgba(Theme.bg.r, Theme.bg.g, Theme.bg.b, 0.85)
    }
    Rectangle {
        anchors.centerIn: parent
        width: Theme.px(760)
        height: column.height + Theme.px(48)
        radius: Theme.radiusL
        color: Theme.bg
        border.color: Theme.line
        border.width: Math.max(1, Theme.px(1))
        Column {
            id: column
            anchors { left: parent.left; right: parent.right; top: parent.top; margins: Theme.px(24) }
            spacing: Theme.px(16)
            Text {
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                text: Tr.tr("clock.title")
                color: Theme.fg
                font.family: Theme.displayFont
                font.weight: Font.Bold
                font.pixelSize: Theme.fs(24)
            }
            Text {
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                text: Tr.tr("clock.text")
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(14.5)
                wrapMode: Text.Wrap
            }
            Row {
                anchors.horizontalCenter: parent.horizontalCenter
                spacing: Theme.px(10)
                Repeater {
                    // the fields with what stands between them: / / then a gap, then :
                    model: [{ i: 0 }, { sep: "/" }, { i: 1 }, { sep: "/" }, { i: 2 }, { gap: true }, { i: 3 }, { sep: ":" }, { i: 4 }]
                    Item {
                        readonly property bool isField: modelData.i !== undefined
                        width: isField ? Theme.fs(modelData.i === 2 ? 92 : 68) : modelData.gap ? Theme.px(14) : sep.implicitWidth
                        height: Theme.fs(56)
                        Rectangle {
                            visible: parent.isField
                            anchors.fill: parent
                            radius: Theme.radiusM
                            color: Theme.surface
                            border.color: parent.isField && root.at === modelData.i ? Theme.accent : Theme.line
                            border.width: parent.isField && root.at === modelData.i ? Theme.px(3) : Math.max(1, Theme.px(1))
                            Text {
                                anchors.centerIn: parent
                                text: {
                                    const v = root.fields[modelData.i];
                                    return modelData.i === 2 ? String(v) : (v < 10 ? "0" : "") + v;
                                }
                                color: Theme.fg
                                font.family: Theme.textFont
                                font.pixelSize: Theme.fs(24)
                            }
                        }
                        Text {
                            id: sep
                            visible: !parent.isField && !modelData.gap
                            anchors.centerIn: parent
                            text: modelData.sep || ""
                            color: Theme.muted
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(24)
                        }
                    }
                }
            }
        }
    }
    PromptBar {
        anchors {
            left: parent.left; right: parent.right; bottom: parent.bottom
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; bottomMargin: Theme.safeBottom
        }
        prompts: [{ buttons: ["dpad"], label: Tr.tr("prompt.change") }, { buttons: ["a"], label: Tr.tr("clock.save") },
                  { buttons: ["b"], label: Tr.tr("prompt.back") }]
    }
}
