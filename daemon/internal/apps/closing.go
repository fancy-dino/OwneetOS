// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"regexp"
	"strings"

	"golang.org/x/sys/unix"
)

// A console game quit from its own menu: the home screen comes back as soon as the game's own
// programs have ended, not when its whole unit has (owner, FrankenPC, 2026-11-09: The Witcher 2's
// last picture stayed about 2 s while Wine and its container closed after the game, and the "back
// home" sound came late). The game's programs are the processes that showed its windows and, for
// a Windows game, every Windows program of its unit outside C:\windows (Wine and Proton's own
// programs live there; the test VM, 2026-11-09: "X:\Giochi\Exe Test\game.exe" next to
// "C:\windows\system32\services.exe"). The end of each is an event (a pidfd), and while one is
// alive (a launcher that started the game, the game) nothing happens.

// windowsProgram: a Windows program of the game itself (Wine shows a Windows process's own path
// as its command line).
var windowsPath = regexp.MustCompile(`^[A-Za-z]:\\`)

func windowsProgram(argv []string) bool {
	if len(argv) == 0 || !windowsPath.MatchString(argv[0]) {
		return false
	}
	return !wineProgram(argv)
}

// wineProgram: one of Wine's own programs (in C:\windows), never the game's, even when it shows a
// window: explorer.exe owns one, and could outlive the game until Wine closes (open item 3b,
// owner 2026-11-13).
func wineProgram(argv []string) bool {
	return len(argv) > 0 && strings.HasPrefix(strings.ToLower(argv[0]), `c:\windows\`)
}

// closesWithPrograms: the apps whose programs are followed (the ones that go alone in front).
func closesWithPrograms(a *App) bool {
	return a.Kind == KindGame && !strings.HasPrefix(a.ID, steamGamePrefix)
}

// pidfdWatch calls ended once the process has ended; false when it cannot be followed (gone
// already).
func pidfdWatch(pid int, ended func()) bool {
	fd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return false
	}
	go func() {
		defer unix.Close(fd)
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		for {
			if _, err := unix.Poll(fds, -1); err != unix.EINTR {
				break
			}
		}
		ended()
	}()
	return true
}

// watchPrograms follows the game's programs not followed yet: the processes given (which showed a
// window of it) and the Windows programs of its unit.
func (m *Manager) watchPrograms(id string, pids ...int) {
	m.mu.Lock()
	a, ok := m.apps[id]
	if !ok || !closesWithPrograms(a) {
		m.mu.Unlock()
		return
	}
	unit, procs, watch := a.unit, m.Procs, m.watchExit
	m.mu.Unlock()
	if watch == nil {
		watch = pidfdWatch
	}
	if procs != nil {
		given := pids
		pids = nil
		for _, pid := range given { // a window of Wine's own does not make its program the game's
			if !wineProgram(procs.Cmdline(pid)) {
				pids = append(pids, pid)
			}
		}
		for _, pid := range procs.InUnit(unit) {
			if windowsProgram(procs.Cmdline(pid)) {
				pids = append(pids, pid)
			}
		}
	}
	for _, pid := range pids {
		m.mu.Lock()
		a, ok := m.apps[id]
		known := ok && a.programs[pid]
		if ok && !known {
			if a.programs == nil {
				a.programs = map[int]bool{}
			}
			a.programs[pid] = true
		}
		m.mu.Unlock()
		if !ok || known {
			continue
		}
		if !watch(pid, func() { m.programEnded(id, pid) }) {
			go m.programEnded(id, pid) // ended already
			continue
		}
		program := ""
		if procs != nil {
			if argv := procs.Cmdline(pid); len(argv) > 0 {
				program = argv[0]
			}
		}
		m.Log.Info("following a program of the game", "id", id, "pid", pid, "program", program)
	}
}

// programEnded: one of the game's programs ended. With none left, no window of it on screen, and
// the game alone in front, the home screen comes back at once.
func (m *Manager) programEnded(id string, pid int) {
	m.mu.Lock()
	if a, ok := m.apps[id]; ok {
		delete(a.programs, pid)
	}
	m.mu.Unlock()
	m.Log.Info("a program of the game ended", "id", id, "pid", pid)
	m.watchPrograms(id) // the ones started meanwhile (a launcher starts the game, then ends)
	m.mu.Lock()
	a, ok := m.apps[id]
	if !ok || len(a.programs) > 0 {
		m.mu.Unlock()
		return
	}
	appID := a.appID
	front := len(m.shown) == 1 && m.shown[0] == appID
	handover, unit, procs := a.handover, a.unit, m.Procs
	m.mu.Unlock()
	if !front {
		return
	}
	// a launcher that hands over: the game it started has no window yet; the app is not over
	// while its unit still has a process
	if handover && procs != nil && len(procs.InUnit(unit)) > 0 {
		return
	}
	sess, err := m.Session()
	if err != nil {
		return
	}
	f := m.focus(sess)
	if f == nil {
		return
	}
	if focused, err := f.Focused(); err != nil || focused == appID {
		return // a window of it is still on screen (e.g. a message of Wine's)
	}
	if err := m.show(f, []uint32{HomeAppID}); err != nil {
		m.Log.Warn("cannot show the home screen", "err", err)
		return
	}
	m.Log.Info("the game's programs ended: home screen", "id", id)
	m.focusChanged("home")
}
