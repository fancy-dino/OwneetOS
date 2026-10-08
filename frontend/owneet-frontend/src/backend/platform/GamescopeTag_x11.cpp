// SPDX-License-Identifier: GPL-3.0-or-later
// OwneetOS addition to the Pegasus Frontend source.

#include "GamescopeTag.h"

#include "Log.h"

#include <QGuiApplication>
#include <QPointer>
#include <QTimer>
#include <QWindow>

#include <xcb/xcb.h>

#include <cstdint>
#include <cstdlib>
#include <cstring>


namespace {

// Must match owneetd's apps.HomeAppID (daemon/internal/apps/apps.go).
constexpr uint32_t HOME_APP_ID = 0x7E000000;

const QString LOGTAG = QStringLiteral("Gamescope");

xcb_atom_t intern_atom(xcb_connection_t* conn, const char* name)
{
    xcb_intern_atom_reply_t* const reply = xcb_intern_atom_reply(conn,
        xcb_intern_atom(conn, 0, std::strlen(name), name), nullptr);
    const xcb_atom_t atom = reply ? reply->atom : XCB_ATOM_NONE;
    std::free(reply);
    return atom;
}

} // namespace


namespace {

// Tags the windows (STEAM_GAME = home app id) if not tagged yet, and, if nobody has chosen what
// gamescope shows (owneetd not running), shows the home screen. Never overrides a value set by
// someone else. Repeated a few times: at first the window is not mapped yet, and gamescope may
// still be starting.
void tag_and_show_home(const QList<QPointer<QWindow>>& windows)
{
    xcb_connection_t* const conn = xcb_connect(nullptr, nullptr);
    if (xcb_connection_has_error(conn)) {
        xcb_disconnect(conn);
        return;
    }
    const xcb_atom_t steam_game = intern_atom(conn, "STEAM_GAME");
    const xcb_atom_t baselayer = intern_atom(conn, "GAMESCOPECTRL_BASELAYER_APPID");

    if (steam_game != XCB_ATOM_NONE) {
        for (const QPointer<QWindow>& window : windows) {
            if (!window || !window->isVisible())
                continue;
            const auto xid = static_cast<xcb_window_t>(window->winId());
            xcb_get_property_reply_t* const current = xcb_get_property_reply(conn,
                xcb_get_property(conn, 0, xid, steam_game, XCB_ATOM_CARDINAL, 0, 1), nullptr);
            const bool tagged = current && xcb_get_property_value_length(current) == 4
                && *static_cast<uint32_t*>(xcb_get_property_value(current)) == HOME_APP_ID;
            std::free(current);
            if (!tagged) {
                xcb_change_property(conn, XCB_PROP_MODE_REPLACE, xid, steam_game,
                                    XCB_ATOM_CARDINAL, 32, 1, &HOME_APP_ID);
            }
        }
    }
    if (baselayer != XCB_ATOM_NONE) {
        xcb_screen_t* const screen = xcb_setup_roots_iterator(xcb_get_setup(conn)).data;
        xcb_get_property_reply_t* const current = xcb_get_property_reply(conn,
            xcb_get_property(conn, 0, screen->root, baselayer, XCB_ATOM_CARDINAL, 0, 16), nullptr);
        const bool unset = !current || xcb_get_property_value_length(current) == 0;
        std::free(current);
        if (unset) {
            xcb_change_property(conn, XCB_PROP_MODE_REPLACE, screen->root, baselayer,
                                XCB_ATOM_CARDINAL, 32, 1, &HOME_APP_ID);
            Log::info(LOGTAG, LOGMSG("Nothing chosen to show yet: showing the home screen"));
        }
    }
    xcb_flush(conn);
    xcb_disconnect(conn);
}

} // namespace


namespace platform {

void tag_windows_for_gamescope(const QList<QObject*>& root_objects)
{
    if (QGuiApplication::platformName() != QLatin1String("xcb"))
        return;

    QList<QPointer<QWindow>> windows;
    for (QObject* const obj : root_objects) {
        if (auto* const window = qobject_cast<QWindow*>(obj)) {
            windows.append(window);
            QObject::connect(window, &QWindow::visibleChanged, window, [windows](bool visible) {
                if (visible)
                    tag_and_show_home(windows);
            });
        }
    }
    tag_and_show_home(windows);
    for (const int ms : { 1000, 3000, 10000, 30000 })
        QTimer::singleShot(ms, [windows]() { tag_and_show_home(windows); });
}

} // namespace platform
