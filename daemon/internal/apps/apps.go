// SPDX-License-Identifier: GPL-3.0-or-later

// Package apps starts, tracks and closes games and apps inside the console session, and handles
// the Guide button (PROJECT_RULES.md section 9.1, docs/daemon-design.md):
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
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// HoldToClose is how long Guide is held to close the game in the cage session.
const HoldToClose = 2 * time.Second

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
}

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
	Start(unit, description string, argv, env []string) error
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
	// Watch tags windows with the app id that tag returns for their process; runs until closed.
	Watch(tag func(pid int) uint32)
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

	mu      sync.Mutex
	apps    map[string]*App // by id
	order   []string        // launch order, oldest first
	lastApp string          // the app shown before the home screen
	hold    *time.Timer     // cage: Guide held
	homePID int             // of the session being watched
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

// tag is the app id of a window's process: the home screen, an app, or 0 (unknown).
func (m *Manager) tag(pid int) uint32 {
	m.mu.Lock()
	home := m.homePID
	m.mu.Unlock()
	if pid == home {
		return HomeAppID
	}
	if m.UnitOf == nil {
		return 0
	}
	unit := m.UnitOf(pid)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.apps {
		if a.unit == unit {
			return a.appID
		}
	}
	return 0
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
		f.Watch(m.tag)
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
		appID: m.newAppID(req.ID)}
	m.apps[app.ID] = app // reserved while starting
	m.order = append(m.order, app.ID)
	m.mu.Unlock()

	// gamescope: the new app comes to the front as soon as it has a window; the home screen
	// stays on screen until then.
	if f := m.focus(sess); f != nil {
		if err := f.Show([]uint32{app.appID, HomeAppID}); err != nil {
			m.Log.Debug("cannot choose what gamescope shows", "err", err)
		}
	}
	if err := m.Units.Start(app.unit, description(app.Kind, app.Name), argv, sess.Env); err != nil {
		m.forget(app.ID)
		return App{}, err
	}
	m.Log.Info("app started", "id", app.ID, "kind", app.Kind, "unit", app.unit)
	m.Broker.Publish("app.started", map[string]string{"id": app.ID, "name": app.Name, "kind": app.Kind})
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
}

func (m *Manager) exited(e Exit) {
	m.mu.Lock()
	var app *App
	for _, a := range m.apps {
		if a.unit == e.Unit {
			app = a
		}
	}
	m.mu.Unlock()
	if app == nil {
		return
	}
	m.forget(app.ID)
	m.Log.Info("app exited", "id", app.ID, "result", e.Result)
	m.Broker.Publish("app.exited", map[string]string{"id": app.ID, "name": app.Name, "result": e.Result})
	if sess, err := m.Session(); err == nil {
		if f := m.focus(sess); f != nil {
			if focused, err := f.Focused(); err == nil && (focused == app.appID || focused == 0) {
				f.Show([]uint32{HomeAppID}) // back to the home screen
			}
		}
	}
}

func (m *Manager) unitOf(id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.init()
	a, ok := m.apps[id]
	if !ok {
		return "", ErrUnknownApp
	}
	return a.unit, nil
}

// Close asks an app to close (SIGTERM to all its processes; SIGKILL after 10 seconds).
func (m *Manager) Close(id string) error {
	unit, err := m.unitOf(id)
	if err != nil {
		return err
	}
	m.Log.Info("closing app", "id", id)
	return m.Units.Stop(unit)
}

// Kill force-closes a frozen app at once.
func (m *Manager) Kill(id string) error {
	unit, err := m.unitOf(id)
	if err != nil {
		return err
	}
	m.Log.Info("force-closing app", "id", id)
	return m.Units.Kill(unit)
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
			if err := f.Show([]uint32{appID, HomeAppID}); err != nil {
				return err
			}
			m.Broker.Publish("focus.changed", map[string]string{"focus": id})
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
		if down {
			m.toggle(sess)
		}
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
		target = ""
		if len(m.order) > 0 {
			target = m.order[len(m.order)-1]
		}
	}
	m.mu.Unlock()

	if shown != nil {
		if err := f.Show([]uint32{HomeAppID}); err != nil {
			m.Log.Warn("cannot show the home screen", "err", err)
			return
		}
		m.Log.Info("Guide: home screen")
		m.Broker.Publish("focus.changed", map[string]string{"focus": "home"})
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
