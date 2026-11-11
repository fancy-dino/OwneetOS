// SPDX-License-Identifier: GPL-3.0-or-later

package player

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The player's bindings (daemon/player/input.conf) are "Inside the player" in
// [[controller-input-map]]: mpv never binds Guide (GAMEPAD_MENU, owneetd's), nor View, LB / RB or
// the sticks, nor Q / E or the Super key; mpv's own bindings are off; every binding goes to the
// script, which knows each name.
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
	never := regexp.MustCompile(`(?i)GAMEPAD_MENU|GAMEPAD_BACK|SHOULDER|GAMEPAD_(LEFT|RIGHT)_STICK_|^(q|e|META|SUPER)$|\bMETA\+|\bSUPER\+`)
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
