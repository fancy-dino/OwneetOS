// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
)

// fakeApps plays apps.Manager: it keeps the running apps and what was asked.
type fakeApps struct {
	mu      sync.Mutex
	running map[string]apps.LaunchRequest
	focused []string
	closed  []string
}

func (f *fakeApps) Status() apps.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := apps.Status{Apps: []apps.App{}}
	for id, r := range f.running {
		st.Apps = append(st.Apps, apps.App{ID: id, Name: r.Name, Kind: r.Kind})
	}
	return st
}
func (f *fakeApps) Launch(req apps.LaunchRequest) (apps.App, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.running == nil {
		f.running = map[string]apps.LaunchRequest{}
	}
	f.running[req.ID] = req
	return apps.App{ID: req.ID, Name: req.Name, Kind: req.Kind}, nil
}
func (f *fakeApps) FocusApp(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.focused = append(f.focused, id)
	return nil
}
func (f *fakeApps) Close(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, id)
	delete(f.running, id)
	return nil
}

func newOpenManager(t *testing.T, display string) (*Manager, *fakeApps) {
	t.Helper()
	m, _ := newManager(t)
	fa := &fakeApps{}
	m.Apps = fa
	m.Session = func() (apps.Session, error) { return apps.Session{Mode: "gamescope", Display: display}, nil }
	m.ProfilesDir = filepath.Join(t.TempDir(), "webapps")
	m.ExtensionDir = t.TempDir()
	return m, fa
}

func TestOpenNeedsAnInstalledWebApp(t *testing.T) {
	m, fa := newOpenManager(t, ":0")
	if err := m.Open("netflix"); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("open before installing: %v, want ErrNotInstalled", err)
	}
	if err := m.Open("brave"); !errors.Is(err, ErrUnknown) {
		t.Errorf("open the engine itself: %v, want ErrUnknown", err)
	}
	if len(fa.Status().Apps) != 0 {
		t.Error("something was started")
	}
}

func TestOpenRunsBraveWithItsOwnProfile(t *testing.T) {
	m, fa := newOpenManager(t, ":0")
	if err := m.Install("netflix"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m)
	if err := m.Open("netflix"); err != nil {
		t.Fatal(err)
	}
	req, ok := fa.running["web:netflix"]
	if !ok || req.Kind != apps.KindWebApp {
		t.Fatalf("running %+v: want web:netflix as a web app", fa.running)
	}
	cmd := strings.Join(req.Command, " ")
	profile := filepath.Join(m.ProfilesDir, "netflix")
	for _, want := range []string{"brave ", "--kiosk", "--user-data-dir=" + profile, "--ozone-platform=x11",
		"--password-store=basic", "--load-extension=" + m.ExtensionDir, "https://www.netflix.com/"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q lacks %q", cmd, want)
		}
	}
	if req.Command[len(req.Command)-1] != "https://www.netflix.com/" {
		t.Errorf("the address must come last: %v", req.Command)
	}
	// Widevine turned on before the first start
	for _, f := range []string{"Local State", filepath.Join("Default", "Preferences")} {
		data, err := os.ReadFile(filepath.Join(profile, f))
		if err != nil {
			t.Fatal(err)
		}
		var prefs struct {
			Brave struct {
				Widevine bool `json:"widevine_opted_in"`
			} `json:"brave"`
		}
		if err := json.Unmarshal(data, &prefs); err != nil || !prefs.Brave.Widevine {
			t.Errorf("%s: %s (%v), want Widevine on", f, data, err)
		}
	}
	// open again: brought back, not started twice
	if err := m.Open("netflix"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(fa.focused, []string{"web:netflix"}) {
		t.Errorf("focused %v, want web:netflix", fa.focused)
	}
}

func TestSeedKeepsAnExistingProfile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "netflix")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := seedProfile(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Local State")); !os.IsNotExist(err) {
		t.Error("a profile Brave already uses must be left as it is")
	}
}

func TestWaylandWithoutAnXServer(t *testing.T) {
	m, fa := newOpenManager(t, "")
	m.ExtensionDir = filepath.Join(t.TempDir(), "missing")
	if err := m.Install("youtube"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m)
	if err := m.Open("youtube"); err != nil {
		t.Fatal(err)
	}
	cmd := strings.Join(fa.running["web:youtube"].Command, " ")
	if !strings.Contains(cmd, "--ozone-platform=wayland") || strings.Contains(cmd, "--load-extension") {
		t.Errorf("command %q: want Wayland and no missing extension", cmd)
	}
}

func TestUserAgentFromTheCatalogue(t *testing.T) {
	m, fa := newOpenManager(t, ":0")
	for _, id := range []string{"netflix", "youtube"} {
		if err := m.Install(id); err != nil {
			t.Fatal(err)
		}
		waitIdle(t, m)
		if err := m.Open(id); err != nil {
			t.Fatal(err)
		}
	}
	yt := fa.running["web:youtube"].Command
	if !slices.Contains(yt, "--user-agent=Mozilla/5.0 (Linux; SMART-TV) Test/1.0") {
		t.Errorf("youtube %q: want the catalogue's user agent as one argument", yt)
	}
	if yt[len(yt)-1] != "https://www.youtube.com/tv" {
		t.Errorf("the address must come last: %v", yt)
	}
	if nf := strings.Join(fa.running["web:netflix"].Command, " "); strings.Contains(nf, "--user-agent") {
		t.Errorf("netflix %q: no user agent in its entry, want Brave's own", nf)
	}
}

func TestRemoveClosesTheWebAppAndDeletesItsData(t *testing.T) {
	m, fa := newOpenManager(t, ":0")
	for _, id := range []string{"netflix", "youtube"} {
		if err := m.Install(id); err != nil {
			t.Fatal(err)
		}
		waitIdle(t, m)
	}
	if err := m.Open("youtube"); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("youtube"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m)
	if !slices.Equal(fa.closed, []string{"web:youtube"}) {
		t.Errorf("closed %v, want web:youtube", fa.closed)
	}
	if _, err := os.Stat(filepath.Join(m.ProfilesDir, "youtube")); !os.IsNotExist(err) {
		t.Errorf("youtube's profile still there (%v)", err)
	}
}
