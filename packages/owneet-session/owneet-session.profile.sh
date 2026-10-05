# SPDX-License-Identifier: GPL-3.0-or-later
# shellcheck shell=sh
# Starts the OwneetOS console session when the console user logs in on tty1 (autologin).
if [ "$(id -un)" = owneet ] && [ "$(tty)" = /dev/tty1 ] && [ -z "${WAYLAND_DISPLAY:-}${DISPLAY:-}" ]; then
    exec /usr/bin/owneet-session
fi
