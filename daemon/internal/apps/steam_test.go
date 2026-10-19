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
	m.StateDir = t.TempDir()
	m.SteamAllowed = func() bool { return true }
	m.NVIDIA = func() bool { return false }
	m.Forward = func(argv, _ []string) error { *forwarded = append(*forwarded, strings.Join(argv, " ")); return nil }
	m.watchSession() // tags windows: the home screen is process 10
	focus.openWindow(10)
	return m, units, focus, procs, forwarded, ch
}

// settle lets Steam's window stay until owneetd shows it
func settle(m *Manager) {
	for i := 0; i < settleChecks; i++ {
		m.checkWindows()
	}
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

func TestSteamStartsInTheConsoleLanguage(t *testing.T) {
	m, units, _, _, _, _ := newSteamManager(t)
	if err := m.Steam(SteamRequest{Open: "store", Language: "it"}); err != nil {
		t.Fatal(err)
	}
	argv := strings.Join(units.argvs[0], " ")
	if argv != "/usr/bin/env -u GAMESCOPE_WAYLAND_DISPLAY /usr/bin/steam -gamepadui -language italian" {
		t.Fatalf("argv %q", argv)
	}
	for tag, want := range map[string]string{"pt-BR": "brazilian", "pt-PT": "portuguese", "it-CH": "italian", "zh-Hant": "tchinese", "xx": ""} {
		if got := steamLanguage(tag); got != want {
			t.Errorf("steamLanguage(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestSteamInCageStartsOnTheAddress(t *testing.T) {
	m, units, _, _ := newManager(t, "cage")
	m.Procs = &fakeProcs{unit: map[int]string{}, cmd: map[int][]string{}, parent: map[int]int{}}
	m.SteamDir = t.TempDir()
	m.StateDir = t.TempDir()
	m.SteamAllowed = func() bool { return true }
	m.NVIDIA = func() bool { return false }
	m.FocusFor = func(Session) Focus { return nil } // cage: owneetd sees no windows
	if err := m.Steam(SteamRequest{Open: "library"}); err != nil {
		t.Fatal(err)
	}
	if argv := strings.Join(units.argvs[0], " "); !strings.HasSuffix(argv, "-gamepadui steam://open/games") {
		t.Fatalf("argv %q", argv)
	}
}

func TestSteamClosesItselfCleanly(t *testing.T) {
	m, units, _, _, _, _ := newSteamManager(t)
	if err := m.Steam(SteamRequest{Open: "library"}); err != nil {
		t.Fatal(err)
	}
	stop := units.stops[0]
	if len(stop) != 4 || stop[0] != "/bin/sh" || !strings.Contains(stop[2], `"$0" -shutdown`) ||
		!strings.Contains(stop[2], `kill -0 "$MAINPID"`) ||
		stop[3] != filepath.Join(m.SteamDir, "ubuntu12_32/steam-runtime/amd64/usr/bin/steam-runtime-steam-remote") {
		t.Fatalf("stop %q", stop)
	}
	// other apps are stopped as before
	if _, err := m.Launch(LaunchRequest{ID: "game", Name: "Game", Kind: KindGame, Command: []string{"/bin/true"}}); err != nil {
		t.Fatal(err)
	}
	if units.stops[1] != nil {
		t.Fatalf("game stop %q", units.stops[1])
	}
}

// Steam's pages keep the GPU on NVIDIA too since the repaint overlay (2026-10-15); the test switch
// steam_cef_gpu=off (not set here) would draw them without it
func TestSteamWithNVIDIAKeepsTheGPU(t *testing.T) {
	m, units, _, _, _, _ := newSteamManager(t)
	m.NVIDIA = func() bool { return true }
	if err := m.Steam(SteamRequest{Open: "library"}); err != nil {
		t.Fatal(err)
	}
	argv := strings.Join(units.argvs[0], " ")
	if argv != "/usr/bin/env -u GAMESCOPE_WAYLAND_DISPLAY /usr/bin/steam -gamepadui" {
		t.Fatalf("argv %q", argv)
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
	// Steam starts on its own home page: Back from the page asked for has somewhere to go
	argv := strings.Join(units.argvs[0], " ")
	if argv != "/usr/bin/env -u GAMESCOPE_WAYLAND_DISPLAY /usr/bin/steam -gamepadui" {
		t.Fatalf("argv %q", argv)
	}
	m.checkWindows()       // no window yet: nothing
	focus.openWindow(1001) // Steam's window: shown once it has stayed for a few seconds
	for i := 0; i < settleChecks-1; i++ {
		m.checkWindows()
		if st := m.Status(); st.Focus != "home" {
			t.Fatalf("Steam shown before its window settled (check %d)", i+1)
		}
	}
	m.checkWindows()
	if e := next(t, ch, "app.window"); e.Data.(map[string]string)["id"] != SteamID {
		t.Fatalf("app.window %v", e.Data)
	}
	// Steam's window took the screen by itself: the interface must stop reading the controllers
	if e := next(t, ch, "focus.changed"); e.Data.(map[string]string)["focus"] != SteamID {
		t.Fatalf("focus.changed %v", e.Data)
	}
	if st := m.Status(); st.Focus != SteamID {
		t.Fatalf("focus %q", st.Focus)
	}
	// shown: now the page asked for, once
	if len(*forwarded) != 1 || !strings.HasSuffix((*forwarded)[0], "/usr/bin/steam steam://open/games") {
		t.Fatalf("forwarded %v", *forwarded)
	}
	m.checkWindows()
	if len(*forwarded) != 1 {
		t.Fatalf("forwarded again: %v", *forwarded)
	}
	// Steam is running: the address goes to it, nothing new starts
	if err := m.Steam(SteamRequest{Open: "store"}); err != nil {
		t.Fatal(err)
	}
	if len(units.started) != 1 || len(*forwarded) != 2 || !strings.HasSuffix((*forwarded)[1], "/usr/bin/steam steam://store") {
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
	settle(m)
	if err := m.Steam(SteamRequest{Run: "480", Name: "Spacewar"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix((*forwarded)[len(*forwarded)-1], "steam://rungameid/480") {
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
	// Guide over a Steam game: a short press is Steam's (its overlay); held, the console's home
	// screen; from there a short press goes back to the game
	m.HomeHold = 100 * time.Millisecond
	m.Guide(true)
	m.Guide(false)
	time.Sleep(200 * time.Millisecond)
	if st := m.Status(); st.Focus != "steam:480" {
		t.Fatalf("short press over Steam %q", st.Focus)
	}
	m.Guide(true)
	time.Sleep(250 * time.Millisecond)
	m.Guide(false)
	if st := m.Status(); st.Focus != "home" {
		t.Fatalf("Guide held %q", st.Focus)
	}
	m.Guide(true)
	m.Guide(false)
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
	settle(m)
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

// A Steam game that has no window yet keeps Steam's window off the screen (Silksong, FrankenPC
// 2026-10-16: Steam's window was shown for 10 s before the game's): the home screen covers the
// wait; Steam comes back behind the game once the game's window is there, or after steamAskAfter
// without one (Steam may be asking the user something).
func TestSteamStaysBehindAGameWithoutWindow(t *testing.T) {
	m, units, focus, procs, _, ch := newSteamManager(t)
	if err := m.Steam(SteamRequest{Run: "480", Name: "Spacewar"}); err != nil {
		t.Fatal(err)
	}
	next(t, ch, "app.started") // Steam itself
	steamUnit := units.started[0]
	focus.openWindow(1001)
	settle(m)
	steamID := m.appIDOf(SteamID)
	shown := func() []uint32 {
		focus.mu.Lock()
		defer focus.mu.Unlock()
		return append([]uint32(nil), focus.shown...)
	}
	procs.add(1001, 1, steamUnit, "steam", "-gamepadui")
	procs.add(2000, 1001, steamUnit, "reaper", "SteamLaunch", "AppId=480", "--", "/g/spacewar")
	procs.add(2001, 2000, steamUnit, "/g/spacewar")
	m.scanSteam()
	next(t, ch, "app.started")
	gameID := m.appIDOf("steam:480")
	if got := shown(); !sameIDs(got, []uint32{gameID, HomeAppID}) || m.Status().Focus != "home" {
		t.Fatalf("game without window: %v, focus %q", got, m.Status().Focus)
	}
	focus.request([]uint32{480, steamUIAppID}) // Steam asks for the game, then itself
	if got := shown(); !sameIDs(got, []uint32{gameID, HomeAppID}) {
		t.Fatalf("Steam's request before the game's window: %v", got)
	}
	focus.openWindow(2001)
	m.checkWindows()
	if got := shown(); !sameIDs(got, []uint32{gameID, steamID, HomeAppID}) || m.Status().Focus != "steam:480" {
		t.Fatalf("game's window: %v, focus %q", got, m.Status().Focus)
	}
	m.mu.Lock()
	game := m.apps["steam:480"]
	m.mu.Unlock()
	m.steamGameNoWindow(game) // the game had its window: nothing changes
	if got := shown(); !sameIDs(got, []uint32{gameID, steamID, HomeAppID}) {
		t.Fatalf("after steamAskAfter with a window: %v", got)
	}

	// another game that never opens a window: Steam is shown after steamAskAfter
	procs.remove(2001)
	procs.remove(2000)
	m.scanSteam()
	next(t, ch, "app.exited")
	procs.add(3000, 1001, steamUnit, "reaper", "SteamLaunch", "AppId=570", "--", "/g/dota")
	procs.add(3001, 3000, steamUnit, "/g/dota")
	m.scanSteam()
	next(t, ch, "app.started")
	otherID := m.appIDOf("steam:570")
	m.mu.Lock()
	other := m.apps["steam:570"]
	m.mu.Unlock()
	if got := shown(); !sameIDs(got, []uint32{otherID, HomeAppID}) {
		t.Fatalf("second game without window: %v", got)
	}
	m.steamGameNoWindow(other)
	if got := shown(); !sameIDs(got, []uint32{otherID, steamID, HomeAppID}) || m.Status().Focus != SteamID {
		t.Fatalf("after steamAskAfter without a window: %v, focus %q", got, m.Status().Focus)
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
	settle(m)
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
	m.checkWindows() // the game's window came
	gameID := m.appIDOf("steam:480")
	focus.request([]uint32{480, steamUIAppID}) // the game, in Steam's codes
	if got := shown(); !sameIDs(got, []uint32{gameID, steamID, HomeAppID}) {
		t.Fatalf("Steam game: %v", got)
	}
	m.HomeHold = 50 * time.Millisecond
	m.Guide(true) // held: the home screen; Steam cannot take it back
	time.Sleep(150 * time.Millisecond)
	m.Guide(false)
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

// cage shows the newest window: the interface hears that an app took the screen, and when the
// home screen is back
func TestCageFocusFollowsTheNewestApp(t *testing.T) {
	m, units, _, ch := newManager(t, "cage")
	if _, err := m.Launch(LaunchRequest{ID: SteamID, Kind: KindApp, Command: []string{"steam"}}); err != nil {
		t.Fatal(err)
	}
	if d := next(t, ch, "focus.changed").Data.(map[string]string); d["focus"] != SteamID {
		t.Fatalf("after start %v", d)
	}
	m.exited(Exit{Unit: units.started[0], Result: "success"})
	if d := next(t, ch, "focus.changed").Data.(map[string]string); d["focus"] != "home" {
		t.Fatalf("after exit %v", d)
	}
}

// writeFile writes a test file, with its folders.
func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// steamManifestText is an appmanifest with these StateFlags and bytes downloaded / to download.
func steamManifestText(id string, flags, done, total int) string {
	return fmt.Sprintf("\"AppState\"\n{\n\t\"appid\"\t\t\"%s\"\n\t\"StateFlags\"\t\t\"%d\"\n"+
		"\t\"BytesToDownload\"\t\t\"%d\"\n\t\"BytesDownloaded\"\t\t\"%d\"\n}\n", id, flags, total, done)
}

// Installing and uninstalling: Steam shows its confirmation; once Steam acts on it (the game's
// appmanifest appears, or goes), the home screen comes back by itself
// ([[step-4.2c-steam-without-its-interface]])
func TestSteamConfirmationsComeBackHome(t *testing.T) {
	m, _, focus, _, forwarded, ch := newSteamManager(t)
	if err := m.Steam(SteamRequest{Open: "store"}); err != nil {
		t.Fatal(err)
	}
	focus.openWindow(1001)
	settle(m)
	m.backHome() // Guide held: the console
	if err := m.Steam(SteamRequest{Install: "480"}); err != nil {
		t.Fatal(err)
	}
	if last := (*forwarded)[len(*forwarded)-1]; !strings.HasSuffix(last, "steam://install/480") {
		t.Fatalf("forwarded %v", *forwarded)
	}
	if st := m.Status(); st.Focus != SteamID {
		t.Fatalf("Steam's confirmation not shown: %q", st.Focus)
	}
	m.watchSteam() // nothing yet: Steam stays on screen
	if st := m.Status(); st.Focus != SteamID {
		t.Fatalf("home too early: %q", st.Focus)
	}
	if e := next(t, ch, "steam.changed"); len(e.Data.(SteamStatus).Downloads) != 0 {
		t.Fatalf("first steam.changed %+v", e.Data)
	}
	manifest := filepath.Join(m.SteamDir, "steamapps", "appmanifest_480.acf")
	writeFile(t, manifest, steamManifestText("480", 1026, 0, 1000))
	m.watchSteam()
	if st := m.Status(); st.Focus != "home" {
		t.Fatalf("after confirming: %q", st.Focus)
	}
	if e := next(t, ch, "steam.changed"); len(e.Data.(SteamStatus).Downloads) != 1 {
		t.Fatalf("steam.changed %+v", e.Data)
	}
	// Guide from the home screen does not bring Steam's own window back
	m.Guide(true)
	m.Guide(false)
	if st := m.Status(); st.Focus != "home" {
		t.Fatalf("Guide brought Steam back: %q", st.Focus)
	}
	// cancelling the download: steam://uninstall, home once the manifest goes
	if err := m.Steam(SteamRequest{Uninstall: "480"}); err != nil {
		t.Fatal(err)
	}
	if last := (*forwarded)[len(*forwarded)-1]; !strings.HasSuffix(last, "steam://uninstall/480") {
		t.Fatalf("forwarded %v", *forwarded)
	}
	os.Remove(manifest)
	m.watchSteam()
	if st := m.Status(); st.Focus != "home" {
		t.Fatalf("after uninstalling: %q", st.Focus)
	}
	for _, bad := range []SteamRequest{{Install: "x"}, {Install: "1", Uninstall: "2"}, {Run: "1", Install: "1"}} {
		if err := m.Steam(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

// Running a game keeps Steam's window hidden when an account has signed in: the home screen stays
// until the game's window; Steam is shown if no game comes (it may wait for the user)
func TestSteamStartsGamesHidden(t *testing.T) {
	m, _, focus, procs, forwarded, _ := newSteamManager(t)
	writeFile(t, filepath.Join(m.SteamDir, "config", "loginusers.vdf"),
		"\"users\"\n{\n\t\"76561190000000001\"\n\t{\n\t\t\"AccountName\"\t\t\"someone\"\n\t}\n}\n")
	if err := m.Steam(SteamRequest{Run: "480", Name: "Spacewar"}); err != nil {
		t.Fatal(err)
	}
	focus.openWindow(1001)
	settle(m)
	if st := m.Status(); st.Focus != "home" {
		t.Fatalf("Steam shown while starting a game: %q", st.Focus)
	}
	if last := (*forwarded)[len(*forwarded)-1]; !strings.HasSuffix(last, "steam://rungameid/480") {
		t.Fatalf("forwarded %v", *forwarded)
	}
	// Steam preparing the game's shaders: still hidden, the wait starts again
	procs.add(1500, 1001, UnitName(SteamID), "/s/ubuntu12_64/fossilize_replay", "--master-process")
	m.steamAskNow()
	if st := m.Status(); st.Focus != "home" {
		t.Fatalf("Steam shown while preparing shaders: %q", st.Focus)
	}
	m.mu.Lock()
	again := m.steamAsk != nil
	m.mu.Unlock()
	if !again {
		t.Fatal("no new wait after the shaders")
	}
	procs.remove(1500)
	// no game: Steam is shown (as steamAskAfter would)
	m.steamAskNow()
	if st := m.Status(); st.Focus != SteamID {
		t.Fatalf("Steam not shown when no game came: %q", st.Focus)
	}
	// the next game, Steam running: still hidden, the game's window comes to the front
	m.backHome()
	if err := m.Steam(SteamRequest{Run: "570"}); err != nil {
		t.Fatal(err)
	}
	if st := m.Status(); st.Focus != "home" {
		t.Fatalf("Steam shown for the second game: %q", st.Focus)
	}
	unit := UnitName(SteamID)
	procs.add(2000, 1001, unit, "/s/reaper", "SteamLaunch", "AppId=570", "--", "/g/dota")
	procs.add(2001, 2000, unit, "/g/dota")
	m.scanSteam()
	focus.openWindow(2001)
	if st := m.Status(); st.Focus != "steam:570" {
		t.Fatalf("game not shown: %q", st.Focus)
	}
	m.mu.Lock()
	asking := m.steamAsk != nil
	m.mu.Unlock()
	if asking {
		t.Fatal("Steam still to be shown after the game started")
	}
}

// The status lists downloads (the one moving first), games on a disk not connected with its
// name, whether someone signed in, and when the licences were written
func TestSteamStatusDownloadsAndDisks(t *testing.T) {
	m, _, _, _, _, _ := newSteamManager(t)
	dir := m.SteamDir
	gone := "/run/media/someone/GamesSSD/SteamLibrary"
	writeFile(t, filepath.Join(dir, "steamapps", "libraryfolders.vdf"), "\"libraryfolders\"\n{\n"+
		"\t\"0\"\n\t{\n\t\t\"path\"\t\t\""+dir+"\"\n\t\t\"apps\"\n\t\t{\n\t\t\t\"480\"\t\t\"1\"\n\t\t}\n\t}\n"+
		"\t\"1\"\n\t{\n\t\t\"path\"\t\t\""+gone+"\"\n\t\t\"label\"\t\t\"\"\n\t\t\"apps\"\n\t\t{\n\t\t\t\"700\"\t\t\"5\"\n\t\t}\n\t}\n"+
		"\t\"2\"\n\t{\n\t\t\"path\"\t\t\"/mnt/elsewhere\"\n\t\t\"label\"\t\t\"Big disk\"\n\t\t\"apps\"\n\t\t{\n\t\t\t\"710\"\t\t\"5\"\n\t\t}\n\t}\n}\n")
	man := func(id string, flags, done, total int) {
		writeFile(t, filepath.Join(dir, "steamapps", "appmanifest_"+id+".acf"), steamManifestText(id, flags, done, total))
	}
	man("480", 4, 0, 0)         // installed
	man("500", 1026, 0, 4000)   // waiting for its turn
	man("510", 1026, 100, 2000) // will move
	man("520", 6, 0, 50)        // installed, an update waiting: playable, not listed
	man("530", 3078, 10, 100)   // uninstalling: not listed
	man("540", 1, 241, 10989)   // a cancelled download, uninstalled: not listed (FrankenPC)
	man("550", 0, 0, 5000)      // bytes to download but nothing asked: not listed
	st := m.SteamStatus()
	if len(st.Downloads) != 2 || st.Downloads[0].State != "queued" || st.Downloads[1].State != "queued" {
		t.Fatalf("first look %+v", st.Downloads)
	}
	man("510", 1026, 900, 2000)
	man("480", 4|stateUpdateRunning, 30, 60)
	st = m.SteamStatus()
	got := []string{}
	for _, d := range st.Downloads {
		got = append(got, d.AppID+":"+d.State)
	}
	if strings.Join(got, ",") != "480:updating,510:downloading,500:queued" && strings.Join(got, ",") != "510:downloading,480:updating,500:queued" {
		t.Fatalf("downloads %v", got)
	}
	for _, d := range st.Downloads {
		if d.AppID == "510" && (d.Done != 900 || d.Total != 2000 || d.Rate <= 0) {
			t.Fatalf("510: %+v", d)
		}
	}
	if st.Away["700"] != "GamesSSD" || st.Away["710"] != "Big disk" || len(st.Away) != 2 {
		t.Fatalf("away %v", st.Away)
	}
	if st.Account || st.Licences != 0 {
		t.Fatalf("account %v, licences %d", st.Account, st.Licences)
	}
	writeFile(t, filepath.Join(dir, "config", "loginusers.vdf"), "\"users\"\n{\n\t\"76561190000000001\"\n\t{\n\t}\n}\n")
	writeFile(t, filepath.Join(dir, "appcache", "packageinfo.vdf"), "x")
	if st = m.SteamStatus(); !st.Account || st.Licences == 0 {
		t.Fatalf("account %v, licences %d", st.Account, st.Licences)
	}
}

func TestKeyValuesText(t *testing.T) {
	kv := kvText("\"AppState\"\n{\n\t\"appid\"\t\"480\" // comment\n\t\"Name\"\t\"Space \\\"war\\\"\"\n\t\"x\"\n\t{\n\t}\n}\n")
	if kvGet(kv, "appstate", "APPID") != "480" || kvGet(kv, "AppState", "name") != "Space \"war\"" {
		t.Fatalf("%v", kv)
	}
	if mountName("/media/u/Disk/Steam") != "Disk" || mountName("/home/u/Steam") != "" {
		t.Fatal("mount names")
	}
}

// A game whose licence agreement Steam will ask to accept: the interface says so first (ask), and
// Steam's window is shown at once for that start only
func TestSteamShownForAGameThatAsks(t *testing.T) {
	m, _, focus, _, _, _ := newSteamManager(t)
	writeFile(t, filepath.Join(m.SteamDir, "config", "loginusers.vdf"), "\"users\"\n{\n\t\"76561190000000001\"\n\t{\n\t}\n}\n")
	if err := m.Steam(SteamRequest{Ask: "208650"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Steam(SteamRequest{Run: "208650"}); err != nil {
		t.Fatal(err)
	}
	focus.openWindow(1001)
	settle(m)
	if st := m.Status(); st.Focus != SteamID {
		t.Fatalf("Steam not shown for a game that asks: %q", st.Focus)
	}
	m.backHome()
	if err := m.Steam(SteamRequest{Run: "208650"}); err != nil { // the next start: hidden again
		t.Fatal(err)
	}
	if st := m.Status(); st.Focus != "home" {
		t.Fatalf("Steam shown again: %q", st.Focus)
	}
	if err := m.Steam(SteamRequest{Ask: "1", Run: "1"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ask with run: %v", err)
	}
}

// Repainter: a Focus that records the changing overlay asked for
type repaintFocus struct {
	*fakeFocus
	calls []bool
}

func (r *repaintFocus) Repaint(on bool) { r.calls = append(r.calls, on) }

// NVIDIA test switch: the overlay runs while Steam's own window is on screen, not over the home
// screen or a game
func TestSteamRepaintWhileSteamIsShown(t *testing.T) {
	m, _, focus, _, _, _ := newSteamManager(t)
	rf := &repaintFocus{fakeFocus: focus}
	m.FocusFor = func(Session) Focus { return rf }
	m.SteamRepaint = func() bool { return true }
	if err := m.Steam(SteamRequest{Open: "store"}); err != nil {
		t.Fatal(err)
	}
	focus.openWindow(1001)
	settle(m)
	if len(rf.calls) == 0 || !rf.calls[len(rf.calls)-1] {
		t.Fatalf("no overlay with Steam on screen: %v", rf.calls)
	}
	m.backHome()
	if rf.calls[len(rf.calls)-1] {
		t.Fatalf("overlay left on over the home screen: %v", rf.calls)
	}
	m.SteamRepaint = func() bool { return false } // switch off (or not NVIDIA)
	m.FocusApp(SteamID)
	if rf.calls[len(rf.calls)-1] {
		t.Fatalf("overlay without the switch: %v", rf.calls)
	}
}

// Steam Input on for every controller, once per account, while Steam is closed; the user's later
// choice in Steam is kept (owner, 2026-10-15)
func TestSteamInputTurnedOnOnce(t *testing.T) {
	m, _, _, _, _, _ := newSteamManager(t)
	file := filepath.Join(m.SteamDir, "userdata", "123", "config", "localconfig.vdf")
	writeFile(t, file, "\"UserLocalConfigStore\"\n{\n\t\"Software\"\n\t{\n\t\t\"SteamController_XBoxSupport\"\t\t\"0\"\n\t}\n"+
		"\t\"SteamController_XBoxSupport\"\t\t\"0\"\n\t\"apps\"\n\t{\n\t}\n}\n")
	if err := m.Steam(SteamRequest{Open: "store"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	kv, _ := kvGet(kvText(string(data)), "UserLocalConfigStore").(map[string]any)
	for _, k := range steamInputSettings {
		if kvGet(kv, k) != "1" {
			t.Fatalf("%s not on:\n%s", k, data)
		}
	}
	if kvGet(kv, "Software", "SteamController_XBoxSupport") != "0" {
		t.Fatalf("a nested block was changed:\n%s", data)
	}
	// the user turns Xbox off in Steam: the next start keeps it
	writeFile(t, file, strings.Replace(string(data), "\"SteamController_XBoxSupport\"\t\t\"1\"", "\"SteamController_XBoxSupport\"\t\t\"0\"", 1))
	if changed, err := steamInputOnce(m.SteamDir, m.StateDir); err != nil || len(changed) != 0 {
		t.Fatalf("set again: %v %v", changed, err)
	}
	if _, ok := setTopLevel("\"other\"\n{\n}\n", "UserLocalConfigStore", steamInputSettings, "1"); ok {
		t.Fatal("changed a file without the root block")
	}
}
