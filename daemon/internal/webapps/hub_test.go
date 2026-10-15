// SPDX-License-Identifier: GPL-3.0-or-later

package webapps

import (
	"errors"
	"testing"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

type fakeScreen struct {
	running []apps.App
	shown   []string
}

func (f *fakeScreen) Status() apps.Status { return apps.Status{Apps: f.running} }
func (f *fakeScreen) ShowHome() error {
	f.shown = append(f.shown, "home")
	return nil
}
func (f *fakeScreen) FocusApp(id string) error {
	f.shown = append(f.shown, id)
	return nil
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
