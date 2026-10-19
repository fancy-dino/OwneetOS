// SPDX-License-Identifier: GPL-3.0-or-later

package vinput

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ErrUnavailable: the virtual keyboard and mouse do not exist yet (no access to /dev/uinput).
var ErrUnavailable = errors.New("virtual input is not available yet (no access to /dev/uinput)")

// Retrying is the virtual keyboard and mouse, made as soon as /dev/uinput can be opened. logind
// gives /dev/uinput to the console user only once their session is active (the uaccess tag), and
// owneetd may start before that: it tries again until it works (the FrankenPC logged "permission
// denied" at every start, 2026-10-19, [[step-4.7-controller-pointer]]).
type Retrying struct {
	mu     sync.Mutex
	in     *Input
	layout string
	ready  chan struct{}
}

// NewRetrying tries to make the devices now, then every `every` until it works or ctx ends.
func NewRetrying(ctx context.Context, layout string, every time.Duration, log *slog.Logger) (*Retrying, error) {
	if _, ok := Layouts[layout]; !ok {
		return nil, fmt.Errorf("unsupported keyboard layout %q", layout)
	}
	r := &Retrying{layout: layout, ready: make(chan struct{})}
	if r.try() {
		return r, nil
	}
	go func() {
		tick := time.NewTicker(every)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if r.try() {
					log.Info("virtual keyboard and mouse ready")
					return
				}
			}
		}
	}()
	return r, nil
}

func (r *Retrying) try() bool {
	r.mu.Lock()
	layout := r.layout
	r.mu.Unlock()
	in, err := New(layout)
	if err != nil {
		return false
	}
	r.mu.Lock()
	r.in = in
	r.mu.Unlock()
	close(r.ready)
	return true
}

// Ready is closed once the devices exist.
func (r *Retrying) Ready() <-chan struct{} { return r.ready }

func (r *Retrying) get() (*Input, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.in == nil {
		return nil, ErrUnavailable
	}
	return r.in, nil
}

func (r *Retrying) TypeText(text string) error {
	in, err := r.get()
	if err != nil {
		return err
	}
	return in.TypeText(text)
}

func (r *Retrying) PressKey(name string, modifiers []string) error {
	in, err := r.get()
	if err != nil {
		return err
	}
	return in.PressKey(name, modifiers)
}

func (r *Retrying) MovePointer(dx, dy, wheel int) error {
	in, err := r.get()
	if err != nil {
		return err
	}
	return in.MovePointer(dx, dy, wheel)
}

func (r *Retrying) Click(button string) error {
	in, err := r.get()
	if err != nil {
		return err
	}
	return in.Click(button)
}

// Layout is the keyboard layout, also before the devices exist.
func (r *Retrying) Layout() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.layout
}

// SetLayout changes the layout now or for the devices to come.
func (r *Retrying) SetLayout(name string) error {
	if _, ok := Layouts[name]; !ok {
		return fmt.Errorf("unsupported keyboard layout %q", name)
	}
	r.mu.Lock()
	r.layout = name
	in := r.in
	r.mu.Unlock()
	if in != nil {
		return in.SetLayout(name)
	}
	return nil
}

// Close removes the devices, if they exist.
func (r *Retrying) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.in != nil {
		r.in.Close()
		r.in = nil
	}
}
