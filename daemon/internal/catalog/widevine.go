// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

// The user's consent to Google's terms for Google Widevine ([[step-5.4-netflix]],
// [[legal-compliance]]). Brave downloads Widevine into each web app's profile and would ask in a
// window of its own, which the controller cannot reach (turned off: profilePrefs); the console
// asks instead, at the first opening of a web app and again whenever a newer Widevine than the
// accepted one is installed. Widevine is turned on in a profile (`widevine_opted_in`) only once
// the user has accepted, and a web app does not open before that. One consent per user: the
// console has one user today, so one entry (consentUser).

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
	"github.com/fancy-dino/OwneetOS/daemon/internal/atomicfile"
)

// ErrWidevineConsent: the user must accept Google's terms for Widevine before the web app opens.
var ErrWidevineConsent = errors.New("the user has not accepted Google's terms for Widevine")

// consentUser: the console's only user today; every person on the console gets their own entry
// once the console has people.
const consentUser = "console"

// WidevineTermsID is the page with Google's terms, opened like a web app (its own profile, never
// in the catalogue) so that the user reads the live terms, never a copy.
const WidevineTermsID = "widevine-terms"

const widevineTermsURL = "https://policies.google.com/terms"

// WidevineStatus is the body of GET /v1/webapps/widevine.
type WidevineStatus struct {
	// Accepted: the user accepted Google's terms for Widevine (perhaps for an older version)
	Accepted bool `json:"accepted"`
	// Ask: opening a web app asks first (never accepted, or Widevine was updated since)
	Ask bool `json:"ask"`
	// Updated: asked again because a newer Widevine is installed than the accepted one
	Updated bool `json:"updated"`
}

// consentFile is what ConsentFile holds.
type consentFile struct {
	Users map[string]acceptance `json:"users"`
}

type acceptance struct {
	// Version: the newest Widevine in the profiles when the user accepted; empty when there was
	// none yet, and then the first one Brave downloads afterwards is the accepted one
	Version  string    `json:"version"`
	Accepted time.Time `json:"accepted"`
}

func (m *Manager) loadConsent() (consentFile, error) {
	c := consentFile{Users: map[string]acceptance{}}
	data, err := os.ReadFile(m.ConsentFile)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	if c.Users == nil {
		c.Users = map[string]acceptance{}
	}
	return c, nil
}

func (m *Manager) saveConsent(c consentFile) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(m.ConsentFile, data, 0o600)
}

// Widevine tells whether opening a web app must first ask for the user's consent.
func (m *Manager) Widevine() WidevineStatus {
	m.consentMu.Lock()
	defer m.consentMu.Unlock()
	c, err := m.loadConsent()
	if err != nil {
		m.Log.Warn("Widevine consent not read: asking again", "err", err)
		return WidevineStatus{Ask: true}
	}
	a, ok := c.Users[consentUser]
	if !ok {
		return WidevineStatus{Ask: true}
	}
	installed := m.installedWidevine()
	if a.Version == "" && installed != "" {
		// the first Widevine downloaded after the user accepted
		a.Version = installed
		c.Users[consentUser] = a
		if err := m.saveConsent(c); err != nil {
			m.Log.Warn("Widevine version not saved", "err", err)
		}
	}
	updated := installed != "" && compareVersions(installed, a.Version) > 0
	return WidevineStatus{Accepted: true, Ask: updated, Updated: updated}
}

// AcceptWidevine records that the user accepted Google's terms for the Widevine installed now (or
// for the first one to come).
func (m *Manager) AcceptWidevine() error {
	m.consentMu.Lock()
	defer m.consentMu.Unlock()
	c, err := m.loadConsent()
	if err != nil {
		m.Log.Warn("Widevine consent not read: writing it anew", "err", err)
		c = consentFile{Users: map[string]acceptance{}}
	}
	c.Users[consentUser] = acceptance{Version: m.installedWidevine(), Accepted: time.Now().UTC()}
	return m.saveConsent(c)
}

var languageCode = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)

// OpenTerms opens Google's terms page in the web apps' window, in the user's language (e.g.
// "it"; empty: Google's choice). It runs like a web app: one game or web app at a time.
func (m *Manager) OpenTerms(language string) error {
	if m.Apps == nil || m.Session == nil {
		return apps.ErrNoSession
	}
	if m.running(WidevineTermsID) {
		return m.Apps.FocusApp(AppID(WidevineTermsID))
	}
	url := widevineTermsURL
	if languageCode.MatchString(language) {
		url += "?hl=" + language
	}
	// no Widevine in this profile: it only shows a page
	return m.launch(WidevineTermsID, Entry{URL: url}, false)
}

var widevineVersion = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)

// installedWidevine is the newest Widevine version in the web apps' profiles: Brave writes the
// folder it loads into WidevineCdm/latest-component-updated-widevine-cdm ({"Path": ".../4.10.3112.0"}).
func (m *Manager) installedWidevine() string {
	if m.ProfilesDir == "" {
		return ""
	}
	entries, err := os.ReadDir(m.ProfilesDir)
	if err != nil {
		return ""
	}
	newest := ""
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.ProfilesDir, e.Name(), "WidevineCdm", "latest-component-updated-widevine-cdm"))
		if err != nil {
			continue
		}
		var hint struct {
			Path string `json:"Path"`
		}
		if json.Unmarshal(data, &hint) != nil {
			continue
		}
		if v := filepath.Base(hint.Path); widevineVersion.MatchString(v) && compareVersions(v, newest) > 0 {
			newest = v
		}
	}
	return newest
}

// compareVersions compares dotted versions number by number ("" is the oldest): -1, 0 or 1.
func compareVersions(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" || b == "" {
		if a == "" {
			return -1
		}
		return 1
	}
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var na, nb int
		if i < len(pa) {
			na, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			nb, _ = strconv.Atoi(pb[i])
		}
		if na != nb {
			if na < nb {
				return -1
			}
			return 1
		}
	}
	return 0
}
