# SPDX-License-Identifier: GPL-3.0-or-later
# OwneetOS: games in the "Games" folders of the disks ([[step-4.3-local-drm-free-games]])
pegasus_add_provider(
    NAME "Games on disks"
    CXXID OWNEET_DISK
    INTERNAL
    SOURCES
        DiskGamesProvider.cpp
        DiskGamesProvider.h
    PLATFORMS
        X11
)
