// SPDX-License-Identifier: GPL-3.0-or-later

package moves

import (
	"encoding/json"
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

// A copy (the original kept) gets a copy of the original's own folders; the original keeps its own.
func TestCopyCopiesTheGamesOwnFolders(t *testing.T) {
	m, game, media := setup(t)
	for _, kind := range dataKinds {
		d := dataDir(m.Home, kind, game)
		os.MkdirAll(filepath.Join(d, "pfx"), 0o755)
		os.WriteFile(filepath.Join(d, "pfx", "save"), []byte(kind), 0o644)
		os.Symlink("../drive_c", filepath.Join(d, "pfx", "c:"))
	}
	if err := m.Start(Request{Games: []Game{{Dir: game, Title: "Tyrian"}}, To: filepath.Join(media, "HDD")}); err != nil {
		t.Fatal(err)
	}
	st := finish(t, m)
	copied := filepath.Join(media, "HDD", "Games", "Tyrian")
	if len(st.Moved) != 1 || st.Moved[0] != copied || len(st.Failed) != 0 {
		t.Fatalf("status %+v", st)
	}
	for _, kind := range dataKinds {
		for _, dir := range []string{game, copied} {
			d := dataDir(m.Home, kind, dir)
			if b, err := os.ReadFile(filepath.Join(d, "pfx", "save")); err != nil || string(b) != kind {
				t.Errorf("%s of %s: %q %v", kind, dir, b, err)
			}
			if link, _ := os.Readlink(filepath.Join(d, "pfx", "c:")); link != "../drive_c" {
				t.Errorf("%s of %s: link %q", kind, dir, link)
			}
		}
	}
}

// When the own folders cannot be copied, the game's copy did not work: it goes, with what was
// copied of its own folders; the original and its own folders are untouched.
func TestCopyFailsWithoutItsOwnFolders(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every file")
	}
	m, game, media := setup(t)
	prefix := dataDir(m.Home, "windows/prefixes", game)
	os.MkdirAll(prefix, 0o755)
	os.WriteFile(filepath.Join(prefix, "a"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(prefix, "secret"), []byte("x"), 0o000)
	if err := m.Start(Request{Games: []Game{{Dir: game, Title: "Tyrian"}}, To: filepath.Join(media, "HDD")}); err != nil {
		t.Fatal(err)
	}
	st := finish(t, m)
	copied := filepath.Join(media, "HDD", "Games", "Tyrian")
	if len(st.Failed) != 1 || len(st.Moved) != 0 {
		t.Fatalf("status %+v", st)
	}
	if _, err := os.Lstat(copied); !os.IsNotExist(err) {
		t.Error("the copy is still there")
	}
	if _, err := os.Lstat(dataDir(m.Home, "windows/prefixes", copied)); !os.IsNotExist(err) {
		t.Error("the partial prefix is still there")
	}
	if _, err := os.Stat(filepath.Join(prefix, "a")); err != nil {
		t.Error("the original's prefix was touched")
	}
}

// A shutdown stops the copy of the own folders: what was copied goes.
func TestCopyOwnStopsAtShutdown(t *testing.T) {
	m, game, media := setup(t)
	prefix := dataDir(m.Home, "windows/prefixes", game)
	os.MkdirAll(prefix, 0o755)
	os.WriteFile(filepath.Join(prefix, "save"), []byte("saved"), 0o644)
	m.aborting = true
	to := filepath.Join(media, "HDD", "Games", "Tyrian")
	if err := m.copyOwn("Tyrian", game, to); err != errStopped {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Lstat(dataDir(m.Home, "windows/prefixes", to)); !os.IsNotExist(err) {
		t.Error("the partial prefix is still there")
	}
}

// The copies' own folders count for the room on the console's disk when the games go there.
func TestCopyNeedsRoomForItsOwnFolders(t *testing.T) {
	m, game, _ := setup(t)
	prefix := dataDir(m.Home, "windows/prefixes", game)
	os.MkdirAll(prefix, 0o755)
	os.WriteFile(filepath.Join(prefix, "big"), make([]byte, 1<<20), 0o644)
	m.Free = func(string) (int64, error) { return size(game) + 1000, nil }
	if err := m.Start(Request{Games: []Game{{Dir: game, Title: "Tyrian"}}, To: Internal}); err != ErrNoSpace {
		t.Fatalf("copy: got %v", err)
	}
	// a move takes its own folders along: no room needed for them
	if err := m.Start(Request{Games: []Game{{Dir: game, Title: "Tyrian"}}, To: Internal, Remove: true}); err != nil {
		t.Fatalf("move: got %v", err)
	}
	finish(t, m)
}

// The file chosen to start a game follows it: a copy gets it too, a move takes it.
func TestTheChosenFileFollowsTheGame(t *testing.T) {
	for _, remove := range []bool{false, true} {
		m, game, media := setup(t)
		other := `"/run/media/owneet/X/Games/Other": "Other.exe"`
		os.MkdirAll(filepath.Dir(m.choicesFile()), 0o755)
		os.WriteFile(m.choicesFile(), []byte(`{`+other+`, "`+game+`": "DOSBOX/start game.exe"}`), 0o644)
		if err := m.Start(Request{Games: []Game{{Dir: game, Title: "Tyrian"}}, To: filepath.Join(media, "HDD"), Remove: remove}); err != nil {
			t.Fatal(err)
		}
		finish(t, m)
		data, _ := os.ReadFile(m.choicesFile())
		var choices map[string]string
		if err := json.Unmarshal(data, &choices); err != nil {
			t.Fatal(err)
		}
		to := filepath.Join(media, "HDD", "Games", "Tyrian")
		if choices[to] != "DOSBOX/start game.exe" || choices["/run/media/owneet/X/Games/Other"] != "Other.exe" {
			t.Errorf("remove %v: %v", remove, choices)
		}
		if _, kept := choices[game]; kept == remove {
			t.Errorf("remove %v: the original's entry %v", remove, choices)
		}
	}
}

// No choices yet: nothing is written.
func TestNoChoiceNoFile(t *testing.T) {
	m, game, media := setup(t)
	if err := m.Start(Request{Games: []Game{{Dir: game, Title: "Tyrian"}}, To: filepath.Join(media, "HDD")}); err != nil {
		t.Fatal(err)
	}
	finish(t, m)
	if _, err := os.Stat(m.choicesFile()); !os.IsNotExist(err) {
		t.Error("a choices file was written")
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
