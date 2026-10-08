// SPDX-License-Identifier: GPL-3.0-or-later
// OwneetOS addition to the Pegasus Frontend source.

#pragma once

#include <QList>

class QObject;


namespace platform {

/// Tags the interface's windows for gamescope (OwneetOS console session).
///
/// gamescope runs with --steam: it shows only windows that carry an app id (X11 property
/// STEAM_GAME) and only once told which app to show (GAMESCOPECTRL_BASELAYER_APPID). owneetd
/// does both; doing them here as well (the second only if unset) keeps the interface visible
/// even when owneetd is not running. Does nothing outside X11.
void tag_windows_for_gamescope(const QList<QObject*>& root_objects);

} // namespace platform
