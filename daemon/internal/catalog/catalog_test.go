// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testCatalog = `{"apps": [
    {"id": "brave", "kind": "engine", "packages": ["brave-bin"], "sizeMiB": 500},
    {"id": "netflix", "kind": "webapp", "nameKey": "app.netflix", "icon": "N", "requires": ["brave"], "url": "https://www.netflix.com/"},
    {"id": "youtube", "kind": "webapp", "nameKey": "app.youtube", "icon": "Y", "requires": ["brave"], "url": "https://www.youtube.com/tv", "userAgent": "Mozilla/5.0 (Linux; SMART-TV) Test/1.0"}
]}`

// fakeHelper plays owneet-apps: it installs or removes the engine's package in a fake pacman
// database and writes the state file, as the root units would.
type fakeHelper struct {
	t     *testing.T
	m     *Manager
	fail  bool
	mu    sync.Mutex
	units []string
}

func (f *fakeHelper) Start(unit string) error {
	f.mu.Lock()
	f.units = append(f.units, unit)
	f.mu.Unlock()
	name := strings.TrimSuffix(strings.TrimPrefix(unit, "owneet-apps-"), ".service")
	action, engine, _ := strings.Cut(name, "@")
	go func() {
		time.Sleep(5 * time.Millisecond)
		state := "done"
		pkg := filepath.Join(f.m.PacmanDB, "brave-bin-1.97.57-1")
		switch {
		case f.fail:
			state = "failed"
		case action == "install":
			if err := os.MkdirAll(pkg, 0o755); err != nil {
				f.t.Error(err)
			}
		default:
			if err := os.RemoveAll(pkg); err != nil {
				f.t.Error(err)
			}
		}
		data := `{"state":"` + state + `","action":"` + action + `","id":"` + engine + `"}`
		if err := os.WriteFile(f.m.StateFile, []byte(data), 0o644); err != nil {
			f.t.Error(err)
		}
	}()
	return nil
}

func (f *fakeHelper) started() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.units...)
}

func newManager(t *testing.T) (*Manager, *fakeHelper) {
	t.Helper()
	dir := t.TempDir()
	m := &Manager{
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		CatalogFile: filepath.Join(dir, "apps.json"),
		StateFile:   filepath.Join(dir, "state.json"),
		ChosenFile:  filepath.Join(dir, "state", "webapps.json"),
		PacmanDB:    filepath.Join(dir, "local"),
		Poll:        2 * time.Millisecond,
		Timeout:     2 * time.Second,
	}
	if err := os.WriteFile(m.CatalogFile, []byte(testCatalog), 0o644); err != nil {
		t.Fatal(err)
	}
	// a package whose name starts like the engine's must not count as it
	if err := os.MkdirAll(filepath.Join(m.PacmanDB, "brave-bin-extras-1.0-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &fakeHelper{t: t, m: m}
	m.Units = h
	return m, h
}

// waitIdle waits for the running installation or removal to end.
func waitIdle(t *testing.T, m *Manager) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
		m.mu.Lock()
		busy := m.busyID
		m.mu.Unlock()
		if busy == "" {
			return
		}
	}
	t.Fatal("still busy")
}

func TestStatusListsOnlyWebApps(t *testing.T) {
	m, _ := newManager(t)
	st := m.Status()
	if len(st.Apps) != 2 || !st.Allowed {
		t.Fatalf("status %+v: want the two web apps, allowed", st)
	}
	if a := app(st, "netflix"); a.Installed || a.SizeMiB != 500 || a.NameKey != "app.netflix" {
		t.Errorf("netflix %+v: want not installed, 500 MiB", a)
	}
}

func TestFirstWebAppInstallsTheEngineAndTheNextOneDoesNot(t *testing.T) {
	m, h := newManager(t)
	if err := m.Install("netflix"); err != nil {
		t.Fatal(err)
	}
	if a := app(m.Status(), "netflix"); a.Busy != "install" {
		t.Errorf("netflix while installing: %+v", a)
	}
	if err := m.Install("youtube"); !errors.Is(err, ErrBusy) {
		t.Errorf("second install while busy: %v, want ErrBusy", err)
	}
	waitIdle(t, m)
	st := m.Status()
	if !app(st, "netflix").Installed || app(st, "youtube").Installed {
		t.Fatalf("after installing netflix: %+v", st.Apps)
	}
	if a := app(st, "youtube"); a.SizeMiB != 0 {
		t.Errorf("youtube with the engine installed takes %d MiB, want 0", a.SizeMiB)
	}
	if err := m.Install("youtube"); err != nil {
		t.Fatal(err)
	}
	if !app(m.Status(), "youtube").Installed {
		t.Error("youtube not installed at once")
	}
	if got := h.started(); len(got) != 1 || got[0] != "owneet-apps-install@brave.service" {
		t.Errorf("units started %v, want only the engine's installation", got)
	}
}

func TestLastWebAppRemovesTheEngine(t *testing.T) {
	m, h := newManager(t)
	for _, id := range []string{"netflix", "youtube"} {
		if err := m.Install(id); err != nil {
			t.Fatal(err)
		}
		waitIdle(t, m)
	}
	if err := m.Remove("youtube"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m)
	if len(h.started()) != 1 {
		t.Fatalf("removing youtube started %v: the engine is still needed", h.started()[1:])
	}
	if err := m.Remove("netflix"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m)
	if got := h.started(); got[len(got)-1] != "owneet-apps-remove@brave.service" {
		t.Errorf("units started %v, want the engine removed last", got)
	}
	st := m.Status()
	if app(st, "netflix").Installed || app(st, "netflix").SizeMiB != 500 {
		t.Errorf("after removing every web app: %+v", st.Apps)
	}
}

func TestFailedInstallLeavesTheAppMissing(t *testing.T) {
	m, h := newManager(t)
	h.fail = true
	if err := m.Install("netflix"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m)
	if app(m.Status(), "netflix").Installed {
		t.Error("netflix installed although its engine failed")
	}
}

func TestRefusals(t *testing.T) {
	m, h := newManager(t)
	if err := m.Install("brave"); !errors.Is(err, ErrUnknown) {
		t.Errorf("install of the engine itself: %v, want ErrUnknown", err)
	}
	if err := m.Install("steam"); !errors.Is(err, ErrUnknown) {
		t.Errorf("install of an app outside the catalogue: %v, want ErrUnknown", err)
	}
	m.Allowed = func() bool { return false }
	if err := m.Install("netflix"); !errors.Is(err, ErrNeedsInstall) {
		t.Errorf("live system with little memory: %v, want ErrNeedsInstall", err)
	}
	if m.Status().Allowed {
		t.Error("status allowed in a live system with little memory")
	}
	m.Allowed = nil
	m.FreeMiB = func() int64 { return 300 }
	if err := m.Install("netflix"); !errors.Is(err, ErrNoSpace) {
		t.Errorf("disk almost full: %v, want ErrNoSpace", err)
	}
	if len(h.started()) != 0 {
		t.Errorf("units started %v after refusals", h.started())
	}
}

// The catalogue shipped by owneet-base: valid, and one app per line, as owneet-apps (root, no JSON
// parser) reads the engines' packages line by line.
func TestShippedCatalog(t *testing.T) {
	path := filepath.Join("..", "..", "..", "packages", "owneet-base", "apps.json")
	if _, err := os.Stat(path); err != nil {
		// the owneetd package is built from daemon/ alone; tools/lint has the whole repository
		t.Skip("packages/owneet-base/apps.json is not here")
	}
	m := &Manager{CatalogFile: path}
	cat, err := m.load()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range cat {
		line := ""
		for _, l := range strings.Split(string(data), "\n") {
			if strings.Contains(l, `"id": "`+e.ID+`",`) {
				line = l
			}
		}
		if line == "" || !strings.Contains(line, `"kind": "`+e.Kind+`"`) {
			t.Errorf("%s: not on one line with its kind", e.ID)
		}
		if e.Kind == KindEngine && (len(e.Packages) == 0 || !strings.Contains(line, `"packages": [`)) {
			t.Errorf("engine %s: its packages must be on its line", e.ID)
		}
		for _, r := range e.Requires {
			if _, ok := find(cat, r, KindEngine); !ok {
				t.Errorf("%s requires %s, which is not an engine of the catalogue", e.ID, r)
			}
		}
	}
	// YouTube's TV interface, which needs a TV user agent ([[step-5.3-youtube]])
	if yt, ok := find(cat, "youtube", KindWebApp); !ok || !strings.HasSuffix(yt.URL, "/tv") || yt.UserAgent == "" {
		t.Errorf("youtube %+v: want the TV interface with a user agent", yt)
	}
	// Spotify's player for an Android TV: another agent gets a page that plays nothing
	// ([[step-5.5-spotify]]); brand-neutral, as YouTube's
	if sp, ok := find(cat, "spotify", KindWebApp); !ok || !strings.Contains(sp.UserAgent, "Android") ||
		!strings.Contains(sp.UserAgent, "Chrome/") || strings.Contains(sp.UserAgent, "Build/") {
		t.Errorf("spotify %+v: want a brand-neutral Android TV user agent", sp)
	}
}
