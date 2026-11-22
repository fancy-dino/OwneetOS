// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"fmt"
	"sort"
	"syscall"
	"testing"
)

// A Ubisoft game: Ubisoft Connect starts first (no game yet: left alone), then the game with its
// window; once the game has ended and only Ubisoft Connect is left, with nothing on screen, the game
// is closed: its launcher's programs get SIGTERM (open item 21).
func TestLauncherLeftAfterTheGameIsClosed(t *testing.T) {
	m, units, focus, procs, _, ch := newSteamManager(t)
	if err := m.Steam(SteamRequest{Open: "library"}); err != nil {
		t.Fatal(err)
	}
	next(t, ch, "app.started")
	unit := units.started[0]
	focus.openWindow(1001)
	settle(m)
	launcher := `C:\Program Files (x86)\Ubisoft\Ubisoft Game Launcher\upc.exe`
	procs.add(4000, 1, unit, "reaper", "SteamLaunch", "AppId=488790", "--", "proton", "waitforexitandrun")
	procs.add(4001, 4000, unit, `C:\windows\system32\services.exe`)
	procs.add(4002, 4000, unit, launcher)
	procs.add(4003, 4002, unit, `C:\Program Files (x86)\Ubisoft\Ubisoft Game Launcher\UplayWebCore.exe`)
	m.scanSteam()
	next(t, ch, "app.started")
	// Ubisoft Connect alone before the game, its login window on screen: not touched
	focus.openWindow(4002)
	m.scanSteam()
	procs.mu.Lock()
	if len(procs.signals) != 0 {
		t.Fatalf("closed before the game: %v", procs.signals)
	}
	procs.mu.Unlock()
	focus.closeWindow(m.appIDOf("steam:488790"))
	m.scanSteam()
	// the game runs with its window
	procs.add(4010, 4002, unit, `Z:\run\media\games\South Park\SouthPark_TFBW.exe`)
	focus.openWindow(4010)
	m.scanSteam()
	// the game quits; Ubisoft Connect shows a window (a question about cloud saves): left alone
	procs.remove(4010)
	m.scanSteam()
	procs.mu.Lock()
	if len(procs.signals) != 0 {
		t.Fatalf("closed with a window on screen: %v", procs.signals)
	}
	procs.mu.Unlock()
	// its window goes: the launcher's programs are closed, Wine's own too (the whole game)
	focus.closeWindow(m.appIDOf("steam:488790"))
	m.scanSteam()
	procs.mu.Lock()
	got := append([]string(nil), procs.signals...)
	procs.mu.Unlock()
	sort.Strings(got)
	want := []string{fmt.Sprintf("4001:%d", syscall.SIGTERM), fmt.Sprintf("4002:%d", syscall.SIGTERM), fmt.Sprintf("4003:%d", syscall.SIGTERM)}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("signals %v, want %v", got, want)
	}
	// once only
	m.scanSteam()
	procs.mu.Lock()
	if len(procs.signals) != 3 {
		t.Fatalf("closed again: %v", procs.signals)
	}
	procs.mu.Unlock()
}

// A game without a publisher's launcher is never closed by this, whatever is on screen.
func TestNoLauncherNothingClosed(t *testing.T) {
	m, units, focus, procs, _, ch := newSteamManager(t)
	if err := m.Steam(SteamRequest{Open: "library"}); err != nil {
		t.Fatal(err)
	}
	next(t, ch, "app.started")
	unit := units.started[0]
	focus.openWindow(1001)
	settle(m)
	procs.add(5000, 1, unit, "reaper", "SteamLaunch", "AppId=480", "--", "proton", "waitforexitandrun")
	procs.add(5001, 5000, unit, `Z:\games\Game\Game.exe`)
	m.scanSteam()
	next(t, ch, "app.started")
	focus.openWindow(5001)
	m.scanSteam()
	procs.add(5002, 5000, unit, `C:\windows\system32\explorer.exe`)
	procs.remove(5001)
	focus.closeWindow(m.appIDOf("steam:480"))
	m.scanSteam()
	procs.mu.Lock()
	defer procs.mu.Unlock()
	if len(procs.signals) != 0 {
		t.Fatalf("closed: %v", procs.signals)
	}
}
