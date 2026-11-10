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
// in: "Play from here / Copy to the console", then which games (approved demo 6.6b: a switch per
// game and "All games", all on at first; the button below copies the ones that are on, selected at
// first so that one A copies them all; a disk with one game skips it), then which disk (the
// originals stay); B goes back one window. Approved demo 6.6c: next to "Copy", each game has a
// "Remove the original" switch (off at first, only for a copied game, never on a read-only disk):
// its original is removed from the USB disk once its copy has passed the check. Approved demo
// 6.6d: the check and the removal of the original show their own percentage and bar, as the copy;
// a copy that keeps the original shows no step for it; copying several games, "Stop?" offers
// "Stop this game" (the next games go on) and "Stop all" while games are left after this one.
import QtQuick 2.15
import "foundation"

Dialog {
    id: root
    property Item theme: null            // diskName(), notify(), openSteam(), readGamesQuietly()
    property var toast: null
    property string mode: ""             // disks, confirm, run, ask, failed, steam, plug, choose
    property var games: []               // [{ dir, title, size }]
    property string from: ""             // the disk they are on ("internal" or its folder)
    property string to: ""
    property bool remove: true
    property bool plugged: false         // the USB disk's games, copied to the console
    property var plugGames: []           // all the plugged-in disk's games (games: the ones chosen)
    property var chosen: ({})            // dir → true: the games switched on in "Which games to copy?"
    property var removing: ({})          // dir → true: their originals removed after the check (6.6c)
    property bool fromReadOnly: false    // the plugged-in disk cannot lose its originals
    property string fromWhy: ""          // why it is read only ("windows": not shut down completely)
    property var volumes: []
    property var st: ({ state: "" })     // owneetd's GET /v1/moves
    readonly property real need: games.reduce((a, g) => a + g.size, 0)
    readonly property string fromName: theme ? theme.diskName(from) : ""
    readonly property string toName: theme ? theme.diskName(to) : ""
    readonly property var current: games[Math.min(st.index || 0, Math.max(0, games.length - 1))] || { title: "" }
    // whether the original of the game being copied is removed after its check
    readonly property bool currentRemove: plugged ? removing[current.dir] === true : remove

    contentWidth: Theme.px(mode === "choose" ? 720 : 620)   // 6.6c: room for the two headings
    scrollTarget: mode === "choose" ? chooseFlick : null
    // the selected switch stays in sight when the games scroll (as VersionsSheet)
    onMoved: if (mode === "choose" && Nav.navigables(chooseList).indexOf(item) >= 0) {
        const y = item.mapToItem(chooseList, 0, 0).y, pad = chooseFlick.pad;
        if (y < chooseFlick.contentY)
            chooseFlick.contentY = Math.max(0, y);
        else if (y + item.height + 2 * pad > chooseFlick.contentY + chooseFlick.height)
            chooseFlick.contentY = Math.min(chooseFlick.contentHeight - chooseFlick.height, y + item.height + 2 * pad - chooseFlick.height);
    }

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
        plugGames = list.map(g => ({ dir: String(g.extra["disk.dir"]), title: g.title, size: diskGames.folderSize(String(g.extra["disk.dir"])) }));
        games = plugGames;
        const all = {};
        plugGames.forEach(g => all[g.dir] = true);
        chosen = all;
        removing = ({});
        const v = storage.volumes().find(v => v.key === key);
        fromReadOnly = !!(v && v.readOnly);
        fromWhy = v && v.why ? String(v.why) : "";
        showPlug();
    }
    function showPlug() {
        mode = "plug";
        title = Tr.tr("move.plug.title", { n: plugGames.length });
        text = Tr.tr("move.plug.text");
        buttons = [{ text: Tr.tr("move.plug.here"), style: "primary", action: () => root.close() },
                   { text: Tr.tr("move.plug.copy"), action: () => plugGames.length > 1 ? root.showChoose() : root.showDisks() }];
        prompts = defaultPrompts;
        backAction = null;
        initialItem = null;
        defaultIndex = 0;
        open(visible);
        if (visible) Qt.callLater(focusDefault);
    }
    // ---- 6.6b / 6.6c: which games to copy, and whose originals go
    readonly property var chosenGames: plugGames.filter(g => chosen[g.dir])
    readonly property bool allRemoved: chosenGames.length > 0 && chosenGames.every(g => removing[g.dir])
    // what happens to the originals (under the list, and in the disk window)
    function originalsNote(list) {
        if (fromReadOnly)
            return Tr.tr(fromWhy === "windows" ? "move.plug.readonly" : "move.plug.readonly.other");
        const r = list.filter(g => removing[g.dir]);
        return r.length === 0 ? Tr.tr("move.plug.where.text")
             : r.length === 1 ? Tr.tr("move.plug.remove.one", { title: r[0].title })
             : Tr.tr("move.plug.remove.many", { n: r.length });
    }
    function goButton() {
        const c = chosenGames;
        const text = c.length === 0 ? Tr.tr("move.plug.go.none")
                   : c.length === 1 ? Tr.tr("move.plug.go.one", { title: c[0].title, size: GameInfo.sizeAuto(c[0].size) })
                   : Tr.tr("move.plug.go", { n: c.length, size: GameInfo.sizeAuto(c.reduce((a, g) => a + g.size, 0)) });
        return [{ text: text, style: "primary", dim: c.length === 0, action: () => root.chooseDone() }];
    }
    function showChoose() {
        chooseFlick.contentY = 0;
        mode = "choose";
        title = Tr.tr("move.plug.choose");
        text = Tr.tr("move.plug.choose.text");
        if (fromReadOnly) removing = ({});
        buttons = goButton();
        prompts = defaultPrompts;
        backAction = () => root.showPlug();
        initialItem = null;
        defaultIndex = 0;
        Qt.callLater(focusDefault);   // the button: one A copies them all
    }
    // A on a switch: it changes in place, the selection stays on it. A game no longer copied keeps
    // no "Remove the original".
    function toggle(dir) {
        const c = Object.assign({}, chosen), r = Object.assign({}, removing);
        if (dir === "") {             // All games
            const all = chosenGames.length === plugGames.length;
            plugGames.forEach(g => { c[g.dir] = !all; if (all) r[g.dir] = false; });
        } else {
            c[dir] = !c[dir];
            if (!c[dir]) r[dir] = false;
        }
        chosen = c;
        removing = r;
        buttons = goButton();
    }
    function toggleRemove(dir) {
        if (fromReadOnly)
            return;
        const r = Object.assign({}, removing);
        if (dir === "") {             // All games: the copied ones
            const all = allRemoved;
            chosenGames.forEach(g => r[g.dir] = !all);
        } else if (chosen[dir]) {
            r[dir] = !r[dir];
        }
        removing = r;
    }
    function chooseDone() {
        if (chosenGames.length === 0) {
            Nav.feedback("edge");
            return;
        }
        games = chosenGames;
        showDisks();
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
        title = plugged ? (games.length === 1 ? Tr.tr("move.plug.where.one", { title: games[0].title, size: GameInfo.sizeAuto(need) })
                                              : Tr.tr("move.plug.where", { n: games.length, size: GameInfo.sizeAuto(need) }))
                        : Tr.tr("move.title", { title: games[0].title });
        text = plugged ? originalsNote(games)
                       : Tr.tr("move.where", { size: GameInfo.sizeAuto(need), disk: fromName });
        buttons = [];
        prompts = defaultPrompts;
        // a plugged-in disk: B goes back one window (demo 6.6b)
        backAction = !plugged ? null : plugGames.length > 1 ? () => root.showChoose() : () => root.showPlug();
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
        // a plugged-in disk: each game's own choice (6.6c); otherwise one for all
        owneetd.post("/v1/moves", { games: games.map(g => ({ dir: g.dir, title: g.title, remove: plugged && removing[g.dir] === true })),
                                    to: to, remove: remove }, (status, data) => {
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
              : Tr.tr(currentRemove ? "move.title" : "copy.title", { title: current.title });
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
        // 6.6d: games left after this one: stop only this game, or all
        const more = (st.index || 0) + 1 < games.length;
        text = Tr.tr(more ? "move.cancel.text.many" : "move.cancel.text", { title: current.title, disk: fromName });
        const stopAll = () => owneetd.post("/v1/moves/stop", {}, () => {});
        buttons = [{ text: Tr.tr("move.cancel.keep"), style: "primary", action: () => root.keepGoing() }].concat(more
            ? [{ text: Tr.tr("move.cancel.one"), action: () => root.stopOne() },
               { text: Tr.tr("move.cancel.all"), action: stopAll }]
            : [{ text: Tr.tr("move.cancel.stop"), action: stopAll }]);
        prompts = [{ buttons: ["a"], label: Tr.tr("prompt.select") }, { buttons: ["b"], label: Tr.tr("move.cancel.keep") }];
        backAction = () => root.keepGoing();
        defaultIndex = 0;
        Qt.callLater(focusDefault);
    }
    // only this game's copy: its partial copy removed, the next game goes on (6.6d)
    function stopOne() {
        const game = current.title;
        owneetd.post("/v1/moves/skip", {}, () => {});
        Nav.feedback("sheet-close");
        if (root.toast) root.toast.show(Tr.tr("move.cancelled", { title: game, disk: fromName }));
        showRun();
    }
    function keepGoing() {
        owneetd.post("/v1/moves/resume", {}, () => {});
        Nav.feedback("sheet-close");
        showRun();
    }
    readonly property var failedTitles: st.failed || []
    readonly property var skippedTitles: st.skipped || []     // stopped alone by the player (6.6d)
    property string failText: ""
    function showFailed() {
        mode = "failed";
        Nav.feedback("error");
        const several = games.length > 1;
        title = several ? Tr.tr("move.fail.some", { ok: games.length - failedTitles.length - skippedTitles.length, n: games.length, to: toName })
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
        // the games copied: not the ones stopped alone (6.6d)
        const copied = games.filter(g => skippedTitles.indexOf(g.title) < 0 && failedTitles.indexOf(g.title) < 0);
        close(true);
        if (root.theme) {
            root.theme.readGamesQuietly();
            if (copied.length > 0 && failedTitles.length === 0) {
                // what happened to the originals (6.6c)
                const gone = copied.filter(g => removing[g.dir]);
                const detail = !plugged ? "" : gone.length === 0 ? { key: "move.plug.where.text", args: {} }
                             : gone.length === 1 ? { key: "move.plug.removed.one", args: { title: gone[0].title } }
                             : { key: "move.plug.removed.many", args: { count: gone.length } };
                if (plugged && copied.length > 1)
                    root.theme.notify({ key: "move.plug.done", args: Object.assign({ n: copied.length, disk: toName }, detail.args), detailKey: detail.key, icon: "check", kind: "info" });
                else if (plugged)
                    root.theme.notify({ key: "copy.done", args: Object.assign({ title: copied[0].title, disk: toName }, detail.args), detailKey: detail.key, icon: "check", kind: "info" });
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
                // a plugged-in disk's games are copied at once, the originals kept (approved demo 6.6)
                onActivated: if (root.plugged) { root.to = modelData.key; root.start(); } else root.confirm(modelData.key)
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
    Flickable {                           // which games to copy (6.6b): many games scroll in the window
        id: chooseFlick
        visible: root.mode === "choose"
        width: Theme.px(720) + 2 * pad   // the rows 720 wide; the margin keeps the selection's frame whole
        height: Math.min(contentHeight, Theme.px(360))
        anchors.horizontalCenter: parent.horizontalCenter
        clip: true
        readonly property real pad: Theme.px(10)
        contentHeight: chooseList.height + 2 * pad
        boundsBehavior: Flickable.StopAtBounds
        Behavior on contentY { enabled: !Theme.reduceMotion; NumberAnimation { duration: Theme.motionMs; easing.type: Easing.OutCubic } }
        Column {
            id: chooseList
            x: chooseFlick.pad; y: chooseFlick.pad
            width: Theme.px(720)
            spacing: Theme.px(10)
            readonly property real copyColumn: Theme.px(110)
            readonly property real removeColumn: Theme.px(190)
            readonly property real gap: Theme.px(8)
            Item {                        // the headings, over the switches
                width: parent.width
                height: copyHead.implicitHeight
                Text {
                    id: copyHead
                    x: parent.width - Theme.px(16) - chooseList.removeColumn - chooseList.gap - chooseList.copyColumn
                    width: chooseList.copyColumn
                    horizontalAlignment: Text.AlignHCenter
                    text: Tr.tr("move.plug.col.copy")
                    color: Theme.muted
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(12.5)
                }
                Text {
                    x: parent.width - Theme.px(16) - chooseList.removeColumn
                    width: chooseList.removeColumn
                    horizontalAlignment: Text.AlignHCenter
                    text: Tr.tr("move.plug.col.remove")
                    color: Theme.muted
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(12.5)
                    elide: Text.ElideRight
                }
            }
            Repeater {
                model: root.mode === "choose" ? [{ dir: "", title: Tr.tr("move.plug.all"), size: root.plugGames.reduce((a, g) => a + g.size, 0) }].concat(root.plugGames) : []
                Rectangle {
                    readonly property bool all: modelData.dir === ""
                    readonly property bool copied: all ? root.chosenGames.length === root.plugGames.length : root.chosen[modelData.dir] === true
                    width: chooseList.width
                    height: Math.max(label.implicitHeight, copySwitch.implicitHeight) + Theme.px(12)
                    radius: Theme.radiusS + Theme.px(2)
                    color: Theme.surface
                    border.color: Theme.line
                    border.width: Math.max(1, Theme.px(1))
                    Row {
                        anchors { left: parent.left; leftMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                        width: parent.width - Theme.px(32) - chooseList.copyColumn - chooseList.removeColumn - 2 * chooseList.gap
                        spacing: Theme.px(6)
                        Text {
                            id: label
                            width: Math.min(implicitWidth, parent.width - sizeText.implicitWidth - parent.spacing)
                            text: modelData.title
                            color: Theme.fg
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(14.5)
                            elide: Text.ElideRight
                        }
                        Text {
                            id: sizeText
                            anchors.baseline: label.baseline
                            text: "· " + GameInfo.sizeAuto(modelData.size)
                            color: Theme.muted
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(12.5)
                        }
                    }
                    SwitchCell {
                        id: copySwitch
                        anchors.verticalCenter: parent.verticalCenter
                        x: parent.width - Theme.px(16) - chooseList.removeColumn - chooseList.gap - chooseList.copyColumn + (chooseList.copyColumn - width) / 2
                        checked: parent.copied
                        onActivated: root.toggle(modelData.dir)
                    }
                    SwitchCell {
                        anchors.verticalCenter: parent.verticalCenter
                        x: parent.width - Theme.px(16) - chooseList.removeColumn + (chooseList.removeColumn - width) / 2
                        checked: parent.all ? root.allRemoved : root.removing[modelData.dir] === true
                        available: !root.fromReadOnly && (parent.all ? root.chosenGames.length > 0 : parent.copied)
                        onActivated: root.toggleRemove(modelData.dir)
                    }
                }
            }
        }
    }
    Text {                                // what happens to the originals (6.6c)
        visible: root.mode === "choose"
        width: Theme.px(720)
        anchors.horizontalCenter: parent.horizontalCenter
        horizontalAlignment: Text.AlignHCenter
        text: root.originalsNote(root.chosenGames)
        color: Theme.muted
        font.family: Theme.textFont
        font.pixelSize: Theme.fs(12.5)
        wrapMode: Text.Wrap
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
            // 6.6d: a kept original has no step
            model: [["copy", Tr.tr("move.step.copy", { disk: root.toName })], ["check", Tr.tr("move.step.check")]]
                   .concat(root.mode !== "failed" && root.currentRemove ? [["remove", Tr.tr("move.step.remove", { disk: root.fromName })]] : [])
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
                        visible: parent.parent.state === "now"      // 6.6d: every step's own percentage
                        text: (root.st.percent || 0) + "%"
                        color: Theme.fg
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                    }
                }
                Rectangle {
                    visible: parent.state === "now"
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
