// SPDX-License-Identifier: GPL-3.0-or-later
// An app's tile, on the home screen's Apps grid and (larger, `big`) on the Apps page
// ([[step-5.7-apps-page]], approved demo design/demos/5.7-apps-page.html; the tile of
// [[step-5.0.0-optional-web-apps]], design/demos/5.0.0-optional-apps.html). Generic tiles, no
// third-party logos: the app's letter or sign. A missing web app is dimmed with a download mark,
// one being installed or removed shows the spinner and its line, the open app a green dot.
// `key`: "videos", "music", "store" or "web:<id>"; ALL_APPS ("allapps") is "All apps", outlined like
// "All games", which opens the Apps page.
import QtQuick 2.15
import "foundation"

Rectangle {
    id: tile
    readonly property bool navigable: true
    property string key: ""
    property var webApp: null                  // a web app's catalogue entry (null until owneetd answers)
    property bool open: false                  // the open web app, or the player of its page
    property bool big: false
    readonly property bool allApps: key === "allapps"
    readonly property string kind: allApps ? "allApps" : "app"
    readonly property bool steamStore: key === "store"
    readonly property string mediaKind: key === "videos" ? "videos" : key === "music" ? "music" : ""
    // generic letters until the catalogue answers
    readonly property var builtIn: ({ "videos": ["app.videos", "▶"], "music": ["app.music", "♪"], "store": ["app.store", "$"],
                                      "web:netflix": ["app.netflix", "N"], "web:youtube": ["app.youtube", "Y"],
                                      "web:spotify": ["app.spotify", "S"] })
    readonly property string nameKey: webApp ? webApp.nameKey : (builtIn[key] ? builtIn[key][0] : "")
    readonly property string sign: webApp ? webApp.icon : (builtIn[key] ? builtIn[key][1] : "")
    readonly property bool busy: webApp !== null && !!webApp.busy
    readonly property bool missing: webApp !== null && !webApp.installed && !webApp.busy
    readonly property real dim: missing ? (activeFocus ? 0.85 : 0.55) : 1
    readonly property real pad: big ? Theme.px(17) : Theme.px(12)

    height: big ? Math.max(Theme.fs(128), name.implicitHeight + icon.height + Theme.px(40))
                : Math.max(Theme.px(78), name.implicitHeight + icon.height + Theme.px(28))
    radius: big ? Theme.radiusL : Theme.radiusM
    color: allApps ? "transparent" : Theme.surface
    border.color: Theme.line
    border.width: allApps ? Theme.px(2) : Math.max(1, Theme.px(1))

    Rectangle {
        id: icon
        visible: !tile.allApps
        x: tile.pad; y: tile.big ? Theme.px(15) : Theme.px(10)
        width: tile.big ? Theme.fs(46) : Theme.fs(30); height: width
        radius: tile.big ? Theme.px(13) : Theme.px(9)
        color: Theme.raised
        opacity: tile.dim
        Text {
            anchors.centerIn: parent
            text: tile.sign
            color: Theme.accent
            font.family: Theme.displayFont
            font.weight: Font.Bold
            font.pixelSize: tile.big ? Theme.fs(22) : Theme.fs(15)
        }
    }
    Rectangle {                 // not installed: download mark
        visible: tile.missing
        anchors { right: parent.right; top: parent.top; rightMargin: tile.pad - Theme.px(2); topMargin: tile.big ? Theme.px(14) : Theme.px(10) }
        width: tile.big ? Theme.fs(26) : Theme.fs(22); height: width; radius: width / 2
        color: Theme.raised
        Icon {
            anchors.centerIn: parent
            width: parent.width * 0.62; height: width
            name: "download"
            color: Theme.fg
        }
    }
    Rectangle {                 // open (5.1, 5.6): green dot, as the running game
        visible: tile.open && !tile.busy
        anchors { right: parent.right; top: parent.top; rightMargin: tile.pad + Theme.px(2); topMargin: tile.big ? Theme.px(20) : Theme.px(16) }
        width: Theme.px(9); height: width; radius: width / 2
        color: Theme.running
    }
    Spinner {                   // installing or removing
        visible: tile.busy
        anchors { right: parent.right; top: parent.top; rightMargin: tile.pad - Theme.px(2); topMargin: tile.big ? Theme.px(14) : Theme.px(10) }
        width: tile.big ? Theme.fs(22) : Theme.fs(18); height: width
    }
    FitText {                   // a long name scrolls while the tile is selected (3.3b)
        id: name
        anchors { left: parent.left; right: parent.right; bottom: parent.bottom; margins: tile.pad; bottomMargin: tile.big ? Theme.px(15) : Theme.px(10) }
        visible: !tile.allApps
        text: tile.busy ? Tr.tr(tile.webApp.busy === "install" ? "apps.installing" : "apps.removing") : Tr.tr(tile.nameKey)
        color: tile.busy ? Theme.muted : Theme.fg
        opacity: tile.dim
        font.family: Theme.textFont
        font.pixelSize: tile.big ? Theme.fs(17) : Theme.fs(14.5)
    }
    Text {                      // "All apps"
        visible: tile.allApps
        anchors { left: parent.left; right: parent.right; verticalCenter: parent.verticalCenter; margins: Theme.px(8) }
        horizontalAlignment: Text.AlignHCenter
        text: Tr.tr("home.all_apps")
        color: Theme.muted
        font.family: Theme.textFont
        font.pixelSize: Theme.fs(14.5)
        wrapMode: Text.Wrap
    }
    FocusFrame {}
}
