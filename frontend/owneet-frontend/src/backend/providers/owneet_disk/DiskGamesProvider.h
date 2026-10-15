// SPDX-License-Identifier: GPL-3.0-or-later
// Games on a disk ([[step-4.3-local-drm-free-games]]): every folder of a "Games" folder at the top
// of a mounted disk, or in the home folder, is a game that can start on Linux.

#pragma once

#include "providers/Provider.h"


namespace providers {
namespace owneet_disk {

class DiskGamesProvider : public Provider {
    Q_OBJECT

public:
    explicit DiskGamesProvider(QObject* parent = nullptr);

    Provider& run(SearchContext&) final;
};

} // namespace owneet_disk
} // namespace providers
