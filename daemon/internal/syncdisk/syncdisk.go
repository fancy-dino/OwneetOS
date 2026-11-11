// SPDX-License-Identifier: GPL-3.0-or-later

// Package syncdisk makes a disk's free space right at once after files are removed. btrfs (the
// console's disk) counts the room of removed files as free only once it commits, up to 30 s later
// (owner, 2026-11-10: after a move the free space came very late,
// [[free-space-shown-late-after-removing-files]]); syncfs makes it commit.
package syncdisk

import (
	"os"

	"golang.org/x/sys/unix"
)

// Dir writes to the disk what the file system holding dir still keeps in memory (syncfs).
func Dir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return unix.Syncfs(int(f.Fd()))
}
