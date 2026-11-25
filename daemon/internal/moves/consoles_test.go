// SPDX-License-Identifier: GPL-3.0-or-later

package moves

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fancy-dino/OwneetOS/daemon/internal/gamedata"
)

// setupConsole: a console of the user's on the USB disk, what was unpacked for its two emulators (an
// archive with a save in it, an AppImage), and what was chosen for it.
func setupConsole(t *testing.T) (*Manager, string, string) {
	t.Helper()
	m, _, media := setup(t)
	console := filepath.Join(media, "USB", "Emulazione", "FunContainer")
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(console, "Emulatori", "BoxEmu.zip"), "zip")
	write(filepath.Join(console, "Emulatori", "FunPlayer.AppImage"), "app")
	write(filepath.Join(console, "Giochi", "Copper Coast.one"), "game")
	for name, files := range map[string][]string{"BoxEmu.zip": {"open/save.dat", "stamp", "program"}, "FunPlayer.AppImage": {"app/squashfs-root/AppRun", "stamp"}} {
		emulator := filepath.Join(console, "Emulatori", name)
		dir := gamedata.Dir(m.Home, "emulators", emulator)
		for _, f := range files {
			write(filepath.Join(dir, f), "x")
		}
		write(filepath.Join(dir, "source"), emulator)
	}
	game := filepath.Join(console, "Giochi", "Copper Coast.one")
	choices, _ := json.Marshal(map[string]any{console: map[string]any{"emulator": "BoxEmu.zip", "told": []string{"BoxEmu.zip"},
		"options": map[string]string{"BoxEmu.zip": "-g"}, "files": map[string]string{game: game}}, "/other": map[string]any{"emulator": "x"}})
	write(filepath.Join(m.Home, ".config", "owneet-frontend", "emulators.json"), string(choices))
	return m, console, media
}

func consoleChoices(t *testing.T, m *Manager) map[string]map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(m.Home, ".config", "owneet-frontend", "emulators.json"))
	if err != nil {
		t.Fatal(err)
	}
	var all map[string]map[string]any
	if err := json.Unmarshal(data, &all); err != nil {
		t.Fatal(err)
	}
	return all
}

func there(path string) bool { _, err := os.Stat(path); return err == nil }

func TestAConsoleMovesWithWhatIsItsOwn(t *testing.T) {
	m, console, media := setupConsole(t)
	if err := m.Start(Request{Games: []Game{{Dir: console, Title: "FunContainer"}}, To: filepath.Join(media, "HDD"), Remove: true}); err != nil {
		t.Fatal(err)
	}
	st := finish(t, m)
	now := filepath.Join(media, "HDD", "Emulazione", "FunContainer") // a new folder, with the name it had
	if len(st.Moved) != 1 || st.Moved[0] != now || len(st.Failed) != 0 {
		t.Fatalf("status %+v", st)
	}
	if there(console) || !there(filepath.Join(now, "Giochi", "Copper Coast.one")) {
		t.Error("not moved")
	}
	for name, kept := range map[string]string{"BoxEmu.zip": "open/save.dat", "FunPlayer.AppImage": "app/squashfs-root/AppRun"} {
		emulator := filepath.Join(now, "Emulatori", name)
		dir := gamedata.Dir(m.Home, "emulators", emulator)
		source, _ := os.ReadFile(filepath.Join(dir, "source"))
		if !there(filepath.Join(dir, kept)) || there(filepath.Join(dir, "stamp")) || string(source) != emulator {
			t.Errorf("%s: its own folder: %q", name, source)
		}
		if there(gamedata.Dir(m.Home, "emulators", filepath.Join(console, "Emulatori", name))) {
			t.Errorf("%s: its old folder is still there", name)
		}
	}
	all := consoleChoices(t, m)
	game := filepath.Join(now, "Giochi", "Copper Coast.one")
	c := all[now]
	if c == nil || c["emulator"] != "BoxEmu.zip" || c["options"].(map[string]any)["BoxEmu.zip"] != "-g" || c["files"].(map[string]any)[game] != game {
		t.Errorf("choices %v", all)
	}
	if all[console] != nil || all["/other"] == nil {
		t.Errorf("choices of the others %v", all)
	}
}

func TestAConsoleCopiedKeepsTheOriginals(t *testing.T) {
	m, console, _ := setupConsole(t)
	// the console's disk has the folder already, with another of its names
	if err := os.MkdirAll(filepath.Join(m.Home, "Emulation"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(Request{Games: []Game{{Dir: console, Title: "FunContainer"}}, To: Internal}); err != nil {
		t.Fatal(err)
	}
	st := finish(t, m)
	now := filepath.Join(m.Home, "Emulation", "FunContainer")
	if len(st.Moved) != 1 || st.Moved[0] != now {
		t.Fatalf("status %+v", st)
	}
	if !there(filepath.Join(console, "Emulatori", "BoxEmu.zip")) {
		t.Error("the original was touched")
	}
	old := gamedata.Dir(m.Home, "emulators", filepath.Join(console, "Emulatori", "BoxEmu.zip"))
	dir := gamedata.Dir(m.Home, "emulators", filepath.Join(now, "Emulatori", "BoxEmu.zip"))
	if !there(filepath.Join(old, "open/save.dat")) || !there(filepath.Join(dir, "open/save.dat")) || there(filepath.Join(dir, "stamp")) {
		t.Error("what the archive left was not copied")
	}
	if there(gamedata.Dir(m.Home, "emulators", filepath.Join(now, "Emulatori", "FunPlayer.AppImage"))) {
		t.Error("an AppImage is unpacked again by itself")
	}
	all := consoleChoices(t, m)
	if all[now] == nil || all[console] == nil {
		t.Errorf("choices %v", all)
	}
}

func TestAConsoleIsNeverPutOverAnother(t *testing.T) {
	m, console, media := setupConsole(t)
	if err := os.MkdirAll(filepath.Join(media, "HDD", "EMULATION", "funcontainer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(Request{Games: []Game{{Dir: console, Title: "FunContainer"}}, To: filepath.Join(media, "HDD"), Remove: true}); !errors.Is(err, ErrTaken) {
		t.Fatalf("err %v", err)
	}
	// the folder itself, or what is in a console, is not a console
	for _, dir := range []string{filepath.Dir(console), filepath.Join(console, "Giochi")} {
		if err := m.Start(Request{Games: []Game{{Dir: dir}}, To: Internal}); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", dir, err)
		}
	}
	for name, want := range map[string]bool{"Emulation": true, "EMULAZIONE": true, "Émulation": true, "Émulation": true, "Emulación": true,
		"emulación": true, "Emulators": false, "Games": false} {
		if isEmulationFolder(name) != want {
			t.Errorf("%q", name)
		}
	}
}

func TestGamesAndConsolesTogether(t *testing.T) {
	m, console, media := setupConsole(t)
	game := filepath.Join(media, "USB", "Giochi", "Tyrian")
	if err := m.Start(Request{Games: []Game{{Dir: console, Title: "FunContainer"}, {Dir: game, Title: "Tyrian"}}, To: Internal}); err != nil {
		t.Fatal(err)
	}
	st := finish(t, m)
	if len(st.Moved) != 2 || st.Moved[0] != filepath.Join(m.Home, "Emulazione", "FunContainer") || st.Moved[1] != filepath.Join(m.Home, "Games", "Tyrian") {
		t.Fatalf("status %+v", st)
	}
}
