// SPDX-License-Identifier: GPL-3.0-or-later

package installs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// a disk the console mounted: media/<disk>/Games
func mediaGames(t *testing.T, m *Manager) string {
	t.Helper()
	m.Media = filepath.Join(t.TempDir(), "media")
	games := filepath.Join(m.Media, "2TB HDD", "Games")
	if err := os.MkdirAll(games, 0o755); err != nil {
		t.Fatal(err)
	}
	return games
}

func mkGame(t *testing.T, dir string, files ...string) {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	for _, f := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDelete(t *testing.T) {
	m, home, _, _ := newManager(t)
	games := mediaGames(t, m)
	onDisk := filepath.Join(games, "Cayne")
	mkGame(t, onDisk, "start.sh", "saves/slot1")
	here := filepath.Join(home, "Giochi", "Tyrian 2000")
	mkGame(t, here, "DOSBOX/DOSBox.exe")
	for _, d := range []string{onDisk, here} {
		if err := m.Delete(d); err != nil || exists(d) {
			t.Fatalf("Delete(%s): %v, still there %v", d, err, exists(d))
		}
	}
	// never outside a games folder, the games folder itself, a link, or what is running
	mkGame(t, filepath.Join(home, "Documents", "x"), "a")
	link := filepath.Join(games, "Link")
	os.Symlink(filepath.Join(home, "Documents"), link)
	busy := filepath.Join(games, "Running")
	mkGame(t, busy, "game")
	m.InUse = func(dir string) bool { return dir == busy }
	for name, c := range map[string]struct {
		dest string
		want error
	}{
		"outside":          {filepath.Join(home, "Documents", "x"), ErrInvalid},
		"the games folder": {games, ErrInvalid},
		"the disk":         {filepath.Dir(games), ErrInvalid},
		"a link":           {link, ErrInvalid},
		"relative":         {"Giochi/x", ErrInvalid},
		"dot dot":          {filepath.Join(games, "..", "..", "x"), ErrInvalid},
		"running":          {busy, ErrInUse},
		"gone":             {filepath.Join(games, "Nothing"), ErrNotFound},
		"a hidden folder":  {filepath.Join(games, ".trash"), ErrInvalid},
	} {
		if err := m.Delete(c.dest); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	if !exists(filepath.Join(home, "Documents", "x", "a")) || !exists(busy) {
		t.Fatal("something was deleted that should not")
	}
}

func writeMark(t *testing.T, dest string, mark map[string]any) {
	t.Helper()
	data, _ := json.Marshal(mark)
	if err := os.WriteFile(filepath.Join(dest, MarkInstalled), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteInstaller(t *testing.T) {
	m, home, _, _ := newManager(t)
	games := mediaGames(t, m)
	// the installer in a folder of its own, on another disk: that folder goes, the game stays
	setupDir := filepath.Join(games, "The Witcher 2")
	mkGame(t, setupDir, "setup_tw2.exe", "setup_tw2-1.bin", "setup_tw2-2.bin")
	game := filepath.Join(home, "Giochi", "The Witcher 2")
	mkGame(t, game, "bin/witcher2.exe")
	writeMark(t, game, map[string]any{"installer": filepath.Join(setupDir, "setup_tw2.exe"), "kind": KindInno, "keep": []string{}})
	if err := m.DeleteInstaller(game); err != nil || exists(setupDir) || !exists(filepath.Join(game, "bin", "witcher2.exe")) {
		t.Fatalf("own folder: %v, installer there %v", err, exists(setupDir))
	}
	mark, _, _ := readMark(game)
	if mark.Installer != "" {
		t.Errorf("mark keeps the installer: %+v", mark)
	}
	if err := m.DeleteInstaller(game); !errors.Is(err, ErrNotFound) {
		t.Errorf("a second time: %v", err)
	}
	// the installer in the game's own folder: only its files go, and the mark forgets them
	game2 := filepath.Join(games, "Shadow Harbor")
	mkGame(t, game2, "setup_sh.exe", "setup_sh-1.bin", "EULA.txt", "game.exe")
	writeMark(t, game2, map[string]any{"installer": filepath.Join(game2, "setup_sh.exe"), "kind": KindInno,
		"keep": []string{"setup_sh.exe", "setup_sh-1.bin", "EULA.txt"}})
	if err := m.DeleteInstaller(game2); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(game2, "setup_sh.exe")) || exists(filepath.Join(game2, "setup_sh-1.bin")) ||
		!exists(filepath.Join(game2, "game.exe")) || !exists(filepath.Join(game2, "EULA.txt")) {
		t.Fatal("wrong files deleted")
	}
	if mark, _, _ := readMark(game2); mark.Installer != "" || strings.Join(mark.Keep, ",") != "EULA.txt" {
		t.Errorf("mark %+v", mark)
	}
	// moved to another disk, it is still uninstalled there: what was kept stays
	if err := m.Uninstall(game2); err != nil || exists(filepath.Join(game2, "game.exe")) || !exists(filepath.Join(game2, "EULA.txt")) {
		t.Fatalf("uninstall on a disk: %v", err)
	}
}

// A Windows installer tells what it has written, and seems stuck when nothing grows (6.8).
func TestWindowsWrittenAndStalled(t *testing.T) {
	m, home, _, ch := newManager(t)
	writtenEvery = 20 * time.Millisecond
	t.Cleanup(func() { writtenEvery = 5 * time.Second })
	m.StallAfter = 150 * time.Millisecond
	src := filepath.Join(home, "Giochi", "Big setup")
	mkGame(t, src, "setup_big.exe")
	runner := m.Runner.(*fakeRunner)
	release := make(chan struct{})
	runner.run = func(ctx context.Context, argv []string) error {
		last := argv[len(argv)-1]
		_, win, _ := strings.Cut(last, "=")
		dir := strings.ReplaceAll(strings.TrimPrefix(win, "Z:"), `\`, "/")
		if err := os.WriteFile(filepath.Join(dir, "data.pak"), make([]byte, 5000), 0o644); err != nil {
			return err
		}
		select { // then it writes nothing until released
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}
	m.Env = func() []string { return nil }
	run(t, m)
	dest := filepath.Join(home, "Giochi", "Big")
	if err := m.Install(Request{Installer: filepath.Join(src, "setup_big.exe"), Kind: KindInno, Dest: dest,
		Prefix: filepath.Join(home, ".local", "share", "owneet", "windows", "prefixes", "big-1")}); err != nil {
		t.Fatal(err)
	}
	if d := next(t, ch, "install.stalled").Data.(map[string]string); d["dest"] != dest {
		t.Fatalf("stalled %v", d)
	}
	st := m.Status()
	if len(st.Installs) != 1 || !st.Installs[0].Stalled || st.Installs[0].Done < 5000 {
		t.Fatalf("status %+v", st)
	}
	close(release)
	next(t, ch, "install.done")
}
