// SPDX-License-Identifier: GPL-3.0-or-later
// The command legend ([[step-5.6a-command-legend]], approved demo
// design/demos/5.6a-command-legend.html, [[controller-input-map]] "The command legend"): what each
// button does in the app on screen, for the screen on show (foundation/legends.js, one source with
// the tests of the extension and the player). View, in the app, opens it: the app's own integration
// asks owneetd ("legend.changed"); the console still intercepts only Guide.
//  - gamescope (`layered`): a panel floating at the top right of the app, in a window of its own
//    that gamescope shows as an external overlay: never focused and never given the controller, so
//    the app keeps working under it. Only View closes it (in the app), and owneetd closes it when
//    something else takes the screen (Guide). Too tall for the screen (large text, a long legend):
//    wider, in two columns.
//  - cage (one window at a time): a page of the console in front, its rows in two cards; B or View
//    closes it (`closed`), and owneetd brings the app back.
import QtQuick 2.15
import QtQuick.Window 2.15
import "foundation"
import "foundation/legends.js" as Legends

FocusScope {
    id: root
    anchors.fill: parent
    visible: false                       // the page (cage)
    z: 162

    property bool layered: false         // gamescope: over the app
    property bool overlayMade: false     // the window is a gamescope overlay (before its first showing)
    property bool shown: false           // over the app (gamescope)
    property string appName: ""
    property string context: ""
    signal closed()                      // B or View on the page

    readonly property var rows: Legends.LEGENDS[context] || []
    readonly property string screenName: {
        const keys = { "netflix.browse": "legend.screen.browse", "netflix.title": "legend.screen.title",
                       "netflix.player": "legend.screen.player", "player.video": "legend.screen.video",
                       "player.song": "legend.screen.song" };
        return keys[context] ? Tr.tr(keys[context]) : "";
    }

    function show(state, name) {
        appName = name;
        context = state.context || "";
        if (layered) {
            if (!overlayMade) {
                overlay.makeOverlay(overWindow);
                overlayMade = true;
            }
            if (!shown)
                Nav.feedback("sheet-open");
            shown = true;
            overlay.showOverlay(overWindow);
        } else {
            if (!visible)
                Nav.feedback("sheet-open");
            visible = true;
            forceActiveFocus();
        }
    }
    function hide() {
        if (shown) {
            overlay.hideOverlay(overWindow);     // transparent first: gamescope repaints without it
            shown = false;
            Nav.feedback("sheet-close");
        }
        if (visible) {
            visible = false;
            Nav.feedback("sheet-close");
        }
    }

    // The page (cage): B or View closes it; the other buttons do nothing here
    Keys.onPressed: {
        const a = Nav.action(event);
        if (Nav.isView(event) || a === "back")
            root.closed();
        else if (a !== "")
            Nav.feedback("edge");
        event.accepted = true;
    }

    // A row: the buttons, then what they do (a row without buttons is a line of its own)
    component Row_: Item {
        property var buttons: []
        property string label: ""
        property color ink: Theme.fg
        property real keysWidth: Theme.fs(86)
        property real size: Theme.fs(14.5)
        implicitHeight: Math.max(keys.height, says.implicitHeight)
        Row {
            id: keys
            visible: parent.buttons.length > 0
            spacing: Theme.px(4)
            anchors.verticalCenter: parent.verticalCenter
            Repeater {
                model: parent.parent.buttons
                Glyph { button: modelData; color: Theme.fg }
            }
        }
        Text {
            id: says
            anchors { left: parent.left; leftMargin: parent.buttons.length > 0 ? parent.keysWidth + Theme.px(12) : 0
                      right: parent.right; verticalCenter: parent.verticalCenter }
            text: parent.label
            color: parent.buttons.length > 0 ? parent.ink : Theme.muted
            wrapMode: Text.Wrap
            font.family: Theme.textFont
            font.pixelSize: parent.size
        }
    }

    // ---- cage: the page
    Rectangle {
        anchors.fill: parent
        color: Theme.bg

        Text {
            id: pageMark
            anchors { left: parent.left; top: parent.top; leftMargin: Theme.safeX; topMargin: Theme.safeTop }
            textFormat: Text.StyledText
            text: "Owneet<font color=\"" + Theme.accent + "\">OS</font>"
            color: Theme.fg
            font.family: Theme.displayFont
            font.weight: Font.ExtraBold
            font.pixelSize: Theme.px(23)
            font.letterSpacing: -Theme.px(0.7)
        }
        Text {
            anchors { right: parent.right; rightMargin: Theme.safeX; verticalCenter: pageMark.verticalCenter }
            color: Theme.fg
            font.family: Theme.textFont
            font.weight: Font.Medium
            font.pixelSize: Theme.fs(14.5)
            function tick() { text = Qt.formatTime(new Date(), "hh:mm"); }
            Component.onCompleted: tick()
            Timer { interval: 10000; running: root.visible; repeat: true; triggeredOnStart: true; onTriggered: parent.tick() }
        }
        Row {
            id: pageTitle
            anchors { left: parent.left; top: pageMark.bottom; leftMargin: Theme.safeX; topMargin: Theme.px(22) }
            spacing: Theme.px(14)
            Text {
                text: Tr.tr("legend.title") + " · " + root.appName
                color: Theme.fg
                font.family: Theme.displayFont
                font.weight: Font.Bold
                font.pixelSize: Theme.fs(40)
            }
            Text {
                anchors.baseline: parent.children[0].baseline
                visible: text !== ""
                text: root.screenName
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(17)
            }
        }
        Item {
            anchors { left: parent.left; right: parent.right; top: pageTitle.bottom; bottom: pagePrompts.top
                      leftMargin: Theme.safeX; rightMargin: Theme.safeX; topMargin: Theme.px(18); bottomMargin: Theme.px(18) }
            readonly property int half: Math.ceil(root.rows.length / 2)
            Repeater {
                model: 2
                Rectangle {
                    readonly property var part: index === 0 ? root.rows.slice(0, parent.half) : root.rows.slice(parent.half)
                    visible: part.length > 0
                    x: index * (parent.width + Theme.px(56)) / 2
                    width: (parent.width - Theme.px(56)) / 2
                    height: Math.min(parent.height, cardRows.implicitHeight + Theme.px(36))
                    radius: Theme.radiusL
                    color: Theme.surface
                    border.color: Theme.line
                    border.width: Math.max(1, Theme.px(1))
                    Column {
                        id: cardRows
                        anchors { left: parent.left; right: parent.right; top: parent.top; margins: Theme.px(18); leftMargin: Theme.px(22); rightMargin: Theme.px(22) }
                        spacing: Theme.px(11)
                        Repeater {
                            model: parent.parent.part
                            Row_ {
                                width: cardRows.width
                                buttons: modelData[0]
                                label: Tr.tr(modelData[1], { app: root.appName })
                                size: Theme.fs(17)
                            }
                        }
                    }
                }
            }
        }
        PromptBar {
            id: pagePrompts
            anchors { left: parent.left; right: parent.right; bottom: parent.bottom
                      leftMargin: Theme.safeX; rightMargin: Theme.safeX; bottomMargin: Theme.safeBottom }
            prompts: [{ buttons: ["b", "view"], label: Tr.tr("legend.back", { app: root.appName }) }]
            globalPrompts: [{ buttons: ["guide"], label: Tr.tr("legend.hold", { app: root.appName }) }]
        }
    }

    // ---- gamescope: the panel over the app (a window of its own, an external overlay)
    Window {
        id: overWindow
        visible: false
        title: "OwneetOS command legend"
        flags: Qt.FramelessWindowHint
        color: "transparent"
        width: Screen.width
        height: Screen.height

        // the rows as one column, measured: too tall for the screen, the panel takes two columns
        readonly property real singleWidth: Theme.px(400) * (0.75 + Theme.textScale / 4)
        readonly property bool wide: measure.implicitHeight > Theme.px(648) - panelHead.height - panelFoot.height - Theme.px(60)
        Column {
            id: measure
            opacity: 0                   // laid out (an invisible item's children are not), never drawn
            enabled: false
            width: overWindow.singleWidth - Theme.px(44)
            spacing: Theme.px(9)
            Repeater {
                model: root.rows
                Row_ { width: measure.width; buttons: modelData[0]; label: Tr.tr(modelData[1], { app: root.appName }) }
            }
        }

        Rectangle {
            id: panel
            anchors { top: parent.top; right: parent.right; topMargin: Theme.safeTop; rightMargin: Theme.px(40) }
            width: overWindow.wide ? overWindow.singleWidth * 1.9 : overWindow.singleWidth
            height: content.implicitHeight + Theme.px(34)
            radius: Theme.radiusL
            color: Qt.rgba(Theme.bg.r, Theme.bg.g, Theme.bg.b, 0.9)
            border.color: Theme.line
            border.width: Math.max(1, Theme.px(1))

            Column {
                id: content
                anchors { left: parent.left; right: parent.right; top: parent.top
                          leftMargin: Theme.px(22); rightMargin: Theme.px(22); topMargin: Theme.px(18) }
                spacing: Theme.px(10)
                Column {
                    id: panelHead
                    width: parent.width
                    spacing: Theme.px(2)
                    Label { text: Tr.tr("legend.title") }
                    Text {
                        width: parent.width
                        text: root.appName
                        color: Theme.fg
                        elide: Text.ElideRight
                        font.family: Theme.displayFont
                        font.weight: Font.Bold
                        font.pixelSize: Theme.fs(24)
                    }
                    Text {
                        visible: text !== ""
                        text: root.screenName
                        color: Theme.muted
                        font.family: Theme.textFont
                        font.pixelSize: Theme.fs(14.5)
                    }
                }
                Grid {
                    id: panelRows
                    width: parent.width
                    columns: overWindow.wide ? 2 : 1
                    columnSpacing: Theme.px(28)
                    rowSpacing: Theme.px(9)
                    Repeater {
                        model: root.rows
                        Row_ {
                            width: (panelRows.width - (panelRows.columns - 1) * panelRows.columnSpacing) / panelRows.columns
                            buttons: modelData[0]
                            label: Tr.tr(modelData[1], { app: root.appName })
                        }
                    }
                }
                Column {
                    id: panelFoot
                    width: parent.width
                    spacing: Theme.px(9)
                    Rectangle { width: parent.width; height: Math.max(1, Theme.px(1)); color: Theme.line }
                    Row_ { width: parent.width; buttons: ["view"]; label: Tr.tr("legend.close"); ink: Theme.muted }
                    Row_ { width: parent.width; buttons: ["guide"]; label: Tr.tr("prompt.home"); ink: Theme.muted }
                }
            }
        }
    }
}
