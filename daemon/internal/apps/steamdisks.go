// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/fancy-dino/OwneetOS/daemon/internal/diskprep"
)

// The disks the console prepared for games are Steam libraries too ([[step-6.7-preparing-other-disks]],
// owner, 2026-11-09): Steam's interface for the controller has no way to add a disk, and outside SteamOS
// Steam ignores steam://addlibraryfolder (FrankenPC, 2026-11-09). So, while Steam is closed, right before
// the console starts it, owneetd writes each one mounted read-write into Steam's own list of libraries
// (steamapps/libraryfolders.vdf), as Steam itself writes them: the disk's SteamLibrary folder with its
// libraryfolder.vdf and steamapps, or the disk itself when Steam already keeps a library at its top. Each
// one is labelled with the disk's name: Steam shows the label instead of the folder (no technical name,
// [[product-invariants]]).
// Steam reads the list when it starts. A disk plugged in while Steam runs is added at Steam's next start.
// A folder Steam lists already is left as it is.

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
	sort.Strings(out)
	return out
}

// libraryContentID: the contentid of a library folder (its libraryfolder.vdf), "" when it has none.
func libraryContentID(lib string) string {
	data, err := os.ReadFile(filepath.Join(lib, "libraryfolder.vdf"))
	if err != nil {
		return ""
	}
	id, _ := kvGet(kvText(string(data)), "libraryfolder", "contentid").(string)
	return id
}

// newContentID: a random number, as Steam gives each library folder.
func newContentID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return strconv.FormatUint(binary.LittleEndian.Uint64(b[:])>>1, 10)
}

// makeLibrary makes a library folder Steam can use: steamapps, and libraryfolder.vdf with its
// contentid (kept when it has one) and its label. It returns the contentid.
func makeLibrary(lib, label string) (string, error) {
	if err := os.MkdirAll(filepath.Join(lib, "steamapps"), 0o755); err != nil {
		return "", err
	}
	if id := libraryContentID(lib); id != "" {
		return id, nil
	}
	id := newContentID()
	if id == "" {
		return "", fmt.Errorf("no random number for the library")
	}
	text := fmt.Sprintf("\"libraryfolder\"\n{\n\t\"contentid\"\t\t%q\n\t\"label\"\t\t%s\n}\n", id, vdfString(label))
	if err := replaceFile(filepath.Join(lib, "libraryfolder.vdf"), []byte(text)); err != nil {
		return "", err
	}
	return id, nil
}

// vdfString: a string as Steam's text files write it (backslash and quote escaped).
func vdfString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// addLibraries adds library folders to the text of libraryfolders.vdf, each as the next numbered
// block with its contentid and label; false when the root block is not there (not the file we know).
func addLibraries(text string, libs, ids, labels []string) (string, bool) {
	root, ok := kvGet(kvText(text), "libraryfolders").(map[string]any)
	if !ok {
		return text, false
	}
	next := 0
	for k := range root {
		if n, err := strconv.Atoi(k); err == nil && n >= next {
			next = n + 1
		}
	}
	end := strings.LastIndex(text, "}")
	if end < 0 {
		return text, false
	}
	var b strings.Builder
	for i, lib := range libs {
		fmt.Fprintf(&b, "\t\"%d\"\n\t{\n\t\t\"path\"\t\t%s\n\t\t\"label\"\t\t%s\n\t\t\"contentid\"\t\t%q\n"+
			"\t\t\"totalsize\"\t\t\"0\"\n\t\t\"apps\"\n\t\t{\n\t\t}\n\t}\n", next+i, vdfString(lib), vdfString(labels[i]), ids[i])
	}
	head := strings.TrimRight(text[:end], " \t")
	if !strings.HasSuffix(head, "\n") {
		head += "\n"
	}
	return head + b.String() + text[end:], true
}

// listedLibraries: the folders a libraryfolders.vdf lists.
func listedLibraries(text string) map[string]bool {
	listed := map[string]bool{}
	root, _ := kvGet(kvText(text), "libraryfolders").(map[string]any)
	for _, v := range root {
		if f, ok := v.(map[string]any); ok {
			if p, _ := kvGet(f, "path").(string); p != "" {
				listed[filepath.Clean(p)] = true
			}
		}
	}
	return listed
}

// steamLibraryDisks writes the disks prepared for games that Steam does not list yet into Steam's
// list of libraries. Steam keeps two copies of it, steamapps/ (read first when it starts) and config/
// (test VM, 2026-11-09): both get them. Steam must be closed (it writes them itself while it runs). It
// returns the folders added.
func steamLibraryDisks(steamDir, stateDir string, uid int) ([]string, error) {
	disks := gameDisks(stateDir, uid)
	if len(disks) == 0 {
		return nil, nil
	}
	type list struct {
		file, text string
		listed     map[string]bool
	}
	var lists []list
	for _, sub := range []string{"steamapps", "config"} {
		file := filepath.Join(steamDir, sub, "libraryfolders.vdf")
		if data, err := os.ReadFile(file); err == nil {
			lists = append(lists, list{file, string(data), listedLibraries(string(data))})
		}
	}
	if len(lists) == 0 {
		return nil, nil // Steam has never run: it makes the list first
	}
	var added []string
	ids := map[string]string{}
	for i := range lists {
		var libs, libIDs, labels []string
		for _, disk := range disks {
			lib := filepath.Join(disk, "SteamLibrary")
			if _, err := os.Stat(filepath.Join(disk, "libraryfolder.vdf")); err == nil {
				lib = disk
			}
			if lists[i].listed[lib] || lists[i].listed[disk] {
				continue
			}
			if ids[lib] == "" {
				id, err := makeLibrary(lib, filepath.Base(disk))
				if err != nil {
					return added, fmt.Errorf("making %s: %w", lib, err)
				}
				ids[lib] = id
				added = append(added, lib)
			}
			// Steam shows the label, not the folder: the disk's name (its folder under /run/media)
			libs, libIDs, labels = append(libs, lib), append(libIDs, ids[lib]), append(labels, filepath.Base(disk))
		}
		if len(libs) == 0 {
			continue
		}
		text, ok := addLibraries(lists[i].text, libs, libIDs, labels)
		if !ok {
			return added, fmt.Errorf("%s is not the file we know", lists[i].file)
		}
		if err := replaceFile(lists[i].file, []byte(text)); err != nil {
			return added, err
		}
	}
	return added, nil
}
