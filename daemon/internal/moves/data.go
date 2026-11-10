// SPDX-License-Identifier: GPL-3.0-or-later

package moves

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A game's own folders on the console's disk, named after the game's folder (owneet-frontend's
// DiskGames::gameDataDir): a Windows game's prefix (its saves, its settings, what its installer
// wrote) and a DOS game's data. They go with the game when it moves (owner, 2026-11-10,
// [[step-6.6-moving-games-and-media-between-disks]]); otherwise the moved game would start
// with new, empty ones and the old ones would stay behind.
var dataKinds = []string{"windows/prefixes", "dos"}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

// dataDir is DiskGames::gameDataDir: ~/.local/share/owneet/KIND/<name>-<hash>, the name made of
// the folder's name (a-z, 0-9 and "-", at most 40 characters), the hash the first 10 hex
// digits of the SHA-1 of the game's folder.
func dataDir(home, kind, gameDir string) string {
	slug := notSlug.ReplaceAllString(strings.ToLower(filepath.Base(gameDir)), "-")
	if len(slug) > 40 {
		slug = slug[:40]
	}
	slug = strings.Trim(slug, "-")
	sum := sha1.Sum([]byte(gameDir))
	return filepath.Join(home, ".local", "share", "owneet", kind, slug+"-"+hex.EncodeToString(sum[:])[:10])
}

// installedMark is the mark of a game the console installed (package installs): its "link" is
// the folder's link the installer wrote through (a Windows installer that cannot take the
// folder's name), which the game's own files and settings may name.
const installedMark = ".owneet-installed"

// carry gives a moved game its own folders and its install's link: from is where it was (its
// original removed), to where it is now. Nothing that is already at the new place is replaced.
func (m *Manager) carry(title, from, to string) {
	for _, kind := range dataKinds {
		old, now := dataDir(m.Home, kind, from), dataDir(m.Home, kind, to)
		if _, err := os.Lstat(old); err != nil {
			continue
		}
		if _, err := os.Lstat(now); err == nil {
			m.Log.Warn("the game's own folder not moved: the new place has one already", "game", title, "from", old, "to", now)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(now), 0o755); err != nil {
			m.Log.Warn("the game's own folder not moved", "game", title, "err", err)
			continue
		}
		if err := os.Rename(old, now); err != nil {
			m.Log.Warn("the game's own folder not moved", "game", title, "err", err)
			continue
		}
		m.Log.Info("the game's own folder moved with it", "game", title, "from", old, "to", now)
	}
	data, err := os.ReadFile(filepath.Join(to, installedMark))
	if err != nil {
		return
	}
	var mark struct {
		Link string `json:"link"`
	}
	links := filepath.Join(m.Home, ".local", "share", "owneet", "installs")
	if json.Unmarshal(data, &mark) != nil || mark.Link == "" || filepath.Dir(filepath.Clean(mark.Link)) != links {
		return
	}
	if target, err := os.Readlink(mark.Link); err != nil || filepath.Clean(target) != from {
		return
	}
	if err := os.Remove(mark.Link); err != nil {
		m.Log.Warn("the install's link not moved", "game", title, "err", err)
		return
	}
	if err := os.Symlink(to, mark.Link); err != nil {
		m.Log.Warn("the install's link not moved", "game", title, "err", err)
		return
	}
	m.Log.Info("the install's link follows the game", "game", title, "link", mark.Link, "to", to)
}
