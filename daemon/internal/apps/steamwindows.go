// SPDX-License-Identifier: GPL-3.0-or-later

package apps

// A Steam game whose Linux version cannot start ([[step-4.5-windows-games-proton]], owner
// 2026-10-13): Steam says "Missing game executable" (Dragon's Wake, 399070: its Linux files lack
// the program its launch settings name). Steam writes it in its console log, an undocumented file
// (read on the FrankenPC, 2026-10-18):
//
//	[2026-10-18 14:24:27] GameAction [AppID 399070, ActionID 1] : LaunchApp failed with AppError_28 with "…/Dragon.x86"
//
// owneetd then sends "steam.launch_failed" and keeps Steam's error hidden: the console asks
// whether to try the version for Windows. Trying it (SteamRequest.Windows) sets Valve's Proton
// for that game (owner, 2026-10-18, [[steam-integration]]), as Steam's own "Force the use of a
// specific compatibility tool" does: in config/config.vdf, which Steam reads when it starts and
// writes when it exits, so Steam is closed first. Then the game starts again; Steam downloads that
// Proton itself the first time.

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// steamLaunch is the game Steam was last asked to start, how far its console log was read and
// what the log says about the launch.
type steamLaunch struct {
	appid  string
	at     time.Time
	offset int64
	state  launchState
	told   string // the task last logged while Steam stays hidden
}

// launchState is what Steam's console log says about a game's launch: Steam goes through tasks
// ("LaunchApp changed task to DownloadingDepots", "CreatingProcess", "WaitingGameWindow"…) and
// sometimes waits for the user's answer ("LaunchApp waiting for user response to …").
type launchState struct {
	failed  bool      // failed for a missing program
	task    string    // the last task, "" before the first
	waiting bool      // Steam waits for the user's answer to it
	at      time.Time // when that line was written
}

// busy: Steam works on the launch (downloading what the game needs, e.g. its version for
// Windows and Proton the first time: 45 s for Dragon's Wake on the FrankenPC, 2026-10-18), so its
// window stays hidden; a question Steam has waited on for steamAnswerWait is the user's.
func (s launchState) busy(now time.Time) bool {
	switch s.task {
	case "", "Failed", "Completed":
		return false
	}
	return !s.waiting || now.Sub(s.at) < steamAnswerWait
}

// steamAnswerWait: Steam answers its own routine questions at once (ShowInterstitials,
// CreatingProcess: within a second); one still open after this is for the user.
const steamAnswerWait = 3 * time.Second

// steamLaunchWatch: how long after asking for a game its launch is followed while Steam is not
// working on it.
const steamLaunchWatch = 2 * time.Minute

// steamCloseWait: how long Steam may take to close before the game's settings are written.
var steamCloseWait = time.Minute

var (
	launchLine = regexp.MustCompile(`^\[(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d)\] GameAction \[AppID (\d+), ActionID \d+\] : LaunchApp (.*)$`)
	launchTask = regexp.MustCompile(`^(?:changed task to|waiting for user response to) (\w+)`)
	protonTool = regexp.MustCompile(`Registering tool (proton_\d+), AppID (\d+)`)
)

func steamConsoleLog(dir string) string { return filepath.Join(dir, "logs", "console_log.txt") }

// steamLogSize is the console log's size now (0 when there is none).
func steamLogSize(dir string) int64 {
	if st, err := os.Stat(steamConsoleLog(dir)); err == nil {
		return st.Size()
	}
	return 0
}

// steamLaunchRead reads Steam's console log from offset and follows appid's launch from state:
// only lines written at or after since count (the log's times are local, to the second). It
// returns the new state and how far it read (whole lines only). Steam starts a new log when it
// starts: a shorter file is read from its start.
func steamLaunchRead(path string, offset int64, appid string, since time.Time, state launchState) (launchState, int64) {
	f, err := os.Open(path)
	if err != nil {
		return state, 0
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return state, offset
	}
	if st.Size() < offset {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return state, offset
	}
	data, err := io.ReadAll(io.LimitReader(f, 8<<20))
	if err != nil {
		return state, offset
	}
	end := bytes.LastIndexByte(data, '\n') + 1
	for _, line := range strings.Split(string(data[:end]), "\n") {
		match := launchLine.FindStringSubmatch(line)
		if match == nil || match[2] != appid {
			continue
		}
		t, err := time.ParseInLocation("2006-01-02 15:04:05", match[1], time.Local)
		if err != nil || t.Before(since.Truncate(time.Second)) {
			continue
		}
		what := match[3]
		switch {
		case strings.HasPrefix(what, "failed with"):
			state.task, state.waiting = "Failed", false
			state.failed = state.failed || strings.HasPrefix(what, "failed with AppError_28")
		case strings.HasPrefix(what, "continues with user response"):
			state.waiting = false
		default:
			task := launchTask.FindStringSubmatch(what)
			if task == nil {
				continue
			}
			state.task, state.waiting = task[1], strings.HasPrefix(what, "waiting")
		}
		state.at = t
	}
	return state, offset + int64(end)
}

// steamProtonTool is Valve's newest Proton that Steam registered (its compatibility log, e.g.
// "Registering tool proton_11, AppID 4628710"), else Proton Experimental, which Steam always offers.
// Newest by app id: Valve's ids grow with each version, while the names drop the dot (proton_513
// is 5.13, older than proton_11: the FrankenPC got it on 2026-10-18).
func steamProtonTool(dir string) string {
	data, _ := os.ReadFile(filepath.Join(dir, "logs", "compat_log.txt"))
	best, newest := "proton_experimental", int64(0)
	for _, match := range protonTool.FindAllStringSubmatch(string(data), -1) {
		if id, err := strconv.ParseInt(match[2], 10, 64); err == nil && id > newest {
			best, newest = match[1], id
		}
	}
	return best
}

// steamConfigPath is where Steam keeps its compatibility tools per game.
var steamConfigPath = []string{"InstallConfigStore", "Software", "Valve", "Steam"}

// steamToolSet: a compatibility tool is already chosen for this game.
func steamToolSet(dir, appid string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "config", "config.vdf"))
	if err != nil {
		return false
	}
	name, _ := kvGet(kvText(string(data)), append(steamConfigPath, "CompatToolMapping", appid, "name")...).(string)
	return name != ""
}

// vdfBlock finds the block "name" directly inside lines[from:to] (a block's content): the lines
// of its "{" and its "}", or -1.
func vdfBlock(lines []string, from, to int, name string) (int, int) {
	depth := 0
	for i := from; i < to; i++ {
		t := strings.TrimSpace(lines[i])
		switch {
		case t == "{":
			depth++
		case t == "}":
			depth--
		case depth == 0 && strings.EqualFold(t, `"`+name+`"`) && i+1 < to && strings.TrimSpace(lines[i+1]) == "{":
			d := 0
			for j := i + 1; j < to; j++ {
				switch strings.TrimSpace(lines[j]) {
				case "{":
					d++
				case "}":
					if d--; d == 0 {
						return i + 1, j
					}
				}
			}
			return -1, -1
		}
	}
	return -1, -1
}

// setCompatTool sets the game's compatibility tool in config.vdf's text as Steam writes a user's
// choice (priority 250), replacing an earlier one; false when the file is not the one we know.
func setCompatTool(text, appid, tool string) (string, bool) {
	lines := strings.Split(text, "\n")
	from, to := 0, len(lines)
	for _, name := range steamConfigPath {
		open, end := vdfBlock(lines, from, to, name)
		if open < 0 {
			return text, false
		}
		from, to = open+1, end
	}
	tabs := func(n int) string { return strings.Repeat("\t", n) }
	entry := []string{tabs(5) + `"` + appid + `"`, tabs(5) + "{",
		tabs(6) + "\"name\"\t\t\"" + tool + "\"", tabs(6) + "\"config\"\t\t\"\"", tabs(6) + "\"priority\"\t\t\"250\"",
		tabs(5) + "}"}
	var add []string
	cut, keep := to, to // lines[cut:keep] are replaced by add
	if open, end := vdfBlock(lines, from, to, "CompatToolMapping"); open >= 0 {
		add, cut, keep = entry, end, end
		if o, e := vdfBlock(lines, open+1, end, appid); o >= 0 {
			cut, keep = o-1, e+1
		}
	} else {
		add = append(append([]string{tabs(4) + `"CompatToolMapping"`, tabs(4) + "{"}, entry...), tabs(4)+"}")
	}
	out := append(append(append([]string{}, lines[:cut]...), add...), lines[keep:]...)
	return strings.Join(out, "\n"), true
}

// watchLaunch (from watchSteam, every second while Steam runs) follows the launch of the game
// asked for: Steam's tasks (steamAskNow keeps Steam hidden while it works) and a failure to start
// for a missing program.
func (m *Manager) watchLaunch() {
	m.mu.Lock()
	launch, dir := m.steamLaunch, m.SteamDir
	if launch != nil && time.Since(launch.at) > steamLaunchWatch && !launch.state.busy(time.Now()) {
		m.steamLaunch, launch = nil, nil
	}
	var appid string
	var since time.Time
	var offset int64
	var state launchState
	if launch != nil {
		appid, since, offset, state = launch.appid, launch.at, launch.offset, launch.state
	}
	m.mu.Unlock()
	if launch == nil {
		return
	}
	failed := state.failed
	state, offset = steamLaunchRead(steamConsoleLog(dir), offset, appid, since, state)
	failed = !failed && state.failed
	m.mu.Lock()
	if m.steamLaunch != launch {
		m.mu.Unlock()
		return
	}
	launch.offset, launch.state = offset, state
	if failed {
		m.steamLaunch = nil
	}
	m.mu.Unlock()
	if !failed {
		return
	}
	if steamToolSet(dir, appid) {
		// already with a compatibility tool (the console's or the user's): Steam shows its error
		m.Log.Info("Steam game did not start, with its compatibility tool", "appid", appid)
		return
	}
	m.Log.Info("the game's Linux version does not start (Steam: missing program): the console asks", "appid", appid)
	m.mu.Lock()
	if m.steamAsk != nil { // the console asks instead of Steam's error window
		m.steamAsk.Stop()
		m.steamAsk = nil
	}
	m.mu.Unlock()
	m.Broker.Publish("steam.launch_failed", map[string]string{"appid": appid, "reason": "linux_version"})
}

// steamWindows sets the game's version for Windows in Steam (Steam closed meanwhile) and starts
// the game again. In the background: closing Steam takes a few seconds.
func (m *Manager) steamWindows(req SteamRequest) {
	appid := req.Windows
	m.mu.Lock()
	_, running := m.apps[SteamID]
	dir := m.SteamDir
	m.mu.Unlock()
	if running {
		m.Log.Info("closing Steam to set the game's version for Windows", "appid", appid)
		if err := m.Close(SteamID); err != nil {
			m.Log.Warn("cannot close Steam", "err", err)
			return
		}
		for deadline := time.Now().Add(steamCloseWait); running; time.Sleep(100 * time.Millisecond) {
			if !time.Now().Before(deadline) {
				m.Log.Warn("Steam did not close: the game's version for Windows is not set", "appid", appid)
				return
			}
			m.mu.Lock()
			_, running = m.apps[SteamID]
			m.mu.Unlock()
		}
	}
	file := filepath.Join(dir, "config", "config.vdf")
	data, err := os.ReadFile(file)
	if err != nil {
		m.Log.Warn("cannot read Steam's settings", "err", err)
		return
	}
	tool := steamProtonTool(dir)
	text, ok := setCompatTool(string(data), appid, tool)
	if !ok {
		m.Log.Warn("Steam's settings are not as expected: left alone", "file", file)
		return
	}
	if err := replaceFile(file, []byte(text)); err != nil {
		m.Log.Warn("cannot write Steam's settings", "err", err)
		return
	}
	m.Log.Info("the game's version for Windows set in Steam", "appid", appid, "tool", tool)
	if err := m.Steam(SteamRequest{Run: appid, Name: req.Name, Language: req.Language}); err != nil {
		m.Log.Warn("cannot start the game again", "appid", appid, "err", err)
	}
}
