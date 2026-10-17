// SPDX-License-Identifier: GPL-3.0-or-later
// The on-screen keyboard ([[step-3.10-on-screen-keyboard]], approved demo
// design/demos/3.10-osk.html): it types into a TextInput of the interface. Five rows: digits and
// delete; letters; shift and punctuation; page, cursor left / right, space, show / hide (password
// fields) and Done. Pages of keys: letters (the layout of Settings → Language → Keyboard layout),
// symbols, accented letters of other languages. Buttons ([[controller-input-map]], on-screen
// keyboard): A types the selected key, X deletes, Y types a space, LB / RB change the page, LT held
// = capital letters while it is held (as on Steam's keyboard; owner, 2026-10-13), Menu = Done, B
// closes; the selection wraps around at every edge. A keyboard plugged in keeps typing as usual.
import QtQuick 2.15

FocusScope {
    id: root
    property Item target: null            // the TextInput to type into
    property bool password: false         // shows the show / hide key
    property bool showPassword: false
    property string layout: "us"          // us, it
    signal done()                         // Done (or Menu, or Enter on a keyboard)
    signal closed()                       // B
    signal togglePassword()               // the show / hide key
    // Each edit that went through (the web keyboard passes them on to the page, 5.2)
    signal typed(string text)
    signal deleted()
    signal cursorMoved(int step)

    visible: false
    anchors { left: parent.left; right: parent.right; bottom: parent.bottom }
    height: parent.height
    z: 160

    // ---- Keys
    readonly property var letterRows: ({
        us: ["1234567890", "qwertyuiop", "asdfghjkl'", "zxcvbnm,.-"],
        it: ["1234567890", "qwertyuiopè", "asdfghjklòàù", "zxcvbnm,.-"]
    })
    readonly property var symbolRows: ["1234567890", "@#€$%&*()_", "+=/\\:;\"!?'", "<>[]{}~^|`"]
    readonly property var accentRows: ["àáâäãåæ", "èéêëìíîï", "òóôöõøùúûü", "çñßÿ"]
    readonly property var pages: ["abc", "sym", "acc"]
    property string page: "abc"
    property int shift: 0                 // 0 off, 1 the next letter, 2 caps lock
    property bool held: false             // LT held: capital letters until it is released
    property int row: 1
    property int col: 0

    readonly property var rows: {
        const src = page === "abc" ? (letterRows[layout] || letterRows.us) : page === "sym" ? symbolRows : accentRows;
        const upper = (shift > 0 || held) && page !== "sym";
        const out = src.map(r => Array.from(r).map(c => ({ t: "char", c: upper ? c.toLocaleUpperCase() : c, w: 1 })));
        out[0].push({ t: "delete", w: 1.6 });
        out[out.length - 1].unshift({ t: "shift", w: 1.6 });
        const next = pages[(pages.indexOf(page) + 1) % pages.length];
        out.push([{ t: "page", p: next, w: 1.6 }, { t: "left", w: 1 }, { t: "space", w: 5.5 }, { t: "right", w: 1 }]
                 .concat(password ? [{ t: "eye", w: 1 }] : [], [{ t: "done", w: 2.1 }]));
        return out;
    }

    function open(input, isPassword) {
        target = input;
        password = isPassword === true;
        page = "abc";
        shift = 0;
        held = false;
        row = 1;
        col = 0;
        visible = true;
        forceActiveFocus();
    }
    function close(quiet) {
        if (!visible)
            return;
        visible = false;
        held = false;
        if (!quiet)
            Nav.feedback("sheet-close");
    }

    // ---- Typing
    function typeText(s) {
        if (!target)
            return;
        const max = target.maximumLength;
        if (max > 0 && target.length + s.length > max) {
            Nav.feedback("edge");
            return;
        }
        target.insert(target.cursorPosition, s);
        if (shift === 1)
            shift = 0;
        Nav.feedback("confirm");
        typed(s);
    }
    function deleteBack() {
        if (!target || target.cursorPosition === 0) {
            Nav.feedback("edge");
            return;
        }
        const p = target.cursorPosition;
        target.remove(p - 1, p);
        Nav.feedback("confirm");
        deleted();
    }
    function moveCursor(step) {
        const next = target ? target.cursorPosition + step : -1;
        if (!target || next < 0 || next > target.length) {
            Nav.feedback("edge");
            return;
        }
        target.cursorPosition = next;
        Nav.feedback("confirm");
        cursorMoved(step);
    }
    function setPage(p) {
        page = p;
        if (shift === 1)
            shift = 0;
        clampSelection();
        Nav.feedback("confirm");
    }
    function activate(k) {
        switch (k.t) {
        case "char": typeText(k.c); break;
        case "delete": deleteBack(); break;
        case "space": typeText(" "); break;
        case "shift": shift = (shift + 1) % 3; Nav.feedback("confirm"); break;
        case "page": setPage(k.p); break;
        case "left": moveCursor(-1); break;
        case "right": moveCursor(1); break;
        case "eye": togglePassword(); Nav.feedback("confirm"); break;
        case "done": Nav.feedback("confirm"); done(); break;
        }
    }

    // ---- Selection: it wraps around at every edge; up / down go to the key nearest in the row
    function keyItem(r, c) {
        const rowItem = keyColumn.children[r];
        if (!rowItem)
            return null;
        const keys = [];
        for (let i = 0; i < rowItem.children.length; i++)
            if (rowItem.children[i].isKey === true)
                keys.push(rowItem.children[i]);
        return keys[c] || null;
    }
    function clampSelection() {
        row = Math.min(row, rows.length - 1);
        col = Math.min(col, rows[row].length - 1);
    }
    function move(dir) {
        if (dir === "left" || dir === "right") {
            const n = rows[row].length;
            col = (col + (dir === "right" ? 1 : -1) + n) % n;
        } else {
            const from = keyItem(row, col);
            const x = from ? from.mapToItem(root, from.width / 2, 0).x : 0;
            row = (row + (dir === "down" ? 1 : -1) + rows.length) % rows.length;
            let best = 0, bestDx = Infinity;
            for (let c = 0; c < rows[row].length; c++) {
                const k = keyItem(row, c);
                if (!k)
                    continue;
                const dx = Math.abs(k.mapToItem(root, k.width / 2, 0).x - x);
                if (dx < bestDx) { bestDx = dx; best = c; }
            }
            col = best;
        }
        Nav.feedback("move");
    }

    Keys.onReleased: {
        if (api.keys.isPageUp(event) && !event.isAutoRepeat) {
            held = false;
            event.accepted = true;
        }
    }
    Keys.onPressed: {
        // A keyboard plugged in types, deletes and confirms as usual
        // (controller buttons and Ctrl combinations carry no printable text)
        const printable = event.text.length === 1 && event.text >= " " && event.text !== "\u007f";
        if (printable && event.key !== Qt.Key_Return && event.key !== Qt.Key_Enter) {
            typeText(event.text);
            event.accepted = true;
            return;
        }
        if (event.key === Qt.Key_Backspace) { deleteBack(); event.accepted = true; return; }
        if (event.key === Qt.Key_Return || event.key === Qt.Key_Enter) { Nav.feedback("confirm"); done(); event.accepted = true; return; }
        if (api.keys.isPageUp(event)) {        // LT: capitals while held (its repeats change nothing)
            if (!event.isAutoRepeat && !held) {
                held = true;
                Nav.feedback("toggle-on");
            }
            event.accepted = true;
            return;
        }
        const a = Nav.action(event);
        if (Nav.isDirection(a)) move(a);
        else if (a === "accept") activate(rows[row][col]);
        else if (a === "back") { closed(); }
        else if (a === "secondary") deleteBack();
        else if (a === "page") typeText(" ");
        else if (a === "section-prev" || a === "section-next")
            setPage(pages[(pages.indexOf(page) + (a === "section-next" ? 1 : pages.length - 1)) % pages.length]);
        else if (a === "options") { Nav.feedback("confirm"); done(); }
        else if (a !== "") Nav.feedback("edge");
        else return;
        event.accepted = true;
    }

    // ---- Look
    Rectangle {
        id: panel
        anchors {
            left: parent.left; right: parent.right; bottom: prompts.top
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; bottomMargin: Theme.px(14)
        }
        height: head.height + keyColumn.height + Theme.px(40)
        radius: Theme.radiusL
        color: Theme.surface
        border.color: Theme.line
        border.width: Math.max(1, Theme.px(1))

        Item {
            id: head
            anchors { left: parent.left; right: parent.right; top: parent.top; margins: Theme.px(16); topMargin: Theme.px(14) }
            height: pageRow.height
            Row {
                id: pageRow
                spacing: Theme.px(6)
                Glyph { button: "lb"; color: Theme.muted; anchors.verticalCenter: parent.verticalCenter; scale: 0.85 }
                Repeater {
                    model: root.pages
                    Rectangle {
                        anchors.verticalCenter: parent.verticalCenter
                        width: pageText.implicitWidth + Theme.px(24)
                        height: pageText.implicitHeight + Theme.px(6)
                        radius: height / 2
                        color: modelData === root.page ? Theme.fg : "transparent"
                        Text {
                            id: pageText
                            anchors.centerIn: parent
                            text: ({ abc: "ABC", sym: "?!#", acc: "àé" })[modelData]
                            color: modelData === root.page ? Theme.bg : Theme.muted
                            font.family: Theme.textFont
                            font.weight: Font.Medium
                            font.pixelSize: Theme.fs(12.5)
                        }
                    }
                }
                Glyph { button: "rb"; color: Theme.muted; anchors.verticalCenter: parent.verticalCenter; scale: 0.85 }
            }
            Text {
                anchors { right: parent.right; verticalCenter: parent.verticalCenter }
                text: Tr.tr("keyboard." + root.layout)
                color: Theme.muted
                font.family: Theme.textFont
                font.pixelSize: Theme.fs(12.5)
            }
        }

        Column {
            id: keyColumn
            anchors { top: head.bottom; topMargin: Theme.px(12); horizontalCenter: parent.horizontalCenter }
            spacing: Theme.px(8)
            readonly property real unit: Theme.px(76)
            Repeater {
                model: root.rows
                Row {
                    id: keyRow
                    readonly property int r: index
                    anchors.horizontalCenter: parent.horizontalCenter
                    spacing: Theme.px(8)
                    Repeater {
                        model: modelData
                        Rectangle {
                            readonly property bool isKey: true
                            readonly property bool selected: root.row === keyRow.r && root.col === index
                            readonly property bool fn: modelData.t !== "char"
                            readonly property bool lit: (modelData.t === "shift" && (root.shift > 0 || root.held)) || (modelData.t === "eye" && root.showPassword)
                            width: keyColumn.unit * modelData.w + Theme.px(8) * (modelData.w - 1)
                            height: Theme.px(46)
                            radius: Theme.radiusS + Theme.px(1)
                            color: modelData.t === "done" ? Theme.accent : lit ? Theme.fg
                                 : fn ? Qt.tint(Theme.raised, Qt.rgba(Theme.line.r, Theme.line.g, Theme.line.b, 0.3)) : Theme.raised
                            readonly property color ink: modelData.t === "done" ? Theme.onAccent : lit ? Theme.bg : Theme.fg
                            Text {
                                visible: modelData.t === "char" || modelData.t === "page"
                                anchors.centerIn: parent
                                text: modelData.t === "char" ? modelData.c : ({ abc: "ABC", sym: "?!#", acc: "àé" })[modelData.p] || ""
                                color: parent.ink
                                font.family: Theme.textFont
                                font.weight: modelData.t === "page" ? Font.Medium : Font.Normal
                                font.pixelSize: modelData.t === "page" ? Theme.fs(14.5) : Theme.fs(19)
                            }
                            Icon {
                                visible: ["delete", "shift", "left", "right", "eye"].indexOf(modelData.t) >= 0
                                anchors.centerIn: parent
                                width: Theme.fs(22); height: width
                                name: modelData.t === "shift" ? (root.shift === 2 ? "caps" : "shift")
                                    : modelData.t === "delete" ? "backspace" : modelData.t === "eye" ? "eye" : modelData.t
                                crossed: modelData.t === "eye" && root.showPassword
                                color: parent.ink
                            }
                            Row {
                                visible: modelData.t === "space" || modelData.t === "done"
                                anchors.centerIn: parent
                                spacing: Theme.px(6)
                                Glyph {
                                    button: modelData.t === "space" ? "y" : "menu"
                                    color: parent.parent.ink
                                    anchors.verticalCenter: parent.verticalCenter
                                    scale: 0.75
                                }
                                Text {
                                    anchors.verticalCenter: parent.verticalCenter
                                    text: Tr.tr(modelData.t === "space" ? "keyboard.space" : "keyboard.done")
                                    color: parent.parent.ink
                                    font.family: Theme.textFont
                                    font.weight: Font.Medium
                                    font.pixelSize: Theme.fs(14.5)
                                }
                            }
                            FocusFrame { shown: parent.selected && root.activeFocus }
                        }
                    }
                }
            }
        }
    }

    PromptBar {
        id: prompts
        anchors {
            left: parent.left; right: parent.right; bottom: parent.bottom
            leftMargin: Theme.safeX; rightMargin: Theme.safeX; bottomMargin: Theme.safeBottom
        }
        prompts: [
            { buttons: ["a"], label: Tr.tr("prompt.type") },
            { buttons: ["x"], label: Tr.tr("prompt.delete") },
            { buttons: ["y"], label: Tr.tr("prompt.space") },
            { buttons: ["lb", "rb"], label: Tr.tr("prompt.symbols") },
            { buttons: ["lt"], label: Tr.tr("prompt.capitals") },
            { buttons: ["menu"], label: Tr.tr("prompt.done") },
            { buttons: ["b"], label: Tr.tr("prompt.close_keyboard") }
        ]
    }
}
