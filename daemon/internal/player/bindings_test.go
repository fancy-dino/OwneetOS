// SPDX-License-Identifier: GPL-3.0-or-later

package player

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The player's bindings (daemon/player/input.conf) are "Inside the player" in
// [[controller-input-map]]: mpv never binds Guide (GAMEPAD_MENU, owneetd's), nor LB / RB or the
// sticks, nor Q / E or the Super key; mpv's own bindings are off; every binding goes to the script,
// which knows each name; View opens the command legend (step 5.6a).
func TestBindings(t *testing.T) {
	data, err := os.ReadFile("../../player/input.conf")
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile("../../player/owneet.lua")
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^bind\("([a-z0-9-]+)"`).FindAllStringSubmatch(string(script), -1) {
		known[m[1]] = true
	}
	bound := map[string]string{}
	for n, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if line == "CLOSE_WIN quit" { // owneetd's close and the window's: quit, saving the position
			bound["CLOSE_WIN"] = "quit"
			continue
		}
		if len(f) != 3 || f[1] != "script-binding" || !strings.HasPrefix(f[2], "owneet/") {
			t.Errorf("line %d: %q is not `KEY script-binding owneet/NAME`", n+1, line)
			continue
		}
		if !known[strings.TrimPrefix(f[2], "owneet/")] {
			t.Errorf("line %d: owneet.lua has no binding %q", n+1, f[2])
		}
		if _, twice := bound[f[0]]; twice {
			t.Errorf("line %d: %s bound twice", n+1, f[0])
		}
		bound[f[0]] = strings.TrimPrefix(f[2], "owneet/")
	}
	never := regexp.MustCompile(`(?i)GAMEPAD_MENU|SHOULDER|GAMEPAD_(LEFT|RIGHT)_STICK_|^(q|e|META|SUPER)$|\bMETA\+|\bSUPER\+`)
	for key := range bound {
		if never.MatchString(key) {
			t.Errorf("%s must never be bound in the player", key)
		}
	}
	want := map[string]string{
		"GAMEPAD_ACTION_DOWN": "play-pause", "GAMEPAD_DPAD_LEFT": "back-10", "GAMEPAD_DPAD_RIGHT": "forward-10",
		"GAMEPAD_LEFT_TRIGGER": "previous", "GAMEPAD_RIGHT_TRIGGER": "next", "GAMEPAD_ACTION_LEFT": "audio",
		// Y Autoplay as in the Videos page, Menu the subtitles (owner, 2026-11-11)
		"GAMEPAD_ACTION_UP": "autoplay", "GAMEPAD_START": "subtitles", "GAMEPAD_ACTION_RIGHT": "leave",
		"GAMEPAD_DPAD_UP": "show", "GAMEPAD_DPAD_DOWN": "show", "GAMEPAD_LEFT_STICK": "show", "GAMEPAD_RIGHT_STICK": "show",
		"GAMEPAD_BACK": "legend",
		// the keyboard fallback: the same meanings
		"ENTER": "play-pause", "SPACE": "play-pause", "LEFT": "back-10", "RIGHT": "forward-10", "z": "previous",
		"c": "next", "x": "audio", "y": "autoplay", "m": "subtitles", "ESC": "leave", "BS": "leave",
		"CLOSE_WIN": "quit",
	}
	for key, name := range want {
		if bound[key] != name {
			t.Errorf("%s: %q, want %q", key, bound[key], name)
		}
	}

	conf, err := os.ReadFile("../../player/mpv.conf")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"input-default-bindings=no", "input-builtin-bindings=no", "input-gamepad=yes",
		"osc=no", "ytdl=no", "load-scripts=no", "scripts=~~/owneet.lua", "watch-later-options=start",
		"write-filename-in-watch-later-config=yes", "hr-seek=yes"} {
		if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(line) + `$`).Match(conf) {
			t.Errorf("mpv.conf lacks %q", line)
		}
	}
	// mpv quits on SIGTERM (owneetd's close) only with its terminal input: without it SDL takes
	// the signal and mpv keeps playing (the owner's test, 2026-11-11)
	if regexp.MustCompile(`(?m)^(no-)?input-terminal(=no)?$`).Match(conf) {
		t.Error("mpv.conf turns off input-terminal: mpv would ignore owneetd's close")
	}
}

// The command legend of the player ([[step-5.6a-command-legend]]) says what input.conf binds:
// every button of the legends of a video and of a song does something there (not only showing
// the display), and the video's names every button that does. The legends live in the theme
// (legends.js, one source with the web apps'); the package build has only daemon/, so there it
// is skipped (tools/lint runs it).
func TestLegendMatchesBindings(t *testing.T) {
	src, err := os.ReadFile("../../../frontend/owneet-frontend/src/themes/owneet/foundation/legends.js")
	if os.IsNotExist(err) {
		t.Skip("the theme is not here (package build)")
	}
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	var legends map[string][][]json.RawMessage
	if start < 0 || end < start || json.Unmarshal([]byte(s[start:end+1]), &legends) != nil {
		t.Fatal("legends.js: the object is not strict JSON")
	}
	conf, err := os.ReadFile("../../player/input.conf")
	if err != nil {
		t.Fatal(err)
	}
	bound := map[string]string{}
	for _, line := range strings.Split(string(conf), "\n") {
		if f := strings.Fields(line); len(f) == 3 && strings.HasPrefix(f[0], "GAMEPAD_") {
			bound[f[0]] = strings.TrimPrefix(f[2], "owneet/")
		}
	}
	// the legend's button names → mpv's
	names := map[string][]string{"a": {"GAMEPAD_ACTION_DOWN"}, "b": {"GAMEPAD_ACTION_RIGHT"}, "x": {"GAMEPAD_ACTION_LEFT"},
		"y": {"GAMEPAD_ACTION_UP"}, "menu": {"GAMEPAD_START"}, "lt": {"GAMEPAD_LEFT_TRIGGER"}, "rt": {"GAMEPAD_RIGHT_TRIGGER"},
		"dpad": {"GAMEPAD_DPAD_LEFT", "GAMEPAD_DPAD_RIGHT"}}
	buttons := func(context string) map[string]bool {
		out := map[string]bool{}
		for _, row := range legends[context] {
			var bs []string
			if len(row) == 2 && json.Unmarshal(row[0], &bs) == nil {
				for _, b := range bs {
					out[b] = true
				}
			}
		}
		return out
	}
	video, song := buttons("player.video"), buttons("player.song")
	if len(video) == 0 || len(song) == 0 {
		t.Fatal("no legend for the player")
	}
	for context, bs := range map[string]map[string]bool{"player.video": video, "player.song": song} {
		for b := range bs {
			if len(names[b]) == 0 {
				t.Errorf("%s: %q is not a button of the player", context, b)
			}
			for _, key := range names[b] {
				if a := bound[key]; a == "" || a == "show" {
					t.Errorf("%s: %s (%s) does nothing in input.conf", context, b, key)
				}
			}
		}
	}
	for b, keys := range names {
		for _, key := range keys {
			if a := bound[key]; a != "" && a != "show" && !video[b] {
				t.Errorf("input.conf binds %s (%s) to %s: the video's legend does not say so", key, b, a)
			}
		}
	}
	// a song: X, Y and Menu only show the display (owneet.lua), so its legend leaves them out
	for _, b := range []string{"x", "y", "menu"} {
		if song[b] {
			t.Errorf("the song's legend names %q, which does nothing for a song", b)
		}
	}
}
