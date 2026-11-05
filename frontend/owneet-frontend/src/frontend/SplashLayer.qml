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

// OwneetOS: the loading screen is black: the logo comes with the start-up animation, fading in from
// black in the palette's colours (owner, 2026-11-05).

import QtQuick 2.15


Rectangle {
    id: root
    color: "#000000"
    anchors.fill: parent

    // Kept for main.qml (Pegasus's loading progress); not shown.
    property real progress: 0
    property bool showDataProgressText: true
    property string stage: ""
}
