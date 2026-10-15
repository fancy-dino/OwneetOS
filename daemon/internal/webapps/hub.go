// SPDX-License-Identifier: GPL-3.0-or-later

// Package webapps connects the console's Brave extension (through owneet-bridge) with the
// interface ([[extension-design]], [[step-5.2-extension-core]]): the palette and language the
// extension draws with, the console's keyboard for the web app's text fields, and "Close {name}?"
// when B has nothing to go back to. Both sides follow the events; owneetd brings the right screen
// to the front.
package webapps

import (
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

var (
	// ErrInvalid: a field is missing or not allowed.
	ErrInvalid = errors.New("invalid request")
	// ErrNoWebApp: no web app is open.
	ErrNoWebApp = errors.New("no web app is open")
	// ErrKeyboardClosed: text arrives while the keyboard is closed.
	ErrKeyboardClosed = errors.New("the keyboard is not open")
)

// Look is what the extension draws with: the palette's accent colour and the console's language.
type Look struct {
	Accent   string `json:"accent,omitempty"`
	Language string `json:"language,omitempty"`
}

// Keyboard opens or closes the console's keyboard for a text field of the web app.
type Keyboard struct {
	Open bool `json:"open"`
	// Kind is the field's type: text, email, password, search, url, tel, number.
	Kind  string `json:"kind,omitempty"`
	Label string `json:"label,omitempty"`
	Value string `json:"value,omitempty"`
	// Next (on closing): Done was chosen, the selection moves on to the next target.
	Next bool `json:"next,omitempty"`
	// App is the open web app's catalogue id (set by owneetd).
	App string `json:"app,omitempty"`
}

// Text is what the keyboard types: Text, or one of the keys backspace, left, right.
type Text struct {
	Text string `json:"text,omitempty"`
	Key  string `json:"key,omitempty"`
}

// Screen shows the home screen or an app (*apps.Manager).
type Screen interface {
	Status() apps.Status
	ShowHome() error
	FocusApp(id string) error
}

const (
	maxLabel = 200
	maxValue = 4096
	maxText  = 64
)

var (
	colour   = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	language = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})?$`)
	kinds    = map[string]bool{"text": true, "email": true, "password": true, "search": true, "url": true, "tel": true, "number": true}
	keys     = map[string]bool{"backspace": true, "left": true, "right": true}
)

// Hub holds the web apps' shared state.
type Hub struct {
	Broker *events.Broker
	Log    *slog.Logger
	// Screen brings the home screen or the web app to the front (nil: not available).
	Screen Screen

	mu       sync.Mutex
	look     Look
	keyboard Keyboard
}

// SetLook keeps the look sent by the interface when it opens a web app; invalid values are dropped.
func (h *Hub) SetLook(l Look) {
	if !colour.MatchString(l.Accent) {
		l.Accent = ""
	}
	if !language.MatchString(l.Language) {
		l.Language = ""
	}
	h.mu.Lock()
	h.look = l
	h.mu.Unlock()
}

// Look returns the look for the extension.
func (h *Hub) Look() Look {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.look
}

// openWebApp returns the catalogue id of the open web app.
func (h *Hub) openWebApp() (string, error) {
	if h.Screen == nil {
		return "", ErrNoWebApp
	}
	for _, a := range h.Screen.Status().Apps {
		if a.Kind == apps.KindWebApp {
			return strings.TrimPrefix(a.ID, "web:"), nil
		}
	}
	return "", ErrNoWebApp
}

// SetKeyboard opens the keyboard (the console comes to the front) or closes it (the web app comes
// back). "webapp.keyboard" tells the interface and the extension.
func (h *Hub) SetKeyboard(k Keyboard) error {
	if k.Open {
		if !kinds[k.Kind] || utf8.RuneCountInString(k.Label) > maxLabel || len(k.Value) > maxValue {
			return ErrInvalid
		}
		id, err := h.openWebApp()
		if err != nil {
			return err
		}
		k.App, k.Next = id, false
		h.mu.Lock()
		h.keyboard = k
		h.mu.Unlock()
		h.Broker.Publish("webapp.keyboard", k)
		h.show(func(s Screen) error { return s.ShowHome() })
		return nil
	}
	h.mu.Lock()
	was := h.keyboard
	h.keyboard = Keyboard{}
	h.mu.Unlock()
	if !was.Open {
		return nil
	}
	h.Broker.Publish("webapp.keyboard", Keyboard{Open: false, Next: k.Next, App: was.App})
	h.show(func(s Screen) error { return s.FocusApp("web:" + was.App) })
	return nil
}

// KeyboardState returns the keyboard's state (e.g. for an interface that restarted).
func (h *Hub) KeyboardState() Keyboard {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.keyboard
}

// Type passes the keyboard's text or key to the extension ("webapp.text").
func (h *Hub) Type(t Text) error {
	if (t.Text == "") == (t.Key == "") || utf8.RuneCountInString(t.Text) > maxText || (t.Key != "" && !keys[t.Key]) {
		return ErrInvalid
	}
	h.mu.Lock()
	open := h.keyboard.Open
	h.mu.Unlock()
	if !open {
		return ErrKeyboardClosed
	}
	h.Broker.Publish("webapp.text", t)
	return nil
}

// AskClose: B with nothing to go back to. The home screen comes to the front and the interface
// asks "Close {name}?" ("webapp.ask_close").
func (h *Hub) AskClose() error {
	id, err := h.openWebApp()
	if err != nil {
		return err
	}
	h.Broker.Publish("webapp.ask_close", map[string]string{"app": id})
	h.show(func(s Screen) error { return s.ShowHome() })
	return nil
}

// show changes what is on screen; in the cage session owneetd cannot, and the interface does it.
func (h *Hub) show(do func(Screen) error) {
	if h.Screen == nil {
		return
	}
	if err := do(h.Screen); err != nil && !errors.Is(err, apps.ErrFocusUnsupported) && h.Log != nil {
		h.Log.Debug("web app: screen not changed", "err", err)
	}
}
