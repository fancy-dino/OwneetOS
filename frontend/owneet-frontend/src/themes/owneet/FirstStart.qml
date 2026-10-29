// SPDX-License-Identifier: GPL-3.0-or-later
// The first start ([[step-6.1-first-boot-flow]], approved demo design/demos/6.1-first-start.html,
// [[first-boot-setup]]): the controller (paired by itself, as in the mockup: nothing to press),
// the language (the keyboard follows it), the network (a cable, Wi-Fi with the console's keyboard,
// or later), the time zone (suggested from the language, or a city from a list; never guessed
// from the place), the Xbox wireless adapter's firmware (only with an adapter and the internet,
// after the player's consent), ready. Shown once on an installed console, until "Ready" (turned
// off half way it starts again, the choices kept), and at every start from the USB stick, where
// nothing is kept: the theme decides (theme.qml, "firstStartDone").
import QtQuick 2.15
import "foundation"

Rectangle {
    id: root
    anchors.fill: parent
    z: 90
    visible: false
    color: Theme.bg

    // ---- From the theme
    property var pads: null              // the connected controllers
    property var network: null           // owneetd's network status
    property bool online: false
    property Item theme: null            // its Wi-Fi window: connectNetwork(net)
    signal finished()

    // ---- Where the player is
    property string screen: "pair"
    readonly property var order: ["pair", "language", "network", "time", "dongle", "ready"]
    readonly property var stepOf: ({ pair: "controller", language: "language", network: "network",
                                     time: "time", dongle: "network", ready: "ready" })
    readonly property var steps: ["controller", "language", "network", "time", "ready"]

    function start() {
        screen = pads && pads.count > 0 ? "language" : "pair";
        choosing = false;
        visible = true;
        refreshNetworks(true);
        refreshZones();
        owneetd.get("/v1/xone", (status, data) => { if (status === 200) root.xone = data; });
        Qt.callLater(focusScreen);
    }
    // back from the Wi-Fi window or the keyboard
    function refocus() { Qt.callLater(focusScreen); }
    // The focus belongs here while it is shown, on a selected item: at the start (the screen's
    // items may not be shown yet), or when the pages under it take it (the home screen when the
    // games arrive), it comes back, unless a window (the Wi-Fi password, the keyboard) has it
    Timer {
        interval: 250
        repeat: true
        running: root.visible && root.theme !== null && !root.theme.windowOpen
                 && !(nav.current && nav.current.visible && nav.current.activeFocus)
                 && root.screen !== "pair" && root.dongleState !== "installing"
        onTriggered: root.focusScreen()
    }

    function go(name) {
        screen = name;
        choosing = false;
        netList.contentY = 0;
        zoneList.contentY = 0;
        Qt.callLater(focusScreen);
    }
    function dongleNeeded() { return xone !== null && xone.needed === true && online; }
    function next() {
        let i = order.indexOf(screen) + 1;
        if (order[i] === "dongle" && !dongleNeeded())
            i++;                    // only with an adapter that needs its firmware, and the internet
        go(order[i]);
    }
    function back() {
        if (screen === "time" && choosing) {
            choosing = false;
            Qt.callLater(() => focusScreen("choose"));
            return;
        }
        let i = order.indexOf(screen);
        if (i <= 1 || screen === "ready" || dongleState === "installing") {
            Nav.feedback("edge");
            return;
        }
        let p = i - 1;
        if (order[p] === "dongle" && !dongleNeeded())
            p--;
        go(order[p]);
    }
    function focusScreen(prefer) {
        if (!visible)
            return;
        nav.forceActiveFocus();
        const wanted = prefer ? itemNamed(prefer) : defaultItem();
        if (wanted)
            nav.focusItem(wanted);
        else
            nav.focusFirst();
    }
    function itemNamed(name) {
        const all = Nav.navigables(nav);
        for (let i = 0; i < all.length; i++)
            if (all[i].key === name)
                return all[i];
        return null;
    }
    function defaultItem() {
        if (screen === "language")
            return itemNamed("lang:" + Tr.language);
        if (screen === "network" && (wired || wifiConnected))
            return itemNamed("continue");
        return null;
    }

    // ---- Controller: owneetd pairs any controller in pairing mode while none is connected
    Connections {
        target: root.pads
        function onCountChanged() {
            if (!root.visible || root.screen !== "pair" || root.pads.count === 0)
                return;
            Nav.feedback("notify");
            const name = root.pads.get(0).name || "";
            if (name !== "")
                toast.show(Tr.tr("first.pair.found", { name: name }));
            root.go("language");
        }
    }
    property int helloIndex: 0
    Timer {                               // the title in every language, the brands lit in turn
        interval: 2200
        repeat: true
        running: root.visible && root.screen === "pair"
        onTriggered: root.helloIndex++
    }
    readonly property string hello: {
        const langs = Tr.languages;
        if (langs.length === 0)
            return Tr.tr("first.hello");
        return Tr.trIn(langs[helloIndex % langs.length].tag, "first.hello");
    }

    // ---- Network: a cable, or Wi-Fi with the console's keyboard (the theme's window)
    readonly property bool netAvailable: network !== null && network.available !== false
    readonly property bool wired: netAvailable && network.ethernet && network.ethernet.connected === true
    readonly property bool wifiPresent: netAvailable && network.wifi && network.wifi.present === true
    readonly property bool wifiConnected: netAvailable && network.wifi && network.wifi.state === "connected"
    property var wifiNetworks: []
    function refreshNetworks(scan) {
        owneetd.get("/v1/network/wifi", (status, data) => {
            if (status !== 200) { root.wifiNetworks = []; return; }
            const list = (data.networks || []).filter(n => n.ssid !== "");
            list.sort((a, b) => (b.connected - a.connected) || (b.strength - a.strength));
            const joined = list.some(n => n.connected) !== root.wifiNetworks.some(n => n.connected);
            root.wifiNetworks = list;
            if (joined) netList.contentY = 0;     // the network just joined is at the top
        });
        if (scan && wifiPresent)
            owneetd.post("/v1/network/wifi/scan", {}, () => {});
    }
    readonly property string networkName: wired ? Tr.tr("first.net.cable")
        : wifiConnected ? (network.wifi.ssid || connectedSsid()) : Tr.tr("first.ready.offline")
    function connectedSsid() {
        for (let i = 0; i < wifiNetworks.length; i++)
            if (wifiNetworks[i].connected)
                return wifiNetworks[i].ssid;
        return "";
    }
    onWifiConnectedChanged: if (visible && screen === "network") { refreshNetworks(false); Qt.callLater(focusScreen); }
    onWiredChanged: if (visible && screen === "network") Qt.callLater(focusScreen)

    // ---- Time zone: suggested from the language, or a city of the language's zones; "Choose
    // another city" lists them all. The cities' names are messages (city.<id>).
    readonly property var cityZones: ({
        rome: "Europe/Rome", berlin: "Europe/Berlin", paris: "Europe/Paris", brussels: "Europe/Brussels",
        geneva: "Europe/Zurich", montreal: "America/Toronto", madrid: "Europe/Madrid",
        mexico_city: "America/Mexico_City", bogota: "America/Bogota", lima: "America/Lima",
        buenos_aires: "America/Argentina/Buenos_Aires", santiago: "America/Santiago",
        london: "Europe/London", dublin: "Europe/Dublin", new_york: "America/New_York",
        toronto: "America/Toronto", chicago: "America/Chicago", denver: "America/Denver",
        los_angeles: "America/Los_Angeles", vancouver: "America/Vancouver", sydney: "Australia/Sydney",
        auckland: "Pacific/Auckland", johannesburg: "Africa/Johannesburg", singapore: "Asia/Singapore"
    })
    readonly property var languageCities: ({
        it: ["rome"], de: ["berlin"], fr: ["paris", "brussels", "geneva", "montreal"],
        es: ["madrid", "mexico_city", "bogota", "lima", "buenos_aires", "santiago"],
        en: ["london", "dublin", "new_york", "toronto", "chicago", "denver", "los_angeles", "vancouver",
             "sydney", "auckland", "johannesburg", "singapore"]
    })
    property bool choosing: false        // "Choose another city": every city
    property string city: ""             // the chosen one ("": the language's suggestion)
    property var offsets: ({})           // zone -> seconds from UTC now (owneetd)
    property real now: Date.now()
    Timer { interval: 20000; repeat: true; running: root.visible; onTriggered: root.now = Date.now() }
    readonly property var ownCities: languageCities[Tr.language] || languageCities.en
    readonly property var allCities: {
        const seen = {};
        const out = [];
        for (const tag in languageCities)
            languageCities[tag].forEach(c => { if (!seen[c]) { seen[c] = true; out.push(c); } });
        const off = offsets;
        out.sort((a, b) => ((off[cityZones[a]] || 0) - (off[cityZones[b]] || 0))
                           || Tr.tr("city." + a).localeCompare(Tr.tr("city." + b)));
        return out;
    }
    readonly property string shownCity: city !== "" ? city : ownCities[0]
    function refreshZones() {
        const zones = Object.keys(cityZones).map(c => "zone=" + encodeURIComponent(cityZones[c]));
        owneetd.get("/v1/time/zones?" + zones.join("&"), (status, data) => {
            if (status !== 200)
                return;
            const map = {};
            (data.zones || []).forEach(z => { map[z.zone] = z.offset; });
            root.offsets = map;
        });
    }
    function clockAt(cityId) {
        const offset = offsets[cityZones[cityId]] || 0;
        const d = new Date(now + offset * 1000);
        const two = n => (n < 10 ? "0" : "") + n;
        return two(d.getUTCHours()) + ":" + two(d.getUTCMinutes());
    }
    function utcLabel(cityId) {
        const offset = offsets[cityZones[cityId]] || 0;
        const h = Math.floor(Math.abs(offset) / 3600), m = Math.floor(Math.abs(offset) % 3600 / 60);
        return "UTC" + (offset < 0 ? "−" : "+") + h + (m > 0 ? ":" + (m < 10 ? "0" : "") + m : "");
    }
    function chooseCity(cityId) {
        city = cityId;
        owneetd.put("/v1/time/zone", { zone: cityZones[cityId] }, status => {
            if (status !== 200) toast.show(Tr.tr("first.time.failed"));
        });
        next();
    }

    // ---- The Xbox wireless adapter's firmware (owneetd: GET /v1/xone, "xone.changed")
    property var xone: null
    property string dongleState: ""      // "", installing, failed
    function downloadFirmware() {
        dongleState = "installing";
        owneetd.post("/v1/xone/install", {}, status => {
            if (status !== 202 && status !== 409) {
                Nav.feedback("error");
                root.dongleState = "failed";
                Qt.callLater(root.focusScreen);
            }
        });
    }
    Connections {
        target: owneetd
        function onEvent(type, data) {
            if (type === "xone.changed") {
                root.xone = data;
                if (root.dongleState !== "installing" || data.state === "installing")
                    return;
                if (data.state === "installed") {
                    root.dongleState = "";
                    Nav.feedback("notify");
                    toast.show(Tr.tr("first.dongle.done"));
                    if (root.screen === "dongle") root.next();
                } else {
                    Nav.feedback("error");
                    root.dongleState = "failed";
                    Qt.callLater(root.focusScreen);
                }
            } else if (type === "wifi.scan_done" || type === "network.changed") {
                if (root.visible) root.refreshNetworks(false);
            }
        }
    }

    // ---- Ready: the home screen, and the first start is done
    function finish() {
        api.memory.set("firstStartDone", true);
        visible = false;
        finished();
    }

    // ================= Layout =================
    Item {                                // the top: the wordmark (where the start-up animation lands) and the steps
        id: top
        anchors {
            left: parent.left; right: parent.right; top: parent.top
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; topMargin: Theme.safeTop
        }
        height: Math.max(mark.height, stepRow.height)
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
            id: stepRow
            anchors { right: parent.right; verticalCenter: parent.verticalCenter }
            spacing: Theme.px(9)
            Repeater {
                model: root.steps
                Row {
                    readonly property int current: root.steps.indexOf(root.stepOf[root.screen])
                    spacing: Theme.px(9)
                    Rectangle {
                        visible: index > 0
                        anchors.verticalCenter: parent.verticalCenter
                        width: Theme.fs(20); height: Math.max(1, Theme.px(2))
                        color: Theme.line
                    }
                    Row {
                        spacing: Theme.px(7)
                        anchors.verticalCenter: parent.verticalCenter
                        Rectangle {
                            width: Theme.fs(23); height: width; radius: width / 2
                            anchors.verticalCenter: parent.verticalCenter
                            color: index === current ? Theme.accent : "transparent"
                            border.color: index <= current ? Theme.accent : Theme.line
                            border.width: Math.max(1, Theme.px(2))
                            Text {
                                anchors.centerIn: parent
                                text: index + 1
                                color: index === current ? Theme.onAccent : index < current ? Theme.accent : Theme.muted
                                font.family: Theme.textFont
                                font.weight: index === current ? Font.DemiBold : Font.Normal
                                font.pixelSize: Theme.fs(13)
                            }
                        }
                        Text {
                            anchors.verticalCenter: parent.verticalCenter
                            text: Tr.tr("first.step." + modelData)
                            color: index === current ? Theme.fg : Theme.muted
                            font.family: Theme.textFont
                            font.pixelSize: Theme.fs(14.5)
                        }
                    }
                }
            }
        }
    }

    NavArea {
        id: nav
        anchors {
            left: parent.left; right: parent.right; top: top.bottom; bottom: prompts.top
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; topMargin: Theme.px(20); bottomMargin: Theme.px(20)
        }
        focus: root.visible
        scrollTarget: root.screen === "network" ? netList : root.screen === "time" ? zoneList : null
        onCancelled: root.back()
        onMoved: root.reveal(item)
        // the focus came back (from a window, or once the window is active): to the selected item
        onActiveFocusChanged: {
            if (!activeFocus)
                return;
            if (current && current.visible)
                current.forceActiveFocus();
            else
                Qt.callLater(root.focusScreen);
        }
        // the pairing screen has nothing to press; a keyboard (with no controller at all) goes on
        onOtherAction: {
            if (root.screen === "pair" && action === "accept") {
                Nav.feedback("confirm");
                root.go("language");
                event.accepted = true;
            }
        }

        // ---- Controller (the approved mockup)
        Row {
            visible: root.screen === "pair"
            anchors.centerIn: parent
            spacing: Theme.px(48)
            Item {                        // the searching radar
                id: radar
                width: Theme.px(330); height: width
                anchors.verticalCenter: parent.verticalCenter
                // three rings a third of a turn apart, growing and fading (3 s each)
                property real phase: 0
                NumberAnimation on phase {
                    from: 0; to: 1; duration: 3000
                    loops: Animation.Infinite
                    running: root.visible && root.screen === "pair" && !Theme.reduceMotion
                }
                Repeater {
                    model: 3
                    Rectangle {
                        readonly property real t: (radar.phase + index / 3) % 1
                        anchors.centerIn: parent
                        width: parent.width; height: width; radius: width / 2
                        color: "transparent"
                        border.color: Theme.accent
                        border.width: Math.max(1, Theme.px(2))
                        scale: 0.45 + 0.55 * t
                        opacity: Theme.reduceMotion ? (index === 0 ? 0.5 : 0) : 0.9 * (1 - t)
                    }
                }
                Rectangle {
                    anchors.centerIn: parent
                    width: parent.width * 0.52; height: width; radius: width / 2
                    color: Theme.surface
                    border.color: Theme.line
                    border.width: Math.max(1, Theme.px(1))
                    Icon {
                        anchors.centerIn: parent
                        width: parent.width * 0.56; height: width
                        name: "controller"
                        color: Theme.fg
                    }
                }
            }
            Column {
                width: Theme.px(600)
                anchors.verticalCenter: parent.verticalCenter
                spacing: Theme.px(18)
                Text {
                    id: helloText
                    width: parent.width
                    text: root.hello
                    color: Theme.fg
                    font.family: Theme.displayFont
                    font.weight: Font.Bold
                    font.pixelSize: Theme.fs(40)
                    wrapMode: Text.Wrap
                    Behavior on text {
                        SequentialAnimation {
                            NumberAnimation { target: helloText; property: "opacity"; to: 0; duration: Theme.reduceMotion ? 0 : 300 }
                            PropertyAction {}
                            NumberAnimation { target: helloText; property: "opacity"; to: 1; duration: Theme.reduceMotion ? 0 : 300 }
                        }
                    }
                }
                Row {
                    spacing: Theme.px(10)
                    Rectangle {
                        anchors.verticalCenter: parent.verticalCenter
                        width: Theme.fs(10); height: width; radius: width / 2
                        color: Theme.accent
                        SequentialAnimation on opacity {
                            running: root.visible && root.screen === "pair" && !Theme.reduceMotion
                            loops: Animation.Infinite
                            NumberAnimation { to: 0.25; duration: 600 }
                            NumberAnimation { to: 1; duration: 600 }
                        }
                    }
                    Text {
                        width: Theme.px(560)
                        text: Tr.tr("first.pair.searching")
                        color: Theme.accent
                        font.family: Theme.textFont
                        font.weight: Font.Medium
                        font.pixelSize: Theme.fs(17)
                        wrapMode: Text.Wrap
                    }
                }
                Grid {
                    columns: 2
                    spacing: Theme.px(11)
                    Repeater {
                        // brand names only say what works ([[legal-compliance]]); no logos
                        model: [["PlayStation", "first.pair.ps"], ["Xbox", "first.pair.xbox"],
                                ["Nintendo Switch Pro", "first.pair.switch"], ["", "first.pair.cable"]]
                        Rectangle {
                            readonly property bool lit: root.helloIndex % 4 === index
                            width: Theme.px(294)
                            height: brand.implicitHeight + Theme.px(26)
                            radius: Theme.radiusM
                            color: lit ? Theme.raised : Theme.surface
                            border.color: lit ? Theme.accent : Theme.line
                            border.width: Math.max(1, Theme.px(1))
                            Behavior on color { ColorAnimation { duration: Theme.reduceMotion ? 0 : 300 } }
                            Column {
                                id: brand
                                x: Theme.px(16); y: Theme.px(13)
                                width: parent.width - Theme.px(32)
                                spacing: Theme.px(4)
                                Text {
                                    text: modelData[0] !== "" ? modelData[0] : Tr.tr("first.pair.cable.title")
                                    color: Theme.fg
                                    font.family: Theme.textFont
                                    font.weight: Font.DemiBold
                                    font.pixelSize: Theme.fs(14.5)
                                }
                                Text {
                                    width: parent.width
                                    text: Tr.tr(modelData[1])
                                    color: Theme.muted
                                    font.family: Theme.textFont
                                    font.pixelSize: Theme.fs(14.5)
                                    wrapMode: Text.Wrap
                                }
                            }
                        }
                    }
                }
                Text {
                    width: parent.width
                    text: Tr.tr("first.pair.note")
                    color: Theme.muted
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                    wrapMode: Text.Wrap
                }
            }
        }

        // ---- Language
        Column {
            visible: root.screen === "language"
            anchors.centerIn: parent
            width: Theme.px(860)
            spacing: Theme.px(16)
            Heading { text: Tr.tr("first.lang.title") }
            Para { text: Tr.tr("first.lang.text") }
            Column {
                width: parent.width
                spacing: Theme.px(10)
                Repeater {
                    model: Tr.languages
                    ChoiceRow {
                        key: "lang:" + modelData.tag
                        text: modelData.name
                        ticked: modelData.tag === Tr.language
                        onActivated: { Tr.setLanguage(modelData.tag); root.next(); }
                    }
                }
            }
        }

        // ---- Network
        Column {
            id: netScreen
            visible: root.screen === "network"
            anchors.centerIn: parent
            width: Theme.px(860)
            spacing: Theme.px(16)
            Heading { text: Tr.tr("first.net.title") }
            Para { text: Tr.tr("first.net.text") }
            ChoiceRow {                   // a cable is all it takes
                visible: root.wired
                navigable: false
                text: Tr.tr("first.net.cable")
                tag: Tr.tr("first.net.connected")
            }
            Para {
                visible: !root.wired && !root.wifiPresent
                text: Tr.tr("first.net.no_wifi")
            }
            Para {
                visible: !root.wired && root.wifiPresent && root.wifiNetworks.length === 0
                text: Tr.tr("first.net.searching")
            }
            Flickable {
                id: netList
                visible: !root.wired && root.wifiPresent && root.wifiNetworks.length > 0
                // room for the focus ring and the lift on every side
                x: -Theme.px(22)
                width: parent.width + Theme.px(44)
                height: Math.min(netColumn.height, Theme.px(310))
                contentHeight: netColumn.height
                clip: true
                interactive: false
                Column {
                    id: netColumn
                    x: Theme.px(22)
                    width: parent.width - Theme.px(44)
                    spacing: Theme.px(10)
                    topPadding: Theme.px(12); bottomPadding: Theme.px(12)
                    Repeater {
                        model: root.wifiNetworks
                        ChoiceRow {
                            key: "net:" + modelData.ssid
                            text: modelData.ssid
                            detail: modelData.security === "open" ? Tr.tr("first.net.open") : Tr.tr("first.net.secured")
                            tag: modelData.connected ? Tr.tr("first.net.connected") : ""
                            wifiLevel: modelData.connected ? -1 : modelData.strength >= 67 ? 3 : modelData.strength >= 34 ? 2 : 1
                            onActivated: if (!modelData.connected && root.theme) root.theme.connectNetwork(modelData)
                        }
                    }
                }
            }
            Row {
                spacing: Theme.px(16)
                Button {
                    readonly property string key: "continue"
                    visible: root.wired || root.wifiConnected
                    text: Tr.tr("first.continue")
                    style: "primary"
                    onActivated: root.next()
                }
                Button {
                    visible: !root.wired
                    text: Tr.tr("first.net.later")
                    onActivated: root.next()
                }
            }
        }

        // ---- Time zone
        Column {
            visible: root.screen === "time" && root.ownCities.length === 1 && !root.choosing
            anchors.centerIn: parent
            width: Theme.px(860)
            spacing: Theme.px(14)
            Heading { text: Tr.tr("first.time.title"); horizontalAlignment: Text.AlignHCenter }
            Text {
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                text: root.clockAt(root.shownCity)
                color: Theme.fg
                font.family: Theme.displayFont
                font.weight: Font.Bold
                font.pixelSize: Theme.fs(40)
            }
            Text {
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                text: Tr.tr("city." + root.shownCity) + " · " + root.utcLabel(root.shownCity)
                color: Theme.fg
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(17)
            }
            Para { text: Tr.tr("first.time.suggested"); horizontalAlignment: Text.AlignHCenter; small: true }
            Row {
                anchors.horizontalCenter: parent.horizontalCenter
                spacing: Theme.px(16)
                Button {
                    text: Tr.tr("first.continue")
                    style: "primary"
                    onActivated: root.chooseCity(root.shownCity)
                }
                Button {
                    readonly property string key: "choose"
                    text: Tr.tr("first.time.other")
                    onActivated: { root.choosing = true; zoneList.contentY = 0; Qt.callLater(root.focusScreen); }
                }
            }
        }
        Column {
            visible: root.screen === "time" && (root.ownCities.length > 1 || root.choosing)
            anchors.centerIn: parent
            width: Theme.px(860)
            spacing: Theme.px(14)
            Heading { text: Tr.tr("first.time.choose") }
            Para { text: root.choosing ? Tr.tr("first.time.suggested") : Tr.tr("first.time.choose.text") }
            Flickable {
                id: zoneList
                x: -Theme.px(22)
                width: parent.width + Theme.px(44)
                height: Math.min(zoneColumn.height, Theme.px(340))
                contentHeight: zoneColumn.height
                clip: true
                interactive: false
                Column {
                    id: zoneColumn
                    x: Theme.px(22)
                    width: parent.width - Theme.px(44)
                    spacing: Theme.px(10)
                    topPadding: Theme.px(12); bottomPadding: Theme.px(12)
                    Repeater {
                        model: root.choosing ? root.allCities : root.ownCities
                        ChoiceRow {
                            key: "city:" + modelData
                            text: Tr.tr("city." + modelData)
                            value: root.clockAt(modelData) + "   " + root.utcLabel(modelData)
                            onActivated: root.chooseCity(modelData)
                        }
                    }
                }
            }
        }

        // ---- The Xbox wireless adapter's firmware
        Column {
            visible: root.screen === "dongle"
            anchors.centerIn: parent
            width: Theme.px(860)
            spacing: Theme.px(16)
            Heading { text: Tr.tr("first.dongle.title") }
            Para {
                text: Tr.tr("first.dongle.text")
                color: Theme.fg
            }
            Row {
                visible: root.dongleState === "installing"
                spacing: Theme.px(12)
                Spinner { anchors.verticalCenter: parent.verticalCenter }
                Text {
                    anchors.verticalCenter: parent.verticalCenter
                    text: Tr.tr("first.dongle.installing")
                    color: Theme.fg
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(17)
                }
            }
            Para {
                visible: root.dongleState === "failed"
                text: root.xone && root.xone.error === "offline" ? Tr.tr("first.dongle.failed.offline") : Tr.tr("first.dongle.failed")
                color: Theme.accent
            }
            Row {
                visible: root.dongleState !== "installing"
                spacing: Theme.px(16)
                Button {
                    text: root.dongleState === "failed" ? Tr.tr("prompt.retry") : Tr.tr("first.dongle.download")
                    style: "primary"
                    onActivated: root.downloadFirmware()
                }
                Button {
                    text: Tr.tr("first.dongle.later")
                    onActivated: { root.dongleState = ""; root.next(); }
                }
            }
        }

        // ---- Ready
        Column {
            visible: root.screen === "ready"
            anchors.centerIn: parent
            width: Theme.px(560)
            spacing: Theme.px(16)
            Text {
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                text: Tr.tr("first.ready.title")
                color: Theme.fg
                font.family: Theme.displayFont
                font.weight: Font.Bold
                font.pixelSize: Theme.fs(40)
                wrapMode: Text.Wrap
            }
            Para { text: Tr.tr("first.ready.text"); horizontalAlignment: Text.AlignHCenter }
            Repeater {
                model: [[Tr.tr("first.ready.lang"), Tr.languageName(Tr.language)],
                        [Tr.tr("first.ready.net"), root.networkName],
                        [Tr.tr("first.ready.time"), Tr.tr("city." + root.shownCity) + " · " + root.utcLabel(root.shownCity)]]
                ChoiceRow {
                    navigable: false
                    label: modelData[0]
                    value: modelData[1]
                }
            }
            Button {
                anchors.horizontalCenter: parent.horizontalCenter
                text: Tr.tr("first.ready.home")
                style: "primary"
                onActivated: root.finish()
            }
        }
    }
    // keeps the selected row of a list in sight
    function reveal(item) {
        const list = screen === "network" ? netList : screen === "time" ? zoneList : null;
        if (!list || !list.contentItem || !item)
            return;
        const p = item.mapToItem(list.contentItem, 0, 0);
        if (p.y < 0 || p.y > list.contentHeight)
            return;                       // not in the list (a button under it)
        const pad = Theme.px(12);
        if (p.y - pad < list.contentY)
            list.contentY = Math.max(0, p.y - pad);
        else if (p.y + item.height + pad > list.contentY + list.height)
            list.contentY = Math.min(list.contentHeight - list.height, p.y + item.height + pad - list.height);
    }

    PromptBar {
        id: prompts
        anchors {
            left: parent.left; right: parent.right; bottom: parent.bottom
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; bottomMargin: Theme.safeBottom
        }
        globalPrompts: []
        prompts: {
            if (root.screen === "pair" || root.dongleState === "installing")
                return [];
            const out = [{ buttons: ["a"], label: Tr.tr("prompt.select") }];
            if (root.screen !== "language" && root.screen !== "ready")
                out.push({ buttons: ["b"], label: Tr.tr("prompt.back") });
            const list = nav.scrollTarget;
            if (list && list.visible && list.contentHeight > list.height)
                out.push({ buttons: ["rs"], label: Tr.tr("prompt.scroll") });
            return out;
        }
    }
    Toast { id: toast }

    // ---- The screens' parts
    component Heading: Text {
        width: parent ? parent.width : 0
        color: Theme.fg
        font.family: Theme.displayFont
        font.weight: Font.Bold
        font.pixelSize: Theme.fs(24)
        wrapMode: Text.Wrap
    }
    component Para: Text {
        property bool small: false
        width: parent ? parent.width : 0
        color: Theme.muted
        font.family: Theme.textFont
        font.pixelSize: Theme.fs(small ? 14.5 : 17)
        wrapMode: Text.Wrap
    }
    // a row of a list: a name (or a label and its value), a tick, a tag or the Wi-Fi signal
    component ChoiceRow: Rectangle {
        id: row
        property string key: ""
        property string text: ""
        property string detail: ""
        property string label: ""
        property string value: ""
        property string tag: ""
        property bool ticked: false
        property int wifiLevel: 0        // 1-3: the signal; 0 or -1: none
        property bool navigable: true
        signal activated()
        width: parent ? parent.width : 0
        height: Math.max(Theme.px(60), texts.implicitHeight + Theme.px(28))
        radius: Theme.radiusM
        color: Theme.surface
        border.color: Theme.line
        border.width: Math.max(1, Theme.px(1))
        Column {
            id: texts
            anchors { left: parent.left; leftMargin: Theme.px(18); verticalCenter: parent.verticalCenter }
            width: parent.width * 0.62
            spacing: Theme.px(3)
            Text {
                width: parent.width
                text: row.label !== "" ? row.label : row.text
                color: row.label !== "" ? Theme.muted : Theme.fg
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(17)
                elide: Text.ElideRight
            }
            Text {
                visible: row.detail !== ""
                text: row.detail
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(14.5)
            }
        }
        Row {
            anchors { right: parent.right; rightMargin: Theme.px(18); verticalCenter: parent.verticalCenter }
            spacing: Theme.px(10)
            Text {
                visible: row.value !== ""
                anchors.verticalCenter: parent.verticalCenter
                text: row.value
                color: row.label !== "" ? Theme.fg : Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(17)
            }
            Rectangle {
                visible: row.tag !== "" || row.ticked
                anchors.verticalCenter: parent.verticalCenter
                width: tagText.implicitWidth + Theme.px(20)
                height: tagText.implicitHeight + Theme.px(4)
                radius: height / 2
                color: Theme.accent
                Text {
                    id: tagText
                    anchors.centerIn: parent
                    text: row.ticked ? "✓" : row.tag
                    color: Theme.onAccent
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                }
            }
            Icon {
                visible: row.wifiLevel > 0
                anchors.verticalCenter: parent.verticalCenter
                width: Theme.fs(22); height: width
                name: "wifi"
                level: row.wifiLevel
            }
        }
        FocusFrame {}
    }
}
