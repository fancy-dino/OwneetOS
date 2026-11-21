// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Native Linux games outside Steam run in Valve's "Steam Linux Runtime 1.0 (scout)", as Steam runs
// them on a Steam Deck and as Heroic runs GOG's (step 3 of [[game-compatibility-method]],
// [[umu-fixes-and-runtime]], owner 2026-11-20): umu-launcher's "umu-scout" tool (Valve's scout
// runtime, 32-bit libraries included) in the soldier container. It is downloaded once, right
// after what Windows games need (the owner's choice: with the Windows-games download), by
// umu-launcher itself; owneet-native-run starts the games in it, or as before while it is missing.

// NativePreparer downloads the runtime of native games and returns umu-scout's folder.
type NativePreparer interface {
	PrepareNative(ctx context.Context) (toolDir string, err error)
}

// nativeRetry: after a failed download, the next try (a network change tries at once)
const nativeRetry = 10 * time.Minute

// NativeDir is umu-scout's folder made ready earlier, if it is still there.
func (m *Manager) NativeDir() string {
	data, err := os.ReadFile(filepath.Join(m.DataDir, "native"))
	if err != nil {
		return ""
	}
	dir := strings.TrimSpace(string(data))
	if _, err := os.Stat(filepath.Join(dir, "toolmanifest.vdf")); err != nil {
		return ""
	}
	return dir
}

// maybeNative starts the native games' download once Windows games are ready, online, at most
// one at a time.
func (m *Manager) maybeNative(ctx context.Context) {
	m.mu.Lock()
	busy := m.nativeRunning
	recent := !m.nativeFailed.IsZero() && time.Since(m.nativeFailed) < nativeRetry
	m.mu.Unlock()
	if busy || recent || m.Native == nil || m.NativeDir() != "" {
		return
	}
	if m.Online != nil && !m.Online() {
		return
	}
	m.mu.Lock()
	m.nativeRunning = true
	m.mu.Unlock()
	go m.prepareNative(ctx)
}

func (m *Manager) prepareNative(ctx context.Context) {
	defer func() {
		m.mu.Lock()
		m.nativeRunning = false
		m.mu.Unlock()
	}()
	if m.Log != nil {
		m.Log.Info("getting native games' runtime ready")
	}
	dir, err := m.Native.PrepareNative(ctx)
	if err == nil && dir == "" {
		err = errors.New("no umu-scout folder")
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(m.DataDir, "native"), []byte(dir+"\n"), 0o644)
	}
	if err != nil {
		if m.Log != nil {
			m.Log.Warn("native games' runtime not ready: they start as before", "err", err)
		}
		m.mu.Lock()
		m.nativeFailed = time.Now()
		m.mu.Unlock()
		return
	}
	if m.Log != nil {
		m.Log.Info("native games' runtime is ready", "tool", dir)
	}
}

// PrepareNative runs `true` with umu-run in umu-scout: umu-launcher downloads umu-scout (about
// 60 MB, from its GitHub releases, checked with its SHA-512) and the soldier runtime it needs (from
// Valve's repository, checked with its SHA-256), then runs `true` in it, which proves it works.
func (u *Umu) PrepareNative(ctx context.Context) (string, error) {
	if u.UmuDir == "" {
		return "", errors.New("umu-launcher's folder unknown")
	}
	if err := os.MkdirAll(u.PrefixDir, 0o755); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, u.Command, "true")
	// no prefix is made for a tool that is not Proton: WINEPREFIX only keeps umu-launcher's
	// default folder (~/Games) from being chosen
	cmd.Env = append(os.Environ(), "GAMEID=umu-0", "PROTONPATH=umu-scout", "UMU_LOG=1", "WINEPREFIX="+u.PrefixDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // stopped with everything it started
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("umu-run in umu-scout: %w (%s)", err, lastLine(out))
	}
	tool := filepath.Join(u.UmuDir, "compatibilitytools", "umu-scout")
	for _, need := range []string{filepath.Join(tool, "toolmanifest.vdf"), filepath.Join(u.UmuDir, "steamrt2", "VERSIONS.txt")} {
		if _, err := os.Stat(need); err != nil {
			return "", fmt.Errorf("umu-scout not complete: %w", err)
		}
	}
	return tool, nil
}

// lastLine is the last line of a program's output that is not empty.
func lastLine(out []byte) string {
	lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	return string(lines[len(lines)-1])
}
