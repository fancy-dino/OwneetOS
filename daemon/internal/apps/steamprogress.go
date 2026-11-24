// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"bufio"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	// downloading the game's shader pre-cache before the game itself: Steam shows its bytes (THE FINALS,
	// 1.1 GB, FrankenPC 2026-11-23, [[open-items]] 25), so the console shows it as the download it is
	steamShaders = "shaders"
)

var (
	steamContentLine = regexp.MustCompile(`^\[[^\]]*\] AppID ([0-9]+) (App|Shader) update changed : (.*)$`)
	steamCanceled    = regexp.MustCompile(`^\[[^\]]*\] AppID ([0-9]+) update canceled`)
	// the sizes of the update Steam starts ("AppID 2073850 update started : download 0/1166…, store 0/0,
	// reuse 0/0, delta 0/0, stage 0/1201…"): what it has already written of them, and what it writes in all
	steamStarted = regexp.MustCompile(`^\[[^\]]*\] AppID ([0-9]+) update started : download [0-9]+/[0-9]+, .*stage ([0-9]+)/([0-9]+)`)
	// a new start of Steam: what the log said before is over
	steamContentStart = regexp.MustCompile(`^\[[^\]]*\] Client version:`)
)

// steamContent follows Steam's content log from where it was read last.
type steamContent struct {
	offset  int64
	phase   map[string]string      // by app id
	started map[string]steamUpdate // the last update Steam started, by app id
	updates int                    // how many it has started
}

// steamUpdate is an update as Steam starts it ("update started : … stage DONE/TOTAL").
type steamUpdate struct {
	n           int   // tells it from the app's other updates
	done, total int64 // bytes written by then, bytes to write
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
	case kind == "Shader" && !strings.Contains(flags, "Stopping") && (strings.Contains(flags, "Downloading") ||
		strings.Contains(flags, "Staging") || strings.Contains(flags, "Committing")):
		return steamShaders
	case strings.Contains(flags, "Stopping"): // stopped (another game first, cancelled): not preparing
		return steamIdle
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
	if c.started == nil {
		c.started = map[string]steamUpdate{}
	}
	f, err := os.Open(steamContentLog(dir))
	if err != nil {
		return
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || st.Size() < c.offset {
		c.offset = 0
		c.phase = map[string]string{}
		c.started = map[string]steamUpdate{}
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
			c.started = map[string]steamUpdate{}
		case steamCanceled.MatchString(line):
			c.phase[steamCanceled.FindStringSubmatch(line)[1]] = steamIdle
		case steamStarted.MatchString(line):
			m := steamStarted.FindStringSubmatch(line)
			done, _ := strconv.ParseInt(m[2], 10, 64)
			total, _ := strconv.ParseInt(m[3], 10, 64)
			c.updates++
			c.started[m[1]] = steamUpdate{n: c.updates, done: done, total: total}
		default:
			if m := steamContentLine.FindStringSubmatch(line); m != nil {
				p := steamPhaseOf(m[2], m[3])
				switch {
				case p != steamIdle || m[2] == "App":
					c.phase[m[1]] = p
				case c.phase[m[1]] == steamShaders: // the shader pre-cache over: Steam prepares the game next
					c.phase[m[1]] = steamPreparing
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

// steamShaderDone: how much of its shader pre-cache update u Steam has written: what is on the disks in
// its download folder (steamapps/shadercache/APPID/downloads), measured every steamWrittenEvery. The
// folder already holds what a resumed update had done (u.done), so the two are never added. Steam
// downloads it in several updates one after the other (THE FINALS: three), each counted on its own;
// within one, never a step back (the files leave the folder at its end).
func steamShaderDone(libs []string, appid string, u steamUpdate, s *steamSample, now time.Time) int64 {
	if s.shaderUpdate != u.n {
		s.shaderUpdate, s.shaderDone, s.shaderMeasured = u.n, u.done, time.Time{}
	}
	if s.shaderMeasured.IsZero() || now.Sub(s.shaderMeasured) >= steamWrittenEvery {
		s.shaderMeasured = now
		var written int64
		for _, lib := range libs {
			written += writtenIn(filepath.Join(lib, "steamapps", "shadercache", appid, "downloads"))
		}
		if written > s.shaderDone {
			s.shaderDone = written
		}
	}
	if s.shaderDone > u.total {
		return u.total
	}
	return s.shaderDone
}

// writtenIn: the bytes really written in a folder's files. Steam writes a file's pieces out of order, so
// its length runs ahead (Dota 2's: 53% of it when Steam had written 28%): the room it takes on the
// disk is counted, at most its length.
func writtenIn(dir string) int64 {
	var written int64
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		size := info.Size()
		if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Blocks*512 < size {
			size = st.Blocks * 512
		}
		written += size
		return nil
	})
	return written
}
