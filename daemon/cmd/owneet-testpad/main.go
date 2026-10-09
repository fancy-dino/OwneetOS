// SPDX-License-Identifier: GPL-3.0-or-later

// owneet-testpad is a test tool, not shipped in the image: a virtual Xbox-style controller that
// presses buttons given on the command line, for checking the console in the test VM.
//
//	owneet-testpad [-hold DURATION] [-wait DURATION] BUTTON...
//
// Buttons: a b x y lb rb view menu guide up down left right; "pause" waits one second. The
// controller appears, waits (-wait, default 2 s, so that owneetd and the interface notice it),
// presses the buttons one after another and disappears. Needs write access to /dev/uinput.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/evdev"
	"github.com/fancy-dino/OwneetOS/daemon/internal/uinput"
)

var buttons = map[string]int{
	"a": evdev.BtnSouth, "b": evdev.BtnSouth + 1, "x": evdev.BtnSouth + 3, "y": evdev.BtnSouth + 4,
	"lb": evdev.BtnSouth + 6, "rb": evdev.BtnSouth + 7, "view": evdev.BtnSouth + 10, "menu": evdev.BtnSouth + 11,
	"guide": evdev.BtnMode,
}

// D-pad on the hat axes
var hat = map[string][2]int32{"up": {evdev.AbsHat0X + 1, -1}, "down": {evdev.AbsHat0X + 1, 1}, "left": {evdev.AbsHat0X, -1}, "right": {evdev.AbsHat0X, 1}}

func main() {
	hold := flag.Duration("hold", 100*time.Millisecond, "how long each button is held")
	wait := flag.Duration("wait", 2*time.Second, "wait after the controller appears")
	flag.Parse()

	keys := []int{}
	for _, code := range buttons {
		keys = append(keys, code)
	}
	pad, err := uinput.Create(uinput.Spec{
		Name: "OwneetOS Test Pad",
		ID:   evdev.ID{Bustype: evdev.BusUSB, Vendor: 0x045e, Product: 0x028e, Version: 0x110},
		Keys: keys,
		Axes: []uinput.Axis{
			{Code: evdev.AbsX, Min: -32768, Max: 32767}, {Code: evdev.AbsY, Min: -32768, Max: 32767},
			{Code: evdev.AbsHat0X, Min: -1, Max: 1}, {Code: evdev.AbsHat0X + 1, Min: -1, Max: 1},
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "owneet-testpad:", err)
		os.Exit(1)
	}
	defer pad.Close()
	time.Sleep(*wait)

	for _, name := range flag.Args() {
		if name == "pause" {
			time.Sleep(time.Second)
			continue
		}
		if code, ok := buttons[name]; ok {
			must(pad.Emit(evdev.EvKey, uint16(code), 1), pad.Sync())
			time.Sleep(*hold)
			must(pad.Emit(evdev.EvKey, uint16(code), 0), pad.Sync())
		} else if h, ok := hat[name]; ok {
			must(pad.Emit(evdev.EvAbs, uint16(h[0]), h[1]), pad.Sync())
			time.Sleep(*hold)
			must(pad.Emit(evdev.EvAbs, uint16(h[0]), 0), pad.Sync())
		} else {
			fmt.Fprintln(os.Stderr, "owneet-testpad: unknown button", name)
			os.Exit(2)
		}
		time.Sleep(300 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
}

func must(errs ...error) {
	for _, err := range errs {
		if err != nil {
			fmt.Fprintln(os.Stderr, "owneet-testpad:", err)
			os.Exit(1)
		}
	}
}
