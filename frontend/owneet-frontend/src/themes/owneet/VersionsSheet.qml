// SPDX-License-Identifier: GPL-3.0-or-later
// Settings → System → "Other versions" ([[step-7.1b-going-back-to-any-version]], approved demo
// design/demos/7.1-updates.html): this version and the older ones of the signed list, newest first,
// one per line in a list that scrolls with the selection (the right stick scrolls it too); this
// version is off, the spare copy's says "Previous, on this console" and goes to "Go back to the
// previous version". Another one asks first, Cancel selected: personal data do not go back, an old
// version has old security problems; its download size appears once owneetd has worked it out
// (POST /v1/system-update/size). "Download and go back" is an update in reverse
// (POST /v1/system-update/go-to), followed by the update's own window.
import QtQuick 2.15
import "foundation"

Dialog {
    id: root
    property Item theme: null            // systemUpdate, goBackVersion(), showSystemUpdate(), updateError()
    property string mode: ""             // list, confirm
    property string version: ""          // the one chosen
    readonly property var st: theme && theme.systemUpdate ? theme.systemUpdate : ({ releases: [] })
    readonly property string spare: st.previous ? st.previous.version || "" : ""
    readonly property string size: {
        const g = st.goto;
        return g && g.version === version && !g.sizing && !g.error ? GameInfo.sizeAuto(g.size) : "";
    }

    scrollTarget: mode === "list" ? flick : null
    onMoved: if (mode === "list") ensureVisible(item)

    function shortDate(date) {           // "2026-10-18" in the language's short date (18/10/2026)
        const p = String(date).split("-");
        const d = new Date(+p[0], +p[1] - 1, +p[2]);
        return Qt.formatDate(d, Qt.locale(Tr.language).dateFormat(Locale.ShortFormat).replace(/y+/, "yyyy"));
    }
    function ensureVisible(item) {
        const p = item.mapToItem(list, 0, 0);
        const pad = Theme.px(14);
        if (p.y - pad < flick.contentY)
            flick.contentY = Math.max(0, p.y - pad);
        else if (p.y + item.height + pad > flick.contentY + flick.height)
            flick.contentY = Math.min(flick.contentHeight - flick.height, p.y + item.height + pad - flick.height);
    }

    function openList() {
        mode = "list";
        version = "";
        title = Tr.tr("upd.other.title");
        text = "";
        buttons = [];
        contentWidth = Theme.px(780);
        prompts = [{ buttons: ["a"], label: Tr.tr("prompt.select") }, { buttons: ["b"], label: Tr.tr("prompt.back") },
                   { buttons: ["rs"], label: Tr.tr("prompt.scroll") }];
        backAction = null;
        initialItem = null;
        flick.contentY = 0;
        open();
        Qt.callLater(() => {
            const rows = Nav.navigables(list);
            if (rows[0]) focusItem(rows[0]);
        });
    }
    function choose(v) {
        if (v === spare) {
            close(true);
            if (theme) theme.goBackVersion();
            return;
        }
        version = v;
        mode = "confirm";
        title = Tr.tr("upd.old.confirm", { v: v });
        contentWidth = Theme.px(640);
        buttons = [{ text: Tr.tr("upd.old.go"), action: () => root.goTo() },
                   { text: Tr.tr("prompt.cancel"), style: "primary", action: () => root.close() }];
        defaultIndex = 1;
        prompts = [{ buttons: ["a"], label: Tr.tr("prompt.select") }, { buttons: ["b"], label: Tr.tr("prompt.cancel") }];
        Qt.callLater(focusDefault);
        owneetd.post("/v1/system-update/size", { version: v });
    }
    function goTo() {
        const v = version;
        const bytes = st.goto && st.goto.version === v ? st.goto.size || 0 : 0;
        close(true);
        owneetd.post("/v1/system-update/go-to", { version: v }, (status, data) => {
            const code = data && data.error ? data.error.code : "";
            if (status === 202 || code === "system_update.busy") return;
            if (theme) theme.updateError("upd.error.download");
        });
        if (theme) theme.showSystemUpdate(Object.assign({}, st, { state: "downloading", percent: 0, version: v, size: bytes }), true);
    }

    Flickable {                           // the versions
        id: flick
        visible: root.mode === "list"
        anchors.horizontalCenter: parent.horizontalCenter
        width: Theme.px(780)
        height: Math.min(contentHeight, Theme.px(360))
        clip: true
        contentHeight: list.height + Theme.px(24)
        boundsBehavior: Flickable.StopAtBounds
        Behavior on contentY { enabled: !Theme.reduceMotion; NumberAnimation { duration: Theme.motionMs; easing.type: Easing.OutCubic } }
        Column {
            id: list
            x: Theme.px(14); y: Theme.px(12)
            width: flick.width - Theme.px(28)
            spacing: Theme.px(10)
            Repeater {
                model: root.mode === "list" ? root.st.releases : []
                Rectangle {
                    readonly property bool navigable: modelData.version !== root.st.current
                    signal activated()
                    onActivated: root.choose(modelData.version)
                    width: list.width
                    height: name.implicitHeight + Theme.px(22)
                    radius: Theme.px(14)
                    color: Theme.raised
                    opacity: navigable ? 1 : 0.55
                    FitText {
                        id: name
                        anchors { left: parent.left; leftMargin: Theme.px(26); right: tag.left; rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                        text: "OwneetOS " + modelData.version
                        color: Theme.fg
                        font.family: Theme.textFont
                        font.weight: Font.Medium
                        font.pixelSize: Theme.fs(17)
                    }
                    Text {
                        id: tag
                        anchors { right: date.left; rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                        text: modelData.version === root.st.current ? Tr.tr("upd.this")
                            : modelData.version === root.spare ? Tr.tr("upd.onconsole") : ""
                        color: Theme.muted
                        font.family: Theme.textFont
                        font.weight: Font.Medium
                        font.pixelSize: Theme.fs(14.5)
                    }
                    Text {
                        id: date
                        anchors { right: parent.right; rightMargin: Theme.px(26); verticalCenter: parent.verticalCenter }
                        width: Theme.px(130)
                        text: root.shortDate(modelData.date)
                        color: Theme.muted
                        font.family: Theme.textFont
                        font.weight: Font.Medium
                        font.pixelSize: Theme.fs(17)
                    }
                    FocusFrame {}
                }
            }
        }
    }
    Column {                              // the warning
        visible: root.mode === "confirm"
        width: parent.width
        spacing: Theme.px(14)
        Text {
            width: parent.width
            horizontalAlignment: Text.AlignHCenter
            text: Tr.tr("upd.old.data")
            color: Theme.fg
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(17)
            wrapMode: Text.Wrap
        }
        Text {
            width: parent.width
            horizontalAlignment: Text.AlignHCenter
            text: Tr.tr("upd.old.risk")
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(17)
            wrapMode: Text.Wrap
        }
        Text {                            // empty until owneetd knows the size (it keeps its room)
            width: parent.width
            horizontalAlignment: Text.AlignHCenter
            text: root.size !== "" ? Tr.tr("upd.size", { size: root.size }) : " "
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(14.5)
        }
    }
}
