// SPDX-License-Identifier: GPL-3.0-or-later

//go:build uinput

// Integration test: types through the real virtual keyboard and reads the key events back from
// its /dev/input node. Needs root (or access to /dev/uinput): go test -tags uinput ./internal/vinput/
package vinput

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/evdev"
)

func findDevice(t *testing.T, name string) *evdev.Device {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		paths, _ := filepath.Glob("/dev/input/event*")
		for _, p := range paths {
			d, err := evdev.Open(p)
			if err != nil {
				continue
			}
			if d.Name == name {
				return d
			}
			d.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("virtual device %q not found", name)
	return nil
}

func TestTypingProducesKeyEvents(t *testing.T) {
	if _, err := os.Stat("/dev/uinput"); err != nil {
		t.Skip("no /dev/uinput")
	}
	in, err := New("it")
	if err != nil {
		t.Skipf("cannot create virtual devices: %v", err)
	}
	defer in.Close()
	dev := findDevice(t, KeyboardName)
	defer dev.Close()
	time.Sleep(100 * time.Millisecond)

	if err := in.TypeText("a@"); err != nil {
		t.Fatal(err)
	}
	if err := in.PressKey("enter", nil); err != nil {
		t.Fatal(err)
	}

	// Expected key transitions: a, AltGr+ò (=@ on the Italian layout), Enter.
	want := []string{"30:1", "30:0", "100:1", "39:1", "39:0", "100:0", "28:1", "28:0"}
	var got []string
	deadline := time.Now().Add(3 * time.Second)
	for len(got) < len(want) && time.Now().Before(deadline) {
		evs, err := dev.Read()
		if err != nil {
			t.Fatal(err)
		}
		for _, ev := range evs {
			if ev.Type == evdev.EvKey {
				got = append(got, strconv.Itoa(int(ev.Code))+":"+strconv.Itoa(int(ev.Value)))
			}
		}
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("key events\n got  %v\n want %v", got, want)
	}
}
