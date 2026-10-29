// SPDX-License-Identifier: GPL-3.0-or-later

package live

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func useDir(t *testing.T) string {
	dir := t.TempDir()
	ModeFile, Marker, MemInfo = filepath.Join(dir, "mode"), filepath.Join(dir, "archiso"), filepath.Join(dir, "meminfo")
	t.Cleanup(func() { ModeFile, Marker, MemInfo = "/run/owneet/mode", "/run/archiso", "/proc/meminfo" })
	return dir
}

func TestMode(t *testing.T) {
	useDir(t)
	if got := Mode(); got != ModeInstalled {
		t.Errorf("no mode file, no marker: %q, want installed", got)
	}
	if err := os.Mkdir(Marker, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Mode(); got != ModeLive {
		t.Errorf("marker only: %q, want live", got)
	}
	for content, want := range map[string]string{"tester\n": ModeTester, "installed\n": ModeInstalled,
		"live\n": ModeLive, "nonsense\n": ModeLive} {
		if err := os.WriteFile(ModeFile, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := Mode(); got != want {
			t.Errorf("mode file %q: %q, want %q", content, got, want)
		}
	}
}

func TestAllowed(t *testing.T) {
	useDir(t)

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
