// SPDX-License-Identifier: GPL-3.0-or-later

package apps

// For the controller pointer ([[step-4.7-controller-pointer]]): which window it follows, and the
// home screen for the console's keyboard.

// AppIDOf is the gamescope app id of a running app's windows, 0 when it does not run.
func (m *Manager) AppIDOf(id string) uint32 { return m.appIDOf(id) }

// ShowHome brings the home screen to the front, as Guide does over an app (Guide or Resume goes
// back to it); nothing when the home screen is on screen already, or outside gamescope.
func (m *Manager) ShowHome() {
	sess, err := m.Session()
	if err != nil {
		return
	}
	f := m.focus(sess)
	if f == nil {
		return
	}
	if focused, err := f.Focused(); err == nil && focused != HomeAppID {
		m.toggle(sess)
	}
}
