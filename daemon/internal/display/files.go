// SPDX-License-Identifier: GPL-3.0-or-later

package display

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Mode is a screen mode as gamescope lists it: "1920x1080@60".
var modePattern = regexp.MustCompile(`^[0-9]{2,5}x[0-9]{2,5}@[0-9]{1,3}$`)

// ValidMode tells whether m looks like a mode.
func ValidMode(m string) bool { return modePattern.MatchString(m) }

// ModeFile is gamescope's GAMESCOPE_MODE_SAVE_FILE: one "Description:WxH@Hz" line per screen
// (gamescope reads the description up to the first ":").
type ModeFile struct{ Path string }

// Get returns the mode saved for a screen, or "" (gamescope then picks the screen's preferred one).
func (f ModeFile) Get(desc string) string {
	for _, line := range readLines(f.Path) {
		d, m, ok := strings.Cut(line, ":")
		if fields := strings.Fields(m); ok && d == desc && len(fields) > 0 { // "WxH@Hz [broadcast RGB]"
			return fields[0]
		}
	}
	return ""
}

// Set saves a mode for a screen; "" forgets it.
func (f ModeFile) Set(desc, mode string) error {
	if mode != "" && !ValidMode(mode) {
		return fmt.Errorf("invalid mode %q", mode)
	}
	var out []string
	for _, line := range readLines(f.Path) {
		if d, _, ok := strings.Cut(line, ":"); ok && d == desc {
			continue
		}
		out = append(out, line)
	}
	if mode != "" {
		out = append(out, desc+":"+mode)
	}
	return writeLines(f.Path, out)
}

// Prefs is display.conf, read by owneet-session when the compositor starts: OUTPUT (gamescope's
// --prefer-output), VK_DEVICE (--prefer-vk-device), KEYBOARD (XKB_DEFAULT_LAYOUT).
type Prefs struct{ Path string }

var prefValue = regexp.MustCompile(`^[A-Za-z0-9:_-]*$`)

// Read returns the saved values.
func (p Prefs) Read() map[string]string {
	out := map[string]string{}
	for _, line := range readLines(p.Path) {
		if k, v, ok := strings.Cut(line, "="); ok && prefValue.MatchString(v) {
			out[k] = v
		}
	}
	return out
}

// Write replaces the saved values with these ("" values are left out).
func (p Prefs) Write(values map[string]string) error {
	keys := make([]string, 0, len(values))
	for k, v := range values {
		if !prefValue.MatchString(v) || !prefValue.MatchString(k) {
			return fmt.Errorf("invalid setting %s=%q", k, v)
		}
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	lines := []string{"# Written by owneetd (Settings → Display); read by owneet-session."}
	for _, k := range keys {
		lines = append(lines, k+"="+values[k])
	}
	return writeLines(p.Path, lines)
}

// Update changes some values and keeps the others.
func (p Prefs) Update(changes map[string]string) error {
	values := p.Read()
	for k, v := range changes {
		values[k] = v
	}
	return p.Write(values)
}

func readLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out
}

func writeLines(path string, lines []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	data := strings.Join(lines, "\n")
	if data != "" {
		data += "\n"
	}
	if err := os.WriteFile(tmp, []byte(data), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
