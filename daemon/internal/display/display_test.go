// SPDX-License-Identifier: GPL-3.0-or-later

package display

import (
	"os"
	"path/filepath"
	"testing"
)

// edid builds a minimal EDID with a maker id and a product name descriptor.
func edid(maker string, name string) []byte {
	b := make([]byte, 128)
	copy(b, []byte{0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00})
	id := uint16(maker[0]-'@')<<10 | uint16(maker[1]-'@')<<5 | uint16(maker[2]-'@')
	b[8], b[9] = byte(id>>8), byte(id)
	d := b[72:90]
	d[3] = 0xfc
	text := []byte(name + "\n            ")
	copy(d[5:], text[:13])
	return b
}

func TestParseEDID(t *testing.T) {
	pnp, model, ok := ParseEDID(edid("GSM", "LG TV"))
	if !ok || pnp != "GSM" || model != "LG TV" {
		t.Fatalf("got %q %q %v", pnp, model, ok)
	}
	if _, _, ok := ParseEDID([]byte{1, 2, 3}); ok {
		t.Fatal("a short EDID was accepted")
	}
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListScreens(t *testing.T) {
	sys := t.TempDir()
	writeFile(t, filepath.Join(sys, "card0/device/vendor"), "0x1002\n")
	writeFile(t, filepath.Join(sys, "card0/device/device"), "0x1638\n")
	writeFile(t, filepath.Join(sys, "card0-eDP-1/status"), "connected\n")
	writeFile(t, filepath.Join(sys, "card0-eDP-1/enabled"), "disabled\n")
	writeFile(t, filepath.Join(sys, "card0-HDMI-A-1/status"), "connected\n")
	writeFile(t, filepath.Join(sys, "card0-HDMI-A-1/enabled"), "enabled\n")
	writeFile(t, filepath.Join(sys, "card0-HDMI-A-1/edid"), string(edid("GSM", "LG TV")))
	writeFile(t, filepath.Join(sys, "card0-DP-1/status"), "disconnected\n")

	list := ListScreens(sys, map[string]string{"GSM": "Goldstar Company Ltd"})
	if len(list) != 2 {
		t.Fatalf("want 2 connected screens, got %+v", list)
	}
	hdmi, edp := list[0], list[1] // sorted by id: "card0-HDMI-A-1" < "card0-eDP-1"
	if hdmi.Connector != "HDMI-A-1" || hdmi.Name != "LG TV" || !hdmi.lit || hdmi.gpu != "1002:1638" ||
		hdmi.description != "Goldstar Company Ltd LG TV" || hdmi.Internal {
		t.Errorf("HDMI screen: %+v", hdmi)
	}
	if !edp.Internal || edp.description != "Internal screen" || edp.lit {
		t.Errorf("built-in screen: %+v", edp)
	}
}

func TestModeFile(t *testing.T) {
	f := ModeFile{Path: filepath.Join(t.TempDir(), "modes")}
	writeFile(t, f.Path, "Other screen:1280x720@60\nLG Electronics LG TV:3840x2160@60 1\n")
	if got := f.Get("LG Electronics LG TV"); got != "3840x2160@60" {
		t.Fatalf("Get = %q", got)
	}
	if err := f.Set("LG Electronics LG TV", "1920x1080@120"); err != nil {
		t.Fatal(err)
	}
	if got, other := f.Get("LG Electronics LG TV"), f.Get("Other screen"); got != "1920x1080@120" || other != "1280x720@60" {
		t.Fatalf("after Set: %q, %q", got, other)
	}
	if err := f.Set("LG Electronics LG TV", ""); err != nil || f.Get("LG Electronics LG TV") != "" {
		t.Fatalf("the saved mode was not forgotten (%v)", err)
	}
	if err := f.Set("x", "1920x1080"); err == nil {
		t.Fatal("an invalid mode was saved")
	}
}

func TestPrefs(t *testing.T) {
	p := Prefs{Path: filepath.Join(t.TempDir(), "owneet", "display.conf")}
	if err := p.Update(map[string]string{"OUTPUT": "HDMI-A-1", "VK_DEVICE": "10de:2882"}); err != nil {
		t.Fatal(err)
	}
	if err := p.Update(map[string]string{"KEYBOARD": "it", "VK_DEVICE": ""}); err != nil {
		t.Fatal(err)
	}
	got := p.Read()
	if got["OUTPUT"] != "HDMI-A-1" || got["KEYBOARD"] != "it" || got["VK_DEVICE"] != "" {
		t.Fatalf("Read = %v", got)
	}
	if err := p.Update(map[string]string{"OUTPUT": "HDMI-A-1; rm -rf /"}); err == nil {
		t.Fatal("an unsafe value was saved")
	}
}

func TestBacklight(t *testing.T) {
	sys := t.TempDir()
	writeFile(t, filepath.Join(sys, "acpi_video0/type"), "firmware\n")
	writeFile(t, filepath.Join(sys, "acpi_video0/max_brightness"), "15\n")
	writeFile(t, filepath.Join(sys, "amdgpu_bl0/type"), "raw\n")
	writeFile(t, filepath.Join(sys, "amdgpu_bl0/max_brightness"), "255\n")
	writeFile(t, filepath.Join(sys, "amdgpu_bl0/brightness"), "128\n")
	b := FindBacklight(sys)
	if b == nil || b.Name != "acpi_video0" {
		t.Fatalf("want the firmware backlight, got %+v", b)
	}
	raw := &Backlight{Name: "amdgpu_bl0", dir: filepath.Join(sys, "amdgpu_bl0"), max: 255}
	if p := raw.Percent(); p != 50 {
		t.Errorf("Percent = %d, want 50", p)
	}
	if v := raw.raw(0); v != 2 {
		t.Errorf("0%% gives %d: the screen must never go fully dark", v)
	}
}

func TestUniqueModes(t *testing.T) {
	got := uniqueModes("3840x2160@60 1920x1080@120 1920x1080@120 junk 1280x720@60")
	if len(got) != 3 || got[0] != "3840x2160@60" {
		t.Fatalf("uniqueModes = %v", got)
	}
}
