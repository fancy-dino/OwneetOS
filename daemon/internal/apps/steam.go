// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Steam ([[step-4.2-steam-integration]]) runs as the app "steam". It is started without
// GAMESCOPE_WAYLAND_DISPLAY: with it, Steam takes over gamescope's control of what is on screen and
// its own window stays hidden ([[owneetd-design]]). A game started by Steam runs inside Steam's
// unit, under Steam's launcher ("reaper SteamLaunch AppId=N -- …"): owneetd lists it as the game
// "steam:N", tags its windows with an app id of its own and closes it without closing Steam.

// SteamID is the id of the Steam app.
const SteamID = "steam"

const steamGamePrefix = "steam:"

// steamUIAppID is the code Steam uses for its own window when it writes what gamescope shows.
const steamUIAppID = 769

// SteamLiveMinRAM is the memory Steam needs in the live system, where its first download (about
// 2.4 GB) is kept in RAM: 12 GB of RAM, as the kernel reports it ([[steam-integration]],
// 2026-10-11).
const SteamLiveMinRAM = 11 << 30

// ErrSteamNeedsInstall: the live system has too little memory for Steam's files.
var ErrSteamNeedsInstall = errors.New("Steam needs the installed system: the live system has too little memory for its files")

// SteamRequest is the body of POST /v1/steam: open a page of Steam, or run a game.
type SteamRequest struct {
	Open string `json:"open,omitempty"` // "library" or "store"
	Run  string `json:"run,omitempty"`  // a Steam app id
	Name string `json:"name,omitempty"` // the game's name (shown while it runs)
}

// SteamStatus is the body of GET /v1/steam.
type SteamStatus struct {
	Installed bool `json:"installed"` // the steam program is there
	Allowed   bool `json:"allowed"`   // false in a live system with too little memory
	Prepared  bool `json:"prepared"`  // Steam has started once (no long first download)
	Running   bool `json:"running"`
	// Games are the Steam app ids installed completely, on every library folder present: when
	// they change (a game installed or removed in Steam) the interface reads its games again
	Games []string `json:"games"`
}

// Procs reads and signals processes (/proc by default).
type Procs interface {
	InUnit(unit string) []int
	Cmdline(pid int) []string
	Parent(pid int) int
	Signal(pid int, sig syscall.Signal) error
}

type procFS struct{}

func (procFS) InUnit(unit string) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var pids []int
	for _, e := range entries {
		if pid, err := strconv.Atoi(e.Name()); err == nil && UnitOfPID(pid) == unit {
			pids = append(pids, pid)
		}
	}
	return pids
}

func (procFS) Cmdline(pid int) []string {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil || len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
}

func (procFS) Parent(pid int) int {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0
	}
	// "PID (name) STATE PPID …": the name may contain spaces and parentheses
	s := string(data)
	fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
	if len(fields) < 2 {
		return 0
	}
	ppid, _ := strconv.Atoi(fields[1])
	return ppid
}

func (procFS) Signal(pid int, sig syscall.Signal) error { return syscall.Kill(pid, sig) }

// steamAllowed: always in the installed system; in the live system (/run/archiso) with enough RAM.
func steamAllowed() bool {
	if _, err := os.Stat("/run/archiso"); err != nil {
		return true
	}
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "MemTotal:"); ok {
			kb, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(rest), " kB"), 10, 64)
			return kb*1024 >= SteamLiveMinRAM
		}
	}
	return false
}

// forward hands a steam:// address to the Steam already running (the command returns at once).
func forward(argv, env []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), env...)
	return cmd.Run()
}

// steamArgv starts the steam program without gamescope's control socket (see above).
func (m *Manager) steamArgv(args ...string) ([]string, error) {
	path, err := m.LookPath("steam")
	if err != nil {
		return nil, fmt.Errorf("%w: steam", ErrNotFound)
	}
	return append([]string{"env", "-u", "GAMESCOPE_WAYLAND_DISPLAY", path}, args...), nil
}

var appIDPattern = regexp.MustCompile(`^[0-9]{1,10}$`)

// Steam opens Steam's library or store, or runs a Steam game; Steam starts if needed.
func (m *Manager) Steam(req SteamRequest) error {
	var url string
	switch {
	case req.Open == "" && appIDPattern.MatchString(req.Run):
		url = "steam://rungameid/" + req.Run
	case req.Run == "" && req.Open == "library":
		url = "steam://open/games"
	case req.Run == "" && req.Open == "store":
		url = "steam://store"
	default:
		return fmt.Errorf("%w: open (library, store) or run (a Steam app id)", ErrInvalid)
	}
	m.mu.Lock()
	m.init()
	allowed := m.SteamAllowed
	m.mu.Unlock()
	if !allowed() {
		return ErrSteamNeedsInstall
	}

	m.mu.Lock()
	if req.Run != "" {
		for _, a := range m.apps {
			if a.Kind == KindGame {
				m.mu.Unlock()
				return ErrGameRunning
			}
		}
		if req.Name != "" {
			m.steamNames[req.Run] = req.Name
		}
	}
	_, running := m.apps[SteamID]
	m.mu.Unlock()

	if !running {
		argv, err := m.steamArgv("-gamepadui", url)
		if err != nil {
			return err
		}
		_, err = m.Launch(LaunchRequest{ID: SteamID, Name: "Steam", Kind: KindApp, Command: argv})
		return err
	}
	sess, err := m.Session()
	if err != nil {
		return ErrNoSession
	}
	argv, err := m.steamArgv(url)
	if err != nil {
		return err
	}
	if err := m.Forward(argv, sess.Env); err != nil {
		return fmt.Errorf("cannot reach Steam: %w", err)
	}
	// Steam's window comes to the front (a game's window takes over once it appears)
	if err := m.FocusApp(SteamID); errors.Is(err, ErrNoWindow) {
		if f := m.focus(sess); f != nil {
			m.show(f, []uint32{m.appIDOf(SteamID), HomeAppID})
		}
	}
	return nil
}

func (m *Manager) appIDOf(id string) uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.apps[id]; ok {
		return a.appID
	}
	return 0
}

// SteamStatus tells the interface what opening Steam will do.
func (m *Manager) SteamStatus() SteamStatus {
	m.mu.Lock()
	m.init()
	_, running := m.apps[SteamID]
	allowed, dir := m.SteamAllowed, m.SteamDir
	m.mu.Unlock()
	_, err := m.LookPath("steam")
	_, prepared := os.Stat(filepath.Join(dir, "ubuntu12_32", "steam"))
	return SteamStatus{Installed: err == nil, Allowed: allowed(), Prepared: prepared == nil, Running: running,
		Games: installedSteamGames(dir)}
}

var manifestFlags = regexp.MustCompile(`(?m)^\s*"StateFlags"\s+"([0-9]+)"`)

// steamLibraries: Steam's folder and the library folders it lists.
func steamLibraries(dir string) []string {
	libraries := []string{dir}
	if data, err := os.ReadFile(filepath.Join(dir, "steamapps", "libraryfolders.vdf")); err == nil {
		for _, match := range libraryPath.FindAllStringSubmatch(string(data), -1) {
			if match[1] != dir {
				libraries = append(libraries, match[1])
			}
		}
	}
	return libraries
}

// installedSteamGames lists the app ids installed completely (StateFlags bit 4), sorted.
func installedSteamGames(dir string) []string {
	ids := []string{}
	seen := map[string]bool{}
	for _, lib := range steamLibraries(dir) {
		files, _ := filepath.Glob(filepath.Join(lib, "steamapps", "appmanifest_*.acf"))
		for _, f := range files {
			id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "appmanifest_"), ".acf")
			data, err := os.ReadFile(f)
			match := manifestFlags.FindSubmatch(data)
			if err != nil || match == nil || seen[id] {
				continue
			}
			if flags, _ := strconv.Atoi(string(match[1])); flags&4 != 0 {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	sort.Strings(ids)
	return ids
}

// reaperAppID returns the Steam app id of Steam's launcher process ("reaper SteamLaunch AppId=N").
func reaperAppID(argv []string) string {
	if len(argv) < 3 || filepath.Base(argv[0]) != "reaper" {
		return ""
	}
	launch := false
	for _, a := range argv[1:] {
		if a == "--" {
			break
		}
		if a == "SteamLaunch" {
			launch = true
		}
		if id, ok := strings.CutPrefix(a, "AppId="); ok && launch && appIDPattern.MatchString(id) {
			return id
		}
	}
	return ""
}

// reaperOf returns the Steam game a process belongs to: its app id and launcher, if any.
func (m *Manager) reaperOf(pid int) (string, int) {
	for i := 0; i < 64 && pid > 1; i++ {
		if id := reaperAppID(m.Procs.Cmdline(pid)); id != "" {
			return id, pid
		}
		pid = m.Procs.Parent(pid)
	}
	return "", 0
}

func (m *Manager) descends(pid, ancestor int) bool {
	for i := 0; i < 64 && pid > 1; i++ {
		pid = m.Procs.Parent(pid)
		if pid == ancestor {
			return true
		}
	}
	return false
}

var manifestName = regexp.MustCompile(`(?m)^\s*"name"\s+"([^"]+)"`)
var libraryPath = regexp.MustCompile(`(?m)^\s*"path"\s+"([^"]+)"`)

// steamGameName: the name the interface gave, else the game's manifest, else "Steam".
func (m *Manager) steamGameName(appid string) string {
	m.mu.Lock()
	name, dir := m.steamNames[appid], m.SteamDir
	m.mu.Unlock()
	if name != "" {
		return name
	}
	for _, lib := range steamLibraries(dir) {
		data, err := os.ReadFile(filepath.Join(lib, "steamapps", "appmanifest_"+appid+".acf"))
		if match := manifestName.FindStringSubmatch(string(data)); err == nil && match != nil {
			return match[1]
		}
	}
	return "Steam"
}

// steamGame returns the running Steam game with this app id, listing it when it is new.
func (m *Manager) steamGame(appid string, reaper int, unit string) *App {
	id := steamGamePrefix + appid
	m.mu.Lock()
	if a, ok := m.apps[id]; ok {
		m.mu.Unlock()
		return a
	}
	m.mu.Unlock()
	name := m.steamGameName(appid)
	m.mu.Lock()
	if a, ok := m.apps[id]; ok { // listed meanwhile
		m.mu.Unlock()
		return a
	}
	app := &App{ID: id, Name: name, Kind: KindGame, Started: time.Now(), unit: unit, appID: m.newAppID(id), reaper: reaper}
	m.apps[id] = app
	m.order = append(m.order, id)
	steam := uint32(0)
	if s, ok := m.apps[SteamID]; ok {
		steam = s.appID
	}
	delete(m.steamNames, appid)
	m.mu.Unlock()

	m.Log.Info("Steam game started", "id", id, "name", name)
	m.Broker.Publish("app.started", map[string]string{"id": id, "name": name, "kind": KindGame})
	if sess, err := m.Session(); err == nil {
		if f := m.focus(sess); f != nil {
			m.show(f, []uint32{app.appID, steam, HomeAppID})
		}
		m.cageFocus(sess)
	}
	return app
}

// scanSteam lists the games running inside Steam and notices the ones that ended.
func (m *Manager) scanSteam() {
	m.mu.Lock()
	m.init()
	unit := ""
	if s, ok := m.apps[SteamID]; ok {
		unit = s.unit
	}
	m.mu.Unlock()
	if unit == "" {
		return
	}
	found := map[string]int{}
	for _, pid := range m.Procs.InUnit(unit) {
		if id := reaperAppID(m.Procs.Cmdline(pid)); id != "" {
			if old, ok := found[id]; !ok || pid < old {
				found[id] = pid
			}
		}
	}
	for appid, reaper := range found {
		m.steamGame(appid, reaper, unit)
	}
	m.mu.Lock()
	var gone []*App
	for _, a := range m.apps {
		if a.reaper != 0 {
			if pid, ok := found[strings.TrimPrefix(a.ID, steamGamePrefix)]; !ok || pid != a.reaper {
				gone = append(gone, a)
			}
		}
	}
	m.mu.Unlock()
	for _, a := range gone {
		m.ended(a, "success")
	}
}

// signalGame signals every process of a Steam game (not Steam itself); with SIGKILL the launcher too.
func (m *Manager) signalGame(a *App, sig syscall.Signal) {
	for _, pid := range m.Procs.InUnit(a.unit) {
		if m.descends(pid, a.reaper) || (sig == syscall.SIGKILL && pid == a.reaper) {
			m.Procs.Signal(pid, sig)
		}
	}
}

// closeSteamGame asks a Steam game to close, and force-closes it after 10 seconds.
func (m *Manager) closeSteamGame(a *App, force bool) {
	if force {
		m.Log.Info("force-closing Steam game", "id", a.ID)
		m.signalGame(a, syscall.SIGKILL)
		return
	}
	m.Log.Info("closing Steam game", "id", a.ID)
	m.signalGame(a, syscall.SIGTERM)
	time.AfterFunc(10*time.Second, func() {
		m.mu.Lock()
		still := m.apps[a.ID] == a
		m.mu.Unlock()
		if still {
			m.signalGame(a, syscall.SIGKILL)
		}
	})
}

// steamRequest: Steam wrote what gamescope should show, in its own codes (769: its own window; a
// Steam app id: that game; others are unknown). While Steam or one of its games is in front,
// owneetd shows what Steam asks for, in owneetd's app ids; otherwise (e.g. the home screen after
// Guide) owneetd puts its own choice back: Steam never takes the screen by itself.
func (m *Manager) steamRequest(ids []uint32) {
	sess, err := m.Session()
	if err != nil {
		return
	}
	f := m.focus(sess)
	if f == nil {
		return
	}
	m.mu.Lock()
	m.init()
	steam, running := m.apps[SteamID]
	shown := append([]uint32(nil), m.shown...)
	steamFront := false
	if running && len(shown) > 0 {
		steamFront = shown[0] == steam.appID
		for _, a := range m.apps {
			steamFront = steamFront || (a.reaper != 0 && a.appID == shown[0])
		}
	}
	var want []uint32
	if steamFront {
		seen := map[uint32]bool{}
		add := func(id uint32) {
			if !seen[id] {
				seen[id] = true
				want = append(want, id)
			}
		}
		for _, id := range ids {
			if id == steamUIAppID {
				add(steam.appID)
			} else if g, ok := m.apps[steamGamePrefix+strconv.FormatUint(uint64(id), 10)]; ok {
				add(g.appID)
			}
		}
		add(steam.appID)
		add(HomeAppID)
	}
	m.mu.Unlock()
	if want == nil {
		if len(shown) == 0 {
			shown = []uint32{HomeAppID}
		}
		m.Log.Debug("something else chose what gamescope shows: put back", "asked", ids, "shown", shown)
		m.show(f, shown)
		return
	}
	m.Log.Debug("Steam chose what gamescope shows", "asked", ids, "shown", want)
	m.show(f, want)
}
