// SPDX-License-Identifier: GPL-3.0-or-later

// Package live tells the live system (started from the USB stick) from the installed one, and
// whether the live system has the memory for apps that keep their files in RAM there: Steam
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

// Paths read by Allowed (changed by tests).
var (
	Marker  = "/run/archiso"  // present in the live system only
	MemInfo = "/proc/meminfo" // MemTotal
)

// Allowed: always in the installed system; in the live system with at least minRAM bytes of RAM.
func Allowed(minRAM int64) bool {
	if _, err := os.Stat(Marker); err != nil {
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
