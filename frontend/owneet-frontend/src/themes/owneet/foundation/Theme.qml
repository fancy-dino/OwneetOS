// SPDX-License-Identifier: GPL-3.0-or-later
// OwneetOS design tokens and scale ([[design-tokens]]; approved demo
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
    property bool uiSounds: true                         // [[step-3.12-interface-sounds]]
    property int uiSoundsVolume: 70
    property string keyboardSetting: "same"              // "same" as the language, or a layout
    readonly property int motionMs: reduceMotion ? 0 : 160
    property string glyphSetting: "auto"                 // auto, xbox, ps, nintendo
    property string padKind: "xbox"                      // from the connected controller
    readonly property string glyphs: glyphSetting === "auto" ? padKind : glyphSetting

    // ---- Before the start-up animation (boot splash, loading screen): white on black, the same
    // for every palette (owner, 2026-10-15); the animation brings the palette's colours
    readonly property color splashBg: "#000000"
    readonly property color splashFg: "#FFFFFF"
    readonly property color splashAccent: "#B0B0B0"     // the logo's symbol and "OS" there

    // ---- Fonts (bundled, SIL Open Font License)
    readonly property string displayFont: "Bricolage Grotesque"
    readonly property string textFont: "Lexend"

    // ---- Palettes: eight tokens each; their names are messages "palette.<key>" (i18n files). Every palette passes a contrast check: text >= 7:1,
    // secondary text and text on the accent >= 4.5:1, accent against the background >= 3:1.
    readonly property var palettes: [
        { key: "dusk", logoButtons: "#6FD3FF", group: "dark", c: ["#0E1424", "#172036", "#222D4A", "#2E3A5C", "#EEF1F8", "#8F9AB5", "#FF8A5B", "#1E0E06"] },
        { key: "tide", logoButtons: "#FF9E8A", group: "dark", c: ["#031A1C", "#06292D", "#0A3A3F", "#0F5157", "#E4FFFC", "#7ED9D0", "#2EF2DA", "#00231F"] },
        { key: "bloom", logoButtons: "#7FE8C8", group: "dark", c: ["#1A1020", "#25182D", "#33223D", "#45304F", "#F7EEF6", "#AE93AE", "#FF7AA8", "#2A0614"] },
        { key: "ember", logoButtons: "#7CCBFF", group: "dark", c: ["#1C0F0C", "#2A1713", "#3A201A", "#4E2C24", "#FBEDE9", "#C49C93", "#FF6B47", "#2A0A03"] },
        { key: "espresso", logoButtons: "#8AD0E8", group: "dark", c: ["#1A1310", "#261C17", "#33261F", "#47362C", "#F5ECE4", "#BBA693", "#D9A066", "#2A1707"] },
        { key: "harbor", logoButtons: "#8FC4F5", group: "dark", c: ["#0D1B2E", "#14263D", "#1D3350", "#2A4566", "#F3EAD8", "#B7AE99", "#E8D3A8", "#142033"] },
        { key: "forest", logoButtons: "#FFA8C8", group: "dark", c: ["#0E1A14", "#15261D", "#1E3428", "#2B4636", "#ECF5EE", "#95B2A0", "#7BD88F", "#072012"] },
        { key: "lagoon", logoButtons: "#FFC56B", group: "dark", c: ["#0A1426", "#101E38", "#182A4B", "#23395F", "#EAF1FF", "#90A4C9", "#4DA3FF", "#03162E"] },
        { key: "amethyst", logoButtons: "#FFD66B", group: "dark", c: ["#15121F", "#1F1A2E", "#2A2340", "#3A3155", "#F1EDFA", "#A99FC4", "#B69CFF", "#1B0E3A"] },
        { key: "crimson", logoButtons: "#7FE3EE", group: "dark", c: ["#170A0E", "#241018", "#321722", "#47202F", "#FCECEF", "#C597A1", "#FF4D6D", "#2B0510"] },
        { key: "synthwave", logoButtons: "#6FE8FF", group: "dark", c: ["#140B24", "#1F1236", "#2B1A4A", "#3D2563", "#FBEFFF", "#B99DD8", "#FF4FD8", "#2B0326"] },
        { key: "amber", logoButtons: "#8CBEFF", group: "dark", c: ["#0F0B05", "#1A1409", "#251C0D", "#3A2C14", "#FFE8C2", "#C9AA78", "#FFB000", "#241700"] },
        { key: "olive", logoButtons: "#CDB2FF", group: "dark", c: ["#14160E", "#1E2116", "#292D1F", "#3A402B", "#F1F3E6", "#ABB092", "#C5D86D", "#1C2306"] },
        { key: "graphite", group: "dark", c: ["#121212", "#1C1C1C", "#262626", "#383838", "#F2F2F2", "#A6A6A6", "#DADADA", "#121212"] },
        { key: "daylight", logoButtons: "#B8400B", group: "light", c: ["#EEF0F5", "#FFFFFF", "#E2E6EF", "#C9CFDC", "#121826", "#5A6378", "#2F5BEA", "#FFFFFF"] },
        { key: "arctic", logoButtons: "#B8400B", group: "light", c: ["#EEF5FA", "#FFFFFF", "#DCE9F3", "#C3D5E4", "#0F1E2B", "#4E6577", "#0A78B8", "#FFFFFF"] },
        { key: "sand", logoButtons: "#0B6A85", group: "light", c: ["#F4EEE3", "#FFFBF4", "#E8DFD0", "#D3C6B1", "#2A2117", "#6A5C49", "#B04A28", "#FFFFFF"] },
        { key: "latte", logoButtons: "#0B6A85", group: "light", c: ["#F3ECE4", "#FFFAF5", "#E6DACD", "#D2C1AF", "#2B1E14", "#6B5746", "#8B5A2B", "#FFFFFF"] },
        { key: "sakura", logoButtons: "#0A6F6A", group: "light", c: ["#FBEFF3", "#FFFFFF", "#F3DDE5", "#E3C3CF", "#2A1420", "#76505F", "#C92A62", "#FFFFFF"] },
        { key: "contrast", logoButtons: "#8AB4FF", group: "access", c: ["#000000", "#000000", "#161616", "#FFFFFF", "#FFFFFF", "#E0E0E0", "#FFE14D", "#000000"] },
        { key: "contrast-light", logoButtons: "#6E5200", group: "access", c: ["#FFFFFF", "#FFFFFF", "#EDEDED", "#000000", "#000000", "#262626", "#0038B8", "#FFFFFF"] }
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
    // the logo's D-pad and buttons: their own colour in each palette, at least 4.5:1 on its background
    // (7.6, owner); in Graphite, black and white, the symbol's colour
    readonly property color logoButtons: current.logoButtons || accent
    readonly property color onAccent: current.c[7]
    // "Running": a game or web app open behind the home screen (demo 3.6), the same in every palette
    readonly property color running: "#4ADE80"
    readonly property color runningInk: "#062010"

    // ---- Shapes
    readonly property real radiusS: px(10)
    readonly property real radiusM: px(14)
    readonly property real radiusL: px(18)
}
