// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// fakeUnits records calls; every started unit gets a window whose pid is 1000 + start order.
type fakeUnits struct {
	mu       sync.Mutex
	started  []string
	stopped  []string
	killed   []string
	exits    chan Exit
	failAt   string
	workdirs []string
}

func (f *fakeUnits) Start(unit, desc string, argv, env []string, workdir string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if unit == f.failAt {
		return errors.New("exec failed")
	}
	f.started = append(f.started, unit)
	f.workdirs = append(f.workdirs, workdir)
	return nil
}
func (f *fakeUnits) Stop(unit string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, unit)
	return nil
}
func (f *fakeUnits) Kill(unit string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.killed = append(f.killed, unit)
	return nil
}
func (f *fakeUnits) Running() (map[string]string, error) {
	return map[string]string{UnitName("steam/42"): "OwneetOS game: Old Game"}, nil
}
func (f *fakeUnits) Exited() <-chan Exit { return f.exits }

// fakeFocus is gamescope --steam: it shows the first app of the last Show list that has a window.
type fakeFocus struct {
	mu      sync.Mutex
	tag     func(pid int) uint32
	visible []uint32
	shown   []uint32
	focused uint32
}

func (f *fakeFocus) Watch(tag func(pid int) uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tag = tag
}
func (f *fakeFocus) recompute() {
	f.focused = 0
	for _, id := range f.shown {
		for _, v := range f.visible {
			if v == id && f.focused == 0 {
				f.focused = id
			}
		}
	}
}
func (f *fakeFocus) Show(ids []uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shown = append([]uint32(nil), ids...)
	f.recompute()
	return nil
}
func (f *fakeFocus) Focused() (uint32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.focused, nil
}
func (f *fakeFocus) Visible() ([]uint32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uint32(nil), f.visible...), nil
}

// openWindow simulates a window of process pid appearing: owneetd tags it, gamescope lists it.
func (f *fakeFocus) openWindow(pid int) {
	f.mu.Lock()
	tag := f.tag
	f.mu.Unlock()
	id := tag(pid)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.visible = append(f.visible, id)
	f.recompute()
}

func newManager(t *testing.T, mode string) (*Manager, *fakeUnits, *fakeFocus, <-chan events.Event) {
	t.Helper()
	units := &fakeUnits{exits: make(chan Exit, 4)}
	focus := &fakeFocus{}
	broker := events.NewBroker()
	ch, unsub := broker.Subscribe(64)
	t.Cleanup(unsub)
	// pids 1001, 1002… belong to the units in start order.
	unitOf := func(pid int) string {
		units.mu.Lock()
		defer units.mu.Unlock()
		if i := pid - 1001; i >= 0 && i < len(units.started) {
			return units.started[i]
		}
		return ""
	}
	m := &Manager{
		Units: units, Broker: broker, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Session: func() (Session, error) {
			return Session{Mode: mode, Env: []string{"DISPLAY=:1"}, Display: ":1", HomePID: 10}, nil
		},
		FocusFor: func(Session) Focus { return focus },
		UnitOf:   unitOf,
		LookPath: func(p string) (string, error) {
			if strings.Contains(p, "missing") {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + strings.TrimPrefix(p, "/usr/bin/"), nil
		},
		Hold: 300 * time.Millisecond,
	}
	return m, units, focus, ch
}

func TestUnitName(t *testing.T) {
	cases := map[string]string{
		"steam:1245620": `owneet-app-steam:1245620.service`,
		"local/My Game": `owneet-app-local-My\x20Game.service`,
		".hidden":       `owneet-app-\x2ehidden.service`,
	}
	for id, want := range cases {
		if got := UnitName(id); got != want {
			t.Errorf("UnitName(%q) = %s, want %s", id, got, want)
		}
		back := unescapeUnit(strings.TrimSuffix(strings.TrimPrefix(UnitName(id), "owneet-app-"), ".service"))
		if back != id {
			t.Errorf("round trip %q -> %q", id, back)
		}
	}
	if got := unescapeBusPath("owneet_2dapp_2dsteam_3a1_2eservice"); got != "owneet-app-steam:1.service" {
		t.Errorf("unescapeBusPath = %s", got)
	}
}

func TestAppIDsAreStableAndUnique(t *testing.T) {
	m, _, _, _ := newManager(t, "gamescope")
	m.mu.Lock()
	m.init()
	first := m.newAppID("steam:42")
	m.apps["a"] = &App{ID: "a", appID: first}
	second := m.newAppID("steam:42") // same id while the first is running: must differ
	m.mu.Unlock()
	if first == second || first <= HomeAppID || second <= HomeAppID {
		t.Fatalf("app ids %d and %d", first, second)
	}
	m2, _, _, _ := newManager(t, "gamescope")
	m2.mu.Lock()
	defer m2.mu.Unlock()
	m2.init()
	if again := m2.newAppID("steam:42"); again != first {
		t.Fatalf("app id after a restart: %d, want %d", again, first)
	}
}

func TestLaunchRules(t *testing.T) {
	m, units, _, _ := newManager(t, "gamescope")
	bad := []LaunchRequest{
		{ID: "", Command: []string{"x"}},
		{ID: "a", Command: nil},
		{ID: "a", Kind: "toy", Command: []string{"x"}},
		{ID: strings.Repeat("x", 101), Command: []string{"x"}},
		{ID: "a", Command: []string{"x"}, Workdir: "relative/dir"},
		{ID: "a", Command: []string{"x"}, Workdir: "/no/such/folder"},
	}
	for _, r := range bad {
		if _, err := m.Launch(r); !errors.Is(err, ErrInvalid) {
			t.Errorf("Launch(%+v) = %v, want ErrInvalid", r, err)
		}
	}
	if _, err := m.Launch(LaunchRequest{ID: "a", Command: []string{"missing-program"}}); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing program: %v", err)
	}
	dir := t.TempDir()
	app, err := m.Launch(LaunchRequest{ID: "game:1", Name: "One", Kind: KindGame, Command: []string{"one"}, Workdir: dir})
	if err != nil || app.Name != "One" || len(units.started) != 1 || units.workdirs[0] != dir {
		t.Fatalf("launch: %+v %v (workdirs %v)", app, err, units.workdirs)
	}
	if _, err := m.Launch(LaunchRequest{ID: "game:1", Command: []string{"one"}}); !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("same id: %v", err)
	}
	if _, err := m.Launch(LaunchRequest{ID: "game:2", Kind: KindGame, Command: []string{"two"}}); !errors.Is(err, ErrGameRunning) {
		t.Errorf("second game: %v", err)
	}
	if _, err := m.Launch(LaunchRequest{ID: "browser", Command: []string{"brave"}}); err != nil {
		t.Errorf("an app next to a game: %v", err)
	}
	units.failAt = UnitName("broken")
	if _, err := m.Launch(LaunchRequest{ID: "broken", Command: []string{"broken"}}); err == nil {
		t.Error("a failed start must fail the launch")
	}
	if st := m.Status(); len(st.Apps) != 2 {
		t.Errorf("apps = %+v (the failed one must not stay listed)", st.Apps)
	}
}

func TestExitAndClose(t *testing.T) {
	m, units, _, ch := newManager(t, "gamescope")
	stop := make(chan struct{})
	defer close(stop)
	go m.Run(stop)
	waitApps(t, m, 1) // the adopted app still running from before
	if st := m.Status(); st.Apps[0].ID != "steam/42" || st.Apps[0].Kind != KindGame || st.Apps[0].Name != "Old Game" {
		t.Fatalf("adopted = %+v", st.Apps[0])
	}
	if err := m.Close("steam/42"); err != nil || len(units.stopped) != 1 {
		t.Fatalf("close: %v %v", err, units.stopped)
	}
	if err := m.Kill("nope"); !errors.Is(err, ErrUnknownApp) {
		t.Fatalf("kill unknown: %v", err)
	}
	units.exits <- Exit{Unit: UnitName("steam/42"), Result: "signal"}
	waitApps(t, m, 0)
	for ev := range ch {
		if ev.Type == "app.exited" {
			if d := ev.Data.(map[string]string); d["id"] != "steam/42" || d["result"] != "signal" {
				t.Fatalf("event = %+v", d)
			}
			break
		}
	}
}

func waitApps(t *testing.T, m *Manager, n int) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if len(m.Status().Apps) == n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("apps = %+v, want %d", m.Status().Apps, n)
}

func TestGuideTogglesHomeAndGame(t *testing.T) {
	m, _, focus, _ := newManager(t, "gamescope")
	m.watchSession() // tags windows: the home screen is process 10
	focus.openWindow(10)
	focus.Show([]uint32{HomeAppID})
	if st := m.Status(); st.Focus != "home" {
		t.Fatalf("focus = %q, want home", st.Focus)
	}
	m.Guide(true) // no app: nothing happens
	if st := m.Status(); st.Focus != "home" {
		t.Fatal("Guide without apps changed the screen")
	}
	if _, err := m.Launch(LaunchRequest{ID: "game", Kind: KindGame, Command: []string{"game"}}); err != nil {
		t.Fatal(err)
	}
	if st := m.Status(); st.Focus != "home" {
		t.Fatalf("before its window: focus %q, want home", st.Focus)
	}
	focus.openWindow(1001) // the game's window
	if st := m.Status(); st.Focus != "game" {
		t.Fatalf("focus = %q, want game", st.Focus)
	}
	if err := m.FocusApp("nope"); !errors.Is(err, ErrUnknownApp) {
		t.Fatalf("FocusApp(unknown) = %v", err)
	}
	m.Guide(true) // game -> home
	m.Guide(false)
	if st := m.Status(); st.Focus != "home" {
		t.Fatalf("after Guide: focus %q", st.Focus)
	}
	m.Guide(true) // home -> game
	if st := m.Status(); st.Focus != "game" {
		t.Fatalf("after second Guide: focus %q", st.Focus)
	}
	m.Guide(true) // home again, then "Resume" from the home screen
	if err := m.FocusApp("game"); err != nil || m.Status().Focus != "game" {
		t.Fatalf("FocusApp: %v", err)
	}
	if got := m.tag(4242); got != 0 {
		t.Fatalf("a process outside the apps got app id %d", got)
	}
}

func TestCageHoldGuideToClose(t *testing.T) {
	m, units, _, _ := newManager(t, "cage")
	if _, err := m.Launch(LaunchRequest{ID: "game", Kind: KindGame, Command: []string{"game"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.FocusApp("game"); !errors.Is(err, ErrFocusUnsupported) {
		t.Fatalf("FocusApp in cage: %v", err)
	}
	m.Guide(true) // short press: released at once, well before Hold
	m.Guide(false)
	time.Sleep(600 * time.Millisecond)
	units.mu.Lock()
	n := len(units.stopped)
	units.mu.Unlock()
	if n != 0 {
		t.Fatal("a short press closed the game")
	}
	m.Guide(true) // held
	deadline := time.Now().Add(5 * time.Second)
	for {
		units.mu.Lock()
		n := len(units.stopped)
		units.mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	m.Guide(false)
	units.mu.Lock()
	defer units.mu.Unlock()
	if len(units.stopped) != 1 || units.stopped[0] != UnitName("game") {
		t.Fatalf("stopped = %v", units.stopped)
	}
}
