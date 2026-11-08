// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/diskprep"
)

// The disks the console prepared for games are Steam libraries too ([[step-6.7-preparing-other-disks]],
// owner, 2026-11-08): Steam's interface for the controller has no way to add a disk (on SteamOS the
// system tells Steam). As SteamOS's own mounter does (steamos-automount.sh), each one mounted
// read-write is handed to the running Steam as steam://addlibraryfolder/<folder>, every byte of the
// folder written as %XX: the disk's SteamLibrary folder (made here), or the disk itself when Steam
// already keeps a library there (libraryfolder.vdf). Steam then lists it in its storage and among the
// places to install a game. Once per disk while Steam runs, and never for a folder Steam lists already.

// steamDisksAfter: the disks are handed to a Steam whose window has not been seen after this long
// (the cage session, where owneetd does not follow windows). A Steam nobody has signed in to showed
// nothing of it (test VM, 2026-11-08).
const steamDisksAfter = time.Minute

// MountStateDir is where owneet-mount writes what it mounted: one file per partition, its folder,
// "rw" or "ro", and why read only.
const MountStateDir = "/run/owneet-mount"

// ext4Magic is ext4's file system type, as statfs gives it.
const ext4Magic = 0xEF53

// ext4At tells whether a folder is on an ext4 file system (tests replace it).
var ext4At = func(dir string) bool {
	var fs syscall.Statfs_t
	return syscall.Statfs(dir, &fs) == nil && fs.Type == ext4Magic
}

// preparedForGames: what owneet-prepare-disk leaves on a disk: ext4, its top folder owned by the
// console's user, and the Games folder in one of its names.
func preparedForGames(dir string, uid int) bool {
	if !ext4At(dir) {
		return false
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return false
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); !ok || int(sys.Uid) != uid {
		return false
	}
	for _, f := range diskprep.Folders {
		if fi, err := os.Stat(filepath.Join(dir, f)); err == nil && fi.IsDir() {
			return true
		}
	}
	return false
}

// gameDisks: the disks prepared for games, mounted read-write, from owneet-mount's state.
func gameDisks(stateDir string, uid int) []string {
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(stateDir, e.Name()))
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		if len(lines) < 2 || lines[1] != "rw" || !filepath.IsAbs(lines[0]) {
			continue // also the lists of disks being installed on or prepared
		}
		if dir := filepath.Clean(lines[0]); preparedForGames(dir, uid) {
			out = append(out, dir)
		}
	}
	return out
}

// steamURLPath: every byte as %XX, as SteamOS's mounter writes it.
func steamURLPath(path string) string {
	var b strings.Builder
	for i := 0; i < len(path); i++ {
		fmt.Fprintf(&b, "%%%02x", path[i])
	}
	return b.String()
}

// steamAddDisks hands Steam the disks prepared for games it does not list yet. Called by watchSteam
// while Steam's window is ready.
func (m *Manager) steamAddDisks() {
	disks := gameDisks(m.MountState, os.Getuid())
	if len(disks) == 0 {
		return
	}
	listed := map[string]bool{}
	for _, f := range steamFolders(m.SteamDir) {
		listed[filepath.Clean(f.path)] = true
	}
	var sess Session
	haveSess := false
	for _, disk := range disks {
		lib := filepath.Join(disk, "SteamLibrary")
		if _, err := os.Stat(filepath.Join(disk, "libraryfolder.vdf")); err == nil {
			lib = disk
		}
		m.steamMu.Lock()
		sent := m.steamDisks[disk]
		if !sent {
			m.steamDisks[disk] = true
		}
		m.steamMu.Unlock()
		if sent || listed[lib] {
			continue
		}
		if lib != disk {
			if err := os.MkdirAll(lib, 0o755); err != nil {
				m.Log.Warn("cannot make Steam's folder on a disk for games", "dir", lib, "err", err)
				continue
			}
		}
		if !haveSess {
			s, err := m.Session()
			if err != nil {
				return
			}
			sess, haveSess = s, true
		}
		m.Log.Info("a disk for games handed to Steam", "dir", lib)
		_ = m.forwardSteam("steam://addlibraryfolder/"+steamURLPath(lib), sess)
	}
}
