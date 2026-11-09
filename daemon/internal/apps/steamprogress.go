// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// What Steam is doing with a download, as it happens ([[step-4.2c-steam-without-its-interface]], owner,
// 2026-11-09). Steam rewrites a game's appmanifest only now and then while it downloads (Batman:
// Arkham Knight on FrankenPC: its bytes moved twice in 11 minutes, and its StateFlags never said
// "running"), so the console said "Waiting" while Steam was at 4%. Steam's content log tells each
// change at once ("AppID 208650 App update changed : Running Update,Downloading,Staging,"), and the
// room Steam takes on its library disks grows as it writes.

// Phases of a download read from Steam's content log.
const (
	steamIdle        = ""            // nothing running (none, suspended, cancelled, or not seen)
	steamPreparing   = "preparing"   // reconfiguring, verifying, preallocating, the shader cache
	steamDownloading = "downloading" // downloading or writing the game's files
)

var (
	steamContentLine = regexp.MustCompile(`^\[[^\]]*\] AppID ([0-9]+) (App|Shader) update changed : (.*)$`)
	steamCanceled    = regexp.MustCompile(`^\[[^\]]*\] AppID ([0-9]+) update canceled`)
	// a new start of Steam: what the log said before is over
	steamContentStart = regexp.MustCompile(`^\[[^\]]*\] Client version:`)
)

// steamContent follows Steam's content log from where it was read last.
type steamContent struct {
	offset int64
	phase  map[string]string // by app id
}

func steamContentLog(dir string) string { return filepath.Join(dir, "logs", "content_log.txt") }

// steamPhaseOf: the phase an "update changed" line's flags say.
func steamPhaseOf(kind, flags string) string {
	flags = strings.TrimSpace(flags)
	switch {
	case flags == "" || strings.HasPrefix(flags, "None"):
		return steamIdle
	case kind == "App" && (strings.Contains(flags, "Downloading") || strings.Contains(flags, "Staging") ||
		strings.Contains(flags, "Committing")):
		return steamDownloading
	case strings.Contains(flags, "Running Update"):
		return steamPreparing // the game's own preparation, or its shader cache before the download
	}
	return steamIdle
}

// read reads the lines Steam added since the last time (all of them the first time, or after Steam
// started a new log).
func (c *steamContent) read(dir string) {
	if c.phase == nil {
		c.phase = map[string]string{}
	}
	f, err := os.Open(steamContentLog(dir))
	if err != nil {
		return
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || st.Size() < c.offset {
		c.offset = 0
		c.phase = map[string]string{}
	}
	if _, err := f.Seek(c.offset, io.SeekStart); err != nil {
		return
	}
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return // a line not finished yet is read whole next time
		}
		c.offset += int64(len(line))
		line = strings.TrimRight(line, "\r\n")
		switch {
		case steamContentStart.MatchString(line):
			c.phase = map[string]string{}
		case steamCanceled.MatchString(line):
			c.phase[steamCanceled.FindStringSubmatch(line)[1]] = steamIdle
		default:
			if m := steamContentLine.FindStringSubmatch(line); m != nil {
				if p := steamPhaseOf(m[2], m[3]); p != steamIdle || m[2] == "App" {
					c.phase[m[1]] = p
				}
			}
		}
	}
}

// steamWrittenEvery: how often the disks are measured again during a download.
const steamWrittenEvery = 3 * time.Second

// usedOn: the bytes used on the file systems of these folders, each file system once (tests replace
// it).
var usedOn = func(dirs []string) int64 {
	var used int64
	seen := map[[2]int32]bool{}
	for _, d := range dirs {
		var fs syscall.Statfs_t
		if syscall.Statfs(d, &fs) != nil {
			continue
		}
		id := [2]int32{int32(fs.Fsid.X__val[0]), int32(fs.Fsid.X__val[1])}
		if seen[id] {
			continue
		}
		seen[id] = true
		used += int64(fs.Blocks-fs.Bfree) * int64(fs.Bsize)
	}
	return used
}

// steamWritten: what Steam has written for a download so far: how much more room its library disks
// take than when the download was first measured (base, kept in the sample). Steam writes one
// download at a time, and before a game it writes what the game needs to run (Proton, Steam's
// runtime, common files: into the main library, counted in the game's download; test VM,
// 2026-11-09), so the game's own folders do not tell it all.
func steamWritten(libs []string, s *steamSample, now time.Time) int64 {
	if !s.measured.IsZero() && now.Sub(s.measured) < steamWrittenEvery {
		return s.written
	}
	used := usedOn(libs)
	if s.measured.IsZero() {
		s.base = used
	}
	written := used - s.base
	if written < s.written {
		written = s.written // a file removed meanwhile is not progress lost
	}
	s.written, s.measured = written, now
	return written
}
