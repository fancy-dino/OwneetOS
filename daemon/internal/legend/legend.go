// SPDX-License-Identifier: GPL-3.0-or-later

// Package legend holds the command legend's state ([[step-5.6a-command-legend]],
// [[controller-input-map]] "The command legend"): View, in a web app or the player, opens a legend
// of what each button does in that app, for the screen on show. The app's own integration catches
// View (the console's extension, through owneet-bridge and POST /v1/legend; the player's script,
// through its control socket) and asks owneetd, which still intercepts only Guide; the interface
// draws the legend ("legend.changed"):
//   - gamescope: over the app, which keeps working under it; only View closes it, and it closes
//     when the app leaves the screen (Guide) or exits, and when the console's keyboard opens over
//     the page (gamescope draws one external overlay at a time);
//   - cage (one window at a time): a page of its own in front (owneetd shows the console); B or
//     View closes it ("close"), and the app comes back.
package legend

import (
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
	"github.com/fancy-dino/OwneetOS/daemon/internal/webapps"
)

// State is the body of GET /v1/legend and of "legend.changed".
type State struct {
	Open bool `json:"open"`
	// App is the app's id among the running apps: "web:netflix", "player"
	App string `json:"app,omitempty"`
	// Context is the legend to show: the app and the screen on show, e.g. "netflix.player",
	// "player.video", "web" (a web app without a navigation of its own)
	Context string `json:"context,omitempty"`
	// Page: the interface shows it as a page in front (cage); otherwise over the app (gamescope)
	Page bool `json:"page,omitempty"`
}

// Request is the body of POST /v1/legend.
type Request struct {
	// Action: "toggle" (View in the web app), "screen" (the web app's screen changed: its legend,
	// when open, follows), "close" (the interface: B or View on the page)
	Action  string `json:"action"`
	Context string `json:"context,omitempty"`
}

// Errors returned to the API.
var (
	ErrInvalid = errors.New("invalid legend request")
	ErrNoApp   = errors.New("no web app is open")
)

// Screen shows the home screen or an app (*apps.Manager).
type Screen interface {
	Status() apps.Status
	ShowHome() error
	FocusApp(id string) error
	// Layered: the interface can draw over an app (gamescope)
	Layered() bool
}

// Manager keeps the legend's state.
type Manager struct {
	Broker *events.Broker
	Log    *slog.Logger
	Screen Screen

	mu    sync.Mutex
	state State
}

var contextRE = regexp.MustCompile(`^[a-z]{1,20}(\.[a-z]{1,20})?$`)

// Toggle: View in an app (app: its id), on a screen (context). The same app's legend open closes;
// otherwise it opens.
func (m *Manager) Toggle(app, context string) error {
	if !contextRE.MatchString(context) || app == "" {
		return ErrInvalid
	}
	m.mu.Lock()
	open := m.state.Open && m.state.App == app
	m.mu.Unlock()
	if open {
		m.close(true)
		return nil
	}
	page := m.Screen == nil || !m.Screen.Layered()
	st := State{Open: true, App: app, Context: context, Page: page}
	m.mu.Lock()
	m.state = st
	m.mu.Unlock()
	m.logInfo("legend opened", "app", app, "context", context)
	m.Broker.Publish("legend.changed", st)
	if page && m.Screen != nil {
		if err := m.Screen.ShowHome(); err != nil {
			m.logDebug("legend: the console not shown", "err", err)
		}
	}
	return nil
}

// SetScreen: the app's screen changed (Netflix's title window, its player…): its legend, when
// open, follows it.
func (m *Manager) SetScreen(app, context string) error {
	if !contextRE.MatchString(context) {
		return ErrInvalid
	}
	m.mu.Lock()
	if !m.state.Open || m.state.App != app || m.state.Context == context {
		m.mu.Unlock()
		return nil
	}
	m.state.Context = context
	st := m.state
	m.mu.Unlock()
	m.Broker.Publish("legend.changed", st)
	return nil
}

// close closes the legend; back: a page (cage) gives the screen back to its app.
func (m *Manager) close(back bool) {
	m.mu.Lock()
	was := m.state
	if !was.Open {
		m.mu.Unlock()
		return
	}
	m.state = State{}
	m.mu.Unlock()
	m.logInfo("legend closed", "app", was.App)
	m.Broker.Publish("legend.changed", State{App: was.App})
	if back && was.Page && m.Screen != nil {
		if err := m.Screen.FocusApp(was.App); err != nil {
			m.logDebug("legend: the app not brought back", "err", err)
		}
	}
}

// Handle answers POST /v1/legend: the open web app's View and screen (from the extension), and the
// interface's close.
func (m *Manager) Handle(req Request) error {
	switch req.Action {
	case "toggle", "screen":
		app, err := m.webApp()
		if err != nil {
			return err
		}
		if req.Action == "toggle" {
			return m.Toggle(app, req.Context)
		}
		return m.SetScreen(app, req.Context)
	case "close":
		m.close(true)
		return nil
	}
	return ErrInvalid
}

// webApp is the open web app's id ("web:…").
func (m *Manager) webApp() (string, error) {
	if m.Screen == nil {
		return "", ErrNoApp
	}
	for _, a := range m.Screen.Status().Apps {
		if a.Kind == apps.KindWebApp {
			return a.ID, nil
		}
	}
	return "", ErrNoApp
}

// Status is the legend's state (e.g. for an interface that restarted).
func (m *Manager) Status() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Run closes the legend when its app exits, and, drawn over the app, when something else takes
// the screen (Guide: an overlay would float over the console too) or the console's keyboard opens
// over the page: gamescope draws one external overlay at a time, the most opaque, so the keyboard
// might not be drawn ([[gamescope-overlay-window-not-drawn-or-left-on-screen]]). It returns when
// stop is closed.
func (m *Manager) Run(stop <-chan struct{}) {
	evs, unsubscribe := m.Broker.Subscribe(32)
	defer unsubscribe()
	for {
		select {
		case <-stop:
			return
		case ev, ok := <-evs:
			if !ok {
				return
			}
			data, _ := ev.Data.(map[string]string)
			m.mu.Lock()
			st := m.state
			m.mu.Unlock()
			if !st.Open {
				continue
			}
			switch {
			case ev.Type == "app.exited" && data["id"] == st.App:
				m.close(false)
			case ev.Type == "focus.changed" && !st.Page && data["focus"] != st.App:
				m.close(false)
			case ev.Type == "webapp.keyboard" && !st.Page:
				if k, ok := ev.Data.(webapps.Keyboard); ok && k.Open {
					m.close(false)
				}
			}
		}
	}
}

func (m *Manager) logInfo(msg string, args ...any) {
	if m.Log != nil {
		m.Log.Info(msg, args...)
	}
}

func (m *Manager) logDebug(msg string, args ...any) {
	if m.Log != nil {
		m.Log.Debug(msg, args...)
	}
}

// IsWebApp: the legend belongs to a web app (the extension follows it).
func (s State) IsWebApp() bool { return strings.HasPrefix(s.App, "web:") }
