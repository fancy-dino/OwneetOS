// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"bufio"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Umu prepares Windows games with umu-launcher: run once with GE-Proton and nothing to start, it
// downloads the latest GE-Proton (about 500 MB, from its GitHub releases, checked with its
// SHA-512) and the Steam Linux Runtime Proton needs (about 150 MB, from Valve's repository,
// checked with its SHA-256), then would start the program: it is stopped there (checked on the
// builder VM, umu-launcher 1.4, 2026-10-16). Its log says which part it is on; the size of the
// files being downloaded (in ~/.cache/umu) gives the progress within each part.
type Umu struct {
	Command   string // "umu-run"
	PrefixDir string // a prefix of its own (nothing runs in it)
	CacheDir  string // umu-launcher's downloads, e.g. ~/.cache/umu
}

// Rough sizes of the two downloads, for the progress only (a part never shows more than its share)
const (
	protonBytes  = 520 << 20
	runtimeBytes = 160 << 20
)

// Prepare runs umu-run until GE-Proton and its runtime are ready; it returns GE-Proton's folder.
func (u *Umu) Prepare(ctx context.Context, progress func(int)) (string, error) {
	cmd := exec.CommandContext(ctx, u.Command, "")
	cmd.Env = append(os.Environ(), "GAMEID=umu-default", "PROTONPATH=GE-Proton", "UMU_LOG=1",
		"WINEPREFIX="+u.PrefixDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // stopped with everything it started
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	out, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	cmd.Stderr = cmd.Stdout
	if err := os.MkdirAll(u.PrefixDir, 0o755); err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	// part: 0 nothing yet, 1 GE-Proton downloading, 2 GE-Proton unpacking, 3 runtime
	// downloading, 4 runtime checked
	var mu sync.Mutex
	part := 0
	done := make(chan struct{})
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
			}
			mu.Lock()
			p := part
			mu.Unlock()
			progress(percentOf(p, downloadedBytes(u.CacheDir)))
		}
	}()

	proton, ready := u.follow(out, func(p int) {
		mu.Lock()
		part = p
		mu.Unlock()
		progress(percentOf(p, downloadedBytes(u.CacheDir)))
	})
	close(done)
	if ready {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // nothing is to start
	}
	waitErr := cmd.Wait()
	switch {
	case ready && proton != "":
		return proton, nil
	case ctx.Err() != nil:
		return "", ctx.Err()
	case waitErr != nil:
		return "", waitErr
	default:
		return "", errors.New("umu-run ended before Proton was ready")
	}
}

// follow reads umu-run's log until it is about to start the program: GE-Proton's folder, and
// whether both downloads are ready.
func (u *Umu) follow(r io.Reader, part func(int)) (proton string, ready bool) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.Contains(line, "Downloading GE-Proton"):
			part(1)
		case strings.Contains(line, "Extracting GE-Proton"):
			part(2)
		case strings.Contains(line, "Downloading steamrt"):
			part(3)
		case strings.Contains(line, "mtree is OK"):
			part(4)
		case strings.Contains(line, "DEBUG: PROTONPATH=/"):
			proton = strings.TrimSpace(line[strings.Index(line, "PROTONPATH=")+len("PROTONPATH="):])
		case strings.Contains(line, "INFO: Running '") && strings.Contains(line, "using runtime"):
			return proton, true
		}
	}
	return proton, false
}

// percentOf turns the part and the bytes downloaded so far into 0…99: GE-Proton's download is
// 0–60, its unpacking 60–65, the runtime 65–97
func percentOf(part int, bytes int64) int {
	share := func(from, to int, total int64) int {
		if bytes >= total {
			return to
		}
		return from + int(int64(to-from)*bytes/total)
	}
	switch part {
	case 1:
		return share(0, 60, protonBytes)
	case 2:
		return 62
	case 3:
		return share(65, 97, runtimeBytes)
	case 4:
		return 98
	}
	return 0
}

// downloadedBytes is the size of the files umu-launcher is downloading now (*.parts, and the
// archive it has just finished) in its cache folder.
func downloadedBytes(cache string) int64 {
	var total int64
	_ = filepath.WalkDir(cache, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.Contains(name, ".parts") || strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tar.xz") {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}
