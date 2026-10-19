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
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
	"github.com/fancy-dino/OwneetOS/daemon/internal/atomicfile"
)

// ErrWidevineConsent: the user must accept Google's terms for Widevine before the web app opens.
var ErrWidevineConsent = errors.New("the user has not accepted Google's terms for Widevine")

// ErrPreparing: a web app is getting Widevine for its first opening (one game or web app at a
// time).
var ErrPreparing = errors.New("a web app is getting ready")

// widevineFetchTimeout: how long a first opening waits for Brave to download Widevine (20 s in the
// test VM); after it the opening fails and the interface says that Widevine was not downloaded.
const widevineFetchTimeout = 60 * time.Second

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
	if m.isPreparing() {
		return ErrPreparing
	}
	url := widevineTermsURL
	if languageCode.MatchString(language) {
		url += "?hl=" + language
	}
	// no Widevine in this profile: it only shows a page
	return m.launch(WidevineTermsID, Entry{URL: url}, false)
}

// startPreparing marks a web app as getting ready; false when one already is.
func (m *Manager) startPreparing(id string) bool {
	m.prepMu.Lock()
	defer m.prepMu.Unlock()
	if m.preparing != "" {
		return false
	}
	m.preparing = id
	return true
}

func (m *Manager) donePreparing() {
	m.prepMu.Lock()
	defer m.prepMu.Unlock()
	m.preparing = ""
}

func (m *Manager) isPreparing() bool {
	m.prepMu.Lock()
	defer m.prepMu.Unlock()
	return m.preparing != ""
}

// hasWidevine: Brave has downloaded Widevine into the profile.
func hasWidevine(profile string) bool {
	_, err := os.Stat(filepath.Join(profile, "WidevineCdm", "latest-component-updated-widevine-cdm"))
	return err == nil
}

// prepareAndLaunch is a web app's first opening ([[step-5.4-netflix]]): Brave can use a Widevine
// downloaded during a session only after a restart, so it first runs with no window in the web
// app's profile until Widevine is there (about 20 s in the test VM), then the web app opens. Each
// web app does it once, in its own profile.
func (m *Manager) prepareAndLaunch(id string, e Entry) {
	defer m.donePreparing()
	start := time.Now()
	err := m.prepareProfile(id, true)
	if err == nil {
		fetch := m.FetchWidevine
		if fetch == nil {
			fetch = fetchWidevine
		}
		err = fetch(m.profile(id))
	}
	if err != nil {
		// without Widevine the web app cannot play: it does not open, the interface says why and
		// offers to try again (owner, 2026-10-19)
		m.Log.Warn("web app's first opening: Widevine not downloaded", "id", id, "err", err)
		m.openFailed(id, "apps.widevine_failed", err)
		return
	}
	m.Log.Info("web app got Widevine before its first opening", "id", id, "took", time.Since(start).Round(time.Second))
	if err := m.launch(id, e, true); err != nil {
		m.Log.Warn("web app not opened after getting ready", "id", id, "err", err)
		m.openFailed(id, "apps.failed", err)
	}
}

// openFailed tells the interface that a web app's first opening failed ("apps.open_failed"; code:
// apps.widevine_failed, Widevine was not downloaded; apps.failed, the web app did not start).
func (m *Manager) openFailed(id, code string, err error) {
	if m.Broker != nil {
		m.Broker.Publish("apps.open_failed", map[string]string{"id": AppID(id), "code": code, "error": err.Error()})
	}
}

// fetchWidevine runs Brave with no window in the profile until it has downloaded Widevine, or
// for widevineFetchTimeout at most (e.g. no internet).
func fetchWidevine(profile string) error {
	cmd := exec.Command("brave", "--headless=new", "--user-data-dir="+profile, "--no-first-run",
		"--password-store=basic", "--component-updater=fast-update", "about:blank")
	// a process group of its own: Brave's helper processes stop with it
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	defer stopGroup(cmd.Process.Pid, exited)
	deadline := time.After(widevineFetchTimeout)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			if hasWidevine(profile) {
				return nil
			}
		case err := <-exited:
			exited <- err // for stopGroup
			if hasWidevine(profile) {
				return nil
			}
			return fmt.Errorf("the hidden Brave stopped before Widevine was downloaded: %v", err)
		case <-deadline:
			return errors.New("Widevine was not downloaded in time")
		}
	}
}

// stopGroup ends the hidden Brave and its helpers: SIGTERM (Brave saves its files), SIGKILL after
// 5 seconds.
func stopGroup(pid int, exited chan error) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		<-exited
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL) // helpers still running, if any
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
