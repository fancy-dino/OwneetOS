// SPDX-License-Identifier: GPL-3.0-or-later

#include "Overlay.h"

#include "model/internal/GamepadButtonNavigation.h"
#include "platform/GamescopeTag.h"

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

void Overlay::setOverlayKind(QObject* window, const QString& kind)
{
    platform::set_overlay_kind(qobject_cast<QWindow*>(window), kind);
}

} // namespace owneet
