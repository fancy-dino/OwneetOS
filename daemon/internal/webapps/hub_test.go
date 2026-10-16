// SPDX-License-Identifier: GPL-3.0-or-later

package webapps

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

type fakeScreen struct {
	running []apps.App
	mu      sync.Mutex
	shown   []string
}

func (f *fakeScreen) Status() apps.Status { return apps.Status{Apps: f.running} }
func (f *fakeScreen) ShowHome() error     { return f.FocusApp("home") }
func (f *fakeScreen) FocusApp(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shown = append(f.shown, id)
	return nil
}

// screens waits a moment for n screens (the timers show them after their event), then returns
// what was shown.
func (f *fakeScreen) screens(n int) []string {
	for deadline := time.Now().Add(time.Second); ; time.Sleep(5 * time.Millisecond) {
		f.mu.Lock()
		got := append([]string(nil), f.shown...)
		f.mu.Unlock()
		if len(got) >= n || time.Now().After(deadline) {
			return got
		}
	}
}

func hub(t *testing.T, running ...apps.App) (*Hub, *fakeScreen, <-chan events.Event) {
	t.Helper()
	b := events.NewBroker()
	ch, stop := b.Subscribe(16)
	t.Cleanup(stop)
	s := &fakeScreen{running: running}
	return &Hub{Broker: b, Screen: s}, s, ch
}

func next(t *testing.T, ch <-chan events.Event) events.Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
	return events.Event{}
}

var netflix = apps.App{ID: "web:netflix", Kind: apps.KindWebApp}

func TestKeyboardOpensOverTheWebAppAndCloses(t *testing.T) {
	h, s, ch := hub(t, apps.App{ID: "mpv", Kind: apps.KindApp}, netflix)
	if err := h.SetKeyboard(Keyboard{Open: true, Kind: "email", Label: "Email", Value: "a@"}); err != nil {
		t.Fatal(err)
	}
	ev := next(t, ch)
	k, _ := ev.Data.(Keyboard)
	if ev.Type != "webapp.keyboard" || !k.Open || k.App != "netflix" || k.Value != "a@" {
		t.Fatalf("open event: %+v", ev)
	}
	if err := h.Type(Text{Text: "b"}); err != nil {
		t.Fatal(err)
	}
	if ev := next(t, ch); ev.Type != "webapp.text" {
		t.Fatalf("text event: %+v", ev)
	}
	if err := h.SetKeyboard(Keyboard{Open: false, Next: true}); err != nil {
		t.Fatal(err)
	}
	ev = next(t, ch)
	if k, _ := ev.Data.(Keyboard); k.Open || !k.Next || k.App != "netflix" {
		t.Fatalf("close event: %+v", ev)
	}
	if len(s.shown) != 2 || s.shown[0] != "home" || s.shown[1] != "web:netflix" {
		t.Errorf("screens shown: %v (want the home screen, then the web app)", s.shown)
	}
	// Closing again changes nothing; typing needs the keyboard open
	if err := h.SetKeyboard(Keyboard{Open: false}); err != nil || len(s.shown) != 2 {
		t.Errorf("second close: %v %v", err, s.shown)
	}
	if err := h.Type(Text{Text: "c"}); !errors.Is(err, ErrKeyboardClosed) {
		t.Errorf("typing with the keyboard closed: %v", err)
	}
}

func TestKeyboardNeedsAWebAppAndValidFields(t *testing.T) {
	h, _, _ := hub(t)
	if err := h.SetKeyboard(Keyboard{Open: true, Kind: "text"}); !errors.Is(err, ErrNoWebApp) {
		t.Errorf("no web app: %v", err)
	}
	h, _, _ = hub(t, netflix)
	if err := h.SetKeyboard(Keyboard{Open: true, Kind: "color"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown kind: %v", err)
	}
	h.SetKeyboard(Keyboard{Open: true, Kind: "text"})
	for _, bad := range []Text{{}, {Text: "a", Key: "left"}, {Key: "enter"}, {Text: string(make([]rune, 65))}} {
		if err := h.Type(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

func TestAskClose(t *testing.T) {
	h, s, ch := hub(t, netflix)
	if err := h.AskClose(); err != nil {
		t.Fatal(err)
	}
	ev := next(t, ch)
	if d, _ := ev.Data.(map[string]string); ev.Type != "webapp.ask_close" || d["app"] != "netflix" {
		t.Fatalf("event: %+v", ev)
	}
	if len(s.shown) != 1 || s.shown[0] != "home" {
		t.Errorf("shown: %v", s.shown)
	}
	h, _, _ = hub(t)
	if err := h.AskClose(); !errors.Is(err, ErrNoWebApp) {
		t.Errorf("no web app: %v", err)
	}
}

func TestLookDropsInvalidValues(t *testing.T) {
	h, _, _ := hub(t)
	h.SetLook(Look{Accent: "#FF8A5B", Language: "it"})
	if l := h.Look(); l.Accent != "#FF8A5B" || l.Language != "it" {
		t.Errorf("look: %+v", l)
	}
	h.SetLook(Look{Accent: "red; background: url(x)", Language: "../../x"})
	if l := h.Look(); l.Accent != "" || l.Language != "" {
		t.Errorf("invalid look kept: %+v", l)
	}
}

// The controller never stays stuck: without the interface's confirmation the keyboard closes by
// itself and the web app comes back.
func TestUnconfirmedKeyboardCloses(t *testing.T) {
	h, s, ch := hub(t, netflix)
	h.Confirm = 50 * time.Millisecond
	if err := h.SetKeyboard(Keyboard{Open: true, Kind: "text"}); err != nil {
		t.Fatal(err)
	}
	next(t, ch) // opened
	ev := next(t, ch)
	if k, _ := ev.Data.(Keyboard); ev.Type != "webapp.keyboard" || k.Open || k.Next || k.App != "netflix" {
		t.Fatalf("close event: %+v", ev)
	}
	if got := s.screens(2); len(got) != 2 || got[1] != "web:netflix" {
		t.Errorf("shown: %v (want the web app back)", got)
	}
	if err := h.KeyboardShown(); !errors.Is(err, ErrKeyboardClosed) {
		t.Errorf("confirming a closed keyboard: %v", err)
	}
}

// Confirmed, the keyboard stays open while the interface runs, and closes when it stops.
func TestConfirmedKeyboardClosesWhenTheInterfaceStops(t *testing.T) {
	h, _, ch := hub(t, netflix)
	var pid atomic.Int64
	pid.Store(100)
	h.HomePID = func() int { return int(pid.Load()) }
	h.Confirm, h.Poll = 50*time.Millisecond, 10*time.Millisecond
	if err := h.SetKeyboard(Keyboard{Open: true, Kind: "text"}); err != nil {
		t.Fatal(err)
	}
	next(t, ch)
	if err := h.KeyboardShown(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if !h.KeyboardState().Open {
		t.Fatal("a confirmed keyboard closed while the interface runs")
	}
	pid.Store(0) // the interface stopped
	if ev := next(t, ch); ev.Type != "webapp.keyboard" || ev.Data.(Keyboard).Open {
		t.Fatalf("close event: %+v", ev)
	}
}

// A new opening is not closed by the timer of the one before.
func TestReopenedKeyboardGetsItsOwnTime(t *testing.T) {
	h, _, ch := hub(t, netflix)
	h.Confirm = 80 * time.Millisecond
	h.SetKeyboard(Keyboard{Open: true, Kind: "text"})
	next(t, ch)
	time.Sleep(50 * time.Millisecond)
	h.SetKeyboard(Keyboard{Open: true, Kind: "email"})
	next(t, ch)
	h.KeyboardShown()
	time.Sleep(80 * time.Millisecond) // the first opening's time is over
	if !h.KeyboardState().Open {
		t.Fatal("closed by the timer of an earlier opening")
	}
}

// The keyboard closes when its web app exits; there is nothing to bring back.
func TestKeyboardClosesWhenItsWebAppExits(t *testing.T) {
	h, s, ch := hub(t, netflix)
	stop := make(chan struct{})
	defer close(stop)
	go h.Follow(stop)
	time.Sleep(20 * time.Millisecond) // Follow subscribed
	h.SetKeyboard(Keyboard{Open: true, Kind: "text"})
	next(t, ch)
	h.KeyboardShown()
	h.Broker.Publish("app.exited", map[string]string{"id": "web:netflix"})
	next(t, ch) // app.exited
	if ev := next(t, ch); ev.Type != "webapp.keyboard" || ev.Data.(Keyboard).Open {
		t.Fatalf("close event: %+v", ev)
	}
	time.Sleep(50 * time.Millisecond)
	if got := s.screens(1); len(got) != 1 {
		t.Errorf("shown: %v (nothing to bring back)", got)
	}
}
