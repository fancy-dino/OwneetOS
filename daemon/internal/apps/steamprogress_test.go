// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func appendFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func download(t *testing.T, m *Manager, id string) SteamDownload {
	t.Helper()
	for _, d := range m.SteamStatus().Downloads {
		if d.AppID == id {
			return d
		}
	}
	t.Fatalf("no download %s", id)
	return SteamDownload{}
}

// A download Steam works on while its manifest stays still (Batman: Arkham Knight, FrankenPC,
// 2026-11-09): its state comes from Steam's content log, its bytes from the room Steam takes on its disks
// ([[step-4.2c-steam-without-its-interface]]).
func TestSteamDownloadFromItsLogAndFiles(t *testing.T) {
	m, _, focus, _, _, _ := newSteamManager(t)
	used := int64(10000) // the room taken on Steam's disks
	usedOn0 := usedOn
	usedOn = func([]string) int64 { return used }
	t.Cleanup(func() { usedOn = usedOn0 })
	if err := m.Steam(SteamRequest{Open: "library"}); err != nil {
		t.Fatal(err)
	}
	focus.openWindow(1001)
	settle(m)
	dir := m.SteamDir
	// what Steam wrote: 1000 bytes to download, 2000 to write, nothing done yet, never "running"
	writeFile(t, filepath.Join(dir, "steamapps", "appmanifest_208650.acf"), "\"AppState\"\n{\n\t\"appid\"\t\t\"208650\"\n"+
		"\t\"installdir\"\t\t\"Batman Arkham Knight\"\n\t\"StateFlags\"\t\t\"1026\"\n\t\"BytesToDownload\"\t\t\"1000\"\n"+
		"\t\"BytesDownloaded\"\t\t\"0\"\n\t\"BytesToStage\"\t\t\"2000\"\n\t\"BytesStaged\"\t\t\"0\"\n}\n")
	logFile := steamContentLog(dir)
	appendFile(t, logFile, "[2026-11-09 09:00:00] AppID 208650 App update changed : Running Update,Downloading,Staging,\n"+
		"[2026-11-09 09:00:01] Client version: 1792618285\n") // an older start of Steam: forgotten
	if d := download(t, m, "208650"); d.State != "queued" {
		t.Fatalf("before Steam's log says anything: %+v", d)
	}

	appendFile(t, logFile, "[2026-11-09 09:30:17] AppID 208650 App update changed : Running Update,Reconfiguring,\n")
	if d := download(t, m, "208650"); d.State != "preparing" || d.Done != 0 || d.Rate != 0 {
		t.Fatalf("preparing: %+v", d)
	}
	appendFile(t, logFile, "[2026-11-09 09:30:30] AppID 208650 App update changed : Running Update,Verifying Installed,\n"+
		"[2026-11-09 09:30:31] AppID 208650 App update changed : Running Update,\n"+
		"[2026-11-09 09:30:31] AppID 208650 App update changed : Running Update,Preallocating,\n")
	if d := download(t, m, "208650"); d.State != "preparing" {
		t.Fatalf("still preparing: %+v", d)
	}
	// Steam downloads: measured from here, then 500 of the 2000 bytes written
	appendFile(t, logFile, "[2026-11-09 09:30:35] AppID 208650 App update changed : Running Update,Downloading,Staging,\n")
	if d := download(t, m, "208650"); d.State != "downloading" || d.Done != 0 || d.Total != 3000 {
		t.Fatalf("download started: %+v", d)
	}
	used += 500
	m.steamMu.Lock()
	m.steamSamples["208650"].measured = time.Now().Add(-steamWrittenEvery)
	m.steamMu.Unlock()
	if d := download(t, m, "208650"); d.State != "downloading" || d.Done != 750 || d.Total != 3000 {
		t.Fatalf("downloading: %+v", d)
	}
	// more written (and something else deleted: no step back), measured again after steamWrittenEvery
	used += 500
	if d := download(t, m, "208650"); d.Done != 750 {
		t.Fatalf("measured again too soon: %+v", d)
	}
	m.steamMu.Lock()
	m.steamSamples["208650"].measured = time.Now().Add(-steamWrittenEvery)
	m.steamSamples["208650"].at = time.Now().Add(-steamWrittenEvery)
	m.steamMu.Unlock()
	if d := download(t, m, "208650"); d.State != "downloading" || d.Done != 1500 || d.Rate <= 0 {
		t.Fatalf("downloading on: %+v", d)
	}
	used -= 5000
	m.steamMu.Lock()
	m.steamSamples["208650"].measured = time.Now().Add(-steamWrittenEvery)
	m.steamMu.Unlock()
	if d := download(t, m, "208650"); d.Done != 1500 {
		t.Fatalf("after a deletion: %+v", d)
	}
	// Steam stops it (another game first, or no internet)
	appendFile(t, logFile, "[2026-11-09 09:30:36] AppID 208650 update canceled : Shader Priority (Suspended)\n")
	if d := download(t, m, "208650"); d.State != "queued" {
		t.Fatalf("suspended: %+v", d)
	}
	// its shader cache first: still not the download, Steam prepares it
	appendFile(t, logFile, "[2026-11-09 09:30:40] AppID 208650 Shader update changed : Running Update,Downloading,Staging,\n")
	if d := download(t, m, "208650"); d.State != "preparing" {
		t.Fatalf("shader cache: %+v", d)
	}
	appendFile(t, logFile, "[2026-11-09 09:30:41] AppID 208650 Shader update changed : Running Update,Stopping,\n"+
		"[2026-11-09 09:30:41] AppID 208650 Shader update changed : None\n")
	if d := download(t, m, "208650"); d.State != "preparing" {
		t.Fatalf("after its shader cache (Shader None leaves the phase as it was): %+v", d)
	}
	appendFile(t, logFile, "[2026-11-09 09:36:00] AppID 208650 App update changed : Running Update,Downloading,Staging,\n")
	if d := download(t, m, "208650"); d.State != "downloading" || d.Done < 1500 {
		t.Fatalf("downloading again: %+v", d)
	}
}

// A new game's shader pre-cache downloaded before the game (THE FINALS, FrankenPC 2026-11-23, [[open-items]] 25):
// shown as a download, its bytes from its download folder against the size Steam started it with. Steam does it
// in several updates (three for THE FINALS), each counted on its own; a resumed one is not counted twice; a file
// longer than what is written counts for what is written. Then "preparing" until the game's own download. An
// installed game's shader update is not listed, as before.
func TestSteamShaderPreCacheDownload(t *testing.T) {
	m, _, focus, _, _, _ := newSteamManager(t)
	if err := m.Steam(SteamRequest{Open: "library"}); err != nil {
		t.Fatal(err)
	}
	focus.openWindow(1001)
	settle(m)
	dir := m.SteamDir
	const kib = 1024
	manifest := func(id, name, flags string) {
		writeFile(t, filepath.Join(dir, "steamapps", "appmanifest_"+id+".acf"), "\"AppState\"\n{\n\t\"appid\"\t\t\""+id+"\"\n"+
			"\t\"installdir\"\t\t\""+name+"\"\n\t\"StateFlags\"\t\t\""+flags+"\"\n\t\"BytesToDownload\"\t\t\"5000\"\n"+
			"\t\"BytesDownloaded\"\t\t\"0\"\n\t\"BytesToStage\"\t\t\"8000\"\n\t\"BytesStaged\"\t\t\"0\"\n}\n")
	}
	manifest("2073850", "THE FINALS", "1026")
	logFile := steamContentLog(dir)
	line := func(text string) { appendFile(t, logFile, "[2026-11-23 20:44:20] AppID 2073850 "+text+"\n") }
	update := func(done, total int) {
		line(fmt.Sprintf("update started : download 0/9, store 0/0, reuse 0/0, delta 0/0, stage %d/%d ", done, total))
		line("Shader update changed : Running Update,Downloading,Staging,")
	}
	downloads := filepath.Join(dir, "steamapps", "shadercache", "2073850", "downloads")
	later := func() { // the folder is measured again
		m.steamMu.Lock()
		m.steamSamples["2073850"].shaderMeasured = time.Now().Add(-steamWrittenEvery)
		m.steamSamples["2073850"].at = time.Now().Add(-steamWrittenEvery)
		m.steamMu.Unlock()
	}
	want := func(what, state string, done, total int64) SteamDownload {
		t.Helper()
		d := download(t, m, "2073850")
		if d.State != state || (state == "downloading" && (d.Done != done || d.Total != total)) {
			t.Fatalf("%s: %+v, want %s %d/%d", what, d, state, done, total)
		}
		return d
	}

	line("Shader update changed : Running Update,Downloading,Staging,")
	want("its size not known", "preparing", 0, 0)
	// the first update
	update(0, 1200*kib)
	want("started", "downloading", 0, 1200*kib)
	writeFile(t, filepath.Join(downloads, "a.foz"), strings.Repeat("x", 600*kib))
	want("measured too soon", "downloading", 0, 1200*kib)
	later()
	if d := want("half way", "downloading", 600*kib, 1200*kib); d.Rate <= 0 {
		t.Fatalf("no speed: %+v", d)
	}
	// its files leave the folder at its end: no step back
	line("Shader update changed : Running Update,Committing,")
	if err := os.RemoveAll(downloads); err != nil {
		t.Fatal(err)
	}
	later()
	want("its files moved", "downloading", 600*kib, 1200*kib)
	line("Shader update changed : Running Update,")
	line("Shader update changed : None")
	want("over", "preparing", 0, 0)
	// the next update starts again from nothing, also without a check in between
	update(0, 1000*kib)
	later()
	want("the next update", "downloading", 0, 1000*kib)
	// a file longer than what is written of it
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(downloads, "b.foz"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(strings.Repeat("x", 100*kib))); err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(900 * kib); err != nil {
		t.Fatal(err)
	}
	f.Close()
	later()
	if d := download(t, m, "2073850"); d.State != "downloading" || d.Done < 100*kib || d.Done > 200*kib {
		t.Fatalf("a file longer than what is written: %+v", d)
	}
	// stopped and resumed: what Steam had done is already in the folder
	line("Shader update changed : Running Update,Stopping,")
	writeFile(t, filepath.Join(downloads, "b.foz"), strings.Repeat("x", 400*kib))
	update(390*kib, 1000*kib)
	want("resumed", "downloading", 400*kib, 1000*kib)
	// the game's own download
	line("Shader update changed : None")
	line("App update changed : Running Update,Downloading,Staging,")
	if d := download(t, m, "2073850"); d.State != "downloading" || d.Total != 13000 {
		t.Fatalf("the game's own download: %+v", d)
	}
	// an installed game's shader update: not a download of the console
	manifest("570", "dota 2 beta", "4")
	appendFile(t, logFile, "[2026-11-23 20:52:00] AppID 570 update started : download 0/900, store 0/0, reuse 0/0, delta 0/0, stage 0/900 \n"+
		"[2026-11-23 20:52:00] AppID 570 Shader update changed : Running Update,Downloading,Staging,\n")
	for _, d := range m.SteamStatus().Downloads {
		if d.AppID == "570" {
			t.Fatalf("installed game's shader update listed: %+v", d)
		}
	}
}

func TestSteamPhaseOf(t *testing.T) {
	for _, c := range []struct{ kind, flags, want string }{
		{"App", "None", steamIdle},
		{"App", "Running Update,Downloading,Staging,", steamDownloading},
		{"App", "Running Update,Committing,", steamDownloading},
		{"App", "Running Update,Verifying Installed,", steamPreparing},
		{"App", "Running Update,Preallocating,", steamPreparing},
		{"Shader", "Running Update,Downloading,Staging,", steamShaders},
		{"Shader", "Running Update,Committing,", steamShaders},
		{"Shader", "Running Update,Downloading,Staging,Stopping,", steamIdle},
		{"Shader", "Running Update,Reconfiguring,", steamPreparing},
		{"App", "Running Update,Downloading,Staging,Stopping,", steamDownloading},
		{"App", "Running Update,Stopping,", steamIdle},
		{"App", "Running Update,Reconfiguring,Stopping,", steamIdle},
		{"App", "Running Update,Reconfiguring,", steamPreparing},
		{"Shader", "Running Update,Stopping,", steamIdle},
	} {
		if got := steamPhaseOf(c.kind, c.flags); got != c.want {
			t.Errorf("%s %q: %q, want %q", c.kind, c.flags, got, c.want)
		}
	}
}

// A download resumed after a restart: the room measured from then adds to what Steam had done
func TestSteamDownloadResumed(t *testing.T) {
	m, _, focus, _, _, _ := newSteamManager(t)
	used := int64(10000)
	usedOn0 := usedOn
	usedOn = func([]string) int64 { return used }
	t.Cleanup(func() { usedOn = usedOn0 })
	if err := m.Steam(SteamRequest{Open: "library"}); err != nil {
		t.Fatal(err)
	}
	focus.openWindow(1001)
	settle(m)
	dir := m.SteamDir
	writeFile(t, filepath.Join(dir, "steamapps", "appmanifest_945360.acf"), "\"AppState\"\n{\n\t\"appid\"\t\t\"945360\"\n"+
		"\t\"StateFlags\"\t\t\"1026\"\n\t\"BytesToDownload\"\t\t\"1000\"\n\t\"BytesDownloaded\"\t\t\"600\"\n"+
		"\t\"BytesToStage\"\t\t\"2000\"\n\t\"BytesStaged\"\t\t\"1200\"\n}\n")
	appendFile(t, steamContentLog(dir), "[2026-11-09 09:11:05] AppID 945360 App update changed : Running Update,Downloading,Staging,\n")
	if d := download(t, m, "945360"); d.State != "downloading" || d.Done != 1800 {
		t.Fatalf("resumed: %+v", d)
	}
	used += 400 // a fifth of what is written on the disks: 600 of the 3000
	m.steamMu.Lock()
	m.steamSamples["945360"].measured = time.Now().Add(-steamWrittenEvery)
	m.steamMu.Unlock()
	if d := download(t, m, "945360"); d.Done != 2400 {
		t.Fatalf("resumed, on: %+v", d)
	}
}
