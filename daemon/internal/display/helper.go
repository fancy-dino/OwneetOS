// SPDX-License-Identifier: GPL-3.0-or-later

package display

import (
	"fmt"
	"regexp"

	"github.com/godbus/dbus/v5"
)

// SystemdHelper starts and stops owneet-screens-off@CARD.service through systemd (a polkit rule
// allows it to the console user, for these units only).
type SystemdHelper struct{ conn *dbus.Conn }

// NewSystemdHelper connects to the system bus.
func NewSystemdHelper() (*SystemdHelper, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	return &SystemdHelper{conn: conn}, nil
}

// Close disconnects from the bus.
func (h *SystemdHelper) Close() { h.conn.Close() }

var helperCard = regexp.MustCompile(`^card[0-9]+$`)

func (h *SystemdHelper) call(method, card string) error {
	if !helperCard.MatchString(card) {
		return fmt.Errorf("invalid card %q", card)
	}
	unit := "owneet-screens-off@" + card + ".service"
	var job dbus.ObjectPath
	return h.conn.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1").
		Call("org.freedesktop.systemd1.Manager."+method, 0, unit, "replace").Store(&job)
}

// Start turns off the screens of every card but keepCard.
func (h *SystemdHelper) Start(keepCard string) error { return h.call("StartUnit", keepCard) }

// Stop lets those screens go.
func (h *SystemdHelper) Stop(keepCard string) error { return h.call("StopUnit", keepCard) }
