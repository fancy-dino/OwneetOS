// SPDX-License-Identifier: GPL-3.0-or-later
// OwneetOS addition to the Pegasus Frontend source.

#pragma once

#include <QObject>


namespace platform {

/// Hides the mouse pointer of a gamepad-first interface: hidden at start and on every key or
/// gamepad input, shown again as soon as the mouse moves (keyboard and mouse stay a fallback).
class CursorHider : public QObject {
    Q_OBJECT

public:
    explicit CursorHider(QObject* parent = nullptr);

protected:
    bool eventFilter(QObject* watched, QEvent* event) override;

private:
    bool m_hidden;

    void hide();
    void show();
};

} // namespace platform
