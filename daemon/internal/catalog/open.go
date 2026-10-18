// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

// Opening the web apps ([[step-5.1-brave-in-the-os]]): each web app runs Brave in kiosk mode with
// its own profile, in its own unit (apps.Manager), treated like a game. owneetd builds the
// command itself from the catalogue: the interface never sends programs, flags or addresses.
// Brave's settings are locked by the managed policies of owneet-base
// (/etc/brave/policies/managed/owneet.json).

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
)

// ErrNotInstalled: the web app must be installed before it is opened.
var ErrNotInstalled = errors.New("the web app is not installed")

// Launcher starts, shows and closes apps in the console session (*apps.Manager).
type Launcher interface {
	Status() apps.Status
	Launch(req apps.LaunchRequest) (apps.App, error)
	FocusApp(id string) error
	Close(id string) error
}

// AppID is the id of a running web app in apps.Manager.
func AppID(id string) string { return "web:" + id }

// profileSeed turns Widevine on (Brave downloads it at runtime, never redistributed;
// [[architecture-stack]]) and skips Brave's welcome pages. Written once, before the profile's
// first start; Brave keeps the file up to date afterwards.
var profileSeed = map[string]any{
	"brave":   map[string]any{"widevine_opted_in": true},
	"browser": map[string]any{"has_seen_welcome_page": true},
}

// seedProfile creates the profile folder of a web app the first time it is opened.
func seedProfile(dir string) error {
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	data, err := json.Marshal(profileSeed)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "Default"), 0o700); err != nil {
		return err
	}
	// Local State holds the browser-wide settings, Default/Preferences the profile's own
	for _, f := range []string{"Local State", filepath.Join("Default", "Preferences")} {
		if err := os.WriteFile(filepath.Join(dir, f), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// braveCommand runs Brave fullscreen on the web app's address, with its own profile and the
// console's extension. X11 under gamescope (which shows only X11 windows it can tag); Wayland in
// a session without an X server.
func (m *Manager) braveCommand(e Entry, profile string, sess apps.Session) []string {
	platform := "wayland"
	if sess.Display != "" {
		platform = "x11"
	}
	argv := []string{"env", "-u", "GAMESCOPE_WAYLAND_DISPLAY", "brave",
		"--user-data-dir=" + profile,
		"--kiosk",
		"--no-first-run",
		"--no-default-browser-check",
		// no keyring: a keyring would ask for a password, which needs a keyboard
		"--password-store=basic",
		"--noerrdialogs",
		"--disable-session-crashed-bubble",
		"--hide-crash-restore-bubble",
		"--ozone-platform=" + platform,
	}
	if st, err := os.Stat(m.ExtensionDir); err == nil && st.IsDir() {
		// The console's extension ([[step-5.2-extension-core]]), loaded from the system's files:
		// no store and no update server
		argv = append(argv, "--load-extension="+m.ExtensionDir,
			"--disable-features=DisableLoadExtensionCommandLineSwitch")
	}
	if e.UserAgent != "" {
		// One argument, spaces included: the command is run without a shell
		argv = append(argv, "--user-agent="+e.UserAgent)
	}
	return append(argv, e.URL)
}

func (m *Manager) profile(id string) string { return filepath.Join(m.ProfilesDir, id) }

func (m *Manager) running(id string) bool {
	if m.Apps == nil {
		return false
	}
	for _, a := range m.Apps.Status().Apps {
		if a.ID == AppID(id) {
			return true
		}
	}
	return false
}

// Open opens an installed web app, or brings it back when it is already open.
func (m *Manager) Open(id string) error {
	if m.Apps == nil || m.Session == nil {
		return apps.ErrNoSession
	}
	m.mu.Lock()
	cat, err := m.load()
	if err != nil {
		m.mu.Unlock()
		return err
	}
	e, ok := find(cat, id, KindWebApp)
	if !ok {
		m.mu.Unlock()
		return ErrUnknown
	}
	installed := app(m.status(), id).Installed
	m.mu.Unlock()
	if !installed {
		return ErrNotInstalled
	}
	if m.running(id) {
		return m.Apps.FocusApp(AppID(id))
	}
	sess, err := m.Session()
	if err != nil {
		return apps.ErrNoSession
	}
	if err := seedProfile(m.profile(id)); err != nil {
		return err
	}
	_, err = m.Apps.Launch(apps.LaunchRequest{ID: AppID(id), Name: id, Kind: apps.KindWebApp,
		Command: m.braveCommand(e, m.profile(id), sess)})
	return err
}

// closeAndForget closes the web app if it is open and deletes its profile: removing a web app
// deletes its data on the console, the login included ([[optional-apps]]).
func (m *Manager) closeAndForget(id string) {
	if m.running(id) {
		if err := m.Apps.Close(AppID(id)); err != nil {
			m.Log.Warn("web app not closed before its removal", "id", id, "err", err)
		}
		// systemd stops it within 10 seconds (SIGKILL after that)
		for deadline := time.Now().Add(15 * time.Second); m.running(id) && time.Now().Before(deadline); {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if m.ProfilesDir == "" {
		return
	}
	if err := os.RemoveAll(m.profile(id)); err != nil {
		m.Log.Warn("web app's data not deleted", "id", id, "err", err)
	}
}

func app(st Status, id string) App {
	for _, a := range st.Apps {
		if a.ID == id {
			return a
		}
	}
	return App{}
}
