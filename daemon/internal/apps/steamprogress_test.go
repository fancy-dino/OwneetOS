// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"os"
	"path/filepath"
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
	if d := download(t, m, "208650"); d.State != "queued" || d.Done != 0 {
		t.Fatalf("preparing: %+v", d)
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
	// its shader cache first: still not the download
	appendFile(t, logFile, "[2026-11-09 09:30:40] AppID 208650 Shader update changed : Running Update,Downloading,Staging,\n")
	if d := download(t, m, "208650"); d.State != "queued" {
		t.Fatalf("shader cache: %+v", d)
	}
	appendFile(t, logFile, "[2026-11-09 09:36:00] AppID 208650 App update changed : Running Update,Downloading,Staging,\n")
	if d := download(t, m, "208650"); d.State != "downloading" || d.Done < 1500 {
		t.Fatalf("downloading again: %+v", d)
	}
}

func TestSteamPhaseOf(t *testing.T) {
	for _, c := range []struct{ kind, flags, want string }{
		{"App", "None", steamIdle},
		{"App", "Running Update,Downloading,Staging,", steamDownloading},
		{"App", "Running Update,Committing,", steamDownloading},
		{"App", "Running Update,Verifying Installed,", steamPreparing},
		{"App", "Running Update,Preallocating,", steamPreparing},
		{"Shader", "Running Update,Downloading,Staging,", steamPreparing},
		{"App", "Running Update,Downloading,Staging,Stopping,", steamDownloading},
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
