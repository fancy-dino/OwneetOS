// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
	"github.com/fancy-dino/OwneetOS/daemon/internal/webapps"
)

func TestMessagesRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := &writer{w: &buf}
	if err := w.send(map[string]any{"type": "keyboard", "open": true, "label": "E-mail ✉"}); err != nil {
		t.Fatal(err)
	}
	var n uint32
	binary.Read(bytes.NewReader(buf.Bytes()[:4]), binary.NativeEndian, &n)
	if int(n) != buf.Len()-4 {
		t.Fatalf("length %d, body %d bytes", n, buf.Len()-4)
	}
	msg, err := readMessage(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if msg["type"] != "keyboard" || msg["open"] != true || msg["label"] != "E-mail ✉" {
		t.Errorf("read back %v", msg)
	}
	if _, err := readMessage(&buf); err == nil {
		t.Error("read past the end")
	}
}

func TestTooLongMessageRefused(t *testing.T) {
	var buf bytes.Buffer
	binary.Write(&buf, binary.NativeEndian, uint32(maxMessage+1))
	if _, err := readMessage(&buf); err == nil {
		t.Error("a message longer than the limit was accepted")
	}
}

func TestEventsForTheExtension(t *testing.T) {
	for _, c := range []struct {
		ev   events.Event
		want any
	}{
		{events.Event{Type: "webapp.keyboard", Data: webapps.Keyboard{Open: false, Next: true}},
			map[string]any{"type": "keyboard_closed", "next": true}},
		{events.Event{Type: "webapp.keyboard", Data: webapps.Keyboard{Open: true, Kind: "text"}}, nil},
		{events.Event{Type: "webapp.text", Data: webapps.Text{Text: "à"}}, map[string]any{"type": "text", "text": "à"}},
		{events.Event{Type: "webapp.text", Data: webapps.Text{Key: "backspace"}}, map[string]any{"type": "key", "key": "backspace"}},
		{events.Event{Type: "guide.pressed"}, nil},
	} {
		if got := forEvent(c.ev); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v, want %v", c.ev.Type, got, c.want)
		}
	}
}

// The host's manifest is installed system-wide (/etc/opt/chrome/native-messaging-hosts/, the
// only place this Brave looks: not the profile's folder) and lets only the console's extension
// start the bridge. Brave gives an extension loaded unpacked the first 32 hex digits of the
// SHA-256 of its absolute path as id, written with the letters a-p ([[extension-design]]).
func TestManifestAllowsOnlyTheConsolesExtension(t *testing.T) {
	sum := sha256.Sum256([]byte("/usr/share/owneet/extension"))
	id := []byte(hex.EncodeToString(sum[:16]))
	for i, c := range id {
		if c <= '9' {
			id[i] = 'a' + (c - '0')
		} else {
			id[i] = 'a' + 10 + (c - 'a')
		}
	}
	data, err := os.ReadFile("org.owneet.bridge.json")
	if err != nil {
		t.Fatal(err)
	}
	var host struct {
		Name    string   `json:"name"`
		Path    string   `json:"path"`
		Type    string   `json:"type"`
		Origins []string `json:"allowed_origins"`
	}
	if err := json.Unmarshal(data, &host); err != nil {
		t.Fatal(err)
	}
	want := []string{"chrome-extension://" + string(id) + "/"}
	if host.Name != "org.owneet.bridge" || host.Path != "/usr/lib/owneet/owneet-bridge" ||
		host.Type != "stdio" || !reflect.DeepEqual(host.Origins, want) {
		t.Errorf("host manifest: %s, want origins %v", data, want)
	}
}
