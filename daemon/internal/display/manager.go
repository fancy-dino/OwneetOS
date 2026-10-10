// SPDX-License-Identifier: GPL-3.0-or-later

package display

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// Errors of the display API.
var (
	ErrNotGamescope  = errors.New("screens and modes can change only in the main (gamescope) session")
	ErrUnknownScreen = errors.New("no such screen")
	ErrUnknownMode   = errors.New("the screen has no such mode")
	ErrAppsOpen      = errors.New("a game or app is open: the display restarts to change screen")
	ErrNotPending    = errors.New("no change is waiting for confirmation")
	ErrNoBrightness  = errors.New("the brightness of this screen cannot be changed")
)

// Status is what Settings → Display shows.
type Status struct {
	Available  bool     `json:"available"` // gamescope session: the screen and its mode can change
	Screens    []Screen `json:"screens"`
	Modes      []string `json:"modes"`                // of the screen in use ("WxH@Hz", preferred first); none for built-in screens
	Mode       string   `json:"mode"`                 // "auto" or one of Modes
	Brightness *int     `json:"brightness,omitempty"` // 0-100, only when the screen lets the console change it
	Pending    *Pending `json:"pending,omitempty"`
}

// Pending is a change waiting for "Keep this?": without an answer it goes back.
type Pending struct {
	Kind    string `json:"kind"`    // "screen" or "mode"
	Seconds int    `json:"seconds"` // until it goes back
}

// Helper turns off the screens of the graphics cards the console does not use
// (owneet-screens-off@CARD.service).
type Helper interface {
	Start(keepCard string) error
	Stop(keepCard string) error
}

// Manager is Settings → Display.
type Manager struct {
	Broker   *events.Broker
	Log      *slog.Logger
	Session  func() (apps.Session, error)
	AppsOpen func() bool // a game or app runs (it would be closed by a restart of the display)

	Backlight interface{ Set(*Backlight, int) error } // nil without logind
	DDC       *DDC                                    // nil without ddcutil
	Helper    Helper                                  // nil: other cards are left alone

	SysDRM       string // /sys/class/drm
	SysBacklight string // /sys/class/backlight
	PNPFile      string // /usr/share/hwdata/pnp.ids
	Modes        ModeFile
	Prefs        Prefs
	RestartFile  string        // tells owneet-session that a stop of gamescope was wanted
	ConfirmTime  time.Duration // 15 s (decision log 2026-10-10)

	mu         sync.Mutex
	pnps       map[string]string
	gs         *apps.Gamescope
	pending    *pending
	helperCard string
	lastScreen string
	lastHome   int
}

type pending struct {
	kind        string
	deadline    time.Time
	timer       *time.Timer
	undo        func()
	waitSession bool // screen: the countdown starts once the console is back on screen
}

// restartGrace bounds the wait for the console to come back on another screen.
const restartGrace = 30 * time.Second

func (m *Manager) init() {
	if m.pnps == nil {
		m.pnps = LoadPNPs(m.PNPFile)
	}
}

// gamescope returns the control of the current gamescope session, or nil.
func (m *Manager) gamescope() *apps.Gamescope {
	s, err := m.Session()
	if err != nil || s.Mode != "gamescope" || s.Display == "" {
		return nil
	}
	if m.gs == nil || m.gs.Display != s.Display {
		if m.gs != nil {
			m.gs.Close()
		}
		m.gs = &apps.Gamescope{Display: s.Display, Log: m.Log}
	}
	return m.gs
}

// screens lists the screens and marks the one the console uses. With gamescope running, it is
// the screen that is on among those of gamescope's graphics card (gamescope turns off the others
// there); a screen of another card may be "on" too, still showing the boot splash. Otherwise:
// the screen that is on, the chosen one, or the only one.
func (m *Manager) screens() []Screen {
	m.init()
	list := ListScreens(m.SysDRM, m.pnps)
	output := m.Prefs.Read()["OUTPUT"]
	card := gamescopeCard(m.SysDRM)
	pick := -1
	lit := 0
	for i, s := range list {
		if s.lit && (card == "" || s.card == card) {
			lit++
			if pick < 0 || s.Connector == output {
				pick = i
			}
		}
	}
	if lit == 0 {
		for i, s := range list {
			if s.Connector == output {
				pick = i
			}
		}
	}
	if pick < 0 && len(list) > 0 {
		pick = 0
	}
	if pick >= 0 {
		list[pick].Active = true
	}
	return list
}

func active(list []Screen) *Screen {
	for i := range list {
		if list[i].Active {
			return &list[i]
		}
	}
	return nil
}

// Status returns the display settings.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.statusLocked()
}

func (m *Manager) statusLocked() Status {
	st := Status{Screens: m.screens(), Modes: []string{}, Mode: "auto"}
	cur := active(st.Screens)
	if gs := m.gamescope(); gs != nil {
		st.Available = true
		if cur != nil && !cur.Internal {
			if list, err := gs.RootString("GAMESCOPE_DISPLAY_MODE_LIST_EXTERNAL"); err == nil {
				st.Modes = uniqueModes(list)
			}
		}
	}
	if cur != nil {
		if saved := m.Modes.Get(cur.description); saved != "" && contains(st.Modes, saved) {
			st.Mode = saved
		}
		if v, ok := m.brightnessOf(cur); ok {
			st.Brightness = &v
		}
	}
	if p := m.pending; p != nil {
		secs := int(time.Until(p.deadline).Round(time.Second) / time.Second)
		st.Pending = &Pending{Kind: p.kind, Seconds: max(0, secs)}
	}
	return st
}

func uniqueModes(list string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, mode := range strings.Fields(list) {
		if ValidMode(mode) && !seen[mode] {
			seen[mode] = true
			out = append(out, mode)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (m *Manager) brightnessOf(s *Screen) (int, bool) {
	if s.Internal {
		if b := FindBacklight(m.SysBacklight); b != nil && m.Backlight != nil {
			return b.Percent(), true
		}
		return 0, false
	}
	if m.DDC != nil {
		return m.DDC.Percent(s.ID)
	}
	return 0, false
}

func (m *Manager) publishLocked() {
	m.Broker.Publish("display.changed", m.statusLocked())
}

// SetMode applies a mode ("auto": the screen's preferred one) to the screen in use; it goes back
// unless confirmed.
func (m *Manager) SetMode(mode string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	gs := m.gamescope()
	if gs == nil {
		return ErrNotGamescope
	}
	st := m.statusLocked()
	cur := active(st.Screens)
	if cur == nil || len(st.Modes) == 0 || (mode != "auto" && !contains(st.Modes, mode)) {
		return ErrUnknownMode
	}
	if mode == st.Mode {
		return nil
	}
	desc := cur.description
	prev := m.Modes.Get(desc)
	if err := m.applyMode(gs, desc, strings.TrimPrefix(mode, "auto")); err != nil {
		return err
	}
	m.startPendingLocked("mode", m.ConfirmTime, false, func() {
		if err := m.applyMode(m.gamescope(), desc, prev); err != nil {
			m.Log.Warn("cannot restore the previous mode", "err", err)
		}
	})
	m.Log.Info("screen mode changed", "screen", desc, "mode", mode)
	return nil
}

func (m *Manager) applyMode(gs *apps.Gamescope, desc, mode string) error {
	if err := m.Modes.Set(desc, mode); err != nil {
		return err
	}
	if gs == nil {
		return nil
	}
	return gs.SetRootCardinal("GAMESCOPE_DISPLAY_MODE_NUDGE", 1)
}

// SetScreen moves the console to another screen: gamescope restarts there (a few seconds) and the
// others are turned off. It goes back unless confirmed. Refused while a game or app is open.
func (m *Manager) SetScreen(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.gamescope() == nil {
		return ErrNotGamescope
	}
	list := m.screens()
	var target *Screen
	for i := range list {
		if list[i].ID == id {
			target = &list[i]
		}
	}
	if target == nil {
		return ErrUnknownScreen
	}
	if target.Active {
		return nil
	}
	if m.AppsOpen != nil && m.AppsOpen() {
		return ErrAppsOpen
	}
	prev := m.Prefs.Read()
	before := map[string]string{"OUTPUT": prev["OUTPUT"], "VK_DEVICE": prev["VK_DEVICE"]}
	// owneet-session goes back to it by itself if gamescope does not start on the new screen
	if err := m.previous().Write(prev); err != nil {
		return err
	}
	if err := m.Prefs.Update(map[string]string{"OUTPUT": target.Connector, "VK_DEVICE": target.gpu}); err != nil {
		return err
	}
	if err := m.restartLocked("screen"); err != nil {
		m.Prefs.Update(before)
		os.Remove(m.previous().Path)
		return err
	}
	m.Log.Info("moving the console to another screen", "screen", target.ID)
	m.startPendingLocked("screen", m.ConfirmTime+restartGrace, true, func() {
		if err := m.Prefs.Update(before); err != nil {
			m.Log.Warn("cannot restore the previous screen", "err", err)
		}
		os.Remove(m.previous().Path)
		// not running: owneet-session has already gone back (or is about to)
		if err := m.restartLocked("revert"); err != nil {
			m.Log.Info("display not restarted", "err", err)
		}
	})
	return nil
}

// previous holds display.conf as it was before a move to another screen, until it is kept.
func (m *Manager) previous() Prefs { return Prefs{Path: m.Prefs.Path + ".previous"} }

// restartLocked stops gamescope; owneet-session starts it again with display.conf. The reason
// ("screen", "revert") tells owneet-session that the stop was wanted.
func (m *Manager) restartLocked(reason string) error {
	pids := processes("/usr/bin/gamescope")
	if len(pids) == 0 {
		return errors.New("gamescope is not running")
	}
	m.stopHelperLocked()
	if err := os.WriteFile(m.RestartFile, []byte(reason+"\n"), 0o644); err != nil {
		return err
	}
	for _, pid := range pids {
		syscall.Kill(pid, syscall.SIGTERM)
	}
	// a gamescope stuck on a screen that does not answer is ended for good
	go func() {
		time.Sleep(5 * time.Second)
		for _, pid := range processes("/usr/bin/gamescope") {
			for _, old := range pids {
				if pid == old {
					m.Log.Warn("gamescope did not stop: killing it", "pid", pid)
					syscall.Kill(pid, syscall.SIGKILL)
				}
			}
		}
	}()
	return nil
}

// gamescopeCard returns the graphics card gamescope drives ("card1"), from the devices it holds
// open (logind hands it the card); "" when gamescope does not run or drives no screen (headless).
func gamescopeCard(sysDRM string) string {
	found := ""
	for _, pid := range processes("/usr/bin/gamescope") {
		fds, _ := os.ReadDir(filepath.Join("/proc", strconv.Itoa(pid), "fd"))
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "fd", fd.Name()))
			if err != nil || !strings.HasPrefix(target, "/dev/dri/card") {
				continue
			}
			card := filepath.Base(target)
			if found == "" || hasLitScreen(sysDRM, card) {
				found = card
			}
		}
	}
	return found
}

func hasLitScreen(sysDRM, card string) bool {
	matches, _ := filepath.Glob(filepath.Join(sysDRM, card+"-*", "enabled"))
	for _, f := range matches {
		if readTrim(f) == "enabled" {
			return true
		}
	}
	return false
}

// processes returns this user's processes running that program (gamescope renames its main
// thread "gamescope-wl": the name cannot be used).
func processes(program string) []int {
	entries, _ := os.ReadDir("/proc")
	uid := uint32(os.Getuid())
	var out []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		info, err := os.Stat(filepath.Join("/proc", e.Name()))
		if err != nil {
			continue
		}
		if st, ok := info.Sys().(*syscall.Stat_t); !ok || st.Uid != uid {
			continue
		}
		if exe, err := os.Readlink(filepath.Join("/proc", e.Name(), "exe")); err == nil && exe == program {
			out = append(out, pid)
		}
	}
	return out
}

func (m *Manager) startPendingLocked(kind string, wait time.Duration, waitSession bool, undo func()) {
	if old := m.pending; old != nil {
		old.timer.Stop()
		if old.kind == kind {
			undo = old.undo // a second change before answering: going back means to the first state
		}
	}
	p := &pending{kind: kind, deadline: time.Now().Add(wait), undo: undo, waitSession: waitSession}
	p.timer = time.AfterFunc(wait, func() { m.expire(p) })
	m.pending = p
	m.Broker.Publish("display.pending", Pending{Kind: kind, Seconds: int(wait / time.Second)})
	m.publishLocked()
}

func (m *Manager) expire(p *pending) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pending != p {
		return
	}
	m.pending = nil
	m.Log.Info("change not confirmed: going back", "kind", p.kind)
	p.undo()
	m.Broker.Publish("display.reverted", map[string]string{"kind": p.kind})
	m.publishLocked()
}

// Confirm keeps the change waiting for confirmation.
func (m *Manager) Confirm() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pending == nil {
		return ErrNotPending
	}
	m.pending.timer.Stop()
	os.Remove(m.previous().Path)
	m.Broker.Publish("display.confirmed", map[string]string{"kind": m.pending.kind})
	m.pending = nil
	m.publishLocked()
	return nil
}

// Revert goes back at once.
func (m *Manager) Revert() error {
	m.mu.Lock()
	p := m.pending
	m.mu.Unlock()
	if p == nil {
		return ErrNotPending
	}
	p.timer.Stop()
	m.expire(p)
	return nil
}

// SetBrightness sets the brightness of the screen in use, 0-100.
func (m *Manager) SetBrightness(percent int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur := active(m.screens())
	if cur == nil {
		return ErrNoBrightness
	}
	percent = max(0, min(100, percent))
	if cur.Internal {
		b := FindBacklight(m.SysBacklight)
		if b == nil || m.Backlight == nil {
			return ErrNoBrightness
		}
		return m.Backlight.Set(b, percent)
	}
	if m.DDC == nil {
		return ErrNoBrightness
	}
	return m.DDC.Set(cur.ID, percent)
}

var layoutName = regexp.MustCompile(`^[a-z]{2,3}(:[a-z0-9_]+)?$`)

// SetKeyboardLayout makes a physical keyboard type with an XKB layout ("it", "us"): at once in
// gamescope, at the next start of the compositor in the reduced (cage) session.
func (m *Manager) SetKeyboardLayout(layout string) error {
	if !layoutName.MatchString(layout) {
		return errors.New("invalid layout")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.Prefs.Update(map[string]string{"KEYBOARD": layout}); err != nil {
		return err
	}
	if gs := m.gamescope(); gs != nil {
		return gs.SetRootString("GAMESCOPE_KEYBOARD_LAYOUT", layout)
	}
	return nil
}

// Run follows the session and the screens: it starts the countdown once the console is back after
// a move to another screen, keeps the other cards' screens off, and reports changes (hot-plug).
func (m *Manager) Run(ctx context.Context) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			m.mu.Lock()
			m.stopHelperLocked()
			m.mu.Unlock()
			return
		case <-tick.C:
			m.check()
		}
	}
}

func (m *Manager) publish() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishLocked()
}

func (m *Manager) check() {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, err := m.Session()
	home := 0
	if err == nil {
		home = sess.HomePID
	}
	list := m.screens()
	var sig []string
	for _, s := range list {
		sig = append(sig, s.ID+"="+strconv.FormatBool(s.lit))
	}
	screens := strings.Join(sig, ",")
	newSession := home != 0 && home != m.lastHome
	changed := screens != m.lastScreen
	m.lastHome, m.lastScreen = home, screens

	if newSession {
		cur := active(list)
		m.Log.Info("console session on screen", "mode", sess.Mode, "card", gamescopeCard(m.SysDRM),
			"screen", func() string {
				if cur == nil {
					return ""
				}
				return cur.ID
			}())
		if p := m.pending; p != nil && p.waitSession {
			// The console is on screen again: now the user can answer
			p.waitSession = false
			p.timer.Reset(m.ConfirmTime)
			p.deadline = time.Now().Add(m.ConfirmTime)
			m.Broker.Publish("display.pending", Pending{Kind: p.kind, Seconds: int(m.ConfirmTime / time.Second)})
		}
	}
	if newSession || changed {
		m.updateHelperLocked(err == nil && sess.Mode == "gamescope", list)
	}
	// ddcutil opens the graphics cards: only once the compositor holds its own, never during its
	// start (a card opened by someone else first cannot be taken by the compositor)
	if m.DDC != nil && home != 0 && (newSession || changed) {
		m.DDC.Find(m.publish)
	}
	if changed {
		m.publishLocked()
	}
}

// updateHelperLocked keeps the screens of the other graphics cards off while gamescope drives
// one (cage drives every screen itself).
func (m *Manager) updateHelperLocked(gamescope bool, list []Screen) {
	if m.Helper == nil {
		return
	}
	want := ""
	if card := gamescopeCard(m.SysDRM); gamescope && card != "" && cards(m.SysDRM) > 1 {
		want = card
	}
	if want == m.helperCard {
		return
	}
	m.stopHelperLocked()
	if want != "" {
		if err := m.Helper.Start(want); err != nil {
			m.Log.Warn("cannot turn off the other screens", "err", err)
			return
		}
		m.Log.Info("turning off the screens of the other graphics cards", "console_card", want)
		m.helperCard = want
	}
}

func (m *Manager) stopHelperLocked() {
	if m.Helper != nil && m.helperCard != "" {
		if err := m.Helper.Stop(m.helperCard); err != nil {
			m.Log.Warn("cannot stop owneet-screens-off", "err", err)
		}
	}
	m.helperCard = ""
}

var cardDir = regexp.MustCompile(`^card[0-9]+$`)

func cards(sysDRM string) int {
	entries, _ := os.ReadDir(sysDRM)
	n := 0
	for _, e := range entries {
		if cardDir.MatchString(e.Name()) {
			n++
		}
	}
	return n
}
