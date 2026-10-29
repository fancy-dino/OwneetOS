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
	// Date and Time now in the zone ("2026-10-29", "14:30"): the interface shows them as they are,
	// whatever time zone it started with
	Date string `json:"date"`
	Time string `json:"time"`
}

// Errors returned to the API.
var (
	// ErrUnknownZone: not a time zone of the system's database.
	ErrUnknownZone = errors.New("unknown time zone")
	// ErrBadTime: not a date and time ("2026-10-29" and "14:30"), or one before 2024.
	ErrBadTime = errors.New("not a valid date and time")
)

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
	// Now is the clock (tests set it; default: time.Now).
	Now func() time.Time
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
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
	now := m.now()
	if loc, err := time.LoadLocation(st.Zone); err == nil {
		now = now.In(loc)
	}
	st.Date, st.Time = now.Format("2006-01-02"), now.Format("15:04")
	return st
}

func validZone(zone string) bool {
	if zone == "" || zone == "Local" || strings.Contains(zone, "..") {
		return false
	}
	_, err := time.LoadLocation(zone)
	return err == nil
}

func (m *Manager) valid(zone string) bool {
	if m.ValidZone != nil {
		return m.ValidZone(zone)
	}
	return validZone(zone)
}

// SetZone sets the time zone and keeps the clock synchronised from the internet; "time.changed"
// follows.
func (m *Manager) SetZone(zone string) error {
	if !m.valid(zone) {
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

// SetTime sets the date and time by hand (date "2026-10-29", time "14:30"): without the internet
// the PC's own clock may be wrong. They are read in the zone given (the first start's city, which
// is not the console's zone yet), or in the console's zone for "". The internet's time is turned
// off for the change (systemd-timedated refuses it otherwise) and on again: once online, the clock
// sets itself. "time.changed" follows.
func (m *Manager) SetTime(date, clockTime, zone string) error {
	if zone == "" {
		zone = m.Status().Zone
	} else if !m.valid(zone) {
		return fmt.Errorf("%w: %q", ErrUnknownZone, zone)
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		loc = time.UTC
	}
	t, err := time.ParseInLocation("2006-01-02 15:04", date+" "+clockTime, loc)
	if err != nil || t.Year() < 2024 || t.Year() > 2100 {
		return fmt.Errorf("%w: %q %q", ErrBadTime, date, clockTime)
	}
	if err := m.Bus.Call("SetNTP", false, false); err != nil {
		m.Log.Warn("time synchronisation could not be turned off", "err", err)
	}
	err = m.Bus.Call("SetTime", t.UnixMicro(), false, false)
	if e := m.Bus.Call("SetNTP", true, false); e != nil {
		m.Log.Warn("time synchronisation could not be turned on again", "err", e)
	}
	if err != nil {
		return err
	}
	m.Log.Info("date and time set by hand", "time", t.Format(time.RFC3339))
	m.Broker.Publish("time.changed", m.Status())
	return nil
}

// Offset is a time zone's distance from UTC now (summer time included), for the list of cities.
type Offset struct {
	Zone   string `json:"zone"`
	Offset int    `json:"offset"` // seconds east of UTC
}

// Offsets tells how far each zone is from UTC now; unknown zones are left out.
func Offsets(zones []string, now time.Time) []Offset {
	out := []Offset{}
	for _, zone := range zones {
		if !validZone(zone) {
			continue
		}
		loc, err := time.LoadLocation(zone)
		if err != nil {
			continue
		}
		_, offset := now.In(loc).Zone()
		out = append(out, Offset{Zone: zone, Offset: offset})
	}
	return out
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
