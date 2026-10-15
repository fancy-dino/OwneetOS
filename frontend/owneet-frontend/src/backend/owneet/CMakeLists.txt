# SPDX-License-Identifier: GPL-3.0-or-later
# OwneetOS additions to the backend
target_sources(pegasus-backend PRIVATE
    Daemon.cpp
    Daemon.h
    DiskGames.cpp
    DiskGames.h
    I18n.cpp
    I18n.h
    Storage.cpp
    Storage.h
    SystemInfo.cpp
    SystemInfo.h
    # QR codes (MIT, Project Nayuki): the address of a game's licence agreement (4.2c)
    ${CMAKE_CURRENT_SOURCE_DIR}/../../../thirdparty/qrcodegen/qrcodegen.cpp
)
target_include_directories(pegasus-backend PRIVATE ${CMAKE_CURRENT_SOURCE_DIR}/../../../thirdparty/qrcodegen)
