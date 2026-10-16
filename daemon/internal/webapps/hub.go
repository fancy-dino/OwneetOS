// SPDX-License-Identifier: GPL-3.0-or-later

// Package webapps connects the console's Brave extension (through owneet-bridge) with the
// interface ([[extension-design]], [[step-5.2-extension-core]]): the palette and language the
// extension draws with, the console's keyboard for the web app's text fields, and "Close {name}?"
// when B has nothing to go back to. Both sides follow the events; owneetd brings the right screen
// to the front.
//
// The controller never stays stuck behind a keyboard nobody shows (owner, 2026-10-16): the
// interface confirms that it shows the keyboard; without that within ConfirmWithin, or when the
// interface stops afterwards, or when the web app exits, owneetd closes the keyboard and the
// extension takes the controller back.
package webapps

import (
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
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

// ConfirmWithin is how long the interface has to confirm that it shows the keyboard.
const ConfirmWithin = 3 * time.Second

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
	// HomePID returns the interface's process (0: none). Once the interface has confirmed the
	// keyboard, a different answer (it stopped, or the session restarted it) closes the keyboard.
	HomePID func() int
	// Confirm and Poll replace ConfirmWithin and the check of the interface's process (tests).
	Confirm, Poll time.Duration

	mu       sync.Mutex
	look     Look
	keyboard Keyboard
	gen      int  // counts the keyboard's openings: timers and watchers of an older one stop
	shown    bool // the interface confirmed this opening
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
		h.gen++
		h.shown = false
		gen := h.gen
		h.mu.Unlock()
		h.Broker.Publish("webapp.keyboard", k)
		h.show(func(s Screen) error { return s.ShowHome() })
		time.AfterFunc(or(h.Confirm, ConfirmWithin), func() {
			h.mu.Lock()
			unconfirmed := h.gen == gen && h.keyboard.Open && !h.shown
			h.mu.Unlock()
			if unconfirmed {
				h.logInfo("the interface did not show the keyboard: closed")
				h.closeKeyboard(gen, false, true)
			}
		})
		return nil
	}
	h.closeKeyboard(0, k.Next, true)
	return nil
}

// closeKeyboard closes the keyboard (of opening gen, or any when 0) and, with back, brings the web
// app back to the front.
func (h *Hub) closeKeyboard(gen int, next, back bool) {
	h.mu.Lock()
	was := h.keyboard
	if !was.Open || (gen != 0 && gen != h.gen) {
		h.mu.Unlock()
		return
	}
	h.keyboard = Keyboard{}
	h.shown = false
	h.mu.Unlock()
	h.Broker.Publish("webapp.keyboard", Keyboard{Open: false, Next: next, App: was.App})
	if back {
		h.show(func(s Screen) error { return s.FocusApp("web:" + was.App) })
	}
}

// KeyboardShown: the interface confirms that it shows the keyboard. From then on the keyboard
// closes if the interface's process stops or is replaced.
func (h *Hub) KeyboardShown() error {
	h.mu.Lock()
	if !h.keyboard.Open {
		h.mu.Unlock()
		return ErrKeyboardClosed
	}
	if h.shown {
		h.mu.Unlock()
		return nil
	}
	h.shown = true
	gen := h.gen
	h.mu.Unlock()
	if h.HomePID == nil {
		return nil
	}
	pid := h.HomePID()
	go func() {
		t := time.NewTicker(or(h.Poll, 500*time.Millisecond))
		defer t.Stop()
		for range t.C {
			h.mu.Lock()
			current := h.gen == gen && h.keyboard.Open
			h.mu.Unlock()
			if !current {
				return
			}
			if h.HomePID() != pid {
				h.logInfo("the interface stopped while showing the keyboard: closed")
				h.closeKeyboard(gen, false, true)
				return
			}
		}
	}()
	return nil
}

// Follow closes the keyboard when its web app exits (nothing to bring back). It returns when
// stop is closed.
func (h *Hub) Follow(stop <-chan struct{}) {
	ch, unsubscribe := h.Broker.Subscribe(16)
	defer unsubscribe()
	for {
		select {
		case <-stop:
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			d, _ := ev.Data.(map[string]string)
			if ev.Type != "app.exited" || !strings.HasPrefix(d["id"], "web:") {
				continue
			}
			h.mu.Lock()
			gen, mine := h.gen, h.keyboard.Open && "web:"+h.keyboard.App == d["id"]
			h.mu.Unlock()
			if mine {
				h.closeKeyboard(gen, false, false)
			}
		}
	}
}

func or(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

func (h *Hub) logInfo(msg string) {
	if h.Log != nil {
		h.Log.Info(msg)
	}
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

// show changes what is on screen (in the cage session the interface carries it out).
func (h *Hub) show(do func(Screen) error) {
	if h.Screen == nil {
		return
	}
	if err := do(h.Screen); err != nil && h.Log != nil {
		h.Log.Debug("web app: screen not changed", "err", err)
	}
}
