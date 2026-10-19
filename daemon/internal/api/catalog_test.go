// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
	"github.com/fancy-dino/OwneetOS/daemon/internal/catalog"
)

type fakeCatalog struct{ calls []string }

func (f *fakeCatalog) Status() catalog.Status {
	return catalog.Status{Apps: []catalog.App{}, Allowed: true}
}
func (f *fakeCatalog) act(what, id string) error {
	switch id {
	case "netflix":
		f.calls = append(f.calls, what+" "+id)
		return nil
	case "busy":
		return catalog.ErrBusy
	case "live":
		return catalog.ErrNeedsInstall
	case "full":
		return catalog.ErrNoSpace
	}
	return catalog.ErrUnknown
}
func (f *fakeCatalog) Install(id string) error { return f.act("install", id) }
func (f *fakeCatalog) Remove(id string) error  { return f.act("remove", id) }
func (f *fakeCatalog) Open(id string) (bool, error) {
	switch id {
	case "spotify":
		return false, catalog.ErrNotInstalled
	case "youtube":
		return false, apps.ErrGameRunning
	case "consent":
		return false, catalog.ErrWidevineConsent
	case "first":
		return true, nil
	case "other":
		return false, catalog.ErrPreparing
	}
	return false, f.act("open", id)
}
func (f *fakeCatalog) Widevine() catalog.WidevineStatus {
	return catalog.WidevineStatus{Accepted: true, Ask: true, Updated: true}
}
func (f *fakeCatalog) AcceptWidevine() error {
	f.calls = append(f.calls, "accept")
	return nil
}
func (f *fakeCatalog) OpenTerms(language string) error {
	if language == "busy" {
		return apps.ErrGameRunning
	}
	if language == "wait" {
		return catalog.ErrPreparing
	}
	f.calls = append(f.calls, "terms "+language)
	return nil
}

func TestWidevineEndpoints(t *testing.T) {
	s, c, _ := start(t)
	fake := &fakeCatalog{}
	s.Catalog = fake
	resp, err := c.Get("http://owneetd/v1/webapps/widevine")
	if err != nil {
		t.Fatal(err)
	}
	var st catalog.WidevineStatus
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil || resp.StatusCode != 200 || !st.Ask || !st.Updated {
		t.Errorf("GET widevine: %d %+v (%v)", resp.StatusCode, st, err)
	}
	resp.Body.Close()
	for _, r := range []struct {
		path, body string
		code       int
		err        string
	}{
		{"/v1/webapps/widevine/accept", ``, 204, ""},
		{"/v1/webapps/widevine/terms", `{"language":"it"}`, 204, ""},
		{"/v1/webapps/widevine/terms", ``, 204, ""},
		{"/v1/webapps/widevine/terms", `{"language":"busy"}`, 409, "apps.game_running"},
		{"/v1/webapps/widevine/terms", `{"lang":"it"}`, 400, "bad_request"},
		{"/v1/webapps/widevine/terms", `{"language":"wait"}`, 409, "apps.preparing"},
		{"/v1/apps/catalog/consent/open", ``, 409, "apps.widevine_consent"},
		{"/v1/apps/catalog/first/open", ``, 202, ""},
		{"/v1/apps/catalog/other/open", ``, 409, "apps.preparing"},
	} {
		if code, errCode := post(t, c, r.path, r.body); code != r.code || errCode != r.err {
			t.Errorf("%s %s: %d %q, want %d %q", r.path, r.body, code, errCode, r.code, r.err)
		}
	}
	if want := []string{"accept", "terms it", "terms "}; !slices.Equal(fake.calls, want) {
		t.Errorf("calls = %q, want %q", fake.calls, want)
	}
}

func TestCatalogEndpoints(t *testing.T) {
	s, c, _ := start(t)
	if code, errCode := post(t, c, "/v1/apps/catalog/netflix/install", ``); code != 503 || errCode != "apps.unavailable" {
		t.Fatalf("without the catalogue: %d %s", code, errCode)
	}
	fake := &fakeCatalog{}
	s.Catalog = fake
	for path, want := range map[string]struct {
		code int
		err  string
	}{
		"/v1/apps/catalog/netflix/install": {202, ""},
		"/v1/apps/catalog/netflix/remove":  {202, ""},
		"/v1/apps/catalog/busy/install":    {409, "apps.busy"},
		"/v1/apps/catalog/live/install":    {409, "apps.needs_install"},
		"/v1/apps/catalog/full/install":    {409, "apps.no_space"},
		"/v1/apps/catalog/steam/install":   {404, "apps.unknown"},
		"/v1/apps/catalog/netflix/open":    {204, ""},
		"/v1/apps/catalog/spotify/open":    {409, "apps.not_installed"},
		"/v1/apps/catalog/youtube/open":    {409, "apps.game_running"},
		"/v1/apps/catalog/steam/open":      {404, "apps.unknown"},
	} {
		if code, errCode := post(t, c, path, ``); code != want.code || errCode != want.err {
			t.Errorf("%s: %d %q, want %d %q", path, code, errCode, want.code, want.err)
		}
	}
	if len(fake.calls) != 3 {
		t.Errorf("calls = %v", fake.calls)
	}
}
