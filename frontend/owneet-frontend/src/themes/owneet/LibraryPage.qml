// SPDX-License-Identifier: GPL-3.0-or-later
// The library (roadmap 3.7, approved demo design/demos/3.7-library.html): every game in one grid,
// filters on LT / RT, sort on Y, the disks with their free space (up from the first row; A opens
// the Disks window). Games owned but not installed arrive with the Steam integration (4.x).
import QtQuick 2.15
import "foundation"

FocusScope {
    id: page
    property var games: []                     // every game (any order)
    signal launch(var game)
    signal details(var game)
    signal options(var game)
    signal chooseSort()
    signal openDisks(var volumes)

    readonly property var filters: ["all", "installed", "favorites", "steam", "local"]
    property string filter: "all"
    property string sort: "recent"             // saved by the shell
    readonly property int columns: 7

    function matches(game, f) {
        return f === "all" || (f === "installed" && GameInfo.installed(game)) || (f === "favorites" && game.favorite)
            || (f === "steam" && GameInfo.isSteam(game)) || (f === "local" && !GameInfo.isSteam(game));
    }
    property int favoritesRevision: 0           // bumped when a favourite changes
    readonly property var shown: {
        const r = favoritesRevision;
        return GameInfo.sorted(games.filter(g => matches(g, filter)), sort);
    }
    function count(f) { const r = favoritesRevision; return games.filter(g => matches(g, f)).length; }

    // Disks: refreshed whenever the library is shown (mounting arrives with step 6.5)
    property var volumes: []
    function refreshVolumes() { volumes = storage.volumes(); }
    onVisibleChanged: if (visible) refreshVolumes()
    Component.onCompleted: refreshVolumes()

    readonly property var prompts: {
        const P = (b, l) => ({ buttons: b, label: Tr.tr(l) });
        const page = [P(["y"], "prompt.sort"), P(["lt", "rt"], "prompt.filter")];
        if (disks.activeFocus)
            return [P(["a"], "prompt.disks")].concat(page);
        if (grid.count > 0)
            return [P(["a"], GameInfo.installed(grid.currentItem ? grid.currentItem.game : null) ? "prompt.play" : "prompt.install"),
                    P(["x"], "prompt.details"), P(["menu"], "prompt.options")].concat(page);
        return page;
    }
    function focusStart() { grid.count > 0 ? grid.forceActiveFocus() : (disks.visible ? disks.forceActiveFocus() : page.forceActiveFocus()); }
    function select(game) {
        filter = "all";
        const i = shown.indexOf(game);
        if (i >= 0) grid.currentIndex = i;
        grid.forceActiveFocus();
    }
    function setFilter(step) {
        const keep = grid.currentItem ? grid.currentItem.game : null;
        filter = filters[(filters.indexOf(filter) + step + filters.length) % filters.length];
        Nav.feedback("tab");
        const i = keep ? shown.indexOf(keep) : -1;
        grid.currentIndex = i >= 0 ? i : 0;
        grid.positionViewAtIndex(grid.currentIndex, GridView.Contain);
    }

    // Filters and sort: for the whole page, wherever the selection is
    Keys.onPressed: {
        const a = Nav.action(event);
        if (a === "filter-prev" || a === "filter-next") setFilter(a === "filter-next" ? 1 : -1);
        else if (a === "page") page.chooseSort();
        else return;
        event.accepted = true;
    }

    // ---- Header: title, count, disks
    Item {
        id: header
        anchors { left: parent.left; right: parent.right; top: parent.top }
        height: Math.max(title.height, disks.height)
        Text {
            id: title
            anchors.bottom: parent.bottom
            text: Tr.tr("tab.library")
            color: Theme.fg
            font.family: Theme.displayFont
            font.weight: Font.Bold
            font.pixelSize: Theme.fs(40)
        }
        Text {
            anchors { left: title.right; leftMargin: Theme.px(14); baseline: title.baseline }
            text: Tr.trn("games.count", grid.count)
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(17)
        }
        Rectangle {
            // Up to two disks; with more, one summary (overflow rule, PROJECT_RULES.md section 9)
            id: disks
            readonly property bool navigable: true
            visible: page.volumes.length > 0
            anchors { right: parent.right; bottom: parent.bottom }
            width: diskRow.width + Theme.px(24)
            height: diskRow.height + Theme.px(16)
            radius: Theme.radiusM
            color: "transparent"
            Row {
                id: diskRow
                anchors.centerIn: parent
                spacing: Theme.px(22)
                Repeater {
                    model: {
                        const v = page.volumes;
                        if (v.length <= 2) return v;
                        const free = v.reduce((s, d) => s + d.free, 0), total = v.reduce((s, d) => s + d.total, 0);
                        return [{ summary: true, count: v.length, free: free, total: total }];
                    }
                    Column {
                        spacing: Theme.px(5)
                        width: Math.max(Theme.px(150), label.implicitWidth)
                        Text {
                            id: label
                            text: modelData.summary
                                ? Tr.tr("disks.summary", { n: modelData.count, free: GameInfo.size(modelData.free) })
                                : Tr.tr("disk.free", { name: modelData.name || Tr.tr("disk.internal"), free: GameInfo.size(modelData.free) })
                            color: Theme.muted
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(12.5)
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
                    }
                }
            }
            FocusFrame {}
            Keys.onPressed: {
                const a = Nav.action(event);
                if (a === "down" && grid.count > 0) { Nav.feedback("move"); grid.forceActiveFocus(); }
                else if (a === "up" || a === "left" || a === "right" || a === "down") Nav.feedback("edge");
                else if (a === "accept") { Nav.feedback("confirm"); page.openDisks(page.volumes); }
                else return;
                event.accepted = true;
            }
        }
    }

    // ---- Filters (LT / RT) and the current sort (Y)
    Item {
        id: filterBar
        anchors { left: parent.left; right: parent.right; top: header.bottom; topMargin: Theme.px(14) }
        height: chips.height
        Row {
            id: chips
            spacing: Theme.px(8)
            Glyph { button: "lt"; color: Theme.muted; anchors.verticalCenter: parent.verticalCenter }
            Repeater {
                model: page.filters
                Rectangle {
                    readonly property bool on: modelData === page.filter
                    anchors.verticalCenter: parent.verticalCenter
                    width: chipText.implicitWidth + Theme.px(30)
                    height: chipText.implicitHeight + Theme.px(12)
                    radius: height / 2
                    color: on ? Theme.accent : Theme.surface
                    border.color: on ? Theme.accent : Theme.line
                    border.width: Math.max(1, Theme.px(1))
                    Text {
                        id: chipText
                        anchors.centerIn: parent
                        text: Tr.tr("library.filter." + modelData) + "  " + page.count(modelData)
                        color: parent.on ? Theme.onAccent : Theme.muted
                        font.family: Theme.textFont
                        font.weight: parent.on ? Font.Medium : Font.Normal
                        font.pixelSize: Theme.fs(14.5)
                    }
                }
            }
            Glyph { button: "rt"; color: Theme.muted; anchors.verticalCenter: parent.verticalCenter }
        }
        Row {
            anchors { right: parent.right; verticalCenter: parent.verticalCenter }
            spacing: Theme.px(8)
            Glyph { button: "y"; color: Theme.muted; anchors.verticalCenter: parent.verticalCenter }
            Text {
                anchors.verticalCenter: parent.verticalCenter
                text: Tr.tr("library.sort." + page.sort)
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(14.5)
            }
        }
    }

    // ---- The grid: it scrolls with the selection and keeps room for the focus ring at both ends
    GridView {
        id: grid
        readonly property real ring: Theme.px(18)
        anchors {
            left: parent.left; right: parent.right; top: filterBar.bottom; bottom: parent.bottom
            leftMargin: -ring; rightMargin: -ring; topMargin: Theme.px(16) - ring
        }
        // GridView lays the cells out in the width without its margins: the right margin is one gap
        // smaller, so that seven covers and six gaps fill the page exactly
        leftMargin: ring; rightMargin: ring - Theme.px(14); topMargin: ring; bottomMargin: ring
        clip: true
        focus: true
        model: page.shown
        cellWidth: Math.floor((width - 2 * ring + Theme.px(14)) / page.columns)
        cellHeight: (cellWidth - Theme.px(14)) * 1.25 + Theme.px(16)
        keyNavigationEnabled: false             // Nav.listKeys moves the selection
        highlightRangeMode: GridView.ApplyRange
        preferredHighlightBegin: ring
        preferredHighlightEnd: height - ring
        highlightMoveDuration: Theme.motionMs
        delegate: Cover {
            width: grid.cellWidth - Theme.px(14)
            height: width * 1.25
            game: modelData
        }
        Keys.onPressed: {
            const a = Nav.action(event);
            const game = currentItem ? currentItem.game : null;
            if (a === "up" && currentIndex < page.columns && disks.visible) {
                Nav.feedback("move");
                disks.forceActiveFocus();       // from the first row up to the disks
            } else if (Nav.listKeys(grid, a)) {
            } else if (a === "accept" && game) {
                Nav.feedback("confirm");
                page.launch(game);
            } else if (a === "secondary" && game) {
                page.details(game);
            } else if (a === "options" && game) {
                page.options(game);
            } else {
                return;
            }
            event.accepted = true;
        }
    }
    Column {                                    // nothing to show for this filter
        visible: grid.count === 0
        anchors { horizontalCenter: grid.horizontalCenter; top: grid.top; topMargin: Theme.px(120) }
        spacing: Theme.px(8)
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            text: Tr.tr(page.filter === "favorites" ? "library.empty.favorites" : page.games.length === 0 ? "games.empty" : "library.empty.filter")
            color: Theme.fg
            font.family: Theme.displayFont
            font.weight: Font.Bold
            font.pixelSize: Theme.fs(24)
        }
        Text {
            visible: page.filter === "favorites"
            anchors.horizontalCenter: parent.horizontalCenter
            text: Tr.tr("library.empty.favorites.hint")
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(17)
        }
    }
}
