// SPDX-License-Identifier: GPL-3.0-or-later

package sysupdate

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
	onStart func(unit string)
}

func (u *fakeUnits) Start(unit string) error {
	u.started = append(u.started, unit)
	if u.onStart != nil {
		u.onStart(unit)
	}
	return u.err
}

func newManager(t *testing.T, units *fakeUnits) *Manager {
	dir := t.TempDir()
	m := &Manager{Broker: events.NewBroker(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Units: units,
		StateFile: filepath.Join(dir, "system-update.json"), ReleasesFile: filepath.Join(dir, "releases.json"),
		RollbackFile: filepath.Join(dir, "rollback.json"), VersionFile: filepath.Join(dir, "version"),
		InstalledMark: filepath.Join(dir, "installed"), SeenFile: filepath.Join(dir, "state", "seen.json"),
		Poll: 5 * time.Millisecond}
	write(t, m.VersionFile, "0.3.0\n")
	write(t, m.InstalledMark, "Installed with owneet-install\n")
	return m
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// waitFor reads "system-update.changed" events until one matches.
func waitFor(t *testing.T, ch <-chan events.Event, what string, ok func(Status) bool) Status {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev := <-ch:
			if st, is := ev.Data.(Status); is && ev.Type == "system-update.changed" && ok(st) {
				return st
			}
		case <-timeout:
			t.Fatalf("no event: %s", what)
		}
	}
}

func TestStatusAvailable(t *testing.T) {
	m := newManager(t, &fakeUnits{})
	if st := m.Status(); !st.Installed || st.State != "" || st.Current != "0.3.0" || st.Previous != nil {
		t.Errorf("not checked yet: %+v", st)
	}
	write(t, m.StateFile, `{"state":"available","current":"0.3.0","version":"0.4.0","date":"2026-11-01","size":432013312,`+
		`"packages":[{"name":"owneetd","from":"0.22.0","to":"0.23.0"}],"percent":0,"error":"","time":"2026-11-01T10:42:00Z"}`)
	write(t, m.ReleasesFile, `{"releases": [
{"version": "0.4.0", "date": "2026-11-01", "archive": "2026/10/28", "notes": {"en": "Faster.", "it": "Più veloce."}},
{"version": "0.3.0", "date": "2026-10-01", "archive": "2026/09/28", "notes": {"en": "Old."}}
]}`)
	write(t, m.RollbackFile, `{"spare":true,"spare_date":"2026-10-12T08:00:00Z","spare_version":"0.2.1","previous":false,"good":true,"fell_back":false,"rolled_back":""}`)
	st := m.Status()
	if st.State != "available" || st.Version != "0.4.0" || st.Size != 432013312 || len(st.Packages) != 1 ||
		st.Notes["it"] != "Più veloce." || st.Checked != "2026-11-01T10:42:00Z" {
		t.Errorf("available: %+v", st)
	}
	if st.Previous == nil || st.Previous.Version != "0.2.1" || st.Previous.Since != "2026-10-12T08:00:00Z" {
		t.Errorf("spare copy: %+v", st.Previous)
	}
}

func TestCheckedKeptAcrossStartUps(t *testing.T) {
	m := newManager(t, &fakeUnits{})
	write(t, m.StateFile, `{"state":"none","current":"0.3.0","time":"2026-10-30T09:00:00Z"}`)
	m.publish()
	// a new start-up: /run is empty
	os.Remove(m.StateFile)
	if st := m.Status(); st.State != "" || st.Checked != "2026-10-30T09:00:00Z" {
		t.Errorf("after a restart: %+v", st)
	}
}

func TestCheck(t *testing.T) {
	units := &fakeUnits{}
	m := newManager(t, units)
	units.onStart = func(string) {
		go func() {
			time.Sleep(20 * time.Millisecond)
			write(t, m.StateFile, `{"state":"none","current":"0.3.0","time":"2026-11-01T11:00:00Z"}`)
		}()
	}
	ch, cancel := m.Broker.Subscribe(16)
	defer cancel()
	if err := m.Check(); err != nil {
		t.Fatal(err)
	}
	if len(units.started) != 1 || units.started[0] != CheckUnit {
		t.Errorf("started %v", units.started)
	}
	if err := m.Check(); !errors.Is(err, ErrBusy) {
		t.Errorf("second Check() = %v, want ErrBusy", err)
	}
	waitFor(t, ch, "checking", func(st Status) bool { return st.Checking })
	st := waitFor(t, ch, "checked", func(st Status) bool { return !st.Checking })
	if st.State != "none" || st.Checked != "2026-11-01T11:00:00Z" {
		t.Errorf("after the check: %+v", st)
	}
}

func TestRefusals(t *testing.T) {
	units := &fakeUnits{}
	m := newManager(t, units)
	if err := m.Install(); !errors.Is(err, ErrNothing) {
		t.Errorf("Install() with nothing found = %v", err)
	}
	if err := m.GoBack(); !errors.Is(err, ErrNoPrevious) {
		t.Errorf("GoBack() without a spare copy = %v", err)
	}
	write(t, m.StateFile, `{"state":"downloading","version":"0.4.0","percent":40}`)
	if err := m.Install(); !errors.Is(err, ErrBusy) {
		t.Errorf("Install() while downloading = %v", err)
	}
	write(t, m.StateFile, `{"state":"available","version":"0.4.0"}`)
	write(t, m.RollbackFile, `{"spare":true,"spare_version":"0.2.1","previous":true}`)
	if err := m.Install(); !errors.Is(err, ErrOnPrevious) {
		t.Errorf("Install() on the previous version = %v", err)
	}
	os.Remove(m.InstalledMark)
	if err := m.Check(); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("Check() on a tester's PC = %v", err)
	}
	if len(units.started) != 0 {
		t.Errorf("units started: %v", units.started)
	}
}

func TestInstall(t *testing.T) {
	units := &fakeUnits{}
	m := newManager(t, units)
	write(t, m.StateFile, `{"state":"available","version":"0.4.0","size":100}`)
	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	if len(units.started) != 1 || units.started[0] != ApplyUnit {
		t.Errorf("started %v", units.started)
	}
}

func TestGoBack(t *testing.T) {
	for _, tc := range []struct {
		name, after, want string
	}{
		{"done", `{"spare":false,"previous":false}`, "done"},
		{"failed", `{"spare":true,"spare_version":"0.2.1","previous":false}`, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			units := &fakeUnits{}
			m := newManager(t, units)
			write(t, m.RollbackFile, `{"spare":true,"spare_date":"2026-10-12T08:00:00Z","spare_version":"0.2.1","previous":false}`)
			time.Sleep(10 * time.Millisecond)
			units.onStart = func(string) {
				go func() {
					time.Sleep(20 * time.Millisecond)
					write(t, m.RollbackFile, tc.after)
				}()
			}
			ch, cancel := m.Broker.Subscribe(16)
			defer cancel()
			if err := m.GoBack(); err != nil {
				t.Fatal(err)
			}
			if len(units.started) != 1 || units.started[0] != BackUnit {
				t.Errorf("started %v", units.started)
			}
			waitFor(t, ch, tc.want, func(st Status) bool { return st.Back == tc.want })
		})
	}
}

func TestRolledBackNotice(t *testing.T) {
	m := newManager(t, &fakeUnits{})
	write(t, m.RollbackFile, `{"spare":false,"previous":false,"rolled_back":"2026-11-01T08:30:00Z"}`)
	if st := m.Status(); st.RolledBack != "2026-11-01T08:30:00Z" {
		t.Errorf("went back by itself: %+v", st)
	}
	m.NoticeSeen()
	if st := m.Status(); st.RolledBack != "" {
		t.Errorf("notice seen, still shown: %+v", st)
	}
	// a later fall back is shown again
	write(t, m.RollbackFile, `{"spare":false,"previous":false,"rolled_back":"2026-11-05T08:30:00Z"}`)
	if st := m.Status(); st.RolledBack != "2026-11-05T08:30:00Z" {
		t.Errorf("second fall back: %+v", st)
	}
}
