// SPDX-License-Identifier: GPL-3.0-or-later

// Package version holds the daemon version, set at build time with
// -ldflags "-X github.com/fancy-dino/OwneetOS/daemon/internal/version.Version=1.2.3".
package version

// Version of owneetd and owneetctl. "dev" for builds outside the package.
var Version = "dev"
