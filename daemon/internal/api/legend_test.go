// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"testing"

	"github.com/fancy-dino/OwneetOS/daemon/internal/legend"
)

type fakeLegend struct{ got []legend.Request }

func (f *fakeLegend) Handle(r legend.Request) error {
	switch r.Action {
	case "toggle", "screen", "close":
		if r.Context == "none" {
			return legend.ErrNoApp
		}
		f.got = append(f.got, r)
		return nil
	}
	return legend.ErrInvalid
}
func (f *fakeLegend) Status() legend.State {
	return legend.State{Open: true, App: "web:netflix", Context: "netflix.browse"}
}

func TestLegendEndpoints(t *testing.T) {
	s, c, _ := start(t)
	if code, errCode := post(t, c, "/v1/legend", `{"action":"toggle","context":"web"}`); code != 503 || errCode != "legend.unavailable" {
		t.Fatalf("no legend: %d %s", code, errCode)
	}
	fl := &fakeLegend{}
	s.Legend = fl
	resp, err := c.Get("http://owneetd/v1/legend")
	if err != nil {
		t.Fatal(err)
	}
	var st legend.State
	json.NewDecoder(resp.Body).Decode(&st)
	resp.Body.Close()
	if resp.StatusCode != 200 || st.Context != "netflix.browse" {
		t.Fatalf("GET: %d %+v", resp.StatusCode, st)
	}
	for body, want := range map[string]struct {
		code int
		err  string
	}{
		`{"action":"toggle","context":"netflix.browse"}`: {204, ""},
		`{"action":"close"}`:                             {204, ""},
		`{"action":"open"}`:                              {400, "legend.invalid"},
		`{"action":"toggle","context":"none"}`:           {409, "legend.no_app"},
		`{"open":true}`:                                  {400, "bad_request"},
	} {
		if code, errCode := post(t, c, "/v1/legend", body); code != want.code || errCode != want.err {
			t.Errorf("%s: %d %s, want %d %s", body, code, errCode, want.code, want.err)
		}
	}
}
