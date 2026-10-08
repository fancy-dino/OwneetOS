// SPDX-License-Identifier: GPL-3.0-or-later
// OwneetOS addition to the Pegasus Frontend source.

#include "GamescopeTag.h"

#include "Log.h"

#include <QGuiApplication>
#include <QWindow>

#include <xcb/xcb.h>

#include <cstdint>
#include <cstdlib>
#include <cstring>


namespace {

// Must match owneetd's apps.HomeAppID (daemon/internal/apps/apps.go).
constexpr uint32_t HOME_APP_ID = 0x7E000000;

const QString LOGTAG = QStringLiteral("Gamescope");

} // namespace


namespace platform {

void tag_windows_for_gamescope(const QList<QObject*>& root_objects)
{
    if (QGuiApplication::platformName() != QLatin1String("xcb"))
        return;

    xcb_connection_t* const conn = xcb_connect(nullptr, nullptr);
    if (xcb_connection_has_error(conn)) {
        xcb_disconnect(conn);
        Log::warning(LOGTAG, LOGMSG("Cannot connect to the X server to tag the window"));
        return;
    }

    const char name[] = "STEAM_GAME";
    xcb_intern_atom_reply_t* const reply = xcb_intern_atom_reply(conn,
        xcb_intern_atom(conn, 0, std::strlen(name), name), nullptr);
    if (reply) {
        for (QObject* const obj : root_objects) {
            auto* const window = qobject_cast<QWindow*>(obj);
            if (!window)
                continue;
            const auto xid = static_cast<xcb_window_t>(window->winId());
            xcb_change_property(conn, XCB_PROP_MODE_REPLACE, xid, reply->atom,
                                XCB_ATOM_CARDINAL, 32, 1, &HOME_APP_ID);
        }
        std::free(reply);
        xcb_flush(conn);
    }
    xcb_disconnect(conn);
}

} // namespace platform
