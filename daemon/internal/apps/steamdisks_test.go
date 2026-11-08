// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// steamLibrariesHanded: the folders handed to Steam with addlibraryfolder, decoded.
func steamLibrariesHanded(t *testing.T, forwarded []string) []string {
	t.Helper()
	var out []string
	for _, f := range forwarded {
		_, enc, ok := strings.Cut(f, "steam://addlibraryfolder/")
		if !ok {
			continue
		}
		path, err := url.PathUnescape(enc)
		if err != nil || len(enc) != 3*len(path) || strings.ToLower(enc) != enc {
			t.Fatalf("not every byte encoded as %%xx: %q", enc)
		}
		out = append(out, path)
	}
	return out
}

// The disks the console prepared for games become Steam libraries once Steam's window is up, once
// per disk while Steam runs; read-only disks, disks not prepared by the console, and folders Steam
// lists already are left alone ([[step-6.7-preparing-other-disks]]).
func TestSteamGetsTheDisksForGames(t *testing.T) {
	m, _, focus, _, forwarded, _ := newSteamManager(t)
	ext4 := ext4At
	ext4At = func(dir string) bool { return !strings.Contains(dir, "exfat") }
	t.Cleanup(func() { ext4At = ext4 })

	media := t.TempDir()
	state := t.TempDir()
	m.MountState = state
	disk := func(name, how string, games bool) string {
		dir := filepath.Join(media, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if games {
			if err := os.Mkdir(filepath.Join(dir, "Giochi"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		writeFile(t, filepath.Join(state, "sd"+name), dir+"\n"+how+"\n\n")
		return dir
	}
	prepared := disk("2TB HDD SATA", "rw", true)
	disk("readonly", "ro", true)
	disk("nogames", "rw", false)
	disk("exfat", "rw", true)
	kept := disk("kept", "rw", true) // Steam's library already at its top
	writeFile(t, filepath.Join(kept, "libraryfolder.vdf"), "\"libraryfolder\"\n{\n}\n")
	listed := disk("listed", "rw", true)
	writeFile(t, filepath.Join(m.SteamDir, "steamapps", "libraryfolders.vdf"),
		"\"libraryfolders\"\n{\n\t\"0\"\n\t{\n\t\t\"path\"\t\t\""+m.SteamDir+"\"\n\t}\n"+
			"\t\"1\"\n\t{\n\t\t\"path\"\t\t\""+listed+"/SteamLibrary\"\n\t}\n}\n")
	writeFile(t, filepath.Join(state, "preparing"), "sdz\n")

	m.watchSteam() // Steam not running
	if got := steamLibrariesHanded(t, *forwarded); len(got) != 0 {
		t.Fatalf("handed before Steam runs: %v", got)
	}
	if err := m.Steam(SteamRequest{Open: "storage"}); err != nil {
		t.Fatal(err)
	}
	m.watchSteam() // no window yet
	if got := steamLibrariesHanded(t, *forwarded); len(got) != 0 {
		t.Fatalf("handed before Steam's window: %v", got)
	}
	focus.openWindow(1001)
	settle(m)
	m.watchSteam()
	got := steamLibrariesHanded(t, *forwarded)
	if len(got) == 0 {
		t.Fatal("nothing handed once Steam's window is up")
	}
	want := []string{filepath.Join(prepared, "SteamLibrary"), kept}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("handed %v, want %v", got, want)
	}
	if st, err := os.Stat(filepath.Join(prepared, "SteamLibrary")); err != nil || !st.IsDir() {
		t.Fatalf("SteamLibrary not made: %v", err)
	}
	m.watchSteam()
	if again := steamLibrariesHanded(t, *forwarded); len(again) != len(want) {
		t.Fatalf("handed twice: %v", again)
	}

	// Steam closed and opened again, its window never seen (the cage session): told again once
	// Steam has run steamDisksAfter
	m.mu.Lock()
	delete(m.apps, SteamID)
	m.mu.Unlock()
	m.watchSteam()
	if err := m.Steam(SteamRequest{Open: "storage"}); err != nil {
		t.Fatal(err)
	}
	m.watchSteam()
	if again := steamLibrariesHanded(t, *forwarded); len(again) != len(want) {
		t.Fatalf("handed too early: %v", again)
	}
	m.mu.Lock()
	m.apps[SteamID].Started = time.Now().Add(-steamDisksAfter)
	m.mu.Unlock()
	m.watchSteam()
	if again := steamLibrariesHanded(t, *forwarded); len(again) != 2*len(want) {
		t.Fatalf("a new Steam not told: %v", again)
	}
}

func TestSteamURLPath(t *testing.T) {
	if got := steamURLPath("/run/a b"); got != "%2f%72%75%6e%2f%61%20%62" {
		t.Fatalf("got %q", got)
	}
}
