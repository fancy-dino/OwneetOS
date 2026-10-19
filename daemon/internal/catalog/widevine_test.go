// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// widevineIn puts a Widevine version into a profile, as Brave does when it downloads one.
func widevineIn(t *testing.T, m *Manager, profile, version string) {
	t.Helper()
	dir := filepath.Join(m.ProfilesDir, profile, "WidevineCdm")
	if err := os.MkdirAll(filepath.Join(dir, version), 0o700); err != nil {
		t.Fatal(err)
	}
	hint, _ := json.Marshal(map[string]string{"Path": filepath.Join(dir, version)})
	if err := os.WriteFile(filepath.Join(dir, "latest-component-updated-widevine-cdm"), hint, 0o600); err != nil {
		t.Fatal(err)
	}
}

func widevineOn(t *testing.T, m *Manager, profile string) bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(m.ProfilesDir, profile, "Default", "Preferences"))
	if err != nil {
		t.Fatal(err)
	}
	var prefs struct {
		Brave struct {
			Widevine bool `json:"widevine_opted_in"`
		} `json:"brave"`
	}
	if err := json.Unmarshal(data, &prefs); err != nil {
		t.Fatal(err)
	}
	return prefs.Brave.Widevine
}

func TestAWebAppOpensOnlyAfterTheConsent(t *testing.T) {
	m, fa := newOpenManager(t, ":0")
	if err := os.Remove(m.ConsentFile); err != nil {
		t.Fatal(err)
	}
	if st := m.Widevine(); st.Accepted || !st.Ask || st.Updated {
		t.Errorf("before the consent: %+v, want ask (not as an update)", st)
	}
	if err := m.Install("netflix"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m)
	if err := m.Open("netflix"); !errors.Is(err, ErrWidevineConsent) {
		t.Fatalf("Open without the consent: %v, want ErrWidevineConsent", err)
	}
	if len(fa.running) != 0 {
		t.Fatalf("started %v without the consent", fa.running)
	}
	if err := m.AcceptWidevine(); err != nil {
		t.Fatal(err)
	}
	if st := m.Widevine(); !st.Accepted || st.Ask {
		t.Errorf("after the consent: %+v", st)
	}
	if err := m.Open("netflix"); err != nil {
		t.Fatal(err)
	}
	if !widevineOn(t, m, "netflix") {
		t.Error("Widevine off after the consent")
	}
}

func TestTheConsentIsAskedAgainForANewerWidevine(t *testing.T) {
	m, _ := newOpenManager(t, ":0") // accepted before any Widevine was downloaded
	// the first one Brave downloads after the consent is the accepted one
	widevineIn(t, m, "netflix", "4.10.3112.0")
	if st := m.Widevine(); st.Ask {
		t.Fatalf("the first download after the consent: %+v, want no question", st)
	}
	// an older or equal one elsewhere asks nothing; a newer one asks again
	widevineIn(t, m, "youtube", "4.9.2000.0")
	if st := m.Widevine(); st.Ask {
		t.Errorf("an older Widevine: %+v, want no question", st)
	}
	widevineIn(t, m, "spotify", "4.10.3200.0")
	if st := m.Widevine(); !st.Accepted || !st.Ask || !st.Updated {
		t.Errorf("a newer Widevine: %+v, want asked again as an update", st)
	}
	if err := m.Install("netflix"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m)
	if err := m.Open("netflix"); !errors.Is(err, ErrWidevineConsent) {
		t.Errorf("Open after an update: %v, want ErrWidevineConsent", err)
	}
	if err := m.AcceptWidevine(); err != nil {
		t.Fatal(err)
	}
	if st := m.Widevine(); st.Ask {
		t.Errorf("after accepting the new version: %+v", st)
	}
	// the consent is kept per user
	data, err := os.ReadFile(m.ConsentFile)
	if err != nil {
		t.Fatal(err)
	}
	var c consentFile
	if err := json.Unmarshal(data, &c); err != nil || c.Users[consentUser].Version != "4.10.3200.0" || c.Users[consentUser].Accepted.IsZero() {
		t.Errorf("consent file %s (%v)", data, err)
	}
}

func TestAnUnreadableConsentAsksAgain(t *testing.T) {
	m, _ := newOpenManager(t, ":0")
	if err := os.WriteFile(m.ConsentFile, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if st := m.Widevine(); !st.Ask {
		t.Errorf("unreadable consent: %+v, want the question", st)
	}
	if err := m.AcceptWidevine(); err != nil {
		t.Fatal(err)
	}
	if st := m.Widevine(); st.Ask {
		t.Errorf("after accepting again: %+v", st)
	}
}

func TestGooglesTermsOpenLiveWithoutWidevine(t *testing.T) {
	m, fa := newOpenManager(t, ":0")
	if err := os.Remove(m.ConsentFile); err != nil { // the terms are read before the consent
		t.Fatal(err)
	}
	if err := m.OpenTerms("it"); err != nil {
		t.Fatal(err)
	}
	req, ok := fa.running[AppID(WidevineTermsID)]
	if !ok || req.Kind != "webapp" {
		t.Fatalf("running %+v: want Google's terms page as a web app", fa.running)
	}
	if last := req.Command[len(req.Command)-1]; last != "https://policies.google.com/terms?hl=it" {
		t.Errorf("address %q", last)
	}
	if widevineOn(t, m, WidevineTermsID) {
		t.Error("Widevine on in the terms page's profile")
	}
	if err := fa.Close(AppID(WidevineTermsID)); err != nil {
		t.Fatal(err)
	}
	// a language that is not a language code is left out
	if err := m.OpenTerms("it&x=1"); err != nil {
		t.Fatal(err)
	}
	if last := fa.running[AppID(WidevineTermsID)].Command; last[len(last)-1] != "https://policies.google.com/terms" {
		t.Errorf("address %q", last[len(last)-1])
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"4.10.3112.0", "4.10.3112.0", 0},
		{"4.10.3200.0", "4.10.3112.0", 1},
		{"4.9.9999.9", "4.10.0.0", -1},
		{"4.10", "4.10.0.0", 0},
		{"4.10.1", "", 1},
		{"", "4.10.1", -1},
	} {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
