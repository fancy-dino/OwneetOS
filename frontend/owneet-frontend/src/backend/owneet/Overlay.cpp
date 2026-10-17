// SPDX-License-Identifier: GPL-3.0-or-later

#include "Overlay.h"

#include "model/internal/GamepadButtonNavigation.h"
#include "platform/GamescopeTag.h"

#include <QPointer>
#include <QTimer>
#include <QWindow>


namespace owneet {

Overlay::Overlay(QObject* parent)
    : QObject(parent)
{}

void Overlay::makeOverlay(QObject* window)
{
    platform::make_external_overlay(qobject_cast<QWindow*>(window));
}

void Overlay::routeGamepad(QObject* item)
{
    GamepadButtonNavigation::setKeyTarget(item);
}

void Overlay::showOverlay(QObject* object)
{
    QWindow* const window = qobject_cast<QWindow*>(object);
    if (!window)
        return;
    window->setOpacity(1.0);
    platform::set_overlay_kind(window, QStringLiteral("external"));
    window->show();
}

void Overlay::hideOverlay(QObject* object)
{
    QWindow* const window = qobject_cast<QWindow*>(object);
    if (!window)
        return;
    window->setOpacity(0.0);
    const QPointer<QWindow> later(window);
    QTimer::singleShot(150, window, [later]() {
        if (later && later->opacity() == 0.0)       // not shown again meanwhile
            later->hide();
    });
}

} // namespace owneet
