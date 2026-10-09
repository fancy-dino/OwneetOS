// SPDX-License-Identifier: GPL-3.0-or-later
// Settings (roadmap 3.9, approved demo design/demos/3.9-settings.html): the sections in a list on
// the left, the panel of the selected one on the right (it follows the selection). Right or A
// enters the panel, B or left at its edge goes back to the list; the right stick scrolls it.
// Built in parts: Sound, Appearance, Language, Storage and System first; Network, Controllers and
// Bluetooth, Display next.
import QtQuick 2.15
import "foundation"

NavArea {
    id: page
    property string section: "audio"
    readonly property var sections: ["audio", "appearance", "language", "storage", "system"]

    signal openPalettes()
    signal openLanguages()
    signal chooseOutput(var outputs, string current)
    signal chooseKeyboard(var layouts, string current)
    signal confirmPower(string action)
    signal openCredits()
    signal notYet()

    scrollTarget: panel

    readonly property var prompts: {
        const P = (b, l) => ({ buttons: [b], label: Tr.tr(l) });
        if (!current || current.section !== undefined)
            return [P("a", "prompt.open")];
        if (current.kind === "slider")
            return [P("lr", "prompt.adjust"), P("b", "prompt.back")];
        if (current.kind === "switch")
            return [P("a", "prompt.toggle"), P("b", "prompt.back")];
        return [P("a", "prompt.select"), P("b", "prompt.back")];
    }

    function navItem(name) {
        for (let i = 0; i < navColumn.children.length; i++)
            if (navColumn.children[i].section === name)
                return navColumn.children[i];
        return null;
    }
    function focusStart() { focusItem(navItem(section)); }
    onActiveFocusChanged: if (activeFocus && !current) focusStart()
    function inPanel(item) { return item && Nav.groupOf(page, item) === panelGroup; }

    onMoved: {
        if (item.section !== undefined) {
            if (section !== item.section) {
                section = item.section;
                panel.contentY = 0;
            }
        } else {
            ensureVisible(item);
        }
    }
    onAccepted: {
        if (item.section !== undefined) {           // A on a section: into its panel
            const first = Nav.navigables(panelColumn)[0];
            if (first) focusItem(first);
            return;
        }
        if (item.activated)
            item.activated();
    }
    onCancelled: if (inPanel(current)) focusItem(navItem(section))

    function ensureVisible(item) {
        const p = item.mapToItem(panelColumn, 0, 0);
        const pad = Theme.px(16);
        if (p.y - pad < panel.contentY)
            panel.contentY = Math.max(0, p.y - pad);
        else if (p.y + item.height + pad > panel.contentY + panel.height)
            panel.contentY = Math.min(panelColumn.height - panel.height, p.y + item.height + pad - panel.height);
    }

    // ---- owneetd state used by the panels
    property var audio: null
    property var keyboardLayouts: []
    property string keyboardLayout: ""
    property var volumes: []
    function refresh() {
        owneetd.get("/v1/audio", (status, data) => { if (status === 200) page.audio = data; });
        owneetd.get("/v1/input/layout", (status, data) => {
            if (status === 200) { page.keyboardLayouts = data.supported || []; page.keyboardLayout = data.layout || ""; }
        });
        volumes = storage.volumes();
    }
    onVisibleChanged: if (visible) refresh()
    Connections {
        target: owneetd
        function onEvent(type, data) {
            if (type === "audio.changed" && page.visible) owneetd.get("/v1/audio", (s, d) => { if (s === 200) page.audio = d; });
            else if (type === "input.layout_changed") page.keyboardLayout = data.layout;
        }
        function onConnectedChanged() { if (owneetd.connected) page.refresh(); }
    }
    readonly property var output: {
        if (!audio || !audio.outputs) return null;
        for (let i = 0; i < audio.outputs.length; i++)
            if (audio.outputs[i].id === audio.output) return audio.outputs[i];
        return null;
    }

    // ---- Sections
    NavGroup {
        id: navGroup
        anchors { left: parent.left; top: parent.top; bottom: parent.bottom }
        width: Theme.px(290)
        Column {
            id: navColumn
            width: parent.width
            spacing: Theme.px(6)
            Repeater {
                model: page.sections
                Rectangle {
                    readonly property string section: modelData
                    readonly property bool navigable: true
                    readonly property bool shown: page.section === modelData
                    width: navColumn.width
                    height: navText.implicitHeight + Theme.px(22)
                    radius: Theme.radiusM
                    color: shown ? Theme.surface : "transparent"
                    Icon {
                        id: navIcon
                        x: Theme.px(16)
                        anchors.verticalCenter: parent.verticalCenter
                        width: Theme.fs(22); height: width
                        name: ({ audio: "sound", appearance: "palette", language: "globe", storage: "disks", system: "gear" })[modelData]
                        color: parent.shown ? Theme.fg : Theme.muted
                    }
                    Text {
                        id: navText
                        anchors { left: navIcon.right; leftMargin: Theme.px(14); verticalCenter: parent.verticalCenter }
                        text: Tr.tr("settings." + modelData)
                        color: parent.shown ? Theme.fg : Theme.muted
                        font.family: Theme.textFont
                        font.weight: parent.shown ? Font.Medium : Font.Normal
                        font.pixelSize: Theme.fs(17)
                    }
                    FocusFrame {}
                }
            }
        }
    }

    // ---- The panel of the selected section
    NavGroup {
        id: panelGroup
        anchors { left: navGroup.right; leftMargin: Theme.px(28); right: parent.right; top: parent.top; bottom: parent.bottom }
        Flickable {
            id: panel
            // room for the focus ring and the lift of wide rows on both sides
            anchors { fill: parent; leftMargin: -Theme.px(22); rightMargin: -Theme.px(22) }
            clip: true
            interactive: false
            contentWidth: width
            contentHeight: panelColumn.height + Theme.px(20)
            Behavior on contentY { enabled: !Theme.reduceMotion; NumberAnimation { duration: Theme.motionMs; easing.type: Easing.OutCubic } }
            Column {
                id: panelColumn
                x: Theme.px(22); y: Theme.px(10)
                width: panel.width - Theme.px(44)
                spacing: Theme.px(12)

                Text {
                    text: Tr.tr("settings." + page.section)
                    color: Theme.fg
                    font.family: Theme.displayFont
                    font.weight: Font.Bold
                    font.pixelSize: Theme.fs(24)
                }

                // ---- Sound
                Column {
                    visible: page.section === "audio"
                    width: parent.width
                    spacing: Theme.px(12)
                    InfoRow {
                        visible: page.audio !== null && !page.audio.available
                        width: parent.width
                        text: Tr.tr("sound.unavailable")
                    }
                    SettingRow {
                        visible: page.output !== null
                        width: parent.width
                        kind: "picker"
                        text: Tr.tr("sound.output")
                        value: page.output ? page.output.name : ""
                        onActivated: page.chooseOutput(page.audio.outputs, page.audio.output)
                    }
                    SettingSlider {
                        visible: page.output !== null
                        width: parent.width
                        text: Tr.tr("sound.volume")
                        value: page.audio ? page.audio.volume : 0
                        onMoved: owneetd.put("/v1/audio/volume", { volume: value })
                    }
                    SettingRow {
                        visible: page.output !== null
                        width: parent.width
                        text: Tr.tr("sound.mute")
                        checked: page.audio ? page.audio.muted : false
                        onActivated: owneetd.put("/v1/audio/mute", { muted: !checked })
                    }
                    SettingRow {
                        width: parent.width
                        text: Tr.tr("sound.interface")
                        checked: Theme.uiSounds
                        onActivated: Theme.uiSounds = !Theme.uiSounds
                    }
                    SettingSlider {
                        visible: Theme.uiSounds
                        width: parent.width
                        text: Tr.tr("sound.interface.volume")
                        value: Theme.uiSoundsVolume
                        onMoved: Theme.uiSoundsVolume = value
                    }
                }

                // ---- Appearance
                Column {
                    visible: page.section === "appearance"
                    width: parent.width
                    spacing: Theme.px(12)
                    SettingRow {
                        width: parent.width
                        kind: "picker"
                        text: Tr.tr("appearance.palette")
                        value: Tr.tr("palette." + Theme.paletteKey)
                        swatches: { const c = Theme.palette(Theme.paletteKey).c; return [c[0], c[2], c[6], c[4]]; }
                        onActivated: page.openPalettes()
                    }
                    Label { text: Tr.tr("appearance.text_size") }
                    Row {
                        id: sizes
                        width: parent.width
                        spacing: Theme.px(6)
                        readonly property real cell: (width - spacing * 3) / 4
                        Repeater {
                            model: [[0.9, "S"], [1.0, "M"], [1.2, "L"], [1.4, "XL"]]
                            Choice {
                                width: sizes.cell
                                text: modelData[1]
                                chosen: Theme.textScale === modelData[0]
                                onActivated: Theme.textScale = modelData[0]
                            }
                        }
                    }
                    Label { text: Tr.tr("appearance.button_prompts") }
                    Row {
                        id: glyphSets
                        width: parent.width
                        spacing: Theme.px(6)
                        readonly property real cell: (width - spacing * 3) / 4
                        Repeater {
                            model: [["auto", Tr.tr("appearance.auto")], ["xbox", "Xbox"], ["ps", "PlayStation"], ["nintendo", "Nintendo"]]
                            Choice {
                                width: glyphSets.cell
                                text: modelData[1]
                                chosen: Theme.glyphSetting === modelData[0]
                                onActivated: Theme.glyphSetting = modelData[0]
                            }
                        }
                    }
                    SettingRow {
                        width: parent.width
                        text: Tr.tr("appearance.reduce_motion")
                        detail: Tr.tr("appearance.reduce_motion.detail")
                        checked: Theme.reduceMotion
                        onActivated: Theme.reduceMotion = !Theme.reduceMotion
                    }
                    SettingRow {
                        width: parent.width
                        text: Tr.tr("appearance.safe_area")
                        checked: Theme.showSafeArea
                        onActivated: Theme.showSafeArea = !Theme.showSafeArea
                    }
                }

                // ---- Language
                Column {
                    visible: page.section === "language"
                    width: parent.width
                    spacing: Theme.px(12)
                    SettingRow {
                        width: parent.width
                        kind: "picker"
                        text: Tr.tr("appearance.language")
                        value: Tr.languageName(Tr.language)
                        onActivated: page.openLanguages()
                    }
                    SettingRow {
                        visible: page.keyboardLayouts.length > 0
                        width: parent.width
                        kind: "picker"
                        text: Tr.tr("language.keyboard")
                        detail: Tr.tr("language.keyboard.detail")
                        value: Theme.keyboardSetting === "same"
                            ? Tr.tr("language.keyboard.same", { layout: Tr.tr("keyboard." + page.keyboardLayout) })
                            : Tr.tr("keyboard." + Theme.keyboardSetting)
                        onActivated: page.chooseKeyboard(page.keyboardLayouts, Theme.keyboardSetting)
                    }
                }

                // ---- Storage
                Column {
                    visible: page.section === "storage"
                    width: parent.width
                    spacing: Theme.px(12)
                    Repeater {
                        model: page.volumes
                        InfoRow {
                            width: parent.width
                            text: modelData.name || Tr.tr("disk.internal")
                            value: Tr.tr("disk.of", { free: GameInfo.size(modelData.free), total: GameInfo.size(modelData.total) })
                            detail: modelData.readOnly ? Tr.tr("disk.note.readonly") : ""
                            Rectangle {
                                width: parent.width; height: Theme.px(6); radius: height / 2
                                color: Theme.raised
                                Rectangle {
                                    width: parent.width * Math.min(1, 1 - modelData.free / modelData.total)
                                    height: parent.height; radius: height / 2
                                    color: Theme.accent
                                }
                            }
                        }
                    }
                }

                // ---- System
                Column {
                    visible: page.section === "system"
                    width: parent.width
                    spacing: Theme.px(12)
                    InfoRow {
                        width: parent.width
                        text: Tr.tr("system.version")
                        value: systemInfo.version || "—"
                    }
                    SettingRow {
                        width: parent.width
                        kind: "picker"
                        text: Tr.tr("system.updates")
                        detail: Tr.tr("system.updates.later")
                        onActivated: page.notYet()
                    }
                    Label { text: Tr.tr("system.power") }
                    Row {
                        id: power
                        width: parent.width
                        spacing: Theme.px(12)
                        readonly property real cell: (width - spacing * 2) / 3
                        Repeater {
                            model: ["suspend", "restart", "shutdown"]
                            Rectangle {
                                readonly property bool navigable: true
                                signal activated()
                                width: power.cell
                                height: powerText.implicitHeight + Theme.px(24)
                                radius: Theme.radiusS + Theme.px(2)
                                color: Theme.surface
                                border.color: Theme.line
                                border.width: Math.max(1, Theme.px(1))
                                onActivated: page.confirmPower(modelData)
                                Text {
                                    id: powerText
                                    anchors.centerIn: parent
                                    text: Tr.tr("system.power." + modelData)
                                    color: Theme.fg
                                    font.family: Theme.textFont
                                    font.pixelSize: Theme.fs(14.5)
                                }
                                FocusFrame {}
                            }
                        }
                    }
                    Label { text: Tr.tr("system.about") }
                    SettingRow {
                        width: parent.width
                        kind: "picker"
                        text: Tr.tr("system.credits")
                        detail: Tr.tr("system.credits.detail")
                        onActivated: page.openCredits()
                    }
                }
            }
        }
    }
}
