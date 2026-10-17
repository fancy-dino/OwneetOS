// SPDX-License-Identifier: GPL-3.0-or-later
// Windows drawn over the app on screen in gamescope ([[step-5.2-extension-core]], part 6): the
// console's keyboard over a web page. gamescope never gives such a window the input focus, so the
// gamepad's keys are sent to an item of it directly. `overlay` in QML.

#pragma once

#include <QObject>


namespace owneet {
class Overlay : public QObject {
    Q_OBJECT

public:
    explicit Overlay(QObject* parent = nullptr);

    /// Makes `window` (a QML Window, not shown yet) a gamescope external overlay: transparent,
    /// drawn over the app on screen, without the input focus. Nothing outside X11.
    Q_INVOKABLE void makeOverlay(QObject* window);
    /// The gamepad's keys go to `item` (null: back to the focused window).
    Q_INVOKABLE void routeGamepad(QObject* item);
};
} // namespace owneet
