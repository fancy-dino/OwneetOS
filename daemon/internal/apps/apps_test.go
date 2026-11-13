// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"errors"
	"io"
	"log/slog"
	"os/exec"
	"runtime"
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
	argvs    [][]string
	stops    [][]string
	whileAny []bool
}

func (f *fakeUnits) Start(unit, desc string, argv, stop, env []string, workdir string, whileAny bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.whileAny = append(f.whileAny, whileAny)
	if unit == f.failAt {
		return errors.New("exec failed")
	}
	f.started = append(f.started, unit)
	f.workdirs = append(f.workdirs, workdir)
	f.argvs = append(f.argvs, argv)
	f.stops = append(f.stops, stop)
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
	request func([]uint32)
	changed func()
	visible []uint32
	shown   []uint32
	focused uint32
}

func (f *fakeFocus) Watch(tag func(pid int) uint32, request func([]uint32), changed func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tag = tag
	f.request = request
	f.changed = changed
}

// closeWindow simulates the app's window going away (the process may still run): gamescope shows
// what is left and announces it
func (f *fakeFocus) closeWindow(id uint32) {
	f.mu.Lock()
	var left []uint32
	for _, v := range f.visible {
		if v != id {
			left = append(left, v)
		}
	}
	f.visible = left
	f.recompute()
	changed := f.changed
	f.mu.Unlock()
	if changed != nil {
		changed()
	}
}

// recompute: the first app asked for that has a window; with none, gamescope keeps the window it
// showed while that window is there (with it gone, its last picture: nothing focused)
func (f *fakeFocus) recompute() {
	before := f.focused
	f.focused = 0
	for _, id := range f.shown {
		for _, v := range f.visible {
			if v == id && f.focused == 0 {
				f.focused = id
			}
		}
	}
	for _, v := range f.visible {
		if f.focused == 0 && v == before {
			f.focused = before
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
		Hold:      300 * time.Millisecond,
		watchExit: func(int, func()) bool { return true }, // the test processes never end by themselves
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
	// a web app is treated like a game: one at a time ([[step-5.1-brave-in-the-os]])
	if _, err := m.Launch(LaunchRequest{ID: "web:netflix", Kind: KindWebApp, Command: []string{"env"}}); !errors.Is(err, ErrGameRunning) {
		t.Errorf("web app next to a game: %v", err)
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

func TestCloseAllWaitsForTheApps(t *testing.T) {
	m, units, _, _ := newManager(t, "gamescope")
	stop := make(chan struct{})
	defer close(stop)
	go m.Run(stop)
	waitApps(t, m, 1) // the adopted app still running from before
	go func() {
		time.Sleep(200 * time.Millisecond) // the app takes a while to close
		units.exits <- Exit{Unit: UnitName("steam/42"), Result: "success"}
	}()
	begin := time.Now()
	m.CloseAll(5 * time.Second)
	if len(m.Status().Apps) != 0 || len(units.stopped) != 1 || time.Since(begin) > 2*time.Second {
		t.Fatalf("apps %v, stopped %v after %v", m.Status().Apps, units.stopped, time.Since(begin))
	}
	// an app that does not close: CloseAll gives up after its wait
	if _, err := m.Launch(LaunchRequest{ID: "app", Name: "App", Kind: KindApp, Command: []string{"/bin/true"}}); err != nil {
		t.Fatal(err)
	}
	begin = time.Now()
	m.CloseAll(300 * time.Millisecond)
	if d := time.Since(begin); d < 300*time.Millisecond || d > 2*time.Second {
		t.Fatalf("waited %v", d)
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
	ch, stop := m.Broker.Subscribe(16)
	m.Guide(true) // no app: the screen stays; the interface is told Home was asked (6.6e)
	stop()
	if st := m.Status(); st.Focus != "home" {
		t.Fatal("Guide without apps changed the screen")
	}
	asked := false
	for len(ch) > 0 {
		if ev := <-ch; ev.Type == "home.requested" {
			asked = true
		}
	}
	if !asked {
		t.Error("Guide on the home screen with nothing running did not send home.requested")
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

// Brave moves its main process (the windows' owner) into a scope of its own; its helpers stay in
// the web app's unit as that process's children (test VM, 2026-10-15).
func TestWebAppWindowOutsideItsUnit(t *testing.T) {
	m, _, focus, _ := newManager(t, "gamescope")
	procs := &fakeProcs{unit: map[int]string{}, cmd: map[int][]string{}, parent: map[int]int{}}
	m.Procs = procs
	m.watchSession()
	focus.openWindow(10)
	focus.Show([]uint32{HomeAppID})
	if _, err := m.Launch(LaunchRequest{ID: "web:netflix", Kind: KindWebApp, Command: []string{"env"}}); err != nil {
		t.Fatal(err)
	}
	// 767: the browser, in no app unit; 779: its helper, in the web app's unit
	procs.add(779, 767, UnitName("web:netflix"), "brave", "--type=zygote")
	focus.openWindow(767)
	if st := m.Status(); st.Focus != "web:netflix" {
		t.Fatalf("focus = %q, want web:netflix", st.Focus)
	}
	m.Guide(true) // web app -> home
	m.Guide(false)
	if st := m.Status(); st.Focus != "home" {
		t.Fatalf("after Guide: focus %q, want home", st.Focus)
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
	// cage cannot switch windows: owneetd says what is on screen and the interface carries it out
	if err := m.ShowHome(); err != nil || m.Status().Focus != "home" {
		t.Fatalf("ShowHome in cage: %v, focus %q", err, m.Status().Focus)
	}
	if err := m.FocusApp("game"); err != nil || m.Status().Focus != "game" {
		t.Fatalf("FocusApp in cage: %v, focus %q", err, m.Status().Focus)
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

// A game's window goes away while its process still ends (Proton): the home screen is announced as
// soon as gamescope says so, not at the next regular check (the late sound, owner's test 2026-11-08)
// An app's window goes while the app runs on: the home screen behind it is announced at once.
func TestHomeAnnouncedWhenTheWindowGoes(t *testing.T) {
	m, _, focus, ch := newManager(t, "gamescope")
	m.checkEvery = time.Hour // only gamescope's notice can do it
	m.watchSession()
	focus.openWindow(10) // the home screen
	if _, err := m.Launch(LaunchRequest{ID: "app", Name: "App", Kind: KindApp, Command: []string{"/bin/app"}}); err != nil {
		t.Fatal(err)
	}
	focus.openWindow(1001)
	m.checkWindows()
	if st := m.Status(); st.Focus != "app" {
		t.Fatalf("app not on screen: %q", st.Focus)
	}
	stop := make(chan struct{})
	defer close(stop)
	go m.Run(stop)
	for len(ch) > 0 {
		<-ch
	}
	focus.closeWindow(m.appIDOf("app"))
	timeout := time.After(2 * time.Second)
	for {
		select {
		case e := <-ch:
			if e.Type == "focus.changed" && e.Data.(map[string]string)["focus"] == "home" {
				return
			}
		case <-timeout:
			t.Fatal("home not announced when gamescope said the window went")
		}
	}
}

// A game started by the console goes alone in front: between two of its windows (its launcher
// closes, its game window opens) gamescope keeps its last picture and the home screen is not
// announced (The Witcher 2 on FrankenPC, 2026-11-09); Guide, Resume and its end work as before.
func TestGameKeepsTheScreenBetweenItsWindows(t *testing.T) {
	m, _, focus, ch := newManager(t, "gamescope")
	m.watchSession()
	focus.openWindow(10) // the home screen
	focus.Show([]uint32{HomeAppID})
	if _, err := m.Launch(LaunchRequest{ID: "game", Name: "Game", Kind: KindGame, Command: []string{"/bin/game"}}); err != nil {
		t.Fatal(err)
	}
	game := m.appIDOf("game")
	if !sameIDs(focus.shown, []uint32{game}) {
		t.Fatalf("shown at launch: %v", focus.shown)
	}
	m.checkWindows()
	if st := m.Status(); st.Focus != "home" { // no window yet: the home screen stays
		t.Fatalf("before its window: %q", st.Focus)
	}
	focus.openWindow(1001) // the launcher
	m.checkWindows()
	if st := m.Status(); st.Focus != "game" {
		t.Fatalf("launcher not on screen: %q", st.Focus)
	}
	for len(ch) > 0 {
		<-ch
	}
	focus.closeWindow(game) // the launcher closes; the game's window is not there yet
	m.checkWindows()
	focus.openWindow(1001) // the game's window
	m.checkWindows()
	for len(ch) > 0 {
		if e := <-ch; e.Type == "focus.changed" && e.Data.(map[string]string)["focus"] == "home" {
			t.Fatal("home announced between two windows of the game")
		}
	}
	if st := m.Status(); st.Focus != "game" {
		t.Fatalf("game window not on screen: %q", st.Focus)
	}
	// Guide, then Resume: the game goes alone in front again
	if err := m.ShowHome(); err != nil || m.Status().Focus != "home" {
		t.Fatalf("home: %v %q", err, m.Status().Focus)
	}
	if err := m.FocusApp("game"); err != nil || !sameIDs(focus.shown, []uint32{game}) || m.Status().Focus != "game" {
		t.Fatalf("resume: %v %v %q", err, focus.shown, m.Status().Focus)
	}
	// the game closes its window, then its process ends: the home screen
	focus.closeWindow(game)
	m.checkWindows()
	m.exited(Exit{Unit: UnitName("game"), Result: "success"})
	if !sameIDs(focus.shown, []uint32{HomeAppID}) || m.Status().Focus != "home" {
		t.Fatalf("after its end: %v %q", focus.shown, m.Status().Focus)
	}
}

// An app (not a game) keeps the home screen behind it.
func TestAppKeepsTheHomeScreenBehind(t *testing.T) {
	m, _, focus, _ := newManager(t, "gamescope")
	m.watchSession()
	focus.openWindow(10)
	if _, err := m.Launch(LaunchRequest{ID: "app", Name: "App", Kind: KindApp, Command: []string{"/bin/app"}}); err != nil {
		t.Fatal(err)
	}
	if !sameIDs(focus.shown, []uint32{m.appIDOf("app"), HomeAppID}) {
		t.Fatalf("shown: %v", focus.shown)
	}
}

// Back to a game (Resume, Guide) only once the button that chose it is up: the game must not take
// it (The Witcher 2 took the A of "Resume", FrankenPC, 2026-11-09). Guide over a game between two
// of its windows shows the home screen.
func TestBackToTheGameWhenTheButtonIsUp(t *testing.T) {
	m, _, focus, _ := newManager(t, "gamescope")
	m.watchSession()
	focus.openWindow(10)
	focus.Show([]uint32{HomeAppID})
	var mu sync.Mutex
	up := make(chan struct{})
	close(up)
	m.SetReleased(func() <-chan struct{} {
		mu.Lock()
		defer mu.Unlock()
		return up
	})
	press := func() { // a button goes down
		mu.Lock()
		up = make(chan struct{})
		mu.Unlock()
	}
	release := func() {
		mu.Lock()
		close(up)
		mu.Unlock()
	}
	if _, err := m.Launch(LaunchRequest{ID: "game", Name: "Game", Kind: KindGame, Command: []string{"/bin/game"}}); err != nil {
		t.Fatal(err)
	}
	focus.openWindow(1001)
	game := m.appIDOf("game")
	// between two of its windows: Guide still shows the home screen
	focus.closeWindow(game)
	m.Guide(true)
	m.Guide(false)
	if st := m.Status(); st.Focus != "home" || !sameIDs(focus.shown, []uint32{HomeAppID}) {
		t.Fatalf("Guide over the game without a window: %q %v", st.Focus, focus.shown)
	}
	focus.openWindow(1001)
	// Resume with A still held
	press()
	done := make(chan error, 1)
	go func() { done <- m.FocusApp("game") }()
	select {
	case <-done:
		t.Fatal("back to the game while A is held")
	default:
	}
	release()
	if err := <-done; err != nil || m.Status().Focus != "game" {
		t.Fatalf("after A came up: %v %q", err, m.Status().Focus)
	}
	// Guide to the home screen, Guide back: the game comes when Guide is up
	m.Guide(true)
	m.Guide(false)
	press()
	m.Guide(true)
	if m.Status().Focus != "home" {
		t.Fatal("back to the game while Guide is held")
	}
	release()
	m.Guide(false)
	deadline := time.Now().Add(2 * time.Second)
	for m.Status().Focus != "game" {
		if time.Now().After(deadline) {
			t.Fatal("not back to the game after Guide came up")
		}
		runtime.Gosched()
	}
}

// A game, or a video app, behind the home screen goes silent; on screen it is heard; music apps
// play on; the interface's own sounds and other processes are never silenced (owner, 2026-11-09).
func TestSilentBehindTheHomeScreen(t *testing.T) {
	m, _, focus, _ := newManager(t, "gamescope")
	m.watchSession()
	focus.openWindow(10)
	focus.Show([]uint32{HomeAppID})
	m.SetSilent(func(id string) bool { return id == "web:netflix" })
	// processes 1001, 1002, 1003 belong to the units in start order
	for i, c := range []struct {
		req    LaunchRequest
		behind bool // silent behind the home screen
	}{
		{LaunchRequest{ID: "game", Name: "Game", Kind: KindGame, Command: []string{"/bin/game"}}, true},
		{LaunchRequest{ID: "web:netflix", Name: "netflix", Kind: KindWebApp, Command: []string{"/bin/brave"}}, true},
		{LaunchRequest{ID: "web:spotify", Name: "spotify", Kind: KindWebApp, Command: []string{"/bin/brave"}}, false},
	} {
		pid := 1001 + i
		if _, err := m.Launch(c.req); err != nil {
			t.Fatal(c.req.ID, err)
		}
		focus.openWindow(pid)
		m.checkWindows()
		if m.Status().Focus != c.req.ID || m.SilentNow(pid) {
			t.Fatalf("%s on screen: focus %q, silent %v", c.req.ID, m.Status().Focus, m.SilentNow(pid))
		}
		if err := m.ShowHome(); err != nil {
			t.Fatal(err)
		}
		if m.SilentNow(pid) != c.behind || m.SilentNow(10) || m.SilentNow(4242) {
			t.Fatalf("%s behind the home screen: silent %v, want %v", c.req.ID, m.SilentNow(pid), c.behind)
		}
		if err := m.FocusApp(c.req.ID); err != nil || m.SilentNow(pid) {
			t.Fatalf("%s back on screen: %v, silent %v", c.req.ID, err, m.SilentNow(pid))
		}
		focus.closeWindow(m.appIDOf(c.req.ID))
		m.exited(Exit{Unit: UnitName(c.req.ID), Result: "success"})
	}
}

// A console game quit from its menu: the home screen comes back once its programs have ended,
// before its unit (Wine, the container) has; a launcher that ends after starting the game does
// not bring it (owner, FrankenPC, 2026-11-09).
func TestHomeWhenTheGamesProgramsEnd(t *testing.T) {
	m, _, focus, _ := newManager(t, "gamescope")
	procs := &fakeProcs{unit: map[int]string{}, cmd: map[int][]string{}, parent: map[int]int{}}
	m.Procs = procs
	m.UnitOf = func(pid int) string {
		procs.mu.Lock()
		defer procs.mu.Unlock()
		return procs.unit[pid]
	}
	var mu sync.Mutex
	ends := map[int]func(){}
	m.watchExit = func(pid int, ended func()) bool {
		procs.mu.Lock()
		_, alive := procs.unit[pid]
		procs.mu.Unlock()
		if !alive {
			return false // as a pidfd: an ended process cannot be followed
		}
		mu.Lock()
		defer mu.Unlock()
		ends[pid] = ended
		return true
	}
	end := func(pid int) {
		procs.remove(pid)
		mu.Lock()
		f := ends[pid]
		mu.Unlock()
		if f == nil {
			t.Fatalf("process %d not followed", pid)
		}
		f()
	}
	followed := func(pid int) {
		deadline := time.Now().Add(2 * time.Second)
		for {
			mu.Lock()
			_, ok := ends[pid]
			mu.Unlock()
			if ok {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("process %d never followed", pid)
			}
			runtime.Gosched()
		}
	}
	shown := func() []uint32 {
		focus.mu.Lock()
		defer focus.mu.Unlock()
		return append([]uint32(nil), focus.shown...)
	}
	m.watchSession()
	procs.add(10, 1, "home.service", "owneet-frontend")
	focus.openWindow(10)
	focus.Show([]uint32{HomeAppID})
	if _, err := m.Launch(LaunchRequest{ID: "game", Name: "Game", Kind: KindGame, Command: []string{"/usr/lib/owneet/owneet-windows-run"}}); err != nil {
		t.Fatal(err)
	}
	unit := UnitName("game")
	game := m.appIDOf("game")
	procs.add(2000, 1, unit, `C:\windows\system32\services.exe`) // Wine's: never counted
	procs.add(2001, 1, unit, `X:\Games\Witcher\Launcher.exe`)
	focus.openWindow(2001)
	m.checkWindows()
	followed(2001)
	// the launcher starts the game, closes its window and ends: the game goes on
	procs.add(2002, 2001, unit, `X:\Games\Witcher\bin\witcher2.exe`, "-silent")
	focus.closeWindow(game)
	end(2001)
	if !sameIDs(shown(), []uint32{game}) || m.Status().Focus != "game" {
		t.Fatalf("the launcher ended: %v %q", shown(), m.Status().Focus)
	}
	followed(2002)
	focus.openWindow(2002)
	m.checkWindows()
	// Wine's explorer.exe shows a window of its own: still Wine's, never waited for (3b)
	procs.add(2003, 1, unit, `C:\windows\system32\explorer.exe`)
	focus.openWindow(2003)
	m.checkWindows()
	mu.Lock()
	_, waited := ends[2003]
	mu.Unlock()
	if waited {
		t.Fatal("Wine's explorer.exe followed as the game's")
	}
	// quit from its menu: its window goes, then its program ends; Wine's go on
	focus.closeWindow(game)
	end(2002)
	// (the window's own look at its process may come last and tell the end once more)
	deadline := time.Now().Add(2 * time.Second)
	for !sameIDs(shown(), []uint32{HomeAppID}) || m.Status().Focus != "home" {
		if time.Now().After(deadline) {
			t.Fatalf("the game's programs ended: %v %q", shown(), m.Status().Focus)
		}
		runtime.Gosched()
	}
	if _, ok := m.apps["game"]; !ok {
		t.Fatal("the game is still running until its unit ends")
	}
}

// A Linux launcher that hands over (owner, 2026-11-12, The Witcher 2's TV launcher): it starts the
// game and ends before the game has a window; the app stays in front while its unit has a process,
// and its unit lasts while any of its processes runs.
func TestHandoverKeepsTheGame(t *testing.T) {
	m, units, focus, _ := newManager(t, "gamescope")
	procs := &fakeProcs{unit: map[int]string{}, cmd: map[int][]string{}, parent: map[int]int{}}
	m.Procs = procs
	m.UnitOf = func(pid int) string {
		procs.mu.Lock()
		defer procs.mu.Unlock()
		return procs.unit[pid]
	}
	var mu sync.Mutex
	ends := map[int]func(){}
	m.watchExit = func(pid int, ended func()) bool {
		mu.Lock()
		defer mu.Unlock()
		ends[pid] = ended
		return true
	}
	end := func(pid int) {
		procs.remove(pid)
		mu.Lock()
		f := ends[pid]
		mu.Unlock()
		if f == nil {
			t.Fatalf("process %d not followed", pid)
		}
		f()
	}
	followed := func(pid int) {
		deadline := time.Now().Add(2 * time.Second)
		for {
			mu.Lock()
			_, ok := ends[pid]
			mu.Unlock()
			if ok {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("process %d never followed", pid)
			}
			runtime.Gosched()
		}
	}
	shown := func() []uint32 {
		focus.mu.Lock()
		defer focus.mu.Unlock()
		return append([]uint32(nil), focus.shown...)
	}
	m.watchSession()
	procs.add(10, 1, "home.service", "owneet-frontend")
	focus.openWindow(10)
	focus.Show([]uint32{HomeAppID})
	if _, err := m.Launch(LaunchRequest{ID: "game", Name: "Game", Kind: KindGame, Command: []string{"/usr/bin/env"}, Handover: true}); err != nil {
		t.Fatal(err)
	}
	units.mu.Lock()
	whileAny := append([]bool(nil), units.whileAny...)
	units.mu.Unlock()
	if len(whileAny) != 1 || !whileAny[0] {
		t.Fatalf("the unit does not last while any process runs: %v", whileAny)
	}
	unit := UnitName("game")
	game := m.appIDOf("game")
	procs.add(3001, 1, unit, "/games/w2/game/tenfoot-launcher")
	focus.openWindow(3001)
	m.checkWindows()
	followed(3001)
	// the launcher starts the game (no window yet), closes its window and ends
	procs.add(3002, 3001, unit, "./witcher2")
	focus.closeWindow(game)
	end(3001)
	if !sameIDs(shown(), []uint32{game}) || m.Status().Focus != "game" {
		t.Fatalf("the launcher handed over: %v %q", shown(), m.Status().Focus)
	}
	// the game's window comes; quit from its menu: the home screen comes back
	focus.openWindow(3002)
	m.checkWindows()
	followed(3002)
	focus.closeWindow(game)
	end(3002)
	deadline := time.Now().Add(2 * time.Second)
	for !sameIDs(shown(), []uint32{HomeAppID}) || m.Status().Focus != "home" {
		if time.Now().After(deadline) {
			t.Fatalf("the game ended: %v %q", shown(), m.Status().Focus)
		}
		runtime.Gosched()
	}
	// other games: the unit ends with its first program, as before
	if _, err := m.Launch(LaunchRequest{ID: "other", Name: "Other", Kind: KindGame, Command: []string{"/usr/bin/env"}}); err == nil {
		t.Fatal("two games at once")
	}
}

func TestWindowsProgram(t *testing.T) {
	for argv, want := range map[string]bool{
		`X:\Giochi\Exe Test\game.exe`:      true,
		`Z:\home\owneet\Games\x\setup.exe`: true,
		`C:\windows\system32\services.exe`: false,
		`c:\windows\system32\umu.exe`:      false,
		`/usr/bin/python3`:                 false,
		``:                                 false,
	} {
		if got := windowsProgram([]string{argv}); got != want {
			t.Errorf("%q: %v", argv, got)
		}
	}
}

// pidfdWatch tells the end of a real process.
func TestPidfdWatch(t *testing.T) {
	cmd := exec.Command("sleep", "0.2")
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start sleep:", err)
	}
	done := make(chan struct{})
	if !pidfdWatch(cmd.Process.Pid, func() { close(done) }) {
		t.Fatal("cannot follow a running process")
	}
	select {
	case <-done:
		t.Fatal("ended before the process did")
	default:
	}
	go cmd.Wait()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the end of the process was not told")
	}
	if pidfdWatch(1<<22+12345, func() {}) {
		t.Fatal("a process that does not exist was followed")
	}
}
