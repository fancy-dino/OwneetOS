// SPDX-License-Identifier: GPL-3.0-or-later

//go:build uinput

// Integration tests with virtual controllers created through /dev/uinput. They need root (or
// write access to /dev/uinput) and run with: go test -tags uinput ./internal/gamepad/
package gamepad

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/evdev"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
	"github.com/fancy-dino/OwneetOS/daemon/internal/sdldb"
	"github.com/fancy-dino/OwneetOS/daemon/internal/uinput"
)

func startManager(t *testing.T, db *sdldb.DB) (*Manager, <-chan events.Event) {
	t.Helper()
	b := events.NewBroker()
	ch, unsub := b.Subscribe(64)
	m := &Manager{Broker: b, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: db}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done; unsub() })
	<-m.Ready()
	return m, ch
}

func waitFor(t *testing.T, ch <-chan events.Event, typ string, match func(events.Event) bool) events.Event {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Type == typ && (match == nil || match(ev)) {
				return ev
			}
		case <-deadline:
			t.Fatalf("no %s event", typ)
		}
	}
}

func padAxes() []uinput.Axis {
	return []uinput.Axis{{Code: evdev.AbsX, Min: -32768, Max: 32767}, {Code: evdev.AbsY, Min: -32768, Max: 32767}}
}

func TestXboxLikePadGuideAndHotplug(t *testing.T) {
	m, ch := startManager(t, nil)
	pad, err := uinput.Create(uinput.Spec{
		Name: "OwneetOS Test Xbox Pad",
		ID:   evdev.ID{Bustype: evdev.BusUSB, Vendor: 0x045e, Product: 0x028e, Version: 1},
		Keys: []int{evdev.BtnSouth, evdev.BtnSouth + 1, evdev.BtnMode},
		Axes: padAxes(),
	})
	if err != nil {
		t.Skipf("cannot create a virtual device (root and /dev/uinput needed): %v", err)
	}
	isOurs := func(ev events.Event) bool {
		c, ok := ev.Data.(Controller)
		return ok && c.Name == "OwneetOS Test Xbox Pad"
	}
	added := waitFor(t, ch, "controller.added", isOurs).Data.(Controller)
	if added.Brand != "xbox" || added.Connection != "usb" || !added.HasGuide {
		t.Fatalf("unexpected controller %+v", added)
	}
	found := false
	for _, c := range m.List() {
		found = found || c.ID == added.ID
	}
	if !found {
		t.Fatal("controller missing from List")
	}

	pad.Press(evdev.BtnSouth) // an ordinary button: no Guide event
	pad.Press(evdev.BtnMode)
	ev := waitFor(t, ch, "guide.pressed", nil)
	if ev.Data.(map[string]string)["controller"] != added.ID {
		t.Fatalf("guide from %v", ev.Data)
	}

	pad.Close()
	waitFor(t, ch, "controller.removed", func(ev events.Event) bool {
		return ev.Data.(map[string]string)["id"] == added.ID
	})
}

func TestGenericPadGuideFromSDLDatabase(t *testing.T) {
	// A pad whose driver does not report BTN_MODE; the database says Guide is button 2.
	id := evdev.ID{Bustype: evdev.BusUSB, Vendor: 0xdead, Product: 0xbeef, Version: 1}
	db, err := sdldb.Parse(strings.NewReader(sdldb.GUID(id) + ",Test Pad,a:b0,b:b1,guide:b2,platform:Linux,\n"))
	if err != nil {
		t.Fatal(err)
	}
	_, ch := startManager(t, db)
	pad, err := uinput.Create(uinput.Spec{
		Name: "OwneetOS Test Generic Pad",
		ID:   id,
		Keys: []int{evdev.BtnJoystick, evdev.BtnJoystick + 1, evdev.BtnJoystick + 2},
		Axes: padAxes(),
	})
	if err != nil {
		t.Skipf("cannot create a virtual device: %v", err)
	}
	defer pad.Close()
	added := waitFor(t, ch, "controller.added", func(ev events.Event) bool {
		c, ok := ev.Data.(Controller)
		return ok && c.Name == "OwneetOS Test Generic Pad"
	}).Data.(Controller)
	if !added.HasGuide || added.Brand != "other" {
		t.Fatalf("unexpected controller %+v", added)
	}
	pad.Press(evdev.BtnJoystick + 1) // button 1: not the Guide
	pad.Press(evdev.BtnJoystick + 2) // button 2: Guide per the database
	ev := waitFor(t, ch, "guide.pressed", nil)
	if ev.Data.(map[string]string)["controller"] != added.ID {
		t.Fatalf("guide from %v", ev.Data)
	}
}
