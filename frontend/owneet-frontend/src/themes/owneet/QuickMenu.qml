// SPDX-License-Identifier: GPL-3.0-or-later
// The quick menu (roadmap 3.13, approved demo design/demos/3.13-startup-power.html): a side panel
// from the right, as on SteamOS, opened by B where there is nothing to go back to (Home, Library,
// the list of Settings sections; PROJECT_RULES.md section 9.1). Power options and "Controls and
// features"; B closes it.
import QtQuick 2.15
import "foundation"

NavArea {
    id: root
    anchors.fill: parent
    visible: false
    z: 102
    backFeedback: ""                // closing plays "sheet-close"

    property bool covered: false    // one of its windows is open over it, with its own prompts
    signal openPower()
    signal openControls()

    function openMenu() {
        Nav.feedback("sheet-open");
        visible = true;
        forceActiveFocus();
        const items = Nav.navigables(itemColumn);
        current = null;
        focusItem(items.indexOf(lastItem) >= 0 ? lastItem : items[0]);
    }
    // back from one of its windows: the panel is still open, with its item selected
    function refocus() {
        forceActiveFocus();
        if (lastItem) { current = null; focusItem(lastItem); }
    }
    function close(quiet) {
        if (!visible)
            return;
        if (!quiet)
            Nav.feedback("sheet-close");
        visible = false;
    }
    property Item lastItem: null
    onCancelled: close()
    onAccepted: {
        lastItem = item;
        if (item.action === "power") openPower();
        else if (item.action === "controls") openControls();
    }

    Rectangle {                     // the page behind, dimmed
        anchors.fill: parent
        color: Qt.rgba(Theme.bg.r, Theme.bg.g, Theme.bg.b, 0.55)
    }
    Rectangle {
        id: panel
        anchors { top: parent.top; bottom: parent.bottom; right: parent.right }
        width: Theme.px(380)
        color: Theme.surface
        Rectangle { width: Math.max(1, Theme.px(1)); anchors { left: parent.left; top: parent.top; bottom: parent.bottom } color: Theme.line }
        transform: Translate {
            x: root.visible ? 0 : panel.width
            Behavior on x { enabled: !Theme.reduceMotion; NumberAnimation { duration: 250; easing.type: Easing.OutCubic } }
        }

        Column {
            anchors { left: parent.left; right: parent.right; top: parent.top; margins: Theme.px(28); topMargin: Theme.safeTop }
            spacing: Theme.px(22)
            Text {
                text: Tr.tr("quick.title")
                color: Theme.fg
                font.family: Theme.displayFont
                font.weight: Font.Bold
                font.pixelSize: Theme.fs(24)
            }
            Column {
                id: itemColumn
                width: parent.width
                spacing: Theme.px(10)
                Repeater {
                    model: [["power", "power", "quick.power"], ["controls", "controller", "quick.controls"]]
                    Rectangle {
                        readonly property bool navigable: true
                        readonly property string action: modelData[0]
                        width: itemColumn.width
                        height: Math.max(texts.height, Theme.fs(24)) + Theme.px(28)
                        radius: Theme.radiusM
                        color: Theme.raised
                        Icon {
                            id: itemIcon
                            anchors { left: parent.left; leftMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                            width: Theme.fs(24); height: width
                            name: modelData[1]
                            color: Theme.fg
                        }
                        Column {
                            id: texts
                            anchors { left: itemIcon.right; leftMargin: Theme.px(14); right: arrow.left; rightMargin: Theme.px(10); verticalCenter: parent.verticalCenter }
                            spacing: Theme.px(2)
                            Text {
                                width: parent.width
                                text: Tr.tr(modelData[2])
                                color: Theme.fg
                                font.family: Theme.textFont
                                font.pixelSize: Theme.fs(17)
                                wrapMode: Text.Wrap
                            }
                            Text {
                                width: parent.width
                                text: Tr.tr(modelData[2] + ".detail")
                                color: Theme.muted
                                font.family: Theme.textFont
                                font.pixelSize: Theme.fs(14.5)
                                wrapMode: Text.Wrap
                            }
                        }
                        Text {
                            id: arrow
                            anchors { right: parent.right; rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                            text: "›"
                            color: Theme.muted
                            font.pixelSize: Theme.fs(17)
                        }
                        FocusFrame {}
                    }
                }
            }
        }
        Text {
            anchors { left: parent.left; leftMargin: Theme.px(28); bottom: parent.bottom; bottomMargin: Theme.safeBottom }
            text: "OwneetOS " + (systemInfo.version || "")
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(14.5)
        }
    }
    // the right side of the prompt bar is under the panel: only its left side
    PromptBar {
        anchors {
            left: parent.left; right: panel.left; bottom: parent.bottom
            leftMargin: Theme.safeX; rightMargin: Theme.px(28); bottomMargin: Theme.safeBottom
        }
        visible: !root.covered
        prompts: [{ buttons: ["a"], label: Tr.tr("prompt.select") }, { buttons: ["b"], label: Tr.tr("prompt.close") }]
        globalPrompts: []
    }
}
