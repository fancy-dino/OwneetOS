// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/fancy-dino/OwneetOS/daemon/internal/audio"
)

type fakeAudio struct {
	volume int
	muted  bool
	output string
}

func (f *fakeAudio) Status() audio.Status {
	return audio.Status{Available: true, Output: f.output, Volume: f.volume, Muted: f.muted}
}
func (f *fakeAudio) SetVolume(p int) error { f.volume = p; return nil }
func (f *fakeAudio) SetMuted(m bool) error { f.muted = m; return nil }
func (f *fakeAudio) SetOutput(id string) error {
	if id != "hdmi" {
		return audio.ErrUnknownOutput
	}
	f.output = id
	return nil
}

func put(t *testing.T, c *http.Client, path, body string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, "http://owneetd"+path, strings.NewReader(body))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestAudioEndpoints(t *testing.T) {
	s, c, _ := start(t)
	if code := put(t, c, "/v1/audio/volume", `{"volume":50}`); code != 503 {
		t.Fatalf("without audio: %d", code)
	}
	fake := &fakeAudio{}
	s.Audio = fake

	if code := put(t, c, "/v1/audio/volume", `{"volume":35}`); code != 204 || fake.volume != 35 {
		t.Fatalf("volume: %d %d", code, fake.volume)
	}
	if code := put(t, c, "/v1/audio/volume", `{"volume":0}`); code != 204 || fake.volume != 0 {
		t.Fatalf("volume 0: %d %d", code, fake.volume)
	}
	for _, bad := range []string{`{"volume":101}`, `{"volume":-1}`, `{}`} {
		if code := put(t, c, "/v1/audio/volume", bad); code != 400 {
			t.Fatalf("volume %s: %d", bad, code)
		}
	}
	if code := put(t, c, "/v1/audio/mute", `{"muted":true}`); code != 204 || !fake.muted {
		t.Fatalf("mute: %d", code)
	}
	if code := put(t, c, "/v1/audio/mute", `{}`); code != 400 {
		t.Fatalf("mute without value: %d", code)
	}
	if code := put(t, c, "/v1/audio/output", `{"id":"hdmi"}`); code != 204 || fake.output != "hdmi" {
		t.Fatalf("output: %d", code)
	}
	if code := put(t, c, "/v1/audio/output", `{"id":"nope"}`); code != 404 {
		t.Fatalf("unknown output: %d", code)
	}
}
