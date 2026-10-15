// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"testing"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
	"github.com/fancy-dino/OwneetOS/daemon/internal/webapps"
)

type webAppScreen struct{ open bool }

func (f *webAppScreen) Status() apps.Status {
	if !f.open {
		return apps.Status{}
	}
	return apps.Status{Apps: []apps.App{{ID: "web:netflix", Kind: apps.KindWebApp}}}
}
func (f *webAppScreen) ShowHome() error          { return nil }
func (f *webAppScreen) FocusApp(id string) error { return nil }

func TestWebAppsEndpoints(t *testing.T) {
	s, c, _ := start(t)
	if code, errCode := post(t, c, "/v1/webapps/ask-close", ``); code != 503 || errCode != "webapps.unavailable" {
		t.Fatalf("without the hub: %d %s", code, errCode)
	}
	screen := &webAppScreen{}
	hub := &webapps.Hub{Broker: s.Broker, Screen: screen}
	s.WebApps = hub
	if code, errCode := post(t, c, "/v1/webapps/ask-close", ``); code != 409 || errCode != "webapps.no_webapp" {
		t.Errorf("no web app: %d %s", code, errCode)
	}
	screen.open = true
	for _, step := range []struct {
		path, body string
		code       int
		err        string
	}{
		{"/v1/webapps/text", `{"text":"a"}`, 409, "webapps.keyboard_closed"},
		{"/v1/webapps/keyboard", `{"open":true,"kind":"color"}`, 400, "webapps.invalid"},
		{"/v1/webapps/keyboard", `{"open":true,"kind":"email","label":"Email"}`, 204, ""},
		{"/v1/webapps/text", `{"text":"a"}`, 204, ""},
		{"/v1/webapps/text", `{"key":"backspace"}`, 204, ""},
		{"/v1/webapps/text", `{"key":"enter"}`, 400, "webapps.invalid"},
		{"/v1/webapps/keyboard", `{"open":false,"next":true}`, 204, ""},
		{"/v1/webapps/ask-close", ``, 204, ""},
	} {
		if code, errCode := post(t, c, step.path, step.body); code != step.code || errCode != step.err {
			t.Errorf("%s %s: %d %q, want %d %q", step.path, step.body, code, errCode, step.code, step.err)
		}
	}
}

// The interface sends the look with its request to open a web app; the bridge reads it back.
func TestOpenKeepsTheLook(t *testing.T) {
	s, c, _ := start(t)
	s.Catalog = &fakeCatalog{}
	s.WebApps = &webapps.Hub{Broker: s.Broker}
	if code, errCode := post(t, c, "/v1/apps/catalog/netflix/open", `{"accent":"#2EF2DA","language":"it"}`); code != 204 {
		t.Fatalf("open: %d %s", code, errCode)
	}
	resp, err := c.Get("http://owneetd/v1/webapps/look")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var look webapps.Look
	if json.NewDecoder(resp.Body).Decode(&look); look.Accent != "#2EF2DA" || look.Language != "it" {
		t.Errorf("look = %+v", look)
	}
	if code, errCode := post(t, c, "/v1/apps/catalog/netflix/open", `{"colour":"red"}`); code != 400 || errCode != "bad_request" {
		t.Errorf("unknown field: %d %s", code, errCode)
	}
}
