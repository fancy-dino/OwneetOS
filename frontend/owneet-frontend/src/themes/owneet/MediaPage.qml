// SPDX-License-Identifier: GPL-3.0-or-later
// The Videos and Music pages ([[step-5.6-local-videos-and-music]], [[local-media]], approved demo
// design/demos/5.6-videos-and-music.html), opened from their tiles on the home screen: the
// "Videos" / "Music" folders of every disk together (owneet::Media, `media`). Videos: "Continue
// watching", then the folders and videos; a folder opens as a list beside its picture, with the
// selected video's languages and where it stopped; Menu on a started video: Resume / Watch from
// the start; Y switches Autoplay. Music: by folder or by artist and album (LT / RT); the songs'
// tags are read the first time, with a spinner. B goes back a folder, then to the home screen.
// Without media, the page explains the folder. A disk plugged in or out changes the page quietly;
// the selection stays where it was. While the player is open, what plays is marked "Playing" (A on
// it brings the player back; a started video playing is first in Continue watching).
import QtQuick 2.15
import "foundation"

FocusScope {
    id: page
    property string kind: "videos"             // "videos" or "music"
    property bool autoplay: true
    property string playingPath: ""            // what the player plays ("": the player is not open)
    property real playingPosition: 0
    signal play(var entry, var list, bool fromStart)
    signal options(var entry, var list)         // Menu on a started video
    signal toggleAutoplay()
    signal leave()                              // B at the top: back to the home screen

    readonly property bool videos: kind === "videos"
    property string view: "folders"            // music: "folders" or "artists"
    readonly property bool byArtist: !videos && view === "artists"
    // Where the page is: the folders opened (paths), or [artist] / [artist, album]; each view
    // keeps its own
    property var videoStack: []
    property var folderStack: []
    property var artistStack: []
    readonly property var stack: videos ? videoStack : byArtist ? artistStack : folderStack
    function setStack(s) {
        if (videos) videoStack = s; else if (byArtist) artistStack = s; else folderStack = s;
    }
    readonly property string folderPath: !byArtist && stack.length > 0 ? stack[stack.length - 1] : ""

    // ---- What the page shows (read only while it is on screen)
    property int visits: 0                      // shown again: the saved positions may have changed
    onVisibleChanged: if (visible) { visits++; if (byArtist && media.tagsState === "none") media.readTags(); }
    readonly property int fileCount: { const r = media.revision; return visible ? media.totals(kind).files : 0; }
    readonly property var topFolder: { const r = media.revision; return visible ? media.folder(kind, "") : { folders: [], files: [], disks: 0 }; }
    readonly property var listing: {
        const r = media.revision;
        return !visible || byArtist ? null : folderPath === "" ? topFolder : media.folder(kind, folderPath);
    }
    readonly property var artists: { const r = media.revision, t = media.tagsState; return visible && byArtist && t === "ready" ? media.artists() : []; }
    readonly property var albums: {
        const r = media.revision, t = media.tagsState;
        return visible && byArtist && t === "ready" && stack.length >= 1 ? media.albums(stack[0]) : [];
    }
    readonly property var songs: {
        const r = media.revision, t = media.tagsState;
        return visible && byArtist && t === "ready" && stack.length === 2 ? media.songs(stack[0], stack[1]) : [];
    }
    readonly property var continueList: { const r = media.revision, v = visits; return visible && videos && videoStack.length === 0 ? media.continueWatching() : []; }
    readonly property string mode: fileCount === 0 ? "empty" : byArtist && media.tagsState !== "ready" ? "reading"
                                 : stack.length === 0 ? "grid" : "list"
    readonly property bool showDisks: topFolder.disks > 1

    // the items of the grid and of the list: { type, id, entry, cont }
    readonly property var continueCards: {
        if (mode !== "grid")
            return [];
        const list = continueList.filter(v => v.path !== playingPath);
        if (videos && playingPath !== "") {
            const folder = parentOf(playingPath);
            const f = media.folder(kind, folder);
            const e = f.files.find(x => x.path === playingPath);
            if (e) list.unshift(Object.assign({}, e, { trail: media.trail(folder), picture: f.picture || "" }));
        }
        return list.map(v => ({ type: "video", id: "continue:" + v.path, entry: v, cont: true }));
    }
    function entries(l) {
        return l.folders.map(f => ({ type: "folder", id: f.path, entry: f })).concat(l.files.map(f => ({ type: f.kind, id: f.path, entry: f })));
    }
    readonly property var cards: mode !== "grid" ? [] : byArtist ? artists.map(a => ({ type: "artist", id: "artist:" + a.key, entry: a }))
                                                 : listing ? entries(listing) : []
    readonly property var rows: {
        if (mode !== "list")
            return [];
        if (byArtist)
            return stack.length === 1 ? albums.map(a => ({ type: "album", id: "album:" + a.key, entry: a }))
                                      : songs.map(s => ({ type: "song", id: s.path, entry: s }));
        return listing ? entries(listing) : [];
    }
    // A folder or an artist that is gone (its disk unplugged): back to where there is something
    onListingChanged: if (listing && listing.exists === false && stack.length > 0) Qt.callLater(() => page.setStack(page.stack.slice(0, -1)))
    onAlbumsChanged: if (byArtist && media.tagsState === "ready" && stack.length === 1 && albums.length === 0) Qt.callLater(() => page.setStack([]))
    onSongsChanged: if (byArtist && media.tagsState === "ready" && stack.length === 2 && songs.length === 0) Qt.callLater(() => page.setStack(page.stack.slice(0, 1)))

    // ---- The selection: kept by id when the items are read again
    property var selected: null
    property string selectedId: ""
    function remember(data) { selected = data; selectedId = data ? data.id : ""; }
    onCardsChanged: Qt.callLater(restore)
    onContinueCardsChanged: Qt.callLater(restore)
    onRowsChanged: Qt.callLater(restore)
    onModeChanged: Qt.callLater(() => { if (page.activeFocus) page.focusStart(); })
    function restore() {
        if (!visible)
            return;                             // hidden, the lists are empty: the selection is kept
        if (mode === "grid") {
            const items = Nav.navigables(gridArea);
            const it = items.find(i => i.itemData && i.itemData.id === selectedId) || items[0] || null;
            if (!it)
                return;
            if (page.activeFocus) gridArea.focusItem(it); else gridArea.current = it;
            remember(it.itemData);
            reveal(it, false);
        } else if (mode === "list") {
            const i = Math.max(0, rows.findIndex(r => r.id === selectedId));
            rowList.currentIndex = i;
            rowList.positionViewAtIndex(i, ListView.Contain);
            remember(rows[i] || null);
        } else {
            remember(null);
        }
    }
    // The player keeps where the page was; when it closes the page is there again, the selection
    // on what played last
    function saveState() {
        return { view: view, videoStack: videoStack.slice(), folderStack: folderStack.slice(), artistStack: artistStack.slice() };
    }
    function restoreState(state, path) {
        if (state) {
            view = state.view;
            videoStack = state.videoStack;
            folderStack = state.folderStack;
            artistStack = state.artistStack;
        }
        if (path) selectedId = path;
        Qt.callLater(restore);
    }
    function focusStart() {
        if (mode === "grid") gridArea.forceActiveFocus();
        else if (mode === "list") rowList.forceActiveFocus();
        else page.forceActiveFocus();
        restore();
    }

    // ---- What the buttons do
    function parentOf(path) { return path.substring(0, path.lastIndexOf("/")); }
    // what plays after it: the rest of its folder, or of the album
    function listFor(data) {
        if (data.cont) return media.folder(kind, parentOf(data.entry.path)).files;
        if (byArtist) return songs;
        return listing ? listing.files : [data.entry];
    }
    function startedVideo(data) {
        return data !== null && data.type === "video" && (media.details(data.entry.path).position || 0) > 0;
    }
    function isPlaying(data) { return data !== null && !!data.entry && data.entry.path === playingPath && playingPath !== ""; }
    function activate(data) {
        if (!data)
            return;
        if (data.type === "folder" || data.type === "artist" || data.type === "album") {
            selectedId = "";
            setStack(stack.concat([data.type === "folder" ? data.entry.path : data.entry.key]));
            return;
        }
        page.play(data.entry, listFor(data), false);
    }
    function optionsFor(data) {
        if (!startedVideo(data) || isPlaying(data))
            return false;
        page.options(data.entry, listFor(data));
        return true;
    }
    function back() {
        Nav.feedback("back");
        if ((mode === "grid" || mode === "list") && stack.length > 0) {
            const last = stack[stack.length - 1];
            selectedId = byArtist ? (stack.length === 2 ? "album:" + last : "artist:" + last) : last;
            setStack(stack.slice(0, -1));
            return;
        }
        page.leave();
    }
    function switchView() {
        view = view === "folders" ? "artists" : "folders";
        Nav.feedback("tab");
        if (view === "artists" && media.tagsState === "none")
            media.readTags();
        selectedId = "";
    }
    // Y, LT / RT and B for the whole page, wherever the selection is (Y never opens the
    // notifications here)
    Keys.onPressed: {
        const a = Nav.action(event);
        if (a === "page") {
            if (videos && mode !== "empty") page.toggleAutoplay(); else Nav.feedback("edge");
        } else if (a === "filter-prev" || a === "filter-next") {
            if (!videos && mode !== "empty") switchView(); else Nav.feedback("edge");
        } else if (a === "back") {
            back();
        } else if (a === "options" || a === "secondary") {
            Nav.feedback("edge");
        } else {
            return;
        }
        event.accepted = true;
    }

    readonly property var prompts: {
        const P = (b, l) => ({ buttons: b, label: Tr.tr(l) });
        const out = [];
        const r = media.detailsRevision;
        const d = selected;
        if ((mode === "grid" || mode === "list") && d) {
            if (d.type === "folder" || d.type === "artist" || d.type === "album") {
                out.push(P(["a"], "prompt.open"));
            } else {
                const started = startedVideo(d), playing = isPlaying(d);
                out.push(P(["a"], started || playing ? "prompt.resume" : "prompt.play"));
                if (started && !playing) out.push(P(["menu"], "prompt.options"));
            }
        }
        if (mode !== "empty")
            out.push(videos ? P(["y"], "prompt.autoplay") : P(["lt", "rt"], "prompt.view"));
        out.push(P(["b"], "prompt.back"));
        return out;
    }

    // ---- Header: the page's name, where it is (or how much there is), Autoplay or the views
    readonly property string crumbs: {
        if (mode === "empty" || mode === "reading")
            return "";
        if (byArtist) {
            if (stack.length === 0) return Tr.trn("media.artists.count", artists.length);
            return "› " + (stack[0] || Tr.tr("media.unknown_artist")) + (stack.length === 2 ? " › " + (stack[1] || Tr.tr("media.other_songs")) : "");
        }
        if (stack.length === 0)
            return videos ? Tr.trn("media.folders.count", topFolder.folders.length) + " · " + Tr.trn("media.videos.count", fileCount)
                          : Tr.trn("media.songs.count", fileCount);
        return "› " + media.trail(folderPath);
    }
    Item {
        id: header
        anchors { left: parent.left; right: parent.right; top: parent.top }
        height: title.height
        Text {
            id: title
            text: Tr.tr(page.videos ? "app.videos" : "app.music")
            color: Theme.fg
            font.family: Theme.displayFont
            font.weight: Font.Bold
            font.pixelSize: Theme.fs(40)
        }
        Text {
            anchors { left: title.right; leftMargin: Theme.px(14); right: tools.left; rightMargin: Theme.px(24); baseline: title.baseline }
            text: page.crumbs
            color: Theme.muted
            elide: Text.ElideRight
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(17)
        }
        Row {
            id: tools
            visible: page.mode !== "empty"
            anchors { right: parent.right; verticalCenter: title.verticalCenter }
            spacing: Theme.px(8)
            Glyph { visible: page.videos; button: "y"; color: Theme.muted; anchors.verticalCenter: parent.verticalCenter }
            Text {
                visible: page.videos
                anchors.verticalCenter: parent.verticalCenter
                text: Tr.tr("media.autoplay", { state: Tr.tr(page.autoplay ? "media.on" : "media.off") })
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(14.5)
            }
            Glyph { visible: !page.videos; button: "lt"; color: Theme.muted; anchors.verticalCenter: parent.verticalCenter }
            Repeater {
                model: page.videos ? [] : ["folders", "artists"]
                Rectangle {
                    readonly property bool on: modelData === page.view
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
                        text: Tr.tr("media.view." + modelData)
                        color: parent.on ? Theme.onAccent : Theme.muted
                        font.family: Theme.textFont
                        font.weight: parent.on ? Font.Medium : Font.Normal
                        font.pixelSize: Theme.fs(14.5)
                    }
                }
            }
            Glyph { visible: !page.videos; button: "rt"; color: Theme.muted; anchors.verticalCenter: parent.verticalCenter }
        }
    }

    // ---- The top of a page, and the artists: cards in a grid (Continue watching first)
    readonly property real ring: Theme.px(18)
    readonly property int columns: videos ? 5 : 6
    readonly property real cardWidth: (gridArea.width - (columns - 1) * Theme.px(14)) / columns
    readonly property real cardHeight: videos ? cardWidth * 9 / 16 : cardWidth
    function reveal(item, animated) {
        const p = item.mapToItem(gridFlick.contentItem, 0, 0);
        let y = gridFlick.contentY;
        if (p.y - ring < y) y = p.y - ring;
        else if (p.y + item.height + ring > y + gridFlick.height) y = p.y + item.height + ring - gridFlick.height;
        y = Math.max(0, Math.min(Math.max(0, gridFlick.contentHeight - gridFlick.height), y));
        scrollAnimation.enabled = animated !== false;
        gridFlick.contentY = y;
    }
    NavArea {
        id: gridArea
        visible: page.mode === "grid"
        anchors { left: parent.left; right: parent.right; top: header.bottom; topMargin: Theme.px(18); bottom: parent.bottom }
        backFeedback: ""
        onMoved: function(item) { page.remember(item.itemData); page.reveal(item, true); }
        onAccepted: function(item) { page.activate(item.itemData); }
        onCancelled: page.back()
        onOtherAction: function(action, event) {
            if (action === "options") {
                if (!page.optionsFor(page.selected)) Nav.feedback("edge");
                event.accepted = true;
            } else if (action === "scroll-up" || action === "scroll-down") {
                // the right stick: two rows at a time
                const items = Nav.navigables(gridArea), i = items.indexOf(gridArea.current);
                const t = Math.max(0, Math.min(items.length - 1, i + (action === "scroll-up" ? -2 : 2) * page.columns));
                if (i < 0 || t === i) Nav.feedback("edge");
                else { Nav.feedback("move"); gridArea.focusItem(items[t]); }
                event.accepted = true;
            }
        }
        Flickable {
            id: gridFlick
            anchors { fill: parent; leftMargin: -page.ring; rightMargin: -page.ring; topMargin: -page.ring }
            contentHeight: gridColumn.height + 2 * page.ring
            interactive: false
            clip: true
            Behavior on contentY { id: scrollAnimation; NumberAnimation { duration: Theme.motionMs; easing.type: Easing.OutCubic } }
            Column {
                id: gridColumn
                x: page.ring; y: page.ring
                width: gridFlick.width - 2 * page.ring
                spacing: Theme.px(10)
                Label { visible: page.continueCards.length > 0; text: Tr.tr("media.continue") }
                NavGroup {
                    visible: page.continueCards.length > 0
                    width: parent.width
                    height: continueGrid.height + Theme.px(10)
                    Grid {
                        id: continueGrid
                        columns: page.columns
                        spacing: Theme.px(14)
                        Repeater {
                            model: page.continueCards
                            MediaCard {
                                width: page.cardWidth; height: page.cardHeight; itemData: modelData
                                playing: page.isPlaying(modelData)
                                livePosition: playing ? page.playingPosition : -1
                            }
                        }
                    }
                }
                Label { visible: page.continueCards.length > 0; text: Tr.tr("media.on_disks") }
                NavGroup {
                    width: parent.width
                    height: mainGrid.height
                    Grid {
                        id: mainGrid
                        columns: page.columns
                        rowSpacing: Theme.px(16)
                        columnSpacing: Theme.px(14)
                        Repeater {
                            model: page.cards
                            MediaCard {
                                width: page.cardWidth; height: page.cardHeight
                                itemData: modelData
                                music: !page.videos
                                showDisk: page.showDisks
                                playing: page.isPlaying(modelData)
                                livePosition: playing ? page.playingPosition : -1
                            }
                        }
                    }
                }
            }
        }
    }

    // ---- A folder, an artist or an album: its picture and facts beside its list
    readonly property real totalSeconds: {
        const r = media.detailsRevision;
        return rows.reduce((t, d) => t + (d.type === "video" || d.type === "song" ? (media.details(d.entry.path).duration || 0) : 0), 0);
    }
    readonly property bool manyArtists: {
        const r = media.detailsRevision;
        if (videos || byArtist) return false;
        const seen = [];
        rows.forEach(d => { if (d.type === "song") { const a = media.details(d.entry.path).artist || ""; if (seen.indexOf(a) < 0) seen.push(a); } });
        return seen.length > 1;
    }
    readonly property var side: {
        if (mode !== "list")
            return null;
        const songsText = n => Tr.trn("media.songs.count", n) + (totalSeconds > 0 ? " · " + MediaText.span(totalSeconds) : "");
        if (byArtist && stack.length === 1) {
            const n = albums.reduce((t, a) => t + a.songs, 0);
            return { title: stack[0] || Tr.tr("media.unknown_artist"), seed: stack[0] || "?", picture: "", square: true, artist: true,
                     facts: [Tr.trn("media.albums.count", albums.length), Tr.trn("media.songs.count", n)] };
        }
        if (byArtist) {
            const al = albums.find(a => a.key === stack[1]) || { title: stack[1], year: 0, picture: "" };
            return { title: al.title || Tr.tr("media.other_songs"), seed: al.title || stack[0] || "?", picture: al.picture, square: true, artist: false,
                     facts: [stack[0] || Tr.tr("media.unknown_artist"), al.year > 0 ? String(al.year) : "", songsText(songs.length)].filter(s => s !== "") };
        }
        if (!listing)
            return null;
        const files = listing.files.length, folders = listing.folders.length;
        const counts = [folders > 0 ? Tr.trn("media.folders.count", folders) : "",
                        files > 0 ? Tr.trn(videos ? "media.videos.count" : "media.songs.count", files) : ""].filter(s => s !== "").join(" · ");
        return { title: listing.name || "", seed: (videos ? media.trail(folderPath).split(" › ")[0] : listing.name) || "?", picture: listing.picture || "",
                 square: !videos, artist: false,
                 facts: [counts, totalSeconds > 0 ? MediaText.span(totalSeconds) : "",
                         page.showDisks ? Tr.tr("media.on_disk", { disk: listing.diskName || Tr.tr("disk.internal") }) : ""].filter(s => s !== "") };
    }
    // beside a folder of videos: the selected one's languages and where it stopped
    readonly property var selectedVideo: {
        const r = media.detailsRevision;
        return mode === "list" && videos && selected && selected.type === "video" ? media.details(selected.entry.path) : null;
    }
    Item {
        id: listArea
        visible: page.mode === "list"
        anchors { left: parent.left; right: parent.right; top: header.bottom; topMargin: Theme.px(18); bottom: parent.bottom }
        Column {
            id: sidePanel
            width: Theme.px(340)
            spacing: Theme.px(10)
            Item {
                width: page.side && page.side.square ? Theme.px(240) : parent.width
                height: page.side && page.side.square ? width : width * 9 / 16
                GameArt {
                    anchors.fill: parent
                    radius: Theme.radiusL
                    seed: page.side ? (page.side.seed || "") : ""
                    image: page.side ? (page.side.picture || "") : ""
                }
                Text {
                    visible: page.side !== null && page.side.artist
                    anchors.centerIn: parent
                    text: page.side && page.side.artist ? (page.side.title || "").split(" ").filter(w => w !== "").map(w => w[0]).slice(0, 2).join("") : ""
                    color: "#FFFFFF"
                    font.family: Theme.displayFont
                    font.weight: Font.ExtraBold
                    font.pixelSize: Theme.fs(72)
                }
            }
            Text {
                width: parent.width
                text: page.side ? (page.side.title || "") : ""
                color: Theme.fg
                wrapMode: Text.Wrap
                maximumLineCount: 2
                elide: Text.ElideRight
                font.family: Theme.displayFont
                font.weight: Font.Bold
                font.pixelSize: Theme.fs(24)
            }
            Repeater {
                model: page.side ? page.side.facts : []
                Text {
                    width: sidePanel.width
                    text: modelData
                    color: Theme.muted
                    elide: Text.ElideRight
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                }
            }
            Rectangle {
                visible: page.selectedVideo !== null
                width: parent.width; height: Math.max(1, Theme.px(1))
                color: Theme.line
            }
            Column {
                visible: page.selectedVideo !== null
                width: parent.width
                spacing: Theme.px(3)
                readonly property var v: page.selectedVideo
                Text {
                    width: parent.width
                    text: page.selected && page.selected.entry ? (page.selected.entry.name || "") : ""
                    color: Theme.fg
                    elide: Text.ElideRight
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(17)
                }
                Text {
                    visible: text !== ""
                    width: parent.width
                    text: !parent.v ? "" : parent.v.position > 0 && parent.v.duration > 0
                          ? Tr.tr("media.stopped", { t: MediaText.clock(parent.v.position), d: MediaText.clock(parent.v.duration) })
                          : parent.v.duration > 0 ? MediaText.span(parent.v.duration) : ""
                    color: Theme.muted
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                }
                Text {
                    readonly property string names: parent.v ? MediaText.languages(parent.v.audio) : ""
                    visible: names !== ""
                    width: parent.width
                    text: Tr.tr("media.audio", { languages: names })
                    color: Theme.muted
                    elide: Text.ElideRight
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                }
                Text {
                    readonly property string names: parent.v ? MediaText.languages(parent.v.subs) : ""
                    visible: parent.v !== null && parent.v.known
                    width: parent.width
                    text: names !== "" ? Tr.tr("media.subtitles", { languages: names })
                          : parent.v && parent.v.subs.length > 0 ? Tr.tr("media.subtitles.some") : Tr.tr("media.subtitles.none")
                    color: Theme.muted
                    elide: Text.ElideRight
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                }
            }
        }
        ListView {
            id: rowList
            anchors { left: sidePanel.right; leftMargin: Theme.px(30) - page.ring; right: parent.right; rightMargin: -page.ring; top: parent.top; topMargin: -page.ring; bottom: parent.bottom }
            topMargin: page.ring; bottomMargin: page.ring
            clip: true
            focus: true
            spacing: Theme.px(8)
            model: page.rows
            keyNavigationEnabled: false             // Nav.listKeys moves the selection
            highlightRangeMode: ListView.ApplyRange
            preferredHighlightBegin: page.ring
            preferredHighlightEnd: height - page.ring
            highlightMoveDuration: Theme.motionMs
            delegate: MediaRow {
                x: page.ring
                width: rowList.width - 2 * page.ring
                itemData: modelData
                music: !page.videos
                showArtist: page.manyArtists
                playing: page.isPlaying(modelData)
                livePosition: playing ? page.playingPosition : -1
                focus: true
            }
            onCurrentIndexChanged: if (page.mode === "list") page.remember(page.rows[currentIndex] || null)
            Keys.onPressed: {
                const a = Nav.action(event);
                const data = page.rows[currentIndex] || null;
                if (Nav.listKeys(rowList, a)) {
                    // moved (or at the end)
                } else if (a === "left" || a === "right") {
                    Nav.feedback("edge");
                } else if (a === "accept" && data) {
                    Nav.feedback("confirm");
                    page.activate(data);
                } else if (a === "options") {
                    if (!page.optionsFor(data)) Nav.feedback("edge");
                } else {
                    return;
                }
                event.accepted = true;
            }
        }
    }

    // ---- Nothing yet: what to do
    Column {
        visible: page.mode === "empty"
        anchors { horizontalCenter: parent.horizontalCenter; verticalCenter: parent.verticalCenter; verticalCenterOffset: Theme.px(20) }
        width: Theme.px(660)
        spacing: Theme.px(12)
        Text {
            width: parent.width
            horizontalAlignment: Text.AlignHCenter
            text: Tr.tr(page.videos ? "media.videos.empty" : "media.music.empty")
            color: Theme.fg
            font.family: Theme.displayFont
            font.weight: Font.Bold
            font.pixelSize: Theme.fs(24)
        }
        Text {
            width: parent.width
            horizontalAlignment: Text.AlignHCenter
            wrapMode: Text.Wrap
            text: Tr.tr(page.videos ? "media.videos.empty.text" : "media.music.empty.text")
            color: Theme.fg
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(17)
        }
        Rectangle {
            anchors.horizontalCenter: parent.horizontalCenter
            width: Math.max(Theme.px(300), tree.width + Theme.px(44))
            height: tree.height + Theme.px(28)
            radius: Theme.radiusM
            color: Theme.surface
            border.color: Theme.line
            border.width: Math.max(1, Theme.px(1))
            Column {
                id: tree
                x: Theme.px(22); y: Theme.px(14)
                spacing: Theme.px(3)
                Repeater {
                    // a USB stick: its media folder at the top, and what goes in it
                    model: page.videos
                        ? [["media.tree.stick", 0, "dir"], ["media.tree.videos", 1, "here"], ["media.tree.holiday", 2, "dir"],
                           ["media.tree.beach", 3, "file"], ["media.tree.film", 2, "file"]]
                        : [["media.tree.stick", 0, "dir"], ["media.tree.music", 1, "here"], ["media.tree.artist", 2, "dir"],
                           ["media.tree.album", 3, "dir"], ["media.tree.song", 4, "file"]]
                    Text {
                        leftPadding: modelData[1] * Theme.px(22)
                        text: (modelData[1] > 0 ? "└ " : "") + Tr.tr(modelData[0])
                        color: modelData[2] === "here" ? Theme.accent : modelData[2] === "dir" ? Theme.fg : Theme.muted
                        font.family: Theme.textFont
                        font.weight: modelData[2] === "file" ? Font.Normal : modelData[2] === "here" ? Font.DemiBold : Font.Medium
                        font.pixelSize: Theme.fs(14.5)
                    }
                }
            }
        }
    }

    // ---- The first visit of Artists: the songs are read
    Column {
        visible: page.mode === "reading"
        anchors.centerIn: parent
        spacing: Theme.px(12)
        Spinner {
            anchors.horizontalCenter: parent.horizontalCenter
            width: Theme.fs(46); height: width
        }
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            text: Tr.tr("media.reading")
            color: Theme.fg
            font.family: Theme.displayFont
            font.weight: Font.Bold
            font.pixelSize: Theme.fs(24)
        }
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            text: Tr.tr("media.reading.count", { read: media.tagsRead, total: media.tagsTotal })
            color: Theme.fg
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(17)
        }
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            text: Tr.tr("media.reading.text")
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(14.5)
        }
    }
}
