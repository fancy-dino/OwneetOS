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
	// the user has accepted Google's terms for Widevine (widevine_test.go tests the question)
	m.ConsentFile = filepath.Join(t.TempDir(), "widevine.json")
	if err := m.AcceptWidevine(); err != nil {
		t.Fatal(err)
	}
	// and the web apps' profiles already have Widevine, as after their first opening (the first
	// opening itself: widevine_test.go); Brave is never started by the tests
	for _, id := range []string{"netflix", "youtube"} {
		widevineIn(t, m, id, "4.10.3112.0")
	}
	m.FetchWidevine = func(profile string) error { return putWidevine(profile, "4.10.3112.0") }
	return m, fa
}

func TestOpenNeedsAnInstalledWebApp(t *testing.T) {
	m, fa := newOpenManager(t, ":0")
	if _, err := m.Open("netflix"); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("open before installing: %v, want ErrNotInstalled", err)
	}
	if _, err := m.Open("brave"); !errors.Is(err, ErrUnknown) {
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
	if _, err := m.Open("netflix"); err != nil {
		t.Fatal(err)
	}
	req, ok := fa.running["web:netflix"]
	if !ok || req.Kind != apps.KindWebApp {
		t.Fatalf("running %+v: want web:netflix as a web app", fa.running)
	}
	cmd := strings.Join(req.Command, " ")
	profile := filepath.Join(m.ProfilesDir, "netflix")
	for _, want := range []string{"brave ", "--kiosk", "--user-data-dir=" + profile, "--ozone-platform=x11",
		"--password-store=basic", "--load-extension=" + m.ExtensionDir,
		"--disable-features=PerformanceInterventionUI,Speedreader,DisableLoadExtensionCommandLineSwitch", "https://www.netflix.com/"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q lacks %q", cmd, want)
		}
	}
	if n := strings.Count(cmd, "--disable-features="); n != 1 {
		t.Errorf("command %q: %d --disable-features, want one (Brave reads only the last)", cmd, n)
	}
	if req.Command[len(req.Command)-1] != "https://www.netflix.com/" {
		t.Errorf("the address must come last: %v", req.Command)
	}
	// Widevine turned on before the first start: the user has accepted Google's terms
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
		checkMouseFree(t, f, data)
	}
	// open again: brought back, not started twice
	if _, err := m.Open("netflix"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(fa.focused, []string{"web:netflix"}) {
		t.Errorf("focused %v, want web:netflix", fa.focused)
	}
}

// checkMouseFree: none of Brave's own windows or panels that need a mouse ([[step-5.3-youtube]])
func checkMouseFree(t *testing.T, file string, data []byte) {
	t.Helper()
	var prefs struct {
		Brave struct {
			Ask     *bool `json:"ask_widevine_install"`
			Sidebar struct {
				Show int `json:"sidebar_show_option"`
			} `json:"sidebar"`
		} `json:"brave"`
	}
	if err := json.Unmarshal(data, &prefs); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	if prefs.Brave.Ask == nil || *prefs.Brave.Ask {
		t.Errorf("%s: %s, want Brave's Widevine question off", file, data)
	}
	if prefs.Brave.Sidebar.Show != 3 {
		t.Errorf("%s: %s, want the sidebar never shown (3)", file, data)
	}
}

func TestBraveSettingsSetAgainBeforeEveryOpening(t *testing.T) {
	m, fa := newOpenManager(t, ":0")
	if err := m.Install("youtube"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m)
	if _, err := m.Open("youtube"); err != nil {
		t.Fatal(err)
	}
	if err := fa.Close("web:youtube"); err != nil {
		t.Fatal(err)
	}
	// Brave rewrote the file meanwhile: its own settings, a large number, the question back on
	// (e.g. after a Widevine update) and an HTML-like string
	prefsFile := filepath.Join(m.ProfilesDir, "youtube", "Default", "Preferences")
	brave := `{"brave":{"ask_widevine_install":true,"widevine_opted_in":true,"sidebar":{"sidebar_show_option":0,"item_list":[1,2]}},` +
		`"profile":{"exit_type":"Normal","created_by_version":"141.1.97.57"},"time":13405678901234567890,"page":"<a&b>"}`
	if err := os.WriteFile(prefsFile, []byte(brave), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Open("youtube"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(prefsFile)
	if err != nil {
		t.Fatal(err)
	}
	checkMouseFree(t, "Preferences", data)
	for _, keep := range []string{`"widevine_opted_in":true`, `"item_list":[1,2]`, `"exit_type":"Normal"`,
		`"created_by_version":"141.1.97.57"`, `"time":13405678901234567890`, `"page":"<a&b>"`} {
		if !strings.Contains(string(data), keep) {
			t.Errorf("Preferences %s lost %s", data, keep)
		}
	}
}

func TestBraveSettingsLeaveAFileThatIsNotJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Preferences")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := enforcePrefs(path, profilePrefs); err == nil {
		t.Error("no error for a file that is not JSON")
	}
	if data, _ := os.ReadFile(path); string(data) != "not json" {
		t.Errorf("the file became %q, want it untouched", data)
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
	if _, err := m.Open("youtube"); err != nil {
		t.Fatal(err)
	}
	cmd := strings.Join(fa.running["web:youtube"].Command, " ")
	if !strings.Contains(cmd, "--ozone-platform=wayland") || strings.Contains(cmd, "--load-extension") {
		t.Errorf("command %q: want Wayland and no missing extension", cmd)
	}
	if !strings.Contains(cmd, "--disable-features=PerformanceInterventionUI,Speedreader ") {
		t.Errorf("command %q: want Brave's performance alert and reading view off, nothing else", cmd)
	}
}

func TestUserAgentFromTheCatalogue(t *testing.T) {
	m, fa := newOpenManager(t, ":0")
	for _, id := range []string{"netflix", "youtube"} {
		if err := m.Install(id); err != nil {
			t.Fatal(err)
		}
		waitIdle(t, m)
		if _, err := m.Open(id); err != nil {
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
	if _, err := m.Open("youtube"); err != nil {
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
