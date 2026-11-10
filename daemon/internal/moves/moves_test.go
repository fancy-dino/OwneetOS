// SPDX-License-Identifier: GPL-3.0-or-later

package moves

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// setup: a home with a games folder, and two disks: USB (with "Giochi" and a game) and HDD (empty)
func setup(t *testing.T) (*Manager, string, string) {
	t.Helper()
	root := t.TempDir()
	home, media := filepath.Join(root, "home"), filepath.Join(root, "media")
	game := filepath.Join(media, "USB", "Giochi", "Tyrian")
	for _, d := range []string{filepath.Join(home, "Games"), filepath.Join(game, "DOSBOX"), filepath.Join(media, "HDD")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(game, "TYRIAN.EXE"), []byte("game"), 0o755)
	os.WriteFile(filepath.Join(game, "DOSBOX", "dosbox.conf"), make([]byte, 3<<20), 0o644)
	os.Symlink("TYRIAN.EXE", filepath.Join(game, "start"))
	m := &Manager{Broker: events.NewBroker(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Home: home, Media: media,
		Free: func(string) (int64, error) { return 1 << 40, nil }}
	return m, game, media
}

func finish(t *testing.T, m *Manager) Status {
	t.Helper()
	for i := 0; i < 500; i++ {
		if st := m.Status(); st.State == "done" || st.State == "cancelled" {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("never finished: %+v", m.Status())
	return Status{}
}

func TestMoveRemovesTheOriginalAfterTheCheck(t *testing.T) {
	m, game, media := setup(t)
	if err := m.Start(Request{Games: []Game{{Dir: game, Title: "Tyrian"}}, To: filepath.Join(media, "HDD"), Remove: true}); err != nil {
		t.Fatal(err)
	}
	st := finish(t, m)
	moved := filepath.Join(media, "HDD", "Games", "Tyrian")
	if len(st.Moved) != 1 || st.Moved[0] != moved || len(st.Failed) != 0 {
		t.Fatalf("status %+v", st)
	}
	if _, err := os.Stat(game); !os.IsNotExist(err) {
		t.Error("the original is still there")
	}
	if info, err := os.Stat(filepath.Join(moved, "TYRIAN.EXE")); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("the game's program: %v %v", info, err)
	}
	if link, _ := os.Readlink(filepath.Join(moved, "start")); link != "TYRIAN.EXE" {
		t.Errorf("link %q", link)
	}
}

func TestCopyKeepsTheOriginalAndGoesToTheConsole(t *testing.T) {
	m, game, _ := setup(t)
	if err := m.Start(Request{Games: []Game{{Dir: game, Title: "Tyrian"}}, To: Internal}); err != nil {
		t.Fatal(err)
	}
	st := finish(t, m)
	if len(st.Moved) != 1 || st.Moved[0] != filepath.Join(m.Home, "Games", "Tyrian") {
		t.Fatalf("status %+v", st)
	}
	if _, err := os.Stat(filepath.Join(game, "TYRIAN.EXE")); err != nil {
		t.Error("the original was touched")
	}
	// copied again: a second folder, never over the first
	if err := m.Start(Request{Games: []Game{{Dir: game, Title: "Tyrian"}}, To: Internal}); err != nil {
		t.Fatal(err)
	}
	if st := finish(t, m); st.Moved[0] != filepath.Join(m.Home, "Games", "Tyrian (2)") {
		t.Errorf("second copy at %v", st.Moved)
	}
}

func TestStopRemovesThePartialCopy(t *testing.T) {
	m, game, media := setup(t)
	if err := m.Start(Request{Games: []Game{{Dir: game, Title: "Tyrian"}}, To: filepath.Join(media, "HDD"), Remove: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.Pause(); err != nil && !errors.Is(err, ErrIdle) {
		t.Fatal(err)
	}
	if m.Status().State == "copy" {
		if err := m.Stop(); err != nil {
			t.Fatal(err)
		}
		if st := finish(t, m); st.State != "cancelled" {
			t.Fatalf("status %+v", st)
		}
		if _, err := os.Stat(filepath.Join(media, "HDD", "Games", "Tyrian")); !os.IsNotExist(err) {
			t.Error("the partial copy is still there")
		}
	}
	if _, err := os.Stat(filepath.Join(game, "TYRIAN.EXE")); err != nil {
		t.Error("the original was touched")
	}
}

func TestCheckFindsADifferentCopy(t *testing.T) {
	_, game, media := setup(t)
	other := filepath.Join(media, "copy")
	m := &Manager{Broker: events.NewBroker(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := m.copyTree(game, other); err != nil {
		t.Fatal(err)
	}
	if err := same(game, other, nil); err != nil {
		t.Fatalf("a good copy: %v", err)
	}
	os.WriteFile(filepath.Join(other, "TYRIAN.EXE"), []byte("gamf"), 0o755)
	if same(game, other, nil) == nil {
		t.Error("a different file passed the check")
	}
	os.WriteFile(filepath.Join(other, "TYRIAN.EXE"), []byte("game"), 0o755)
	os.WriteFile(filepath.Join(other, "extra"), nil, 0o644)
	if same(game, other, nil) == nil {
		t.Error("a copy with another file passed the check")
	}
}

func TestStartRefuses(t *testing.T) {
	m, game, media := setup(t)
	for name, req := range map[string]Request{
		"no games":           {To: Internal},
		"not a games folder": {Games: []Game{{Dir: filepath.Join(media, "USB", "Giochi")}}, To: Internal},
		"outside the disks":  {Games: []Game{{Dir: "/etc"}}, To: Internal},
		"not a disk":         {Games: []Game{{Dir: game}}, To: "/tmp"},
		"onto its own disk":  {Games: []Game{{Dir: game}}, To: filepath.Join(media, "USB")},
	} {
		if err := m.Start(req); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	m.Free = func(string) (int64, error) { return 10, nil }
	if err := m.Start(Request{Games: []Game{{Dir: game}}, To: Internal}); !errors.Is(err, ErrNoSpace) {
		t.Errorf("no space: %v", err)
	}
}

// A plugged-in disk's games chosen game by game (approved demo 6.6c): only the originals asked
// for are removed, each after its own check; the others stay.
func TestRemoveGameByGame(t *testing.T) {
	m, game, media := setup(t)
	other := filepath.Join(media, "USB", "Giochi", "Cayne")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(other, "CAYNE.EXE"), []byte("other"), 0o755)
	seen := map[int]bool{} // by game
	ch, stop := m.Broker.Subscribe(256)
	defer stop()
	if err := m.Start(Request{Games: []Game{{Dir: game, Title: "Tyrian", Remove: true}, {Dir: other, Title: "Cayne"}}, To: Internal}); err != nil {
		t.Fatal(err)
	}
	st := finish(t, m)
	if len(st.Moved) != 2 || len(st.Failed) != 0 {
		t.Fatalf("status %+v", st)
	}
	if _, err := os.Stat(game); !os.IsNotExist(err) {
		t.Error("Tyrian's original is still there")
	}
	if _, err := os.Stat(filepath.Join(other, "CAYNE.EXE")); err != nil {
		t.Error("Cayne's original was touched")
	}
	// the status says each game's own choice while it is copied
	for len(ch) > 0 {
		if s, ok := (<-ch).Data.(Status); ok && s.State == "copy" {
			seen[s.Index] = s.Remove
		}
	}
	if len(seen) != 2 || !seen[0] || seen[1] {
		t.Errorf("remove per game in the status: %v", seen)
	}
}

// Copying several games, the player stops only one game's copy (approved demo 6.6d): its partial
// copy goes, its original stays, the next game is copied.
func TestSkipStopsOnlyThisGamesCopy(t *testing.T) {
	m, game, media := setup(t)
	other := filepath.Join(media, "USB", "Giochi", "Cayne")
	os.MkdirAll(other, 0o755)
	os.WriteFile(filepath.Join(other, "CAYNE.EXE"), []byte("other"), 0o755)
	// paused before the first byte (as Start, but waiting): the skip comes during the first copy
	m.running, m.paused = true, true
	m.resume, m.stop, m.skip = make(chan struct{}), make(chan struct{}), make(chan struct{})
	m.st = Status{Count: 2, Failed: []string{}, Skipped: []string{}, Moved: []string{}}
	dest, _ := m.gamesFolder(Internal)
	go m.run(Request{Games: []Game{{Dir: game, Title: "Tyrian", Remove: true}, {Dir: other, Title: "Cayne"}}, To: Internal}, dest)
	for i := 0; m.Status().State != "copy"; i++ {
		if i > 500 {
			t.Fatal("the copy never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := m.Skip(); err != nil {
		t.Fatal(err)
	}
	if err := m.Skip(); !errors.Is(err, ErrIdle) {
		t.Errorf("a second skip of the same game: %v", err)
	}
	st := finish(t, m)
	if len(st.Skipped) != 1 || st.Skipped[0] != "Tyrian" || len(st.Moved) != 1 || len(st.Failed) != 0 {
		t.Fatalf("status %+v", st)
	}
	if _, err := os.Stat(filepath.Join(m.Home, "Games", "Tyrian")); !os.IsNotExist(err) {
		t.Error("the stopped game's partial copy is still there")
	}
	if _, err := os.Stat(filepath.Join(game, "TYRIAN.EXE")); err != nil {
		t.Error("the stopped game's original was touched")
	}
	if _, err := os.Stat(filepath.Join(m.Home, "Games", "Cayne", "CAYNE.EXE")); err != nil {
		t.Error("the next game was not copied")
	}
}

// The check and the removal tell their own percentage, up to 100.
func TestCheckAndRemovalTellTheirProgress(t *testing.T) {
	m, game, media := setup(t)
	ch, stop := m.Broker.Subscribe(1024)
	defer stop()
	if err := m.Start(Request{Games: []Game{{Dir: game, Title: "Tyrian"}}, To: filepath.Join(media, "HDD"), Remove: true}); err != nil {
		t.Fatal(err)
	}
	finish(t, m)
	most := map[string]int{}
	for len(ch) > 0 {
		if s, ok := (<-ch).Data.(Status); ok {
			most[s.State] = max(most[s.State], s.Percent)
		}
	}
	if _, ok := most["check"]; !ok {
		t.Fatalf("no check in the status: %v", most)
	}
	if _, ok := most["remove"]; !ok {
		t.Fatalf("no removal in the status: %v", most)
	}
	// the last value of each step is sent at once only at the step's change: the percentages
	// themselves come at most a few times a second, so here the counters are checked directly
	moved, read := filepath.Join(media, "HDD", "Games", "Tyrian"), 0
	if err := same(moved, moved, func(n int64) error { read += int(n); return nil }); err != nil || int64(read) != 2*size(moved) || read == 0 {
		t.Errorf("the check read %d of %d: %v", read, 2*size(moved), err)
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755)
	os.WriteFile(filepath.Join(dir, "a", "b", "f"), nil, 0o644)
	os.Symlink("f", filepath.Join(dir, "a", "l"))
	total, removed := entries(dir), int64(0)
	if err := removeTree(dir, func(n int64) { removed += n }); err != nil || removed != total || total != 5 {
		t.Errorf("removed %d of %d: %v", removed, total, err)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Error("the folder is still there")
	}
}

// Shutting down during a copy: Abort stops it, waits for its end, the partial copy goes and the
// original stays.
func TestAbortStopsTheCopyAndWaits(t *testing.T) {
	m, game, _ := setup(t)
	if !m.Abort(time.Second) {
		t.Error("with no copy, Abort must not wait")
	}
	m.running, m.paused = true, true
	m.resume, m.stop, m.skip = make(chan struct{}), make(chan struct{}), make(chan struct{})
	m.done = make(chan struct{})
	m.st = Status{Count: 1, Failed: []string{}, Skipped: []string{}, Moved: []string{}}
	dest, _ := m.gamesFolder(Internal)
	go m.run(Request{Games: []Game{{Dir: game, Title: "Tyrian", Remove: true}}, To: Internal}, dest)
	for i := 0; m.Status().State != "copy"; i++ {
		if i > 500 {
			t.Fatal("the copy never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !m.Abort(5 * time.Second) {
		t.Fatal("Abort returned before the copy ended")
	}
	if st := m.Status(); st.State != "cancelled" {
		t.Errorf("status %+v", st)
	}
	if _, err := os.Stat(filepath.Join(m.Home, "Games", "Tyrian")); !os.IsNotExist(err) {
		t.Error("the partial copy is still there")
	}
	if _, err := os.Stat(filepath.Join(game, "TYRIAN.EXE")); err != nil {
		t.Error("the original was touched")
	}
}

// The check stops as soon as its reader says so, with that error.
func TestTheCheckCanBeStopped(t *testing.T) {
	_, game, _ := setup(t)
	read := 0
	err := same(game, game, func(n int64) error { read++; return errStopped })
	if !errors.Is(err, errStopped) || read != 1 {
		t.Errorf("the check went on: %v after %d reads", err, read)
	}
}

// syncDisk syncs the disk of a folder; a folder that is not there is an error.
func TestSyncDisk(t *testing.T) {
	if err := syncDisk(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := syncDisk(filepath.Join(t.TempDir(), "gone")); err == nil {
		t.Fatal("no error for a missing folder")
	}
}
