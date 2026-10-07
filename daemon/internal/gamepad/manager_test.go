// SPDX-License-Identifier: GPL-3.0-or-later

package gamepad

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fancy-dino/OwneetOS/daemon/internal/evdev"
)

func TestIsGamepad(t *testing.T) {
	pad, kbd, sensors := &evdev.Device{}, &evdev.Device{}, &evdev.Device{}
	pad.SetCapabilities([]int{evdev.BtnSouth, evdev.BtnMode}, []int{evdev.AbsX, evdev.AbsY})
	kbd.SetCapabilities([]int{30, 31, 32}, nil)                 // KEY_A, KEY_S, KEY_D
	sensors.SetCapabilities(nil, []int{evdev.AbsX, evdev.AbsY}) // DualSense motion sensors
	if !IsGamepad(pad) || IsGamepad(kbd) || IsGamepad(sensors) {
		t.Fatal("classification wrong")
	}
}

func TestReadBattery(t *testing.T) {
	// /sys/devices/.../0005:045E:0B13.0001/input/input7 with power_supply in the HID device
	root := t.TempDir()
	hid := filepath.Join(root, "0005:045E:0B13.0001")
	input := filepath.Join(hid, "input", "input7")
	supply := filepath.Join(hid, "power_supply", "hid-aa:bb-battery")
	for _, d := range []string{input, supply} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(supply, "capacity"), []byte("73\n"), 0o644)
	os.WriteFile(filepath.Join(supply, "capacity_level"), []byte("Normal\n"), 0o644)

	pct, level := ReadBattery(input)
	if pct == nil || *pct != 73 || level != "normal" {
		t.Fatalf("got %v %q", pct, level)
	}
	if pct, level := ReadBattery(filepath.Join(root, "elsewhere")); pct != nil || level != "" {
		t.Fatal("found a battery where there is none")
	}
}

func TestHex4AndBrand(t *testing.T) {
	if hex4(0x45e) != "045e" || brand(0x054c) != "playstation" || brand(0x1234) != "other" {
		t.Fatal("helpers wrong")
	}
}
