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
    /// Shows the overlay window (opaque, marked again as an external overlay).
    Q_INVOKABLE void showOverlay(QObject* window);
    /// Hides it: first fully transparent, which makes gamescope repaint without it, then unmapped
    /// (FrankenPC, 2026-10-17: once unmapped, its last picture stayed on screen over a still page).
    Q_INVOKABLE void hideOverlay(QObject* window);
};
} // namespace owneet
