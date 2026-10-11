// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// fakeProcs: processes with their unit, command line and parent.
type fakeProcs struct {
	mu      sync.Mutex
	unit    map[int]string
	cmd     map[int][]string
	parent  map[int]int
	signals []string
}

func (f *fakeProcs) add(pid, parent int, unit string, argv ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unit[pid], f.parent[pid], f.cmd[pid] = unit, parent, argv
}
func (f *fakeProcs) remove(pid int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.unit, pid)
}
func (f *fakeProcs) InUnit(unit string) []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	var pids []int
	for pid, u := range f.unit {
		if u == unit {
			pids = append(pids, pid)
		}
	}
	return pids
}
func (f *fakeProcs) Cmdline(pid int) []string { f.mu.Lock(); defer f.mu.Unlock(); return f.cmd[pid] }
func (f *fakeProcs) Parent(pid int) int       { f.mu.Lock(); defer f.mu.Unlock(); return f.parent[pid] }
func (f *fakeProcs) Signal(pid int, sig syscall.Signal) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signals = append(f.signals, fmt.Sprintf("%d:%d", pid, sig))
	return nil
}

func newSteamManager(t *testing.T) (*Manager, *fakeUnits, *fakeFocus, *fakeProcs, *[]string, <-chan events.Event) {
	t.Helper()
	m, units, focus, ch := newManager(t, "gamescope")
	procs := &fakeProcs{unit: map[int]string{}, cmd: map[int][]string{}, parent: map[int]int{}}
	forwarded := &[]string{}
	unitOf := m.UnitOf
	m.UnitOf = func(pid int) string {
		if u := procs.unit[pid]; u != "" {
			return u
		}
		return unitOf(pid)
	}
	m.Procs = procs
	m.SteamDir = t.TempDir()
	m.SteamAllowed = func() bool { return true }
	m.Forward = func(argv, _ []string) error { *forwarded = append(*forwarded, strings.Join(argv, " ")); return nil }
	m.watchSession() // tags windows: the home screen is process 10
	focus.openWindow(10)
	return m, units, focus, procs, forwarded, ch
}

func next(t *testing.T, ch <-chan events.Event, typ string) events.Event {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case e := <-ch:
			if e.Type == typ {
				return e
			}
		case <-timeout:
			t.Fatalf("no %s event", typ)
		}
	}
}

func TestReaperAppID(t *testing.T) {
	for argv, want := range map[string]string{
		"/home/u/.local/share/Steam/ubuntu12_32/reaper SteamLaunch AppId=480 -- /x/game": "480",
		"reaper SteamLaunch AppId=480":        "480",
		"reaper AppId=480 -- x":               "", // not a launch
		"reaper SteamLaunch -- AppId=480":     "", // the game's own arguments
		"/usr/bin/game SteamLaunch AppId=480": "",
		"reaper SteamLaunch AppId=48x0 -- x":  "",
	} {
		if got := reaperAppID(strings.Fields(argv)); got != want {
			t.Errorf("%q: got %q, want %q", argv, got, want)
		}
	}
}

func TestSteamStartsWithoutGamescopeControl(t *testing.T) {
	m, units, focus, _, forwarded, ch := newSteamManager(t)
	if err := m.Steam(SteamRequest{Open: "library"}); err != nil {
		t.Fatal(err)
	}
	if len(units.started) != 1 || units.started[0] != UnitName(SteamID) {
		t.Fatalf("started %v", units.started)
	}
	argv := strings.Join(units.argvs[0], " ")
	if argv != "/usr/bin/env -u GAMESCOPE_WAYLAND_DISPLAY /usr/bin/steam -gamepadui steam://open/games" {
		t.Fatalf("argv %q", argv)
	}
	m.checkWindows()       // no window yet: nothing
	focus.openWindow(1001) // Steam's window
	m.checkWindows()
	if e := next(t, ch, "app.window"); e.Data.(map[string]string)["id"] != SteamID {
		t.Fatalf("app.window %v", e.Data)
	}
	if st := m.Status(); st.Focus != SteamID {
		t.Fatalf("focus %q", st.Focus)
	}
	// Steam is running: the address goes to it, nothing new starts
	if err := m.Steam(SteamRequest{Open: "store"}); err != nil {
		t.Fatal(err)
	}
	if len(units.started) != 1 || len(*forwarded) != 1 || !strings.HasSuffix((*forwarded)[0], "/usr/bin/steam steam://store") {
		t.Fatalf("started %v, forwarded %v", units.started, *forwarded)
	}
	for _, bad := range []SteamRequest{{}, {Open: "settings"}, {Run: "12a"}, {Open: "store", Run: "1"}} {
		if err := m.Steam(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	m.SteamAllowed = func() bool { return false }
	if err := m.Steam(SteamRequest{Open: "library"}); !errors.Is(err, ErrSteamNeedsInstall) {
		t.Fatalf("live system without memory: %v", err)
	}
}

func TestSteamGameIsAGameOfItsOwn(t *testing.T) {
	m, units, focus, procs, forwarded, ch := newSteamManager(t)
	if err := m.Steam(SteamRequest{Open: "library"}); err != nil {
		t.Fatal(err)
	}
	next(t, ch, "app.started") // Steam itself
	steamUnit := units.started[0]
	focus.openWindow(1001)
	if err := m.Steam(SteamRequest{Run: "480", Name: "Spacewar"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix((*forwarded)[0], "steam://rungameid/480") {
		t.Fatalf("forwarded %v", *forwarded)
	}
	// Steam starts the game under its launcher, inside Steam's unit
	procs.add(1001, 1, steamUnit, "/home/u/.local/share/Steam/ubuntu12_32/steam", "-gamepadui")
	procs.add(2000, 1001, steamUnit, "/home/u/.local/share/Steam/ubuntu12_32/reaper", "SteamLaunch", "AppId=480", "--", "/g/spacewar")
	procs.add(2001, 2000, steamUnit, "/g/spacewar")
	m.scanSteam()
	e := next(t, ch, "app.started")
	if d := e.Data.(map[string]string); d["id"] != "steam:480" || d["name"] != "Spacewar" || d["kind"] != KindGame {
		t.Fatalf("app.started %v", d)
	}
	focus.openWindow(2001)
	if st := m.Status(); st.Focus != "steam:480" || len(st.Apps) != 2 {
		t.Fatalf("status %+v", st)
	}
	// one game at a time
	if err := m.Steam(SteamRequest{Run: "570"}); !errors.Is(err, ErrGameRunning) {
		t.Fatalf("second game: %v", err)
	}
	if _, err := m.Launch(LaunchRequest{ID: "other", Kind: KindGame, Command: []string{"game"}}); !errors.Is(err, ErrGameRunning) {
		t.Fatalf("other game: %v", err)
	}
	// Guide: home, then back to the game
	m.Guide(true)
	if st := m.Status(); st.Focus != "home" {
		t.Fatalf("after Guide %q", st.Focus)
	}
	m.Guide(true)
	if st := m.Status(); st.Focus != "steam:480" {
		t.Fatalf("Guide again %q", st.Focus)
	}
	// closing the game signals the game only: Steam keeps running
	if err := m.Close("steam:480"); err != nil {
		t.Fatal(err)
	}
	if len(procs.signals) != 1 || procs.signals[0] != fmt.Sprintf("2001:%d", syscall.SIGTERM) || len(units.stopped) != 0 {
		t.Fatalf("signals %v, stopped %v", procs.signals, units.stopped)
	}
	procs.remove(2001)
	procs.remove(2000)
	m.scanSteam()
	if d := next(t, ch, "app.exited").Data.(map[string]string); d["id"] != "steam:480" {
		t.Fatalf("app.exited %v", d)
	}
	if st := m.Status(); st.Focus != "home" || len(st.Apps) != 1 {
		t.Fatalf("after the game %+v", st)
	}
}

func TestSteamGameStartedInSteamAndSteamClosing(t *testing.T) {
	m, units, focus, procs, _, ch := newSteamManager(t)
	if err := m.Steam(SteamRequest{Open: "library"}); err != nil {
		t.Fatal(err)
	}
	next(t, ch, "app.started") // Steam itself
	steamUnit := units.started[0]
	focus.openWindow(1001)
	// a game chosen in Steam's own library; its window appears before the next scan
	procs.add(3000, 1, steamUnit, "reaper", "SteamLaunch", "AppId=570", "--", "/g/dota")
	procs.add(3001, 3000, steamUnit, "/g/dota")
	focus.openWindow(3001)
	if d := next(t, ch, "app.started").Data.(map[string]string); d["id"] != "steam:570" || d["name"] != "Steam" {
		t.Fatalf("app.started %v", d)
	}
	if st := m.Status(); st.Focus != "steam:570" {
		t.Fatalf("focus %q", st.Focus)
	}
	// Steam's unit stops: the game and Steam are gone, newest first
	units.exits <- Exit{Unit: steamUnit, Result: "success"}
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go m.Run(stop)
	if d := next(t, ch, "app.exited").Data.(map[string]string); d["id"] != "steam:570" {
		t.Fatalf("first exit %v", d)
	}
	if d := next(t, ch, "app.exited").Data.(map[string]string); d["id"] != SteamID {
		t.Fatalf("second exit %v", d)
	}
}

// Steam writes what gamescope shows in its own codes: owneetd translates them while Steam is in
// front, and puts its own choice back otherwise (Steam never takes the screen by itself).
func TestSteamRequestsWhatToShow(t *testing.T) {
	m, units, focus, procs, _, ch := newSteamManager(t)
	if err := m.Steam(SteamRequest{Open: "library"}); err != nil {
		t.Fatal(err)
	}
	next(t, ch, "app.started")
	steamUnit := units.started[0]
	focus.openWindow(1001)
	steamID := m.appIDOf(SteamID)
	shown := func() []uint32 {
		focus.mu.Lock()
		defer focus.mu.Unlock()
		return append([]uint32(nil), focus.shown...)
	}

	focus.request([]uint32{413091, steamUIAppID}) // Steam starting: its own window
	if got := shown(); !sameIDs(got, []uint32{steamID, HomeAppID}) {
		t.Fatalf("Steam in front: %v", got)
	}
	procs.add(2000, 1001, steamUnit, "reaper", "SteamLaunch", "AppId=480", "--", "/g/spacewar")
	procs.add(2001, 2000, steamUnit, "/g/spacewar")
	focus.openWindow(2001)
	gameID := m.appIDOf("steam:480")
	focus.request([]uint32{480, steamUIAppID}) // the game, in Steam's codes
	if got := shown(); !sameIDs(got, []uint32{gameID, steamID, HomeAppID}) {
		t.Fatalf("Steam game: %v", got)
	}
	m.Guide(true) // the home screen: Steam cannot take it back
	focus.request([]uint32{steamUIAppID})
	if got := shown(); !sameIDs(got, []uint32{HomeAppID}) || m.Status().Focus != "home" {
		t.Fatalf("home screen: %v, focus %q", got, m.Status().Focus)
	}
}

func TestSteamStatusListsInstalledGames(t *testing.T) {
	m, _, _, _, _, _ := newSteamManager(t)
	other := t.TempDir()
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := func(lib, id, flags string) {
		write(filepath.Join(lib, "steamapps", "appmanifest_"+id+".acf"),
			"\"AppState\"\n{\n\t\"appid\"\t\t\""+id+"\"\n\t\"StateFlags\"\t\t\""+flags+"\"\n}\n")
	}
	write(filepath.Join(m.SteamDir, "steamapps", "libraryfolders.vdf"),
		"\"libraryfolders\"\n{\n\t\"0\"\n\t{\n\t\t\"path\"\t\t\""+m.SteamDir+"\"\n\t}\n\t\"1\"\n\t{\n\t\t\"path\"\t\t\""+other+"\"\n\t}\n}\n")
	manifest(m.SteamDir, "480", "4")
	manifest(m.SteamDir, "570", "1026") // downloading
	manifest(other, "620", "6")         // update waiting: installed
	if got := strings.Join(m.SteamStatus().Games, ","); got != "480,620" {
		t.Fatalf("games %q", got)
	}
}
