// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"net/url"
	"testing"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
)

type fakeApps struct{ calls []string }

func (f *fakeApps) Status() apps.Status { return apps.Status{Apps: []apps.App{}, Focus: "home"} }
func (f *fakeApps) Launch(r apps.LaunchRequest) (apps.App, error) {
	if len(r.Command) == 0 {
		return apps.App{}, apps.ErrInvalid
	}
	f.calls = append(f.calls, "launch "+r.ID)
	return apps.App{ID: r.ID, Name: r.Name, Kind: r.Kind}, nil
}
func (f *fakeApps) act(what, id string) error {
	if id != "steam:42" {
		return apps.ErrUnknownApp
	}
	f.calls = append(f.calls, what+" "+id)
	return nil
}
func (f *fakeApps) Close(id string) error    { return f.act("close", id) }
func (f *fakeApps) Kill(id string) error     { return f.act("kill", id) }
func (f *fakeApps) FocusApp(id string) error { return f.act("focus", id) }
func (f *fakeApps) Steam(r apps.SteamRequest) error {
	switch {
	case r.Run == "13":
		return apps.ErrSteamNeedsInstall
	case r.Open == "" && r.Run == "":
		return apps.ErrInvalid
	}
	f.calls = append(f.calls, "steam "+r.Open+r.Run)
	return nil
}
func (f *fakeApps) SteamStatus() apps.SteamStatus {
	return apps.SteamStatus{Installed: true, Allowed: true}
}

func TestAppsEndpoints(t *testing.T) {
	s, c, _ := start(t)
	if code, errCode := post(t, c, "/v1/apps/launch", `{"id":"x","command":["true"]}`); code != 503 || errCode != "apps.unavailable" {
		t.Fatalf("without apps: %d %s", code, errCode)
	}
	fake := &fakeApps{}
	s.Apps = fake
	if code, _ := post(t, c, "/v1/apps/launch", `{"id":"steam:42","name":"Game","kind":"game","command":["/usr/bin/game"]}`); code != 201 {
		t.Fatalf("launch: %d", code)
	}
	if code, errCode := post(t, c, "/v1/apps/launch", `{"id":"x"}`); code != 400 || errCode != "apps.invalid" {
		t.Fatalf("invalid launch: %d %s", code, errCode)
	}
	id := url.PathEscape("steam:42")
	for _, action := range []string{"focus", "close", "kill"} {
		if code, _ := post(t, c, "/v1/apps/"+id+"/"+action, ``); code != 204 {
			t.Fatalf("%s: %d", action, code)
		}
	}
	if code, errCode := post(t, c, "/v1/apps/other/close", ``); code != 404 || errCode != "apps.unknown_app" {
		t.Fatalf("unknown app: %d %s", code, errCode)
	}
	if len(fake.calls) != 4 {
		t.Fatalf("calls = %v", fake.calls)
	}
}

func TestSteamEndpoints(t *testing.T) {
	s, c, _ := start(t)
	fake := &fakeApps{}
	s.Apps = fake
	if code, _ := post(t, c, "/v1/steam", `{"open":"library"}`); code != 202 {
		t.Fatalf("open: %d", code)
	}
	if code, _ := post(t, c, "/v1/steam", `{"run":"480","name":"Spacewar"}`); code != 202 {
		t.Fatalf("run: %d", code)
	}
	if code, errCode := post(t, c, "/v1/steam", `{"run":"13"}`); code != 409 || errCode != "steam.needs_install" {
		t.Fatalf("live system without memory: %d %s", code, errCode)
	}
	if code, errCode := post(t, c, "/v1/steam", `{}`); code != 400 || errCode != "apps.invalid" {
		t.Fatalf("invalid: %d %s", code, errCode)
	}
	if len(fake.calls) != 2 || fake.calls[1] != "steam 480" {
		t.Fatalf("calls = %v", fake.calls)
	}
}
