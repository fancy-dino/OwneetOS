// SPDX-License-Identifier: GPL-3.0-or-later

package pointer

import (
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/evdev"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
	"github.com/fancy-dino/OwneetOS/daemon/internal/sdldb"
)

type fakeMouse struct {
	mu     sync.Mutex
	dx, dy int
	wheel  int
	clicks []string
}

func (f *fakeMouse) MovePointer(dx, dy, wheel int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dx += dx
	f.dy += dy
	f.wheel += wheel
	return nil
}
func (f *fakeMouse) Click(b string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clicks = append(f.clicks, b)
	return nil
}

type fakeApps struct {
	mu    sync.Mutex
	ids   map[string]uint32
	homes int
}

func (f *fakeApps) AppIDOf(id string) uint32 { f.mu.Lock(); defer f.mu.Unlock(); return f.ids[id] }
func (f *fakeApps) ShowHome() error          { f.mu.Lock(); defer f.mu.Unlock(); f.homes++; return nil }

type fakeScreen struct {
	mu          sync.Mutex
	app, window uint32
	text        bool
}

func (f *fakeScreen) Focused() (uint32, uint32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.app, f.window, nil
}
func (f *fakeScreen) TextCursor() (bool, error) { f.mu.Lock(); defer f.mu.Unlock(); return f.text, nil }
func (f *fakeScreen) Close()                    {}
func (f *fakeScreen) set(app, window uint32) {
	f.mu.Lock()
	f.app, f.window = app, window
	f.mu.Unlock()
}

const gameID uint32 = 0x7E000001

func newPointer(t *testing.T) (*Manager, *fakeMouse, *fakeApps, *fakeScreen, *time.Time, <-chan events.Event) {
	t.Helper()
	broker := events.NewBroker()
	ch, unsub := broker.Subscribe(64)
	t.Cleanup(unsub)
	mouse, apps, screen := &fakeMouse{}, &fakeApps{ids: map[string]uint32{"steam:480": gameID}}, &fakeScreen{app: HomeAppID}
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	m := &Manager{Broker: broker, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Mouse: mouse, Apps: apps,
		Screen: func() (Screen, error) { return screen, nil }, manual: true}
	m.now = func() time.Time { return now }
	return m, mouse, apps, screen, &now, ch
}

func next(t *testing.T, ch <-chan events.Event, typ string) events.Event {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case e := <-ch:
			if e.Type == typ {
				return e
			}
		case <-timeout:
			t.Fatalf("no %s event", typ)
		}
	}
}

// advance moves the clock and runs one step of dt
func advance(m *Manager, now *time.Time, dt time.Duration) {
	*now = now.Add(dt)
	m.step(dt)
}

func TestPointerFollowsOneWindow(t *testing.T) {
	m, mouse, apps, screen, now, ch := newPointer(t)
	if err := m.On("steam:999"); err != ErrUnknownApp {
		t.Fatalf("unknown app: %v", err)
	}
	if err := m.On("steam:480"); err != nil {
		t.Fatal(err)
	}
	if st := next(t, ch, "pointer.changed").Data.(Status); !st.On || st.App != "steam:480" {
		t.Fatalf("status %+v", st)
	}
	// the home screen is still on screen: it waits, nothing moves
	m.input(Input{Control: "leftx", Value: 1})
	advance(m, now, checkEvery)
	advance(m, now, 10*time.Millisecond)
	if mouse.dx != 0 {
		t.Fatalf("moved over the home screen: %d", mouse.dx)
	}
	// the launcher's window comes: the pointer holds for it; the stick pushed fully, 10 ms
	screen.set(gameID, 0x400001)
	advance(m, now, checkEvery)
	m.input(Input{Control: "leftx", Value: 1})
	mouse.dx = 0
	advance(m, now, 10*time.Millisecond)
	if want := int(speed(1) * 10); mouse.dx != want || mouse.dy != 0 {
		t.Fatalf("moved %d,%d, want %d,0", mouse.dx, mouse.dy, want)
	}
	// a slight push is slower, and under the dead zone nothing moves
	m.input(Input{Control: "leftx", Value: 0.1})
	mouse.dx = 0
	advance(m, now, 10*time.Millisecond)
	if mouse.dx != 0 {
		t.Fatalf("dead zone moved %d", mouse.dx)
	}
	m.input(Input{Control: "leftx", Value: 0})
	// buttons: A click, X right click, Y the keyboard (home screen)
	m.input(Input{Control: "a", Value: 1})
	m.input(Input{Control: "a", Value: 0})
	m.input(Input{Control: "x", Value: 1})
	m.input(Input{Control: "y", Value: 1})
	if d := next(t, ch, "pointer.type").Data.(map[string]string); d["app"] != "steam:480" || d["reason"] != "button" {
		t.Fatalf("pointer.type %v", d)
	}
	if len(mouse.clicks) != 2 || mouse.clicks[0] != "left" || mouse.clicks[1] != "right" || apps.homes != 1 {
		t.Fatalf("clicks %v, home %d", mouse.clicks, apps.homes)
	}
	// D-pad: a step at once, then held: repeated after 360 ms, every 40 ms
	mouse.dx, mouse.dy = 0, 0
	m.input(Input{Control: "dpright", Value: 1})
	if mouse.dx != step {
		t.Fatalf("D-pad step %d", mouse.dx)
	}
	advance(m, now, 300*time.Millisecond)
	advance(m, now, 70*time.Millisecond)
	advance(m, now, 40*time.Millisecond)
	if mouse.dx != 3*step {
		t.Fatalf("D-pad held %d", mouse.dx)
	}
	m.input(Input{Control: "dpright", Value: 0})
	advance(m, now, 100*time.Millisecond)
	if mouse.dx != 3*step {
		t.Fatalf("D-pad released %d", mouse.dx)
	}
	// right stick up: the wheel up, a notch every 160 ms
	m.input(Input{Control: "righty", Value: -1})
	advance(m, now, 8*time.Millisecond)
	advance(m, now, 100*time.Millisecond)
	advance(m, now, 100*time.Millisecond)
	if mouse.wheel != 2 {
		t.Fatalf("wheel %d", mouse.wheel)
	}
	m.input(Input{Control: "righty", Value: 0})
	// the game's window replaces the launcher's: off by itself
	screen.set(gameID, 0x400002)
	advance(m, now, checkEvery)
	if st := next(t, ch, "pointer.changed").Data.(Status); st.On || st.Reason != "window" {
		t.Fatalf("after the window changed %+v", st)
	}
	if m.Status().On {
		t.Fatal("still on")
	}
}

func TestPointerOffWhenTheAppCloses(t *testing.T) {
	m, _, apps, screen, now, ch := newPointer(t)
	screen.set(gameID, 7)
	m.On("steam:480")
	next(t, ch, "pointer.changed")
	advance(m, now, checkEvery)
	apps.mu.Lock()
	delete(apps.ids, "steam:480")
	apps.mu.Unlock()
	advance(m, now, checkEvery)
	if st := next(t, ch, "pointer.changed").Data.(Status); st.On || st.Reason != "closed" {
		t.Fatalf("status %+v", st)
	}
	// another app on screen turns it off too
	apps.ids["steam:480"] = gameID
	m.On("steam:480")
	next(t, ch, "pointer.changed")
	screen.set(gameID+1, 9)
	advance(m, now, checkEvery)
	if st := next(t, ch, "pointer.changed").Data.(Status); st.On || st.Reason != "window" {
		t.Fatalf("another app %+v", st)
	}
	m.Off("user") // already off: nothing
}

// A click on a text field (the pointer becomes the text "I") asks the keyboard by itself.
func TestClickOnATextField(t *testing.T) {
	m, _, apps, screen, now, ch := newPointer(t)
	screen.set(gameID, 7)
	screen.text = true
	m.On("steam:480")
	advance(m, now, checkEvery)
	m.input(Input{Control: "a", Value: 1})
	if d := next(t, ch, "pointer.type").Data.(map[string]string); d["reason"] != "field" {
		t.Fatalf("pointer.type %v", d)
	}
	apps.mu.Lock()
	homes := apps.homes
	apps.mu.Unlock()
	if homes != 1 {
		t.Fatalf("home %d", homes)
	}
}

func TestTextShape(t *testing.T) {
	draw := func(w, h int, rows []string) []uint32 {
		img := make([]uint32, w*h)
		for y, r := range rows {
			for x, c := range r {
				if c == '#' {
					img[y*w+x] = 0xFF000000
				}
			}
		}
		return img
	}
	ibeam := []string{
		"###.###", "...#...", "...#...", "...#...", "...#...", "...#...", "...#...", "...#...",
		"...#...", "...#...", "...#...", "...#...", "...#...", "...#...", "...#...", "###.###"}
	arrow := []string{
		"#.........", "##........", "#.#.......", "#..#......", "#...#.....", "#....#....", "#.....#...",
		"#......#..", "#.......#.", "#....#####", "#..#.#....", "#.#..#....", "##....#...", "#.....#...",
		"......#...", ".......#.."}
	if !textShape(32, 32, draw(32, 32, ibeam)) {
		t.Fatal("the text I not seen")
	}
	if textShape(32, 32, draw(32, 32, arrow)) {
		t.Fatal("an arrow seen as the text I")
	}
	if textShape(32, 32, make([]uint32, 32*32)) {
		t.Fatal("an empty cursor")
	}
}

func TestMappingFromTheDatabase(t *testing.T) {
	d := &evdev.Device{ID: evdev.ID{Bustype: 3, Vendor: 0x045e, Product: 0x0b12, Version: 0x0517}}
	d.SetCapabilities([]int{0x130, 0x131, 0x133, 0x134, 0x136, 0x137, 0x13a, 0x13b, 0x13c, 0x13d, 0x13e},
		[]int{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x10, 0x11})
	db, err := sdldb.Parse(strings.NewReader(
		"030000005e040000120b000017050000,Xbox Series Controller,a:b0,b:b1,x:b2,y:b3,leftx:a0,lefty:a1,rightx:a3,righty:a4,dpup:h0.1,dpdown:h0.4,dpleft:h0.8,dpright:h0.2,platform:Linux,\n"))
	if err != nil {
		t.Fatal(err)
	}
	b := mapping(d, db)
	want := map[string]binding{
		"a": {key: 0x130, abs: -1}, "x": {key: 0x133, abs: -1}, "y": {key: 0x134, abs: -1},
		"leftx": {key: -1, abs: 0x00}, "lefty": {key: -1, abs: 0x01}, "righty": {key: -1, abs: 0x04},
		"dpup": {key: -1, abs: 0x11, sign: -1}, "dpright": {key: -1, abs: 0x10, sign: 1},
	}
	for c, w := range want {
		if b[c] != w {
			t.Errorf("%s: %+v, want %+v", c, b[c], w)
		}
	}
	// unknown to the database: the usual layout
	d2 := &evdev.Device{ID: evdev.ID{Bustype: 3, Vendor: 0x1234, Product: 0x5678}}
	d2.SetCapabilities([]int{0x130, 0x133, 0x134, 0x220, 0x221, 0x222, 0x223}, []int{0x00, 0x01, 0x04})
	b = mapping(d2, db)
	if b["a"].key != 0x130 || b["dpleft"].key != 0x222 || b["righty"].abs != 0x04 {
		t.Fatalf("fallback %+v", b)
	}
	if !ignored(&evdev.Device{Name: "Steam Virtual Gamepad", ID: evdev.ID{Vendor: 0x28de, Product: 0x11ff}}) ||
		!ignored(&evdev.Device{ID: evdev.ID{Bustype: evdev.BusVirtual}}) || ignored(d) {
		t.Fatal("ignored devices")
	}
}
