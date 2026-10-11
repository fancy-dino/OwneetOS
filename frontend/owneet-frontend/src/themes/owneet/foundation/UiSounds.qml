// SPDX-License-Identifier: GPL-3.0-or-later
// The interface sounds (roadmap 3.12, design/sounds.md): one per button press, the most specific
// one (Nav.feedback picks it). Every sound is loaded at start, so it plays at once. Settings →
// Sound turns them on or off and sets their volume; they are silent while a game is on screen.
// A silent sound loops from the start, so the sound output (e.g. a TV over HDMI) is running and in
// sync before the first real sound: otherwise the first one after boot is lost while the TV
// picks up the audio (owner's NiPoGi test, 2026-10-11).
import QtQuick 2.15
import QtMultimedia 5.15

QtObject {
    id: root
    property string dir: ""
    property bool muted: false

    // What Nav.feedback reports → the file that plays (eleven sounds; one "click" for many events)
    readonly property var fileOf: ({
        move: "nav-move", edge: "nav-edge",
        confirm: "nav-confirm", back: "nav-confirm", tab: "nav-confirm", "toggle-on": "nav-confirm", "toggle-off": "nav-confirm",
        section: "nav-section", "sheet-open": "sheet-open", "sheet-close": "sheet-close",
        notify: "notify", "notify-error": "notify-error", error: "error",
        launch: "game-launch", startup: "startup"
    })
    readonly property var files: ["nav-move", "nav-edge", "nav-confirm", "nav-section", "sheet-open", "sheet-close",
                                  "notify", "notify-error", "error", "game-launch", "startup"]
    // The slider is in steps of what the ear hears, not of the signal
    readonly property real gain: QtMultimedia.convertVolume(Theme.uiSoundsVolume / 100,
                                                            QtMultimedia.LogarithmicVolumeScale,
                                                            QtMultimedia.LinearVolumeScale)

    // The start-up sound waits until the output has been running for a moment and it is loaded
    property bool startupPending: false
    signal startupStarted()             // the start-up animation goes with it (3.13)
    function playStartup() { startupPending = true; startupTimer.restart(); }
    property Timer startupTimer: Timer {
        interval: 1200
        onTriggered: {
            const e = root.effects.objectAt(root.files.indexOf("startup"));
            if (!root.startupPending)
                return;
            if (e && e.status === SoundEffect.Ready && root.awake.playing) {
                root.startupPending = false;
                root.startupStarted();
                root.play("startup");
            } else if (e && e.status !== SoundEffect.Error) {
                restart();                      // not ready yet: try again
            }
        }
    }
    property SoundEffect awake: SoundEffect {
        source: root.dir !== "" ? "file://" + root.dir + "/silence.wav" : ""
        loops: SoundEffect.Infinite
        volume: 0.01                        // silence anyway; a stream at volume 0 may be skipped
        Component.onCompleted: play()
    }

    function play(kind) {
        if (!Theme.uiSounds || muted || dir === "")
            return;
        const effect = effects.objectAt(files.indexOf(fileOf[kind]));
        if (effect)
            effect.play();
    }

    property Instantiator effects: Instantiator {
        model: root.files
        delegate: SoundEffect {
            source: root.dir !== "" ? "file://" + root.dir + "/" + modelData + ".wav" : ""
            volume: root.gain
        }
    }
}
