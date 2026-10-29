// SPDX-License-Identifier: GPL-3.0-or-later

// Package clock reads and sets the console's time zone through systemd-timedated, which polkit
// allows to the console user (50-owneet-time.rules). The first start asks for it, Settings
// changes it ([[step-6.1-first-boot-flow]], [[first-boot-setup]]); until then the system runs on
// UTC. The time itself comes from the internet (systemd-timesyncd), never the place: no
// geolocation.
package clock

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

const (
	timedateService = "org.freedesktop.timedate1"
	timedatePath    = dbus.ObjectPath("/org/freedesktop/timedate1")
	ifaceTimedate   = "org.freedesktop.timedate1"
)

// Status is the body of GET /v1/time.
type Status struct {
	Zone string `json:"zone"` // e.g. Europe/Rome; "UTC" until one is chosen
	// Synchronized: the clock has been set from the internet
	Synchronized bool `json:"synchronized"`
}

// ErrUnknownZone: not a time zone of the system's database.
var ErrUnknownZone = errors.New("unknown time zone")

// Bus is the part of systemd-timedated the manager uses (a fake in tests).
type Bus interface {
	Property(name string) (dbus.Variant, error)
	Call(method string, args ...any) error
}

// Manager reads and sets the time zone.
type Manager struct {
	Bus    Bus
	Broker *events.Broker
	Log    *slog.Logger
	// ValidZone tells whether a name is in the time zone database (default: time.LoadLocation).
	ValidZone func(string) bool
}

// Status reads the time zone and whether the clock is synchronised.
func (m *Manager) Status() Status {
	st := Status{Zone: "UTC"}
	if v, err := m.Bus.Property("Timezone"); err == nil {
		if zone, ok := v.Value().(string); ok && zone != "" {
			st.Zone = zone
		}
	}
	if v, err := m.Bus.Property("NTPSynchronized"); err == nil {
		st.Synchronized, _ = v.Value().(bool)
	}
	return st
}

func validZone(zone string) bool {
	if zone == "" || zone == "Local" || strings.Contains(zone, "..") {
		return false
	}
	_, err := time.LoadLocation(zone)
	return err == nil
}

// SetZone sets the time zone and keeps the clock synchronised from the internet; "time.changed"
// follows.
func (m *Manager) SetZone(zone string) error {
	valid := m.ValidZone
	if valid == nil {
		valid = validZone
	}
	if !valid(zone) {
		return fmt.Errorf("%w: %q", ErrUnknownZone, zone)
	}
	if err := m.Bus.Call("SetTimezone", zone, false); err != nil {
		return err
	}
	// The time comes from the internet (it fails without systemd-timesyncd: the zone is still set)
	if err := m.Bus.Call("SetNTP", true, false); err != nil {
		m.Log.Warn("time synchronisation could not be turned on", "err", err)
	}
	m.Log.Info("time zone set", "zone", zone)
	m.Broker.Publish("time.changed", m.Status())
	return nil
}

// Timedated is systemd-timedated on the system bus.
type Timedated struct {
	conn *dbus.Conn
	obj  dbus.BusObject
}

// NewTimedated connects to the system bus (timedated itself starts when first called).
func NewTimedated() (*Timedated, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("system bus: %w", err)
	}
	return &Timedated{conn: conn, obj: conn.Object(timedateService, timedatePath)}, nil
}

// Close disconnects from the bus.
func (t *Timedated) Close() { t.conn.Close() }

// Property reads a property of timedated.
func (t *Timedated) Property(name string) (dbus.Variant, error) {
	return t.obj.GetProperty(ifaceTimedate + "." + name)
}

// Call calls a method of timedated.
func (t *Timedated) Call(method string, args ...any) error {
	return t.obj.Call(ifaceTimedate+"."+method, 0, args...).Err
}
