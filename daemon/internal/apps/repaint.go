// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"bufio"
	"errors"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Repainting Steam's window on NVIDIA ([[step-4.2c-steam-without-its-interface]]): with NVIDIA's
// driver, parts of Steam's window stay stale or unfinished inside gamescope while games are fine
// (FrankenPC). Another console session saw the same and found that a layer over the screen that
// changes every frame (MangoHud's, made transparent) makes gamescope compose every frame again,
// and the stale parts go. owneetd does that without MangoHud: while Steam's own window is on
// screen, a window of one all but transparent pixel, marked as an external overlay for gamescope
// (as mangoapp's), changes every frame. Steam's window gets a little slower. On every NVIDIA PC
// since the FrankenPC test (owner, 2026-10-15), until Valve fixes it; test switch
// owneet.debug.steam_repaint=off turns it off (kernel command line or owneet-debug.txt on the
// OWNEETLOG stick).

// repaintFrame: how often the overlay changes (about 60 times a second).
const repaintFrame = 16 * time.Millisecond

// Repainter is a Focus that can keep gamescope composing every frame.
type Repainter interface {
	Repaint(on bool)
}

// Repaint starts or stops the changing overlay (its own X11 connection).
func (g *Gamescope) Repaint(on bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if on == (g.repaintStop != nil) {
		return
	}
	if !on {
		close(g.repaintStop)
		g.repaintStop = nil
		return
	}
	stop := make(chan struct{})
	g.repaintStop = stop
	go func() {
		if err := g.repaint(stop); err != nil && g.Log != nil {
			g.Log.Warn("cannot keep gamescope composing", "err", err)
		}
	}()
}

func (g *Gamescope) repaint(stop <-chan struct{}) error {
	if g.Display == "" {
		return errors.New("no X11 display")
	}
	conn, err := xgb.NewConnDisplay(g.Display)
	if err != nil {
		return err
	}
	defer conn.Close()
	screen := xproto.Setup(conn).DefaultScreen(conn)
	// a visual with an alpha channel: the pixel is all but transparent
	var visual xproto.Visualid
	for _, d := range screen.AllowedDepths {
		if d.Depth != 32 {
			continue
		}
		for _, v := range d.Visuals {
			if v.Class == xproto.VisualClassTrueColor {
				visual = v.VisualId
				break
			}
		}
	}
	if visual == 0 {
		return errors.New("no 32-bit visual")
	}
	atom, err := xproto.InternAtom(conn, false, uint16(len("GAMESCOPE_EXTERNAL_OVERLAY")), "GAMESCOPE_EXTERNAL_OVERLAY").Reply()
	if err != nil {
		return err
	}
	cmap, err := xproto.NewColormapId(conn)
	if err != nil {
		return err
	}
	if err := xproto.CreateColormapChecked(conn, xproto.ColormapAllocNone, cmap, screen.Root, visual).Check(); err != nil {
		return err
	}
	win, err := xproto.NewWindowId(conn)
	if err != nil {
		return err
	}
	if err := xproto.CreateWindowChecked(conn, 32, win, screen.Root, 0, 0, 1, 1, 0, xproto.WindowClassInputOutput, visual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwOverrideRedirect|xproto.CwColormap,
		[]uint32{0, 0, 1, uint32(cmap)}).Check(); err != nil {
		return err
	}
	defer xproto.DestroyWindow(conn, win)
	if err := setCardinals(conn, win, atom.Atom, []uint32{1}); err != nil {
		return err
	}
	if err := xproto.MapWindowChecked(conn, win).Check(); err != nil {
		return err
	}
	gc, err := xproto.NewGcontextId(conn)
	if err != nil {
		return err
	}
	if err := xproto.CreateGCChecked(conn, gc, xproto.Drawable(win), xproto.GcForeground, []uint32{0}).Check(); err != nil {
		return err
	}
	if g.Log != nil {
		g.Log.Info("keeping gamescope composing while Steam's window is on screen")
	}
	tick := time.NewTicker(repaintFrame)
	defer tick.Stop()
	alpha := uint32(0)
	for {
		select {
		case <-stop:
			if g.Log != nil {
				g.Log.Info("gamescope composing as usual again")
			}
			return nil
		case <-tick.C:
			alpha ^= 0x01000000 // transparent, or alpha 1/255: invisible either way
			xproto.ChangeGC(conn, gc, xproto.GcForeground, []uint32{alpha})
			if err := xproto.PolyFillRectangleChecked(conn, xproto.Drawable(win), gc,
				[]xproto.Rectangle{{X: 0, Y: 0, Width: 1, Height: 1}}).Check(); err != nil {
				return err
			}
		}
	}
}

// DebugOption reads a test switch, owneet.debug.NAME=VALUE: on the kernel command line, or in
// owneet-debug.txt on the OWNEETLOG stick (its value wins), as the session's own switches.
func DebugOption(name string) string {
	re := regexp.MustCompile(`(^|\s)owneet\.debug\.` + regexp.QuoteMeta(name) + `=([a-z]+)`)
	value := ""
	if data, err := os.ReadFile("/proc/cmdline"); err == nil {
		if m := re.FindStringSubmatch(string(data)); m != nil {
			value = m[2]
		}
	}
	if f, err := os.Open("/run/owneet-testlog/owneet-debug.txt"); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if m := re.FindStringSubmatch(strings.TrimSpace(sc.Text())); m != nil {
				value = m[2]
			}
		}
	}
	return value
}

// steamRepaint: NVIDIA's driver runs the screen, and the test switch does not turn it off.
func (m *Manager) steamRepaint() bool {
	return m.NVIDIA() && DebugOption("steam_repaint") != "off"
}

// updateRepaint keeps gamescope composing while Steam's own window is on screen (not its games).
func (m *Manager) updateRepaint(focus string) {
	m.mu.Lock()
	m.init()
	want := m.SteamRepaint
	m.mu.Unlock()
	sess, err := m.Session()
	if err != nil {
		return
	}
	r, ok := m.focus(sess).(Repainter)
	if !ok {
		return
	}
	r.Repaint(focus == SteamID && want())
}
