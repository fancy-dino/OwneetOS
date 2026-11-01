// SPDX-License-Identifier: GPL-3.0-or-later

// Package sysupdate is Settings → System's updates of an installed OwneetOS ([[step-7.1-updates]],
// [[distribution-and-updates]]): checking, installing when the player asks, going back to the
// previous version, and the notice after the console went back by itself. The work is done by root
// units that polkit allows to the console user (owneet-updates-check / -apply.service,
// owneet-rollback-now.service); they write their state to /run/owneet/system-update.json and
// /run/owneet/rollback.json, which owneetd follows and sends as "system-update.changed". Testers'
// PCs update from a USB stick instead (package update).
package sysupdate

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/atomicfile"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// The root units owneetd starts.
const (
	CheckUnit = "owneet-updates-check.service"
	ApplyUnit = "owneet-updates-apply.service"
	BackUnit  = "owneet-rollback-now.service"
)

// Errors returned to the API.
var (
	ErrNotInstalled = errors.New("updates need a system installed by the OwneetOS installer")
	ErrBusy         = errors.New("an update is already being checked, installed or undone")
	ErrNothing      = errors.New("there is no update to install")
	ErrOnPrevious   = errors.New("the console was started from the previous version")
	ErrNoPrevious   = errors.New("there is no previous version")
)

// Package is an OwneetOS or Arch Linux package the update changes ("Details").
type Package struct {
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Previous is the spare copy: the version "Go back to the previous version" starts.
type Previous struct {
	Version string `json:"version"`
	Since   string `json:"since"` // when it was kept (RFC 3339)
}

// Status is the body of GET /v1/system-update.
type Status struct {
	// Installed: installed by the OwneetOS installer, with the rollback the updates rely on
	Installed bool `json:"installed"`
	// State: "" (not checked since the start), none (up to date), available, downloading,
	// installing, ready (restart to finish), failed
	State string `json:"state"`
	// Error (when failed): offline, not_signed, not_installed, on_previous, download_failed,
	// install_failed
	Error   string `json:"error,omitempty"`
	Current string `json:"current"`
	// The newer version, its download size in bytes and its notes per language
	Version  string            `json:"version,omitempty"`
	Size     int64             `json:"size"`
	Notes    map[string]string `json:"notes,omitempty"`
	Packages []Package         `json:"packages"`
	Percent  int               `json:"percent"`
	// Checked: when the console last looked for updates (RFC 3339), kept across start-ups
	Checked  string `json:"checked,omitempty"`
	Checking bool   `json:"checking"`
	// Previous is nil when there is no spare copy yet
	Previous   *Previous `json:"previous"`
	OnPrevious bool      `json:"on_previous"`
	// RolledBack: when the console went back by itself, until the player has seen the notice
	RolledBack string `json:"rolled_back,omitempty"`
	// Back: going back from Settings: working, done (restart now), failed
	Back string `json:"back,omitempty"`
}

// Starter starts a systemd unit of the system manager.
type Starter interface {
	Start(unit string) error
}

// Manager follows the units' state and starts them.
type Manager struct {
	Broker        *events.Broker
	Log           *slog.Logger
	Units         Starter
	StateFile     string // /run/owneet/system-update.json
	ReleasesFile  string // /run/owneet/releases.json
	RollbackFile  string // /run/owneet/rollback.json
	VersionFile   string // /usr/share/owneet/version
	InstalledMark string // /etc/owneet/installed
	SeenFile      string // owneetd's own: the last check, the notice already shown
	Poll          time.Duration

	mu       sync.Mutex
	checking bool
	back     string
	last     []byte
}

// what the units write
type stateFile struct {
	State    string    `json:"state"`
	Current  string    `json:"current"`
	Version  string    `json:"version"`
	Size     int64     `json:"size"`
	Packages []Package `json:"packages"`
	Percent  int       `json:"percent"`
	Error    string    `json:"error"`
	Time     string    `json:"time"`
}

type rollbackFile struct {
	Spare        bool   `json:"spare"`
	SpareDate    string `json:"spare_date"`
	SpareVersion string `json:"spare_version"`
	Previous     bool   `json:"previous"`
	RolledBack   string `json:"rolled_back"`
}

type seenFile struct {
	Checked    string `json:"checked,omitempty"`
	RolledBack string `json:"rolled_back,omitempty"`
}

func readJSON(path string, v any) (time.Time, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, v) != nil {
		return info.ModTime(), false
	}
	return info.ModTime(), true
}

func (m *Manager) installed() bool {
	_, err := os.Stat(m.InstalledMark)
	return err == nil
}

func (m *Manager) seen() seenFile {
	var s seenFile
	readJSON(m.SeenFile, &s)
	return s
}

func (m *Manager) saveSeen(s seenFile) {
	data, _ := json.Marshal(s)
	if err := os.MkdirAll(filepath.Dir(m.SeenFile), 0o755); err == nil {
		err = atomicfile.Write(m.SeenFile, append(data, '\n'), 0o644)
		if err != nil {
			m.Log.Warn("system update state not saved", "err", err)
		}
	}
}

// notes returns the notes of a release, per language, from the verified list.
func (m *Manager) notes(version string) map[string]string {
	var list struct {
		Releases []struct {
			Version string            `json:"version"`
			Notes   map[string]string `json:"notes"`
		} `json:"releases"`
	}
	if _, ok := readJSON(m.ReleasesFile, &list); !ok {
		return nil
	}
	for _, r := range list.Releases {
		if r.Version == version {
			return r.Notes
		}
	}
	return nil
}

// Status reads what the units last wrote.
func (m *Manager) Status() Status {
	st := Status{Installed: m.installed(), Packages: []Package{}}
	if data, err := os.ReadFile(m.VersionFile); err == nil {
		st.Current = strings.TrimSpace(string(data))
	}
	seen := m.seen()
	var f stateFile
	if _, ok := readJSON(m.StateFile, &f); ok {
		st.State, st.Error, st.Percent = f.State, f.Error, f.Percent
		if f.State != "none" && f.State != "failed" {
			st.Version, st.Size = f.Version, f.Size
			if f.Packages != nil {
				st.Packages = f.Packages
			}
			st.Notes = m.notes(f.Version)
		}
		if f.State == "none" || f.State == "available" {
			st.Checked = f.Time
		}
	}
	if st.Checked == "" {
		st.Checked = seen.Checked
	}
	var r rollbackFile
	if _, ok := readJSON(m.RollbackFile, &r); ok {
		if r.Spare {
			st.Previous = &Previous{Version: r.SpareVersion, Since: r.SpareDate}
		}
		st.OnPrevious = r.Previous
		if r.RolledBack != "" && r.RolledBack != seen.RolledBack {
			st.RolledBack = r.RolledBack
		}
	}
	m.mu.Lock()
	st.Checking, st.Back = m.checking, m.back
	m.mu.Unlock()
	return st
}

func busy(st Status) bool {
	return st.Checking || st.State == "downloading" || st.State == "installing" || st.Back == "working"
}

// Check looks for a newer release now; "system-update.changed" follows.
func (m *Manager) Check() error {
	st := m.Status()
	if !st.Installed {
		return ErrNotInstalled
	}
	if busy(st) {
		return ErrBusy
	}
	before, _ := readJSON(m.StateFile, &stateFile{})
	m.mu.Lock()
	m.checking = true
	m.mu.Unlock()
	if err := m.Units.Start(CheckUnit); err != nil {
		m.mu.Lock()
		m.checking = false
		m.mu.Unlock()
		return err
	}
	m.Log.Info("system update: check started")
	m.publish()
	go m.waitCheck(before)
	return nil
}

// waitCheck ends "checking" once the unit has written its result (5 minutes at most, its own limit).
func (m *Manager) waitCheck(before time.Time) {
	deadline := time.Now().Add(6 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(m.poll())
		var f stateFile
		if written, ok := readJSON(m.StateFile, &f); ok && written.After(before) {
			break
		}
	}
	m.mu.Lock()
	m.checking = false
	m.mu.Unlock()
	m.publish()
}

// Install downloads and installs the release found; its progress comes as "system-update.changed".
func (m *Manager) Install() error {
	st := m.Status()
	switch {
	case !st.Installed:
		return ErrNotInstalled
	case busy(st):
		return ErrBusy
	case st.OnPrevious:
		return ErrOnPrevious
	case st.State != "available":
		return ErrNothing
	}
	if err := m.Units.Start(ApplyUnit); err != nil {
		return err
	}
	m.Log.Info("system update: installation started", "version", st.Version)
	return nil
}

// GoBack makes the previous version the system at the next start-up; Back becomes "done" (the
// interface restarts the console) or "failed".
func (m *Manager) GoBack() error {
	st := m.Status()
	switch {
	case !st.Installed:
		return ErrNotInstalled
	case busy(st):
		return ErrBusy
	case st.OnPrevious:
		return ErrOnPrevious
	case st.Previous == nil:
		return ErrNoPrevious
	}
	before, _ := readJSON(m.RollbackFile, &rollbackFile{})
	m.mu.Lock()
	m.back = "working"
	m.mu.Unlock()
	if err := m.Units.Start(BackUnit); err != nil {
		m.mu.Lock()
		m.back = ""
		m.mu.Unlock()
		return err
	}
	m.Log.Info("system update: going back to the previous version", "version", st.Previous.Version)
	m.publish()
	go m.waitBack(before)
	return nil
}

// waitBack: owneet-rollback publishes its state when it ends, with or without the spare copy.
func (m *Manager) waitBack(before time.Time) {
	result := "failed"
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(m.poll())
		var r rollbackFile
		if written, ok := readJSON(m.RollbackFile, &r); ok && written.After(before) {
			if !r.Spare {
				result = "done"
			}
			break
		}
	}
	m.Log.Info("system update: going back", "result", result)
	m.mu.Lock()
	m.back = result
	m.mu.Unlock()
	m.publish()
}

// NoticeSeen: the player has read that the console went back by itself.
func (m *Manager) NoticeSeen() {
	var r rollbackFile
	if _, ok := readJSON(m.RollbackFile, &r); ok && r.RolledBack != "" {
		s := m.seen()
		s.RolledBack = r.RolledBack
		m.saveSeen(s)
	}
	m.mu.Lock()
	if m.back == "failed" {
		m.back = ""
	}
	m.mu.Unlock()
	m.publish()
}

func (m *Manager) poll() time.Duration {
	if m.Poll == 0 {
		return 2 * time.Second
	}
	return m.Poll
}

// publish sends "system-update.changed" when the status differs from the last one sent, and keeps
// the time of the last check across start-ups (/run is emptied).
func (m *Manager) publish() {
	st := m.Status()
	if st.Checked != "" {
		if s := m.seen(); s.Checked != st.Checked {
			s.Checked = st.Checked
			m.saveSeen(s)
		}
	}
	data, _ := json.Marshal(st)
	m.mu.Lock()
	changed := string(data) != string(m.last)
	m.last = data
	m.mu.Unlock()
	if changed {
		m.Broker.Publish("system-update.changed", st)
	}
}

// Run follows the units' files: the daily check, the installation's progress, the start-up's
// rollback state.
func (m *Manager) Run(done <-chan struct{}) {
	t := time.NewTicker(m.poll())
	defer t.Stop()
	m.publish()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			m.publish()
		}
	}
}
