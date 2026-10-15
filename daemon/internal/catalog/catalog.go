// SPDX-License-Identifier: GPL-3.0-or-later

// Package catalog installs and removes the optional apps ([[step-5.0.0-optional-web-apps]]).
//
// The catalogue (/usr/share/owneet/apps.json, root's) lists the web apps the user sees (Netflix,
// YouTube, Spotify) and the engines they need (Brave), which the user never sees. A web app is
// installed when the user chose it (a list kept by owneetd) and its engines are installed. The
// first web app installs its engine, the last one removes it: owneetd starts the root units
// owneet-apps-install@ENGINE / owneet-apps-remove@ENGINE.service (a polkit rule allows only these
// to the console user), which install from the ISO's bundled repository and write their result to
// /run/owneet/apps.json. Every change is sent as "apps.catalog.changed".
package catalog

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
	"github.com/fancy-dino/OwneetOS/daemon/internal/atomicfile"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// Kinds of catalogue entries.
const (
	KindEngine = "engine" // hidden: installed with the first app that needs it
	KindWebApp = "webapp" // shown on the home screen's Apps grid
)

// Errors returned to the API.
var (
	ErrUnknown      = errors.New("no such app in the catalogue")
	ErrBusy         = errors.New("an app is being installed or removed")
	ErrNeedsInstall = errors.New("the live system has too little memory for apps: install OwneetOS")
	ErrNoSpace      = errors.New("not enough free space for the app")
)

// Entry is an app of the catalogue file.
type Entry struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	NameKey  string   `json:"nameKey,omitempty"`
	Icon     string   `json:"icon,omitempty"`
	Packages []string `json:"packages,omitempty"`
	Requires []string `json:"requires,omitempty"`
	SizeMiB  int64    `json:"sizeMiB,omitempty"`
	URL      string   `json:"url,omitempty"`
}

// App is a web app as the interface shows it.
type App struct {
	ID        string `json:"id"`
	NameKey   string `json:"nameKey"`
	Icon      string `json:"icon"`
	URL       string `json:"url"`
	Installed bool   `json:"installed"`
	// Busy is "install" or "remove" while that runs
	Busy string `json:"busy,omitempty"`
	// SizeMiB is the space installing it takes now: its engines that are not installed yet
	SizeMiB int64 `json:"sizeMiB"`
}

// Result is the outcome of an installation or a removal, sent with "apps.catalog.changed".
type Result struct {
	ID     string `json:"id"`
	Action string `json:"action"` // install, remove
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"` // apps.failed
}

// Status is the body of GET /v1/apps/catalog and of "apps.catalog.changed".
type Status struct {
	Apps []App `json:"apps"`
	// Allowed is false in a live system with too little memory ([[optional-apps]])
	Allowed bool    `json:"allowed"`
	Result  *Result `json:"result,omitempty"`
}

// Starter starts a systemd unit of the system manager.
type Starter interface {
	Start(unit string) error
}

// helperState is what owneet-apps writes to the state file.
type helperState struct {
	State  string `json:"state"` // installing, removing, done, failed
	Action string `json:"action"`
	ID     string `json:"id"`
}

// Manager keeps the user's web apps and installs or removes their engines.
type Manager struct {
	Broker      *events.Broker
	Log         *slog.Logger
	Units       Starter
	CatalogFile string // /usr/share/owneet/apps.json
	StateFile   string // /run/owneet/apps.json, written by owneet-apps
	ChosenFile  string // the web apps the user installed (owneetd's state folder)
	PacmanDB    string // /var/lib/pacman/local: the installed packages
	// Allowed: false in a live system with too little memory (live.Allowed)
	Allowed func() bool
	// FreeMiB is the free space of the system disk (nil: not checked)
	FreeMiB func() int64
	Poll    time.Duration
	Timeout time.Duration
	// Opening the web apps (open.go, [[step-5.1-brave-in-the-os]]): the running apps, the console
	// session, the web apps' profiles and the console's browser extension
	Apps         Launcher
	Session      func() (apps.Session, error)
	ProfilesDir  string // $XDG_DATA_HOME/owneet/webapps: one folder per web app
	ExtensionDir string // /usr/share/owneet/extension
	BridgePath   string // /usr/lib/owneet/owneet-bridge (the extension's way to owneetd)

	mu         sync.Mutex
	busyID     string
	busyAction string
}

// spaceMargin is kept free beyond an engine's own size (pacman's database, logs).
const spaceMargin = 100

func (m *Manager) load() ([]Entry, error) {
	data, err := os.ReadFile(m.CatalogFile)
	if err != nil {
		return nil, err
	}
	var f struct {
		Apps []Entry `json:"apps"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return f.Apps, nil
}

func find(cat []Entry, id, kind string) (Entry, bool) {
	for _, e := range cat {
		if e.ID == id && e.Kind == kind {
			return e, true
		}
	}
	return Entry{}, false
}

// installedPackages are the names of the packages in pacman's local database ("NAME-VER-REL").
func (m *Manager) installedPackages() map[string]bool {
	entries, err := os.ReadDir(m.PacmanDB)
	names := map[string]bool{}
	if err != nil {
		return names
	}
	for _, e := range entries {
		parts := strings.Split(e.Name(), "-")
		if e.IsDir() && len(parts) >= 3 {
			names[strings.Join(parts[:len(parts)-2], "-")] = true
		}
	}
	return names
}

func engineInstalled(e Entry, pkgs map[string]bool) bool {
	for _, p := range e.Packages {
		if !pkgs[p] {
			return false
		}
	}
	return len(e.Packages) > 0
}

// missingEngines are the engines app needs that are not installed.
func missingEngines(cat []Entry, app Entry, pkgs map[string]bool) []Entry {
	var missing []Entry
	for _, id := range app.Requires {
		if e, ok := find(cat, id, KindEngine); ok && !engineInstalled(e, pkgs) {
			missing = append(missing, e)
		}
	}
	return missing
}

func (m *Manager) chosen() []string {
	var f struct {
		WebApps []string `json:"webapps"`
	}
	if data, err := os.ReadFile(m.ChosenFile); err == nil {
		_ = json.Unmarshal(data, &f)
	}
	return f.WebApps
}

func (m *Manager) saveChosen(ids []string) error {
	if ids == nil {
		ids = []string{}
	}
	data, err := json.Marshal(struct {
		WebApps []string `json:"webapps"`
	}{ids})
	if err != nil {
		return err
	}
	// flushed to the disk: a power cut must not lose the user's apps
	return atomicfile.Write(m.ChosenFile, append(data, '\n'), 0o644)
}

func (m *Manager) allowed() bool { return m.Allowed == nil || m.Allowed() }

// Status lists the web apps of the catalogue.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status()
}

func (m *Manager) status() Status {
	st := Status{Apps: []App{}, Allowed: m.allowed()}
	cat, err := m.load()
	if err != nil {
		return st
	}
	pkgs, chosen := m.installedPackages(), m.chosen()
	for _, e := range cat {
		if e.Kind != KindWebApp {
			continue
		}
		missing := missingEngines(cat, e, pkgs)
		app := App{ID: e.ID, NameKey: e.NameKey, Icon: e.Icon, URL: e.URL,
			Installed: slices.Contains(chosen, e.ID) && len(missing) == 0}
		for _, eng := range missing {
			app.SizeMiB += eng.SizeMiB
		}
		if m.busyID == e.ID {
			app.Busy = m.busyAction
		}
		st.Apps = append(st.Apps, app)
	}
	return st
}

func (m *Manager) publish(result *Result) {
	if m.Broker == nil {
		return
	}
	m.mu.Lock()
	st := m.status()
	m.mu.Unlock()
	st.Result = result
	m.Broker.Publish("apps.catalog.changed", st)
}

// Install installs the web app id: at once when its engines are there, otherwise after them
// (202: the outcome comes with "apps.catalog.changed").
func (m *Manager) Install(id string) error {
	m.mu.Lock()
	cat, err := m.load()
	if err != nil {
		m.mu.Unlock()
		return err
	}
	app, ok := find(cat, id, KindWebApp)
	if !ok {
		m.mu.Unlock()
		return ErrUnknown
	}
	if m.busyID != "" {
		m.mu.Unlock()
		return ErrBusy
	}
	missing := missingEngines(cat, app, m.installedPackages())
	if len(missing) == 0 {
		chosen := m.chosen()
		if !slices.Contains(chosen, id) {
			chosen = append(chosen, id)
		}
		err := m.saveChosen(chosen)
		m.mu.Unlock()
		if err != nil {
			return err
		}
		m.publish(&Result{ID: id, Action: "install", OK: true})
		return nil
	}
	if !m.allowed() {
		m.mu.Unlock()
		return ErrNeedsInstall
	}
	var need int64
	for _, e := range missing {
		need += e.SizeMiB
	}
	if m.FreeMiB != nil && m.FreeMiB() < need+spaceMargin {
		m.mu.Unlock()
		return ErrNoSpace
	}
	m.busyID, m.busyAction = id, "install"
	m.mu.Unlock()
	m.publish(nil)
	go m.run(id, "install", missing)
	return nil
}

// Remove removes the web app id (closed first if it is open, its data deleted), and the engines
// no other installed web app needs.
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	cat, err := m.load()
	if err != nil {
		m.mu.Unlock()
		return err
	}
	if _, ok := find(cat, id, KindWebApp); !ok {
		m.mu.Unlock()
		return ErrUnknown
	}
	if m.busyID != "" {
		m.mu.Unlock()
		return ErrBusy
	}
	m.mu.Unlock()
	// outside the lock: closing the web app can take a few seconds
	m.closeAndForget(id)
	m.mu.Lock()
	if m.busyID != "" {
		m.mu.Unlock()
		return ErrBusy
	}
	chosen := slices.DeleteFunc(m.chosen(), func(c string) bool { return c == id })
	if err := m.saveChosen(chosen); err != nil {
		m.mu.Unlock()
		return err
	}
	// engines still needed by another chosen web app stay
	needed := map[string]bool{}
	for _, c := range chosen {
		if app, ok := find(cat, c, KindWebApp); ok {
			for _, r := range app.Requires {
				needed[r] = true
			}
		}
	}
	pkgs := m.installedPackages()
	var unused []Entry
	for _, e := range cat {
		if e.Kind == KindEngine && !needed[e.ID] && engineInstalled(e, pkgs) {
			unused = append(unused, e)
		}
	}
	if len(unused) == 0 {
		m.mu.Unlock()
		m.publish(&Result{ID: id, Action: "remove", OK: true})
		return nil
	}
	m.busyID, m.busyAction = id, "remove"
	m.mu.Unlock()
	m.publish(nil)
	go m.run(id, "remove", unused)
	return nil
}

// run installs or removes the engines one after the other, then tells the outcome.
func (m *Manager) run(id, action string, engines []Entry) {
	ok := true
	for _, e := range engines {
		_, before := m.readState()
		unit := "owneet-apps-" + action + "@" + e.ID + ".service"
		if err := m.Units.Start(unit); err != nil {
			m.Log.Warn("optional app: unit not started", "unit", unit, "err", err)
			ok = false
			break
		}
		m.Log.Info("optional app", "action", action, "app", id, "engine", e.ID)
		if st := m.wait(before, e.ID); st.State != "done" {
			m.Log.Warn("optional app failed", "action", action, "engine", e.ID, "state", st.State)
			ok = false
			break
		}
	}
	m.mu.Lock()
	if action == "install" && ok {
		chosen := m.chosen()
		if !slices.Contains(chosen, id) {
			if err := m.saveChosen(append(chosen, id)); err != nil {
				m.Log.Warn("optional apps: list not saved", "err", err)
				ok = false
			}
		}
	}
	m.busyID, m.busyAction = "", ""
	m.mu.Unlock()
	result := &Result{ID: id, Action: action, OK: ok}
	if action == "remove" {
		// The web app is gone for the user either way; an engine left behind only takes space
		result.OK = true
	} else if !ok {
		result.Error = "apps.failed"
	}
	m.publish(result)
}

func (m *Manager) readState() (helperState, time.Time) {
	var st helperState
	info, err := os.Stat(m.StateFile)
	if err != nil {
		return st, time.Time{}
	}
	if data, err := os.ReadFile(m.StateFile); err == nil {
		_ = json.Unmarshal(data, &st)
	}
	return st, info.ModTime()
}

// wait returns the final state owneet-apps writes for the engine, or "failed" after the timeout.
func (m *Manager) wait(before time.Time, engine string) helperState {
	poll, timeout := m.Poll, m.Timeout
	if poll == 0 {
		poll = 300 * time.Millisecond
	}
	if timeout == 0 {
		timeout = 15 * time.Minute
	}
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(poll) {
		st, written := m.readState()
		if written.After(before) && st.ID == engine && (st.State == "done" || st.State == "failed") {
			return st
		}
	}
	return helperState{State: "failed", ID: engine}
}

// DiskFreeMiB is the free space of the file system holding path, for an unprivileged user.
func DiskFreeMiB(path string) int64 {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return 0
	}
	return int64(fs.Bavail) * fs.Bsize >> 20
}
