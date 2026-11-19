// SPDX-License-Identifier: GPL-3.0-or-later

package legend

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

type fakeScreen struct {
	mu      sync.Mutex
	layered bool
	apps    []apps.App
	calls   []string
}

func (f *fakeScreen) Status() apps.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return apps.Status{Apps: f.apps}
}
func (f *fakeScreen) ShowHome() error {
	f.mu.Lock()
	f.calls = append(f.calls, "home")
	f.mu.Unlock()
	return nil
}
func (f *fakeScreen) FocusApp(id string) error {
	f.mu.Lock()
	f.calls = append(f.calls, "focus "+id)
	f.mu.Unlock()
	return nil
}
func (f *fakeScreen) Layered() bool { return f.layered }
func (f *fakeScreen) called() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func setup(layered bool) (*Manager, *fakeScreen, <-chan events.Event) {
	fs := &fakeScreen{layered: layered, apps: []apps.App{{ID: "web:netflix", Kind: apps.KindWebApp}}}
	m := &Manager{Broker: events.NewBroker(), Screen: fs}
	ch, _ := m.Broker.Subscribe(16)
	return m, fs, ch
}

func next(t *testing.T, ch <-chan events.Event) State {
	t.Helper()
	select {
	case ev := <-ch:
		if ev.Type != "legend.changed" {
			t.Fatalf("event %s", ev.Type)
		}
		return ev.Data.(State)
	case <-time.After(time.Second):
		t.Fatal("no legend.changed")
	}
	return State{}
}

// gamescope: drawn over the web app, which stays in front; the screen it shows follows the app
func TestOverTheApp(t *testing.T) {
	m, fs, ch := setup(true)
	if err := m.Handle(Request{Action: "toggle", Context: "netflix.browse"}); err != nil {
		t.Fatal(err)
	}
	if st := next(t, ch); !st.Open || st.App != "web:netflix" || st.Context != "netflix.browse" || st.Page {
		t.Fatalf("opened %+v", st)
	}
	if err := m.Handle(Request{Action: "screen", Context: "netflix.player"}); err != nil {
		t.Fatal(err)
	}
	if st := next(t, ch); st.Context != "netflix.player" || !st.Open {
		t.Fatalf("followed %+v", st)
	}
	if err := m.Handle(Request{Action: "toggle", Context: "netflix.player"}); err != nil {
		t.Fatal(err)
	}
	if st := next(t, ch); st.Open || st.App != "web:netflix" {
		t.Fatalf("closed %+v", st)
	}
	if c := fs.called(); len(c) != 0 {
		t.Fatalf("the screen changed: %v", c)
	}
	// a screen change with the legend closed: nothing
	m.Handle(Request{Action: "screen", Context: "netflix.title"})
	select {
	case ev := <-ch:
		t.Fatalf("event while closed: %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

// cage: a page in front (the console shown), closed by the interface; the app comes back
func TestPage(t *testing.T) {
	m, fs, ch := setup(false)
	if err := m.Toggle("player", "player.video"); err != nil {
		t.Fatal(err)
	}
	if st := next(t, ch); !st.Open || !st.Page || st.App != "player" {
		t.Fatalf("opened %+v", st)
	}
	if err := m.Handle(Request{Action: "close"}); err != nil {
		t.Fatal(err)
	}
	if st := next(t, ch); st.Open {
		t.Fatalf("closed %+v", st)
	}
	if c := fs.called(); len(c) != 2 || c[0] != "home" || c[1] != "focus player" {
		t.Fatalf("screen %v", c)
	}
}

func TestRefuses(t *testing.T) {
	m, fs, _ := setup(true)
	for _, r := range []Request{{Action: "open"}, {Action: "toggle", Context: "Netflix Player"}, {Action: "toggle"}} {
		if err := m.Handle(r); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: %v", r, err)
		}
	}
	fs.apps = nil
	if err := m.Handle(Request{Action: "toggle", Context: "web"}); !errors.Is(err, ErrNoApp) {
		t.Errorf("no web app: %v", err)
	}
}

// closedSoon: a "legend.changed" closing it arrives within a second (other events skipped)
func closedSoon(ch <-chan events.Event) bool {
	deadline := time.After(time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Type == "legend.changed" {
				return !ev.Data.(State).Open
			}
		case <-deadline:
			return false
		}
	}
}

// Drawn over the app it closes when something else takes the screen (Guide) or the app exits; a
// page (cage) stays while the console is in front
func TestClosesWithTheApp(t *testing.T) {
	m, _, ch := setup(true)
	stop := make(chan struct{})
	defer close(stop)
	go m.Run(stop)
	time.Sleep(20 * time.Millisecond)
	m.Toggle("web:netflix", "netflix.browse")
	next(t, ch)
	m.Broker.Publish("focus.changed", map[string]string{"focus": "web:netflix"})
	m.Broker.Publish("focus.changed", map[string]string{"focus": "home"})
	if !closedSoon(ch) {
		t.Fatal("not closed by Guide")
	}
	m.Toggle("web:netflix", "netflix.browse")
	m.Broker.Publish("app.exited", map[string]string{"id": "web:netflix"})
	time.Sleep(100 * time.Millisecond)
	if m.Status().Open {
		t.Fatal("open after its app exited")
	}

	p, _, pch := setup(false)
	go p.Run(stop)
	time.Sleep(20 * time.Millisecond)
	p.Toggle("player", "player.song")
	next(t, pch)
	p.Broker.Publish("focus.changed", map[string]string{"focus": "home"})
	time.Sleep(100 * time.Millisecond)
	if !p.Status().Open {
		t.Fatal("the cage page closed when the console came to the front")
	}
}
