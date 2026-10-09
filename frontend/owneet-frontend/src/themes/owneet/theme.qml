// SPDX-License-Identifier: GPL-3.0-or-later
// The OwneetOS theme: the shell around the sections (top bar, sections on LB / RB, prompt bar,
// windows). Foundations: step 3.3 (design/demos/3.3-theme-foundations.html); home: step 3.6
// (design/demos/3.6-home.html). Library (3.7) and Settings (3.9) are temporary pages for now.
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
        refreshGames();
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

    // ---- Games, most recently played first (shared by the sections)
    property var games: []
    function refreshGames() { games = GameInfo.recent(api.allGames); }
    Connections {
        target: api.allGames
        function onCountChanged() { root.refreshGames(); }
    }

    // ---- Sections (LB / RB)
    readonly property var sections: ["home", "library", "settings"]
    property string section: "home"
    readonly property Item page: section === "home" ? home : section === "library" ? library : settings
    function goSection(name) {
        if (name === section)
            return;
        section = name;
        Nav.feedback("section");
        page.focusStart();
    }
    readonly property bool modalOpen: appearance.visible || palettes.visible || languages.visible || dialog.visible
    function backToPage() { Qt.callLater(() => { if (!root.modalOpen) root.page.forceActiveFocus(); }); }

    // ---- Buttons the sections leave to the whole interface (section 9.1)
    Keys.onPressed: {
        const a = Nav.action(event);
        if ((a === "section-prev" || a === "section-next") && !modalOpen) {
            const i = sections.indexOf(section) + (a === "section-next" ? 1 : -1);
            goSection(sections[(i + sections.length) % sections.length]);
        } else if (a.startsWith("section-") || a.startsWith("filter-") || a === "page") {
            Nav.feedback("edge");         // no filters or page options on these pages yet
        } else {
            return;
        }
        event.accepted = true;
    }

    // ---- Games: launch, details, options
    function launch(game) {
        Nav.feedback("launch");
        toast.show(Tr.tr("toast.launch", { title: game.title }));
        game.launch();                    // through owneetd from step 3.8
        refreshTimer.restart();           // "last played" changes once the game has started
    }
    Timer { id: refreshTimer; interval: 3000; onTriggered: root.refreshGames() }
    function showDetails(game) {
        dialog.game = game;
        dialog.mode = "details";
        dialog.title = game.title;
        dialog.text = "";
        dialog.buttons = [{ text: Tr.tr("home.play"), style: "primary", action: () => { dialog.close(true); root.launch(game); } }];
        dialog.defaultIndex = 0;
        dialog.open();
    }
    function showOptions(game) {
        dialog.game = game;
        dialog.mode = "options";
        dialog.title = game.title;
        dialog.text = "";
        dialog.buttons = [
            { text: Tr.tr("home.play"), style: "primary", action: () => { dialog.close(true); root.launch(game); } },
            { text: Tr.tr("home.details"), action: () => { dialog.close(true); root.showDetails(game); } },
            { text: Tr.tr(game.favorite ? "options.unfavorite" : "options.favorite"), action: () => {
                game.favorite = !game.favorite;
                dialog.close();
                toast.show(Tr.tr(game.favorite ? "toast.favorite" : "toast.unfavorite"));
            } },
            { text: Tr.tr("options.library"), action: () => {
                dialog.close(true);
                root.goSection("library");
                library.select(game);
            } }
        ];
        dialog.defaultIndex = 0;
        dialog.open();
    }
    function showHowToAdd() {
        dialog.game = null;
        dialog.mode = "how";
        dialog.title = Tr.tr("how.title");
        dialog.text = Tr.tr("how.steam") + "\n\n" + Tr.tr("how.local");
        dialog.buttons = [{ text: Tr.tr("home.empty.store"), style: "primary", action: () => { dialog.close(); root.notYet(); } }];
        dialog.defaultIndex = 0;
        dialog.open();
    }
    function notYet() { toast.show(Tr.tr("toast.later")); }

    Rectangle {
        anchors.fill: parent
        color: Theme.bg
    }

    // ---- Top bar: wordmark, sections, controller, clock
    Item {
        id: top
        anchors {
            left: parent.left; right: parent.right; top: parent.top
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; topMargin: Theme.safeTop
        }
        height: Math.max(mark.height, tabs.height)

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
            id: tabs
            anchors { left: mark.right; leftMargin: Theme.px(36); verticalCenter: parent.verticalCenter }
            spacing: Theme.px(6)
            Glyph { button: "lb"; anchors.verticalCenter: parent.verticalCenter; color: Theme.muted }
            Repeater {
                model: root.sections
                Rectangle {
                    readonly property bool current: modelData === root.section
                    width: tabText.implicitWidth + Theme.px(36)
                    height: tabText.implicitHeight + Theme.px(14)
                    radius: height / 2
                    color: current ? Theme.fg : "transparent"
                    Text {
                        id: tabText
                        anchors.centerIn: parent
                        text: Tr.tr("tab." + modelData)
                        color: parent.current ? Theme.bg : Theme.muted
                        font.family: Theme.textFont
                        font.weight: parent.current ? Font.Medium : Font.Normal
                        font.pixelSize: Theme.fs(14.5)
                    }
                }
            }
            Glyph { button: "rb"; anchors.verticalCenter: parent.verticalCenter; color: Theme.muted }
        }
        Row {
            anchors { right: parent.right; verticalCenter: parent.verticalCenter }
            spacing: Theme.px(20)
            Canvas {                       // a controller is connected (battery level: 3.8)
                id: padIcon
                visible: root.padName !== ""
                anchors.verticalCenter: parent.verticalCenter
                width: Theme.fs(20); height: width
                property color ink: Theme.muted
                onInkChanged: requestPaint()
                onWidthChanged: requestPaint()
                onPaint: {
                    const ctx = getContext("2d");
                    ctx.reset();
                    ctx.scale(width / 24, height / 24);
                    ctx.strokeStyle = ink;
                    ctx.lineWidth = 2;
                    ctx.lineJoin = "round";
                    ctx.path = "M7 8h10a5 5 0 0 1 4.8 6.3l-.9 3.2a2 2 0 0 1-3.3.9L15 16H9l-2.6 2.4a2 2 0 0 1-3.3-.9l-.9-3.2A5 5 0 0 1 7 8Z";
                    ctx.stroke();
                }
            }
            Text {
                id: clock
                anchors.verticalCenter: parent.verticalCenter
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

    // ---- Sections
    FocusScope {
        id: pages
        focus: true
        anchors {
            left: parent.left; right: parent.right; top: top.bottom; bottom: prompts.top
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; topMargin: Theme.px(22); bottomMargin: Theme.px(20)
        }
        HomePage {
            id: home
            anchors.fill: parent
            visible: root.section === "home"
            focus: visible
            games: root.games
            onLaunch: root.launch(game)
            onDetails: root.showDetails(game)
            onOptions: root.showOptions(game)
            onOpenLibrary: root.goSection("library")
            onHowToAdd: root.showHowToAdd()
            onNotYet: root.notYet()
        }
        LibraryPage {
            id: library
            anchors.fill: parent
            visible: root.section === "library"
            focus: visible
            games: root.games
            onLaunch: root.launch(game)
            onDetails: root.showDetails(game)
            onOptions: root.showOptions(game)
        }
        SettingsPage {
            id: settings
            anchors.fill: parent
            visible: root.section === "settings"
            focus: visible
            onOpenAppearance: appearance.openSheet(true)
        }
    }
    Timer { interval: 0; running: true; onTriggered: home.focusStart() }   // after the first layout

    PromptBar {
        id: prompts
        anchors {
            left: parent.left; right: parent.right; bottom: parent.bottom
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; bottomMargin: Theme.safeBottom
        }
        visible: !root.modalOpen
        prompts: root.page.prompts
        globalPrompts: [{ buttons: ["lb", "rb"], label: Tr.tr("prompt.sections") }, { buttons: ["guide"], label: Tr.tr("prompt.home") }]
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

    // ---- Windows
    Dialog {
        id: dialog
        property var game: null
        property string mode: ""
        onVisibleChanged: if (!visible) root.backToPage()
        Column {                          // details: facts about the game
            visible: dialog.mode === "details" && dialog.game !== null
            anchors.horizontalCenter: parent.horizontalCenter
            spacing: Theme.px(6)
            Repeater {
                model: dialog.game ? [
                    [Tr.tr("details.source"), GameInfo.source(dialog.game)],
                    [Tr.tr("details.last"), GameInfo.lastPlayed(dialog.game)],
                    [Tr.tr("details.time"), GameInfo.played(dialog.game) ? GameInfo.playTime(dialog.game) : "\u2014"]
                ] : []
                Row {
                    spacing: Theme.px(18)
                    anchors.horizontalCenter: parent.horizontalCenter
                    Text { text: modelData[0]; color: Theme.muted; font.family: Theme.textFont; font.pixelSize: Theme.fs(14.5) }
                    Text { text: modelData[1]; color: Theme.fg; font.family: Theme.textFont; font.pixelSize: Theme.fs(14.5) }
                }
            }
        }
    }
    Toast { id: toast }

    AppearanceSheet {
        id: appearance
        z: 100
        onVisibleChanged: if (!visible && !palettes.visible && !languages.visible) root.backToPage()
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
