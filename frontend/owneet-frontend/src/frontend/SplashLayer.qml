// Pegasus Frontend
// Copyright (C) 2017-2018  Mátyás Mustoha
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

// OwneetOS: the loading screen looks exactly like the boot splash (Plymouth theme "owneet",
// package owneet-branding): same background, wordmark and spinner at the same places, so the
// handover from the boot splash to the interface is invisible.

import QtQuick 2.15
import QtQuick.Shapes 1.15


Rectangle {
    id: root
    color: "#0E1424"
    anchors.fill: parent

    // Kept for main.qml (Pegasus's loading progress); not shown.
    property real progress: 0
    property bool showDataProgressText: true
    property string stage: ""

    // Plymouth places images by their top-left corner: (screen - image) * alignment.
    Image {
        id: wordmark
        source: "file:///usr/share/owneet/branding/owneetos-wordmark.svg"
        sourceSize.width: 420
        x: Math.round((root.width - width) * 0.5)
        y: Math.round((root.height - height) * 0.46)
    }

    Shape {
        id: spinner
        width: 48
        height: 48
        x: Math.round((root.width - width) * 0.5)
        y: Math.round((root.height - height) * 0.64)
        layer.enabled: true
        layer.samples: 4

        ShapePath {
            strokeColor: Qt.rgba(1.0, 0.541, 0.357, 0.18)
            strokeWidth: 4
            fillColor: "transparent"
            PathAngleArc { centerX: 24; centerY: 24; radiusX: 18; radiusY: 18; startAngle: 0; sweepAngle: 360 }
        }
        ShapePath {
            strokeColor: "#FF8A5B"
            strokeWidth: 4
            fillColor: "transparent"
            capStyle: ShapePath.RoundCap
            PathAngleArc { centerX: 24; centerY: 24; radiusX: 18; radiusY: 18; startAngle: -90; sweepAngle: 90 }
        }

        RotationAnimation on rotation {
            from: 0; to: 360
            duration: 1200
            loops: Animation.Infinite
        }
    }
}
