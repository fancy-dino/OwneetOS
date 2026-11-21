// SPDX-License-Identifier: GPL-3.0-or-later
// The installer ([[step-6.4-installer-ui]], approved demo design/demos/6.4-installer.html), opened
// by "Install OwneetOS" on the USB stick ([[step-6.2-live-vs-installed-mode]]): a welcome, the
// internal disks with what is on them (owneetd: GET /v1/installer/disks; never the USB stick,
// never USB disks; too small ones off), the console's name (the console's keyboard), the warning
// (a disk with Windows or BitLocker needs its switch on), the final confirmation held for three
// seconds (nothing is erased by one press, [[user-data]]), the progress in the backend's steps
// (POST /v1/installer/start, "installer.changed"), the end and the errors in the player's words.
// The other internal disks ([[step-6.7-preparing-other-disks]], approved demo
// design/demos/6.7-installer-other-disks.html): after the disk, "Other disks" (any size, "Leave as it
// is" first), then their names ("Games", "Games 2"…, up to 16 characters, never another disk's
// name); the warning and the confirmation name every disk erased, the progress has their phase.
import QtQuick 2.15
import "foundation"

Rectangle {
    id: root
    anchors.fill: parent
    z: 92
    visible: false
    color: Theme.bg

    property Item theme: null            // its name window: renameConsole(name, done)
    signal closed()

    // ---- Where the player is
    property string screen: "welcome"    // welcome, disk, others, dnames, name, warn, confirm, progress, done, error
    readonly property var stepOf: ({ disk: "disk", others: "disk", dnames: "disk", name: "name", warn: "confirm", confirm: "confirm",
                                     progress: "install", done: "install", error: "install" })
    readonly property var steps: ["disk", "name", "confirm", "install"]
    property var disks: []
    property var disk: null              // the one chosen, as it was listed
    property string consoleName: "OwneetOS"
    property bool understood: false      // Windows or BitLocker: the switch
    // The other internal disks to prepare for games (their paths, in the list's order) and their names
    property var others: []
    property var gameNames: ({})
    readonly property var otherDisks: disk === null ? [] : disks.filter(d => d.path !== disk.path)
    readonly property var gamesDisks: disks.filter(d => others.indexOf(d.path) >= 0)
    readonly property var erased: disk === null ? [] : [disk].concat(gamesDisks)
    readonly property bool hasWindows: erased.some(d => d.windows || d.bitlocker)
    property var status: ({ state: "", percent: 0 })
    property string error: ""            // the backend's code, or owneetd's refusal

    function open() {
        screen = "welcome";
        disk = null;
        others = [];
        gameNames = {};
        understood = false;
        error = "";
        visible = true;
        refreshDisks();
        Qt.callLater(focusScreen);
    }
    function close() {
        visible = false;
        closed();
    }
    function refreshDisks() {
        owneetd.get("/v1/installer/disks", (status, data) => {
            if (status === 200) root.disks = data.disks || [];
        });
    }
    function go(name) {
        screen = name;
        Qt.callLater(focusScreen);
    }
    function back() {
        const prev = { disk: "welcome", others: "disk", dnames: "others",
                       name: others.length > 0 ? "dnames" : otherDisks.length > 0 ? "others" : "disk",
                       warn: "name", confirm: "warn", error: "confirm" }[screen];
        if (!prev) {
            Nav.feedback("edge");
            return;
        }
        hold.cancel();
        go(prev);
    }
    // back from the name window
    function refocus() {
        Qt.callLater(() => {
            nav.forceActiveFocus();
            if (nav.current && nav.current.visible) nav.focusItem(nav.current);
            else focusScreen();
        });
    }
    function focusScreen() {
        if (!visible)
            return;
        nav.forceActiveFocus();
        nav.focusFirst();
    }

    // ---- The other disks: switched on or off, then named
    function toggleOther(path) {
        const i = others.indexOf(path);
        const next = others.slice();
        if (i >= 0) next.splice(i, 1); else next.push(path);
        others = disks.map(d => d.path).filter(p => next.indexOf(p) >= 0);
        understood = false;
    }
    function afterOthers() {
        if (others.length === 0) {
            go("name");
            return;
        }
        const names = Object.assign({}, gameNames);
        for (const p in names)
            if (others.indexOf(p) < 0) delete names[p];
        gameNames = names;
        others.forEach(p => { if (!gameNames[p]) { const n = Object.assign({}, gameNames); n[p] = freeName(p); gameNames = n; } });
        go("dnames");
    }
    // A disk's name: ext4's label (1 to 16 bytes, no "/"), never the name another disk keeps or gets
    function utf8Bytes(t) { return unescape(encodeURIComponent(t)).length; }
    function nameProblem(name, path) {
        if (name === "" || name.indexOf("/") >= 0 || name.charAt(0) === ".")
            return "installer.dname.empty";
        if (utf8Bytes(name) > 16)
            return "installer.dname.long";
        const lower = name.toLowerCase();
        for (let i = 0; i < disks.length; i++) {
            const d = disks[i];
            if (d.path === path || d.path === disk.path)
                continue;
            if (others.indexOf(d.path) >= 0) {
                if ((gameNames[d.path] || "").toLowerCase() === lower)
                    return "installer.dname.taken";
            } else if ((d.partitions || []).some(p => (p.label || "").toLowerCase() === lower)) {
                return "installer.dname.taken";
            }
        }
        return "";
    }
    function freeName(path) {
        const base = Tr.tr("installer.dname.default");
        for (let n = 1; ; n++) {
            const t = n === 1 ? base : base + " " + n;
            if (nameProblem(t, path) === "")
                return t;
        }
    }
    function renameOther(path) {
        const d = disks.find(x => x.path === path);
        if (!root.theme || !d)
            return;
        root.theme.renameDisk(Tr.tr("installer.dname.change", { disk: d.model || d.path }), gameNames[path], 16,
                              name => nameProblem(name, path), name => {
            const n = Object.assign({}, root.gameNames);
            n[path] = name;
            root.gameNames = n;
        });
    }
    function diskList(list) {           // "A, B and C"
        const names = list.map(d => d.model || d.path);
        return names.length > 1 ? names.slice(0, -1).join(", ") + " " + Tr.tr("installer.and") + " " + names[names.length - 1] : names[0] || "";
    }

    // ---- The installation
    function start() {
        status = { state: "preparing", percent: 0 };
        error = "";
        Nav.feedback("confirm");
        go("progress");
        const games = gamesDisks.map(d => ({ disk: d.path, size: d.size, serial: d.serial, name: gameNames[d.path],
                                             erase_windows: (d.windows || d.bitlocker) && understood }));
        owneetd.post("/v1/installer/start", { disk: disk.path, size: disk.size, serial: disk.serial, name: consoleName,
                                               erase_windows: (disk.windows || disk.bitlocker) && understood,
                                               games: games, folder: Tr.language.indexOf("it") === 0 ? "Giochi" : "Games" }, (code, data) => {
            if (code === 202 || code === 200)
                return;
            // owneetd refused before anything was touched
            const refused = data && data.error ? data.error.code : "";
            fail(refused === "installer.disk_changed" ? "disk_changed"
                 : refused === "installer.not_usable" ? "not_usable"
                 : refused === "installer.not_live" ? "not_live"
                 : refused === "installer.windows_not_confirmed" ? "windows_not_confirmed"
                 : refused === "installer.bad_name" || refused === "installer.name_taken" ? "name_taken" : "failed");
        });
    }
    function fail(code) {
        error = code;
        Nav.feedback("error");
        screen = "error";
        Qt.callLater(focusScreen);
        if (code === "disk_changed" || code === "no_disk")
            refreshDisks();
    }
    Connections {
        target: owneetd
        function onEvent(type, data) {
            if (type !== "installer.changed" || !root.visible || root.screen !== "progress")
                return;
            root.status = data;
            if (data.state === "done") {
                Nav.feedback("notify");
                root.screen = "done";
                Qt.callLater(root.focusScreen);
            } else if (data.state === "failed") {
                root.fail(data.error || "failed");
            }
        }
    }
    // the backend's codes in the player's words; before the disk was touched, nothing was changed
    function errorText(code) {
        const same = { no_disk: "disk_changed", erase_not_confirmed: "not_confirmed",
                       windows_not_confirmed: "not_confirmed", partition_failed: "disk_failed",
                       format_failed: "disk_failed", boot_failed: "setup_failed", bad_name: "name_taken" };
        const key = "installer.err." + (same[code] || code);
        return Tr.tr(key) !== key ? Tr.tr(key) : Tr.tr("installer.err.failed");
    }
    readonly property bool untouched: ["not_live", "not_uefi", "no_disk", "disk_changed", "not_usable",
                                       "erase_not_confirmed", "windows_not_confirmed", "bad_name", "name_taken"].indexOf(error) >= 0
    readonly property var phases: others.length > 0 ? ["partitioning", "copying", "setting_up", "boot", "games"]
                                                     : ["partitioning", "copying", "setting_up", "boot"]
    readonly property int phase: Math.max(0, phases.indexOf(status.state))

    // ================= Layout =================
    Item {                                // the logo and the steps
        id: top
        anchors {
            left: parent.left; right: parent.right; top: parent.top
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; topMargin: Theme.safeTop
        }
        height: Math.max(mark.height, stepRow.height)
        Logo {
            id: mark
            anchors.verticalCenter: parent.verticalCenter
        }
        Row {
            id: stepRow
            visible: root.screen !== "welcome"
            anchors { right: parent.right; verticalCenter: parent.verticalCenter }
            spacing: Theme.px(8)
            Repeater {
                model: root.steps
                Rectangle {
                    readonly property bool current: root.stepOf[root.screen] === modelData
                    width: stepText.implicitWidth + Theme.px(28)
                    height: stepText.implicitHeight + Theme.px(10)
                    radius: height / 2
                    color: current ? Theme.surface : "transparent"
                    Text {
                        id: stepText
                        anchors.centerIn: parent
                        text: Tr.tr("installer.step." + modelData)
                        color: parent.current ? Theme.fg : Theme.muted
                        font.family: Theme.textFont
                        font.weight: parent.current ? Font.Medium : Font.Normal
                        font.pixelSize: Theme.fs(14.5)
                    }
                }
            }
        }
    }

    NavArea {
        id: nav
        anchors {
            left: parent.left; right: parent.right; top: top.bottom; bottom: prompts.top
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; topMargin: Theme.px(20); bottomMargin: Theme.px(20)
        }
        focus: root.visible
        onCancelled: root.back()
        onActiveFocusChanged: {
            if (!activeFocus)
                return;
            if (current && current.visible)
                current.forceActiveFocus();
            else
                Qt.callLater(root.focusScreen);
        }

        // ---- Welcome
        Column {
            visible: root.screen === "welcome"
            anchors.centerIn: parent
            width: Theme.px(860)
            spacing: Theme.px(18)
            Heading { text: Tr.tr("installer.welcome.title"); big: true; horizontalAlignment: Text.AlignHCenter }
            Para { text: Tr.tr("installer.welcome.text"); horizontalAlignment: Text.AlignHCenter }
            Row {
                anchors.horizontalCenter: parent.horizontalCenter
                spacing: Theme.px(16)
                topPadding: Theme.px(6)
                Button { text: Tr.tr("installer.start"); style: "primary"; onActivated: root.go("disk") }
                Button { text: Tr.tr("installer.notnow"); onActivated: root.close() }
            }
        }

        // ---- The disks
        Column {
            visible: root.screen === "disk"
            anchors.centerIn: parent
            width: Theme.px(860)
            spacing: Theme.px(18)
            Heading { text: Tr.tr("installer.disk.title") }
            Para { text: Tr.tr("installer.disk.text") }
            Column {
                width: parent.width
                spacing: Theme.px(12)
                Repeater {
                    model: root.disks
                    Rectangle {
                        readonly property bool navigable: modelData.usable
                        signal activated()
                        onActivated: {
                            if (root.disk === null || root.disk.path !== modelData.path) {
                                root.others = [];
                                root.gameNames = {};
                            }
                            root.disk = modelData;
                            root.understood = false;
                            root.go(root.otherDisks.length > 0 ? "others" : "name");
                        }
                        width: parent.width
                        height: Math.max(Theme.px(64), diskText.implicitHeight + Theme.px(28))
                        radius: Theme.radiusM
                        color: Theme.surface
                        border.color: Theme.line
                        border.width: Math.max(1, Theme.px(1))
                        opacity: modelData.usable ? 1 : 0.5
                        Column {
                            id: diskText
                            anchors { left: parent.left; leftMargin: Theme.px(18); verticalCenter: parent.verticalCenter }
                            width: parent.width * 0.6
                            spacing: Theme.px(3)
                            FitText {
                                width: parent.width
                                text: modelData.model || modelData.path
                                color: Theme.fg
                                font.family: Theme.textFont
                                font.weight: Font.Medium
                                font.pixelSize: Theme.fs(17)
                            }
                            Text {
                                text: GameInfo.size(modelData.size)
                                      + (modelData.usable ? "" : " · " + Tr.tr("installer.disk.small"))
                                color: Theme.muted
                                font.family: Theme.textFont
                                font.pixelSize: Theme.fs(14.5)
                            }
                        }
                        Row {                     // what is on it
                            anchors { right: parent.right; rightMargin: Theme.px(18); verticalCenter: parent.verticalCenter }
                            spacing: Theme.px(10)
                            Repeater {
                                model: root.contents(modelData)
                                Tag { tag: modelData }
                            }
                        }
                        FocusFrame {}
                    }
                }
            }
        }

        // ---- The other disks (6.7): each one "Leave as it is" or "Erase and prepare for games"
        Column {
            visible: root.screen === "others"
            anchors.centerIn: parent
            width: Theme.px(860)
            spacing: Theme.px(18)
            Heading { text: Tr.tr("installer.others.title") }
            Para { text: Tr.tr("installer.others.text") }
            Column {
                width: parent.width
                spacing: Theme.px(12)
                Repeater {
                    model: root.screen === "others" ? root.otherDisks : []
                    Rectangle {
                        readonly property bool on: root.others.indexOf(modelData.path) >= 0
                        readonly property bool navigable: true
                        readonly property bool isSwitch: true
                        readonly property string feedbackKind: on ? "toggle-off" : "toggle-on"
                        signal activated()
                        onActivated: root.toggleOther(modelData.path)
                        width: parent.width
                        height: Math.max(Theme.px(64), otherText.implicitHeight + Theme.px(28))
                        radius: Theme.radiusM
                        color: Theme.surface
                        border.color: Theme.line
                        border.width: Math.max(1, Theme.px(1))
                        Column {
                            id: otherText
                            anchors { left: parent.left; leftMargin: Theme.px(18); verticalCenter: parent.verticalCenter }
                            width: parent.width * 0.55
                            spacing: Theme.px(3)
                            FitText {
                                width: parent.width
                                text: modelData.model || modelData.path
                                color: Theme.fg
                                font.family: Theme.textFont
                                font.weight: Font.Medium
                                font.pixelSize: Theme.fs(17)
                            }
                            Text {
                                text: GameInfo.size(modelData.size) + " · " + Tr.tr(parent.parent.on ? "installer.others.prepare" : "installer.others.leave")
                                color: Theme.muted
                                font.family: Theme.textFont
                                font.pixelSize: Theme.fs(14.5)
                            }
                        }
                        Row {                     // what is on it, then the switch
                            anchors { right: parent.right; rightMargin: Theme.px(18); verticalCenter: parent.verticalCenter }
                            spacing: Theme.px(10)
                            Repeater {
                                model: root.contents(modelData)
                                Tag { tag: modelData }
                            }
                            Rectangle {
                                anchors.verticalCenter: parent.verticalCenter
                                width: Theme.px(44); height: Theme.px(24); radius: height / 2
                                color: parent.parent.on ? Theme.accent : Theme.raised
                                border.color: parent.parent.on ? Theme.accent : Theme.line
                                border.width: Math.max(1, Theme.px(1))
                                Rectangle {
                                    width: Theme.px(16); height: width; radius: width / 2
                                    anchors.verticalCenter: parent.verticalCenter
                                    x: parent.parent.parent.on ? Theme.px(23) : Theme.px(3)
                                    color: parent.parent.parent.on ? Theme.onAccent : Theme.muted
                                    Behavior on x { NumberAnimation { duration: Theme.motionMs; easing.type: Easing.OutCubic } }
                                }
                            }
                        }
                        FocusFrame {}
                    }
                }
            }
            Row {
                spacing: Theme.px(16)
                topPadding: Theme.px(6)
                Button { text: Tr.tr("installer.continue"); style: "primary"; onActivated: root.afterOthers() }
                Button { text: Tr.tr("installer.back"); onActivated: root.back() }
            }
        }

        // ---- The names of the disks for games (6.7)
        Column {
            visible: root.screen === "dnames"
            anchors.centerIn: parent
            width: Theme.px(860)
            spacing: Theme.px(18)
            Heading { text: Tr.tr("installer.dnames.title") }
            Para { text: Tr.tr("installer.dnames.text") }
            Repeater {
                model: root.screen === "dnames" ? root.gamesDisks : []
                Rectangle {
                    readonly property bool navigable: true
                    signal activated()
                    onActivated: root.renameOther(modelData.path)
                    width: parent.width
                    height: dnameColumn.implicitHeight + Theme.px(28)
                    radius: Theme.radiusM
                    color: Theme.surface
                    border.color: Theme.line
                    border.width: Math.max(1, Theme.px(1))
                    Column {
                        id: dnameColumn
                        anchors { left: parent.left; right: parent.right; leftMargin: Theme.px(18); rightMargin: Theme.px(18); verticalCenter: parent.verticalCenter }
                        spacing: Theme.px(4)
                        Text {
                            text: Tr.tr("installer.dname.label", { disk: modelData.model || modelData.path })
                            color: Theme.muted
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(14.5)
                        }
                        FitText {
                            width: parent.width
                            text: root.gameNames[modelData.path] || ""
                            color: Theme.fg
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(24)
                        }
                    }
                    FocusFrame {}
                }
            }
            Row {
                spacing: Theme.px(16)
                topPadding: Theme.px(6)
                Button { text: Tr.tr("installer.continue"); style: "primary"; onActivated: root.go("name") }
                Button { text: Tr.tr("installer.back"); onActivated: root.back() }
            }
        }

        // ---- The console's name
        Column {
            visible: root.screen === "name"
            anchors.centerIn: parent
            width: Theme.px(860)
            spacing: Theme.px(18)
            Heading { text: Tr.tr("installer.name.title") }
            Para { text: Tr.tr("installer.name.text") }
            Rectangle {
                readonly property bool navigable: true
                signal activated()
                onActivated: if (root.theme) root.theme.renameConsole(root.consoleName, name => { root.consoleName = name; })
                width: parent.width
                height: nameColumn.implicitHeight + Theme.px(28)
                radius: Theme.radiusM
                color: Theme.surface
                border.color: Theme.line
                border.width: Math.max(1, Theme.px(1))
                Column {
                    id: nameColumn
                    anchors { left: parent.left; right: parent.right; leftMargin: Theme.px(18); rightMargin: Theme.px(18); verticalCenter: parent.verticalCenter }
                    spacing: Theme.px(4)
                    Text {
                        text: Tr.tr("installer.name.label")
                        color: Theme.muted
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                    }
                    FitText {
                        width: parent.width
                        text: root.consoleName
                        color: Theme.fg
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(24)
                    }
                }
                FocusFrame {}
            }
            Row {
                spacing: Theme.px(16)
                topPadding: Theme.px(6)
                Button { text: Tr.tr("installer.continue"); style: "primary"; onActivated: root.go("warn") }
                Button { text: Tr.tr("installer.back"); onActivated: root.back() }
            }
        }

        // ---- The warning
        Column {
            visible: root.screen === "warn"
            anchors.centerIn: parent
            width: Theme.px(860)
            spacing: Theme.px(18)
            Heading { text: Tr.tr(root.others.length > 0 ? "installer.warn.titles" : "installer.warn.title") }
            Para {
                visible: root.others.length === 0
                text: root.disk ? Tr.tr("installer.warn.text", { disk: root.disk.model || root.disk.path, size: GameInfo.size(root.disk.size) }) : ""
                color: Theme.fg
            }
            Repeater {                    // 6.7: every disk erased, and what becomes of it
                model: root.others.length > 0 && root.screen === "warn" ? root.erased : []
                Para {
                    text: Tr.tr(index === 0 ? "installer.warn.system" : "installer.warn.games",
                                { disk: modelData.model || modelData.path, size: GameInfo.size(modelData.size), name: root.gameNames[modelData.path] || "" })
                    color: Theme.fg
                }
            }
            Para {
                visible: root.others.length > 0
                text: Tr.tr("installer.warn.others")
                small: true
            }
            Rectangle {                   // Windows or BitLocker: what will be lost
                visible: root.hasWindows
                width: parent.width
                height: alertColumn.implicitHeight + Theme.px(32)
                radius: Theme.radiusM
                color: Qt.tint(Theme.bg, Qt.rgba(Theme.accent.r, Theme.accent.g, Theme.accent.b, 0.1))
                border.color: Theme.accent
                border.width: Theme.px(2)
                Column {
                    id: alertColumn
                    anchors { left: parent.left; right: parent.right; leftMargin: Theme.px(18); rightMargin: Theme.px(18); verticalCenter: parent.verticalCenter }
                    spacing: Theme.px(6)
                    Text {
                        width: parent.width
                        // 6.7: with more disks, which ones
                        text: root.others.length === 0 ? Tr.tr("installer.warn.windows")
                              : root.erased.filter(d => d.windows || d.bitlocker).map(d =>
                                    [d.windows ? Tr.tr("installer.warn.windows.on", { disk: d.model || d.path }) : "",
                                     d.bitlocker ? Tr.tr("installer.warn.bitlocker.on", { disk: d.model || d.path }) : ""].filter(x => x).join(" · ")).join(" · ")
                        color: Theme.fg
                        font.family: Theme.textFont
                        font.weight: Font.DemiBold
                        font.pixelSize: Theme.fs(17)
                        wrapMode: Text.Wrap
                    }
                    Text {
                        width: parent.width
                        text: Tr.tr("installer.warn.windows.text")
                        color: Theme.fg
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                        wrapMode: Text.Wrap
                    }
                }
            }
            Rectangle {                   // the switch to turn on before "Continue"
                id: understandRow
                visible: root.hasWindows
                readonly property bool navigable: true
                readonly property bool isSwitch: true
                readonly property string feedbackKind: root.understood ? "toggle-off" : "toggle-on"
                signal activated()
                onActivated: root.understood = !root.understood
                width: parent.width
                height: Math.max(Theme.px(64), understandText.implicitHeight + Theme.px(28))
                radius: Theme.radiusM
                color: Theme.surface
                border.color: Theme.line
                border.width: Math.max(1, Theme.px(1))
                Text {
                    id: understandText
                    anchors { left: parent.left; leftMargin: Theme.px(18); right: knob.left; rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                    text: Tr.tr("installer.warn.understand")
                    color: Theme.fg
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(17)
                    wrapMode: Text.Wrap
                }
                Rectangle {
                    id: knob
                    anchors { right: parent.right; rightMargin: Theme.px(18); verticalCenter: parent.verticalCenter }
                    width: Theme.px(44); height: Theme.px(24); radius: height / 2
                    color: root.understood ? Theme.accent : Theme.raised
                    border.color: root.understood ? Theme.accent : Theme.line
                    border.width: Math.max(1, Theme.px(1))
                    Rectangle {
                        width: Theme.px(16); height: width; radius: width / 2
                        anchors.verticalCenter: parent.verticalCenter
                        x: root.understood ? Theme.px(23) : Theme.px(3)
                        color: root.understood ? Theme.onAccent : Theme.muted
                        Behavior on x { NumberAnimation { duration: Theme.motionMs; easing.type: Easing.OutCubic } }
                    }
                }
                FocusFrame {}
            }
            Row {
                spacing: Theme.px(16)
                topPadding: Theme.px(6)
                Button {
                    text: Tr.tr("installer.continue")
                    style: "primary"
                    enabled: !root.hasWindows || root.understood
                    opacity: enabled ? 1 : 0.45
                    onActivated: root.go("confirm")
                }
                Button { text: Tr.tr("installer.back"); onActivated: root.back() }
            }
        }

        // ---- The final confirmation: A held for three seconds
        Column {
            visible: root.screen === "confirm"
            anchors.centerIn: parent
            width: Theme.px(860)
            spacing: Theme.px(18)
            Heading { text: Tr.tr("installer.confirm.title"); horizontalAlignment: Text.AlignHCenter }
            Para {
                text: !root.disk ? "" : root.others.length > 0 ? Tr.tr("installer.confirm.texts", { disks: root.diskList(root.erased) })
                      : Tr.tr("installer.confirm.text", { disk: root.disk.model || root.disk.path })
                color: Theme.fg
                horizontalAlignment: Text.AlignHCenter
            }
            Column {
                anchors.horizontalCenter: parent.horizontalCenter
                spacing: Theme.px(14)
                HoldButton {
                    id: hold
                    anchors.horizontalCenter: parent.horizontalCenter
                    onConfirmed: root.start()
                }
                Text {
                    anchors.horizontalCenter: parent.horizontalCenter
                    text: Tr.tr("installer.confirm.hold")
                    color: Theme.muted
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                }
            }
            Row {
                anchors.horizontalCenter: parent.horizontalCenter
                topPadding: Theme.px(6)
                Button { text: Tr.tr("installer.back"); onActivated: root.back() }
            }
        }

        // ---- The progress
        Column {
            visible: root.screen === "progress"
            anchors.centerIn: parent
            width: Theme.px(860)
            spacing: Theme.px(18)
            Heading { text: Tr.tr("installer.install.title"); horizontalAlignment: Text.AlignHCenter }
            Para { text: Tr.tr("installer.install.text"); horizontalAlignment: Text.AlignHCenter }
            Column {
                anchors.horizontalCenter: parent.horizontalCenter
                width: Theme.px(760)
                spacing: Theme.px(16)
                Rectangle {
                    width: parent.width
                    height: Theme.px(10)
                    radius: height / 2
                    color: Theme.raised
                    clip: true
                    Rectangle {
                        width: parent.width * Math.max(0, Math.min(100, root.status.percent || 0)) / 100
                        height: parent.height
                        radius: height / 2
                        color: Theme.accent
                        Behavior on width { NumberAnimation { duration: Theme.motionMs * 2 } }
                    }
                }
                Column {
                    spacing: Theme.px(8)
                    Repeater {
                        model: root.phases
                        Row {
                            spacing: Theme.px(8)
                            Spinner {
                                visible: index === root.phase
                                anchors.verticalCenter: parent.verticalCenter
                                width: Theme.fs(16); height: width
                            }
                            Text {
                                visible: index < root.phase
                                anchors.verticalCenter: parent.verticalCenter
                                text: "✓"
                                color: Theme.accent
                                font.family: Theme.textFont
                                font.pixelSize: Theme.fs(17)
                            }
                            Text {
                                anchors.verticalCenter: parent.verticalCenter
                                text: Tr.tr("installer.phase." + modelData)
                                color: index === root.phase ? Theme.fg : Theme.muted
                                font.family: Theme.textFont
                                font.weight: index === root.phase ? Font.Medium : Font.Normal
                                font.pixelSize: Theme.fs(17)
                            }
                        }
                    }
                }
            }
        }

        // ---- Installed
        Column {
            visible: root.screen === "done"
            anchors.centerIn: parent
            width: Theme.px(860)
            spacing: Theme.px(18)
            Heading { text: Tr.tr("installer.done.title"); big: true; horizontalAlignment: Text.AlignHCenter }
            Para { text: Tr.tr("installer.done.text"); horizontalAlignment: Text.AlignHCenter }
            Row {
                anchors.horizontalCenter: parent.horizontalCenter
                topPadding: Theme.px(6)
                Button {
                    text: Tr.tr("installer.done.restart")
                    style: "primary"
                    onActivated: owneetd.post("/v1/power/restart", {}, () => {})
                }
            }
        }

        // ---- Stopped
        Column {
            visible: root.screen === "error"
            anchors.centerIn: parent
            width: Theme.px(860)
            spacing: Theme.px(18)
            Heading { text: Tr.tr("installer.err.title"); horizontalAlignment: Text.AlignHCenter }
            Para { text: root.errorText(root.error); color: Theme.fg; horizontalAlignment: Text.AlignHCenter }
            Para {
                text: Tr.tr(root.untouched ? "installer.err.note.none" : root.others.length > 0 ? "installer.err.notes" : "installer.err.note")
                small: true
                horizontalAlignment: Text.AlignHCenter
            }
            Row {
                anchors.horizontalCenter: parent.horizontalCenter
                spacing: Theme.px(16)
                topPadding: Theme.px(6)
                Button { text: Tr.tr("installer.err.again"); style: "primary"; onActivated: root.go("confirm") }
                Button { text: Tr.tr("installer.back"); onActivated: root.back() }
            }
        }
    }

    PromptBar {
        id: prompts
        anchors {
            left: parent.left; right: parent.right; bottom: parent.bottom
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; bottomMargin: Theme.safeBottom
        }
        globalPrompts: []
        visible: !(root.theme && root.theme.windowOpen)     // a window (the keyboard) has its own
        prompts: {
            const item = nav.current;
            if (root.screen === "progress" || !item)
                return [];
            const P = (b, l) => ({ buttons: [b], label: Tr.tr(l) });
            if (item.isHold === true)
                return [P("a", "installer.p.hold"), P("b", "prompt.back")];
            if (item.isSwitch === true)
                return [P("a", "installer.p.toggle"), P("b", "prompt.back")];
            if (root.screen === "welcome" || root.screen === "done")
                return [P("a", "prompt.select")];
            return [P("a", "prompt.select"), P("b", "prompt.back")];
        }
    }

    // What is on a disk: [tag, attention]
    function contents(d) {
        const t = [];
        if (d.windows) t.push(["windows", true]);
        if (d.bitlocker) t.push(["bitlocker", true]);
        if (d.linux) t.push(["linux", false]);
        if (!d.windows && !d.bitlocker && !d.linux && (d.partitions || []).length > 0)
            t.push(["data", false]);
        if (t.length === 0) t.push(["empty", false]);
        return t;
    }

    // ---- The screens' parts
    component Tag: Rectangle {
        property var tag: ["", false]       // [what, attention]
        width: tagText.implicitWidth + Theme.px(20)
        height: tagText.implicitHeight + Theme.px(4)
        radius: height / 2
        color: tag[1] ? Theme.accent : Theme.raised
        Text {
            id: tagText
            anchors.centerIn: parent
            text: Tr.tr("installer.disk." + tag[0])
            color: tag[1] ? Theme.onAccent : Theme.fg
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(16)
        }
    }
    component Heading: Text {
        property bool big: false
        width: parent ? parent.width : 0
        color: Theme.fg
        font.family: Theme.displayFont
        font.weight: Font.Bold
        font.pixelSize: Theme.fs(big ? 40 : 24)
        wrapMode: Text.Wrap
    }
    component Para: Text {
        property bool small: false
        width: parent ? parent.width : 0
        color: Theme.muted
        font.family: Theme.textFont
        font.pixelSize: Theme.fs(small ? 14.5 : 17)
        wrapMode: Text.Wrap
    }
}
