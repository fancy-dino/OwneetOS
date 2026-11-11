// SPDX-License-Identifier: GPL-3.0-or-later

package syncdisk

import (
	"path/filepath"
	"testing"
)

// A folder's disk is synced; a folder that is not there is an error.
func TestDir(t *testing.T) {
	if err := Dir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := Dir(filepath.Join(t.TempDir(), "gone")); err == nil {
		t.Fatal("no error for a missing folder")
	}
}
