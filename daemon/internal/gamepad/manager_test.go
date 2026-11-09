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

// On a keyboard, Super is Guide ([[controller-input-map]]); other devices with keys are not
// keyboards.
func TestKeyboardSuperIsGuide(t *testing.T) {
	kbd, power, pad := &evdev.Device{}, &evdev.Device{}, &evdev.Device{}
	kbd.SetCapabilities([]int{evdev.KeyA, evdev.KeyZ, evdev.KeyLeftMeta, evdev.KeyRightMeta}, nil)
	power.SetCapabilities([]int{116}, nil) // KEY_POWER only
	pad.SetCapabilities([]int{evdev.BtnSouth, evdev.BtnMode}, []int{evdev.AbsX, evdev.AbsY})
	if !IsKeyboard(kbd) || IsKeyboard(power) || IsKeyboard(pad) {
		t.Fatal("keyboard classification wrong")
	}
	k := &tracked{keyboard: true, guideCode: -1}
	if !k.isGuide(evdev.KeyLeftMeta) || !k.isGuide(evdev.KeyRightMeta) || k.isGuide(evdev.KeyA) {
		t.Error("on a keyboard only Super is Guide")
	}
	if k.pad() {
		t.Error("a keyboard is not listed as a controller")
	}
	p := &tracked{guideCode: evdev.BtnMode}
	if !p.isGuide(evdev.BtnMode) || p.isGuide(evdev.KeyLeftMeta) || !p.pad() {
		t.Error("a controller's Guide is its own button only")
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

// Devices made by programs through uinput are left out; Bluetooth (uhid) and USB controllers and
// the project's test devices stay
func TestFromProgram(t *testing.T) {
	root := t.TempDir()
	m := &Manager{SysInputDir: filepath.Join(root, "class", "input")}
	link := func(event, target string) *evdev.Device {
		dir := filepath.Join(root, target)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(m.SysInputDir, event), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(dir, filepath.Join(m.SysInputDir, event, "device")); err != nil {
			t.Fatal(err)
		}
		return &evdev.Device{Path: "/dev/input/" + event}
	}
	steam := link("event16", "devices/virtual/input/input20")
	steam.Name = "Microsoft X-Box 360 pad 0"
	bt := link("event17", "devices/virtual/misc/uhid/0005:045E:0B13.0001/input/input17")
	bt.Name = "Xbox Wireless Controller"
	usb := link("event18", "devices/pci0000:00/0000:00:14.0/usb1/1-4/1-4:1.0/input/input18")
	usb.Name = "Microsoft X-Box 360 pad"
	test := link("event19", "devices/virtual/input/input21")
	test.Name = "OwneetOS Test Pad"
	own := link("event20", "devices/virtual/input/input22")
	own.Name = "OwneetOS Virtual Keyboard"
	if !m.fromProgram(steam) || !m.fromProgram(own) {
		t.Fatal("a device made by a program kept")
	}
	if m.fromProgram(bt) || m.fromProgram(usb) || m.fromProgram(test) {
		t.Fatal("a real or test controller left out")
	}
}

// WhenReleased waits until no button or key is held on any device.
func TestWhenReleased(t *testing.T) {
	m := &Manager{devs: map[string]*tracked{}}
	pad, kbd := &tracked{}, &tracked{keyboard: true}
	m.devs["/dev/input/event1"], m.devs["/dev/input/event2"] = pad, kbd
	closed := func(ch <-chan struct{}) bool {
		select {
		case <-ch:
			return true
		default:
			return false
		}
	}
	if !closed(m.WhenReleased()) {
		t.Fatal("nothing held, yet waiting")
	}
	m.hold(pad, evdev.BtnSouth, true) // A down
	m.hold(kbd, evdev.KeyA, true)
	ch := m.WhenReleased()
	m.hold(pad, evdev.BtnSouth, false)
	if closed(ch) {
		t.Fatal("released while a key is still held")
	}
	m.hold(kbd, evdev.KeyA, false)
	if !closed(ch) {
		t.Fatal("not released when nothing is held")
	}
}
