// SPDX-License-Identifier: GPL-3.0-or-later
// OwneetOS addition to the Pegasus Frontend source.

#pragma once

#include <QList>

class QObject;
class QWindow;


namespace platform {

/// Tags the interface's windows for gamescope (OwneetOS console session).
///
/// gamescope runs with --steam: it shows only windows that carry an app id (X11 property
/// STEAM_GAME) and only once told which app to show (GAMESCOPECTRL_BASELAYER_APPID). owneetd
/// does both; doing them here as well (the second only if unset) keeps the interface visible
/// even when owneetd is not running. Does nothing outside X11.
void tag_windows_for_gamescope(const QList<QObject*>& root_objects);

/// Makes a window a gamescope external overlay (X11 property GAMESCOPE_EXTERNAL_OVERLAY = 1):
/// gamescope draws it over the app on screen, blended with its transparency, and never gives it
/// the input focus (the app keeps it). Asks for a surface with an alpha channel, so call it before
/// the window is first shown. Does nothing outside X11.
void make_external_overlay(QWindow* window);

} // namespace platform
