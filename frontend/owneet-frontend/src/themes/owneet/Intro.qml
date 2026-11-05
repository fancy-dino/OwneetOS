// SPDX-License-Identifier: GPL-3.0-or-later
// The start-up animation ([[step-7.6-final-logo-and-brand]], design/demos/7.6-logo.html), once per boot
// with the start-up sound. A black screen stands between the boot splash and the session
// ([[black-screen-between-boot-splash-and-session]]), so it starts from black (owner, 2026-11-05): in
// four beats, over 2 s the logo fades in, in the palette's colours, and the black becomes the palette's
// background; at 2 s, 2.5 s and 3 s the slogan's three words appear, each at once. 0.8 s later it all
// fades into the pages in 0.4 s, 4.2 s in all, as on the website. "Reduce motion": a plain fade.
import QtQuick 2.15
import "foundation"

Item {
    id: root
    anchors.fill: parent
    z: 300
    visible: false

    property Item pages: null           // they come up as the animation fades
    readonly property bool running: visible
    property int words: 0               // the slogan's words on screen
    signal finished()

    // Shown at once, black, until go(): the start-up sound plays then
    function show() {
        content.opacity = 1;
        backdrop.opacity = 1;
        paletteLogo.opacity = 0;
        words = 0;
        if (pages) pages.opacity = 0;
        visible = true;
    }
    function go() {
        if (!visible || colours.running || beats.running || ending.running || fade.running)
            return;
        if (Theme.reduceMotion) {
            fade.start();
            return;
        }
        beats.beat = 0;
        beats.interval = 2000;
        beats.start();
        colours.start();
    }
    function done() {
        visible = false;
        if (pages) pages.opacity = 1;
        finished();
    }

    Item {
        id: content
        anchors.fill: parent
        Rectangle {                     // the palette's background, under the splash's black
            anchors.fill: parent
            color: Theme.bg
        }
        Rectangle {
            id: backdrop
            anchors.fill: parent
            color: Theme.splashBg
        }
        Item {                          // in the middle, unscaled, with the slogan's room below it
            width: LogoData.splashWidth
            height: LogoData.splashHeight
            x: Math.round((root.width - width) / 2)
            y: Math.round((root.height - height) / 2)
            Logo {
                id: paletteLogo
                layout: "above"
                size: LogoData.splashWidth
                opacity: 0
            }
            FontMetrics {
                id: sloganFont
                font.family: Theme.textFont
                font.pixelSize: LogoData.sloganSize
            }
            Row {                       // laid out at once: the line never moves as the words appear
                anchors.horizontalCenter: parent.horizontalCenter
                y: LogoData.splashLogoHeight + LogoData.sloganGap + LogoData.sloganSize * 0.72 - sloganFont.ascent
                Repeater {
                    model: ["startup.slogan.plug", "startup.slogan.play", "startup.slogan.own"]
                    Text {
                        text: (index > 0 ? " " : "") + Tr.tr(modelData)
                        opacity: index < root.words ? 1 : 0
                        color: Theme.muted
                        font: sloganFont.font
                    }
                }
            }
        }
    }

    // The logo fades in and the black goes, over the first beat
    ParallelAnimation {
        id: colours
        NumberAnimation { target: paletteLogo; property: "opacity"; to: 1; duration: 2000; easing.type: Easing.OutCubic }
        NumberAnimation { target: backdrop; property: "opacity"; to: 0; duration: 2000; easing.type: Easing.OutCubic }
    }
    // The beats, on a timer and not on animations (which follow the screen's frames), to stay with
    // the start-up sound: its hits fall on the three words
    Timer {
        id: beats
        property int beat: 0
        onTriggered: {
            beat++;
            if (beat > 3) {
                ending.start();
                return;
            }
            root.words = beat;
            interval = beat < 3 ? 500 : 800;
            start();
        }
    }
    SequentialAnimation {
        id: ending
        ParallelAnimation {
            NumberAnimation { target: content; property: "opacity"; to: 0; duration: 400 }
            NumberAnimation { target: root.pages; property: "opacity"; to: 1; duration: 400 }
        }
        ScriptAction { script: root.done() }
    }
    SequentialAnimation {
        id: fade
        ParallelAnimation {
            NumberAnimation { target: content; property: "opacity"; to: 0; duration: 600 }
            NumberAnimation { target: root.pages; property: "opacity"; to: 1; duration: 600 }
        }
        ScriptAction { script: root.done() }
    }
}
