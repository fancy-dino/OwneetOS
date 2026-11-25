// SPDX-License-Identifier: GPL-3.0-or-later

package moves

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/fancy-dino/OwneetOS/daemon/internal/atomicfile"
	"github.com/fancy-dino/OwneetOS/daemon/internal/gamedata"
)

// A console of the user's: its emulators and its games together, in a folder "Emulation" at the top
// of a disk ([[step-4.9-the-users-own-emulators]], approved demo 4.9b). It moves as a game does, to the
// "Emulation" folder of the other disk; it is never put beside one of the same name, nor into it.

// EmulationFolders are the names of that folder, in any case (owneet-frontend's
// Emulators::baseNames). An accent may be written as a letter and a sign ("É" in two).
var EmulationFolders = []string{"emulation", "emulazione", "émulation", "emulación", "emulacion"}

// ErrTaken: the disk has a console of that name already.
var ErrTaken = errors.New("a console with that name is on that disk already")

var joinAccents = strings.NewReplacer("é", "é", "ó", "ó")

func isEmulationFolder(name string) bool {
	name = joinAccents.Replace(strings.ToLower(name))
	for _, e := range EmulationFolders {
		if name == e {
			return true
		}
	}
	return false
}

// isConsole: the folder is in a folder "Emulation"
func isConsole(dir string) bool { return isEmulationFolder(filepath.Base(filepath.Dir(dir))) }

// folder is where an item goes on a disk (root): the games folder, or for a console the folder
// "Emulation": the one the disk has, or a new one, with the name it had.
func folder(root, dir string) string {
	is, name := isGamesFolder, "Games"
	if isConsole(dir) {
		is, name = isEmulationFolder, filepath.Base(filepath.Dir(dir))
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.IsDir() && is(e.Name()) {
			return filepath.Join(root, e.Name())
		}
	}
	return filepath.Join(root, name)
}

// taken: a name the folder has already, in any case (a disk may not tell the cases apart)
func taken(dest, name string) bool {
	entries, _ := os.ReadDir(dest)
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) {
			return true
		}
	}
	return false
}

// emulatorData are the folders on the console's disk of the emulators under a console's folder
// (owneet-emulator-run: what it unpacked, and the file "source" that says for which emulator):
// the emulator's path → its folder.
func (m *Manager) emulatorData(console string) map[string]string {
	out := map[string]string{}
	sources, _ := filepath.Glob(filepath.Join(m.Home, ".local", "share", "owneet", "emulators", "*", "source"))
	for _, s := range sources {
		data, err := os.ReadFile(s)
		if err == nil && within(string(data), console) {
			out[string(data)] = filepath.Dir(s)
		}
	}
	return out
}

// consoleOwnSize is the bytes a copied console's own folders take (what its archives left).
func (m *Manager) consoleOwnSize(console string) int64 {
	var n int64
	for _, dir := range m.emulatorData(console) {
		n += size(filepath.Join(dir, "open"))
	}
	return n
}

// carryConsole gives a console moved from one place to another what was unpacked for its emulators
// (an emulator opened from an archive may keep its saves there). Copied (moved false): a copy of what
// its archives left; an AppImage is unpacked again by itself. Nothing at the new place is replaced.
func (m *Manager) carryConsole(title, from, to string, moved bool) error {
	for emulator, old := range m.emulatorData(from) {
		place := to + strings.TrimPrefix(emulator, from)
		now := gamedata.Dir(m.Home, "emulators", place)
		if _, err := os.Lstat(now); err == nil {
			m.Log.Warn("an emulator's own folder not carried: the new place has one already", "console", title, "to", now)
			continue
		}
		if moved {
			if err := os.Rename(old, now); err != nil {
				m.Log.Warn("an emulator's own folder not moved", "console", title, "err", err)
				continue
			}
		} else {
			if _, err := os.Stat(filepath.Join(old, "open")); err != nil {
				continue
			}
			stop := func() error {
				if m.isAborting() {
					return errStopped
				}
				return nil
			}
			if err := copyFiles(filepath.Join(old, "open"), filepath.Join(now, "open"), stop, nil); err != nil {
				os.RemoveAll(now)
				return err
			}
		}
		// its program's path was inside the old folder: found again at the next start
		os.Remove(filepath.Join(now, "stamp"))
		if err := os.WriteFile(filepath.Join(now, "source"), []byte(place), 0o644); err != nil {
			m.Log.Warn("an emulator's own folder: cannot say whose it is", "console", title, "err", err)
		}
		m.Log.Info("an emulator's own folder goes with its console", "console", title, "from", old, "to", now)
	}
	return nil
}

// keepConsoleChoices gives a moved or copied console what was chosen for it (owneet-frontend's
// emulators.json: the emulator, how it starts, the files): the entry's name and the paths in it follow.
func (m *Manager) keepConsoleChoices(title, from, to string, moved bool) {
	path := filepath.Join(m.Home, ".config", "owneet-frontend", "emulators.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return // nothing chosen yet
	}
	var all map[string]map[string]any
	if err := json.Unmarshal(data, &all); err != nil {
		m.Log.Warn("what was chosen for the console not kept: the choices cannot be read", "console", title, "err", err)
		return
	}
	choices, ok := all[from]
	if !ok {
		return
	}
	follow := func(p string) string {
		if within(p, from) {
			return to + strings.TrimPrefix(p, from)
		}
		return p
	}
	copied := map[string]any{}
	for k, v := range choices {
		copied[k] = v
	}
	if files, ok := choices["files"].(map[string]any); ok {
		now := map[string]any{}
		for game, file := range files {
			if f, ok := file.(string); ok {
				now[follow(game)] = follow(f)
			}
		}
		copied["files"] = now
	}
	all[to] = copied
	if moved {
		delete(all, from)
	}
	out, err := json.MarshalIndent(all, "", "    ")
	if err == nil {
		err = atomicfile.Write(path, append(out, '\n'), 0o644)
	}
	if err != nil {
		m.Log.Warn("what was chosen for the console not kept", "console", title, "err", err)
	}
}
