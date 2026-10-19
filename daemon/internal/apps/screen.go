// SPDX-License-Identifier: GPL-3.0-or-later

package apps

// For the controller pointer ([[step-4.7-controller-pointer]]): which window it follows. The home
// screen for the console's keyboard: ShowHome (apps.go), shared with the web apps.

// AppIDOf is the gamescope app id of a running app's windows, 0 when it does not run.
func (m *Manager) AppIDOf(id string) uint32 { return m.appIDOf(id) }
