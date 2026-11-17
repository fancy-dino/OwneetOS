// SPDX-License-Identifier: GPL-3.0-or-later

package installs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/fancy-dino/OwneetOS/daemon/internal/gamedata"
)

// Games on any disk ([[step-6.8-games-on-disk-delete-and-progress]]): a game's folder or an
// installer deleted from its disk, a game uninstalled wherever it was moved to.

// ErrReadOnly: the disk is read only (e.g. NTFS left by Windows, [[disks]]): nothing is deleted.
var ErrReadOnly = errors.New("the disk is read only")

// mediaRoot is where the console mounts the disks (owneet-mount).
func (m *Manager) mediaRoot() string {
	if m.Media != "" {
		return filepath.Clean(m.Media)
	}
	return "/run/media/owneet"
}

// gameFolder checks that dest is a game's folder: a folder, not a link, in a games folder at the top
// of the home folder or of a disk the console mounted.
func (m *Manager) gameFolder(dest string) (string, error) {
	dest = filepath.Clean(dest)
	if !filepath.IsAbs(dest) {
		return "", ErrInvalid
	}
	name, parent := filepath.Base(dest), filepath.Dir(dest)
	if name == "" || strings.HasPrefix(name, ".") {
		return "", ErrInvalid
	}
	games := false
	for _, g := range GamesFolders {
		games = games || strings.EqualFold(filepath.Base(parent), g)
	}
	root := filepath.Dir(parent)
	if !games || (root != filepath.Clean(m.Home) && filepath.Dir(root) != m.mediaRoot()) {
		return "", ErrInvalid
	}
	st, err := os.Lstat(dest)
	if err == nil && st.Mode()&os.ModeSymlink != 0 {
		return "", ErrInvalid
	}
	if err != nil || !st.IsDir() {
		return "", ErrNotFound
	}
	if real, err := filepath.EvalSymlinks(dest); err != nil || real != dest {
		return "", ErrInvalid
	}
	return dest, nil
}

// readOnly: the file system holding path is mounted read only.
func readOnly(path string) bool {
	var st syscall.Statfs_t
	return syscall.Statfs(path, &st) == nil && st.Flags&1 != 0 // ST_RDONLY
}

// busy: an install works in the folder (its game, or its installer is there).
func (m *Manager) busy(dir string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.queue {
		if j.Dest == dir || within(j.Dest, dir) || within(filepath.Clean(j.req.Installer), dir) {
			return true
		}
	}
	return false
}

func (m *Manager) deletable(dir string) error {
	if readOnly(dir) {
		return ErrReadOnly
	}
	if m.busy(dir) {
		return ErrExists
	}
	// never under a running game (FrankenPC, 2026-10-19: ScummVM lost its files and crashed)
	if m.InUse(dir) {
		return ErrInUse
	}
	return nil
}

type installedMark struct {
	Installer string   `json:"installer"`
	Keep      []string `json:"keep"`
	Link      string   `json:"link"`
}

func readMark(dest string) (installedMark, map[string]any, error) {
	var mark installedMark
	raw := map[string]any{}
	data, err := os.ReadFile(filepath.Join(dest, MarkInstalled))
	if err != nil {
		return mark, raw, err
	}
	_ = json.Unmarshal(data, &mark)
	_ = json.Unmarshal(data, &raw)
	return mark, raw, nil
}

// Delete deletes a game's folder from its disk, with everything in it (saves kept there too): a game
// ready to start, a folder with only an installer, or a game the console installed.
func (m *Manager) Delete(dest string) error {
	m.init()
	dest, err := m.gameFolder(dest)
	if err != nil {
		return err
	}
	if err := m.deletable(dest); err != nil {
		return err
	}
	mark, _, _ := readMark(dest)
	err = os.RemoveAll(dest)
	m.synced(filepath.Dir(dest))
	if err != nil {
		return err
	}
	if mark.Link != "" && within(filepath.Clean(mark.Link), m.linksDir()) {
		os.Remove(mark.Link)
	}
	m.Log.Info("game deleted from its disk", "dest", dest)
	// the game and its installer both gone (the installer was in the folder, or none is left)
	if !installerLeft(dest, mark.Installer) {
		m.removeData(dest)
	}
	return nil
}

// installerLeft: the installer of a game the console installed is still there (in its folder after
// an uninstall, or elsewhere): the game may be installed again, its own folders stay.
func installerLeft(dest, installer string) bool {
	if installer == "" {
		return false
	}
	_, err := os.Stat(filepath.Clean(installer))
	return err == nil
}

// removeData: a game that leaves an internal disk for good (it and its installer removed through the
// console) leaves nothing behind (owner, 2026-11-12, [[disks]]): its own folders on the console's disk
// go too, its prefix with its saves included. On an external disk (USB) they stay: the user removes
// them with the file manager (6.11).
func (m *Manager) removeData(dest string) {
	if m.External(dest) {
		return
	}
	removed := false
	for _, dir := range gamedata.All(m.Home, dest) {
		if _, err := os.Lstat(dir); err != nil {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			m.Log.Warn("the game's own folder could not be removed", "dest", dest, "dir", dir, "err", err)
			continue
		}
		removed = true
		m.Log.Info("the game's own folder removed with it", "dest", dest, "dir", dir)
	}
	if removed {
		m.synced(filepath.Join(m.Home, ".local", "share", "owneet"))
	}
}

// installerFiles: an installer and its parts (GOG's setup_*.exe with its setup_*-N.bin).
func installerFiles(installer string) []string {
	files := []string{installer}
	base := strings.TrimSuffix(filepath.Base(installer), filepath.Ext(installer))
	parts, _ := filepath.Glob(filepath.Join(filepath.Dir(installer), globEscape(base)+"-*.bin"))
	return append(files, parts...)
}

// DeleteInstaller deletes the installer of a game the console installed; the game stays. An installer
// in the game's own folder goes alone (with its parts); one in a folder of its own (e.g. on another
// disk) goes with that folder.
func (m *Manager) DeleteInstaller(dest string) error {
	m.init()
	dest, err := m.gameFolder(dest)
	if err != nil {
		return err
	}
	mark, raw, err := readMark(dest)
	if err != nil || mark.Installer == "" {
		return ErrNotFound
	}
	installer := filepath.Clean(mark.Installer)
	if within(installer, dest) {
		if err := m.deletable(dest); err != nil {
			return err
		}
		gone := map[string]bool{}
		for _, f := range installerFiles(installer) {
			if filepath.Dir(f) != dest {
				continue // only the files at the top of the folder are the installer's
			}
			if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
				m.synced(dest)
				return err
			}
			gone[filepath.Base(f)] = true
		}
		m.synced(dest)
		var keep []string
		for _, k := range mark.Keep {
			if !gone[k] {
				keep = append(keep, k)
			}
		}
		raw["keep"] = keep
	} else {
		// the installer's own game folder: the installer's folder, or the one above it in a games folder
		folder := filepath.Dir(installer)
		for i := 0; i < 8; i++ {
			if f, err := m.gameFolder(folder); err == nil {
				folder = f
				break
			}
			folder = filepath.Dir(folder)
		}
		if _, err := m.gameFolder(folder); err != nil || folder == dest {
			return ErrInvalid
		}
		if err := m.deletable(folder); err != nil {
			return err
		}
		err := os.RemoveAll(folder)
		m.synced(filepath.Dir(folder))
		if err != nil {
			return err
		}
	}
	raw["installer"] = ""
	if data, err := json.Marshal(raw); err == nil {
		_ = os.WriteFile(filepath.Join(dest, MarkInstalled), data, 0o644)
	}
	m.Log.Info("installer deleted", "game", dest, "installer", installer)
	return nil
}
