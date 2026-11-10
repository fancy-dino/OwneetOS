// SPDX-License-Identifier: GPL-3.0-or-later

// Package moves copies games between disks and, when the player asks, removes the originals
// ([[step-6.6-moving-games-and-media-between-disks]], [[disks]]): the copy with its progress (it
// can wait while the player is asked "Stop?", and stop: the partial copy is removed), the check of
// the copy (every file's size and SHA-256 against the original), and only then the original
// removed, never before ([[user-data]]). A copy that fails, or does not pass its check, is removed
// and the original is never touched; with several games the others go on. The copy, the check and
// the removal each tell their own percentage; with several games the player can stop only the
// current game's copy (its partial copy is removed, the next game goes on) or all of them
// (approved demo 6.6d).
package moves

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// Internal is the console's own disk as a destination: the games folder of the home folder.
const Internal = "internal"

// GamesFolders are the names of a games folder ([[games-on-disk]]), in any case; a new one is
// "Games".
var GamesFolders = []string{"games", "giochi"}

// Game is a game's folder and its title (for the screens).
type Game struct {
	Dir   string `json:"dir"`
	Title string `json:"title"`
	// Remove: this game's original is removed once its copy is checked, whatever the request's
	// Remove says (a plugged-in disk's games, chosen game by game: approved demo 6.6c)
	Remove bool `json:"remove,omitempty"`
}

// Request: the games, where they go (Internal or a mounted disk's folder), and whether the
// originals are removed once their copies are checked (all of them; or each game's own Remove).
type Request struct {
	Games  []Game `json:"games"`
	To     string `json:"to"`
	Remove bool   `json:"remove"`
}

// Status is the body of GET /v1/moves.
type Status struct {
	// State: "" (nothing yet), copy, check, remove, done, cancelled
	State   string `json:"state"`
	Paused  bool   `json:"paused"` // the copy waits: the player is asked "Stop?"
	Index   int    `json:"index"`  // the game being moved, from 0
	Count   int    `json:"count"`
	Title   string `json:"title"`
	Percent int    `json:"percent"` // of the current game's current step (copy, check, remove)
	Remove  bool   `json:"remove"`  // the current game's original is removed after its check
	// Failed: the titles whose copy did not work (the originals untouched)
	Failed []string `json:"failed"`
	// Skipped: the titles whose copy the player stopped alone (the originals untouched)
	Skipped []string `json:"skipped"`
	// Moved: where each game that worked is now
	Moved []string `json:"moved"`
}

// Errors returned to the API.
var (
	ErrInvalid = errors.New("not a game's folder or a disk the console can use")
	ErrNoSpace = errors.New("not enough space on that disk")
	ErrBusy    = errors.New("games are already being moved")
	ErrIdle    = errors.New("no copy is running")
)

var (
	errStopped = errors.New("stopped")
	errSkipped = errors.New("this game's copy stopped")
)

// Manager runs one move at a time.
type Manager struct {
	Broker *events.Broker
	Log    *slog.Logger
	Home   string // the console user's home (its games folder is the console's)
	Media  string // where the disks are mounted: /run/media/owneet
	// Free tells the free bytes at a path (tests replace it)
	Free func(path string) (int64, error)

	mu       sync.Mutex
	st       Status
	running  bool
	paused   bool
	resume   chan struct{}
	stop     chan struct{}
	skip     chan struct{} // the current game's copy only; a new one for each game
	lastSent time.Time
}

func free(path string) (int64, error) {
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return 0, err
	}
	return int64(s.Bavail) * int64(s.Bsize), nil
}

// Status tells how the move goes.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.st
	st.Failed = append([]string{}, m.st.Failed...)
	st.Skipped = append([]string{}, m.st.Skipped...)
	st.Moved = append([]string{}, m.st.Moved...)
	return st
}

func within(path, dir string) bool {
	return strings.HasPrefix(path, filepath.Clean(dir)+string(filepath.Separator))
}

func isGamesFolder(name string) bool {
	for _, g := range GamesFolders {
		if strings.EqualFold(name, g) {
			return true
		}
	}
	return false
}

// gameDir checks that dir is a game's folder in a games folder, on a mounted disk or in the home.
func (m *Manager) gameDir(dir string) (string, error) {
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) || !(within(dir, m.Media) || within(dir, m.Home)) {
		return "", ErrInvalid
	}
	parent := filepath.Dir(dir)
	if !isGamesFolder(filepath.Base(parent)) || strings.HasPrefix(filepath.Base(dir), ".") {
		return "", ErrInvalid
	}
	if st, err := os.Lstat(dir); err != nil || !st.IsDir() {
		return "", ErrInvalid
	}
	return dir, nil
}

// gamesFolder is the games folder of a destination: the one it has (any case), or a new "Games".
func (m *Manager) gamesFolder(to string) (string, error) {
	root := m.Home
	if to != Internal {
		root = filepath.Clean(to)
		if filepath.Dir(root) != filepath.Clean(m.Media) {
			return "", ErrInvalid
		}
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return "", ErrInvalid
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.IsDir() && isGamesFolder(e.Name()) {
			return filepath.Join(root, e.Name()), nil
		}
	}
	return filepath.Join(root, "Games"), nil
}

func size(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if info, e := d.Info(); e == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}

// Start moves (or copies) the games; "moves.changed" follows it.
func (m *Manager) Start(req Request) error {
	if len(req.Games) == 0 {
		return ErrInvalid
	}
	dest, err := m.gamesFolder(req.To)
	if err != nil {
		return err
	}
	var need int64
	for i, g := range req.Games {
		dir, err := m.gameDir(g.Dir)
		if err != nil || within(dest, dir) || filepath.Dir(dir) == dest {
			return ErrInvalid
		}
		req.Games[i].Dir = dir
		need += size(dir)
	}
	freeAt := m.Free
	if freeAt == nil {
		freeAt = free
	}
	room, err := freeAt(filepath.Dir(dest))
	if err != nil {
		return ErrInvalid
	}
	if room < need {
		return ErrNoSpace
	}
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return ErrBusy
	}
	m.running, m.paused = true, false
	m.resume, m.stop, m.skip = make(chan struct{}), make(chan struct{}), make(chan struct{})
	m.st = Status{State: "copy", Count: len(req.Games), Title: req.Games[0].Title, Remove: req.Remove || req.Games[0].Remove,
		Failed: []string{}, Skipped: []string{}, Moved: []string{}}
	m.mu.Unlock()
	m.Log.Info("moving games", "count", len(req.Games), "to", dest, "remove", req.Remove)
	m.publish(true)
	go m.run(req, dest)
	return nil
}

// Pause makes the copy wait (the player is asked "Stop?"); Resume goes on; Stop ends it: the
// partial copy is removed and the game stays where it was; Skip stops only the current game's
// copy (its partial copy removed) and the next game goes on. Only during the copy.
func (m *Manager) Pause() error  { return m.control(func() { m.paused = true; m.st.Paused = true }) }
func (m *Manager) Resume() error { return m.control(m.unpause) }
func (m *Manager) Stop() error   { return m.control(func() { close(m.stop); m.unpause() }) }
func (m *Manager) Skip() error {
	var err error
	cerr := m.control(func() {
		select {
		case <-m.skip:
			err = ErrIdle // already asked for this game
		default:
			close(m.skip)
			m.unpause()
		}
	})
	if cerr != nil {
		return cerr
	}
	return err
}

func (m *Manager) unpause() {
	if m.paused {
		m.paused = false
		m.st.Paused = false
		close(m.resume)
		m.resume = make(chan struct{})
	}
}

func (m *Manager) control(f func()) error {
	m.mu.Lock()
	if !m.running || m.st.State != "copy" {
		m.mu.Unlock()
		return ErrIdle
	}
	select {
	case <-m.stop:
		m.mu.Unlock()
		return ErrIdle
	default:
	}
	f()
	m.mu.Unlock()
	m.publish(true)
	return nil
}

// wait blocks while paused; errStopped when the player stopped the copy, errSkipped when only
// this game's.
func (m *Manager) wait() error {
	for {
		m.mu.Lock()
		paused, resume, stop, skip := m.paused, m.resume, m.stop, m.skip
		m.mu.Unlock()
		select {
		case <-stop:
			return errStopped
		case <-skip:
			return errSkipped
		default:
		}
		if !paused {
			return nil
		}
		select {
		case <-resume:
		case <-stop:
			return errStopped
		case <-skip:
			return errSkipped
		}
	}
}

func (m *Manager) set(f func(*Status), force bool) {
	m.mu.Lock()
	f(&m.st)
	m.mu.Unlock()
	m.publish(force)
}

// publish sends "moves.changed": at most a few times a second for the copy's progress.
func (m *Manager) publish(force bool) {
	m.mu.Lock()
	if !force && time.Since(m.lastSent) < 250*time.Millisecond {
		m.mu.Unlock()
		return
	}
	m.lastSent = time.Now()
	m.mu.Unlock()
	m.Broker.Publish("moves.changed", m.Status())
}

func (m *Manager) run(req Request, dest string) {
	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
		m.publish(true)
	}()
	for i, g := range req.Games {
		remove := req.Remove || g.Remove
		m.mu.Lock()
		select {
		case <-m.skip: // the previous game's
			m.skip = make(chan struct{})
		default:
		}
		m.mu.Unlock()
		m.set(func(s *Status) { s.State, s.Index, s.Title, s.Percent, s.Remove = "copy", i, g.Title, 0, remove }, true)
		target := freeName(filepath.Join(dest, filepath.Base(g.Dir)))
		err := m.copyTree(g.Dir, target)
		if errors.Is(err, errStopped) {
			os.RemoveAll(target)
			m.Log.Info("move stopped by the player: the partial copy removed", "game", g.Title)
			m.set(func(s *Status) { s.State = "cancelled" }, true)
			return
		}
		if errors.Is(err, errSkipped) {
			os.RemoveAll(target)
			m.Log.Info("this game's copy stopped by the player: the partial copy removed, the next game goes on", "game", g.Title)
			m.set(func(s *Status) { s.Skipped = append(s.Skipped, g.Title) }, true)
			continue
		}
		if err == nil {
			m.set(func(s *Status) { s.State, s.Percent = "check", 0 }, true)
			err = same(g.Dir, target, m.percent(2*size(g.Dir)))
		}
		if err != nil {
			// the copy did not work: it goes, the original is never touched
			os.RemoveAll(target)
			m.Log.Warn("copy did not work: removed, the original untouched", "game", g.Title, "err", err)
			m.set(func(s *Status) { s.Failed = append(s.Failed, g.Title) }, true)
			continue
		}
		if remove {
			m.set(func(s *Status) { s.State, s.Percent = "remove", 0 }, true)
			if err := removeTree(g.Dir, m.percent(entries(g.Dir))); err != nil {
				m.Log.Warn("the original could not be removed", "game", g.Title, "err", err)
			} else {
				m.carry(g.Title, g.Dir, target)
			}
		}
		m.set(func(s *Status) { s.Moved = append(s.Moved, target) }, true)
	}
	m.set(func(s *Status) { s.State = "done" }, true)
}

// percent returns a counter of the current step's progress: n more of total (bytes read, files
// removed) sets the status's percentage.
func (m *Manager) percent(total int64) func(n int64) {
	var done int64
	return func(n int64) {
		done += n
		if total > 0 {
			pct := int(min(100, done*100/total))
			m.set(func(s *Status) { s.Percent = pct }, false)
		}
	}
}

// entries counts what removeTree removes: every file, link and folder.
func entries(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, _ fs.DirEntry, err error) error {
		if err == nil {
			n++
		}
		return nil
	})
	return n
}

// removeTree removes a folder as os.RemoveAll does, telling each entry removed: the contents
// before their folder (the walk's order, backwards).
func removeTree(dir string, removed func(n int64)) error {
	var paths []string
	if err := filepath.WalkDir(dir, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		return err
	}
	for i := len(paths) - 1; i >= 0; i-- {
		if err := os.Remove(paths[i]); err != nil && !os.IsNotExist(err) {
			return err
		}
		removed(1)
	}
	return nil
}

// freeName: the folder's name, or "name (2)", "name (3)"… when the disk already has one.
func freeName(path string) string {
	if _, err := os.Lstat(path); err != nil {
		return path
	}
	for n := 2; ; n++ {
		p := fmt.Sprintf("%s (%d)", path, n)
		if _, err := os.Lstat(p); err != nil {
			return p
		}
	}
}

// copyTree copies a folder with its files' permissions and links, telling the progress.
func (m *Manager) copyTree(src, dst string) error {
	total := size(src)
	var done int64
	buf := make([]byte, 1<<20)
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := m.wait(); err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		out := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(out, info.Mode().Perm()|0o700)
		case d.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, out)
		case !d.Type().IsRegular():
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		o, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm()|0o600)
		if err != nil {
			return err
		}
		for {
			if err := m.wait(); err != nil {
				o.Close()
				return err
			}
			n, rerr := in.Read(buf)
			if n > 0 {
				if _, werr := o.Write(buf[:n]); werr != nil {
					o.Close()
					return werr
				}
				done += int64(n)
				if total > 0 {
					pct := int(done * 100 / total)
					m.set(func(s *Status) { s.Percent = pct }, false)
				}
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				o.Close()
				return rerr
			}
		}
		if err := o.Sync(); err != nil {
			o.Close()
			return err
		}
		return o.Close()
	})
}

// same checks that the copy is the original: the same files and links, each file's size and
// SHA-256. read (may be nil) is told the bytes read, of both.
func same(src, dst string, read func(n int64)) error {
	count := 0
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		other := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			if st, err := os.Lstat(other); err != nil || !st.IsDir() {
				return fmt.Errorf("%s: folder missing", rel)
			}
		case d.Type()&fs.ModeSymlink != 0:
			a, _ := os.Readlink(path)
			b, err := os.Readlink(other)
			if err != nil || a != b {
				return fmt.Errorf("%s: link differs", rel)
			}
		case d.Type().IsRegular():
			a, err := digest(path, read)
			if err != nil {
				return err
			}
			b, err := digest(other, read)
			if err != nil || a != b {
				return fmt.Errorf("%s: differs", rel)
			}
		default:
			return nil
		}
		count++
		return nil
	})
	if err != nil {
		return err
	}
	// nothing more in the copy than in the original
	copied := 0
	_ = filepath.WalkDir(dst, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && (d.IsDir() || d.Type()&fs.ModeSymlink != 0 || d.Type().IsRegular()) {
			copied++
		}
		return nil
	})
	if copied != count {
		return errors.New("the copy has other files")
	}
	return nil
}

func digest(path string, read func(n int64)) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	var r io.Reader = f
	if read != nil {
		r = &counting{r: f, read: read}
	}
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// counting tells the bytes read through it.
type counting struct {
	r    io.Reader
	read func(n int64)
}

func (c *counting) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.read(int64(n))
	}
	return n, err
}
