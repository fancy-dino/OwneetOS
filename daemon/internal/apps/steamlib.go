// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// What owneetd reads of Steam's library ([[step-4.2c-steam-without-its-interface]]): the games
// being downloaded (from their appmanifest files) and the games installed on a disk that is not
// connected (from libraryfolders.vdf). The interface lists every owned game itself, from Steam's
// caches ([[frontend-steam-local-files]]).

// Steam's states of an app (StateFlags of its appmanifest).
const (
	stateUninstalled    = 1
	stateUpdateRequired = 2
	stateFullyInstalled = 4
	stateUpdateRunning  = 256
	stateUpdatePaused   = 512
	stateUpdateStarted  = 1024
	stateUninstalling   = 2048
	stateDownloading    = 1 << 20
)

// SteamDownload is a game Steam is installing or updating.
type SteamDownload struct {
	AppID string `json:"appid"`
	// State: "downloading" (a new game, the one Steam downloads now), "updating" (an installed
	// game, now), "queued" (waiting for its turn, or paused by Steam, e.g. without internet)
	State string `json:"state"`
	Done  int64  `json:"done"`  // bytes downloaded and written
	Total int64  `json:"total"` // bytes to download and write
	Rate  int64  `json:"rate"`  // bytes per second, over the last check
}

// steamSample is the last bytes seen of a download, to tell the one Steam works on and its speed.
type steamSample struct {
	done  int64
	at    time.Time
	rate  int64
	moved time.Time // the bytes last grew
}

// steamMoving: a download whose bytes grew this recently is the one Steam works on.
const steamMoving = 5 * time.Second

// kvText reads Steam's text KeyValues ("key" "value", "key" { … }) into nested maps; keys are
// kept as written.
func kvText(data string) map[string]any {
	toks := kvTokens(data)
	i := 0
	var block func() map[string]any
	block = func() map[string]any {
		out := map[string]any{}
		for i < len(toks) {
			t := toks[i]
			i++
			if t == "}" {
				return out
			}
			if i >= len(toks) {
				return out
			}
			if toks[i] == "{" {
				i++
				out[t] = block()
			} else if toks[i] != "}" {
				out[t] = toks[i]
				i++
			}
		}
		return out
	}
	return block()
}

func kvTokens(data string) []string {
	var toks []string
	for i := 0; i < len(data); i++ {
		switch c := data[i]; {
		case c == '{' || c == '}':
			toks = append(toks, string(c))
		case c == '"':
			var b strings.Builder
			for i++; i < len(data) && data[i] != '"'; i++ {
				if data[i] == '\\' && i+1 < len(data) {
					i++
				}
				b.WriteByte(data[i])
			}
			toks = append(toks, b.String())
		case c == '/' && i+1 < len(data) && data[i+1] == '/':
			for i < len(data) && data[i] != '\n' {
				i++
			}
		}
	}
	return toks
}

// kvGet returns a value by path, keys compared without regard to case (Steam writes both).
func kvGet(m map[string]any, path ...string) any {
	var v any = m
	for _, key := range path {
		mm, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = nil
		for k, val := range mm {
			if strings.EqualFold(k, key) {
				v = val
				break
			}
		}
	}
	return v
}

func kvInt(m map[string]any, key string) int64 {
	s, _ := kvGet(m, key).(string)
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// steamFolder is a library folder of libraryfolders.vdf.
type steamFolder struct {
	path  string
	label string
	apps  []string // the apps Steam installed there
}

func steamFolders(dir string) []steamFolder {
	data, err := os.ReadFile(filepath.Join(dir, "steamapps", "libraryfolders.vdf"))
	if err != nil {
		return nil
	}
	root, _ := kvGet(kvText(string(data)), "libraryfolders").(map[string]any)
	var out []steamFolder
	for _, v := range root {
		f, ok := v.(map[string]any)
		if !ok {
			continue
		}
		path, _ := kvGet(f, "path").(string)
		label, _ := kvGet(f, "label").(string)
		var ids []string
		if apps, ok := kvGet(f, "apps").(map[string]any); ok {
			for id := range apps {
				if appIDPattern.MatchString(id) {
					ids = append(ids, id)
				}
			}
		}
		if path != "" {
			out = append(out, steamFolder{path: path, label: label, apps: ids})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// mountName: the name of the disk a path is on, from where disks are mounted
// (/run/media/USER/NAME/…, /media/USER/NAME/…, /mnt/NAME/…); "" when unknown.
func mountName(path string) string {
	parts := strings.Split(strings.Trim(filepath.Clean(path), "/"), "/")
	switch {
	case len(parts) >= 4 && parts[0] == "run" && parts[1] == "media":
		return parts[3]
	case len(parts) >= 3 && parts[0] == "media":
		return parts[2]
	case len(parts) >= 2 && parts[0] == "mnt":
		return parts[1]
	}
	return ""
}

// steamAway: the games installed on a library folder whose disk is not connected, with the
// disk's name (its Steam label, or the name it was mounted with; "" when unknown).
func steamAway(dir string) map[string]string {
	away := map[string]string{}
	for _, f := range steamFolders(dir) {
		if _, err := os.Stat(f.path); err == nil {
			continue
		}
		name := f.label
		if name == "" {
			name = mountName(f.path)
		}
		for _, id := range f.apps {
			away[id] = name
		}
	}
	return away
}

// steamAccount: an account has signed in to Steam on this computer (Steam remembers it).
var loginUser = regexp.MustCompile(`"[0-9]{17}"\s*\{`)

func steamAccount(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "config", "loginusers.vdf"))
	return err == nil && loginUser.Match(data)
}

// steamLicences: when Steam last wrote the account's licences (Unix time, 0 when never): a game
// bought or added since the interface read its games makes it read them again.
func steamLicences(dir string) int64 {
	st, err := os.Stat(filepath.Join(dir, "appcache", "packageinfo.vdf"))
	if err != nil {
		return 0
	}
	return st.ModTime().Unix()
}

// steamDownloads reads the games Steam is installing or updating, on every library folder
// present, in the order Steam works on them as far as the files tell (the one moving first, then
// the oldest request). samples keeps the bytes seen at the last check (updated here).
func steamDownloads(dir string, samples map[string]*steamSample, now time.Time) []SteamDownload {
	type entry struct {
		d     SteamDownload
		since time.Time
	}
	var found []entry
	seen := map[string]bool{}
	for _, lib := range steamLibraries(dir) {
		files, _ := filepath.Glob(filepath.Join(lib, "steamapps", "appmanifest_*.acf"))
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			st, _ := kvGet(kvText(string(data)), "AppState").(map[string]any)
			id, _ := kvGet(st, "appid").(string)
			if !appIDPattern.MatchString(id) || seen[id] {
				continue
			}
			flags := kvInt(st, "StateFlags")
			// being uninstalled, or uninstalled: Steam may leave the manifest with the bytes of a
			// cancelled download (FrankenPC, 2026-10-15), which is not a download
			if flags&(stateUninstalling|stateUninstalled) != 0 {
				continue
			}
			installed := flags&stateFullyInstalled != 0
			busy := flags&(stateUpdateRunning|stateDownloading) != 0
			pending := flags&(stateUpdateRequired|stateUpdateStarted|stateUpdatePaused) != 0
			done := kvInt(st, "BytesDownloaded") + kvInt(st, "BytesStaged")
			total := kvInt(st, "BytesToDownload") + kvInt(st, "BytesToStage")

			s := samples[id]
			if s == nil {
				s = &steamSample{done: done, at: now}
				samples[id] = s
			} else {
				if dt := now.Sub(s.at).Seconds(); dt > 0 && done >= s.done {
					s.rate = int64(float64(done-s.done) / dt)
				}
				if done > s.done {
					s.moved = now
				}
				s.done, s.at = done, now
			}
			moving := busy || (!s.moved.IsZero() && now.Sub(s.moved) < steamMoving)
			var state string
			switch {
			case installed && moving:
				state = "updating"
			case installed: // up to date, or an update waiting (Steam updates it before playing)
				continue
			case moving:
				state = "downloading"
			case pending:
				state = "queued"
			default:
				continue
			}
			seen[id] = true
			rate := s.rate
			if !moving {
				rate = 0
			}
			since := time.Time{}
			if info, err := os.Stat(file); err == nil {
				since = info.ModTime()
			}
			found = append(found, entry{SteamDownload{AppID: id, State: state, Done: done, Total: total, Rate: rate}, since})
		}
	}
	for id := range samples {
		if !seen[id] {
			delete(samples, id)
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if (a.d.State == "queued") != (b.d.State == "queued") {
			return b.d.State == "queued"
		}
		return a.since.Before(b.since)
	})
	out := make([]SteamDownload, 0, len(found))
	for _, e := range found {
		out = append(out, e.d)
	}
	return out
}

// steamManifest: an app's StateFlags, and whether it has an appmanifest at all (Steam has it, or
// is installing it), on the library folders present.
func steamManifest(dir, appid string) (flags int64, ok bool) {
	for _, lib := range steamLibraries(dir) {
		data, err := os.ReadFile(filepath.Join(lib, "steamapps", "appmanifest_"+appid+".acf"))
		if err == nil {
			st, _ := kvGet(kvText(string(data)), "AppState").(map[string]any)
			return kvInt(st, "StateFlags"), true
		}
	}
	return 0, false
}

// Steam Input for every controller (owner, 2026-10-15): with it off (Steam's default for Xbox,
// Switch and generic controllers) games read the controller themselves, and some get it wrong
// (Silksong with an Xbox controller over Bluetooth: X did not attack, RT looked up). The settings
// are the account's, in localconfig.vdf (UserLocalConfigStore, read on the FrankenPC); PlayStation
// controllers keep Steam's default ("on in games without support"), as the owner chose. The console
// turns them on once per account, while Steam is closed (Steam writes the file when it exits), and
// then leaves them to the user's choices in Steam.
var steamInputSettings = []string{"SteamController_XBoxSupport", "SteamController_SwitchSupport", "SteamController_GenericGamepadSupport"}

// steamInputOnce turns Steam Input on for the accounts that never had it done; doneDir keeps one
// mark per account.
func steamInputOnce(dir, doneDir string) (changed []string, err error) {
	files, _ := filepath.Glob(filepath.Join(dir, "userdata", "*", "config", "localconfig.vdf"))
	for _, file := range files {
		account := filepath.Base(filepath.Dir(filepath.Dir(file)))
		mark := filepath.Join(doneDir, "steam-input-"+account)
		if _, err := os.Stat(mark); err == nil {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return changed, err
		}
		text, ok := setTopLevel(string(data), "UserLocalConfigStore", steamInputSettings, "1")
		if !ok {
			continue // not the file we know: leave it alone
		}
		if text != string(data) {
			if err := replaceFile(file, []byte(text)); err != nil {
				return changed, err
			}
		}
		if err := os.MkdirAll(doneDir, 0o755); err != nil {
			return changed, err
		}
		if err := os.WriteFile(mark, nil, 0o644); err != nil {
			return changed, err
		}
		changed = append(changed, account)
	}
	return changed, nil
}

// setTopLevel sets keys of a text KeyValues file's root block (named root) to value, replacing
// the lines that have them and adding the others first in the block; false when the root block
// is not there.
func setTopLevel(text, root string, keys []string, value string) (string, bool) {
	lines := strings.Split(text, "\n")
	open := -1
	for i := 0; i+1 < len(lines); i++ {
		if strings.EqualFold(strings.TrimSpace(lines[i]), `"`+root+`"`) && strings.TrimSpace(lines[i+1]) == "{" {
			open = i + 1
			break
		}
	}
	if open < 0 {
		return text, false
	}
	found := map[string]bool{}
	depth := 1
	for i := open + 1; i < len(lines) && depth > 0; i++ {
		t := strings.TrimSpace(lines[i])
		switch {
		case t == "{":
			depth++
		case t == "}":
			depth--
		case depth == 1:
			toks := kvTokens(t)
			if len(toks) != 2 {
				continue
			}
			for _, k := range keys {
				if strings.EqualFold(toks[0], k) {
					lines[i] = "\t\"" + k + "\"\t\t\"" + value + "\""
					found[k] = true
				}
			}
		}
	}
	var add []string
	for _, k := range keys {
		if !found[k] {
			add = append(add, "\t\""+k+"\"\t\t\""+value+"\"")
		}
	}
	lines = append(lines[:open+1], append(add, lines[open+1:]...)...)
	return strings.Join(lines, "\n"), true
}

// replaceFile writes a file through a temporary one, keeping its permissions.
func replaceFile(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp := path + ".owneet-tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
