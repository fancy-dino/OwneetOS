// SPDX-License-Identifier: GPL-3.0-or-later
// The console's keyboard for a web app's text field ([[step-5.2-extension-core]], approved demo
// design/demos/5.2-web-app-controls.html, [[extension-design]]): the console in front with the web
// app's name, a line with the field's label and text (dots for passwords, shown with the eye key)
// and the keyboard. Every edit goes to the page through owneetd (/v1/webapps/text): the extension
// types it into the field, whose cursor starts at the end like the line's. Done (Menu) moves the
// page's selection on to the next target; B closes. owneetd opens it ("webapp.keyboard"), and
// closes it by itself when it is not confirmed (/v1/webapps/keyboard/shown), when the interface
// stops, or when the web app exits. In gamescope the keyboard will be drawn over the page instead
// (part 6, owner 2026-10-17); until then both sessions show this screen.
import QtQuick 2.15
import "foundation"

FocusScope {
    id: root
    anchors.fill: parent
    visible: false
    z: 165

    property string layout: "us"
    property string appName: ""
    property string fieldLabel: ""
    property bool password: false
    property bool showPassword: false
    signal finished()                    // closed, by the user or by owneetd

    function open(k, name) {
        appName = name;
        fieldLabel = k.label || "";
        password = k.kind === "password";
        showPassword = false;
        line.text = k.value || "";
        line.cursorPosition = line.length;
        if (!visible)
            Nav.feedback("sheet-open");
        visible = true;
        keys.open(line, password);
        owneetd.post("/v1/webapps/keyboard/shown", {});
    }
    // Closed by owneetd (no confirmation, the interface or the web app stopped): nothing to send
    function dismiss() {
        if (!visible)
            return;
        keys.close(true);
        visible = false;
        finished();
    }
    // Done (next: the page's selection moves on) or B
    function finish(next) {
        if (!visible)
            return;
        owneetd.post("/v1/webapps/keyboard", { open: false, next: next });
        keys.close(next);                // Done already made its sound
        visible = false;
        finished();
    }
    function send(body) { owneetd.post("/v1/webapps/text", body); }

    Rectangle {
        anchors.fill: parent
        color: Theme.bg
    }
    Text {
        x: Theme.safeX; y: Theme.safeTop
        textFormat: Text.StyledText
        text: "Owneet<font color=\"" + Theme.accent + "\">OS</font>"
        color: Theme.fg
        font.family: Theme.displayFont
        font.weight: Font.ExtraBold
        font.pixelSize: Theme.px(23)
        font.letterSpacing: -Theme.px(0.7)
    }
    Column {
        anchors { horizontalCenter: parent.horizontalCenter; top: parent.top; topMargin: Theme.safeTop + Theme.px(44) }
        spacing: Theme.px(14)
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            text: root.appName
            color: Theme.fg
            font.family: Theme.displayFont
            font.weight: Font.Bold
            font.pixelSize: Theme.fs(28)
        }
        Rectangle {                      // the line: the field's label and text
            width: Theme.px(704)
            height: lineColumn.height + Theme.px(24)
            radius: Theme.px(12)
            color: Theme.surface
            border.color: Theme.accent
            border.width: Math.max(1, Theme.px(1))
            Column {
                id: lineColumn
                x: Theme.px(16); y: Theme.px(12)
                width: parent.width - Theme.px(32)
                spacing: Theme.px(4)
                Text {
                    visible: text !== ""
                    width: parent.width
                    text: root.fieldLabel
                    elide: Text.ElideRight
                    color: Theme.muted
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(14.5)
                }
                TextInput {
                    id: line
                    width: parent.width
                    clip: true
                    echoMode: root.password && !root.showPassword ? TextInput.Password : TextInput.Normal
                    passwordCharacter: "•"
                    color: Theme.fg
                    font.family: Theme.textFont
                    font.pixelSize: Theme.fs(17)
                    font.letterSpacing: echoMode === TextInput.Password ? Theme.fs(17) * 0.1 : 0
                    cursorVisible: root.visible
                    activeFocusOnPress: false
                }
            }
        }
    }
    Keyboard {
        id: keys
        layout: root.layout
        showPassword: root.showPassword
        onTogglePassword: root.showPassword = !root.showPassword
        onTyped: function(text) { root.send({ text: text }); }
        onDeleted: root.send({ key: "backspace" })
        onCursorMoved: function(step) { root.send({ key: step < 0 ? "left" : "right" }); }
        onDone: root.finish(true)
        onClosed: root.finish(false)
    }
}
