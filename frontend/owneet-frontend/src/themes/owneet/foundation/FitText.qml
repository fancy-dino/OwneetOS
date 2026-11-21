// SPDX-License-Identifier: GPL-3.0-or-later
// A one-line text that may not fit its width ([[design-direction]], owner 2026-11-11; approved demo
// design/demos/3.3b-scrolling-text.html, 2026-11-12). It ends with "…"; while the item it belongs to
// is highlighted (the nearest selectable item around it, `owner`), a text that does not fit scrolls
// instead, so it can be read whole: 1.5 s still, then 45 px a second (1280 x 720 grid) to its end,
// 1.5 s still, a short fade back to the start, again. Soft edges while it has moved.
// Use it as a Text: text, color, font, horizontalAlignment; texts never highlighted keep "…".
import QtQuick 2.15

Item {
    id: root
    property alias text: label.text
    property alias color: label.color
    property alias font: label.font
    property alias horizontalAlignment: label.horizontalAlignment
    property alias style: label.style
    property alias styleColor: label.styleColor
    // The selectable item it belongs to; found by itself (the nearest one with `navigable`)
    property Item owner: null
    property bool active: owner ? owner.activeFocus : false

    readonly property real edge: Theme.px(14)
    readonly property real overflow: Math.max(0, label.implicitWidth - width)
    readonly property bool scrolling: active && overflow > 1 && visible

    implicitWidth: label.implicitWidth
    implicitHeight: label.implicitHeight
    baselineOffset: label.baselineOffset
    clip: true

    Component.onCompleted: {
        if (owner)
            return;
        for (let p = parent; p; p = p.parent) {
            if (p.navigable === true) {
                owner = p;
                return;
            }
        }
    }

    Text {
        id: label
        width: root.scrolling ? implicitWidth : root.width
        height: root.height
        elide: root.scrolling ? Text.ElideNone : Text.ElideRight
    }

    onScrollingChanged: {
        label.x = 0;
        label.opacity = 1;
    }
    SequentialAnimation {
        running: root.scrolling
        loops: Animation.Infinite
        PauseAnimation { duration: 1500 }
        NumberAnimation {
            target: label; property: "x"; from: 0; to: -(root.overflow + root.edge)
            duration: Math.max(1, (root.overflow + root.edge) / Theme.px(45) * 1000)
        }
        PauseAnimation { duration: 1500 }
        NumberAnimation { target: label; property: "opacity"; to: 0; duration: 200 }
        PropertyAction { target: label; property: "x"; value: 0 }
        NumberAnimation { target: label; property: "opacity"; to: 1; duration: 200 }
    }

    // Soft edges: on the left once the text has moved, on the right until its end shows
    layer.enabled: scrolling
    layer.effect: ShaderEffect {
        property real edge: root.width > 0 ? root.edge / root.width : 0
        property real fadeLeft: label.x < -0.5 ? 1 : 0
        property real fadeRight: label.x > -(root.overflow + root.edge) + 0.5 ? 1 : 0
        fragmentShader: "
            varying highp vec2 qt_TexCoord0;
            uniform sampler2D source;
            uniform lowp float qt_Opacity;
            uniform highp float edge;
            uniform lowp float fadeLeft;
            uniform lowp float fadeRight;
            void main() {
                highp float x = qt_TexCoord0.x;
                highp float a = mix(1.0, clamp(x / edge, 0.0, 1.0), fadeLeft)
                              * mix(1.0, clamp((1.0 - x) / edge, 0.0, 1.0), fadeRight);
                gl_FragColor = texture2D(source, qt_TexCoord0) * a * qt_Opacity;
            }"
    }
}
