// SPDX-License-Identifier: GPL-3.0-or-later

// Package pointer makes the controller a mouse for the windows made for one
// ([[step-4.7-controller-pointer]], owner 2026-10-19, [[controller-input-map]]): a game's launcher,
// an app's window. The user turns it on by hand from the home screen ("Resume with the pointer")
// for the running game or app; it holds for that app's window only and turns off by itself when
// another window takes the screen (e.g. the game after its launcher) or the app closes. While the
// home screen is shown it waits.
//
// While it is on: left stick moves the pointer (faster the further it is pushed), D-pad moves it
// in small steps, A clicks, X right-clicks, right stick scrolls, Y asks the console's keyboard
// ("pointer.type"); A on a text field (the pointer has the text "I" shape after the click) asks
// it too. Nothing is taken from the app: the controllers are read without a grab, and the
// pointer only adds a virtual mouse's movements and clicks.
package pointer

import (
	"errors"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// HomeAppID is the home screen's gamescope app id (apps.HomeAppID).
const HomeAppID uint32 = 0x7E000000

var (
	ErrUnavailable = errors.New("the virtual mouse is not available")
	ErrUnknownApp  = errors.New("no such running app")
)

// Mouse is the virtual mouse (vinput).
type Mouse interface {
	MovePointer(dx, dy, wheel int) error
	Click(button string) error
}

// Apps gives a running app's gamescope app id (0: not running) and brings the home screen up.
type Apps interface {
	AppIDOf(id string) uint32
	ShowHome()
}

// Screen is gamescope's X server: what is on screen, and the pointer's shape.
type Screen interface {
	// Focused: the gamescope app id and the X window on screen
	Focused() (app, window uint32, err error)
	// TextCursor: the pointer has the text "I" shape (over a text field)
	TextCursor() (bool, error)
	Close()
}

// Input is a control of a controller: a button (0 / 1) or an axis (-1…1).
type Input struct {
	Control string // a, x, y, leftx, lefty, righty, dpup, dpdown, dpleft, dpright
	Value   float64
}

// Status is GET /v1/pointer and the "pointer.changed" event.
type Status struct {
	On     bool   `json:"on"`
	App    string `json:"app,omitempty"`
	Reason string `json:"reason,omitempty"` // why it turned off: window, closed, user
}

// Timing and speeds (owner's demo, 2026-10-19).
const (
	tick        = 8 * time.Millisecond
	checkEvery  = 200 * time.Millisecond // what is on screen
	deadZone    = 0.15
	step        = 6 // D-pad: pixels per step
	firstRepeat = 360 * time.Millisecond
	nextRepeat  = 40 * time.Millisecond
	wheelEvery  = 160 * time.Millisecond
	textDelay   = 120 * time.Millisecond // after a click, the app changes the pointer's shape
)

// speed is the pointer's speed in pixels per millisecond for a push of 0…1 (slow at first).
func speed(push float64) float64 { return 0.25 + 1.1*push*push }

// Manager is the pointer.
type Manager struct {
	Broker *events.Broker
	Log    *slog.Logger
	Mouse  Mouse
	Apps   Apps
	// Screen connects to gamescope's X server (the session's DISPLAY)
	Screen func() (Screen, error)
	// Pads starts reading the controllers into in until stop is closed
	Pads func(in chan<- Input, stop <-chan struct{})

	mu      sync.Mutex
	status  Status
	appID   uint32 // the app's gamescope id
	window  uint32 // the window it holds for (0 until the app is on screen)
	active  bool   // the app's window is on screen (not the home screen)
	stop    chan struct{}
	screen  Screen
	state   map[string]float64
	held    map[string]time.Time // D-pad: when the direction was pressed
	repeat  map[string]time.Time // D-pad: the next repeat
	wheelAt time.Time
	restX   float64 // fractions of a pixel left from the last move
	restY   float64
	lastChk time.Time
	now     func() time.Time
	manual  bool // tests: no loop, step and input are called by hand
}

// Status tells whether the pointer is on, and for which app.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// On turns the pointer on for a running app: it holds for the window the app shows next.
func (m *Manager) On(app string) error {
	if m.Mouse == nil || m.Screen == nil {
		return ErrUnavailable
	}
	id := m.Apps.AppIDOf(app)
	if id == 0 {
		return ErrUnknownApp
	}
	screen, err := m.Screen()
	if err != nil {
		m.Log.Warn("pointer: cannot read the screen", "err", err)
		return ErrUnavailable
	}
	m.mu.Lock()
	if m.stop != nil { // on already: for this app now
		close(m.stop)
		m.screen.Close()
	}
	m.status = Status{On: true, App: app}
	m.appID, m.window, m.active = id, 0, false
	m.screen = screen
	m.state, m.held, m.repeat = map[string]float64{}, map[string]time.Time{}, map[string]time.Time{}
	m.stop = make(chan struct{})
	stop := m.stop
	m.mu.Unlock()
	m.Log.Info("pointer on", "app", app)
	m.publish()
	in := make(chan Input, 64)
	if m.manual {
		return nil
	}
	if m.Pads != nil {
		go m.Pads(in, stop)
	}
	go m.loop(in, stop)
	return nil
}

// Off turns the pointer off (reason: user, window, closed).
func (m *Manager) Off(reason string) {
	m.mu.Lock()
	if m.stop == nil {
		m.mu.Unlock()
		return
	}
	close(m.stop)
	m.stop = nil
	m.screen.Close()
	app := m.status.App
	m.status = Status{Reason: reason}
	m.mu.Unlock()
	m.Log.Info("pointer off", "app", app, "reason", reason)
	m.publish()
}

func (m *Manager) publish() {
	if m.Broker != nil {
		m.Broker.Publish("pointer.changed", m.Status())
	}
}

func (m *Manager) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

func (m *Manager) loop(in <-chan Input, stop <-chan struct{}) {
	t := time.NewTicker(tick)
	defer t.Stop()
	last := m.clock()
	for {
		select {
		case <-stop:
			return
		case ev := <-in:
			m.input(ev)
		case <-t.C:
			now := m.clock()
			m.step(now.Sub(last))
			last = now
		}
	}
}

// check reads what is on screen: the app's own window (active), the home screen (waits),
// another window of the app or another app (off), the app gone (off).
func (m *Manager) check() {
	m.mu.Lock()
	screen, appID, window, app := m.screen, m.appID, m.window, m.status.App
	m.mu.Unlock()
	if m.Apps.AppIDOf(app) == 0 {
		m.Off("closed")
		return
	}
	focused, win, err := screen.Focused()
	if err != nil {
		return
	}
	switch {
	case focused == HomeAppID || focused == 0:
		m.setActive(false)
	case focused == appID && (window == 0 || win == window):
		m.mu.Lock()
		if m.window == 0 {
			m.window = win
		}
		m.mu.Unlock()
		m.setActive(true)
	default: // another window took the screen
		m.Off("window")
	}
}

func (m *Manager) setActive(active bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = active
	if !active { // nothing held carries over a visit to the home screen
		m.state, m.held, m.repeat = map[string]float64{}, map[string]time.Time{}, map[string]time.Time{}
	}
}

// input records a control; buttons act on their press.
func (m *Manager) input(ev Input) {
	m.mu.Lock()
	if m.state == nil {
		m.mu.Unlock()
		return
	}
	before := m.state[ev.Control]
	m.state[ev.Control] = ev.Value
	active := m.active
	pressed := before < 0.5 && ev.Value >= 0.5
	now := m.clock()
	if len(ev.Control) > 2 && ev.Control[:2] == "dp" {
		if pressed {
			m.held[ev.Control] = now
			m.repeat[ev.Control] = now.Add(firstRepeat)
		} else if ev.Value < 0.5 {
			delete(m.held, ev.Control)
		}
	}
	m.mu.Unlock()
	if !active || !pressed {
		return
	}
	switch ev.Control {
	case "a":
		m.Mouse.Click("left")
		go m.afterClick()
	case "x":
		m.Mouse.Click("right")
	case "y":
		m.askKeyboard("button")
	case "dpup", "dpdown", "dpleft", "dpright":
		m.nudge(ev.Control)
	}
}

// afterClick: a click on a text field (the pointer has the text "I" shape) asks the keyboard.
func (m *Manager) afterClick() {
	time.Sleep(textDelay)
	m.mu.Lock()
	screen, active := m.screen, m.active
	m.mu.Unlock()
	if !active || screen == nil {
		return
	}
	if text, err := screen.TextCursor(); err == nil && text {
		m.askKeyboard("field")
	}
}

// askKeyboard: the home screen comes up with the console's keyboard; its text goes to the window
// (POST /v1/input/text) and Resume comes back, the pointer still on.
func (m *Manager) askKeyboard(reason string) {
	app := m.Status().App
	m.Log.Info("pointer: the keyboard", "app", app, "reason", reason)
	m.Apps.ShowHome()
	if m.Broker != nil {
		m.Broker.Publish("pointer.type", map[string]string{"app": app, "reason": reason})
	}
}

func (m *Manager) nudge(dir string) {
	dx, dy := 0, 0
	switch dir {
	case "dpup":
		dy = -step
	case "dpdown":
		dy = step
	case "dpleft":
		dx = -step
	case "dpright":
		dx = step
	}
	m.Mouse.MovePointer(dx, dy, 0)
}

// step moves the pointer for dt: the left stick, held D-pad directions, the right stick's wheel;
// and reads the screen now and then.
func (m *Manager) step(dt time.Duration) {
	now := m.clock()
	m.mu.Lock()
	check := now.Sub(m.lastChk) >= checkEvery
	if check {
		m.lastChk = now
	}
	m.mu.Unlock()
	if check {
		m.check()
	}
	m.mu.Lock()
	if !m.active || m.state == nil {
		m.mu.Unlock()
		return
	}
	x, y, ry := m.state["leftx"], m.state["lefty"], m.state["righty"]
	push := math.Min(1, math.Hypot(x, y))
	var dx, dy int
	if push > deadZone {
		ms := float64(dt) / float64(time.Millisecond)
		v := speed((push-deadZone)/(1-deadZone)) * ms
		m.restX += x / push * v
		m.restY += y / push * v
		dx, dy = int(m.restX), int(m.restY)
		m.restX -= float64(dx)
		m.restY -= float64(dy)
	} else {
		m.restX, m.restY = 0, 0
	}
	var nudges []string
	for dir, at := range m.repeat {
		if _, held := m.held[dir]; held && !now.Before(at) {
			nudges = append(nudges, dir)
			m.repeat[dir] = now.Add(nextRepeat)
		}
	}
	wheel := 0
	if math.Abs(ry) > 0.5 && !now.Before(m.wheelAt) {
		wheel = 1 // the stick up scrolls up (a positive notch)
		if ry > 0 {
			wheel = -1
		}
		m.wheelAt = now.Add(wheelEvery)
	} else if math.Abs(ry) <= 0.5 {
		m.wheelAt = time.Time{}
	}
	m.mu.Unlock()
	if dx != 0 || dy != 0 || wheel != 0 {
		m.Mouse.MovePointer(dx, dy, wheel)
	}
	for _, dir := range nudges {
		m.nudge(dir)
	}
}
