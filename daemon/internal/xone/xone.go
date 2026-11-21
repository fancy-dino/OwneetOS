// SPDX-License-Identifier: GPL-3.0-or-later

// Package xone tells the interface when an Xbox wireless adapter needs its firmware, and installs
// it once the player has accepted Microsoft's terms ([[xbox-wireless-dongle]],
// [[step-6.1-first-boot-flow]]). The firmware is Microsoft's and is never shipped: udev marks a
// plugged-in adapter without it (/run/owneet/xone-firmware-needed), and the root unit
// owneet-xone-firmware-install.service, which polkit allows to the console user, downloads it on
// this PC and writes its progress to /run/owneet/xone-firmware.json.
package xone

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// Unit is the root unit that installs the firmware.
const Unit = "owneet-xone-firmware-install.service"

// Status is the body of GET /v1/xone.
type Status struct {
	// Needed: an adapter is plugged in without its firmware
	Needed bool `json:"needed"`
	// State: "" (nothing done yet), installing, installed, failed
	State string `json:"state"`
	// Error (when failed): offline, no_adapter, failed
	Error string `json:"error,omitempty"`
}

// ErrBusy: the firmware is already being installed.
var ErrBusy = errors.New("the firmware is already being installed")

// Starter starts a systemd unit of the system manager.
type Starter interface {
	Start(unit string) error
}

// Manager reads the adapter's state and installs its firmware.
type Manager struct {
	Broker     *events.Broker
	Log        *slog.Logger
	Units      Starter
	NeededFile string // /run/owneet/xone-firmware-needed
	StateFile  string // /run/owneet/xone-firmware.json
	Poll       time.Duration
	Timeout    time.Duration

	mu      sync.Mutex
	running bool
	last    Status
}

func (m *Manager) read() (Status, time.Time) {
	var st Status
	if data, err := os.ReadFile(m.NeededFile); err == nil && strings.TrimSpace(string(data)) != "" {
		st.Needed = true
	}
	var modified time.Time
	if info, err := os.Stat(m.StateFile); err == nil {
		modified = info.ModTime()
		if data, err := os.ReadFile(m.StateFile); err == nil {
			var file struct{ State, Error string }
			if json.Unmarshal(data, &file) == nil {
				st.State, st.Error = file.State, file.Error
			}
		}
	}
	if st.State != "failed" {
		st.Error = ""
	}
	return st, modified
}

// Status tells whether an adapter needs its firmware and how its installation went.
func (m *Manager) Status() Status {
	st, _ := m.read()
	m.mu.Lock()
	if m.running && st.State != "installing" {
		st.State = "installing" // started, the unit has not written yet
	}
	m.mu.Unlock()
	return st
}

// Install starts the firmware's installation (the player has accepted Microsoft's terms on the
// console); "xone.changed" follows it.
func (m *Manager) Install() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return ErrBusy
	}
	m.running = true
	m.mu.Unlock()
	_, before := m.read()
	if err := m.Units.Start(Unit); err != nil {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
		return err
	}
	m.Log.Info("Xbox adapter firmware: installation started")
	m.Broker.Publish("xone.changed", m.Status())
	go m.watch(before)
	return nil
}

// watch follows the unit's progress until it is installed or failed.
func (m *Manager) watch(before time.Time) {
	poll, timeout := m.Poll, m.Timeout
	if poll == 0 {
		poll = 500 * time.Millisecond
	}
	if timeout == 0 {
		timeout = 11 * time.Minute // the unit gives up after 10
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(poll)
		st, written := m.read()
		if !written.After(before) || st.State == "installing" {
			continue
		}
		m.finish(st)
		return
	}
	m.finish(Status{Needed: true, State: "failed", Error: "failed"})
}

func (m *Manager) finish(st Status) {
	m.mu.Lock()
	m.running = false
	m.mu.Unlock()
	m.Log.Info("Xbox adapter firmware: installation finished", "state", st.State, "error", st.Error)
	m.Broker.Publish("xone.changed", m.Status())
}

// Run sends "xone.changed" when an adapter that needs its firmware is plugged in or out.
func (m *Manager) Run(done <-chan struct{}) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	m.mu.Lock()
	m.last = m.Status()
	m.mu.Unlock()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			st := m.Status()
			m.mu.Lock()
			changed := st != m.last
			m.last = st
			m.mu.Unlock()
			if changed {
				m.Broker.Publish("xone.changed", st)
			}
		}
	}
}
