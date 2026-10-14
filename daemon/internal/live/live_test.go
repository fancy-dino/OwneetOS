// SPDX-License-Identifier: GPL-3.0-or-later

package live

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestAllowed(t *testing.T) {
	dir := t.TempDir()
	Marker, MemInfo = filepath.Join(dir, "archiso"), filepath.Join(dir, "meminfo")
	t.Cleanup(func() { Marker, MemInfo = "/run/archiso", "/proc/meminfo" })

	if !Allowed(MinRAM) {
		t.Fatal("the installed system must always be allowed")
	}
	if err := os.Mkdir(Marker, 0o755); err != nil {
		t.Fatal(err)
	}
	for kb, want := range map[int]bool{8 * 1024 * 1024: false, 16 * 1024 * 1024: true} {
		data := []byte("MemTotal:       " + strconv.Itoa(kb) + " kB\nMemFree:        1 kB\n")
		if err := os.WriteFile(MemInfo, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := Allowed(MinRAM); got != want {
			t.Errorf("live system with %d kB: allowed %v, want %v", kb, got, want)
		}
	}
}
