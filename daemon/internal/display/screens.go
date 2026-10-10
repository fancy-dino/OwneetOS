// SPDX-License-Identifier: GPL-3.0-or-later

// Package display is Settings → Display (roadmap 3.9, part 3): the screens connected to the
// console and the one it uses, the screen's resolution and refresh rate, brightness where the
// screen allows it, and the layout of a physical keyboard. The console runs in gamescope, which
// drives one screen and turns off the others on its graphics card; owneetd tells it which screen
// and which mode to use the same way Steam does on SteamOS:
//   - the screen: gamescope's --prefer-output and --prefer-vk-device, read from display.conf by
//     owneet-session when gamescope starts (changing it restarts gamescope);
//   - the mode: GAMESCOPE_MODE_SAVE_FILE ("Make Model:WxH@Hz" lines), then the
//     GAMESCOPE_DISPLAY_MODE_NUDGE root property makes gamescope apply it at once; the screen's
//     modes come from GAMESCOPE_DISPLAY_MODE_LIST_EXTERNAL (TVs and monitors only: gamescope keeps
//     a laptop's built-in screen at its native mode);
//   - the keyboard: GAMESCOPE_KEYBOARD_LAYOUT, and XKB_DEFAULT_LAYOUT at the next start.
//
// Screens on another graphics card are turned off by owneet-screens-off (a small root service).
// PROJECT_RULES.md, decision log 2026-10-10.
package display

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Screen is a connected screen.
type Screen struct {
	ID        string `json:"id"`        // the DRM connector in sysfs, e.g. "card1-HDMI-A-1"
	Connector string `json:"connector"` // e.g. "HDMI-A-1"
	Name      string `json:"name"`      // the product name from the screen (EDID), or ""
	Internal  bool   `json:"internal"`  // a laptop's built-in screen
	Active    bool   `json:"active"`    // the one the console uses

	card        string // "card1"
	gpu         string // "vendor:device" of its graphics card, for --prefer-vk-device
	description string // gamescope's name for it, the key of the saved modes
	lit         bool   // sysfs says it is on
}

var connectorDir = regexp.MustCompile(`^(card[0-9]+)-(.+)$`)

// ListScreens reads the connected screens from sysfs (/sys/class/drm). pnps maps PNP ids to
// maker names (hwdata's pnp.ids), as gamescope does.
func ListScreens(sysDRM string, pnps map[string]string) []Screen {
	entries, _ := os.ReadDir(sysDRM)
	var out []Screen
	for _, e := range entries {
		m := connectorDir.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		dir := filepath.Join(sysDRM, e.Name())
		if readTrim(filepath.Join(dir, "status")) != "connected" {
			continue
		}
		s := Screen{ID: e.Name(), Connector: m[2], card: m[1]}
		s.Internal = isInternal(s.Connector)
		s.lit = readTrim(filepath.Join(dir, "enabled")) == "enabled"
		s.gpu = gpuID(filepath.Join(sysDRM, s.card, "device"))
		edid, _ := os.ReadFile(filepath.Join(dir, "edid"))
		pnp, model, _ := ParseEDID(edid)
		maker := pnp
		if name, ok := pnps[pnp]; ok {
			maker = name
		}
		s.Name = model
		if s.Name == "" {
			s.Name = maker
		}
		s.description = description(s.Internal, maker, model)
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func isInternal(connector string) bool {
	for _, p := range []string{"eDP-", "LVDS-", "DSI-"} {
		if strings.HasPrefix(connector, p) {
			return true
		}
	}
	return false
}

// description is the name gamescope gives a screen (DRMBackend.cpp, setup_best_connector).
func description(internal bool, maker, model string) string {
	if internal {
		return "Internal screen"
	}
	return maker + " " + model
}

func gpuID(device string) string {
	vendor := strings.TrimPrefix(readTrim(filepath.Join(device, "vendor")), "0x")
	dev := strings.TrimPrefix(readTrim(filepath.Join(device, "device")), "0x")
	if vendor == "" || dev == "" {
		return ""
	}
	return vendor + ":" + dev
}

func readTrim(path string) string {
	b, _ := os.ReadFile(path)
	return strings.TrimSpace(string(b))
}

// ParseEDID returns the maker's PNP id (three letters) and the product name of an EDID.
func ParseEDID(b []byte) (pnp, model string, ok bool) {
	header := []byte{0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00}
	if len(b) < 128 || !bytes.Equal(b[:8], header) {
		return "", "", false
	}
	id := uint16(b[8])<<8 | uint16(b[9])
	pnp = string([]byte{byte('@' + (id>>10)&0x1f), byte('@' + (id>>5)&0x1f), byte('@' + id&0x1f)})
	// Four 18-byte descriptors; tag 0xFC is the product name, up to 13 characters ended by a
	// line feed and padded with spaces (as libdisplay-info reads it).
	for o := 54; o+18 <= 126; o += 18 {
		d := b[o : o+18]
		if d[0] != 0 || d[1] != 0 || d[3] != 0xfc {
			continue
		}
		text := d[5:18]
		if i := bytes.IndexByte(text, '\n'); i >= 0 {
			text = text[:i]
		}
		model = strings.TrimRight(string(text), " ")
	}
	return pnp, model, true
}

// LoadPNPs reads hwdata's pnp.ids: "ID<tab>Maker" lines.
func LoadPNPs(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if id, name, ok := strings.Cut(sc.Text(), "\t"); ok {
			out[id] = name
		}
	}
	return out
}
