// SPDX-License-Identifier: GPL-3.0-or-later

// Package live tells the live system (started from the USB stick) from the installed one
// ([[step-6.2-live-vs-installed-mode]]: owneet-mode writes /run/owneet/mode early at every start),
// and whether the live system has the memory for apps that keep their files in RAM there: Steam
// ([[steam-integration]]) and the optional apps ([[optional-apps]]).
package live

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// MinRAM is the memory Steam and the optional apps need in the live system, where everything
// they write is kept in RAM: 12 GB of RAM, as the kernel reports it (2026-10-11, 2026-10-14).
const MinRAM = 11 << 30

// Paths read by Mode and Allowed (changed by tests).
var (
	ModeFile = "/run/owneet/mode" // live, tester or installed (owneet-mode)
	Marker   = "/run/archiso"     // present in the live system only: before owneet-mode has run
	MemInfo  = "/proc/meminfo"    // MemTotal
)

// The system's modes.
const (
	ModeLive      = "live"      // started from the USB stick: nothing is kept at power-off
	ModeTester    = "tester"    // installed by owneet-tester-install
	ModeInstalled = "installed" // installed by owneet-install
)

// Mode tells how the system runs: ModeLive, ModeTester or ModeInstalled.
func Mode() string {
	if data, err := os.ReadFile(ModeFile); err == nil {
		switch m := strings.TrimSpace(string(data)); m {
		case ModeLive, ModeTester, ModeInstalled:
			return m
		}
	}
	if _, err := os.Stat(Marker); err == nil {
		return ModeLive
	}
	return ModeInstalled
}

// Allowed: always in the installed system; in the live system with at least minRAM bytes of RAM.
func Allowed(minRAM int64) bool {
	if Mode() != ModeLive {
		return true
	}
	f, err := os.Open(MemInfo)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "MemTotal:"); ok {
			kb, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(rest), " kB"), 10, 64)
			return kb*1024 >= minRAM
		}
	}
	return false
}
