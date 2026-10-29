// SPDX-License-Identifier: GPL-3.0-or-later

package clock

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

type fakeBus struct {
	props   map[string]any
	calls   []string
	failNTP bool
	onCall  func(method string, args []any)
}

func (b *fakeBus) Property(name string) (dbus.Variant, error) {
	v, ok := b.props[name]
	if !ok {
		return dbus.Variant{}, errors.New("no such property")
	}
	return dbus.MakeVariant(v), nil
}

func (b *fakeBus) Call(method string, args ...any) error {
	b.calls = append(b.calls, method)
	if b.onCall != nil {
		b.onCall(method, args)
	}
	if method == "SetTimezone" {
		b.props["Timezone"] = args[0]
	}
	if method == "SetNTP" && b.failNTP {
		return errors.New("NTP not supported")
	}
	return nil
}

func newManager(bus *fakeBus) *Manager {
	return &Manager{Bus: bus, Broker: events.NewBroker(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestStatus(t *testing.T) {
	m := newManager(&fakeBus{props: map[string]any{"Timezone": "Europe/Rome", "NTPSynchronized": true}})
	if st := m.Status(); st.Zone != "Europe/Rome" || !st.Synchronized {
		t.Errorf("Status() = %+v", st)
	}
	// nothing chosen yet, or timedated not answering: UTC
	m = newManager(&fakeBus{props: map[string]any{"Timezone": ""}})
	if st := m.Status(); st.Zone != "UTC" || st.Synchronized {
		t.Errorf("Status() without a zone = %+v", st)
	}
}

func TestSetZone(t *testing.T) {
	bus := &fakeBus{props: map[string]any{}}
	m := newManager(bus)
	ch, cancel := m.Broker.Subscribe(4)
	defer cancel()
	if err := m.SetZone("Europe/Berlin"); err != nil {
		t.Fatal(err)
	}
	if bus.props["Timezone"] != "Europe/Berlin" || len(bus.calls) != 2 || bus.calls[1] != "SetNTP" {
		t.Errorf("calls %v, zone %v", bus.calls, bus.props["Timezone"])
	}
	if ev := <-ch; ev.Type != "time.changed" {
		t.Errorf("event %q", ev.Type)
	}
	// without time synchronisation the zone is still set
	bus = &fakeBus{props: map[string]any{}, failNTP: true}
	if err := newManager(bus).SetZone("America/Bogota"); err != nil || bus.props["Timezone"] != "America/Bogota" {
		t.Errorf("SetZone without NTP: %v, %v", err, bus.props["Timezone"])
	}
}

func TestOffsets(t *testing.T) {
	summer := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	got := Offsets([]string{"Europe/Rome", "Mars/Olympus", "America/Bogota", "Asia/Kolkata"}, summer)
	want := []Offset{{"Europe/Rome", 7200}, {"America/Bogota", -18000}, {"Asia/Kolkata", 19800}}
	if len(got) != len(want) {
		t.Fatalf("Offsets() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Offsets()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	winter := Offsets([]string{"Europe/Rome"}, time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	if len(winter) != 1 || winter[0].Offset != 3600 {
		t.Errorf("winter offset of Rome = %v", winter)
	}
}

func TestSetZoneRefusesUnknown(t *testing.T) {
	for _, zone := range []string{"", "Local", "Mars/Olympus", "../../etc/passwd"} {
		bus := &fakeBus{props: map[string]any{}}
		if err := newManager(bus).SetZone(zone); !errors.Is(err, ErrUnknownZone) || len(bus.calls) != 0 {
			t.Errorf("SetZone(%q) = %v, calls %v", zone, err, bus.calls)
		}
	}
}

func TestSetTime(t *testing.T) {
	bus := &fakeBus{props: map[string]any{"Timezone": "Europe/Rome"}}
	m := newManager(bus)
	var set int64
	bus.onCall = func(method string, args []any) {
		if method == "SetTime" {
			set = args[0].(int64)
		}
	}
	if err := m.SetTime("2026-10-29", "15:30", ""); err != nil {
		t.Fatal(err)
	}
	// 15:30 in Rome on 29 October 2026 (CET) is 14:30 UTC; the internet's time off, then on again
	if want := time.Date(2026, 10, 29, 14, 30, 0, 0, time.UTC).UnixMicro(); set != want {
		t.Errorf("SetTime(%d), want %d", set, want)
	}
	if got := bus.calls; len(got) != 3 || got[0] != "SetNTP" || got[1] != "SetTime" || got[2] != "SetNTP" {
		t.Errorf("calls %v", got)
	}
	for _, bad := range [][2]string{{"2026-13-01", "10:00"}, {"2026-10-29", "25:00"}, {"1999-01-01", "10:00"}, {"", ""}} {
		if err := m.SetTime(bad[0], bad[1], ""); !errors.Is(err, ErrBadTime) {
			t.Errorf("SetTime(%q, %q) = %v, want ErrBadTime", bad[0], bad[1], err)
		}
	}
}

func TestSetTimeInZone(t *testing.T) {
	// the first start: the console is still on UTC, the time is typed for the city shown
	bus := &fakeBus{props: map[string]any{"Timezone": "UTC"}}
	m := newManager(bus)
	var set int64
	bus.onCall = func(method string, args []any) {
		if method == "SetTime" {
			set = args[0].(int64)
		}
	}
	if err := m.SetTime("2026-10-29", "15:30", "Europe/Rome"); err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 29, 14, 30, 0, 0, time.UTC).UnixMicro(); set != want {
		t.Errorf("SetTime(%d), want %d", set, want)
	}
	if err := m.SetTime("2026-10-29", "15:30", "Mars/Olympus"); !errors.Is(err, ErrUnknownZone) {
		t.Errorf("SetTime in an unknown zone = %v, want ErrUnknownZone", err)
	}
}

func TestStatusDateAndTime(t *testing.T) {
	m := newManager(&fakeBus{props: map[string]any{"Timezone": "America/Bogota"}})
	m.Now = func() time.Time { return time.Date(2026, 10, 29, 3, 5, 0, 0, time.UTC) }
	if st := m.Status(); st.Date != "2026-10-28" || st.Time != "22:05" {
		t.Errorf("Status() = %+v, want 2026-10-28 22:05", st)
	}
}
