// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Steam's console log as on the FrankenPC (2026-10-18), with an older failure and another game's
func TestSteamLaunchRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "console_log.txt")
	since := time.Date(2026, 10, 18, 14, 24, 25, 500e6, time.Local)
	head := `[2026-10-18 14:20:01] GameAction [AppID 399070, ActionID 1] : LaunchApp failed with AppError_28 with "/old"
[2026-10-18 14:24:25] ExecuteSteamURL: "steam://rungameid/399070"
[2026-10-18 14:24:26] GameAction [AppID 399070, ActionID 1] : LaunchApp changed task to DownloadingDepots with ""
[2026-10-18 14:24:27] GameAction [AppID 570, ActionID 2] : LaunchApp failed with AppError_28 with "/g/dota"
[2026-10-18 14:24:27] GameAction [AppID 399070, ActionID 1] : LaunchApp waiting for user response to CreatingProcess ""
`
	failure := `[2026-10-18 14:24:27] GameAction [AppID 399070, ActionID 1] : LaunchApp failed with AppError_28 with "/home/owneet/.local/share/Steam/steamapps/common/Dragon's Wake/Dragon.x86"` + "\n"
	if err := os.WriteFile(path, []byte(head+failure[:40]), 0o644); err != nil {
		t.Fatal(err)
	}
	state, offset := steamLaunchRead(path, 0, "399070", since, launchState{})
	if state.failed || state.task != "CreatingProcess" || !state.waiting || offset != int64(len(head)) {
		t.Fatalf("before the failure: %+v at %d (want %d)", state, offset, len(head))
	}
	at := time.Date(2026, 10, 18, 14, 24, 27, 0, time.Local)
	if !state.busy(at.Add(time.Second)) || state.busy(at.Add(steamAnswerWait+time.Second)) {
		t.Fatal("a question just asked is Steam's, one still open is the user's")
	}
	if err := os.WriteFile(path, []byte(head+failure), 0o644); err != nil {
		t.Fatal(err)
	}
	if state, offset = steamLaunchRead(path, offset, "399070", since, state); !state.failed || state.busy(at) || offset != int64(len(head+failure)) {
		t.Fatalf("the failure: %+v at %d", state, offset)
	}
	// Steam started again: a new, shorter log is read from its start
	if err := os.WriteFile(path, []byte(failure), 0o644); err != nil {
		t.Fatal(err)
	}
	if state, _ = steamLaunchRead(path, offset, "399070", since, launchState{}); !state.failed {
		t.Fatal("new log not read")
	}
	if state, _ = steamLaunchRead(path, 0, "399070", since.Add(time.Minute), launchState{}); state.failed || state.task != "" {
		t.Fatalf("lines before the request counted: %+v", state)
	}
	if state, _ = steamLaunchRead(filepath.Join(t.TempDir(), "none"), 0, "399070", since, launchState{}); state.failed {
		t.Fatal("no log")
	}
}

func TestSteamProtonTool(t *testing.T) {
	dir := t.TempDir()
	if got := steamProtonTool(dir); got != "proton_experimental" {
		t.Fatalf("no log: %q", got)
	}
	os.MkdirAll(filepath.Join(dir, "logs"), 0o755)
	log := `[2026-10-18 14:20:30] Registering tool proton_experimental, AppID 1493710
[2026-10-18 14:20:30] Registering tool proton_9, AppID 2805730
[2026-10-18 14:20:30] Registering tool proton_11, AppID 4628710
[2026-10-18 14:20:30] Registering tool proton_10, AppID 3658110
[2026-10-18 14:20:30] Registering tool proton_hotfix, AppID 2180100
[2026-10-18 14:20:30] Registering tool proton_513, AppID 1420170
[2026-10-18 14:20:30] Registering tool proton_63, AppID 1580130
`
	os.WriteFile(filepath.Join(dir, "logs", "compat_log.txt"), []byte(log), 0o644)
	if got := steamProtonTool(dir); got != "proton_11" {
		t.Fatalf("newest: %q (5.13 is proton_513)", got)
	}
}

// Steam downloading what a game needs (its version for Windows, Proton) stays hidden, however
// long it takes; a question Steam waits on is shown (FrankenPC, 2026-10-18: Steam's window was
// shown 20 s after the start while it downloaded for 45 s).
func TestSteamHiddenWhileItPreparesTheGame(t *testing.T) {
	m, units, focus, _, _, ch := newSteamManager(t)
	dir := m.SteamDir
	os.MkdirAll(filepath.Join(dir, "logs"), 0o755)
	os.MkdirAll(filepath.Join(dir, "config"), 0o755)
	os.WriteFile(filepath.Join(dir, "config", "loginusers.vdf"), []byte(`"users" { "76561190000000001" { "AccountName" "a" } }`), 0o644)
	log := filepath.Join(dir, "logs", "console_log.txt")
	os.WriteFile(log, nil, 0o644)
	line := func(at time.Time, what string) {
		f, _ := os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0)
		f.WriteString("[" + at.Format("2006-01-02 15:04:05") + "] GameAction [AppID 399070, ActionID 1] : LaunchApp " + what + "\n")
		f.Close()
	}
	if err := m.Steam(SteamRequest{Run: "399070", Name: "Dragon's Wake"}); err != nil {
		t.Fatal(err)
	}
	next(t, ch, "app.started")
	_ = units
	m.mu.Lock()
	m.steamLaunch.at = time.Now().Add(-time.Minute) // asked a minute ago: the lines below count
	m.mu.Unlock()
	focus.openWindow(1001)
	settle(m)
	line(time.Now(), `changed task to DownloadingDepots with ""`)
	m.watchSteam()
	m.steamAskNow() // as steamAskAfter would
	if st := m.Status(); st.Focus != "home" {
		t.Fatalf("Steam shown while it downloads: %q", st.Focus)
	}
	m.mu.Lock()
	again := m.steamAsk != nil
	m.mu.Unlock()
	if !again {
		t.Fatal("not looked at again")
	}
	line(time.Now(), `waiting for user response to ShowInterstitials ""`) // a routine question
	m.watchSteam()
	m.steamAskNow()
	if st := m.Status(); st.Focus != "home" {
		t.Fatalf("Steam shown for its own question: %q", st.Focus)
	}
	line(time.Now().Add(-5*time.Second), `waiting for user response to ShowCloudConflict ""`) // still open
	m.watchSteam()
	m.steamAskNow()
	if st := m.Status(); st.Focus != SteamID {
		t.Fatalf("Steam not shown for a question to the user: %q", st.Focus)
	}
}

const steamConfig = `"InstallConfigStore"
{
	"Software"
	{
		"Valve"
		{
			"Steam"
			{
				"AutoUpdateWindowEnabled"		"0"
				"CompatToolMapping"
				{
					"0"
					{
						"name"		"proton_9"
						"config"		""
						"priority"		"75"
					}
					"399070"
					{
						"name"		""
						"config"		""
						"priority"		"250"
					}
				}
				"Accounts"
				{
					"someone"
					{
						"SteamID"		"76561190000000001"
					}
				}
			}
		}
	}
	"Music"
	{
		"CrawlSteamInstallFolders"		"1"
	}
}
`

func TestSetCompatTool(t *testing.T) {
	check := func(text, appid, want string) map[string]any {
		t.Helper()
		kv := kvText(text)
		if got, _ := kvGet(kv, append(steamConfigPath, "CompatToolMapping", appid, "name")...).(string); got != want {
			t.Fatalf("%s: tool %q, want %q\n%s", appid, got, want, text)
		}
		if got, _ := kvGet(kv, append(steamConfigPath, "Accounts", "someone", "SteamID")...).(string); got != "76561190000000001" {
			t.Fatalf("the rest of the file changed:\n%s", text)
		}
		return kv
	}
	// an entry there already (Steam writes one with no name after "default"): replaced
	text, ok := setCompatTool(steamConfig, "399070", "proton_11")
	if !ok {
		t.Fatal("not recognised")
	}
	kv := check(text, "399070", "proton_11")
	if got, _ := kvGet(kv, append(steamConfigPath, "CompatToolMapping", "0", "name")...).(string); got != "proton_9" {
		t.Fatalf("other mapping changed: %q", got)
	}
	if strings.Count(text, `"399070"`) != 1 {
		t.Fatalf("entry twice:\n%s", text)
	}
	// a new entry in the existing block
	text, _ = setCompatTool(text, "480", "proton_experimental")
	check(text, "480", "proton_experimental")
	check(text, "399070", "proton_11")
	// no block yet: made in Steam's block
	bare := strings.Replace(steamConfig, steamConfig[strings.Index(steamConfig, "\t\t\t\t\"CompatToolMapping\""):strings.Index(steamConfig, "\t\t\t\t\"Accounts\"")], "", 1)
	text, ok = setCompatTool(bare, "399070", "proton_11")
	if !ok {
		t.Fatal("no CompatToolMapping: not recognised")
	}
	check(text, "399070", "proton_11")
	if got, _ := kvGet(kvText(text), "InstallConfigStore", "Music", "CrawlSteamInstallFolders").(string); got != "1" {
		t.Fatalf("block after Steam's changed:\n%s", text)
	}
	if _, ok := setCompatTool(`"Something"
{
}
`, "399070", "proton_11"); ok {
		t.Fatal("unknown file changed")
	}
}

// The console's request for a game whose Linux version does not start: owneetd sees Steam's
// failure, says so (Steam stays hidden), and Try sets the version for Windows with Steam closed,
// then starts the game again.
func TestSteamWindowsVersion(t *testing.T) {
	m, units, focus, _, forwarded, ch := newSteamManager(t)
	dir := m.SteamDir
	for _, d := range []string{"logs", "config"} {
		os.MkdirAll(filepath.Join(dir, d), 0o755)
	}
	os.WriteFile(filepath.Join(dir, "config", "loginusers.vdf"), []byte(`"users" { "76561190000000001" { "AccountName" "a" } }`), 0o644)
	os.WriteFile(filepath.Join(dir, "config", "config.vdf"), []byte(steamConfig), 0o644)
	os.WriteFile(filepath.Join(dir, "logs", "compat_log.txt"), []byte("Registering tool proton_11, AppID 4628710\n"), 0o644)
	log := filepath.Join(dir, "logs", "console_log.txt")
	os.WriteFile(log, []byte("[2020-01-01 10:00:00] GameAction [AppID 399070, ActionID 1] : LaunchApp failed with AppError_28 with \"/old\"\n"), 0o644)

	if err := m.Steam(SteamRequest{Run: "399070", Name: "Dragon's Wake"}); err != nil {
		t.Fatal(err)
	}
	next(t, ch, "app.started") // Steam itself
	steamUnit := units.started[0]
	focus.openWindow(1001)
	settle(m)
	m.watchSteam() // only an old failure in the log
	f, _ := os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("[" + time.Now().Format("2006-01-02 15:04:05") + "] GameAction [AppID 399070, ActionID 1] : LaunchApp failed with AppError_28 with \"/g/Dragon.x86\"\n")
	f.Close()
	m.watchSteam()
	e := next(t, ch, "steam.launch_failed")
	if d := e.Data.(map[string]string); d["appid"] != "399070" || d["reason"] != "linux_version" {
		t.Fatalf("steam.launch_failed %v", d)
	}
	if st := m.Status(); st.Focus != "home" {
		t.Fatalf("Steam shown with its error: %q", st.Focus)
	}
	m.mu.Lock()
	asking := m.steamAsk != nil
	m.mu.Unlock()
	if asking {
		t.Fatal("Steam still to be shown after steamAskAfter")
	}

	// Try: Steam closes, the setting is written, the game starts again
	if err := m.Steam(SteamRequest{Windows: "399070", Name: "Dragon's Wake"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		units.mu.Lock()
		stopped := len(units.stopped) > 0 && units.stopped[0] == steamUnit
		units.mu.Unlock()
		if stopped {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Steam not closed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "config", "config.vdf")); strings.Contains(string(data), "proton_11") {
		t.Fatal("settings written while Steam runs")
	}
	m.exited(Exit{Unit: steamUnit, Result: "success"}) // Steam has closed
	next(t, ch, "app.exited")
	next(t, ch, "app.started") // Steam again, for the game
	data, _ := os.ReadFile(filepath.Join(dir, "config", "config.vdf"))
	if got, _ := kvGet(kvText(string(data)), append(steamConfigPath, "CompatToolMapping", "399070", "name")...).(string); got != "proton_11" {
		t.Fatalf("tool %q", got)
	}
	focus.openWindow(1002)
	settle(m)
	if last := (*forwarded)[len(*forwarded)-1]; !strings.HasSuffix(last, "steam://rungameid/399070") {
		t.Fatalf("forwarded %v", *forwarded)
	}
	// with a tool set, a failure is Steam's to show: not asked again
	f, _ = os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("[" + time.Now().Format("2006-01-02 15:04:05") + "] GameAction [AppID 399070, ActionID 3] : LaunchApp failed with AppError_28 with \"/g/Dragon.exe\"\n")
	f.Close()
	m.watchSteam()
	select {
	case e := <-ch:
		if e.Type == "steam.launch_failed" {
			t.Fatal("asked again with the version for Windows")
		}
	case <-time.After(100 * time.Millisecond):
	}
}
