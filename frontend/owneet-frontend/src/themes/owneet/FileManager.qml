// SPDX-License-Identifier: GPL-3.0-or-later
// Settings → Advanced → File manager ([[step-6.11-simplified-file-manager]], approved demo
// design/demos/6.11-file-manager.html, owner 2026-11-13): a very simplified file manager, laid out as
// a desktop one (folder-shaped icons in a grid, in the palette's accent; a bar under the grid with
// the selected item's name, kind, size and date). It opens on places named for the user, never on
// technical folders: each disk's games folder, the Windows games' data (each with its game's
// name, "Game no longer here" when it is gone), the games' logs, Steam's leftovers. A opens a
// folder, B goes up (and closes on the places), Options: Delete, which asks first (Cancel
// selected); Windows data first say they may hold saves; Steam's leftovers only with Steam closed.
// Only the games folders' folders open: the other places' items are only deleted.
import QtQuick 2.15
import "foundation"

FocusScope {
    id: root
    anchors.fill: parent
    visible: false
    z: 101

    property var toast: null           // the theme's, which stays after the window closes
    property var games: []             // every game (api.allGames as a list): names for the data
    property bool steamRunning: false
    property var diskName: function(root) { return root; }
    property bool covered: false        // a window of the theme over it: its own prompts give way
    // asks for a window of the theme: kind "options" | "saves" | "delete", the item, its answer
    signal ask(string kind, var item, var answer)
    signal deletedInGames()             // something was deleted in a games folder

    // where it is: [] (the places), then the place and its folders: { kind, path, root, name }
    property var path: []
    property var items: []
    property bool measuring: false
    property int token: 0
    property var placeSizes: ({})

    function open() {
        path = [];
        Nav.feedback("sheet-open");
        Nav.windowOpened();
        visible = true;
        load();
        forceActiveFocus();
    }
    function close() {
        if (!visible)
            return;
        Nav.feedback("sheet-close");
        visible = false;
    }
    function refocus() { forceActiveFocus(); }
    // sizes as in the library, and small files in KB (a script is not "1 MB")
    function size(bytes) {
        if (bytes < 1e6)
            return Number(Math.max(1, Math.round(bytes / 1e3))).toLocaleString(Qt.locale(), "f", 0) + " KB";
        return GameInfo.sizeAuto(bytes);
    }

    // ---- What the items are called
    function prettySlug(name) {             // a game's data folder: "the-witcher-2-win-b264e729e7"
        const s = name.replace(/-[0-9a-f]{10}$/, "").replace(/-/g, " ").trim();
        return s.length ? s.charAt(0).toUpperCase() + s.slice(1) : name;
    }
    readonly property var windowsGames: {    // the data folder's name → the game's title
        const map = {};
        for (const g of games)
            if (GameInfo.isWindows(g) && g.extra["disk.dir"])
                map[files.windowsPrefix(String(g.extra["disk.dir"])).split("/").pop()] = g.title;
        return map;
    }
    readonly property var steamTitles: {
        const map = {};
        for (const g of games)
            if (GameInfo.isSteam(g)) map[GameInfo.appid(g)] = g.title;
        return map;
    }
    function placeName(p) {
        if (p.kind !== "games")
            return Tr.tr("files." + p.kind);
        const name = Tr.tr("files.games", { disk: diskName(p.root) });
        // two games folders on one disk ("Games" and "Giochi"): which one
        const twins = files.places().filter(q => q.kind === "games" && q.root === p.root).length > 1;
        return twins ? name + " (" + p.path.split("/").pop() + ")" : name;
    }
    function itemName(x) {
        if (x.place) return placeName(x);
        const place = path.length ? path[0].kind : "";
        if (x.steam) {
            const g = steamTitles[x.appid] || Tr.tr("files.steam_app", { id: x.appid });
            return Tr.tr(x.steam === "shaders" ? "files.shaders" : x.steam === "data" ? "files.data" : "files.download", { g: g });
        }
        if (place === "windows" || place === "logs")
            return windowsGames[x.name] || prettySlug(x.name);
        return x.name;
    }
    function itemState(x) {                 // Windows data and logs: is their game still here?
        const place = path.length ? path[0].kind : "";
        if (x.place || (place !== "windows" && place !== "logs")) return "";
        return windowsGames[x.name] ? "installed" : "gone";
    }
    function opens(x) { return !!x && (x.place || (path.length && path[0].kind === "games" && x.folder)); }
    function deletable(x) { return !!x && !x.place; }
    function isWindowsData(x) { return !!x && ((path.length && path[0].kind === "windows") || x.steam === "data"); }

    // ---- Reading
    function load() {
        items = [];
        if (path.length === 0) {
            const places = files.places().map(p => Object.assign({ place: true }, p));
            items = places;
            measuring = false;
            for (const p of places) {           // each place's size, as it comes
                const t = files.list(p.kind, p.path);
                pending[t] = p.path;
            }
            return;
        }
        measuring = true;
        const here = path[path.length - 1];
        token = files.list(path.length === 1 ? here.kind : "folder", here.path);
    }
    property var pending: ({})
    Connections {
        target: files
        function onListed(t, entries) {
            if (root.pending[t] !== undefined) {
                const sizes = Object.assign({}, root.placeSizes);
                sizes[root.pending[t]] = entries.reduce((s, e) => s + e.size, 0);
                delete root.pending[t];
                root.placeSizes = sizes;
                return;
            }
            if (t !== root.token) return;
            root.measuring = false;
            root.items = entries;
            grid.currentIndex = Math.min(grid.currentIndex, Math.max(0, entries.length - 1));
        }
        function onRemoved(t, p, ok, freed) {
            if (t !== root.removing) return;
            root.removing = -1;
            if (!ok) {
                Nav.feedback("error");
                if (root.toast) root.toast.show(Tr.tr("files.error", { name: root.removingName }));
                return;
            }
            Nav.feedback("confirm");
            if (root.toast) root.toast.show(Tr.tr("files.deleted", { name: root.removingName, size: root.size(freed) }));
            if (root.path.length && root.path[0].kind === "games") root.deletedInGames();
            root.load();
        }
    }
    property int removing: -1
    property string removingName: ""

    // ---- Moving
    function openItem() {
        const x = items[grid.currentIndex];
        if (!opens(x)) { Nav.feedback("edge"); return; }
        Nav.feedback("confirm");
        path = path.concat([x]);
        grid.currentIndex = 0;
        load();
    }
    function up() {
        if (path.length === 0) { close(); return; }
        Nav.feedback("back");
        const was = path[path.length - 1];
        path = path.slice(0, -1);
        load();
        Qt.callLater(() => {
            const i = items.findIndex(x => x.path === was.path);
            grid.currentIndex = Math.max(0, i);
        });
    }
    function options() {
        const x = items[grid.currentIndex];
        if (!deletable(x)) { Nav.feedback("edge"); return; }
        ask("options", { name: itemName(x) }, () => askDelete(x));
    }
    // the disk a file is on: the mounted disk whose folder holds it, else the console's own
    function diskOf(file) {
        const v = storage.volumes().filter(v => v.key !== "internal" && file.startsWith(v.key + "/"))
                                   .sort((a, b) => b.key.length - a.key.length)[0];
        return v || storage.volumes().find(v => v.key === "internal") || { key: "internal", readOnly: false };
    }
    function askDelete(x) {
        if (x.steam && steamRunning) { Nav.feedback("edge"); if (toast) toast.show(Tr.tr("files.steam_open")); return; }
        const v = diskOf(x.path);
        if (v.readOnly) { Nav.feedback("edge"); if (toast) toast.show(Tr.tr("files.read_only", { disk: diskName(v.key) })); return; }
        const name = itemName(x);
        const sure = () => ask("delete", { name: name, size: root.size(x.size), disk: diskName(v.key) }, () => {
            removingName = name;
            removing = files.remove(x.path);
        });
        if (isWindowsData(x))           // the owner's first question (2026-11-13): they may hold saves
            ask("saves", { name: name, game: x.steam ? (steamTitles[x.appid] || name) : name }, sure);
        else
            sure();
    }

    Keys.onPressed: {
        const a = Nav.action(event);
        const cols = grid.columns;
        const n = items.length;
        let i = grid.currentIndex;
        if (a === "left" || a === "right" || a === "up" || a === "down") {
            const to = a === "left" ? i - 1 : a === "right" ? i + 1 : a === "up" ? i - cols : i + cols;
            if (n === 0 || to < 0 || to >= n || (a === "left" && i % cols === 0) || (a === "right" && i % cols === cols - 1)) {
                Nav.feedback("edge");
            } else {
                grid.currentIndex = to;
                Nav.feedback("move");
            }
        } else if (a === "accept") {
            openItem();
        } else if (a === "back") {
            up();
        } else if (a === "options") {
            options();
        } else {
            return;
        }
        event.accepted = true;
    }

    Rectangle {
        anchors { fill: parent; margins: -Theme.bleed }
        color: Theme.bg
    }
    MouseArea {                         // nothing below it is reached
        anchors.fill: parent
        acceptedButtons: Qt.AllButtons
        onWheel: wheel.accepted = true
    }

    Text {
        id: title
        x: Theme.safeX; y: Theme.safeTop
        text: Tr.tr("advanced.files")
        color: Theme.fg
        font.family: Theme.displayFont
        font.weight: Font.Bold
        font.pixelSize: Theme.fs(34)
    }
    Row {                               // Places › Games · 2TB HDD SATA › a folder
        id: crumbs
        anchors { left: title.left; top: title.bottom; topMargin: Theme.px(12) }
        width: parent.width - 2 * Theme.safeX
        spacing: Theme.px(10)
        clip: true
        Repeater {
            model: [Tr.tr("files.places")].concat(root.path.map(p => root.itemName(p)))
            Row {
                spacing: Theme.px(10)
                Text {
                    visible: index > 0
                    text: "›"
                    color: Theme.muted
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(17)
                }
                Text {
                    text: modelData
                    color: index === root.path.length ? Theme.fg : Theme.muted
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(17)
                }
            }
        }
    }

    GridView {
        id: grid
        anchors { left: title.left; right: parent.right; top: crumbs.bottom; bottom: status.top
                  topMargin: Theme.px(16); bottomMargin: Theme.px(14); rightMargin: Theme.safeX }
        readonly property int columns: 7
        cellWidth: width / columns
        cellHeight: Theme.px(124) + Theme.fs(17) * 3
        clip: true
        interactive: false
        model: root.items
        highlightFollowsCurrentItem: false
        preferredHighlightBegin: 0
        preferredHighlightEnd: height
        highlightRangeMode: GridView.ApplyRange
        delegate: Item {
            id: tile
            width: grid.cellWidth
            height: grid.cellHeight
            readonly property bool selected: GridView.isCurrentItem && root.activeFocus
            readonly property var x0: modelData
            readonly property string state0: root.itemState(modelData)
            readonly property string emblem: modelData.place ? ({ games: "controller", windows: "windows", logs: "logs", steam: "steam" })[modelData.kind]
                                             : root.path.length && root.path[0].kind === "windows" ? "windows"
                                             : modelData.steam ? "steam" : ""
            Rectangle {                 // the selected tile
                anchors { fill: parent; margins: Theme.px(5) }
                radius: Theme.radiusM
                color: tile.selected ? Theme.surface : "transparent"
                border.color: Theme.accent
                border.width: tile.selected ? Theme.px(3) : 0
            }
            Item {                      // the icon: a folder in the accent, or a document with its kind
                id: icon
                anchors.horizontalCenter: parent.horizontalCenter
                y: Theme.px(16)
                width: Theme.px(96); height: Theme.px(80)
                readonly property bool folder: modelData.place || modelData.folder || root.path.length && root.path[0].kind !== "games" && root.path[0].kind !== "logs"
                Rectangle {             // the folder's tab
                    visible: icon.folder
                    x: Theme.px(6); y: Theme.px(8)
                    width: Theme.px(36); height: Theme.px(16)
                    radius: Theme.px(5)
                    color: Theme.accent
                    opacity: 0.75
                }
                Rectangle {             // the folder
                    visible: icon.folder
                    x: Theme.px(4); y: Theme.px(18)
                    width: Theme.px(88); height: Theme.px(58)
                    radius: Theme.px(7)
                    color: Theme.accent
                    Rectangle {
                        anchors { left: parent.left; right: parent.right; top: parent.top }
                        height: Theme.px(8)
                        radius: Theme.px(4)
                        color: Theme.onAccent
                        opacity: 0.18
                    }
                }
                Rectangle {             // a document
                    visible: !icon.folder
                    anchors.horizontalCenter: parent.horizontalCenter
                    y: Theme.px(4)
                    width: Theme.px(44); height: Theme.px(72)
                    radius: Theme.px(4)
                    color: Theme.raised
                    border.color: Theme.line
                    border.width: Theme.px(2)
                    Text {
                        anchors { horizontalCenter: parent.horizontalCenter; bottom: parent.bottom; bottomMargin: Theme.px(14) }
                        text: {
                            const m = /\.([A-Za-z0-9]{1,4})$/.exec(modelData.name || "");
                            return m ? m[1].toUpperCase() : (root.path.length && root.path[0].kind === "logs" ? "LOG" : "");
                        }
                        color: Theme.muted
                        font.family: Theme.textFont
                        font.weight: Font.DemiBold
                        font.pixelSize: Theme.px(12)
                    }
                }
                Rectangle {             // the place's emblem
                    visible: tile.emblem !== ""
                    anchors { right: parent.right; rightMargin: Theme.px(2); bottom: parent.bottom }
                    width: Theme.px(34); height: width
                    radius: width / 2
                    color: Theme.bg
                    border.color: Theme.line
                    border.width: Theme.px(2)
                    Icon {
                        anchors.centerIn: parent
                        width: Theme.px(20); height: width
                        name: tile.emblem
                        color: Theme.fg
                    }
                }
            }
            Text {                      // the name, on two lines at most
                id: nameText
                anchors { top: icon.bottom; topMargin: Theme.px(8); horizontalCenter: parent.horizontalCenter }
                width: parent.width - Theme.px(20)
                horizontalAlignment: Text.AlignHCenter
                text: root.itemName(modelData)
                color: Theme.fg
                font.family: Theme.textFont
                font.weight: Font.Medium
                font.pixelSize: Theme.fs(14.5)
                wrapMode: Text.WrapAtWordBoundaryOrAnywhere
                maximumLineCount: 2
                elide: Text.ElideRight
                lineHeight: 1.1
            }
            Text {                      // the game of Windows data or logs: still here or not
                visible: tile.state0 !== ""
                anchors { top: nameText.bottom; topMargin: Theme.px(2); horizontalCenter: parent.horizontalCenter }
                width: parent.width - Theme.px(20)
                horizontalAlignment: Text.AlignHCenter
                text: tile.state0 ? Tr.tr("files." + tile.state0) : ""
                color: tile.state0 === "gone" ? Theme.accent : Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(12.5)
                wrapMode: Text.Wrap
                maximumLineCount: 2
                elide: Text.ElideRight
            }
        }
    }
    Text {                              // nothing in it, or measuring
        visible: root.measuring || (root.items.length === 0 && root.path.length > 0)
        anchors { left: grid.left; top: grid.top; topMargin: Theme.px(10) }
        text: Tr.tr(root.measuring ? "files.measuring" : "files.empty")
        color: Theme.muted
        font.family: Theme.textFont
        font.pixelSize: Theme.fs(17)
    }
    Rectangle {                         // the selected item: name, kind, size, date
        id: status
        anchors { left: title.left; right: parent.right; rightMargin: Theme.safeX; bottom: bar.top; bottomMargin: Theme.px(16) }
        height: statusRow.implicitHeight + Theme.px(20)
        radius: Theme.radiusM
        color: Theme.surface
        border.color: Theme.line
        border.width: 1
        readonly property var sel: root.items[grid.currentIndex] || null
        FitText {
            id: statusName
            anchors { left: parent.left; leftMargin: Theme.px(16); right: statusRow.left; rightMargin: Theme.px(20); verticalCenter: parent.verticalCenter }
            text: status.sel ? root.itemName(status.sel) : ""
            color: Theme.fg
            font.family: Theme.textFont
            font.weight: Font.Medium
            font.pixelSize: Theme.fs(14.5)
        }
        Row {
            id: statusRow
            anchors { right: parent.right; rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
            spacing: Theme.px(20)
            Text {
                text: !status.sel ? "" : status.sel.place ? (status.sel.kind === "games" ? "" : Tr.tr("files." + status.sel.kind + ".detail"))
                      : root.itemState(status.sel) ? Tr.tr("files." + root.itemState(status.sel))
                      : Tr.tr(status.sel.folder ? "files.folder" : "files.file")
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(14.5)
            }
            Text {
                readonly property var bytes: !status.sel ? undefined : status.sel.place ? root.placeSizes[status.sel.path] : status.sel.size
                text: bytes === undefined ? "" : root.size(bytes)
                color: Theme.fg
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(14.5)
            }
            Text {
                text: status.sel && !status.sel.place ? new Date(status.sel.modified).toLocaleDateString(Qt.locale(), Locale.ShortFormat) : ""
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(14.5)
            }
        }
    }
    PromptBar {
        id: bar
        visible: !root.covered
        anchors {
            left: parent.left; right: parent.right; bottom: parent.bottom
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; bottomMargin: Theme.safeBottom
        }
        readonly property var sel: root.items[grid.currentIndex] || null
        prompts: (root.opens(sel) ? [{ buttons: ["a"], label: Tr.tr("prompt.open") }] : [])
            .concat(root.deletable(sel) ? [{ buttons: ["menu"], label: Tr.tr("prompt.options") }] : [])
            .concat([{ buttons: ["b"], label: Tr.tr(root.path.length ? "prompt.up" : "prompt.back") }])
    }
}
