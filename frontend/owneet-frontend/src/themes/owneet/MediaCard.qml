// SPDX-License-Identifier: GPL-3.0-or-later
// A card on the Videos and Music pages (approved demo design/demos/5.6-videos-and-music.html): a
// folder, a video, a song or an artist, with its picture (the folder's own, a season's series',
// the song's own) or art drawn from its name. A folder says what it holds, a video its length, a
// started video what is left, with its progress under it; the disk's name shows when the media
// folders are on several disks. What the player plays is marked "Playing", its progress live.
import QtQuick 2.15
import "foundation"

Item {
    id: root
    property var itemData: null                 // { type: folder, video, song or artist; id; entry; cont }
    property bool showDisk: false
    property bool music: false                  // a folder's songs, not videos
    property bool playing: false                // the player plays it
    property real livePosition: -1              // where the player is in it (-1: not playing)
    readonly property bool navigable: true
    readonly property real radius: Theme.radiusM
    readonly property string type: itemData ? itemData.type : ""
    readonly property var entry: itemData ? itemData.entry : null
    readonly property var details: {
        const r = media.detailsRevision;
        return entry && (type === "video" || type === "song") ? media.details(entry.path) : null;
    }
    readonly property real position: livePosition >= 0 ? livePosition : details && details.position ? details.position : 0
    readonly property real duration: details ? details.duration : 0
    readonly property string title: !entry ? ""
        : type === "song" ? (details && details.title ? details.title : entry.name)
        : type === "artist" ? (entry.name || Tr.tr("media.unknown_artist"))
        : entry.name
    readonly property string subtitle: {
        if (!entry)
            return "";
        if (type === "folder")
            return root.counts(entry.folders, entry.files);
        if (type === "artist")
            return Tr.trn("media.albums.count", entry.albums);
        if (duration <= 0)
            return "";
        if (type === "song")
            return MediaText.clock(duration);
        if (itemData.cont && position > 0)      // Continue watching: its series, what is left
            return [entry.trail ? entry.trail.split(" › ")[0] : "", Tr.tr("media.left", { t: MediaText.span(duration - position) })]
                .filter(s => s !== "").join(" · ");
        return MediaText.span(duration);
    }
    function counts(folders, files) {
        return [folders > 0 ? Tr.trn("media.folders.count", folders) : "",
                files > 0 ? Tr.trn(root.music ? "media.songs.count" : "media.videos.count", files) : ""]
            .filter(s => s !== "").join(" · ");
    }
    signal activated()

    GameArt {
        anchors.fill: parent
        radius: root.radius
        // a series' videos share its colours
        seed: !root.entry ? "" : root.type === "video" && root.entry.trail ? root.entry.trail.split(" › ")[0] : root.title
        image: !root.entry ? "" : root.type === "song" ? (root.details && root.details.picture ? root.details.picture : "")
               : (root.entry.picture || "")
        shade: true
    }
    Text {                                      // an artist: the initials of the name
        visible: root.type === "artist"
        anchors { horizontalCenter: parent.horizontalCenter; verticalCenter: parent.verticalCenter; verticalCenterOffset: -parent.height * 0.1 }
        text: root.entry && root.entry.name ? root.entry.name.split(" ").filter(w => w !== "").map(w => w[0]).slice(0, 2).join("") : "?"
        color: "#FFFFFF"
        opacity: 0.9
        font.family: Theme.displayFont
        font.weight: Font.ExtraBold
        font.pixelSize: Theme.fs(52)
    }
    Column {
        anchors { left: parent.left; right: parent.right; bottom: parent.bottom; margins: Theme.px(12) }
        spacing: Theme.px(2)
        Text {
            width: parent.width
            text: root.title
            color: "#FFFFFF"
            font.family: Theme.displayFont
            font.weight: Font.Bold
            font.pixelSize: Theme.fs(17)
            wrapMode: Text.Wrap
            maximumLineCount: 2
            elide: Text.ElideRight
            lineHeight: 0.95
        }
        Text {
            visible: text !== ""
            width: parent.width
            text: root.subtitle
            color: "#FFFFFF"
            opacity: 0.92
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(12.5)
            elide: Text.ElideRight
        }
        Item { width: 1; height: Theme.px(4); visible: progress.visible }
        Rectangle {                             // a started video: how far it got
            id: progress
            visible: root.position > 0 && root.duration > 0
            width: parent.width; height: Theme.px(5); radius: height / 2
            color: "#40FFFFFF"
            Rectangle {
                width: parent.width * Math.min(1, root.position / Math.max(1, root.duration))
                height: parent.height; radius: height / 2
                color: Theme.accent
            }
        }
    }
    Rectangle {                                 // the disk it is on
        visible: root.showDisk && root.entry !== null && root.type === "folder"
        x: Theme.px(8); y: Theme.px(8)
        width: Math.min(diskText.implicitWidth + Theme.px(18), parent.width - Theme.px(50))
        height: diskText.implicitHeight + Theme.px(4)
        radius: height / 2
        color: "#73000000"
        Text {
            id: diskText
            anchors.centerIn: parent
            width: Math.min(implicitWidth, parent.width - Theme.px(18))
            text: root.entry ? (root.entry.diskName || Tr.tr("disk.internal")) : ""
            color: "#FFFFFF"
            elide: Text.ElideRight
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(12.5)
        }
    }
    Rectangle {                                 // playing (5.6): as the running game's badge
        visible: root.playing
        anchors { right: parent.right; top: parent.top; margins: Theme.px(8) }
        width: playingText.implicitWidth + Theme.px(18); height: playingText.implicitHeight + Theme.px(4)
        radius: height / 2
        color: Theme.running
        Text {
            id: playingText
            anchors.centerIn: parent
            text: Tr.tr("media.playing")
            color: Theme.runningInk
            font.family: Theme.textFont
            font.weight: Font.DemiBold
            font.pixelSize: Theme.fs(12.5)
        }
    }
    Rectangle {                                 // folder, video or song
        visible: root.type !== "artist" && !root.playing
        anchors { right: parent.right; top: parent.top; margins: Theme.px(8) }
        width: Theme.px(26); height: width; radius: width / 2
        color: "#73000000"
        Icon {
            anchors.centerIn: parent
            width: Theme.px(14); height: width
            name: root.type === "folder" ? "folder" : root.type === "song" ? "note" : "play"
            color: "#FFFFFF"
        }
    }
    FocusFrame { radius: root.radius }
}
