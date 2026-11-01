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
	if err := same(game, other); err != nil {
		t.Fatalf("a good copy: %v", err)
	}
	os.WriteFile(filepath.Join(other, "TYRIAN.EXE"), []byte("gamf"), 0o755)
	if same(game, other) == nil {
		t.Error("a different file passed the check")
	}
	os.WriteFile(filepath.Join(other, "TYRIAN.EXE"), []byte("game"), 0o755)
	os.WriteFile(filepath.Join(other, "extra"), nil, 0o644)
	if same(game, other) == nil {
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
