// SPDX-License-Identifier: GPL-3.0-or-later
// A section label: small, upper case, secondary color.
import QtQuick 2.15

Text {
    color: Theme.muted
    font.family: Theme.textFont
    font.weight: Font.Medium
    font.pixelSize: Theme.fs(12.5)
    font.capitalization: Font.AllUppercase
    font.letterSpacing: Theme.fs(12.5) * 0.09
}
