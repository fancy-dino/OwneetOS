// SPDX-License-Identifier: GPL-3.0-or-later

// Package windows makes the console ready for Windows games ([[step-4.5-windows-games-proton]]):
// Proton (GE-Proton) and the Steam Linux Runtime it runs in are downloaded once, in the
// background, by umu-launcher, the first time the console is online with a Windows game in the
// library (the interface says when there is one: SetNeeded). Until then a Windows game cannot
// start; the interface shows the progress ("windows.changed"). Proton is never named to the user.
//
// The Proton that is ready is written to DataDir/proton (its folder); owneet-windows-run starts
// the games with it, so that a game's start never downloads anything.
package windows

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// States of the preparation.
const (
	StateNone        = "none"        // no Windows game in the library: nothing is downloaded
	StateWaiting     = "waiting"     // needed, waiting for an internet connection
	StateDownloading = "downloading" // getting ready (Percent)
	StateReady       = "ready"       // Windows games can start
	StateFailed      = "failed"      // the last try failed: tried again when the network changes
)

// Status is GET /v1/windows and the "windows.changed" event.
type Status struct {
	State   string `json:"state"`
	Percent int    `json:"percent"`
	// Needed: a Windows game is (or was) in the library
	Needed bool `json:"needed"`
}

// Preparer downloads what Windows games need and returns the folder of the Proton to use;
// progress receives 0…100 while it works.
type Preparer interface {
	Prepare(ctx context.Context, progress func(percent int)) (protonDir string, err error)
}

// Manager keeps the preparation's state.
type Manager struct {
	Broker   *events.Broker
	Log      *slog.Logger
	DataDir  string      // e.g. ~/.local/share/owneet/windows
	Online   func() bool // the console has an internet connection
	Preparer Preparer

	mu      sync.Mutex
	status  Status
	running bool
	failed  time.Time // the last failed try: the next one after a network change or 10 minutes
	kick    chan struct{}
	once    sync.Once
}

func (m *Manager) init() {
	m.once.Do(func() {
		m.kick = make(chan struct{}, 1)
		m.status = Status{State: StateNone}
		if _, err := os.Stat(filepath.Join(m.DataDir, "needed")); err == nil {
			m.status.Needed = true
			m.status.State = StateWaiting
		}
		if dir := m.protonDir(); dir != "" {
			m.status = Status{State: StateReady, Percent: 100, Needed: true}
		}
	})
}

// protonDir is the Proton made ready earlier, if its folder is still there.
func (m *Manager) protonDir() string {
	data, err := os.ReadFile(filepath.Join(m.DataDir, "proton"))
	if err != nil {
		return ""
	}
	dir := strings.TrimSpace(string(data))
	if st, err := os.Stat(filepath.Join(dir, "proton")); err != nil || st.IsDir() {
		return ""
	}
	return dir
}

// Status reports the preparation's state.
func (m *Manager) Status() Status {
	m.init()
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// SetNeeded records that the library has a Windows game: the preparation starts as soon as the
// console is online (and is remembered across restarts).
func (m *Manager) SetNeeded() error {
	m.init()
	m.mu.Lock()
	already := m.status.Needed
	if !already {
		m.status.Needed = true
		if m.status.State == StateNone {
			m.status.State = StateWaiting
		}
	}
	m.mu.Unlock()
	if !already {
		if err := os.MkdirAll(m.DataDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(m.DataDir, "needed"), []byte("1\n"), 0o644); err != nil {
			return err
		}
		m.publish()
	}
	m.Kick()
	return nil
}

// Kick checks whether the preparation can start now (e.g. the network changed).
func (m *Manager) Kick() {
	m.init()
	select {
	case m.kick <- struct{}{}:
	default:
	}
}

func (m *Manager) publish() {
	if m.Broker != nil {
		m.Broker.Publish("windows.changed", m.Status())
	}
}

func (m *Manager) set(update func(*Status)) {
	m.mu.Lock()
	before := m.status
	update(&m.status)
	changed := m.status != before
	m.mu.Unlock()
	if changed {
		m.publish()
	}
}

// Run starts the preparation whenever it is needed and possible, until ctx ends. Network changes
// (the "network.changed" event) and SetNeeded wake it up; it also looks every minute.
func (m *Manager) Run(ctx context.Context) {
	m.init()
	var netEvents <-chan events.Event
	if m.Broker != nil {
		ch, cancel := m.Broker.Subscribe(16)
		defer cancel()
		netEvents = ch
	}
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	m.Kick()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-netEvents:
			if !ok {
				netEvents = nil
				continue
			}
			if ev.Type != "network.changed" {
				continue
			}
			m.mu.Lock()
			m.failed = time.Time{}
			m.mu.Unlock()
		case <-tick.C:
		case <-m.kick:
		}
		m.maybeStart(ctx)
	}
}

func (m *Manager) maybeStart(ctx context.Context) {
	m.mu.Lock()
	st := m.status
	busy := m.running
	recent := !m.failed.IsZero() && time.Since(m.failed) < 10*time.Minute
	m.mu.Unlock()
	if busy || recent || !st.Needed || st.State == StateReady || m.Preparer == nil {
		return
	}
	if m.Online != nil && !m.Online() {
		m.set(func(s *Status) {
			if s.State != StateFailed {
				s.State = StateWaiting
			}
		})
		return
	}
	m.mu.Lock()
	m.running = true
	m.mu.Unlock()
	m.set(func(s *Status) { s.State = StateDownloading })
	go m.prepare(ctx)
}

func (m *Manager) prepare(ctx context.Context) {
	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()
	if m.Log != nil {
		m.Log.Info("getting Windows games ready")
	}
	dir, err := m.Preparer.Prepare(ctx, func(p int) {
		m.set(func(s *Status) {
			if p > s.Percent && p < 100 {
				s.Percent = p
			}
		})
	})
	if err == nil && dir == "" {
		err = errors.New("no Proton folder")
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(m.DataDir, "proton"), []byte(dir+"\n"), 0o644)
	}
	if err != nil {
		if m.Log != nil {
			m.Log.Warn("Windows games could not be made ready", "err", err)
		}
		m.mu.Lock()
		m.failed = time.Now()
		m.mu.Unlock()
		m.set(func(s *Status) { s.State = StateFailed })
		return
	}
	if m.Log != nil {
		m.Log.Info("Windows games are ready", "proton", dir)
	}
	m.set(func(s *Status) { *s = Status{State: StateReady, Percent: 100, Needed: true} })
}
