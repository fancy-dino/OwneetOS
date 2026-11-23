// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"strings"
)

// A Steam game started through its publisher's launcher: the launcher stays after the game, so Steam
// (and the console, which shows what Steam says) keeps the game running, "Resume" offered for a game
// already quit (South Park: The Fractured But Whole with Ubisoft Connect, FrankenPC 2026-11-22,
// [[open-items]] 21). Known on SteamOS and Heroic too; Proton's own fix, proposed and not merged yet
// (ValveSoftware/Proton pull request 10356), ends the launcher once the game has ended and the
// launcher shows no window (a window may be a question about cloud saves). owneetd does the same:
// when every program of the game that is left is the launcher's, the game's own programs having run
// and shown a window, and nothing of it is on screen, the game is closed as from the console.

// publisherLauncher: the programs of the publishers' launchers that stay after the game, by file
// name (lower case). Ubisoft Connect only, as seen; others are added when a test shows them.
var publisherLauncher = map[string]bool{
	"upc.exe":                true,
	"uplayservice.exe":       true,
	"uplaywebcore.exe":       true,
	"uplaycrashreporter.exe": true,
}

// programName: a program's file name, lower case ("upc.exe")
func programName(argv []string) string {
	path := strings.ToLower(strings.ReplaceAll(argv[0], "\\", "/"))
	return path[strings.LastIndex(path, "/")+1:]
}

// launcherLeftOver closes a Steam game whose own programs have ended while only its publisher's
// launcher stays (see above). Called on each of owneetd's checks.
func (m *Manager) launcherLeftOver(a *App, visible map[uint32]bool) {
	own, launcher := false, false
	for _, pid := range m.Procs.InUnit(a.unit) {
		if !m.descends(pid, a.reaper) {
			continue
		}
		argv := m.Procs.Cmdline(pid)
		switch {
		case !windowsProgram(argv) || xaliaProgram(argv): // Proton's, Steam's and Wine's own (closing.go)
		case publisherLauncher[programName(argv)]:
			launcher = true
		default:
			own = true
		}
	}
	shown := visible[a.appID]
	m.mu.Lock()
	if own && shown {
		a.ownWindowSeen = true // the game itself ran, with a window (not a launcher before its game)
	}
	leftOver := !own && launcher && a.ownWindowSeen && !shown && !a.launcherClosed
	if leftOver {
		a.launcherClosed = true
	}
	m.mu.Unlock()
	if !leftOver {
		return
	}
	// testers: owneet.debug.launcher_close=off leaves the launcher alone, to see what it does after
	// the game (its cloud sync, open item 21)
	if DebugOption("launcher_close") == "off" {
		m.Log.Info("the game has ended, its publisher's launcher stays: left alone (testers' switch)", "id", a.ID)
		return
	}
	m.Log.Info("the game has ended, its publisher's launcher stays: closed", "id", a.ID)
	m.closeSteamGame(a, false)
}

// leftOverLaunchers checks the running Steam games (found: their Steam launcher process by app id).
func (m *Manager) leftOverLaunchers(found map[string]int) {
	m.mu.Lock()
	var games []*App
	for _, a := range m.apps {
		if a.reaper != 0 && found[strings.TrimPrefix(a.ID, steamGamePrefix)] == a.reaper {
			games = append(games, a)
		}
	}
	m.mu.Unlock()
	if len(games) == 0 {
		return
	}
	visible := map[uint32]bool{}
	if sess, err := m.Session(); err == nil {
		if f := m.focus(sess); f != nil {
			ids, err := f.Visible()
			if err != nil {
				return // what is on screen is unknown: nothing is closed
			}
			for _, id := range ids {
				visible[id] = true
			}
		}
	}
	for _, a := range games {
		m.launcherLeftOver(a, visible)
	}
}
