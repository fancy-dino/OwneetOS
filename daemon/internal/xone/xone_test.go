// SPDX-License-Identifier: GPL-3.0-or-later

package xone

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

type fakeUnits struct {
	started []string
	err     error
	onStart func()
}

func (u *fakeUnits) Start(unit string) error {
	u.started = append(u.started, unit)
	if u.onStart != nil {
		u.onStart()
	}
	return u.err
}

func newManager(t *testing.T, units *fakeUnits) *Manager {
	dir := t.TempDir()
	return &Manager{Broker: events.NewBroker(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Units: units,
		NeededFile: filepath.Join(dir, "needed"), StateFile: filepath.Join(dir, "state.json"),
		Poll: 5 * time.Millisecond, Timeout: 2 * time.Second}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStatus(t *testing.T) {
	m := newManager(t, &fakeUnits{})
	if st := m.Status(); st.Needed || st.State != "" {
		t.Errorf("nothing plugged in: %+v", st)
	}
	write(t, m.NeededFile, "02e6\n")
	write(t, m.StateFile, `{"state":"failed","error":"offline"}`)
	if st := m.Status(); !st.Needed || st.State != "failed" || st.Error != "offline" {
		t.Errorf("adapter without firmware, last try offline: %+v", st)
	}
}

func TestInstall(t *testing.T) {
	units := &fakeUnits{}
	m := newManager(t, units)
	write(t, m.NeededFile, "02e6\n")
	// the unit installs the firmware: the flag goes, the state says installed
	units.onStart = func() {
		go func() {
			time.Sleep(20 * time.Millisecond)
			os.Remove(m.NeededFile)
			write(t, m.StateFile, `{"state":"installed","error":""}`)
		}()
	}
	ch, cancel := m.Broker.Subscribe(8)
	defer cancel()
	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	if len(units.started) != 1 || units.started[0] != Unit {
		t.Errorf("started %v", units.started)
	}
	if err := m.Install(); !errors.Is(err, ErrBusy) {
		t.Errorf("second Install() = %v, want ErrBusy", err)
	}
	var last Status
	for i := 0; i < 2; i++ {
		select {
		case ev := <-ch:
			last = ev.Data.(Status)
		case <-time.After(time.Second):
			t.Fatal("no xone.changed")
		}
	}
	if last.Needed || last.State != "installed" {
		t.Errorf("after the installation: %+v", last)
	}
}

func TestInstallRefusedByPolkit(t *testing.T) {
	m := newManager(t, &fakeUnits{err: errors.New("access denied")})
	if err := m.Install(); err == nil {
		t.Fatal("Install() = nil, want the unit's error")
	}
	if st := m.Status(); st.State != "" {
		t.Errorf("after a refused start: %+v", st)
	}
}
