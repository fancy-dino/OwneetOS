# SPDX-License-Identifier: GPL-3.0-or-later
# OwneetOS additions to the backend
target_sources(pegasus-backend PRIVATE
    Daemon.cpp
    Daemon.h
    I18n.cpp
    I18n.h
    Storage.cpp
    Storage.h
    SystemInfo.cpp
    SystemInfo.h
)
