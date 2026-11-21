// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const steamLibrariesText = `"libraryfolders"
{
	"0"
	{
		"path"		"%STEAM%"
		"label"		""
		"contentid"		"111"
		"totalsize"		"0"
		"apps"
		{
			"228980"		"0"
		}
	}
	"1"
	{
		"path"		"%LISTED%/SteamLibrary"
		"label"		""
		"contentid"		"222"
		"totalsize"		"0"
		"apps"
		{
		}
	}
}
`

// The disks the console prepared for games are written into Steam's list of libraries when the
// console starts Steam: read-only disks, disks not prepared by the console and folders Steam lists
// already are left alone ([[step-6.7-preparing-other-disks]]).
func TestSteamGetsTheDisksForGames(t *testing.T) {
	m, _, _, _, _, _ := newSteamManager(t)
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
	writeFile(t, filepath.Join(kept, "libraryfolder.vdf"), "\"libraryfolder\"\n{\n\t\"contentid\"\t\t\"333\"\n\t\"label\"\t\t\"\"\n}\n")
	listed := disk("listed", "rw", true)
	writeFile(t, filepath.Join(state, "preparing"), "sdz\n")
	vdf := filepath.Join(m.SteamDir, "steamapps", "libraryfolders.vdf")
	writeFile(t, vdf, strings.NewReplacer("%STEAM%", m.SteamDir, "%LISTED%", listed).Replace(steamLibrariesText))
	// Steam's second copy (config/): here without the listed disk, which it gets too
	configVdf := filepath.Join(m.SteamDir, "config", "libraryfolders.vdf")
	writeFile(t, configVdf, strings.NewReplacer("%STEAM%", m.SteamDir, "%LISTED%/SteamLibrary", m.SteamDir+"/other").Replace(steamLibrariesText))

	if err := m.Steam(SteamRequest{Open: "storage"}); err != nil {
		t.Fatal(err)
	}
	var paths []string
	ids := map[string]string{}
	for _, f := range steamFolders(m.SteamDir) {
		paths = append(paths, f.path)
	}
	data, _ := os.ReadFile(vdf)
	root, _ := kvGet(kvText(string(data)), "libraryfolders").(map[string]any)
	for k, v := range root {
		f := v.(map[string]any)
		ids[f["path"].(string)] = k + ":" + f["contentid"].(string)
	}
	lib := filepath.Join(prepared, "SteamLibrary")
	want := []string{lib, kept, listed + "/SteamLibrary", m.SteamDir}
	got := strings.Join(paths, "|")
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Fatalf("libraries %v, want %v", paths, want)
		}
	}
	if len(paths) != len(want) {
		t.Fatalf("libraries %v, want %v", paths, want)
	}
	if ids[kept] != "3:333" || !strings.HasPrefix(ids[lib], "2:") || ids[m.SteamDir] != "0:111" {
		t.Fatalf("numbers and contentids %v", ids)
	}
	if id := libraryContentID(lib); id == "" || "2:"+id != ids[lib] {
		t.Fatalf("libraryfolder.vdf of the new library: %q, listed %q", id, ids[lib])
	}
	labels := map[string]string{}
	for _, v := range root {
		f := v.(map[string]any)
		labels[f["path"].(string)] = f["label"].(string)
	}
	if labels[lib] != "2TB HDD SATA" || labels[kept] != "kept" || labels[m.SteamDir] != "" {
		t.Fatalf("labels %v", labels)
	}
	if lf, _ := os.ReadFile(filepath.Join(lib, "libraryfolder.vdf")); !strings.Contains(string(lf), "\"label\"\t\t\"2TB HDD SATA\"") {
		t.Fatalf("libraryfolder.vdf without the label:\n%s", lf)
	}
	if st, err := os.Stat(filepath.Join(lib, "steamapps")); err != nil || !st.IsDir() {
		t.Fatalf("steamapps not made: %v", err)
	}
	if !strings.Contains(string(data), "\"228980\"\t\t\"0\"") {
		t.Fatal("Steam's own entries changed")
	}
	cfg, _ := os.ReadFile(configVdf)
	cfgListed := listedLibraries(string(cfg))
	for _, w := range []string{lib, kept, listed + "/SteamLibrary", m.SteamDir + "/other"} {
		if !cfgListed[w] {
			t.Fatalf("config/ copy lacks %s:\n%s", w, cfg)
		}
	}
	if id := libraryContentID(lib); !strings.Contains(string(cfg), "\""+id+"\"") {
		t.Fatalf("config/ copy has another contentid for %s:\n%s", lib, cfg)
	}

	// Steam started again: nothing added twice
	before, cfgBefore := string(data), string(cfg)
	m.mu.Lock()
	delete(m.apps, SteamID)
	m.mu.Unlock()
	if err := m.Steam(SteamRequest{Open: "storage"}); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(vdf); string(after) != before {
		t.Fatalf("written again:\n%s", after)
	}
	if after, _ := os.ReadFile(configVdf); string(after) != cfgBefore {
		t.Fatalf("config/ copy written again:\n%s", after)
	}
}

// Steam never started: its list is not made by owneetd
func TestSteamDisksBeforeSteamsFirstStart(t *testing.T) {
	steam := t.TempDir()
	if libs, err := steamLibraryDisks(steam, t.TempDir(), os.Getuid()); err != nil || libs != nil {
		t.Fatalf("got %v, %v", libs, err)
	}
	if _, err := os.Stat(filepath.Join(steam, "steamapps", "libraryfolders.vdf")); err == nil {
		t.Fatal("libraryfolders.vdf made")
	}
}

// A file that is not Steam's list is left alone
func TestSteamDisksStrangeFile(t *testing.T) {
	if _, ok := addLibraries("\"other\"\n{\n}\n", []string{"/a"}, []string{"1"}, []string{"a"}); ok {
		t.Fatal("added to a file that is not libraryfolders")
	}
}

func TestVdfString(t *testing.T) {
	if got := vdfString(`My "big" disk\x`); got != `"My \"big\" disk\\x"` {
		t.Fatalf("got %s", got)
	}
}
