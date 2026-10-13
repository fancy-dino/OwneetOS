// SPDX-License-Identifier: GPL-3.0-or-later

package update

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

// fakeUnits writes what owneet-update would, a moment after the unit is started.
type fakeUnits struct {
	state   string
	results []string
	started []string
}

func (f *fakeUnits) Start(unit string) error {
	f.started = append(f.started, unit)
	go func() {
		for _, r := range f.results {
			time.Sleep(30 * time.Millisecond)
			_ = os.WriteFile(f.state, []byte(r), 0o644)
		}
	}()
	return nil
}

func newManager(t *testing.T, tester bool, results ...string) (*Manager, *fakeUnits, <-chan events.Event) {
	t.Helper()
	dir := t.TempDir()
	mark := filepath.Join(dir, "tester")
	if tester {
		if err := os.WriteFile(mark, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	units := &fakeUnits{state: filepath.Join(dir, "update.json"), results: results}
	broker := events.NewBroker()
	ch, unsub := broker.Subscribe(16)
	t.Cleanup(unsub)
	m := &Manager{Broker: broker, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Units: units,
		StateFile: units.state, TesterMark: mark, Poll: 10 * time.Millisecond, Timeout: 5 * time.Second}
	return m, units, ch
}

func next(t *testing.T, ch <-chan events.Event) Status {
	t.Helper()
	select {
	case e := <-ch:
		return e.Data.(Status)
	case <-time.After(3 * time.Second):
		t.Fatal("no update.changed event")
	}
	return Status{}
}

func TestOnlyTesters(t *testing.T) {
	m, units, _ := newManager(t, false)
	if err := m.Check(); !errors.Is(err, ErrNotTester) {
		t.Fatalf("check without the tester mark: %v", err)
	}
	if len(units.started) != 0 || m.Status().Tester {
		t.Fatal("a unit was started on a user system")
	}
}

func TestCheckThenApply(t *testing.T) {
	m, units, ch := newManager(t, true,
		`{"state":"available","message":"2 newer","packages":[{"name":"owneetd","from":"0.11.6-1","to":"0.11.7-1"},{"name":"owneet-frontend","from":"0.16.3-1","to":"0.16.4-1"}]}`)
	if err := m.Check(); err != nil {
		t.Fatal(err)
	}
	if units.started[0] != "owneet-update@check.service" {
		t.Fatalf("started %v", units.started)
	}
	st := next(t, ch)
	if st.State != "available" || len(st.Packages) != 2 || st.Packages[0].To != "0.11.7-1" || st.Running {
		t.Fatalf("after the check %+v", st)
	}
	units.results = []string{`{"state":"installing","packages":[]}`, `{"state":"done","message":"Restart","packages":[]}`}
	if err := m.Apply(); err != nil {
		t.Fatal(err)
	}
	if err := m.Apply(); !errors.Is(err, ErrBusy) {
		t.Fatalf("second apply while running: %v", err)
	}
	if st := next(t, ch); st.State != "installing" || !st.Running {
		t.Fatalf("installing %+v", st)
	}
	if st := next(t, ch); st.State != "done" || st.Running {
		t.Fatalf("done %+v", st)
	}
}
