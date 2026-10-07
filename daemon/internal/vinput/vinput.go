// SPDX-License-Identifier: GPL-3.0-or-later

// Package vinput provides the virtual keyboard and mouse that the on-screen keyboard and other
// gamepad features use to type text and press keys in the focused app (roadmap step 2.4).
package vinput

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/evdev"
	"github.com/fancy-dino/OwneetOS/daemon/internal/uinput"
)

// Device names, so the virtual devices can be recognised (and ignored by the controller tracker).
const (
	KeyboardName = "OwneetOS Virtual Keyboard"
	MouseName    = "OwneetOS Virtual Mouse"
)

// UnsupportedCharError is returned when a character cannot be typed with the current layout.
// Nothing is typed in that case.
type UnsupportedCharError struct {
	Char   rune
	Layout string
}

func (e *UnsupportedCharError) Error() string {
	return fmt.Sprintf("character %q cannot be typed with the %q keyboard layout", e.Char, e.Layout)
}

// ErrUnknownKey is returned for key or modifier names PressKey does not know.
var ErrUnknownKey = errors.New("unknown key name")

// Input owns the virtual keyboard and mouse.
type Input struct {
	mu         sync.Mutex
	layoutName string
	layout     Layout
	kbd        *uinput.Device
	mouse      *uinput.Device
	// KeyDelay is the pause between key strokes, so apps do not miss fast typing.
	KeyDelay time.Duration
}

// New creates the virtual devices. layout is an XKB layout name ("us", "it").
func New(layout string) (*Input, error) {
	mk, ok := Layouts[layout]
	if !ok {
		return nil, fmt.Errorf("unsupported keyboard layout %q (supported: %s)", layout, strings.Join(LayoutNames(), ", "))
	}
	keys := make([]int, 0, 128)
	for c := 1; c <= 127; c++ { // the main keyboard block, enough for every layout above
		keys = append(keys, c)
	}
	kbd, err := uinput.Create(uinput.Spec{
		Name: KeyboardName,
		ID:   evdev.ID{Bustype: evdev.BusVirtual, Vendor: 0x4f57, Product: 0x0001, Version: 1},
		Keys: keys,
	})
	if err != nil {
		return nil, fmt.Errorf("virtual keyboard: %w", err)
	}
	mouse, err := uinput.Create(uinput.Spec{
		Name: MouseName,
		ID:   evdev.ID{Bustype: evdev.BusVirtual, Vendor: 0x4f57, Product: 0x0002, Version: 1},
		Keys: []int{btnLeft, btnRight, btnMiddle},
		Rel:  []int{relX, relY, relWheel},
	})
	if err != nil {
		kbd.Close()
		return nil, fmt.Errorf("virtual mouse: %w", err)
	}
	return &Input{layoutName: layout, layout: mk(), kbd: kbd, mouse: mouse, KeyDelay: 2 * time.Millisecond}, nil
}

// LayoutNames lists the supported layouts.
func LayoutNames() []string {
	names := make([]string, 0, len(Layouts))
	for n := range Layouts {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Strokes converts text to key strokes for a layout; it fails on the first character that the
// layout cannot type (exported for tests).
func Strokes(layoutName, text string) ([]stroke, error) {
	mk, ok := Layouts[layoutName]
	if !ok {
		return nil, fmt.Errorf("unsupported keyboard layout %q", layoutName)
	}
	l := mk()
	out := make([]stroke, 0, len(text))
	for _, r := range text {
		s, ok := l[r]
		if !ok {
			return nil, &UnsupportedCharError{Char: r, Layout: layoutName}
		}
		out = append(out, s)
	}
	return out, nil
}

// TypeText types text into the focused app. The whole text is checked first: if a character is
// not available in the layout nothing is typed.
func (in *Input) TypeText(text string) error {
	in.mu.Lock()
	defer in.mu.Unlock()
	strokes, err := Strokes(in.layoutName, text)
	if err != nil {
		return err
	}
	for _, s := range strokes {
		var mods []int
		if s.shift {
			mods = append(mods, keyLeftShift)
		}
		if s.altgr {
			mods = append(mods, keyRightAlt)
		}
		if err := in.tap(s.code, mods); err != nil {
			return err
		}
	}
	return nil
}

// PressKey presses a named special key ("enter", "backspace", "up", ...) with optional modifiers
// ("ctrl", "shift", "alt", "meta").
func (in *Input) PressKey(name string, modifiers []string) error {
	code, ok := namedKeys[strings.ToLower(name)]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownKey, name)
	}
	var mods []int
	for _, m := range modifiers {
		mc, ok := modifierKeys[strings.ToLower(m)]
		if !ok {
			return fmt.Errorf("%w: modifier %q", ErrUnknownKey, m)
		}
		mods = append(mods, mc)
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.tap(code, mods)
}

// tap presses the modifiers, presses and releases the key, then releases the modifiers.
func (in *Input) tap(code int, mods []int) error {
	steps := make([][2]int32, 0, 2*len(mods)+2)
	for _, m := range mods {
		steps = append(steps, [2]int32{int32(m), 1})
	}
	steps = append(steps, [2]int32{int32(code), 1}, [2]int32{int32(code), 0})
	for i := len(mods) - 1; i >= 0; i-- {
		steps = append(steps, [2]int32{int32(mods[i]), 0})
	}
	for _, st := range steps {
		if err := in.kbd.Emit(evdev.EvKey, uint16(st[0]), st[1]); err != nil {
			return err
		}
		if err := in.kbd.Sync(); err != nil {
			return err
		}
	}
	if in.KeyDelay > 0 {
		time.Sleep(in.KeyDelay)
	}
	return nil
}

// Mouse buttons and axes.
const (
	btnLeft   = 0x110
	btnRight  = 0x111
	btnMiddle = 0x112
	relX      = 0x00
	relY      = 0x01
	relWheel  = 0x08
)

var mouseButtons = map[string]int{"left": btnLeft, "right": btnRight, "middle": btnMiddle}

// MovePointer moves the mouse pointer by dx, dy and scrolls by wheel notches.
func (in *Input) MovePointer(dx, dy, wheel int) error {
	in.mu.Lock()
	defer in.mu.Unlock()
	for _, ax := range [][2]int{{relX, dx}, {relY, dy}, {relWheel, wheel}} {
		if ax[1] != 0 {
			if err := in.mouse.Emit(evdev.EvRel, uint16(ax[0]), int32(ax[1])); err != nil {
				return err
			}
		}
	}
	return in.mouse.Sync()
}

// Click clicks a mouse button ("left", "right", "middle").
func (in *Input) Click(button string) error {
	code, ok := mouseButtons[strings.ToLower(button)]
	if !ok {
		return fmt.Errorf("%w: button %q", ErrUnknownKey, button)
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.mouse.Press(code)
}

// Layout returns the current keyboard layout name.
func (in *Input) Layout() string {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.layoutName
}

// SetLayout changes the keyboard layout (it must match the session's layout).
func (in *Input) SetLayout(name string) error {
	mk, ok := Layouts[name]
	if !ok {
		return fmt.Errorf("unsupported keyboard layout %q", name)
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	in.layoutName, in.layout = name, mk()
	return nil
}

// Close removes the virtual devices.
func (in *Input) Close() {
	in.kbd.Close()
	in.mouse.Close()
}
