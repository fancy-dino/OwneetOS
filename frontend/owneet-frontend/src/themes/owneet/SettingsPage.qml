// SPDX-License-Identifier: GPL-3.0-or-later
// Settings (roadmap 3.9, approved demo design/demos/3.9-settings.html): the sections in a list on
// the left, the panel of the selected one on the right (it follows the selection). Right or A
// enters the panel, B or left at its edge goes back to the list; the right stick scrolls it.
// Built in parts: Sound, Appearance, Language, Storage and System first; Network, Controllers and
// Bluetooth second; Display next. Rows that come from owneetd (networks, controllers, devices)
// have a `key`, so the selection stays on the same one when the lists change.
import QtQuick 2.15
import "foundation"

NavArea {
    id: page
    property string section: "network"
    readonly property var sections: ["network", "controllers", "audio", "appearance", "language", "storage", "system"]
    property var network: null          // owneetd's network state (from the shell)
    property var controllers: []        // connected controllers (from the shell)

    signal openPalettes()
    signal openLanguages()
    signal chooseOutput(var outputs, string current)
    signal chooseKeyboard(var layouts, string current)
    signal confirmPower(string action)
    signal openCredits()
    signal notYet()
    signal connectNetwork(var net)
    signal networkOptions(var net)
    signal controllerOptions(var pad)
    signal deviceOptions(var device)
    signal pairController()
    signal failed(string message)       // a message key, shown as a toast

    scrollTarget: panel

    readonly property var prompts: {
        const P = (b, l) => ({ buttons: [b], label: Tr.tr(l) });
        if (!current || current.section !== undefined)
            return [P("a", "prompt.open")];
        if (current.kind === "slider")
            return [P("lr", "prompt.adjust"), P("b", "prompt.back")];
        if (current.kind === "switch")
            return [P("a", "prompt.toggle"), P("b", "prompt.back")];
        if (current.kind === "network") {
            const n = current.net;
            return [P("a", n.connected ? "prompt.options" : "prompt.connect")]
                .concat(n.known && !n.connected ? [P("menu", "prompt.options")] : [], [P("b", "prompt.back")]);
        }
        if (current.kind === "pad" || current.device === true)
            return [P("a", "prompt.options"), P("b", "prompt.back")];
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

    // The selected row's key; when its list changes and the row is made again, the selection goes
    // to the new one (or to the first row of the panel when it is gone)
    property string currentKey: ""
    function restoreFocus() {
        const items = Nav.navigables(panelColumn);
        if (currentKey === "" || (current && (items.indexOf(current) >= 0 || current.section !== undefined)))
            return;
        const item = items.find(i => i.key === currentKey) || items[0] || navItem(section);
        if (activeFocus) {
            focusItem(item);
        } else {                        // a window is open, or another section: no focus taken
            current = item;
            const group = Nav.groupOf(page, item);
            if (group) group.remembered = item;
            item.focus = true;
        }
    }
    onCurrentChanged: if (!current) Qt.callLater(restoreFocus)
    onWifiNetworksChanged: Qt.callLater(restoreFocus)
    onKnownDevicesChanged: Qt.callLater(restoreFocus)
    onControllersChanged: Qt.callLater(restoreFocus)

    onMoved: {
        currentKey = item.key || "";
        if (item.section !== undefined) {
            if (section !== item.section) {
                section = item.section;
                panel.contentY = 0;
                if (section === "network") refreshNetworks(true);
                else if (section === "controllers") refreshBluetooth();
            }
        } else {
            ensureVisible(item);
        }
    }
    // Menu on a saved network, a controller or a device: its options
    onOtherAction: {
        if (action !== "options" || !current || !inPanel(current))
            return;
        if (current.kind === "network" && current.net.known) {
            Nav.feedback("confirm");
            networkOptions(current.net);
        } else if (current.kind === "pad" || current.device === true) {
            Nav.feedback("confirm");
            current.activated();
        } else {
            Nav.feedback("edge");
        }
        event.accepted = true;
    }
    onAccepted: {
        if (item.section !== undefined) {           // A on a section: into its panel
            const first = Nav.navigables(panelColumn)[0];
            if (first) focusItem(first);
        }
        // other items: NavArea has already called their activated()
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
    property var wifiNetworks: []
    property bool scanning: false
    property var bluetooth: null
    function refresh() {
        if (section === "network") refreshNetworks(true);
        refreshBluetooth();
        owneetd.get("/v1/audio", (status, data) => { if (status === 200) page.audio = data; });
        owneetd.get("/v1/input/layout", (status, data) => {
            if (status === 200) { page.keyboardLayouts = data.supported || []; page.keyboardLayout = data.layout || ""; }
        });
        volumes = storage.volumes();
    }
    // Networks nearby: the connected one first, then by signal; `scan` also asks for a new search
    function refreshNetworks(scan) {
        owneetd.get("/v1/network/wifi", (status, data) => {
            if (status !== 200) { page.wifiNetworks = []; return; }
            const list = (data.networks || []).filter(n => n.ssid !== "");
            list.sort((a, b) => (b.connected - a.connected) || (b.strength - a.strength));
            page.wifiNetworks = list;
        });
        if (scan && wifiOn) {
            owneetd.post("/v1/network/wifi/scan", {}, status => {
                page.scanning = status === 202;
                if (page.scanning) scanTimer.restart();
            });
        }
    }
    Timer { id: scanTimer; interval: 20000; onTriggered: page.scanning = false }
    function refreshBluetooth() {
        owneetd.get("/v1/bluetooth", (status, data) => { page.bluetooth = status === 200 ? data : null; });
    }
    onVisibleChanged: if (visible) refresh()
    Connections {
        target: owneetd
        function onEvent(type, data) {
            if (type === "audio.changed" && page.visible) owneetd.get("/v1/audio", (s, d) => { if (s === 200) page.audio = d; });
            else if (type === "input.layout_changed") page.keyboardLayout = data.layout;
            else if (type === "wifi.scan_done") { page.scanning = false; if (page.visible) page.refreshNetworks(false); }
            else if ((type === "network.changed" || type === "network.forgotten") && page.visible) page.refreshNetworks(false);
            else if ((type.startsWith("bluetooth.") || type.startsWith("controller.")) && page.visible) page.refreshBluetooth();
        }
        function onConnectedChanged() { if (owneetd.connected) page.refresh(); }
    }
    // ---- Network and controllers, as the panels show them
    readonly property bool netAvailable: network !== null && network.available !== false
    readonly property bool wifiPresent: netAvailable && network.wifi && network.wifi.present === true
    readonly property bool wifiOn: wifiPresent && network.wifi.enabled && network.wifi.hardware_enabled !== false
    readonly property bool wired: netAvailable && network.ethernet && network.ethernet.connected === true
    readonly property bool offline: netAvailable && !wired && !(network.wifi && network.wifi.state === "connected")
    // connected to a network that does not reach the internet (or asks for a sign-in page)
    readonly property bool limited: netAvailable && !offline
                                    && ["none", "limited", "portal"].indexOf(network.connectivity) >= 0
    function isController(address) {
        const a = address.toLowerCase();
        return controllers.some(c => c.connection === "bluetooth" && c.id.toLowerCase() === a);
    }
    // Paired Bluetooth devices that are not a connected controller: controllers turned off,
    // headphones…
    readonly property var knownDevices: {
        if (!bluetooth || !bluetooth.devices) return [];
        return bluetooth.devices.filter(d => d.paired && !(d.connected && isController(d.address)))
                                .sort((a, b) => a.name.localeCompare(b.name));
    }
    function bluetoothDevice(pad) {
        if (!bluetooth || !bluetooth.devices || pad.connection !== "bluetooth") return null;
        return bluetooth.devices.find(d => d.address.toLowerCase() === pad.id.toLowerCase()) || null;
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
                        name: ({ network: "wifi", controllers: "controller", audio: "sound", appearance: "palette",
                                 language: "globe", storage: "disks", system: "gear" })[modelData]
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

                // ---- Network
                Column {
                    visible: page.section === "network"
                    width: parent.width
                    spacing: Theme.px(12)
                    InfoRow {
                        visible: !page.netAvailable
                        width: parent.width
                        text: Tr.tr("network.unavailable")
                    }
                    InfoRow {
                        visible: page.offline
                        width: parent.width
                        icon: page.wifiPresent ? "wifi" : "ethernet"
                        crossed: true
                        text: Tr.tr("network.offline")
                        detail: Tr.tr(!page.wifiPresent ? "network.offline.cable" : page.wifiOn ? "network.offline.detail" : "network.offline.wifi_off")
                    }
                    InfoRow {
                        visible: page.limited
                        width: parent.width
                        icon: page.wired ? "ethernet" : "wifi"
                        text: Tr.tr("network.limited")
                        detail: Tr.tr("network.limited.detail")
                    }
                    SettingRow {
                        visible: page.wifiPresent
                        width: parent.width
                        text: Tr.tr("network.wifi")
                        detail: page.wifiPresent && page.network.wifi.hardware_enabled === false ? Tr.tr("network.wifi.hardware") : ""
                        checked: page.wifiOn
                        onActivated: owneetd.put("/v1/network/wifi/enabled", { enabled: !checked }, status => {
                            if (status >= 300 || status === 0) page.failed("error.network");
                        })
                    }
                    InfoRow {
                        visible: page.netAvailable && page.network.ethernet && page.network.ethernet.present === true
                        width: parent.width
                        text: Tr.tr("network.cable")
                        value: Tr.tr(page.wired ? "network.cable.on" : "network.cable.off")
                    }
                    Label {
                        visible: page.wifiOn
                        topPadding: Theme.px(8)
                        text: Tr.tr("network.nearby")
                    }
                    Repeater {
                        model: page.wifiOn ? page.wifiNetworks : []
                        Rectangle {
                            id: netRow
                            readonly property bool navigable: true
                            readonly property string kind: "network"
                            readonly property string key: "net:" + modelData.ssid
                            readonly property var net: modelData
                            signal activated()
                            onActivated: modelData.connected ? page.networkOptions(modelData) : page.connectNetwork(modelData)
                            width: parent.width
                            height: Math.max(ssid.implicitHeight, Theme.fs(24)) + Theme.px(22)
                            radius: Theme.radiusS + Theme.px(2)
                            color: Theme.surface
                            border.color: Theme.line
                            border.width: Math.max(1, Theme.px(1))
                            Row {
                                anchors { left: parent.left; leftMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                                spacing: Theme.px(10)
                                Icon {
                                    anchors.verticalCenter: parent.verticalCenter
                                    width: Theme.fs(20); height: width
                                    name: "wifi"
                                    level: modelData.strength >= 67 ? 3 : modelData.strength >= 34 ? 2 : 1
                                    color: Theme.fg
                                }
                                Text {
                                    id: ssid
                                    anchors.verticalCenter: parent.verticalCenter
                                    width: Math.min(implicitWidth, netRow.width * 0.55)
                                    text: modelData.ssid
                                    color: Theme.fg
                                    font.family: Theme.textFont
                                    font.pixelSize: Theme.fs(14.5)
                                    elide: Text.ElideRight
                                }
                            }
                            Row {
                                anchors { right: parent.right; rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                                spacing: Theme.px(12)
                                Rectangle {          // Connected / Saved
                                    visible: modelData.connected || modelData.known
                                    anchors.verticalCenter: parent.verticalCenter
                                    width: badge.implicitWidth + Theme.px(20)
                                    height: badge.implicitHeight + Theme.px(4)
                                    radius: height / 2
                                    color: modelData.connected ? Theme.accent : Theme.raised
                                    Text {
                                        id: badge
                                        anchors.centerIn: parent
                                        text: Tr.tr(modelData.connected ? "network.connected" : "network.saved")
                                        color: modelData.connected ? Theme.onAccent : Theme.fg
                                        font.family: Theme.textFont
                                        font.pixelSize: Theme.fs(12.5)
                                    }
                                }
                                Text {
                                    visible: modelData.security === "open"
                                    anchors.verticalCenter: parent.verticalCenter
                                    text: Tr.tr("network.open")
                                    color: Theme.muted
                                    font.family: Theme.textFont
                                    font.pixelSize: Theme.fs(14.5)
                                }
                                Icon {
                                    visible: modelData.security !== "open"
                                    anchors.verticalCenter: parent.verticalCenter
                                    width: Theme.fs(20); height: width
                                    name: "lock"
                                    color: Theme.muted
                                }
                            }
                            FocusFrame {}
                        }
                    }
                    InfoRow {
                        visible: page.wifiOn && page.wifiNetworks.length === 0
                        width: parent.width
                        text: Tr.tr(page.scanning ? "network.searching" : "network.none")
                    }
                    SettingRow {
                        visible: page.wifiOn
                        width: parent.width
                        kind: "action"
                        text: Tr.tr("network.scan")
                        value: page.scanning ? Tr.tr("network.searching") : ""
                        onActivated: page.refreshNetworks(true)
                    }
                }

                // ---- Controllers and Bluetooth
                Column {
                    visible: page.section === "controllers"
                    width: parent.width
                    spacing: Theme.px(12)
                    Label { text: Tr.tr("pad.connected") }
                    Repeater {
                        model: page.controllers
                        Rectangle {
                            id: padRow
                            readonly property bool navigable: true
                            readonly property string kind: "pad"
                            readonly property string key: "pad:" + modelData.id
                            // A percentage when the driver gives one, otherwise a level
                            readonly property var battery: {
                                const levels = { full: 1, high: 0.75, normal: 0.5, low: 0.2, critical: 0.08 };
                                if (modelData.battery !== undefined)
                                    return { fill: modelData.battery / 100, text: modelData.battery + "%" };
                                if (levels[modelData.battery_level] !== undefined)
                                    return { fill: levels[modelData.battery_level], text: "" };
                                return null;
                            }
                            signal activated()
                            onActivated: page.controllerOptions(Object.assign({ device: page.bluetoothDevice(modelData) }, modelData))
                            width: parent.width
                            height: padLabels.implicitHeight + Theme.px(22)
                            radius: Theme.radiusS + Theme.px(2)
                            color: Theme.surface
                            border.color: Theme.line
                            border.width: Math.max(1, Theme.px(1))
                            Icon {
                                id: padIcon
                                anchors { left: parent.left; leftMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                                width: Theme.fs(20); height: width
                                name: "controller"
                                color: Theme.fg
                            }
                            Column {
                                id: padLabels
                                anchors { left: padIcon.right; leftMargin: Theme.px(10); verticalCenter: parent.verticalCenter }
                                width: parent.width * 0.6
                                spacing: Theme.px(2)
                                Text {
                                    width: parent.width
                                    text: modelData.name
                                    color: Theme.fg
                                    font.family: Theme.textFont
                                    font.pixelSize: Theme.fs(14.5)
                                    elide: Text.ElideRight
                                }
                                Text {
                                    text: Tr.tr("pad.connection." + (["usb", "bluetooth", "dongle"].indexOf(modelData.connection) >= 0 ? modelData.connection : "other"))
                                    color: Theme.muted
                                    font.family: Theme.textFont
                                    font.pixelSize: Theme.fs(12.5)
                                }
                            }
                            Row {
                                visible: padRow.battery !== null
                                anchors { right: parent.right; rightMargin: Theme.px(16); verticalCenter: parent.verticalCenter }
                                spacing: Theme.px(6)
                                Icon {
                                    anchors.verticalCenter: parent.verticalCenter
                                    width: Theme.fs(20); height: width
                                    name: "battery"
                                    fill: padRow.battery ? padRow.battery.fill : 1
                                    color: Theme.muted
                                }
                                Text {
                                    anchors.verticalCenter: parent.verticalCenter
                                    text: padRow.battery ? padRow.battery.text : ""
                                    color: Theme.muted
                                    font.family: Theme.textFont
                                    font.pixelSize: Theme.fs(14.5)
                                }
                            }
                            FocusFrame {}
                        }
                    }
                    InfoRow {
                        visible: page.controllers.length === 0
                        width: parent.width
                        text: Tr.tr("pad.none")
                    }
                    SettingRow {
                        visible: page.bluetooth !== null && page.bluetooth.adapter
                        width: parent.width
                        kind: "picker"
                        text: Tr.tr("pad.pair")
                        detail: Tr.tr("pad.pair.detail")
                        onActivated: page.pairController()
                    }
                    InfoRow {
                        visible: page.bluetooth === null || !page.bluetooth.adapter
                        width: parent.width
                        text: Tr.tr("pad.no_bluetooth")
                        detail: Tr.tr("pad.no_bluetooth.detail")
                    }
                    Label {
                        visible: page.knownDevices.length > 0
                        topPadding: Theme.px(8)
                        text: Tr.tr("pad.known")
                    }
                    Repeater {
                        model: page.knownDevices
                        SettingRow {
                            readonly property string key: "dev:" + modelData.address
                            width: parent.width
                            kind: "action"
                            text: modelData.name
                            value: modelData.connected ? Tr.tr("pad.device.connected") : ""
                            readonly property bool device: true
                            onActivated: page.deviceOptions(modelData)
                        }
                    }
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
