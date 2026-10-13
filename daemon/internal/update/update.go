// SPDX-License-Identifier: GPL-3.0-or-later

// Package update lets testers update the OwneetOS packages of an installed test PC from a USB
// stick ([[step-4.2b-updates-from-a-usb-stick]]): owneetd starts the root units owneet-update@check
// / @apply.service (a polkit rule allows only these to the console user), which write their result
// to /run/owneet/update.json; owneetd reads it and sends "update.changed". The OwneetOS updates of
// every user come later (7.1).
package update

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
	"github.com/godbus/dbus/v5"
)

// Errors returned to the API.
var (
	ErrNotTester = errors.New("updates from a USB stick are for systems installed by testers")
	ErrBusy      = errors.New("an update check or installation is already running")
)

// Package is an OwneetOS package with a newer version on the stick.
type Package struct {
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Status is the body of GET /v1/update.
type Status struct {
	Tester bool `json:"tester"`
	// State: "" (not checked yet), none, available, installing, done, failed, no_stick, not_tester
	State    string    `json:"state"`
	Message  string    `json:"message,omitempty"`
	Packages []Package `json:"packages"`
	Running  bool      `json:"running"`
}

// Starter starts a systemd unit of the system manager.
type Starter interface {
	Start(unit string) error
}

// Manager reads the update state and starts checks and installations.
type Manager struct {
	Broker     *events.Broker
	Log        *slog.Logger
	Units      Starter
	StateFile  string // /run/owneet/update.json
	TesterMark string // /etc/owneet/tester
	Poll       time.Duration
	Timeout    time.Duration

	mu      sync.Mutex
	running bool
}

func (m *Manager) tester() bool {
	_, err := os.Stat(m.TesterMark)
	return err == nil
}

// read returns the last result the units wrote, and when.
func (m *Manager) read() (Status, time.Time) {
	st := Status{Packages: []Package{}}
	info, err := os.Stat(m.StateFile)
	if err != nil {
		return st, time.Time{}
	}
	data, err := os.ReadFile(m.StateFile)
	if err == nil {
		_ = json.Unmarshal(data, &st)
	}
	if st.Packages == nil {
		st.Packages = []Package{}
	}
	return st, info.ModTime()
}

// Status tells what the last check or installation found.
func (m *Manager) Status() Status {
	st, _ := m.read()
	st.Tester = m.tester()
	m.mu.Lock()
	st.Running = m.running
	m.mu.Unlock()
	return st
}

// Check looks at the stick; the result arrives as "update.changed".
func (m *Manager) Check() error { return m.start("check") }

// Apply installs the newer packages of the stick; "update.changed" follows the installation.
func (m *Manager) Apply() error { return m.start("apply") }

func (m *Manager) start(action string) error {
	if !m.tester() {
		return ErrNotTester
	}
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return ErrBusy
	}
	m.running = true
	m.mu.Unlock()
	_, before := m.read()
	if err := m.Units.Start("owneet-update@" + action + ".service"); err != nil {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
		return err
	}
	m.Log.Info("update started", "action", action)
	go m.watch(before)
	return nil
}

// final states end a check or an installation
func final(state string) bool { return state != "" && state != "installing" }

// watch sends "update.changed" each time the units write a new result, until a final one.
func (m *Manager) watch(before time.Time) {
	poll, timeout := m.Poll, m.Timeout
	if poll == 0 {
		poll = 500 * time.Millisecond
	}
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	last := before
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(poll)
		st, written := m.read()
		if !written.After(last) {
			continue
		}
		last = written
		done := final(st.State)
		if done {
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
		}
		m.Broker.Publish("update.changed", m.Status())
		if done {
			m.Log.Info("update finished", "state", st.State)
			return
		}
	}
	m.mu.Lock()
	m.running = false
	m.mu.Unlock()
	m.Broker.Publish("update.changed", m.Status())
}

// SystemdStarter starts units through systemd on the system bus.
type SystemdStarter struct{ conn *dbus.Conn }

// NewSystemdStarter connects to the system bus.
func NewSystemdStarter() (*SystemdStarter, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	return &SystemdStarter{conn: conn}, nil
}

// Close disconnects from the bus.
func (s *SystemdStarter) Close() { s.conn.Close() }

// Start queues the start of a unit (polkit decides whether the console user may).
func (s *SystemdStarter) Start(unit string) error {
	var job dbus.ObjectPath
	return s.conn.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1").
		Call("org.freedesktop.systemd1.Manager.StartUnit", 0, unit, "replace").Store(&job)
}
