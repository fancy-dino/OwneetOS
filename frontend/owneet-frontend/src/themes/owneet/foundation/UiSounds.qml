// SPDX-License-Identifier: GPL-3.0-or-later
// The interface sounds (roadmap 3.12, design/sounds.md): one per button press, the most specific
// one (Nav.feedback picks it). Every sound is loaded at start, so it plays at once. Settings →
// Sound turns them on or off and sets their volume; they are silent while a game is on screen.
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
