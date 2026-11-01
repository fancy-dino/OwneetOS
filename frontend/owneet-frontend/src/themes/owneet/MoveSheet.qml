// SPDX-License-Identifier: GPL-3.0-or-later
// Moving and copying games between disks ([[step-6.6-moving-games-and-media-between-disks]],
// approved demo design/demos/6.6-moving-games.html): Menu → "Move to another disk" lists the other
// disks with their free space (one without room, or read only, is off and says why; the one with
// the most room first); the confirmation with the switch "Remove the original after the copy is
// checked" (off: a copy); then the steps, each ticked when done: the copy (Cancel, or B, asks
// "Stop?" first and the copy waits), the check of the copy, and only then the original removed
// (owneetd: /v1/moves, "moves.changed"). A copy that fails its check is removed and the original
// stays untouched: "The copy did not work" with Close (selected) and Try again; with several games
// a summary. A Steam game: Steam moves it (its storage settings). A USB disk with games plugged
// in: "Play from here / Copy to the console", then which disk (the originals stay).
import QtQuick 2.15
import "foundation"

Dialog {
    id: root
    property Item theme: null            // diskName(), notify(), openSteam(), readGamesQuietly()
    property var toast: null
    property string mode: ""             // disks, confirm, run, ask, failed, steam, plug
    property var games: []               // [{ dir, title, size }]
    property string from: ""             // the disk they are on ("internal" or its folder)
    property string to: ""
    property bool remove: true
    property bool plugged: false         // the USB disk's games, copied to the console
    property var volumes: []
    property var st: ({ state: "" })     // owneetd's GET /v1/moves
    readonly property real need: games.reduce((a, g) => a + g.size, 0)
    readonly property string fromName: theme ? theme.diskName(from) : ""
    readonly property string toName: theme ? theme.diskName(to) : ""
    readonly property var current: games[Math.min(st.index || 0, Math.max(0, games.length - 1))] || { title: "" }

    contentWidth: Theme.px(620)

    // why a disk cannot take the games ("" when it can)
    function why(v) {
        if (v.readOnly)
            return Tr.tr(v.why === "windows" ? "move.readonly" : "disk.note.readonly");
        if (v.free < need)
            return Tr.tr("move.nospace", { size: GameInfo.sizeAuto(need) });
        return "";
    }
    function targets() { return volumes.filter(v => v.key !== from); }

    // ---- Menu → Move to another disk
    function openMove(game) {
        const dir = String(game.extra["disk.dir"]);
        games = [{ dir: dir, title: game.title, size: diskGames.folderSize(dir) }];
        from = GameInfo.diskRoot(game);
        plugged = false;
        remove = true;
        showDisks();
    }
    // a USB disk with games plugged in (key: its folder)
    function openPlug(key, list) {
        from = key;
        plugged = true;
        remove = false;
        games = list.map(g => ({ dir: String(g.extra["disk.dir"]), title: g.title, size: diskGames.folderSize(String(g.extra["disk.dir"])) }));
        mode = "plug";
        title = Tr.tr("move.plug.title", { n: games.length });
        text = Tr.tr("move.plug.text");
        buttons = [{ text: Tr.tr("move.plug.here"), style: "primary", action: () => root.close() },
                   { text: Tr.tr("move.plug.copy"), action: () => root.showDisks() }];
        prompts = defaultPrompts;
        backAction = null;
        initialItem = null;
        defaultIndex = 0;
        open();
    }
    function openSteam(game) {
        mode = "steam";
        title = Tr.tr("move.steam.title");
        text = Tr.tr("move.steam.text", { title: game.title });
        buttons = [{ text: Tr.tr("move.steam.open"), style: "primary", action: () => { root.close(true); if (root.theme) root.theme.openSteam("storage"); } },
                   { text: Tr.tr("prompt.cancel"), action: () => root.close() }];
        prompts = defaultPrompts;
        backAction = null;
        initialItem = null;
        defaultIndex = 0;
        open();
    }
    function showDisks() {
        volumes = storage.volumes();
        mode = "disks";
        title = plugged ? Tr.tr("move.plug.where", { n: games.length, size: GameInfo.sizeAuto(need) })
                        : Tr.tr("move.title", { title: games[0].title });
        text = plugged ? Tr.tr("move.plug.where.text")
                       : Tr.tr("move.where", { size: GameInfo.sizeAuto(need), disk: fromName });
        buttons = [];
        prompts = defaultPrompts;
        backAction = null;
        // the usable disk with the most room first
        const best = targets().filter(v => why(v) === "").sort((a, b) => b.free - a.free)[0];
        initialItem = null;
        open(visible);
        Qt.callLater(() => {
            const rows = Nav.navigables(diskList);
            const row = rows.find(r => best && r.key === best.key) || rows[0];
            if (row) focusItem(row);
        });
    }
    function confirm(key) {
        to = key;
        mode = "confirm";
        title = Tr.tr("move.confirm", { title: games[0].title, disk: toName });
        text = Tr.tr("move.confirm.text", { size: GameInfo.sizeAuto(need) });
        buttons = [{ text: Tr.tr("move.start"), style: "primary", action: () => root.start() },
                   { text: Tr.tr("move.back"), action: () => root.showDisks() }];
        backAction = () => root.showDisks();
        initialItem = null;
        defaultIndex = 0;
        Qt.callLater(focusDefault);
    }
    function start() {
        st = { state: "copy", index: 0, percent: 0, failed: [], moved: [] };
        showRun();
        owneetd.post("/v1/moves", { games: games.map(g => ({ dir: g.dir, title: g.title })), to: to, remove: remove }, (status, data) => {
            if (status === 202)
                return;
            Nav.feedback("error");
            root.close(true);
            const code = data && data.error ? data.error.code : "";
            if (root.toast) root.toast.show(Tr.tr(code === "moves.no_space" ? "move.error.space" : "move.error"));
        });
    }

    // ---- The steps
    function showRun() {
        mode = "run";
        text = "";
        title = plugged && games.length > 1 ? Tr.tr("move.plug.copying", { title: current.title, i: (st.index || 0) + 1, n: games.length })
              : Tr.tr(remove ? "move.title" : "copy.title", { title: current.title });
        const copying = st.state === "copy";
        buttons = copying ? [{ text: Tr.tr("prompt.cancel"), action: () => root.ask() }] : [];
        prompts = copying ? [{ buttons: ["a"], label: Tr.tr("prompt.cancel") }] : [];
        // B: during the copy it asks; the check and the removal are never stopped half way
        backAction = () => { if (root.st.state === "copy") root.ask(); else Nav.feedback("edge"); };
        initialItem = null;
        defaultIndex = 0;
        if (copying) Qt.callLater(focusDefault);
    }
    function ask() {
        owneetd.post("/v1/moves/pause", {}, () => {});
        Nav.feedback("sheet-open");
        mode = "ask";
        title = Tr.tr("move.cancel.ask");
        text = Tr.tr("move.cancel.text", { title: current.title, disk: fromName });
        buttons = [{ text: Tr.tr("move.cancel.keep"), style: "primary", action: () => root.keepGoing() },
                   { text: Tr.tr("move.cancel.stop"), action: () => owneetd.post("/v1/moves/stop", {}, () => {}) }];
        prompts = [{ buttons: ["a"], label: Tr.tr("prompt.select") }, { buttons: ["b"], label: Tr.tr("move.cancel.keep") }];
        backAction = () => root.keepGoing();
        defaultIndex = 0;
        Qt.callLater(focusDefault);
    }
    function keepGoing() {
        owneetd.post("/v1/moves/resume", {}, () => {});
        Nav.feedback("sheet-close");
        showRun();
    }
    readonly property var failedTitles: st.failed || []
    property string failText: ""
    function showFailed() {
        mode = "failed";
        Nav.feedback("error");
        const several = games.length > 1;
        title = several ? Tr.tr("move.fail.some", { ok: games.length - failedTitles.length, n: games.length, to: toName })
                        : Tr.tr("move.fail.title", { title: failedTitles[0] });
        text = "";                        // under the steps, as in the demo
        failText = (several ? Tr.tr("move.fail.some.text", { titles: failedTitles.join(", ") }) + " " : "")
                 + Tr.tr(failedTitles.length > 1 ? "move.fail.many.text" : "move.fail.text", { from: fromName, to: toName });
        buttons = [{ text: Tr.tr("move.fail.close"), style: "primary", action: () => root.finish() },
                   { text: Tr.tr("move.fail.retry"), action: () => root.retry() }];
        prompts = [{ buttons: ["a"], label: Tr.tr("prompt.select") }, { buttons: ["b"], label: Tr.tr("move.fail.close") }];
        backAction = () => root.finish();
        defaultIndex = 0;
        Qt.callLater(focusDefault);
    }
    function retry() {
        games = games.filter(g => failedTitles.indexOf(g.title) >= 0);
        start();
    }
    // the end: the games read again where they are now, and the notice
    function finish() {
        const ok = games.length - failedTitles.length;
        close(true);
        if (root.theme) {
            root.theme.readGamesQuietly();
            if (ok > 0 && failedTitles.length === 0) {
                if (plugged)
                    root.theme.notify({ key: "move.plug.done", args: { n: games.length, disk: toName }, detailKey: "move.plug.where.text", icon: "check", kind: "info" });
                else
                    root.theme.notify({ key: remove ? "move.done" : "copy.done", args: { title: games[0].title, disk: toName },
                                        detailKey: remove ? "move.done.text" : "copy.done.text", icon: "check", kind: "info" });
            }
        }
    }
    Connections {
        target: owneetd
        function onEvent(type, data) {
            if (type !== "moves.changed" || !root.visible || (root.mode !== "run" && root.mode !== "ask"))
                return;
            const before = root.st.state, index = root.st.index;
            root.st = data;
            if (data.state === "cancelled") {
                root.close();
                if (root.toast) root.toast.show(Tr.tr("move.cancelled", { title: root.current.title, disk: root.fromName }));
            } else if (data.state === "done") {
                if ((data.failed || []).length > 0) root.showFailed();
                else { root.mode = "run"; doneTimer.start(); }
            } else if (root.mode === "run" && (data.state !== before || data.index !== index)) {
                root.showRun();
            }
        }
    }
    Timer { id: doneTimer; interval: 700; onTriggered: root.finish() }   // every step ticked, a moment

    // ---- The window's own content
    Column {                              // disks
        id: diskList
        visible: root.mode === "disks"
        width: Theme.px(620)
        anchors.horizontalCenter: parent.horizontalCenter
        spacing: Theme.px(12)
        Repeater {
            model: root.mode === "disks" ? root.targets() : []
            Rectangle {
                readonly property string key: modelData.key
                readonly property string reason: root.why(modelData)
                readonly property bool navigable: reason === ""
                signal activated()
                onActivated: root.confirm(modelData.key)
                width: parent.width
                height: rowColumn.implicitHeight + Theme.px(24)
                radius: Theme.radiusM
                color: Theme.surface
                border.color: Theme.line
                border.width: Math.max(1, Theme.px(1))
                opacity: navigable ? 1 : 0.5
                Column {
                    id: rowColumn
                    anchors { left: parent.left; right: parent.right; leftMargin: Theme.px(16); rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                    spacing: Theme.px(6)
                    Item {
                        width: parent.width
                        height: Math.max(rowName.implicitHeight, rowFree.implicitHeight)
                        Text {
                            id: rowName
                            text: root.theme ? root.theme.diskName(modelData.key) : ""
                            color: Theme.fg
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(17)
                        }
                        Text {
                            id: rowFree
                            anchors { right: parent.right; baseline: rowName.baseline }
                            text: Tr.tr("move.free", { free: GameInfo.size(modelData.free) })
                            color: Theme.muted
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(14.5)
                        }
                    }
                    Rectangle {
                        width: parent.width; height: Theme.px(6); radius: height / 2
                        color: Theme.raised
                        Rectangle {
                            width: parent.width * Math.min(1, 1 - modelData.free / modelData.total)
                            height: parent.height; radius: height / 2
                            color: Theme.accent
                        }
                    }
                    Text {
                        visible: text !== ""
                        width: parent.width
                        text: parent.parent.reason
                        color: Theme.muted
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(12.5)
                        wrapMode: Text.Wrap
                    }
                }
                FocusFrame {}
            }
        }
    }
    Column {                              // the confirmation's switch
        visible: root.mode === "confirm"
        width: Theme.px(620)
        anchors.horizontalCenter: parent.horizontalCenter
        spacing: Theme.px(10)
        SettingRow {
            width: parent.width
            text: Tr.tr("move.remove")
            checked: root.remove
            onActivated: root.remove = !root.remove
        }
        Text {
            width: parent.width
            text: root.remove ? Tr.tr("move.remove.on", { to: root.toName }) : Tr.tr("move.remove.off", { from: root.fromName })
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(14.5)
            wrapMode: Text.Wrap
        }
    }
    Column {                              // the steps
        visible: root.mode === "run" || (root.mode === "failed" && root.games.length === 1)
        width: Theme.px(620)
        anchors.horizontalCenter: parent.horizontalCenter
        spacing: Theme.px(10)
        Repeater {
            model: [["copy", Tr.tr("move.step.copy", { disk: root.toName })], ["check", Tr.tr("move.step.check")],
                    ["remove", root.remove ? Tr.tr("move.step.remove", { disk: root.fromName }) : Tr.tr("move.step.keep", { disk: root.fromName })]]
                   .slice(0, root.mode === "failed" ? 2 : 3)
            Column {
                readonly property string step: modelData[0]
                readonly property var order: ["copy", "check", "remove", "end"]
                readonly property string phase: root.mode === "failed" ? "failed" : root.st.state === "done" ? "end" : root.st.state
                readonly property string state: phase === "failed" ? (step === "check" ? "bad" : "done")
                    : order.indexOf(step) < order.indexOf(phase) ? "done" : step === phase ? "now" : ""
                width: parent.width
                spacing: Theme.px(6)
                Row {
                    width: parent.width
                    spacing: Theme.px(10)
                    Rectangle {
                        width: Theme.fs(22); height: width; radius: width / 2
                        anchors.verticalCenter: parent.verticalCenter
                        color: parent.parent.state === "done" ? Theme.accent : parent.parent.state === "bad" ? Theme.fg : "transparent"
                        border.color: parent.parent.state === "done" ? Theme.accent : parent.parent.state === "bad" ? Theme.fg
                                    : parent.parent.state === "now" ? Theme.accent : Theme.line
                        border.width: Theme.px(2)
                        Text {
                            anchors.centerIn: parent
                            text: parent.parent.parent.state === "done" ? "✓" : parent.parent.parent.state === "bad" ? "✕" : ""
                            color: parent.parent.parent.state === "bad" ? Theme.bg : Theme.onAccent
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(13)
                        }
                        RotationAnimation on rotation {
                            running: parent.parent.state === "now" && !Theme.reduceMotion
                            from: 0; to: 360; duration: 1000; loops: Animation.Infinite
                        }
                    }
                    Text {
                        anchors.verticalCenter: parent.verticalCenter
                        width: parent.width - Theme.fs(22) - Theme.px(80)
                        text: modelData[1]
                        color: parent.parent.state === "" ? Theme.muted : Theme.fg
                        font.family: Theme.textFont
                        font.weight: parent.parent.state === "now" || parent.parent.state === "bad" ? Font.Medium : Font.Normal
                        font.pixelSize: Theme.fs(17)
                        elide: Text.ElideRight
                    }
                    Text {
                        anchors.verticalCenter: parent.verticalCenter
                        visible: parent.parent.step === "copy" && parent.parent.state === "now"
                        text: (root.st.percent || 0) + "%"
                        color: Theme.fg
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                    }
                }
                Rectangle {
                    visible: parent.step === "copy" && parent.state === "now"
                    x: Theme.fs(22) + Theme.px(10)
                    width: parent.width - x; height: Theme.px(8); radius: height / 2
                    color: Theme.raised
                    Rectangle {
                        width: parent.width * Math.min(1, (root.st.percent || 0) / 100)
                        height: parent.height; radius: height / 2
                        color: Theme.accent
                    }
                }
            }
        }
    }
    Text {                                // what did not work, under the steps
        visible: root.mode === "failed"
        width: Theme.px(620)
        anchors.horizontalCenter: parent.horizontalCenter
        horizontalAlignment: Text.AlignHCenter
        text: root.failText
        color: Theme.muted
        font.family: Theme.textFont
        font.pixelSize: Theme.fs(17)
        wrapMode: Text.Wrap
    }
}
