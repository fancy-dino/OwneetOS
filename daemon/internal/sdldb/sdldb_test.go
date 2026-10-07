// SPDX-License-Identifier: GPL-3.0-or-later

package sdldb

import (
	"strings"
	"testing"

	"github.com/fancy-dino/OwneetOS/daemon/internal/evdev"
)

const sample = `# comment
030000005e0400008e02000000000000,Xbox 360 Controller,a:b0,guide:b8,platform:Windows,
030000005e0400008e02000014010000,Xbox 360 Controller,a:b0,b:b1,guide:b8,platform:Linux,
03000000adde0000efbe000000000000,Test Pad,a:b0,guide:b10,platform:Linux,
`

func TestGUID(t *testing.T) {
	got := GUID(evdev.ID{Bustype: 3, Vendor: 0x045e, Product: 0x028e, Version: 0x0114})
	if got != "030000005e0400008e02000014010000" {
		t.Fatalf("GUID %s", got)
	}
}

func TestParseKeepsLinuxAndFallsBackToVersionZero(t *testing.T) {
	db, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if db.Len() != 2 {
		t.Fatalf("%d Linux mappings", db.Len())
	}
	m, ok := db.Mapping(evdev.ID{Bustype: 3, Vendor: 0xdead, Product: 0xbeef, Version: 0x0110})
	if !ok || m["guide"] != "b10" || m["name"] != "Test Pad" {
		t.Fatalf("mapping %v %v", m, ok)
	}
}

func TestButtonCode(t *testing.T) {
	// SDL order: codes >= BTN_JOYSTICK first, then lower codes.
	keys := []int{evdev.BtnMisc, evdev.BtnSouth, evdev.BtnSouth + 1, evdev.BtnTriggerHappy}
	cases := map[string]int{"b0": evdev.BtnSouth, "b2": evdev.BtnTriggerHappy, "b3": evdev.BtnMisc}
	for ref, want := range cases {
		if got, ok := ButtonCode(keys, ref); !ok || got != want {
			t.Errorf("%s -> %#x, want %#x", ref, got, want)
		}
	}
	if _, ok := ButtonCode(keys, "b4"); ok {
		t.Error("out of range accepted")
	}
	if _, ok := ButtonCode(keys, "h0.1"); ok {
		t.Error("hat accepted as button")
	}
}
