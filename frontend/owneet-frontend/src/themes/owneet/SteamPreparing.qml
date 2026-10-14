// SPDX-License-Identifier: GPL-3.0-or-later
// While Steam starts ([[step-4.2-steam-integration]]): Steam shows nothing until its window is
// ready, which takes a few minutes the first time (it downloads about 2.4 GB). The shell closes
// this screen when Steam's or the game's window appears, or when Steam stops; B hides it and Steam
// goes on.
import QtQuick 2.15
import "foundation"

NavArea {
    id: root
    anchors.fill: parent
    visible: false
    z: 140
    backFeedback: ""

    property bool firstTime: false          // Steam has never started here: the long download

    function start(first) {
        firstTime = first;
        if (!visible)
            Nav.feedback("sheet-open");
        visible = true;
        forceActiveFocus();
    }
    function close(quiet) {
        if (!visible)
            return;
        if (!quiet)
            Nav.feedback("sheet-close");
        visible = false;
    }
    onCancelled: close()

    Rectangle {
        anchors.fill: parent
        color: Theme.bg
    }
    Column {
        anchors.centerIn: parent
        width: Math.min(parent.width - 2 * Theme.safeX, Theme.px(640))
        spacing: Theme.px(18)
        Spinner {
            anchors.horizontalCenter: parent.horizontalCenter
            width: Theme.px(44); height: width
        }
        Text {
            width: parent.width
            horizontalAlignment: Text.AlignHCenter
            text: Tr.tr("steam.prep.title")
            color: Theme.fg
            font.family: Theme.displayFont
            font.weight: Font.Bold
            font.pixelSize: Theme.fs(24)
            wrapMode: Text.Wrap
        }
        Text {
            width: parent.width
            horizontalAlignment: Text.AlignHCenter
            text: Tr.tr(root.firstTime ? "steam.prep.first" : "steam.prep.text")
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(17)
            wrapMode: Text.Wrap
        }
    }
    PromptBar {
        anchors {
            left: parent.left; right: parent.right; bottom: parent.bottom
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; bottomMargin: Theme.safeBottom
        }
        prompts: [{ buttons: ["b"], label: Tr.tr("prompt.hide") }]
    }
}
