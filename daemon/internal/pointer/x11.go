// SPDX-License-Identifier: GPL-3.0-or-later

package pointer

import (
	"errors"
	"strings"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xfixes"
	"github.com/jezek/xgb/xproto"
)

// X is gamescope's X server, read for the pointer: GAMESCOPE_FOCUSED_APP and
// GAMESCOPE_FOCUSED_WINDOW on the root window (what is on screen), and the pointer's image
// through XFixes (gamescope draws the cursor the apps set, so the X server has it).
type X struct {
	conn   *xgb.Conn
	root   xproto.Window
	app    xproto.Atom
	window xproto.Atom
}

// DialX connects to the X server of the console's session (its DISPLAY).
func DialX(display string) (Screen, error) {
	if display == "" {
		return nil, errors.New("no X11 display")
	}
	conn, err := xgb.NewConnDisplay(display)
	if err != nil {
		return nil, err
	}
	x := &X{conn: conn, root: xproto.Setup(conn).DefaultScreen(conn).Root}
	for name, dst := range map[string]*xproto.Atom{"GAMESCOPE_FOCUSED_APP": &x.app, "GAMESCOPE_FOCUSED_WINDOW": &x.window} {
		reply, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply()
		if err != nil {
			conn.Close()
			return nil, err
		}
		*dst = reply.Atom
	}
	if err := xfixes.Init(conn); err != nil {
		conn.Close()
		return nil, err
	}
	if _, err := xfixes.QueryVersion(conn, 4, 0).Reply(); err != nil {
		conn.Close()
		return nil, err
	}
	return x, nil
}

func (x *X) cardinal(atom xproto.Atom) (uint32, error) {
	reply, err := xproto.GetProperty(x.conn, false, x.root, atom, xproto.AtomCardinal, 0, 1).Reply()
	if err != nil {
		return 0, err
	}
	if reply.Format != 32 || len(reply.Value) < 4 {
		return 0, nil
	}
	v := reply.Value
	return uint32(v[0]) | uint32(v[1])<<8 | uint32(v[2])<<16 | uint32(v[3])<<24, nil
}

func (x *X) Focused() (uint32, uint32, error) {
	app, err := x.cardinal(x.app)
	if err != nil {
		return 0, 0, err
	}
	win, err := x.cardinal(x.window)
	return app, win, err
}

// textCursorNames: the names toolkits give the text cursor (Qt, GTK, Chromium, X core cursors).
var textCursorNames = []string{"text", "xterm", "ibeam", "vertical-text"}

// TextCursor: the pointer is the text "I": by its name when the app named it, else by its shape
// (a narrow, tall outline: Wine and Proton make nameless cursors from the Windows ones).
func (x *X) TextCursor() (bool, error) {
	reply, err := xfixes.GetCursorImageAndName(x.conn).Reply()
	if err != nil {
		return false, err
	}
	name := strings.ToLower(reply.Name)
	for _, n := range textCursorNames {
		if name == n {
			return true, nil
		}
	}
	if name != "" {
		return false, nil // a named cursor that is not the text one (e.g. "left_ptr", "pointer")
	}
	return textShape(int(reply.Width), int(reply.Height), reply.CursorImage), nil
}

// textShape: the visible pixels (ARGB, alpha above half) make the text "I": tall and narrow
// (at least 1.4 times taller than wide) and mirrored left to right (an arrow or a hand is not).
func textShape(w, h int, argb []uint32) bool {
	seen := func(x, y int) bool { return y*w+x < len(argb) && argb[y*w+x]>>24 > 0x80 }
	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if seen(x, y) {
				minX, maxX = min(minX, x), max(maxX, x)
				minY, maxY = min(minY, y), max(maxY, y)
			}
		}
	}
	if maxX < 0 {
		return false
	}
	bw, bh := maxX-minX+1, maxY-minY+1
	if bh < 12 || bw*14 > bh*10 {
		return false
	}
	same, all := 0, 0
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if seen(x, y) {
				all++
				if seen(minX+maxX-x, y) {
					same++
				}
			}
		}
	}
	return same*100 >= all*85
}

func (x *X) Close() { x.conn.Close() }
