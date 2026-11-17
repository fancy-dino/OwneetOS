// SPDX-License-Identifier: GPL-3.0-or-later
// Options on a local game → "Add cover image" / "Change cover image"
// ([[step-6.9-cover-images-from-a-disk]], approved demo 6.9-cover-images): the pictures of the
// disks plugged in by name (a sub-folder's name in front of the file's), scrolling as the
// credits; on the right the selected one as the cover will look. A uses it (the console copies it
// into the game's folder: coverImages.use), B goes back.
import QtQuick 2.15
import "foundation"

Sheet {
    id: root
    title: Tr.tr("covers.title", { g: game ? game.title : "" })
    subtitle: disks.length === 0 ? "" : Tr.trn("covers.from", pictures.length, { disk: disks.join(", "), n: pictures.length })
    prompts: [{ buttons: ["a"], label: Tr.tr("prompt.use_picture") }, { buttons: ["b"], label: Tr.tr("prompt.cancel") }]
    topMargin: 40
    bottomMargin: 82
    asideWidth: 300

    property var game: null
    property var pictures: []
    property string selected: ""        // the selected picture's path
    readonly property var disks: {
        const out = [];
        pictures.forEach(p => { if (out.indexOf(p.disk) < 0) out.push(p.disk); });
        return out;
    }
    readonly property var current: pictures.find(p => p.path === selected) || null
    signal chosen(var forGame, string picture)

    function openFor(g, list) {
        game = g;
        pictures = list;
        selected = list.length ? list[0].path : "";
        flickable.contentY = 0;
        open();
        Qt.callLater(focusFirst);
    }
    // The pictures read again while the window is open (a disk plugged in or out): the same
    // picture stays selected when it is still there
    function update(list) {
        if (!visible || JSON.stringify(list) === JSON.stringify(pictures))
            return;
        if (list.length === 0) {
            close(true);
            return;
        }
        const keep = selected;
        pictures = list;
        Qt.callLater(() => {
            rows.forceLayout();
            for (let i = 0; i < rows.children.length; i++)
                if (rows.children[i].path === keep) { focusItem(rows.children[i]); return; }
            focusFirst();
        });
    }
    onMoved: if (item && item.path !== undefined) selected = item.path
    onAccepted: {
        if (item.path === undefined)
            return;
        close(true);
        chosen(game, item.path);
    }

    Column {
        id: rows
        width: parent.width
        spacing: Theme.px(6)
        Repeater {
            model: root.visible ? root.pictures : []
            Rectangle {
                readonly property string path: modelData.path
                readonly property bool navigable: true
                width: parent.width
                height: Math.max(name.implicitHeight, kind.implicitHeight) + Theme.px(20)
                radius: Theme.radiusS + Theme.px(2)
                color: Theme.surface
                onActiveFocusChanged: if (activeFocus) root.selected = path
                Text {
                    id: name
                    anchors { left: parent.left; right: kind.left; leftMargin: Theme.px(14); rightMargin: Theme.px(16)
                              verticalCenter: parent.verticalCenter }
                    // with pictures on more than one disk, the disk's name first
                    readonly property var folders: (root.disks.length > 1 ? [modelData.disk] : [])
                                                   .concat(modelData.folder ? modelData.folder.split("/") : [])
                    textFormat: Text.StyledText
                    text: (folders.length ? "<font color=\"" + Theme.muted + "\">" + esc(folders.join(" / ")) + " / </font>" : "")
                          + esc(modelData.name)
                    function esc(s) { return String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;"); }
                    color: Theme.fg
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(17)
                    elide: Text.ElideLeft
                }
                Text {
                    id: kind
                    anchors { right: parent.right; rightMargin: Theme.px(14); verticalCenter: parent.verticalCenter }
                    text: String(modelData.name).split(".").pop().toUpperCase()
                    color: Theme.muted
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                }
                FocusFrame {}
            }
        }
    }

    aside: [
        Column {
            width: parent.width
            spacing: Theme.px(10)
            Cover {
                anchors.horizontalCenter: parent.horizontalCenter
                width: Theme.px(240)
                height: Theme.px(300)
                navigable: false
                game: root.game
                picture: root.current ? root.current.url : ""
            }
            Text {
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                text: Tr.tr("covers.preview")
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(14.5)
                wrapMode: Text.Wrap
            }
        }
    ]
}
