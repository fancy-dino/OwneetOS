// SPDX-License-Identifier: GPL-3.0-or-later

// Package gamedata names a game's own folders on the console's disk (owneet-frontend's
// DiskGames::gameDataDir): a Windows game's prefix (its saves, its settings, what its installer
// wrote), a DOS game's data, and the Windows game's logs (named after its prefix). They go with the
// game when it moves (package moves, owner 2026-11-10) and with it when it leaves an internal disk
// for good (package installs, owner 2026-11-12, [[disks]]).
package gamedata

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Kinds of a game's own folders that hold what the game keeps (moved and copied with it).
var Kinds = []string{"windows/prefixes", "dos"}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Dir is ~/.local/share/owneet/KIND/<name>-<hash>: the name made of the game folder's name (a-z,
// 0-9 and "-", at most 40 characters), the hash the first 10 hex digits of the SHA-1 of the game's
// folder.
func Dir(home, kind, gameDir string) string {
	slug := notSlug.ReplaceAllString(strings.ToLower(filepath.Base(gameDir)), "-")
	if len(slug) > 40 {
		slug = slug[:40]
	}
	slug = strings.Trim(slug, "-")
	sum := sha1.Sum([]byte(gameDir))
	return filepath.Join(home, ".local", "share", "owneet", kind, slug+"-"+hex.EncodeToString(sum[:])[:10])
}

// All is every folder of the game's own: Kinds, and its Windows logs (named as its prefix).
func All(home, gameDir string) []string {
	var out []string
	for _, kind := range Kinds {
		out = append(out, Dir(home, kind, gameDir))
	}
	prefix := Dir(home, "windows/prefixes", gameDir)
	return append(out, filepath.Join(home, ".local", "share", "owneet", "windows", "logs", filepath.Base(prefix)))
}

// External tells whether a folder is on a disk plugged in from outside (USB): the device its file
// system is mounted from hangs from a USB bus in /sys. The console's own disk and the disks inside
// it (SATA, NVMe) are not.
func External(dir string) bool {
	dev := mountSource(dir)
	if !strings.HasPrefix(dev, "/dev/") {
		return false
	}
	real, err := filepath.EvalSymlinks(dev)
	if err != nil {
		real = dev
	}
	sys, err := filepath.EvalSymlinks(filepath.Join("/sys/class/block", filepath.Base(real)))
	return err == nil && strings.Contains(sys, "/usb")
}

// mountSource is the device of the mount that holds dir (/proc/self/mountinfo: the longest mount
// point that is dir or above it).
func mountSource(dir string) string {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return ""
	}
	defer f.Close()
	dir = filepath.Clean(dir)
	best, src := "", ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		sep := -1
		for i, x := range fields {
			if x == "-" {
				sep = i
				break
			}
		}
		if len(fields) < 5 || sep < 0 || sep+2 >= len(fields) {
			continue
		}
		mp := unescape(fields[4])
		if (dir == mp || strings.HasPrefix(dir, strings.TrimSuffix(mp, "/")+"/")) && len(mp) >= len(best) {
			best, src = mp, fields[sep+2]
		}
	}
	return src
}

// unescape undoes mountinfo's octal escapes (a space is \040)
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			n := 0
			ok := true
			for _, c := range s[i+1 : i+4] {
				if c < '0' || c > '7' {
					ok = false
					break
				}
				n = n*8 + int(c-'0')
			}
			if ok {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
