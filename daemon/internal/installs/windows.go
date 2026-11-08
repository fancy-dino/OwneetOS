// SPDX-License-Identifier: GPL-3.0-or-later

package installs

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// errWindowsNotReady: owneet-windows-run's exit code 3, Proton is not ready yet.
var errWindowsNotReady = errors.New("Windows games are not ready yet")

// windowsNeed estimates the space a Windows installer needs: twice its files (GOG's setup_*.exe
// with its setup_*-N.bin parts), as installers compress the game.
func windowsNeed(installer string) int64 {
	var size int64
	if st, err := os.Stat(installer); err == nil {
		size = st.Size()
	}
	base := strings.TrimSuffix(filepath.Base(installer), filepath.Ext(installer))
	parts, _ := filepath.Glob(filepath.Join(filepath.Dir(installer), globEscape(base)+"-*.bin"))
	for _, p := range parts {
		if st, err := os.Stat(p); err == nil {
			size += st.Size()
		}
	}
	return 2 * size
}

func globEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`)
	return r.Replace(s)
}

// folderLicence is a licence's text file next to a Windows installer (or in its docs folder):
// Windows installers keep theirs inside, which the console does not read.
func folderLicence(dir string) string {
	for _, d := range []string{dir, filepath.Join(dir, "docs")} {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Type().IsRegular() && licenceName.MatchString(e.Name()) {
				f, err := os.Open(filepath.Join(d, e.Name()))
				if err != nil {
					continue
				}
				data := make([]byte, maxLicence)
				n, _ := f.Read(data)
				f.Close()
				if n > 0 {
					return text(data[:n])
				}
			}
		}
	}
	return ""
}

// winPath is a Linux path as Proton sees it (drive Z: is the whole file system).
func winPath(p string) string { return "Z:" + strings.ReplaceAll(p, "/", `\`) }

// windowsArgs are an installer's silent options, installing into winDest.
func windowsArgs(kind, installer, winDest string) []string {
	switch kind {
	case KindInno:
		return []string{installer, "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/SP-", "/NOICONS", "/DIR=" + winDest}
	case KindNSIS:
		return []string{installer, "/S", "/D=" + winDest} // /D last, never quoted
	default: // MSI
		return []string{"msiexec", "/i", winPath(installer), "/qn", "TARGETDIR=" + winDest, "INSTALLDIR=" + winDest}
	}
}

// windowsInstall runs a Windows installer silently, in the game's prefix. NSIS and MSI cannot
// take a folder with spaces: they install through a link without any, kept for the game (its
// registry may point there).
func (m *Manager) windowsInstall(ctx context.Context, j *job) (string, error) {
	dest, link := j.Dest, ""
	if j.Kind != KindInno {
		sum := sha1.Sum([]byte(j.Dest))
		link = filepath.Join(m.linksDir(), hex.EncodeToString(sum[:])[:12])
		if err := os.MkdirAll(m.linksDir(), 0o755); err != nil {
			return "", err
		}
		os.Remove(link)
		if err := os.Symlink(j.Dest, link); err != nil {
			return "", err
		}
		dest = link
	}
	argv := append([]string{m.WindowsRun, j.req.Prefix}, windowsArgs(j.Kind, j.req.Installer, winPath(dest))...)
	// no percentage: what it has written so far, and whether it seems stuck (6.8)
	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	go m.watchWritten(watchCtx, j)
	sum := sha1.Sum([]byte(j.Dest))
	unit := "owneet-install-" + hex.EncodeToString(sum[:])[:10] + ".service"
	err := m.Runner.Run(ctx, unit, argv, m.Env())
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 3 {
		return link, errWindowsNotReady
	}
	if err != nil {
		return link, err
	}
	// an installer that ends well but wrote nothing (e.g. it needed the user)
	entries, _ := os.ReadDir(j.Dest)
	wrote := false
	for _, e := range entries {
		if e.Name() != MarkInstalling && !contains(j.keep, e.Name()) {
			wrote = true
			break
		}
	}
	if !wrote {
		return link, errors.New("the installer wrote no files")
	}
	return link, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// systemdRun runs the installer as a transient unit of the user's systemd and waits for it
// (systemd-run --wait gives its exit code); a cancelled install stops the unit, with every
// process the installer started.
type systemdRun struct{}

func (systemdRun) Run(ctx context.Context, unit string, argv, env []string) error {
	args := []string{"--user", "--wait", "--collect", "--quiet", "--unit=" + unit,
		"--description=OwneetOS install", "--property=KillMode=control-group"}
	for _, e := range env {
		args = append(args, "--setenv="+e)
	}
	args = append(append(args, "--"), argv...)
	cmd := exec.Command("systemd-run", args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if err := exec.Command("systemctl", "--user", "stop", unit).Run(); err != nil {
			cmd.Process.Kill()
		}
		<-done
		return fmt.Errorf("stopped: %w", ctx.Err())
	}
}

// writtenEvery: how often a Windows installer's folder is measured.
var writtenEvery = 5 * time.Second

// watchWritten measures what a Windows installer has written into the game's folder, and marks it
// stalled when nothing grows for StallAfter (and not stalled again once it grows).
func (m *Manager) watchWritten(ctx context.Context, j *job) {
	stall := m.StallAfter
	if stall == 0 {
		stall = 10 * time.Minute
	}
	tick := time.NewTicker(writtenEvery)
	defer tick.Stop()
	last, grew := int64(-1), time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		size := dirSize(j.Dest)
		if size != last {
			last, grew = size, time.Now()
		}
		stalled := time.Since(grew) >= stall
		m.mu.Lock()
		changed := j.Stalled != stalled
		j.Stalled = stalled
		m.mu.Unlock()
		if changed && stalled {
			m.Log.Warn("the Windows installer writes nothing", "dest", j.Dest, "for", stall)
			m.event("install.stalled", map[string]string{"dest": j.Dest, "title": j.Title})
		}
		m.progress(j, size)
		if changed {
			m.publish()
		}
	}
}

// dirSize: the bytes of the files in a folder, sub-folders too.
func dirSize(dir string) int64 {
	var size int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return size
}
