// SPDX-License-Identifier: GPL-3.0-or-later

// Package bluetooth manages Bluetooth through BlueZ: device list, connect/disconnect/forget and
// automatic gamepad pairing with no input (PROJECT_RULES.md section 7): while auto-pair is on,
// every device that identifies as a gamepad is paired, trusted and connected by itself.
package bluetooth

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// Device is a Bluetooth device known to BlueZ.
type Device struct {
	Address   string `json:"address"`
	Name      string `json:"name"`
	Paired    bool   `json:"paired"`
	Trusted   bool   `json:"trusted"`
	Connected bool   `json:"connected"`
	Gamepad   bool   `json:"gamepad"`
	Battery   *int   `json:"battery,omitempty"`

	Class      uint32 `json:"-"`
	Appearance uint16 `json:"-"`
	Icon       string `json:"-"`
}

// State is a snapshot of the adapter and its devices.
type State struct {
	Adapter     bool
	Powered     bool
	Discovering bool
	Devices     []Device
}

// Status is the body of GET /v1/bluetooth.
type Status struct {
	Adapter     bool     `json:"adapter"`
	Powered     bool     `json:"powered"`
	Discovering bool     `json:"discovering"`
	AutoPair    bool     `json:"auto_pair"`
	Devices     []Device `json:"devices"`
}

// Backend is BlueZ (or a fake in tests).
type Backend interface {
	Snapshot() (State, error)
	SetPowered(on bool) error
	SetDiscovery(on bool) error
	Pair(address string) error
	SetTrusted(address string, on bool) error
	Connect(address string) error
	Disconnect(address string) error
	Remove(address string) error
	// Changes signals that something changed in BlueZ.
	Changes() <-chan struct{}
}

// Errors returned to the API.
var (
	ErrNoAdapter     = errors.New("no Bluetooth adapter")
	ErrUnknownDevice = errors.New("unknown Bluetooth device")
)

// IsGamepad tells whether a device identifies as a gamepad or joystick: Bluetooth Classic class
// of device (peripheral, minor joystick/gamepad), Bluetooth LE appearance, or BlueZ's icon.
func IsGamepad(d Device) bool {
	major := (d.Class >> 8) & 0x1f
	minor := (d.Class >> 2) & 0x0f
	if major == 0x05 && (minor == 0x01 || minor == 0x02) {
		return true
	}
	if d.Appearance == 0x03c3 || d.Appearance == 0x03c4 {
		return true
	}
	return d.Icon == "input-gaming"
}

// Manager keeps the Bluetooth state and runs automatic pairing.
type Manager struct {
	Backend Backend
	Broker  *events.Broker
	Log     *slog.Logger
	// Controllers returns how many game controllers are connected (any connection). When it is 0
	// and AutoPairWithoutController is set, auto-pair turns on by itself.
	Controllers               func() int
	AutoPairWithoutController bool
	// Interval of the reconcile loop; RetryAfter between pairing attempts for the same device.
	Interval   time.Duration
	RetryAfter time.Duration

	mu          sync.Mutex
	state       State
	autoUntil   time.Time // zero: off
	autoForever bool      // on because no controller is connected
	ourScan     bool      // discovery was started by us
	inFlight    map[string]bool
	lastTry     map[string]time.Time
	lastPaired  map[string]bool
}

func (m *Manager) init() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inFlight == nil {
		m.inFlight, m.lastTry, m.lastPaired = map[string]bool{}, map[string]time.Time{}, map[string]bool{}
	}
	if m.Interval == 0 {
		m.Interval = 2 * time.Second
	}
	if m.RetryAfter == 0 {
		m.RetryAfter = 10 * time.Second
	}
}

// Run keeps the state fresh and drives auto-pairing until ctx ends.
func (m *Manager) Run(ctx context.Context) {
	m.init()
	tick := time.NewTicker(m.Interval)
	defer tick.Stop()
	m.reconcile()
	for {
		select {
		case <-ctx.Done():
			m.stopOurScan()
			return
		case <-m.Backend.Changes():
			m.reconcile()
		case <-tick.C:
			m.reconcile()
		}
	}
}

// Status returns the current state for the API.
func (m *Manager) Status() Status {
	m.init()
	m.mu.Lock()
	defer m.mu.Unlock()
	devs := append([]Device{}, m.state.Devices...)
	sort.Slice(devs, func(i, j int) bool { return devs[i].Address < devs[j].Address })
	return Status{Adapter: m.state.Adapter, Powered: m.state.Powered, Discovering: m.state.Discovering,
		AutoPair: m.autoActiveLocked(time.Now()), Devices: devs}
}

func (m *Manager) autoActiveLocked(now time.Time) bool {
	return m.autoForever || now.Before(m.autoUntil)
}

// SetAutoPair turns automatic gamepad pairing on for the given time, or off.
func (m *Manager) SetAutoPair(on bool, d time.Duration) error {
	m.init()
	m.mu.Lock()
	if !m.state.Adapter {
		m.mu.Unlock()
		return ErrNoAdapter
	}
	if on {
		m.autoUntil = time.Now().Add(d)
	} else {
		m.autoUntil, m.autoForever = time.Time{}, false
	}
	m.mu.Unlock()
	m.Broker.Publish("bluetooth.auto_pair", map[string]bool{"enabled": on})
	m.reconcile()
	return nil
}

// AllowPairing tells the BlueZ agent whether to accept a pairing request from address: only
// gamepads, only while auto-pair is on.
func (m *Manager) AllowPairing(address string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.autoActiveLocked(time.Now()) {
		return false
	}
	for _, d := range m.state.Devices {
		if strings.EqualFold(d.Address, address) {
			return d.Gamepad
		}
	}
	return false
}

// Name returns the name of a known device, or "" when BlueZ does not know it (its alias is then
// just the address).
func (m *Manager) Name(address string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.state.Devices {
		if strings.EqualFold(d.Address, address) {
			if strings.EqualFold(strings.ReplaceAll(d.Name, "-", ":"), d.Address) {
				return ""
			}
			return d.Name
		}
	}
	return ""
}

// IsTrusted tells the agent whether a device is already trusted (its services are authorised).
func (m *Manager) IsTrusted(address string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.state.Devices {
		if strings.EqualFold(d.Address, address) {
			return d.Trusted
		}
	}
	return false
}

func (m *Manager) device(address string) (Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.state.Adapter {
		return Device{}, ErrNoAdapter
	}
	for _, d := range m.state.Devices {
		if strings.EqualFold(d.Address, address) {
			return d, nil
		}
	}
	return Device{}, ErrUnknownDevice
}

// Connect, Disconnect and Forget act on a known device.
func (m *Manager) Connect(address string) error {
	d, err := m.device(address)
	if err != nil {
		return err
	}
	return m.Backend.Connect(d.Address)
}

func (m *Manager) Disconnect(address string) error {
	d, err := m.device(address)
	if err != nil {
		return err
	}
	return m.Backend.Disconnect(d.Address)
}

func (m *Manager) Forget(address string) error {
	d, err := m.device(address)
	if err != nil {
		return err
	}
	if err := m.Backend.Remove(d.Address); err != nil {
		return err
	}
	m.Broker.Publish("bluetooth.forgotten", map[string]string{"address": d.Address})
	return nil
}

// reconcile refreshes the state, applies the "no controller" policy and starts pairings.
func (m *Manager) reconcile() {
	st, err := m.Backend.Snapshot()
	if err != nil {
		m.Log.Debug("bluetooth state unavailable", "err", err)
		st = State{}
	}
	for i := range st.Devices {
		st.Devices[i].Gamepad = IsGamepad(st.Devices[i])
	}

	controllers := -1
	if m.Controllers != nil {
		controllers = m.Controllers()
	}

	m.mu.Lock()
	m.state = st
	now := time.Now()
	policyChanged := false
	if m.AutoPairWithoutController && st.Adapter && controllers == 0 && !m.autoForever {
		m.autoForever, policyChanged = true, true
		m.Log.Info("no controller connected: automatic gamepad pairing on")
	} else if m.autoForever && controllers > 0 {
		m.autoForever, policyChanged = false, true
		m.Log.Info("a controller is connected: automatic gamepad pairing off")
	}
	active := st.Adapter && m.autoActiveLocked(now)
	var toPair []Device
	if active {
		for _, d := range st.Devices {
			if d.Gamepad && !d.Paired && !m.inFlight[d.Address] && now.Sub(m.lastTry[d.Address]) > m.RetryAfter {
				m.inFlight[d.Address], m.lastTry[d.Address] = true, now
				toPair = append(toPair, d)
			}
		}
	}
	// Report newly paired devices once.
	var newlyPaired []Device
	for _, d := range st.Devices {
		if d.Paired && !m.lastPaired[d.Address] {
			newlyPaired = append(newlyPaired, d)
		}
		m.lastPaired[d.Address] = d.Paired
	}
	m.mu.Unlock()

	if policyChanged {
		m.Broker.Publish("bluetooth.auto_pair", map[string]bool{"enabled": active})
	}
	if active {
		if !st.Powered {
			if err := m.Backend.SetPowered(true); err != nil {
				m.Log.Warn("cannot power on Bluetooth", "err", err)
			}
		}
		if st.Powered && !st.Discovering {
			if err := m.Backend.SetDiscovery(true); err == nil {
				m.mu.Lock()
				m.ourScan = true
				m.mu.Unlock()
			}
		}
	} else {
		m.stopOurScan()
	}
	for _, d := range newlyPaired {
		m.Broker.Publish("bluetooth.paired", map[string]string{"address": d.Address, "name": d.Name})
	}
	for _, d := range toPair {
		go m.pair(d)
	}
}

func (m *Manager) stopOurScan() {
	m.mu.Lock()
	ours := m.ourScan
	m.ourScan = false
	m.mu.Unlock()
	if ours {
		m.Backend.SetDiscovery(false)
	}
}

// pair pairs, trusts and connects a gamepad.
func (m *Manager) pair(d Device) {
	defer func() {
		m.mu.Lock()
		delete(m.inFlight, d.Address)
		m.mu.Unlock()
	}()
	m.Log.Info("pairing gamepad", "address", d.Address, "name", d.Name)
	m.Broker.Publish("bluetooth.pairing", map[string]string{"address": d.Address, "name": d.Name})
	steps := []struct {
		what string
		fn   func() error
	}{
		{"pair", func() error { return m.Backend.Pair(d.Address) }},
		{"trust", func() error { return m.Backend.SetTrusted(d.Address, true) }},
		{"connect", func() error { return m.Backend.Connect(d.Address) }},
	}
	for _, s := range steps {
		if err := s.fn(); err != nil {
			m.Log.Warn("gamepad pairing failed", "address", d.Address, "step", s.what, "err", err)
			m.Broker.Publish("bluetooth.pair_failed", map[string]string{"address": d.Address, "name": d.Name, "step": s.what})
			return
		}
	}
	m.reconcile()
}
