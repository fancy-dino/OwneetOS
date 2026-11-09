// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
	"github.com/fancy-dino/OwneetOS/daemon/internal/player"
)

type fakePlayer struct{ opened []player.Request }

func (f *fakePlayer) Open(r player.Request) (player.State, error) {
	switch r.Kind {
	case "busy":
		return player.State{}, apps.ErrGameRunning
	case "gone":
		return player.State{}, player.ErrNotFound
	case "videos", "music":
		f.opened = append(f.opened, r)
		return player.State{Open: true, Kind: r.Kind, Count: len(r.Items), Title: r.Items[r.Index].Title}, nil
	}
	return player.State{}, player.ErrInvalid
}
func (f *fakePlayer) Status() player.State { return player.State{Open: true, Title: "Episode 2"} }

func TestPlayerEndpoints(t *testing.T) {
	s, c, _ := start(t)
	if code, errCode := post(t, c, "/v1/player", `{"kind":"videos"}`); code != 503 || errCode != "player.unavailable" {
		t.Fatalf("no player: %d %s", code, errCode)
	}
	fp := &fakePlayer{}
	s.Player = fp
	resp, err := c.Get("http://owneetd/v1/player")
	if err != nil {
		t.Fatal(err)
	}
	var st player.State
	json.NewDecoder(resp.Body).Decode(&st)
	resp.Body.Close()
	if resp.StatusCode != 200 || !st.Open || st.Title != "Episode 2" {
		t.Fatalf("GET: %d %+v", resp.StatusCode, st)
	}
	for body, want := range map[string]struct {
		code int
		err  string
	}{
		`{"kind":"videos","items":[{"path":"/m/a.mkv","title":"A"}],"index":0}`: {201, ""},
		`{"kind":"games"}`: {400, "player.invalid"},
		`{"kind":"busy"}`:  {409, "apps.game_running"},
		`{"kind":"gone"}`:  {404, "player.not_found"},
		`{"kind":"videos"`: {400, "bad_request"},
		`{"colour":"red"}`: {400, "bad_request"},
	} {
		if code, errCode := post(t, c, "/v1/player", body); code != want.code || errCode != want.err {
			t.Errorf("%s: %d %s, want %d %s", body, code, errCode, want.code, want.err)
		}
	}
	// a long album: more than the other routes take
	items := make([]string, 3000)
	for i := range items {
		items[i] = `{"path":"/media/Music/Some artist/Some album/a song with a long name.flac","title":"A song with a long name"}`
	}
	if code, errCode := post(t, c, "/v1/player", `{"kind":"music","index":0,"items":[`+strings.Join(items, ",")+`]}`); code != 201 {
		t.Fatalf("3000 songs: %d %s", code, errCode)
	}
	if n := len(fp.opened); n != 2 || len(fp.opened[1].Items) != 3000 {
		t.Fatalf("opened %d", n)
	}
}
