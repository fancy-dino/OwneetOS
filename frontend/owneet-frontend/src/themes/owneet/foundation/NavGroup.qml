// SPDX-License-Identifier: GPL-3.0-or-later
// A group of navigable items inside a NavArea (e.g. the home's hero, apps and row of games):
// the selection moves inside it along rows and columns, and leaves it at its edges (see Nav.find).
import QtQuick 2.15

Item {
    readonly property bool navGroup: true
    property Item remembered: null      // the item selected here last time
}
