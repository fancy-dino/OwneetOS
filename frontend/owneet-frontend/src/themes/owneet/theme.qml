// SPDX-License-Identifier: GPL-3.0-or-later
// The OwneetOS theme. Step 3.3 builds its foundations (design tokens and palettes, fonts, the
// 1280 x 720 grid with the TV safe area, focus ring, button prompts, text sizes, reduce motion)
// as approved in design/demos/3.3-theme-foundations.html. Until the home screen (3.6) and the
// library (3.7) exist, the screen lists the games and starts the selected one with A.
import QtQuick 2.15
import "foundation"

FocusScope {
    id: root
    focus: true

    // ---- Preferences, saved in the theme's memory (moved to Settings in step 3.9)
    Binding { target: Theme; property: "width"; value: root.width }
    Binding { target: Theme; property: "height"; value: root.height }
    readonly property var prefs: [
        ["palette", "paletteKey"], ["textScale", "textScale"], ["glyphs", "glyphSetting"],
        ["reduceMotion", "reduceMotion"], ["showSafeArea", "showSafeArea"]
    ]
    property bool loaded: false
    Component.onCompleted: {
        prefs.forEach(p => { if (api.memory.has(p[0])) Theme[p[1]] = api.memory.get(p[0]); });
        loaded = true;
        updatePad();
    }
    function save(key, value) {
        if (loaded && api.memory.get(key) !== value)
            api.memory.set(key, value);
    }
    Connections {
        target: Theme
        function onPaletteKeyChanged() { save("palette", Theme.paletteKey); }
        function onTextScaleChanged() { save("textScale", Theme.textScale); }
        function onGlyphSettingChanged() { save("glyphs", Theme.glyphSetting); }
        function onReduceMotionChanged() { save("reduceMotion", Theme.reduceMotion); }
        function onShowSafeAreaChanged() { save("showSafeArea", Theme.showSafeArea); }
    }

    // ---- Controller: its name, and the button prompts that match it
    readonly property var pads: Internal.gamepad.devices
    property string padName: ""
    function updatePad() {
        const pad = pads.count > 0 ? pads.get(0) : null;
        padName = pad ? pad.name : "";
        // By the names SDL and the kernel give the controllers: PlayStation ("PS5 Controller",
        // "DualSense Wireless Controller", "Sony ... Wireless Controller"), Nintendo ("Nintendo
        // Switch Pro Controller", "Joy-Con"); everything else (Xbox, generic pads) gets the
        // Xbox-style letters.
        if (pad) {
            const n = pad.name;
            if (/xbox|microsoft/i.test(n))
                Theme.padKind = "xbox";
            else if (/nintendo|switch|joy-?con/i.test(n))
                Theme.padKind = "nintendo";
            else if (/sony|dualsense|dualshock|playstation|wireless controller|\bps[345]\b/i.test(n))
                Theme.padKind = "ps";
            else
                Theme.padKind = "xbox";
        }
    }
    Connections {
        target: root.pads
        function onCountChanged() { root.updatePad(); }
    }
    Connections {             // the name can arrive after the controller
        target: root.pads.count > 0 ? root.pads.get(0) : null
        function onNameChanged() { root.updatePad(); }
    }
    Component.onDestruction: Theme.previewKey = ""

    // ---- Buttons the screens leave to the whole interface (section 9.1)
    readonly property bool sheetOpen: appearance.visible || palettes.visible || languages.visible
    Keys.onPressed: {
        const a = Nav.action(event);
        if (a === "page" && !sheetOpen) {
            appearance.openSheet(true);   // Y: page option (temporary home of Appearance)
        } else if (a === "section-prev" || a === "section-next" || a === "filter-prev" || a === "filter-next") {
            Nav.feedback("edge");         // one section and no filters yet (home: 3.6)
        } else {
            return;
        }
        event.accepted = true;
    }

    Rectangle {
        anchors.fill: parent
        color: Theme.bg
    }

    // ---- Top bar: wordmark, controller, clock
    Item {
        id: top
        anchors {
            left: parent.left; right: parent.right; top: parent.top
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; topMargin: Theme.safeTop
        }
        height: Math.max(mark.height, Theme.fs(30))

        Text {
            id: mark
            anchors.verticalCenter: parent.verticalCenter
            textFormat: Text.StyledText
            text: "Owneet<font color=\"" + Theme.accent + "\">OS</font>"
            color: Theme.fg
            font.family: Theme.displayFont
            font.weight: Font.ExtraBold
            font.pixelSize: Theme.px(23)
            font.letterSpacing: -Theme.px(0.7)
        }
        Row {
            anchors { right: parent.right; verticalCenter: parent.verticalCenter }
            spacing: Theme.px(20)
            Text {
                text: root.padName !== "" ? root.padName : Tr.tr("controller.none")
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(14.5)
                elide: Text.ElideRight
                width: Math.min(implicitWidth, Theme.px(300))
            }
            Text {
                id: clock
                color: Theme.fg
                font.family: Theme.textFont
                font.weight: Font.Medium
                font.pixelSize: Theme.fs(14.5)
                function tick() { text = Qt.formatTime(new Date(), "hh:mm"); }
                Component.onCompleted: tick()
                Timer { interval: 10000; running: true; repeat: true; onTriggered: clock.tick() }
            }
        }
    }

    // ---- Games (temporary list until the home screen, step 3.6)
    Text {
        id: heading
        anchors {
            left: parent.left; top: top.bottom
            leftMargin: Theme.safeX; topMargin: Theme.px(22)
        }
        text: Tr.tr("games.title")
        color: Theme.fg
        font.family: Theme.displayFont
        font.weight: Font.Bold
        font.pixelSize: Theme.fs(40)
    }
    Text {
        visible: games.count > 0
        anchors { left: heading.right; leftMargin: Theme.px(14); baseline: heading.baseline }
        text: Tr.trn("games.count", games.count)
        color: Theme.muted
        font.family: Theme.textFont
        font.pixelSize: Theme.fs(17)
    }
    ListView {
        id: games
        anchors {
            left: parent.left; right: parent.right; top: heading.bottom; bottom: prompts.top
            leftMargin: Theme.safeX - Theme.px(20); rightMargin: Theme.safeX - Theme.px(20)
            topMargin: Theme.px(10); bottomMargin: Theme.px(14)
        }
        model: api.allGames
        focus: true
        clip: true
        spacing: Theme.px(10)
        topMargin: Theme.px(8)
        bottomMargin: Theme.px(8)
        // the list scrolls to follow the selection; the prompt bar never moves
        highlightRangeMode: ListView.ApplyRange
        preferredHighlightBegin: Theme.px(16)
        preferredHighlightEnd: height - Theme.px(16)
        highlightMoveDuration: Theme.motionMs
        keyNavigationEnabled: false             // Nav.listKeys moves the selection
        delegate: Rectangle {
            readonly property var game: modelData
            x: Theme.px(20)                      // room for the lift and the focus ring
            width: games.width - Theme.px(40)
            height: title.implicitHeight + Theme.px(28)
            radius: Theme.radiusS + Theme.px(2)
            color: Theme.surface
            border.color: Theme.line
            border.width: Math.max(1, Theme.px(1))
            Text {
                id: title
                anchors {
                    left: parent.left; right: parent.right; verticalCenter: parent.verticalCenter
                    leftMargin: Theme.px(18); rightMargin: Theme.px(18)
                }
                text: modelData.title
                color: Theme.fg
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(17)
                elide: Text.ElideRight
            }
            FocusFrame { shown: parent.ListView.isCurrentItem && games.activeFocus }
        }
        Keys.onPressed: {
            const a = Nav.action(event);
            if (Nav.listKeys(games, a)) {
                event.accepted = true;
            } else if (a === "left" || a === "right") {
                Nav.feedback("edge");
                event.accepted = true;
            } else if (a === "accept" && currentItem) {
                Nav.feedback("confirm");
                currentItem.game.launch();
                event.accepted = true;
            }
        }
    }
    Text {
        visible: games.count === 0
        anchors.centerIn: games
        width: games.width * 0.6
        horizontalAlignment: Text.AlignHCenter
        wrapMode: Text.Wrap
        text: Tr.tr("games.empty")
        color: Theme.muted
        font.family: Theme.textFont
        font.pixelSize: Theme.fs(17)
    }

    PromptBar {
        id: prompts
        anchors {
            left: parent.left; right: parent.right; bottom: parent.bottom
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; bottomMargin: Theme.safeBottom
        }
        visible: !root.sheetOpen
        prompts: games.count > 0
                 ? [{ buttons: ["a"], label: Tr.tr("prompt.play") }, { buttons: ["y"], label: Tr.tr("prompt.appearance") }]
                 : [{ buttons: ["y"], label: Tr.tr("prompt.appearance") }]
    }

    // ---- TV safe area outline (Appearance → Show TV safe area)
    Rectangle {
        visible: Theme.showSafeArea
        anchors {
            fill: parent
            leftMargin: Theme.safeX; rightMargin: Theme.safeX
            topMargin: Theme.safeTop; bottomMargin: Theme.safeBottom
        }
        color: "transparent"
        border.color: Theme.muted
        border.width: Math.max(1, Theme.px(1))
        radius: Theme.px(8)
        opacity: 0.7
        z: 50
    }

    AppearanceSheet {
        id: appearance
        z: 100
        // after Qt has finished moving the focus away from the closed window
        onVisibleChanged: if (!visible) Qt.callLater(() => { if (!root.sheetOpen) games.forceActiveFocus(); })
        onOpenPalettes: { close(true); palettes.openPicker(); }
        onOpenLanguages: { close(true); languages.openPicker(); }
    }
    PalettePicker {
        id: palettes
        z: 101
        onVisibleChanged: if (!visible) appearance.openSheet(false, true)
    }
    LanguagePicker {
        id: languages
        z: 101
        onVisibleChanged: if (!visible) appearance.openSheet(false, true)
    }
}
