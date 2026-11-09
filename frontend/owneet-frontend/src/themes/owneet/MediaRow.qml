// SPDX-License-Identifier: GPL-3.0-or-later
// A row of a folder's, an artist's or an album's list (approved demo
// design/demos/5.6-videos-and-music.html): a folder and what it holds, a video and its length (a
// started one: what is left, with its progress along the bottom), a song with its track number and
// length (its artist too in a folder of several artists), an album with its picture, year and songs.
import QtQuick 2.15
import "foundation"

Rectangle {
    id: root
    property var itemData: null                 // { type: folder, video, song or album; id; entry }
    property bool music: false
    property bool showArtist: false
    readonly property string type: itemData ? itemData.type : ""
    readonly property var entry: itemData ? itemData.entry : null
    readonly property var details: {
        const r = media.detailsRevision;
        return entry && (type === "video" || type === "song") ? media.details(entry.path) : null;
    }
    readonly property real position: details && details.position ? details.position : 0
    readonly property real duration: details ? details.duration : 0
    readonly property string title: !entry ? ""
        : type === "song" ? (details && details.title ? details.title : entry.name)
        : type === "album" ? (entry.title || Tr.tr("media.other_songs"))
        : entry.name
    readonly property string subtitle: {
        if (!entry)
            return "";
        if (type === "folder")
            return [entry.folders > 0 ? Tr.trn("media.folders.count", entry.folders) : "",
                    entry.files > 0 ? Tr.trn(music ? "media.songs.count" : "media.videos.count", entry.files) : ""]
                .filter(s => s !== "").join(" · ");
        if (type === "album")
            return [entry.year > 0 ? String(entry.year) : "", Tr.trn("media.songs.count", entry.songs)].filter(s => s !== "").join(" · ");
        if (type === "video" && position > 0 && duration > 0)
            return Tr.tr("media.left", { t: MediaText.span(duration - position) });
        if (type === "song" && showArtist && details && details.artist)
            return details.artist;
        return "";
    }
    height: Math.max(Theme.px(56), main.implicitHeight + Theme.px(18))
    radius: Theme.radiusM
    color: Theme.surface
    border.color: Theme.line
    border.width: Math.max(1, Theme.px(1))

    Item {                                      // an icon, the track number, or the album's picture
        id: lead
        anchors { left: parent.left; leftMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
        width: Theme.px(34); height: width
        Rectangle {
            visible: root.type === "folder" || root.type === "video"
            anchors.fill: parent
            radius: Theme.radiusS
            color: Theme.raised
            Icon {
                anchors.centerIn: parent
                width: Theme.px(17); height: width
                name: root.type === "folder" ? "folder" : "play"
                color: Theme.accent
            }
        }
        Text {
            visible: root.type === "song" && root.details !== null && root.details.track > 0
            anchors.centerIn: parent
            text: root.details ? String(root.details.track) : ""
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(14.5)
        }
        Icon {
            visible: root.type === "song" && !(root.details !== null && root.details.track > 0)
            anchors.centerIn: parent
            width: Theme.px(17); height: width
            name: "note"
            color: Theme.muted
        }
        GameArt {
            visible: root.type === "album"
            anchors.fill: parent
            radius: Theme.px(8)
            seed: root.title
            image: root.type === "album" && root.entry ? (root.entry.picture || "") : ""
        }
    }
    Column {
        id: main
        anchors { left: lead.right; leftMargin: Theme.px(14); right: end.left; rightMargin: Theme.px(14); verticalCenter: parent.verticalCenter }
        spacing: Theme.px(1)
        Text {
            width: parent.width
            text: root.title
            color: Theme.fg
            elide: Text.ElideRight
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(17)
        }
        Text {
            visible: text !== ""
            width: parent.width
            text: root.subtitle
            color: Theme.muted
            elide: Text.ElideRight
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(12.5)
        }
    }
    Row {
        id: end
        anchors { right: parent.right; rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
        spacing: Theme.px(12)
        Text {
            visible: text !== ""
            anchors.verticalCenter: parent.verticalCenter
            text: root.duration <= 0 ? "" : root.type === "song" ? MediaText.clock(root.duration)
                  : root.type === "video" ? MediaText.span(root.duration) : ""
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(14.5)
        }
        Icon {
            visible: root.type === "folder" || root.type === "album"
            anchors.verticalCenter: parent.verticalCenter
            width: Theme.px(18); height: width
            name: "right"
            color: Theme.muted
        }
    }
    Rectangle {                                 // a started video: how far it got
        visible: root.type === "video" && root.position > 0 && root.duration > 0
        anchors { left: parent.left; right: parent.right; bottom: parent.bottom; leftMargin: root.radius; rightMargin: root.radius; bottomMargin: Theme.px(4) }
        height: Theme.px(4); radius: height / 2
        color: Theme.raised
        Rectangle {
            width: parent.width * Math.min(1, root.position / Math.max(1, root.duration))
            height: parent.height; radius: height / 2
            color: Theme.accent
        }
    }
    FocusFrame { radius: root.radius }
}
