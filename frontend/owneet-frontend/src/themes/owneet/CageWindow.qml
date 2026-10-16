// SPDX-License-Identifier: GPL-3.0-or-later
// The cage session shows only the newest window ([[console-session]]). owneetd decides what is on
// screen ("focus.changed") and, in cage, the interface carries it out with its own window
// (owneetd 0.12.2, [[step-5.2-extension-core]]): to come in front of a running app (the web
// keyboard, "Close {name}?") it shows its window again, which makes it the newest; to give the
// screen back it hides it, and cage shows the app. When an app starts nothing is done here: the
// app's window covers the console as soon as it appears.
import QtQuick 2.15
import QtQuick.Window 2.15

Item {
    id: root
    property bool active: false        // cage session
    property bool raised: false        // the console was brought over a running app
    property bool hidden: false        // the console's window is hidden: an app is on screen
    property string focus: ""          // what owneetd last put on screen
    property var gone: ({})            // apps that exited (their event comes before the focus)
    property bool fullScreen: true     // how the window was shown before it was hidden

    function showWindow() {
        const w = Window.window;
        if (fullScreen) w.showFullScreen(); else w.show();
        hidden = false;
    }
    function hideWindow() {
        const w = Window.window;
        fullScreen = w.visibility === Window.FullScreen;
        w.hide();
        hidden = true;
    }

    Connections {
        target: owneetd
        function onEvent(type, data) {
            if (type === "app.started") {
                delete root.gone[data.id];
                return;
            }
            if (type === "app.exited") {
                root.gone[data.id] = true;
                return;
            }
            if (type !== "focus.changed")
                return;
            const before = root.focus;
            root.focus = data.focus || "";
            if (!root.active)
                return;
            const appRunning = before !== "" && before !== "home" && !root.gone[before];
            if (root.focus === "home") {
                if (root.hidden) {
                    root.showWindow();             // back on screen (over the app, or alone)
                } else if (appRunning) {
                    root.hideWindow();             // under the app: shown again, it is the newest
                    root.showWindow();
                } else {
                    return;
                }
                root.raised = appRunning;
            } else if (root.raised) {
                root.raised = false;
                root.hideWindow();                 // the app comes back
            }
        }
    }
}
