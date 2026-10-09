// SPDX-License-Identifier: GPL-3.0-or-later
// OwneetOS design tokens and scale (PROJECT_RULES.md section 9; approved demo
// design/demos/3.3-theme-foundations.html). A screen never contains a hard-coded color or size:
// it uses these.
pragma Singleton
import QtQuick 2.15

QtObject {
    // ---- Scale: everything is designed on a 1280 x 720 grid. The theme root sets the size.
    property real width: 1280
    property real height: 720
    readonly property real scale: Math.min(width / 1280, height / 720)
    function px(v) { return v * scale }                 // layout sizes
    function fs(v) { return v * scale * textScale }     // text sizes (follow "Text size")
    // TV safe area: 5% horizontally, 5% at the top
    readonly property real safeX: px(64)
    readonly property real safeTop: px(36)
    readonly property real safeBottom: px(28)

    // ---- Preferences (saved by the theme root)
    property real textScale: 1.0                         // 0.9, 1.0, 1.2, 1.4
    property bool reduceMotion: false
    property bool showSafeArea: false                    // dashed outline of the TV safe area
    property bool uiSounds: true                         // interface sounds (step 3.12)
    property int uiSoundsVolume: 70
    property string keyboardSetting: "same"              // "same" as the language, or a layout
    readonly property int motionMs: reduceMotion ? 0 : 160
    property string glyphSetting: "auto"                 // auto, xbox, ps, nintendo
    property string padKind: "xbox"                      // from the connected controller
    readonly property string glyphs: glyphSetting === "auto" ? padKind : glyphSetting

    // ---- Fonts (bundled, SIL Open Font License)
    readonly property string displayFont: "Bricolage Grotesque"
    readonly property string textFont: "Lexend"

    // ---- Palettes: eight tokens each; their names are messages "palette.<key>" (i18n files). Every palette passes a contrast check: text >= 7:1,
    // secondary text and text on the accent >= 4.5:1, accent against the background >= 3:1.
    readonly property var palettes: [
        { key: "dusk", group: "dark", c: ["#0E1424", "#172036", "#222D4A", "#2E3A5C", "#EEF1F8", "#8F9AB5", "#FF8A5B", "#1E0E06"] },
        { key: "tide", group: "dark", c: ["#031A1C", "#06292D", "#0A3A3F", "#0F5157", "#E4FFFC", "#7ED9D0", "#2EF2DA", "#00231F"] },
        { key: "bloom", group: "dark", c: ["#1A1020", "#25182D", "#33223D", "#45304F", "#F7EEF6", "#AE93AE", "#FF7AA8", "#2A0614"] },
        { key: "ember", group: "dark", c: ["#1C0F0C", "#2A1713", "#3A201A", "#4E2C24", "#FBEDE9", "#C49C93", "#FF6B47", "#2A0A03"] },
        { key: "espresso", group: "dark", c: ["#1A1310", "#261C17", "#33261F", "#47362C", "#F5ECE4", "#BBA693", "#D9A066", "#2A1707"] },
        { key: "harbor", group: "dark", c: ["#0D1B2E", "#14263D", "#1D3350", "#2A4566", "#F3EAD8", "#B7AE99", "#E8D3A8", "#142033"] },
        { key: "forest", group: "dark", c: ["#0E1A14", "#15261D", "#1E3428", "#2B4636", "#ECF5EE", "#95B2A0", "#7BD88F", "#072012"] },
        { key: "lagoon", group: "dark", c: ["#0A1426", "#101E38", "#182A4B", "#23395F", "#EAF1FF", "#90A4C9", "#4DA3FF", "#03162E"] },
        { key: "amethyst", group: "dark", c: ["#15121F", "#1F1A2E", "#2A2340", "#3A3155", "#F1EDFA", "#A99FC4", "#B69CFF", "#1B0E3A"] },
        { key: "crimson", group: "dark", c: ["#170A0E", "#241018", "#321722", "#47202F", "#FCECEF", "#C597A1", "#FF4D6D", "#2B0510"] },
        { key: "synthwave", group: "dark", c: ["#140B24", "#1F1236", "#2B1A4A", "#3D2563", "#FBEFFF", "#B99DD8", "#FF4FD8", "#2B0326"] },
        { key: "amber", group: "dark", c: ["#0F0B05", "#1A1409", "#251C0D", "#3A2C14", "#FFE8C2", "#C9AA78", "#FFB000", "#241700"] },
        { key: "olive", group: "dark", c: ["#14160E", "#1E2116", "#292D1F", "#3A402B", "#F1F3E6", "#ABB092", "#C5D86D", "#1C2306"] },
        { key: "graphite", group: "dark", c: ["#121212", "#1C1C1C", "#262626", "#383838", "#F2F2F2", "#A6A6A6", "#DADADA", "#121212"] },
        { key: "daylight", group: "light", c: ["#EEF0F5", "#FFFFFF", "#E2E6EF", "#C9CFDC", "#121826", "#5A6378", "#2F5BEA", "#FFFFFF"] },
        { key: "arctic", group: "light", c: ["#EEF5FA", "#FFFFFF", "#DCE9F3", "#C3D5E4", "#0F1E2B", "#4E6577", "#0A78B8", "#FFFFFF"] },
        { key: "sand", group: "light", c: ["#F4EEE3", "#FFFBF4", "#E8DFD0", "#D3C6B1", "#2A2117", "#6A5C49", "#B04A28", "#FFFFFF"] },
        { key: "latte", group: "light", c: ["#F3ECE4", "#FFFAF5", "#E6DACD", "#D2C1AF", "#2B1E14", "#6B5746", "#8B5A2B", "#FFFFFF"] },
        { key: "sakura", group: "light", c: ["#FBEFF3", "#FFFFFF", "#F3DDE5", "#E3C3CF", "#2A1420", "#76505F", "#C92A62", "#FFFFFF"] },
        { key: "contrast", group: "access", c: ["#000000", "#000000", "#161616", "#FFFFFF", "#FFFFFF", "#E0E0E0", "#FFE14D", "#000000"] },
        { key: "contrast-light", group: "access", c: ["#FFFFFF", "#FFFFFF", "#EDEDED", "#000000", "#000000", "#262626", "#0038B8", "#FFFFFF"] }
    ]
    function palette(key) {
        for (let i = 0; i < palettes.length; i++)
            if (palettes[i].key === key)
                return palettes[i];
        return palettes[0];
    }
    property string paletteKey: "dusk"
    property string previewKey: ""                       // live preview while choosing
    readonly property var current: palette(previewKey !== "" ? previewKey : paletteKey)

    readonly property color bg: current.c[0]
    readonly property color surface: current.c[1]
    readonly property color raised: current.c[2]
    readonly property color line: current.c[3]
    readonly property color fg: current.c[4]
    readonly property color muted: current.c[5]
    readonly property color accent: current.c[6]
    readonly property color onAccent: current.c[7]

    // ---- Shapes
    readonly property real radiusS: px(10)
    readonly property real radiusM: px(14)
    readonly property real radiusL: px(18)
}
