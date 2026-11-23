// SPDX-License-Identifier: GPL-3.0-or-later

// owneet-xprobe is a test tool (not shipped in the image): it prints what gamescope's X server
// knows about windows and focus, to see how Steam and owneetd share the screen
// ([[step-4.2-steam-integration]]).
//
//	owneet-xprobe                 the root window's GAMESCOPE* / STEAM* properties and the active window
//	                              (_NET_ACTIVE_WINDOW, which Proton's Wine reads under gamescope to know
//	                              whether its window is in front again), every mapped window (size,
//	                              position, the properties gamescope reads, WM_STATE) and the unmapped
//	                              windows of an app (STEAM_GAME set: a game's window it hid or iconified)
//	owneet-xprobe -watch 30s      also print the root properties each time they change
//
// It reads DISPLAY from the environment (the console session's, e.g. DISPLAY=:0).
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func atomName(conn *xgb.Conn, a xproto.Atom) string {
	reply, err := xproto.GetAtomName(conn, a).Reply()
	if err != nil {
		return fmt.Sprint(a)
	}
	return reply.Name
}

func atom(conn *xgb.Conn, name string) xproto.Atom {
	reply, err := xproto.InternAtom(conn, true, uint16(len(name)), name).Reply()
	if err != nil {
		return 0
	}
	return reply.Atom
}

// value prints a property as numbers (CARDINAL, WINDOW…) or text
func value(conn *xgb.Conn, w xproto.Window, a xproto.Atom) string {
	reply, err := xproto.GetProperty(conn, false, w, a, xproto.GetPropertyTypeAny, 0, 1024).Reply()
	if err != nil || reply.ValueLen == 0 {
		return ""
	}
	if reply.Format == 8 {
		return strings.ReplaceAll(string(reply.Value), "\x00", "|")
	}
	var parts []string
	for i := 0; i+4 <= len(reply.Value) && reply.Format == 32; i += 4 {
		parts = append(parts, fmt.Sprint(xgb.Get32(reply.Value[i:])))
	}
	return strings.Join(parts, ",")
}

func rootProps(conn *xgb.Conn, root xproto.Window) map[string]string {
	out := map[string]string{}
	reply, err := xproto.ListProperties(conn, root).Reply()
	if err != nil {
		return out
	}
	for _, a := range reply.Atoms {
		name := atomName(conn, a)
		if strings.HasPrefix(name, "GAMESCOPE") || strings.HasPrefix(name, "STEAM") || name == "_NET_ACTIVE_WINDOW" || name == "_NET_SUPPORTED" {
			v := value(conn, root, a)
			if name == "_NET_SUPPORTED" { // atoms: their names
				var names []string
				for _, n := range strings.Split(v, ",") {
					var id uint32
					fmt.Sscan(n, &id)
					names = append(names, atomName(conn, xproto.Atom(id)))
				}
				v = strings.Join(names, " ")
			}
			out[name] = v
		}
	}
	return out
}

func printProps(props map[string]string) {
	names := make([]string, 0, len(props))
	for n := range props {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		v := props[n]
		if len(v) > 120 && n != "_NET_SUPPORTED" {
			v = v[:120] + "…"
		}
		fmt.Printf("  %s = %s\n", n, v)
	}
}

// windows prints the top-level windows: the mapped ones (mapped true), or the unmapped ones that
// belong to an app (STEAM_GAME set)
func windows(conn *xgb.Conn, root xproto.Window, mapped bool) {
	tree, err := xproto.QueryTree(conn, root).Reply()
	if err != nil {
		return
	}
	pid, name, class := atom(conn, "_NET_WM_PID"), atom(conn, "WM_NAME"), atom(conn, "WM_CLASS")
	steamGame := atom(conn, "STEAM_GAME")
	// bottom to top, as they are stacked
	for _, w := range tree.Children {
		attrs, err := xproto.GetWindowAttributes(conn, w).Reply()
		if err != nil || (attrs.MapState == xproto.MapStateViewable) != mapped {
			continue
		}
		if !mapped && (steamGame == 0 || value(conn, w, steamGame) == "") {
			continue
		}
		geo := ""
		if g, err := xproto.GetGeometry(conn, xproto.Drawable(w)).Reply(); err == nil {
			geo = fmt.Sprintf("%dx%d+%d+%d depth=%d", g.Width, g.Height, g.X, g.Y, g.Depth)
		}
		fmt.Printf("  window %d %s override=%v pid=%s class=%q name=%q\n", w, geo, attrs.OverrideRedirect,
			value(conn, w, pid), value(conn, w, class), value(conn, w, name))
		// the properties gamescope reads (STEAM_GAME, STEAM_OVERLAY, opacity, window type…)
		if props, err := xproto.ListProperties(conn, w).Reply(); err == nil {
			for _, a := range props.Atoms {
				n := atomName(conn, a)
				if strings.HasPrefix(n, "STEAM") || strings.HasPrefix(n, "GAMESCOPE") ||
					strings.HasPrefix(n, "_NET_WM_WINDOW") || n == "_NET_WM_STATE" || n == "WM_STATE" {
					fmt.Printf("      %s = %s\n", n, value(conn, w, a))
				}
			}
		}
	}
}

func main() {
	watch := flag.Duration("watch", 0, "also print the root properties when they change, for this long")
	flag.Parse()
	conn, err := xgb.NewConn()
	if err != nil {
		fmt.Fprintln(os.Stderr, "owneet-xprobe:", err)
		os.Exit(1)
	}
	defer conn.Close()
	root := xproto.Setup(conn).DefaultScreen(conn).Root
	fmt.Println("root:")
	before := rootProps(conn, root)
	printProps(before)
	fmt.Println("mapped windows:")
	windows(conn, root, true)
	fmt.Println("unmapped windows of apps:")
	windows(conn, root, false)
	if *watch <= 0 {
		return
	}
	xproto.ChangeWindowAttributes(conn, root, xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange})
	end := time.Now().Add(*watch)
	go func() { time.Sleep(*watch); conn.Close() }()
	for time.Now().Before(end) {
		ev, err := conn.WaitForEvent()
		if ev == nil && err == nil {
			return
		}
		if p, ok := ev.(xproto.PropertyNotifyEvent); ok {
			name := atomName(conn, p.Atom)
			if strings.HasPrefix(name, "GAMESCOPE") || strings.HasPrefix(name, "STEAM") || name == "_NET_ACTIVE_WINDOW" {
				fmt.Printf("%s %s = %s\n", time.Now().Format("15:04:05.000"), name, value(conn, root, p.Atom))
			}
		}
	}
}
