// SPDX-License-Identifier: GPL-3.0-or-later

// Package installs installs games from the installers found in their folders
// ([[step-4.6-installers]], owner 2026-10-19, [[games-on-disk]]): on the console's own disk, in
// its games folder, one at a time, in the background.
//
// GOG's Linux installers (.sh, MojoSetup) are extracted, never run: their data/noarch is the
// game, and the archive gives the exact size (the only installers with a percentage). Windows
// installers (Inno Setup, NSIS, MSI) run silently under Proton in the game's prefix
// (owneet-windows-run), so the publisher's own scripts run too; they tell no progress.
//
// A folder being installed holds a mark (.owneet-installing) that the interface skips; a finished
// one a mark (.owneet-installed) that lets the interface uninstall it. Installs cut short (a
// restart) are removed when owneetd starts again.
package installs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// Kinds of installers the console can use.
const (
	KindSh   = "sh"   // GOG's Linux installer (MojoSetup): extracted
	KindInno = "inno" // Inno Setup (e.g. GOG's setup_*.exe): run silently under Proton
	KindNSIS = "nsis" // NSIS: run silently under Proton
	KindMSI  = "msi"  // Windows Installer: run silently under Proton
)

// States of an install.
const (
	StateQueued     = "queued"     // waiting for the one in progress
	StateInstalling = "installing" // in progress
)

// Reasons an install did not finish ("install.failed").
const (
	ReasonCancelled  = "cancelled"   // stopped by the user
	ReasonSourceGone = "source_gone" // the installer's disk was unplugged
	ReasonWindows    = "windows"     // Windows games are not ready yet
	ReasonNoSpace    = "no_space"    // the console's disk is full
	ReasonFailed     = "failed"      // the installer failed
)

// Marks in a game's folder.
const (
	MarkInstalling = ".owneet-installing"
	MarkInstalled  = ".owneet-installed"
)

// GamesFolders are the names of a games folder (as the interface's DiskGames::folderNames,
// compared without case).
var GamesFolders = []string{"games", "giochi"}

var (
	ErrInvalid  = errors.New("not an installer or a games folder the console can use")
	ErrNoSpace  = errors.New("not enough space on the console's disk")
	ErrExists   = errors.New("a game with that name is already on the console's disk")
	ErrNotFound = errors.New("no such install or installed game")
	ErrInUse    = errors.New("the game is running: close it first")
)

// Install is one install, queued or in progress.
type Install struct {
	Dest  string `json:"dest"` // the game's folder on the console's disk: the install's id
	Title string `json:"title"`
	Kind  string `json:"kind"`
	State string `json:"state"`
	Done  int64  `json:"done"`  // bytes written
	Total int64  `json:"total"` // bytes to write; 0 for Windows installers (no percentage)
}

// Status is GET /v1/installs and the "installs.changed" event.
type Status struct {
	Installs []Install `json:"installs"`
}

// Request asks to install a game.
type Request struct {
	Installer string `json:"installer"` // the installer's file
	Kind      string `json:"kind"`
	Dest      string `json:"dest"`             // the game's folder on the console's disk
	Prefix    string `json:"prefix,omitempty"` // a Windows installer: the game's prefix
	Title     string `json:"title"`
}

// Info is what the console tells before installing.
type Info struct {
	Need    int64  `json:"need"`    // bytes on the console's disk
	Exact   bool   `json:"exact"`   // Need comes from the archive; else an estimate
	Free    int64  `json:"free"`    // free bytes on the console's disk
	Licence string `json:"licence"` // the licence's text; "" when not found
}

// Runner runs a Windows installer in its own systemd unit and waits for it; cancelling ctx stops
// the unit (and every process the installer started).
type Runner interface {
	Run(ctx context.Context, unit string, argv, env []string) error
}

// Manager installs games, one at a time.
type Manager struct {
	Broker *events.Broker
	Log    *slog.Logger
	// Home is the console user's home folder: its games folder, the prefixes, Windows' links.
	Home string
	// StateDir remembers the installs in progress, to clean them after a restart.
	StateDir string
	// WindowsRun is owneet-windows-run; Env the console session's variables (DISPLAY…) for it.
	WindowsRun string
	Env        func() []string
	Runner     Runner
	// Free tells the free bytes of the disk holding path (statfs by default).
	Free func(path string) (int64, error)
	// InUse tells whether a process works in a folder (its working folder or its program is in
	// it: /proc by default)
	InUse func(dir string) bool

	mu    sync.Mutex
	queue []*job
	kick  chan struct{}
	once  sync.Once
	sent  time.Time // the last "installs.changed" sent while bytes move
}

type job struct {
	Install
	req    Request
	ctx    context.Context
	cancel context.CancelFunc
	keep   []string // the folder's entries before the install (an installer in the folder itself)
}

func (m *Manager) init() {
	m.once.Do(func() {
		m.kick = make(chan struct{}, 1)
		if m.Free == nil {
			m.Free = statfsFree
		}
		if m.Runner == nil {
			m.Runner = systemdRun{}
		}
		if m.Env == nil {
			m.Env = func() []string { return nil }
		}
		if m.InUse == nil {
			m.InUse = procInUse
		}
	})
}

func statfsFree(path string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * st.Bsize, nil
}

// Status lists the installs, the one in progress first.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := Status{Installs: []Install{}}
	for _, j := range m.queue {
		out.Installs = append(out.Installs, j.Install)
	}
	return out
}

func (m *Manager) publish() { m.event("installs.changed", m.Status()) }

func (m *Manager) event(name string, data any) {
	if m.Broker != nil {
		m.Broker.Publish(name, data)
	}
}

// within: path is inside dir (both cleaned).
func within(path, dir string) bool {
	return strings.HasPrefix(path, dir+string(filepath.Separator))
}

// gamesDest checks that dest is a game's folder in a games folder of the home folder.
func (m *Manager) gamesDest(dest string) (string, error) {
	dest = filepath.Clean(dest)
	if !filepath.IsAbs(dest) {
		return "", ErrInvalid
	}
	name, parent := filepath.Base(dest), filepath.Dir(dest)
	if name == "" || strings.HasPrefix(name, ".") || filepath.Dir(parent) != filepath.Clean(m.Home) {
		return "", ErrInvalid
	}
	for _, g := range GamesFolders {
		if strings.EqualFold(filepath.Base(parent), g) {
			return dest, nil
		}
	}
	return "", ErrInvalid
}

// installerPath checks that the installer is a file on a disk the console mounted, or in the
// home folder's games folder.
func (m *Manager) installerPath(path string) (string, error) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", ErrInvalid
	}
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return "", ErrInvalid
	}
	if within(path, "/run/media/owneet") {
		return path, nil
	}
	rel, err := filepath.Rel(filepath.Clean(m.Home), path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", ErrInvalid
	}
	top, _, _ := strings.Cut(rel, string(filepath.Separator))
	for _, g := range GamesFolders {
		if strings.EqualFold(top, g) {
			return path, nil
		}
	}
	return "", ErrInvalid
}

func validKind(kind string) bool {
	switch kind {
	case KindSh, KindInno, KindNSIS, KindMSI:
		return true
	}
	return false
}

// Inspect tells what installing needs: the space (exact for GOG's .sh, an estimate for Windows
// installers), the free space, and the licence's text when the console finds it.
func (m *Manager) Inspect(installer, kind string) (Info, error) {
	m.init()
	path, err := m.installerPath(installer)
	if err != nil || !validKind(kind) {
		return Info{}, ErrInvalid
	}
	var info Info
	if kind == KindSh {
		a, err := openArchive(path)
		if err != nil {
			return Info{}, ErrInvalid
		}
		info.Need, info.Exact, info.Licence = a.total, true, a.licence()
		a.Close()
	} else {
		info.Need = windowsNeed(path)
		info.Licence = folderLicence(filepath.Dir(path))
	}
	info.Free, _ = m.Free(m.Home)
	return info, nil
}

// Install queues an install; it starts when the one in progress is done.
func (m *Manager) Install(req Request) error {
	m.init()
	installer, err := m.installerPath(req.Installer)
	if err != nil || !validKind(req.Kind) {
		return ErrInvalid
	}
	dest, err := m.gamesDest(req.Dest)
	if err != nil {
		return err
	}
	prefix := ""
	if req.Kind != KindSh {
		prefix = filepath.Clean(req.Prefix)
		if !within(prefix, filepath.Join(m.Home, ".local", "share", "owneet", "windows", "prefixes")) {
			return ErrInvalid
		}
	}
	// the folder may exist only when the installer is in it (a game's folder on the console's
	// disk): its files are kept, and stay when the game is uninstalled
	var keep []string
	if entries, err := os.ReadDir(dest); err == nil {
		if filepath.Dir(installer) != dest || exists(filepath.Join(dest, MarkInstalled)) {
			return ErrExists
		}
		for _, e := range entries {
			keep = append(keep, e.Name())
		}
	}
	info, err := m.Inspect(installer, req.Kind)
	if err != nil {
		return err
	}
	if info.Free > 0 && info.Need > info.Free {
		return ErrNoSpace
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = filepath.Base(dest)
	}
	m.mu.Lock()
	for _, j := range m.queue {
		if j.Dest == dest {
			m.mu.Unlock()
			return nil // already queued
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	total := int64(0)
	if req.Kind == KindSh {
		total = info.Need
	}
	j := &job{Install: Install{Dest: dest, Title: title, Kind: req.Kind, State: StateQueued, Total: total},
		req: Request{Installer: installer, Kind: req.Kind, Dest: dest, Prefix: prefix, Title: title}, ctx: ctx, cancel: cancel, keep: keep}
	m.queue = append(m.queue, j)
	m.saveActive()
	m.mu.Unlock()
	m.Log.Info("install queued", "dest", dest, "kind", req.Kind, "installer", installer)
	m.publish()
	select {
	case m.kick <- struct{}{}:
	default:
	}
	return nil
}

// Stop stops an install (in progress: what it wrote is removed) or takes it out of the queue.
func (m *Manager) Stop(dest string) error {
	m.init()
	dest = filepath.Clean(dest)
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, j := range m.queue {
		if j.Dest != dest {
			continue
		}
		j.cancel()
		if j.State == StateQueued {
			m.queue = append(m.queue[:i], m.queue[i+1:]...)
			m.saveActive()
			go func() {
				m.publish()
				m.event("install.failed", map[string]string{"dest": dest, "title": j.Title, "reason": ReasonCancelled})
			}()
		}
		return nil
	}
	return ErrNotFound
}

// Uninstall removes a game the console installed: everything in its folder but what was there
// before (an installer in the folder itself); its prefix (with its saves) stays.
func (m *Manager) Uninstall(dest string) error {
	m.init()
	dest, err := m.gamesDest(dest)
	if err != nil {
		return err
	}
	var mark struct {
		Keep []string `json:"keep"`
		Link string   `json:"link"`
	}
	data, err := os.ReadFile(filepath.Join(dest, MarkInstalled))
	if err != nil {
		return ErrNotFound
	}
	_ = json.Unmarshal(data, &mark)
	// never under a running game (FrankenPC, 2026-10-19: ScummVM lost its files and crashed)
	if m.InUse(dest) {
		return ErrInUse
	}
	m.mu.Lock()
	for _, j := range m.queue {
		if j.Dest == dest {
			m.mu.Unlock()
			return ErrExists
		}
	}
	m.mu.Unlock()
	if err := m.remove(dest, mark.Keep); err != nil {
		return err
	}
	if mark.Link != "" && within(filepath.Clean(mark.Link), m.linksDir()) {
		os.Remove(mark.Link)
	}
	m.Log.Info("game uninstalled", "dest", dest)
	return nil
}

// remove deletes the folder, or only what is not in keep.
func (m *Manager) remove(dest string, keep []string) error {
	if len(keep) == 0 {
		return os.RemoveAll(dest)
	}
	kept := map[string]bool{}
	for _, k := range keep {
		kept[k] = true
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !kept[e.Name()] {
			if err := os.RemoveAll(filepath.Join(dest, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// Run installs what is queued until ctx ends; first it removes installs cut short by a restart.
func (m *Manager) Run(ctx context.Context) {
	m.init()
	m.cleanCutShort()
	for {
		m.mu.Lock()
		var next *job
		for _, j := range m.queue {
			if j.State == StateQueued {
				next = j
				break
			}
		}
		if next != nil {
			next.State = StateInstalling
		}
		m.mu.Unlock()
		if next == nil {
			select {
			case <-ctx.Done():
				return
			case <-m.kick:
				continue
			}
		}
		m.publish()
		m.run(ctx, next)
	}
}

// run does one install and tells how it went.
func (m *Manager) run(ctx context.Context, j *job) {
	jctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-j.ctx.Done():
			cancel()
		case <-jctx.Done():
		}
	}()
	m.Log.Info("installing", "dest", j.Dest, "kind", j.Kind)
	link, err := m.install(jctx, j)
	reason := ""
	switch {
	case err == nil:
	case j.ctx.Err() != nil:
		reason = ReasonCancelled
	case !exists(j.req.Installer):
		reason = ReasonSourceGone
	case errors.Is(err, syscall.ENOSPC):
		reason = ReasonNoSpace
	case errors.Is(err, errWindowsNotReady):
		reason = ReasonWindows
	default:
		reason = ReasonFailed
	}
	if reason != "" {
		m.Log.Warn("install did not finish", "dest", j.Dest, "reason", reason, "err", err)
		if rerr := m.remove(j.Dest, j.keep); rerr != nil {
			m.Log.Warn("cannot remove an unfinished install", "dest", j.Dest, "err", rerr)
		}
		if link != "" {
			os.Remove(link)
		}
	} else {
		mark, _ := json.Marshal(map[string]any{"installer": j.req.Installer, "kind": j.Kind, "keep": j.keep,
			"link": link, "installed": time.Now().UTC().Format(time.RFC3339)})
		if werr := os.WriteFile(filepath.Join(j.Dest, MarkInstalled), mark, 0o644); werr != nil {
			m.Log.Warn("cannot mark the game installed", "dest", j.Dest, "err", werr)
		}
		os.Remove(filepath.Join(j.Dest, MarkInstalling))
		m.Log.Info("game installed", "dest", j.Dest)
	}
	m.mu.Lock()
	for i, q := range m.queue {
		if q == j {
			m.queue = append(m.queue[:i], m.queue[i+1:]...)
			break
		}
	}
	m.saveActive()
	m.mu.Unlock()
	m.publish()
	if reason == "" {
		m.event("install.done", map[string]string{"dest": j.Dest, "title": j.Title})
	} else {
		m.event("install.failed", map[string]string{"dest": j.Dest, "title": j.Title, "reason": reason})
	}
}

// install writes the game into its folder (marked while it works); link: the folder's link for
// Windows installers that cannot take a path with spaces.
func (m *Manager) install(ctx context.Context, j *job) (link string, err error) {
	if err := os.MkdirAll(j.Dest, 0o755); err != nil {
		return "", err
	}
	mark, _ := json.Marshal(map[string]any{"installer": j.req.Installer, "keep": j.keep})
	if err := os.WriteFile(filepath.Join(j.Dest, MarkInstalling), mark, 0o644); err != nil {
		return "", err
	}
	if j.Kind == KindSh {
		a, err := openArchive(j.req.Installer)
		if err != nil {
			return "", err
		}
		defer a.Close()
		return "", a.extract(ctx, j.Dest, func(done int64) { m.progress(j, done) })
	}
	return m.windowsInstall(ctx, j)
}

// progress records the bytes written; "installs.changed" at most twice a second.
func (m *Manager) progress(j *job, done int64) {
	m.mu.Lock()
	j.Done = done
	send := time.Since(m.sent) >= 500*time.Millisecond
	if send {
		m.sent = time.Now()
	}
	m.mu.Unlock()
	if send {
		m.publish()
	}
}

// saveActive writes the folders being installed (callers hold m.mu).
func (m *Manager) saveActive() {
	if m.StateDir == "" {
		return
	}
	var dests []string
	for _, j := range m.queue {
		dests = append(dests, j.Dest)
	}
	data, _ := json.Marshal(dests)
	if err := os.MkdirAll(m.StateDir, 0o755); err == nil {
		os.WriteFile(filepath.Join(m.StateDir, "installs.json"), data, 0o644)
	}
}

// cleanCutShort removes the installs a restart cut short (their folder still has its mark).
func (m *Manager) cleanCutShort() {
	if m.StateDir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(m.StateDir, "installs.json"))
	if err != nil {
		return
	}
	var dests []string
	_ = json.Unmarshal(data, &dests)
	for _, d := range dests {
		dest, err := m.gamesDest(d)
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dest, MarkInstalling))
		if err != nil {
			continue
		}
		var mark struct {
			Keep []string `json:"keep"`
		}
		_ = json.Unmarshal(raw, &mark)
		m.Log.Info("removing an install cut short", "dest", dest)
		if err := m.remove(dest, mark.Keep); err != nil {
			m.Log.Warn("cannot remove an install cut short", "dest", dest, "err", err)
		} else if len(mark.Keep) > 0 {
			os.Remove(filepath.Join(dest, MarkInstalling))
		}
	}
	os.Remove(filepath.Join(m.StateDir, "installs.json"))
}

func (m *Manager) linksDir() string {
	return filepath.Join(m.Home, ".local", "share", "owneet", "installs")
}

// procInUse: a process's working folder or program is in dir (the console user's processes).
func procInUse(dir string) bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name()[0] < '0' || e.Name()[0] > '9' {
			continue
		}
		for _, link := range []string{"cwd", "exe"} {
			if target, err := os.Readlink(filepath.Join("/proc", e.Name(), link)); err == nil &&
				(target == dir || within(target, dir)) {
				return true
			}
		}
	}
	return false
}
