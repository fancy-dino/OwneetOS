// SPDX-License-Identifier: GPL-3.0-or-later

// Package atomicfile saves small state files so that a power cut keeps either the old or the new
// content, never an empty or lost file. A console's power can be cut at any time; file systems
// such as btrfs write new data to the disk only every few seconds unless asked (the web apps the
// user chose were lost when the test VM was switched off, [[step-5.1-brave-in-the-os]]).
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write writes data to path through a temporary file: written, flushed to the disk, renamed over
// path, and the folder flushed so that the rename itself is on the disk. The folder is created if
// needed.
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
