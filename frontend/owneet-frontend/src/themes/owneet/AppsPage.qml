// SPDX-License-Identifier: GPL-3.0-or-later
// The Apps page ([[step-5.7-apps-page]], approved demo design/demos/5.7-apps-page.html): every app,
// a page of Home opened by "All apps" (the home's sixth place with more than six apps). The tiles
// of the home, larger, five to a row: the built-in apps first (Videos, Music, the store), then the
// web apps, the installed ones before the missing ones (foundation/apps.js). The home grid's
// buttons ([[controller-input-map]], "The Apps page"): A opens the app or installs a missing one,
// Menu its options (Remove; on Steam's tile, close Steam while it runs), B back to the home on
// "All apps", the right stick scrolls; the grid goes round like the library's.
import QtQuick 2.15
import "foundation"
import "foundation/apps.js" as Apps

FocusScope {
    id: page
    // The web apps of the catalogue (owneetd): { id, nameKey, icon, installed, busy, sizeMiB }
    property var webApps: []
    property var running: null                 // the running game, web app or player: { id, name, kind, started }
    property string playerKind: ""             // the player's page: "videos", "music"
    property bool steamRunning: false          // Steam runs (hidden or not): Menu on its tile closes it
    signal openWebApp(var app)                 // A on a web app: install it, or open it
    signal webAppOptions(var app)              // Menu on a web app
    signal openMedia(string kind)              // the Videos or Music tile
    signal openStore()
    signal steamOptions()
    signal leave()                             // B: back to the home screen, on "All apps"

    readonly property int columns: 5
    function webApp(id) { return webApps.find(a => a.id === id) || null; }
    readonly property bool mediaOpen: running !== null && running.kind === "media"
    function isOpen(app) { return app !== null && running !== null && running.id === "web:" + app.id; }

    // The page's order, changed only when it changes: the tiles stay while an app is installed or
    // removed; when one moves, the selection follows it
    readonly property var keysNow: Apps.pageOrder(Apps.fixedOrder(webApps))
    property var keys: []
    onKeysNowChanged: {
        if (keysNow.join() === keys.join()) return;
        keys = keysNow;
        Qt.callLater(keepSelected);
    }
    Component.onCompleted: keys = keysNow
    property string selectedKey: ""
    function remember() { selectedKey = grid.currentItem ? grid.currentItem.key : ""; }
    function select(i) {
        grid.currentIndex = Math.max(0, Math.min(i, keys.length - 1));
        grid.positionViewAtIndex(grid.currentIndex, GridView.Contain);
        remember();
    }
    function keepSelected() { const i = keys.indexOf(selectedKey); if (i >= 0) select(i); }
    function focusStart() { select(0); grid.forceActiveFocus(); }
    // Back from the Videos or Music page opened here: the selection on its tile
    function focusApp(key) { select(Math.max(0, keys.indexOf(key))); grid.forceActiveFocus(); }

    // The prompt bar follows the selected tile, as on the home screen
    readonly property var prompts: {
        const P = (b, l) => ({ buttons: b, label: Tr.tr(l) });
        const t = grid.currentItem;
        let out = [];
        if (t && t.webApp)
            out = t.webApp.busy ? [] : [P(["a"], isOpen(t.webApp) ? "prompt.resume" : t.webApp.installed ? "prompt.open" : "prompt.install"),
                                        P(["menu"], "prompt.options")];
        else if (t && t.steamStore && steamRunning)
            out = [P(["a"], "prompt.open"), P(["menu"], "prompt.options")];
        else if (t)
            out = [P(["a"], "prompt.open")];
        return out.concat([P(["b"], "prompt.back")]);
    }

    // ---- Header: the page's name and how many apps there are
    Item {
        id: header
        anchors { left: parent.left; right: parent.right; top: parent.top }
        height: title.height
        Text {
            id: title
            text: Tr.tr("apps.title")
            color: Theme.fg
            font.family: Theme.displayFont
            font.weight: Font.Bold
            font.pixelSize: Theme.fs(40)
        }
        Text {
            anchors { left: title.right; leftMargin: Theme.px(14); right: parent.right; baseline: title.baseline }
            text: Tr.trn("apps.count", page.keys.length)
            color: Theme.muted
            elide: Text.ElideRight
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(17)
        }
    }

    // ---- The grid: it scrolls with the selection and keeps room for the focus ring at both ends
    GridView {
        id: grid
        readonly property real ring: Theme.px(18)
        readonly property real gap: Theme.px(16)
        anchors {
            left: parent.left; right: parent.right; top: header.bottom; bottom: parent.bottom
            leftMargin: -ring; rightMargin: -ring; topMargin: Theme.px(18) - ring
        }
        // the cells are laid out in the width without the margins: the right margin is one gap
        // smaller, so that five tiles and four gaps fill the page exactly
        leftMargin: ring; rightMargin: ring - gap; topMargin: ring; bottomMargin: ring
        clip: true
        focus: true
        model: page.keys
        cellWidth: Math.floor((width - 2 * ring + gap) / page.columns)
        cellHeight: Theme.fs(128) + gap
        keyNavigationEnabled: false             // the keys below move the selection
        highlightRangeMode: GridView.ApplyRange
        preferredHighlightBegin: ring
        preferredHighlightEnd: height - ring
        highlightMoveDuration: Theme.motionMs
        delegate: AppTile {
            big: true
            focus: true
            width: grid.cellWidth - grid.gap
            key: modelData
            webApp: key.startsWith("web:") ? page.webApp(key.slice(4)) : null
            open: (webApp !== null && page.isOpen(webApp)) || (mediaKind !== "" && page.mediaOpen && page.playerKind === mediaKind)
        }
        Keys.onPressed: {
            const a = Nav.action(event);
            const t = currentItem;
            const n = count, cols = page.columns;
            const i = currentIndex, row = Math.floor(i / cols), col = i % cols, rows = Math.ceil(n / cols);
            const rowLength = r => Math.min(cols, n - r * cols);
            // round, like the library's grid: left / right at a row's edge to its other end, down /
            // up from the last / first row to the first / last, the same column or the row's last
            let target = -1;
            if (n === 0) target = -1;
            else if (a === "left") target = col > 0 ? i - 1 : row * cols + rowLength(row) - 1;
            else if (a === "right") target = col < rowLength(row) - 1 ? i + 1 : row * cols;
            else if (a === "up" || a === "down") {
                const r = ((a === "down" ? row + 1 : row - 1) + rows) % rows;
                target = r * cols + Math.min(col, rowLength(r) - 1);
            }
            if (target >= 0) {
                if (target === i) {
                    Nav.feedback("edge");
                } else {
                    Nav.feedback("move");
                    page.select(target);
                }
            } else if (Nav.listKeys(grid, a)) {         // the right stick: a page at a time
                page.remember();
            } else if (a === "accept" && t) {
                Nav.feedback("confirm");
                if (t.webApp) page.openWebApp(t.webApp);
                else if (t.mediaKind !== "") page.openMedia(t.mediaKind);
                else if (t.steamStore) page.openStore();
            } else if (a === "options" && t && t.webApp && !t.webApp.busy) {
                page.webAppOptions(t.webApp);
            } else if (a === "options" && t && t.steamStore && page.steamRunning) {
                page.steamOptions();
            } else if (a === "back") {
                Nav.feedback("back");
                page.leave();
            } else {
                return;
            }
            event.accepted = true;
        }
    }
}
