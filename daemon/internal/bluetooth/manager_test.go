// SPDX-License-Identifier: GPL-3.0-or-later

package bluetooth

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// fakeBlueZ simulates an adapter: pairing a device marks it paired; connecting, connected.
type fakeBlueZ struct {
	mu      sync.Mutex
	st      State
	calls   []string
	changes chan struct{}
}

func newFake(devs ...Device) *fakeBlueZ {
	return &fakeBlueZ{st: State{Adapter: true, Devices: devs}, changes: make(chan struct{}, 8)}
}

func (f *fakeBlueZ) Snapshot() (State, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.st
	st.Devices = append([]Device(nil), f.st.Devices...)
	return st, nil
}
func (f *fakeBlueZ) record(c string) { f.calls = append(f.calls, c) }
func (f *fakeBlueZ) SetPowered(on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.st.Powered = on
	f.record("power")
	return nil
}
func (f *fakeBlueZ) SetDiscovery(on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.st.Discovering = on
	if on {
		f.record("scan-on")
	} else {
		f.record("scan-off")
	}
	return nil
}
func (f *fakeBlueZ) update(addr string, fn func(*Device)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.st.Devices {
		if f.st.Devices[i].Address == addr {
			fn(&f.st.Devices[i])
		}
	}
}
func (f *fakeBlueZ) Pair(a string) error {
	f.update(a, func(d *Device) { d.Paired = true })
	f.mu.Lock()
	f.record("pair " + a)
	f.mu.Unlock()
	return nil
}
func (f *fakeBlueZ) SetTrusted(a string, on bool) error {
	f.update(a, func(d *Device) { d.Trusted = on })
	return nil
}
func (f *fakeBlueZ) Connect(a string) error {
	f.update(a, func(d *Device) { d.Connected = true })
	return nil
}
func (f *fakeBlueZ) Disconnect(a string) error { return nil }
func (f *fakeBlueZ) Remove(a string) error     { return nil }
func (f *fakeBlueZ) Changes() <-chan struct{}  { return f.changes }
func (f *fakeBlueZ) addDevice(d Device) {
	f.mu.Lock()
	f.st.Devices = append(f.st.Devices, d)
	f.mu.Unlock()
	f.changes <- struct{}{}
}
func (f *fakeBlueZ) callList() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, ",")
}

const (
	classGamepad  = 0x002508 // peripheral, gamepad
	classKeyboard = 0x002540 // peripheral, keyboard
)

func TestIsGamepad(t *testing.T) {
	cases := []struct {
		d    Device
		want bool
	}{
		{Device{Class: classGamepad}, true},
		{Device{Class: 0x002504}, true}, // joystick
		{Device{Class: classKeyboard}, false},
		{Device{Appearance: 0x03c4}, true}, // BLE gamepad (e.g. Xbox Series controllers)
		{Device{Icon: "input-gaming"}, true},
		{Device{Icon: "audio-headset"}, false},
	}
	for i, c := range cases {
		if got := IsGamepad(c.d); got != c.want {
			t.Errorf("case %d: got %v", i, got)
		}
	}
}

func run(t *testing.T, m *Manager) (<-chan events.Event, func()) {
	t.Helper()
	b := events.NewBroker()
	ch, unsub := b.Subscribe(64)
	m.Broker = b
	m.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	m.Interval = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	return ch, func() { cancel(); <-done; unsub() }
}

func waitEvent(t *testing.T, ch <-chan events.Event, typ string) events.Event {
	t.Helper()
	timeout := time.After(3 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Type == typ {
				return ev
			}
		case <-timeout:
			t.Fatalf("no %s event", typ)
		}
	}
}

func TestNoControllerTurnsAutoPairOnAndPairsOnlyGamepads(t *testing.T) {
	fake := newFake()
	var controllers int
	var cmu sync.Mutex
	m := &Manager{Backend: fake, AutoPairWithoutController: true,
		Controllers: func() int { cmu.Lock(); defer cmu.Unlock(); return controllers }}
	ch, stop := run(t, m)
	defer stop()

	ev := waitEvent(t, ch, "bluetooth.auto_pair")
	if !ev.Data.(map[string]bool)["enabled"] {
		t.Fatal("auto-pair not enabled without controllers")
	}
	fake.addDevice(Device{Address: "AA:00:00:00:00:01", Name: "Keyboard", Class: classKeyboard})
	fake.addDevice(Device{Address: "AA:00:00:00:00:02", Name: "Xbox Wireless Controller", Class: classGamepad})

	paired := waitEvent(t, ch, "bluetooth.paired")
	if paired.Data.(map[string]string)["address"] != "AA:00:00:00:00:02" {
		t.Fatalf("paired %v", paired.Data)
	}
	st := m.Status()
	for _, d := range st.Devices {
		if d.Address == "AA:00:00:00:00:02" && !(d.Paired && d.Trusted && d.Connected) {
			t.Fatalf("gamepad not paired+trusted+connected: %+v", d)
		}
		if d.Address == "AA:00:00:00:00:01" && d.Paired {
			t.Fatal("keyboard was paired")
		}
	}
	calls := fake.callList()
	if !strings.Contains(calls, "power") || !strings.Contains(calls, "scan-on") || strings.Contains(calls, "pair AA:00:00:00:00:01") {
		t.Fatalf("unexpected BlueZ calls: %s", calls)
	}

	// A controller is now connected: auto-pair stops and so does scanning.
	cmu.Lock()
	controllers = 1
	cmu.Unlock()
	ev = waitEvent(t, ch, "bluetooth.auto_pair")
	if ev.Data.(map[string]bool)["enabled"] {
		t.Fatal("auto-pair still on with a controller connected")
	}
	time.Sleep(100 * time.Millisecond)
	if !strings.HasSuffix(fake.callList(), "scan-off") {
		t.Fatalf("scan not stopped: %s", fake.callList())
	}
}

func TestAgentAcceptsOnlyGamepadsWhileAutoPairIsOn(t *testing.T) {
	fake := newFake(Device{Address: "AA:00:00:00:00:02", Class: classGamepad},
		Device{Address: "AA:00:00:00:00:03", Class: classKeyboard})
	m := &Manager{Backend: fake, Controllers: func() int { return 1 }}
	_, stop := run(t, m)
	defer stop()
	time.Sleep(60 * time.Millisecond)

	if m.AllowPairing("aa:00:00:00:00:02") {
		t.Fatal("pairing accepted while auto-pair is off")
	}
	if err := m.SetAutoPair(true, time.Minute); err != nil {
		t.Fatal(err)
	}
	if !m.AllowPairing("aa:00:00:00:00:02") || m.AllowPairing("AA:00:00:00:00:03") {
		t.Fatal("agent decisions wrong while auto-pair is on")
	}
	m.SetAutoPair(false, 0)
	if m.AllowPairing("AA:00:00:00:00:02") {
		t.Fatal("pairing accepted after auto-pair was turned off")
	}
}

func TestNoAdapter(t *testing.T) {
	fake := newFake()
	fake.st.Adapter = false
	m := &Manager{Backend: fake}
	_, stop := run(t, m)
	defer stop()
	time.Sleep(60 * time.Millisecond)
	if err := m.SetAutoPair(true, time.Minute); err != ErrNoAdapter {
		t.Fatalf("got %v", err)
	}
	if err := m.Connect("AA:00:00:00:00:02"); err != ErrNoAdapter {
		t.Fatalf("got %v", err)
	}
}

func TestNameIgnoresAddressAliases(t *testing.T) {
	fake := newFake(
		Device{Address: "AA:00:00:00:00:01", Name: "Xbox Wireless Controller", Paired: true},
		Device{Address: "AA:00:00:00:00:02", Name: "AA-00-00-00-00-02"}, // name not resolved yet
	)
	m := &Manager{Backend: fake}
	_, stop := run(t, m)
	defer stop()
	time.Sleep(60 * time.Millisecond)
	if got := m.Name("aa:00:00:00:00:01"); got != "Xbox Wireless Controller" {
		t.Errorf("Name(known) = %q", got)
	}
	if got := m.Name("AA:00:00:00:00:02"); got != "" {
		t.Errorf("Name(unresolved) = %q, want empty", got)
	}
	if got := m.Name("AA:00:00:00:00:03"); got != "" {
		t.Errorf("Name(unknown) = %q, want empty", got)
	}
}
