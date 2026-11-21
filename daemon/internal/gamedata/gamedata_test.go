// SPDX-License-Identifier: GPL-3.0-or-later

package gamedata

import (
	"path/filepath"
	"testing"
)

// The same names as owneet-frontend's DiskGames::gameDataDir
func TestDir(t *testing.T) {
	got := Dir("/home/owneet", "windows/prefixes", "/home/owneet/Giochi/The Witcher 2 - Win")
	if filepath.Dir(got) != "/home/owneet/.local/share/owneet/windows/prefixes" || filepath.Base(got)[:len("the-witcher-2-win-")] != "the-witcher-2-win-" ||
		len(filepath.Base(got)) != len("the-witcher-2-win-")+10 {
		t.Fatalf("%q", got)
	}
	all := All("/home/owneet", "/home/owneet/Giochi/A")
	if len(all) != 3 || filepath.Base(all[2]) != filepath.Base(all[0]) || filepath.Dir(all[2]) != "/home/owneet/.local/share/owneet/windows/logs" {
		t.Fatalf("%v", all)
	}
}

func TestUnescape(t *testing.T) {
	for in, want := range map[string]string{
		`/run/media/owneet/2TB\040HDD\040SATA`: "/run/media/owneet/2TB HDD SATA",
		`/plain`:                               "/plain",
		`/odd\9x`:                              `/odd\9x`,
	} {
		if got := unescape(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}
