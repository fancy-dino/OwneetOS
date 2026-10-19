// SPDX-License-Identifier: GPL-3.0-or-later

package pointer

import (
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/evdev"
	"github.com/fancy-dino/OwneetOS/daemon/internal/gamepad"
	"github.com/fancy-dino/OwneetOS/daemon/internal/sdldb"
)

// Codes used when SDL_GameControllerDB does not know a controller: the Linux drivers' usual
// layout (xpad and hid-microsoft give X as BTN_NORTH and Y as BTN_WEST).
const (
	btnSouth = 0x130
	btnNorth = 0x133
	btnWest  = 0x134
	btnUp    = 0x220 // BTN_DPAD_UP, DOWN, LEFT, RIGHT
	absRY    = 0x04
	absHat0X = 0x10
	absHat0Y = 0x11
)

// The controls the pointer uses, by their SDL names.
var controls = []string{"a", "x", "y", "leftx", "lefty", "righty", "dpup", "dpdown", "dpleft", "dpright"}

// binding is where a control comes from on one controller.
type binding struct {
	key    int     // a button's code (or -1)
	abs    int     // an axis's code (or -1)
	sign   float64 // for a hat or a half axis: the direction that presses it
	invert bool
}

// mapping is a controller's controls, from SDL_GameControllerDB (as the console reads Guide) or
// the usual layout.
func mapping(d *evdev.Device, db *sdldb.DB) map[string]binding {
	out := map[string]binding{}
	if m, ok := db.Mapping(d.ID); ok {
		axes := axisOrder(d)
		for _, c := range controls {
			ref, ok := m[c]
			if !ok {
				continue
			}
			if b, ok := parseRef(d, ref, axes); ok {
				out[c] = b
			}
		}
	}
	fallback := map[string]binding{
		"a": {key: btnSouth, abs: -1}, "x": {key: btnNorth, abs: -1}, "y": {key: btnWest, abs: -1},
		"leftx": {key: -1, abs: evdev.AbsX}, "lefty": {key: -1, abs: evdev.AbsY}, "righty": {key: -1, abs: absRY},
	}
	if d.HasAbs(absHat0X) {
		fallback["dpup"] = binding{key: -1, abs: absHat0Y, sign: -1}
		fallback["dpdown"] = binding{key: -1, abs: absHat0Y, sign: 1}
		fallback["dpleft"] = binding{key: -1, abs: absHat0X, sign: -1}
		fallback["dpright"] = binding{key: -1, abs: absHat0X, sign: 1}
	} else {
		for i, c := range []string{"dpup", "dpdown", "dpleft", "dpright"} {
			fallback[c] = binding{key: btnUp + i, abs: -1}
		}
	}
	for c, b := range fallback {
		if _, ok := out[c]; !ok && ((b.key >= 0 && d.HasKey(b.key)) || (b.abs >= 0 && d.HasAbs(b.abs))) {
			out[c] = b
		}
	}
	return out
}

// axisOrder: SDL numbers a controller's axes in the order of their codes, hats left out.
func axisOrder(d *evdev.Device) []int {
	var axes []int
	for c := 0; c <= evdev.AbsMax; c++ {
		if c >= absHat0X && c <= 0x17 {
			continue
		}
		if d.HasAbs(c) {
			axes = append(axes, c)
		}
	}
	return axes
}

// parseRef reads an SDL reference: "b2" (a button), "a1" / "+a2" / "-a2" / "a1~" (an axis),
// "h0.1" (a hat's direction: 1 up, 2 right, 4 down, 8 left).
func parseRef(d *evdev.Device, ref string, axes []int) (binding, bool) {
	switch {
	case strings.HasPrefix(ref, "b"):
		code, ok := sdldb.ButtonCode(d.Keys(), ref)
		return binding{key: code, abs: -1}, ok
	case strings.HasPrefix(ref, "h"):
		hat, dir, ok := strings.Cut(ref[1:], ".")
		h, err1 := strconv.Atoi(hat)
		m, err2 := strconv.Atoi(dir)
		if !ok || err1 != nil || err2 != nil {
			return binding{}, false
		}
		x, y := absHat0X+2*h, absHat0Y+2*h
		switch m {
		case 1:
			return binding{key: -1, abs: y, sign: -1}, true
		case 4:
			return binding{key: -1, abs: y, sign: 1}, true
		case 8:
			return binding{key: -1, abs: x, sign: -1}, true
		case 2:
			return binding{key: -1, abs: x, sign: 1}, true
		}
		return binding{}, false
	default:
		b := binding{key: -1}
		r := ref
		if strings.HasSuffix(r, "~") {
			b.invert, r = true, strings.TrimSuffix(r, "~")
		}
		if strings.HasPrefix(r, "+") {
			b.sign, r = 1, r[1:]
		} else if strings.HasPrefix(r, "-") {
			b.sign, r = -1, r[1:]
		}
		n, err := strconv.Atoi(strings.TrimPrefix(r, "a"))
		if !strings.HasPrefix(r, "a") || err != nil || n < 0 || n >= len(axes) {
			return binding{}, false
		}
		b.abs = axes[n]
		return b, true
	}
}

// ignored: devices that are not a real controller in the user's hands: Steam's virtual
// controller (it repeats the real one), owneetd's own virtual devices.
func ignored(d *evdev.Device) bool {
	return d.ID.Bustype == evdev.BusVirtual || (d.ID.Vendor == 0x28de && strings.Contains(strings.ToLower(d.Name), "virtual"))
}

// ReadPads reads every controller until stop is closed (new ones are picked up every 2 s) and
// sends its controls to in: buttons 0 / 1, axes -1…1. Nothing is grabbed: games and apps get the
// same input as always.
func ReadPads(db *sdldb.DB, log *slog.Logger) func(in chan<- Input, stop <-chan struct{}) {
	return func(in chan<- Input, stop <-chan struct{}) {
		open := map[string]*evdev.Device{}
		gone := make(chan string, 8)
		defer func() {
			for _, d := range open {
				d.Close()
			}
		}()
		scan := func() {
			paths, _ := filepath.Glob("/dev/input/event*")
			for _, p := range paths {
				if _, ok := open[p]; ok {
					continue
				}
				d, err := evdev.Open(p)
				if err != nil {
					continue
				}
				if !gamepad.IsGamepad(d) || ignored(d) {
					d.Close()
					continue
				}
				open[p] = d
				log.Debug("pointer: reading a controller", "name", d.Name, "path", p)
				go read(d, mapping(d, db), in, stop, gone)
			}
		}
		scan()
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case p := <-gone:
				if d, ok := open[p]; ok {
					d.Close()
					delete(open, p)
				}
			case <-t.C:
				scan()
			}
		}
	}
}

// read turns one controller's events into controls.
func read(d *evdev.Device, b map[string]binding, in chan<- Input, stop <-chan struct{}, gone chan<- string) {
	ranges := map[int][2]int32{}
	for _, c := range b {
		if c.abs >= 0 {
			if lo, hi, err := d.AbsRange(c.abs); err == nil && hi > lo {
				ranges[c.abs] = [2]int32{lo, hi}
			}
		}
	}
	norm := func(code int, v int32) float64 {
		r, ok := ranges[code]
		if !ok || code == absHat0X || code == absHat0Y || (code >= absHat0X && code <= 0x17) {
			return float64(v) // hats: -1, 0, 1
		}
		return 2*float64(v-r[0])/float64(r[1]-r[0]) - 1
	}
	for {
		evs, err := d.Read()
		if err != nil {
			select {
			case gone <- d.Path:
			case <-stop:
			}
			return
		}
		for _, ev := range evs {
			for name, c := range b {
				var v float64
				switch {
				case ev.Type == evdev.EvKey && c.key >= 0 && int(ev.Code) == c.key:
					v = float64(min(ev.Value, 1))
				case ev.Type == evdev.EvAbs && c.abs >= 0 && int(ev.Code) == c.abs:
					v = norm(c.abs, ev.Value)
					if c.invert {
						v = -v
					}
					if c.sign != 0 { // a half axis or a hat's direction: pressed that way
						if v*c.sign > 0.5 {
							v = 1
						} else {
							v = 0
						}
					}
				default:
					continue
				}
				select {
				case in <- Input{Control: name, Value: v}:
				case <-stop:
					return
				}
			}
		}
	}
}
