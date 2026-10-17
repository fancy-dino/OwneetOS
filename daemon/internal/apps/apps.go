// SPDX-License-Identifier: GPL-3.0-or-later

// Package apps starts, tracks and closes games and apps inside the console session, and handles
// the Guide button ([[controller-input-map]], [[owneetd-design]]):
//   - gamescope session: Guide toggles between the home screen and the running game or app; the
//     game keeps running behind the home screen.
//   - cage session (reduced): the home screen cannot be shown over a game, so holding Guide for
//     two seconds closes the game; a short press does nothing.
//
// Each app runs in its own transient systemd user service, so closing it also closes every
// process it started (launchers, Proton…).
package apps

import (
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// HoldToClose is how long Guide is held to close the game in the cage session.
const HoldToClose = 2 * time.Second

// HoldToHome is how long Guide is held, while Steam or a Steam game is on screen, to come back to
// the console: a short press belongs to Steam then (its menu, its overlay in games;
// [[controller-input-map]], 2026-10-12).
const HoldToHome = time.Second

// Kinds of app.
const (
	KindGame = "game"
	KindApp  = "app"
)

// App is a running game or app.
type App struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Kind    string    `json:"kind"`
	Started time.Time `json:"started"`
	unit    string
	appID   uint32 // gamescope app id of its windows
	reaper  int    // a Steam game: Steam's launcher process (steam.go)
	window  bool   // has shown a window
	// settle: shown only once its window has stayed for a few seconds (Steam changes resolution
	// and flashes windows while it starts: the console's "Preparing Steam" screen covers that)
	settle bool
	steady int // checks in a row with a window
	// steamBehind: a Steam game whose window has not appeared yet; Steam's window is left out of
	// what gamescope may show (Silksong, FrankenPC 2026-10-16: Steam's window was on screen for
	// 10 s before the game's), the home screen's "Starting …" covers the wait (steam.go)
	steamBehind bool
}

// settleChecks is how many once-a-second checks a settling app's window must stay before it is shown.
const settleChecks = 3

// HomeAppID tags the home screen's windows in gamescope; apps get ids just above it (see
// newAppID). The range is far from Steam's app ids.
const HomeAppID uint32 = 0x7E000000

// Status is the body of GET /v1/apps.
type Status struct {
	Apps []App `json:"apps"`
	// Focus is what the screen shows: "home", the id of an app, or "" when unknown (cage
	// session, or no console session).
	Focus string `json:"focus"`
}

// LaunchRequest is the body of POST /v1/apps/launch.
type LaunchRequest struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Command []string `json:"command"`
	// Workdir is the folder the program starts in (absolute; the user's home when empty).
	Workdir string `json:"workdir,omitempty"`
	// Stop closes the app cleanly before its processes are stopped (owneetd's own apps only:
	// Steam).
	Stop []string `json:"-"`
}

// Errors returned to the API.
var (
	ErrInvalid          = errors.New("invalid launch request")
	ErrNoSession        = errors.New("the console session is not running")
	ErrAlreadyRunning   = errors.New("this app is already running")
	ErrGameRunning      = errors.New("another game is running: close it first")
	ErrUnknownApp       = errors.New("this app is not running")
	ErrNotFound         = errors.New("program not found")
	ErrFocusUnsupported = errors.New("the reduced (cage) session shows one app at a time")
	ErrNoWindow         = errors.New("the app has no window yet")
)

// Units runs apps as systemd user services.
type Units interface {
	// stop, when not nil, is run to close the app (systemd's ExecStop) before its processes are
	// stopped.
	Start(unit, description string, argv, stop, env []string, workdir string) error
	Stop(unit string) error
	Kill(unit string) error
	// Running lists the app units already running (owneetd restarted): unit and description.
	Running() (map[string]string, error)
	// Exited reports units that stopped, with systemd's result ("success", "exit-code"…).
	Exited() <-chan Exit
}

// Exit is a stopped unit.
type Exit struct {
	Unit   string
	Result string
}

// Focus chooses what gamescope shows (see Gamescope).
type Focus interface {
	// Watch tags windows with the app id that tag returns for their process, and passes on what
	// someone else (Steam) asks gamescope to show; runs until closed.
	Watch(tag func(pid int) uint32, request func(appIDs []uint32))
	// Show shows the first of these apps that has a window.
	Show(appIDs []uint32) error
	// Focused returns the app id on screen (0: none).
	Focused() (uint32, error)
	// Visible returns the app ids that have a window.
	Visible() ([]uint32, error)
}

// Session is the running console session (from owneet-session-app).
type Session struct {
	Mode    string   // gamescope or cage
	Env     []string // DISPLAY=…, WAYLAND_DISPLAY=… for the apps
	Display string   // X11 display (gamescope)
	HomePID int      // process of the home screen
}

// Manager keeps the running apps.
type Manager struct {
	Units  Units
	Broker *events.Broker
	Log    *slog.Logger
	// Session reads the current console session.
	Session func() (Session, error)
	// FocusFor returns the focus control of a gamescope session (nil for cage).
	FocusFor func(Session) Focus
	// UnitOf returns the systemd unit a process belongs to ("" if none).
	UnitOf func(pid int) string
	// LookPath resolves a program name (exec.LookPath by default).
	LookPath func(string) (string, error)
	// Hold is how long Guide is held to close the app in the cage session (HoldToClose).
	Hold time.Duration
	// HomeHold is how long Guide is held over Steam to come back to the console (HoldToHome).
	HomeHold time.Duration
	// Steam (steam.go): processes, Steam's folder, whether Steam may run, how an address
	// reaches the running Steam.
	Procs        Procs
	SteamDir     string
	StateDir     string // owneetd's own marks (~/.config/owneet): Steam Input set once per account
	SteamAllowed func() bool
	Forward      func(argv, env []string) error
	// NVIDIA tells whether the NVIDIA driver runs the screen (Steam then draws its pages
	// without the GPU, see steamGPUArgs).
	NVIDIA func() bool
	// SteamRepaint: keep gamescope composing every frame while Steam's window is on screen
	// (repaint.go; by default on NVIDIA, unless the test switch owneet.debug.steam_repaint=off).
	SteamRepaint func() bool

	mu       sync.Mutex
	apps     map[string]*App // by id
	order    []string        // launch order, oldest first
	lastApp  string          // the app shown before the home screen
	hold     *time.Timer     // cage: Guide held
	homeHold *time.Timer     // gamescope: Guide held over Steam
	homePID  int             // of the session being watched
	// names given by the interface to Steam games about to start, by Steam app id
	steamNames map[string]string
	// the steam:// address to hand to Steam once its window is shown (it started on its own
	// home page, so that Back from that address has somewhere to go)
	steamPending string
	shown        []uint32 // what owneetd last asked gamescope to show
	lastFocus    string   // the last "focus.changed" sent
	// Steam hidden while it starts a game; shown later if it seems to wait for the user (steam.go)
	steamHidden  bool
	steamAsk     *time.Timer
	steamConfirm *steamConfirm // an installation or uninstallation Steam shows (steam.go)
	steamAskFor  string        // the next start of this app id asks the user first (steam.go)

	steamMu      sync.Mutex
	steamSamples map[string]*steamSample // downloads: bytes at the last check (steamlib.go)
	steamLast    string                  // the last status sent as "steam.changed"
	steamRunning bool                    // Steam ran at the last check
}

func (m *Manager) init() {
	if m.apps == nil {
		m.apps = map[string]*App{}
	}
	if m.LookPath == nil {
		m.LookPath = exec.LookPath
	}
	if m.Hold == 0 {
		m.Hold = HoldToClose
	}
	if m.HomeHold == 0 {
		m.HomeHold = HoldToHome
	}
	if m.Procs == nil {
		m.Procs = procFS{}
	}
	if m.SteamDir == "" {
		home, _ := os.UserHomeDir()
		m.SteamDir = filepath.Join(home, ".local", "share", "Steam")
	}
	if m.StateDir == "" {
		home, _ := os.UserHomeDir()
		m.StateDir = filepath.Join(home, ".config", "owneet")
	}
	if m.SteamAllowed == nil {
		m.SteamAllowed = steamAllowed
	}
	if m.Forward == nil {
		m.Forward = forward
	}
	if m.NVIDIA == nil {
		m.NVIDIA = nvidiaDriver
	}
	if m.SteamRepaint == nil {
		m.SteamRepaint = m.steamRepaint
	}
	if m.steamNames == nil {
		m.steamNames = map[string]string{}
	}
}

// newAppID returns the gamescope app id of an app: derived from its id, so it stays the same
// when owneetd restarts (gamescope keeps showing it), and different from the running apps' ids.
// Callers hold m.mu.
func (m *Manager) newAppID(id string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(id))
	candidate := HomeAppID + 1 + h.Sum32()&0x00FFFFFF
	for {
		if m.appOfID(candidate) == nil && candidate != HomeAppID {
			return candidate
		}
		candidate++
	}
}

// tag is the app id of a window's process: the home screen, an app, or 0 (unknown). Inside
// Steam, the windows of a game get the game's app id.
func (m *Manager) tag(pid int) uint32 {
	m.mu.Lock()
	m.init()
	home := m.homePID
	steamUnit := ""
	if s, ok := m.apps[SteamID]; ok {
		steamUnit = s.unit
	}
	m.mu.Unlock()
	if pid == home {
		return HomeAppID
	}
	if m.UnitOf == nil {
		return 0
	}
	unit := m.UnitOf(pid)
	if unit == "" {
		return 0
	}
	if unit == steamUnit {
		if appid, reaper := m.reaperOf(pid); appid != "" {
			return m.steamGame(appid, reaper, unit).appID
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.apps {
		if a.unit == unit && a.reaper == 0 {
			return a.appID
		}
	}
	return 0
}

// focusChanged sends "focus.changed" when what is on screen changed: the interface reads the
// controllers only while it is on screen.
func (m *Manager) focusChanged(id string) {
	m.updateRepaint(id) // NVIDIA test: gamescope composing while Steam's window is on screen
	m.mu.Lock()
	same := id == m.lastFocus
	m.lastFocus = id
	m.mu.Unlock()
	if !same {
		m.Broker.Publish("focus.changed", map[string]string{"focus": id})
	}
}

// checkWindows sends "app.window" when gamescope can show an app's window for the first time
// (e.g. Steam ready after its first download; its early helper windows are never shown), and
// "focus.changed" when what is on screen changed by itself (e.g. an app's window appeared).
func (m *Manager) checkWindows() {
	sess, err := m.Session()
	if err != nil {
		return
	}
	f := m.focus(sess)
	if f == nil {
		return
	}
	visible, err := f.Visible()
	if err != nil {
		return
	}
	shown := map[uint32]bool{}
	for _, v := range visible {
		shown[v] = true
	}
	m.mu.Lock()
	var ready []string
	var bring []uint32  // settled apps to bring to the front
	var behind []uint32 // Steam games whose window came: Steam back behind them
	pending := ""       // Steam's first page, once Steam is shown
	for _, a := range m.apps {
		if a.window {
			continue
		}
		if !shown[a.appID] {
			a.steady = 0
			continue
		}
		if a.steamBehind {
			a.steamBehind = false
			if s, ok := m.apps[SteamID]; ok && len(m.shown) > 0 && m.shown[0] == a.appID {
				behind = append(behind, a.appID, s.appID)
			}
		}
		if a.settle {
			if a.steady++; a.steady < settleChecks {
				continue
			}
			// still wanted: nothing else was chosen meanwhile; Steam starting a game stays hidden
			if (len(m.shown) == 0 || m.shown[0] == HomeAppID) && !(a.ID == SteamID && m.steamHidden) {
				bring = append(bring, a.appID)
			}
		}
		a.window = true
		ready = append(ready, a.ID)
		if a.ID == SteamID {
			pending, m.steamPending = m.steamPending, ""
		}
	}
	m.mu.Unlock()
	for _, id := range bring {
		m.show(f, []uint32{id, HomeAppID})
	}
	if len(behind) == 2 {
		m.show(f, []uint32{behind[0], behind[1], HomeAppID})
	}
	if pending != "" {
		m.forwardSteam(pending, sess)
		m.mu.Lock()
		hidden := m.steamHidden
		m.mu.Unlock()
		if hidden {
			m.steamAskLater()
		}
	}
	for _, id := range ready {
		m.Broker.Publish("app.window", map[string]string{"id": id})
	}
	focused, err := f.Focused()
	if err != nil || focused == 0 {
		return
	}
	m.mu.Lock()
	id := ""
	if focused == HomeAppID {
		id = "home"
	} else if a := m.appOfID(focused); a != nil {
		id = a.ID
	}
	m.mu.Unlock()
	if id != "" {
		m.focusChanged(id)
	}
}

// Run adopts apps still running from a previous owneetd and tracks exits until stop is closed.
func (m *Manager) Run(stop <-chan struct{}) {
	m.mu.Lock()
	m.init()
	m.mu.Unlock()
	if running, err := m.Units.Running(); err == nil {
		for unit, desc := range running {
			m.adopt(unit, desc)
		}
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	m.watchSession()
	for {
		select {
		case <-stop:
			return
		case e := <-m.Units.Exited():
			m.exited(e)
		case <-tick.C:
			m.watchSession()
			m.scanSteam()
			m.checkWindows()
			m.watchSteam()
		}
	}
}

// watchSession starts tagging the windows of a (new) gamescope session.
func (m *Manager) watchSession() {
	sess, err := m.Session()
	if err != nil {
		return
	}
	m.mu.Lock()
	m.homePID = sess.HomePID
	m.mu.Unlock()
	if f := m.focus(sess); f != nil {
		f.Watch(m.tag, m.steamRequest)
	}
}

// UnitName is the systemd unit of an app id: owneet-app-ID.service, escaped like systemd-escape.
func UnitName(id string) string {
	var b strings.Builder
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c == '/':
			b.WriteByte('-')
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == ':', c == '_',
			c == '.' && i > 0:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, `\x%02x`, c)
		}
	}
	return "owneet-app-" + b.String() + ".service"
}

// description is the unit description; adopt reads name and kind back from it.
func description(kind, name string) string { return "OwneetOS " + kind + ": " + name }

func (m *Manager) adopt(unit, desc string) {
	rest, ok := strings.CutPrefix(desc, "OwneetOS ")
	if !ok {
		return
	}
	kind, name, ok := strings.Cut(rest, ": ")
	if !ok {
		return
	}
	id := unescapeUnit(strings.TrimSuffix(strings.TrimPrefix(unit, "owneet-app-"), ".service"))
	m.mu.Lock()
	defer m.mu.Unlock()
	m.apps[id] = &App{ID: id, Name: name, Kind: kind, Started: time.Now(), unit: unit, appID: m.newAppID(id)}
	m.order = append(m.order, id)
	m.Log.Info("app still running from before", "id", id)
}

func unescapeUnit(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '-':
			b.WriteByte('/')
		case s[i] == '\\' && i+3 < len(s) && s[i+1] == 'x':
			var c byte
			if _, err := fmt.Sscanf(s[i+2:i+4], "%02x", &c); err == nil {
				b.WriteByte(c)
				i += 3
				continue
			}
			b.WriteByte(s[i])
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func validID(id string) bool {
	if id == "" || len(id) > 100 {
		return false
	}
	for _, r := range id {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// Launch starts an app in the console session.
func (m *Manager) Launch(req LaunchRequest) (App, error) {
	if !validID(req.ID) || len(req.Command) == 0 || req.Command[0] == "" {
		return App{}, fmt.Errorf("%w: id (1-100 characters) and command are required", ErrInvalid)
	}
	if req.Kind == "" {
		req.Kind = KindApp
	}
	if req.Kind != KindGame && req.Kind != KindApp {
		return App{}, fmt.Errorf("%w: kind is %q or %q", ErrInvalid, KindGame, KindApp)
	}
	if req.Name == "" {
		req.Name = req.ID
	}
	if req.Workdir != "" {
		if st, err := os.Stat(req.Workdir); !filepath.IsAbs(req.Workdir) || err != nil || !st.IsDir() {
			return App{}, fmt.Errorf("%w: workdir must be an existing absolute folder", ErrInvalid)
		}
	}
	sess, err := m.Session()
	if err != nil {
		return App{}, ErrNoSession
	}
	argv := append([]string(nil), req.Command...)
	if argv[0], err = m.LookPath(argv[0]); err != nil {
		return App{}, fmt.Errorf("%w: %s", ErrNotFound, req.Command[0])
	}

	m.mu.Lock()
	m.init()
	if _, ok := m.apps[req.ID]; ok {
		m.mu.Unlock()
		return App{}, ErrAlreadyRunning
	}
	if req.Kind == KindGame {
		for _, a := range m.apps {
			if a.Kind == KindGame {
				m.mu.Unlock()
				return App{}, ErrGameRunning
			}
		}
	}
	app := &App{ID: req.ID, Name: req.Name, Kind: req.Kind, Started: time.Now(), unit: UnitName(req.ID),
		appID: m.newAppID(req.ID), settle: req.ID == SteamID}
	m.apps[app.ID] = app // reserved while starting
	m.order = append(m.order, app.ID)
	m.mu.Unlock()

	// gamescope: the new app comes to the front as soon as it has a window (a settling one once
	// its window is steady: checkWindows); the home screen stays on screen until then.
	if f := m.focus(sess); f != nil {
		first := []uint32{app.appID, HomeAppID}
		if app.settle {
			first = []uint32{HomeAppID}
		}
		if err := m.show(f, first); err != nil {
			m.Log.Debug("cannot choose what gamescope shows", "err", err)
		}
	}
	if err := m.Units.Start(app.unit, description(app.Kind, app.Name), argv, req.Stop, sess.Env, req.Workdir); err != nil {
		m.forget(app.ID)
		return App{}, err
	}
	m.Log.Info("app started", "id", app.ID, "kind", app.Kind, "unit", app.unit)
	m.Broker.Publish("app.started", map[string]string{"id": app.ID, "name": app.Name, "kind": app.Kind})
	m.cageFocus(sess)
	return *app, nil
}

func (m *Manager) forget(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.apps, id)
	for i, o := range m.order {
		if o == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	if m.lastApp == id {
		m.lastApp = ""
	}
	if id == SteamID {
		m.steamPending = ""
		m.steamHidden = false
		m.steamConfirm = nil
		if m.steamAsk != nil {
			m.steamAsk.Stop()
			m.steamAsk = nil
		}
	}
}

// exited: a unit stopped, with every app in it (Steam and the game running inside it).
func (m *Manager) exited(e Exit) {
	m.mu.Lock()
	var gone []*App
	for _, id := range m.order {
		if a := m.apps[id]; a.unit == e.Unit {
			gone = append([]*App{a}, gone...) // the newest first: a Steam game before Steam
		}
	}
	m.mu.Unlock()
	for _, a := range gone {
		m.ended(a, e.Result)
	}
}

// ended forgets an app that stopped and goes back to the home screen if it was on screen.
func (m *Manager) ended(app *App, result string) {
	m.forget(app.ID)
	m.Log.Info("app exited", "id", app.ID, "result", result)
	m.Broker.Publish("app.exited", map[string]string{"id": app.ID, "name": app.Name, "result": result})
	if sess, err := m.Session(); err == nil {
		if f := m.focus(sess); f != nil {
			if focused, err := f.Focused(); err == nil && (focused == app.appID || focused == 0) {
				m.show(f, []uint32{HomeAppID}) // back to the home screen
			}
		}
		m.cageFocus(sess)
	}
}

// cageFocus: the reduced (cage) session shows the newest window, and owneetd cannot read it: the
// newest app is taken to be on screen, the home screen when none is left.
func (m *Manager) cageFocus(sess Session) {
	if sess.Mode != "cage" {
		return
	}
	m.mu.Lock()
	id := "home"
	if len(m.order) > 0 {
		id = m.order[len(m.order)-1]
	}
	m.mu.Unlock()
	m.focusChanged(id)
}

func (m *Manager) appOf(id string) (*App, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.init()
	a, ok := m.apps[id]
	if !ok {
		return nil, ErrUnknownApp
	}
	return a, nil
}

// Close asks an app to close (SIGTERM to all its processes; SIGKILL after 10 seconds). A Steam
// game closes alone: Steam keeps running.
func (m *Manager) Close(id string) error {
	a, err := m.appOf(id)
	if err != nil {
		return err
	}
	if a.reaper != 0 {
		m.closeSteamGame(a, false)
		return nil
	}
	m.Log.Info("closing app", "id", id)
	return m.Units.Stop(a.unit)
}

// CloseAll closes every app and waits until all have ended, at most `wait` (before shutting
// down or restarting: Steam stopped together with the console session cannot close itself, and
// then checks all its files at the next start).
func (m *Manager) CloseAll(wait time.Duration) {
	m.mu.Lock()
	m.init()
	units := map[string]bool{} // a Steam game runs in Steam's unit
	for _, a := range m.apps {
		units[a.unit] = true
	}
	m.mu.Unlock()
	for unit := range units {
		m.Log.Info("closing app before power off", "unit", unit)
		if err := m.Units.Stop(unit); err != nil {
			m.Log.Warn("cannot close app", "unit", unit, "err", err)
		}
	}
	deadline := time.Now().Add(wait)
	for {
		m.mu.Lock()
		left := len(m.apps)
		m.mu.Unlock()
		if left == 0 || !time.Now().Before(deadline) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Kill force-closes a frozen app at once.
func (m *Manager) Kill(id string) error {
	a, err := m.appOf(id)
	if err != nil {
		return err
	}
	if a.reaper != 0 {
		m.closeSteamGame(a, true)
		return nil
	}
	m.Log.Info("force-closing app", "id", id)
	return m.Units.Kill(a.unit)
}

// show asks gamescope to show the first of these apps that has a window, and remembers it.
func (m *Manager) show(f Focus, appIDs []uint32) error {
	m.mu.Lock()
	m.shown = append([]uint32(nil), appIDs...)
	m.mu.Unlock()
	return f.Show(appIDs)
}

func (m *Manager) focus(s Session) Focus {
	if s.Mode != "gamescope" || m.FocusFor == nil {
		return nil
	}
	return m.FocusFor(s)
}

// appOfID returns the app with a gamescope app id; callers hold m.mu.
func (m *Manager) appOfID(appID uint32) *App {
	for _, a := range m.apps {
		if a.appID == appID {
			return a
		}
	}
	return nil
}

// Status lists the running apps and what the screen shows.
func (m *Manager) Status() Status {
	m.mu.Lock()
	m.init()
	st := Status{Apps: []App{}}
	for _, id := range m.order {
		st.Apps = append(st.Apps, *m.apps[id])
	}
	m.mu.Unlock()
	sort.SliceStable(st.Apps, func(i, j int) bool { return st.Apps[i].Started.Before(st.Apps[j].Started) })
	sess, err := m.Session()
	if err != nil {
		return st
	}
	f := m.focus(sess)
	if f == nil {
		return st
	}
	focused, err := f.Focused()
	if err != nil {
		return st
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if focused == HomeAppID {
		st.Focus = "home"
	} else if a := m.appOfID(focused); a != nil {
		st.Focus = a.ID
	}
	return st
}

// FocusApp brings an app to the front (gamescope only), e.g. "Resume" on the home screen.
func (m *Manager) FocusApp(id string) error {
	m.mu.Lock()
	m.init()
	a, ok := m.apps[id]
	var appID uint32
	if ok {
		appID = a.appID
	}
	m.mu.Unlock()
	if !ok {
		return ErrUnknownApp
	}
	sess, err := m.Session()
	if err != nil {
		return ErrNoSession
	}
	f := m.focus(sess)
	if f == nil {
		return ErrFocusUnsupported
	}
	visible, err := f.Visible()
	if err != nil {
		return err
	}
	for _, v := range visible {
		if v == appID {
			if err := m.show(f, []uint32{appID, HomeAppID}); err != nil {
				return err
			}
			m.focusChanged(id)
			return nil
		}
	}
	return ErrNoWindow
}

// Guide handles the Guide button (from any controller).
func (m *Manager) Guide(down bool) {
	sess, err := m.Session()
	if err != nil {
		return
	}
	switch sess.Mode {
	case "gamescope":
		m.mu.Lock()
		m.init()
		if m.homeHold != nil {
			m.homeHold.Stop()
			m.homeHold = nil
		}
		m.mu.Unlock()
		if !down {
			return
		}
		if m.steamOnScreen(sess) {
			// Steam answers the short press itself; held, Guide comes back to the console
			m.mu.Lock()
			m.homeHold = time.AfterFunc(m.HomeHold, func() { m.heldToHome(sess) })
			m.mu.Unlock()
			return
		}
		m.toggle(sess)
	case "cage":
		m.mu.Lock()
		m.init()
		if m.hold != nil {
			m.hold.Stop()
			m.hold = nil
		}
		if down {
			m.hold = time.AfterFunc(m.Hold, m.closeNewest)
		}
		m.mu.Unlock()
	}
}

// steamOnScreen: Steam or one of its games is on screen.
func (m *Manager) steamOnScreen(sess Session) bool {
	f := m.focus(sess)
	if f == nil {
		return false
	}
	focused, err := f.Focused()
	if err != nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.appOfID(focused)
	return a != nil && (a.ID == SteamID || a.reaper != 0)
}

// heldToHome: Guide held over Steam: the console's home screen; Steam and its game keep running.
func (m *Manager) heldToHome(sess Session) {
	m.mu.Lock()
	m.homeHold = nil
	m.mu.Unlock()
	if !m.steamOnScreen(sess) {
		return
	}
	m.Log.Info("Guide held over Steam: home screen")
	m.toggle(sess)
}

// closeNewest closes the app on screen in the cage session (the newest one).
func (m *Manager) closeNewest() {
	m.mu.Lock()
	m.hold = nil
	id := ""
	if len(m.order) > 0 {
		id = m.order[len(m.order)-1]
	}
	m.mu.Unlock()
	if id == "" {
		return
	}
	m.Log.Info("Guide held: closing the app on screen", "id", id)
	if err := m.Close(id); err != nil {
		m.Log.Warn("cannot close the app", "id", id, "err", err)
	}
}

// toggle switches between the home screen and the app last shown.
func (m *Manager) toggle(sess Session) {
	f := m.focus(sess)
	if f == nil {
		return
	}
	focused, err := f.Focused()
	if err != nil {
		m.Log.Warn("cannot read what gamescope shows", "err", err)
		return
	}
	m.mu.Lock()
	m.init()
	shown := m.appOfID(focused)
	if shown != nil { // an app is on screen: show the home screen
		m.lastApp = shown.ID
	}
	target := m.lastApp
	if _, ok := m.apps[target]; !ok {
		// the newest app; never Steam's own window, which stays hidden unless it was on screen
		// ([[step-4.2c-steam-without-its-interface]])
		target = ""
		for i := len(m.order) - 1; i >= 0 && target == ""; i-- {
			if m.order[i] != SteamID {
				target = m.order[i]
			}
		}
	}
	m.mu.Unlock()

	if shown != nil {
		if err := m.show(f, []uint32{HomeAppID}); err != nil {
			m.Log.Warn("cannot show the home screen", "err", err)
			return
		}
		m.Log.Info("Guide: home screen")
		m.focusChanged("home")
		return
	}
	if target == "" { // home screen, nothing running
		return
	}
	if err := m.FocusApp(target); err != nil {
		m.Log.Debug("cannot go back to the app", "id", target, "err", err)
		return
	}
	m.Log.Info("Guide: back to the app", "id", target)
}
