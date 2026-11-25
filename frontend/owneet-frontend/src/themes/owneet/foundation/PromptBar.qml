// SPDX-License-Identifier: GPL-3.0-or-later
// The prompt bar at the bottom of every screen: what the buttons do here (left) and the
// system-wide buttons (right). It sits inside the safe area and never moves.
// prompts: [{ buttons: ["a"], label: "Select" }, ...]
// The Home prompt only while nothing runs (Nav.homePrompt, owner 2026-11-14).
// When it does not fit (owner 2026-11-12, approved demo design/demos/3.3c-prompt-bar-fit.html): if
// less than twice the gap between prompts would stay between the two groups, the prompts on the
// right show their buttons only; the screen's own prompts keep their words. When even so the two
// groups would meet (the on-screen keyboard in German, text XL), the screen's own prompts scroll, as a
// text that does not fit does (FitText: still, to the end, still, back; owner, 2026-11-25).
import QtQuick 2.15

Item {
    id: root
    property var prompts: []
    property var globalPrompts: Nav.homePrompt ? [{ buttons: ["guide"], label: Tr.tr("prompt.home") }] : []
    property string note: ""
    implicitHeight: Theme.fs(26)

    // The right group's width with its words (measured on an unseen copy), whether they are shown or not
    readonly property real rightFull: fullRight.width
    readonly property bool tight: rightFull > 0 && leftRow.width + rightFull + 2 * Theme.px(26) > width

    Row {
        id: fullRight
        opacity: 0
        spacing: Theme.px(26)
        Text {
            visible: root.note !== ""
            text: root.note
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(14.5)
        }
        Repeater {
            model: root.globalPrompts
            Prompt { buttons: modelData.buttons; label: modelData.label }
        }
    }

    Item {
        id: leftBox
        readonly property real edge: Theme.px(14)
        readonly property real room: root.width - (rightRow.width > 0 ? rightRow.width + Theme.px(26) : 0)
        readonly property real overflow: Math.max(0, leftRow.width - room)
        readonly property bool scrolling: root.visible && overflow > 1
        anchors { left: parent.left; verticalCenter: parent.verticalCenter }
        width: scrolling ? room : leftRow.width
        height: leftRow.height
        clip: scrolling
        onScrollingChanged: { leftRow.x = 0; leftRow.opacity = 1; }
        Row {
            id: leftRow
            spacing: Theme.px(26)
            Repeater {
                model: root.prompts
                Prompt { buttons: modelData.buttons; label: modelData.label }
            }
        }
        SequentialAnimation {
            running: leftBox.scrolling
            loops: Animation.Infinite
            PauseAnimation { duration: 1500 }
            NumberAnimation {
                target: leftRow; property: "x"; from: 0; to: -(leftBox.overflow + leftBox.edge)
                duration: Math.max(1, (leftBox.overflow + leftBox.edge) / Theme.px(45) * 1000)
            }
            PauseAnimation { duration: 1500 }
            NumberAnimation { target: leftRow; property: "opacity"; to: 0; duration: 200 }
            PropertyAction { target: leftRow; property: "x"; value: 0 }
            NumberAnimation { target: leftRow; property: "opacity"; to: 1; duration: 200 }
        }
        layer.enabled: scrolling
        layer.effect: ShaderEffect {          // soft edges, as FitText's
            property real edge: leftBox.width > 0 ? leftBox.edge / leftBox.width : 0
            property real fadeLeft: leftRow.x < -0.5 ? 1 : 0
            property real fadeRight: leftRow.x > -(leftBox.overflow + leftBox.edge) + 0.5 ? 1 : 0
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
    Row {
        id: rightRow
        anchors { right: parent.right; verticalCenter: parent.verticalCenter }
        spacing: root.tight ? Theme.px(18) : Theme.px(26)
        Text {
            id: noteText
            visible: root.note !== ""
            anchors.verticalCenter: parent.verticalCenter
            text: root.note
            color: Theme.muted
            font.family: Theme.textFont
            font.pixelSize: Theme.fs(14.5)
        }
        Repeater {
            model: root.globalPrompts
            Prompt { buttons: modelData.buttons; label: modelData.label; compact: root.tight }
        }
    }
}
