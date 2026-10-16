// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/res"
	"github.com/jezek/xgb/xproto"
)

// Gamescope controls what gamescope shows. gamescope runs with --steam: it then shows only
// windows tagged with an app id (STEAM_GAME property, set by Steam for its games) and lets the
// shell choose which app is on screen. owneetd plays that role:
//   - it watches new windows and tags them: the home screen with HomeAppID, each app with its own;
//   - it writes GAMESCOPECTRL_BASELAYER_APPID: the apps to show, first one with a window wins;
//   - it reads GAMESCOPE_FOCUSED_APP (on screen) and GAMESCOPE_FOCUSABLE_APPS (have a window);
//   - Steam writes GAMESCOPECTRL_BASELAYER_APPID too (its own codes, e.g. when it starts): those
//     writes are reported to the Manager, which translates or undoes them (steam.go).
//
// Settings → Display uses the same root window properties as Steam (RootString, SetRootString,
// SetRootCardinal): the screen's modes, the mode "nudge" and the keyboard layout.
//
// Connections are opened on first use and again after an error (gamescope restarted).
type Gamescope struct {
	Display string
	Log     *slog.Logger

	mu    sync.Mutex
	conn  *xgb.Conn
	root  xproto.Window
	atoms map[string]xproto.Atom

	watchOnce sync.Once
	stop      chan struct{}
	written   []written // owneetd's recent writes of GAMESCOPECTRL_BASELAYER_APPID

	repaintStop chan struct{} // the overlay that keeps gamescope composing runs (repaint.go)
}

// written is a value owneetd wrote: its change notification is not someone else's request.
type written struct {
	ids []uint32
	at  time.Time
}

func sameIDs(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ours tells whether a value is one owneetd wrote in the last few seconds; callers hold g.mu.
func (g *Gamescope) ours(ids []uint32) bool {
	keep := g.written[:0]
	found := false
	for _, w := range g.written {
		if time.Since(w.at) < 5*time.Second {
			keep = append(keep, w)
			found = found || sameIDs(w.ids, ids)
		}
	}
	g.written = keep
	return found
}

var atomNames = []string{"GAMESCOPE_FOCUSED_APP", "GAMESCOPE_FOCUSABLE_APPS",
	"GAMESCOPECTRL_BASELAYER_APPID", "STEAM_GAME", "_NET_WM_PID",
	"GAMESCOPE_DISPLAY_MODE_LIST_EXTERNAL", "GAMESCOPE_DISPLAY_MODE_NUDGE", "GAMESCOPE_KEYBOARD_LAYOUT"}

// dial opens a connection to the X server and looks up the atoms.
func (g *Gamescope) dial() (*xgb.Conn, xproto.Window, map[string]xproto.Atom, error) {
	if g.Display == "" {
		return nil, 0, nil, errors.New("no X11 display")
	}
	conn, err := xgb.NewConnDisplay(g.Display)
	if err != nil {
		return nil, 0, nil, err
	}
	atoms := map[string]xproto.Atom{}
	for _, name := range atomNames {
		reply, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply()
		if err != nil {
			conn.Close()
			return nil, 0, nil, err
		}
		atoms[name] = reply.Atom
	}
	return conn, xproto.Setup(conn).DefaultScreen(conn).Root, atoms, nil
}

func (g *Gamescope) connect() error {
	if g.conn != nil {
		return nil
	}
	conn, root, atoms, err := g.dial()
	if err != nil {
		return err
	}
	g.conn, g.root, g.atoms = conn, root, atoms
	return nil
}

func (g *Gamescope) reset() {
	if g.conn != nil {
		g.conn.Close()
	}
	g.conn = nil
}

func cardinals(conn *xgb.Conn, w xproto.Window, atom xproto.Atom) ([]uint32, error) {
	reply, err := xproto.GetProperty(conn, false, w, atom, xproto.AtomCardinal, 0, 4096).Reply()
	if err != nil {
		return nil, err
	}
	out := make([]uint32, 0, len(reply.Value)/4)
	for i := 0; i+4 <= len(reply.Value); i += 4 {
		out = append(out, xgb.Get32(reply.Value[i:]))
	}
	return out, nil
}

func setCardinals(conn *xgb.Conn, w xproto.Window, atom xproto.Atom, values []uint32) error {
	data := make([]byte, 4*len(values))
	for i, v := range values {
		xgb.Put32(data[4*i:], v)
	}
	return xproto.ChangePropertyChecked(conn, xproto.PropModeReplace, w, atom, xproto.AtomCardinal, 32,
		uint32(len(values)), data).Check()
}

func (g *Gamescope) rootCardinals(name string) ([]uint32, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.connect(); err != nil {
		return nil, err
	}
	v, err := cardinals(g.conn, g.root, g.atoms[name])
	if err != nil {
		g.reset()
	}
	return v, err
}

// RootString reads a text property of the root window ("" when it is not set).
func (g *Gamescope) RootString(name string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.connect(); err != nil {
		return "", err
	}
	reply, err := xproto.GetProperty(g.conn, false, g.root, g.atoms[name], xproto.AtomString, 0, 65536).Reply()
	if err != nil {
		g.reset()
		return "", err
	}
	return strings.TrimRight(string(reply.Value), "\x00"), nil
}

// SetRootString sets a text property of the root window.
func (g *Gamescope) SetRootString(name, value string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.connect(); err != nil {
		return err
	}
	err := xproto.ChangePropertyChecked(g.conn, xproto.PropModeReplace, g.root, g.atoms[name], xproto.AtomString, 8,
		uint32(len(value)), []byte(value)).Check()
	if err != nil {
		g.reset()
	}
	return err
}

// SetRootCardinal sets a number property of the root window.
func (g *Gamescope) SetRootCardinal(name string, value uint32) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.connect(); err != nil {
		return err
	}
	err := setCardinals(g.conn, g.root, g.atoms[name], []uint32{value})
	if err != nil {
		g.reset()
	}
	return err
}

// Focused returns the app id on screen (0: none).
func (g *Gamescope) Focused() (uint32, error) {
	v, err := g.rootCardinals("GAMESCOPE_FOCUSED_APP")
	if err != nil || len(v) == 0 {
		return 0, err
	}
	return v[0], nil
}

// Visible returns the app ids that have a window.
func (g *Gamescope) Visible() ([]uint32, error) {
	return g.rootCardinals("GAMESCOPE_FOCUSABLE_APPS")
}

// Show asks gamescope to show the first of these apps that has a window.
func (g *Gamescope) Show(appIDs []uint32) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.connect(); err != nil {
		return err
	}
	g.written = append(g.written, written{ids: append([]uint32(nil), appIDs...), at: time.Now()})
	err := setCardinals(g.conn, g.root, g.atoms["GAMESCOPECTRL_BASELAYER_APPID"], appIDs)
	if err != nil {
		g.reset()
	}
	return err
}

// Close stops watching and closes the connections.
func (g *Gamescope) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reset()
	if g.stop != nil {
		close(g.stop)
		g.stop = nil
	}
	if g.repaintStop != nil {
		close(g.repaintStop)
		g.repaintStop = nil
	}
}

// Watch tags every window, present and future, with the app id that tag returns for its process
// (0: leave it alone), and reports what someone else (Steam) asks gamescope to show. Until
// something is chosen, the home screen is shown. It runs until Close.
func (g *Gamescope) Watch(tag func(pid int) uint32, request func(appIDs []uint32)) {
	g.watchOnce.Do(func() {
		g.mu.Lock()
		g.stop = make(chan struct{})
		stop := g.stop
		g.mu.Unlock()
		go g.watch(tag, request, stop)
	})
}

func (g *Gamescope) watch(tag func(pid int) uint32, request func([]uint32), stop <-chan struct{}) {
	for {
		if err := g.watchConn(tag, request, stop); err != nil && g.Log != nil {
			g.Log.Debug("gamescope window watch interrupted", "err", err)
		}
		select {
		case <-stop:
			return
		case <-time.After(time.Second):
		}
	}
}

// watchConn watches until the connection fails or stop is closed.
func (g *Gamescope) watchConn(tag func(pid int) uint32, request func([]uint32), stop <-chan struct{}) error {
	conn, root, atoms, err := g.dial()
	if err != nil {
		return err
	}
	var closeOnce sync.Once
	closeConn := func() { closeOnce.Do(conn.Close) }
	defer closeConn()
	done := make(chan struct{})
	defer close(done)
	go func() { // stop interrupts WaitForEvent
		select {
		case <-stop:
			closeConn()
		case <-done:
		}
	}()

	// Map events of the top-level windows, and changes of the root window's properties.
	if err := xproto.ChangeWindowAttributesChecked(conn, root, xproto.CwEventMask,
		[]uint32{xproto.EventMaskSubstructureNotify | xproto.EventMaskPropertyChange}).Check(); err != nil {
		return err
	}
	if current, _ := cardinals(conn, root, atoms["GAMESCOPECTRL_BASELAYER_APPID"]); len(current) == 0 {
		// gamescope --steam shows nothing until told: start with the home screen.
		g.mu.Lock()
		g.written = append(g.written, written{ids: []uint32{HomeAppID}, at: time.Now()})
		g.mu.Unlock()
		setCardinals(conn, root, atoms["GAMESCOPECTRL_BASELAYER_APPID"], []uint32{HomeAppID})
	}
	// X-Resource tells the process of windows without _NET_WM_PID (Xwayland has it)
	xres := res.Init(conn) == nil
	tree, err := xproto.QueryTree(conn, root).Reply()
	if err != nil {
		return err
	}
	for _, w := range tree.Children {
		g.tagWindow(conn, atoms, w, tag, xres)
	}
	for {
		ev, xerr := conn.WaitForEvent()
		if ev == nil && xerr == nil {
			return errors.New("connection closed")
		}
		switch e := ev.(type) {
		case xproto.MapNotifyEvent:
			if g.tagWindow(conn, atoms, e.Window, tag, xres) != HomeAppID {
				clearWindow(conn, e.Window)
			}
		case xproto.PropertyNotifyEvent:
			if e.Atom != atoms["GAMESCOPECTRL_BASELAYER_APPID"] || e.State != xproto.PropertyNewValue || request == nil {
				continue
			}
			ids, err := cardinals(conn, root, e.Atom)
			if err != nil {
				continue
			}
			g.mu.Lock()
			mine := g.ours(ids)
			g.mu.Unlock()
			if !mine {
				request(ids)
			}
		}
	}
}

// tagWindow tags a window with its app's id and returns that id (0: not an app's window).
func (g *Gamescope) tagWindow(conn *xgb.Conn, atoms map[string]xproto.Atom, w xproto.Window, tag func(pid int) uint32, xres bool) uint32 {
	pid, appID := 0, uint32(0)
	if pids, err := cardinals(conn, w, atoms["_NET_WM_PID"]); err == nil && len(pids) > 0 && pids[0] > 0 {
		pid = int(pids[0])
		appID = tag(pid)
	}
	// No _NET_WM_PID, or one that leads to no app (e.g. a process id from inside a container):
	// the process the X server knows
	if appID == 0 && xres {
		if p := clientPID(conn, w); p > 0 && p != pid {
			pid = p
			appID = tag(pid)
		}
	}
	if appID == 0 {
		return 0
	}
	if current, _ := cardinals(conn, w, atoms["STEAM_GAME"]); len(current) == 1 && current[0] == appID {
		return appID
	}
	if err := setCardinals(conn, w, atoms["STEAM_GAME"], []uint32{appID}); err == nil && g.Log != nil {
		g.Log.Debug("window tagged", "window", w, "pid", pid, "app_id", appID)
	}
	return appID
}

// clientPID is the process of the program that made a window, as the X server knows it
// (X-Resource): for windows without _NET_WM_PID, e.g. Unity 5 games such as CAYNE, whose window
// stayed behind the home screen (FrankenPC, 2026-10-16). 0 when unknown.
func clientPID(conn *xgb.Conn, w xproto.Window) int {
	spec := res.ClientIdSpec{Client: uint32(w), Mask: res.ClientIdMaskLocalClientPID}
	reply, err := res.QueryClientIds(conn, 1, []res.ClientIdSpec{spec}).Reply()
	if err != nil || reply == nil {
		return 0
	}
	for _, id := range reply.Ids {
		if id.Spec.Mask&res.ClientIdMaskLocalClientPID != 0 && len(id.Value) > 0 {
			return int(id.Value[0])
		}
	}
	return 0
}

// clearWindow gives a newly shown window a background (black, transparent with an alpha
// channel) and clears it; the server then asks its program to draw it again (Expose). A window
// without a background (X11's default, e.g. Steam's) shows whatever its new memory held until
// the program has drawn all of it: with the NVIDIA driver, pieces of older pictures (glyph
// strips, copies of Steam's own pictures; FrankenPC, 2026-10-13). With a background, parts
// uncovered later (a larger window) are cleared too.
func clearWindow(conn *xgb.Conn, w xproto.Window) {
	xproto.ChangeWindowAttributes(conn, w, xproto.CwBackPixel, []uint32{0})
	xproto.ClearArea(conn, true, w, 0, 0, 0, 0)
}

// GamescopeFocus returns a Manager.FocusFor that keeps one Gamescope per display (the display
// changes when the session restarts).
func GamescopeFocus(log *slog.Logger) func(Session) Focus {
	var mu sync.Mutex
	var current *Gamescope
	return func(s Session) Focus {
		mu.Lock()
		defer mu.Unlock()
		if current == nil || current.Display != s.Display {
			if current != nil {
				current.Close()
			}
			current = &Gamescope{Display: s.Display, Log: log}
		}
		return current
	}
}
