// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
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
func (f *fakeCatalog) Open(id string) error {
	switch id {
	case "spotify":
		return catalog.ErrNotInstalled
	case "youtube":
		return apps.ErrGameRunning
	}
	return f.act("open", id)
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
