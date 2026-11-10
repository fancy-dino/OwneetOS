// SPDX-License-Identifier: GPL-3.0-or-later

package moves

import (
	"os"
	"path/filepath"
	"testing"
)

// The name owneet-frontend's DiskGames::gameDataDir gives (lower case, a-z 0-9 "-", 40 characters,
// the SHA-1's first 10 hex digits).
func TestDataDirIsTheInterfaces(t *testing.T) {
	got := dataDir("/home/owneet", "windows/prefixes", "/home/owneet/Giochi/The Witcher 2: Assassins of Kings Enhanced Edition")
	want := "/home/owneet/.local/share/owneet/windows/prefixes/the-witcher-2-assassins-of-kings-enhance-b98e96fa0d"
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// A moved game takes its prefix and its DOS data with it, and its install's link follows it; a
// copy (the original kept) leaves them to the original.
func TestMoveCarriesTheGamesOwnFolders(t *testing.T) {
	m, game, media := setup(t)
	prefix := dataDir(m.Home, "windows/prefixes", game)
	dos := dataDir(m.Home, "dos", game)
	for _, d := range []string{prefix, dos} {
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "save"), []byte("saved"), 0o644)
	}
	links := filepath.Join(m.Home, ".local", "share", "owneet", "installs")
	os.MkdirAll(links, 0o755)
	link := filepath.Join(links, "abcdef123456")
	os.Symlink(game, link)
	os.WriteFile(filepath.Join(game, installedMark), []byte(`{"link":"`+link+`"}`), 0o644)

	if err := m.Start(Request{Games: []Game{{Dir: game, Title: "Tyrian"}}, To: filepath.Join(media, "HDD"), Remove: true}); err != nil {
		t.Fatal(err)
	}
	st := finish(t, m)
	moved := filepath.Join(media, "HDD", "Games", "Tyrian")
	if len(st.Moved) != 1 || st.Moved[0] != moved {
		t.Fatalf("status %+v", st)
	}
	for _, kind := range dataKinds {
		if b, err := os.ReadFile(filepath.Join(dataDir(m.Home, kind, moved), "save")); err != nil || string(b) != "saved" {
			t.Errorf("%s not moved: %v", kind, err)
		}
		if _, err := os.Lstat(dataDir(m.Home, kind, game)); !os.IsNotExist(err) {
			t.Errorf("%s still at the old place", kind)
		}
	}
	if target, _ := os.Readlink(link); target != moved {
		t.Errorf("link → %q", target)
	}
}

func TestCopyLeavesTheGamesOwnFolders(t *testing.T) {
	m, game, media := setup(t)
	prefix := dataDir(m.Home, "windows/prefixes", game)
	os.MkdirAll(prefix, 0o755)
	if err := m.Start(Request{Games: []Game{{Dir: game, Title: "Tyrian"}}, To: filepath.Join(media, "HDD")}); err != nil {
		t.Fatal(err)
	}
	finish(t, m)
	if _, err := os.Stat(prefix); err != nil {
		t.Error("the original's prefix went")
	}
}

// A new place that already has its own folder keeps it, and the old one stays.
func TestMoveNeverReplacesAFolderAtTheNewPlace(t *testing.T) {
	m, game, media := setup(t)
	moved := filepath.Join(media, "HDD", "Games", "Tyrian")
	old, now := dataDir(m.Home, "windows/prefixes", game), dataDir(m.Home, "windows/prefixes", moved)
	for _, d := range []string{old, now} {
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "save"), []byte(d), 0o644)
	}
	if err := m.Start(Request{Games: []Game{{Dir: game, Title: "Tyrian"}}, To: filepath.Join(media, "HDD"), Remove: true}); err != nil {
		t.Fatal(err)
	}
	finish(t, m)
	for _, d := range []string{old, now} {
		if b, err := os.ReadFile(filepath.Join(d, "save")); err != nil || string(b) != d {
			t.Errorf("%s changed: %v", d, err)
		}
	}
}
