// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	systemdService = "org.freedesktop.systemd1"
	systemdPath    = dbus.ObjectPath("/org/freedesktop/systemd1")
	ifaceManager   = "org.freedesktop.systemd1.Manager"
	ifaceUnit      = "org.freedesktop.systemd1.Unit"
	ifaceService   = "org.freedesktop.systemd1.Service"
	unitPathPrefix = "/org/freedesktop/systemd1/unit/"
)

// Systemd runs apps as transient services of the console user's systemd instance (session bus).
type Systemd struct {
	conn  *dbus.Conn
	log   *slog.Logger
	exits chan Exit

	mu   sync.Mutex
	jobs map[string]chan string // unit -> result of its start job
	// seen: units seen active. A new transient unit may report "inactive" before it starts:
	// only units seen active can stop.
	seen map[string]bool
}

// NewSystemd connects to the user's systemd and starts listening for job and unit changes.
func NewSystemd(log *slog.Logger) (*Systemd, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("session bus: %w", err)
	}
	for _, opts := range [][]dbus.MatchOption{
		{dbus.WithMatchSender(systemdService), dbus.WithMatchInterface(ifaceManager), dbus.WithMatchMember("JobRemoved")},
		{dbus.WithMatchSender(systemdService), dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
			dbus.WithMatchMember("PropertiesChanged"), dbus.WithMatchPathNamespace(dbus.ObjectPath(strings.TrimSuffix(unitPathPrefix, "/")))},
	} {
		if err := conn.AddMatchSignal(opts...); err != nil {
			conn.Close()
			return nil, err
		}
	}
	s := &Systemd{conn: conn, log: log, exits: make(chan Exit, 16), jobs: map[string]chan string{}, seen: map[string]bool{}}
	if err := s.manager().Call(ifaceManager+".Subscribe", 0).Err; err != nil {
		conn.Close()
		return nil, err
	}
	signals := make(chan *dbus.Signal, 128)
	conn.Signal(signals)
	go s.listen(signals)
	return s, nil
}

func (s *Systemd) manager() dbus.BusObject { return s.conn.Object(systemdService, systemdPath) }

// Close disconnects from the bus.
func (s *Systemd) Close() { s.conn.Close() }

// Exited reports app units that stopped.
func (s *Systemd) Exited() <-chan Exit { return s.exits }

func (s *Systemd) listen(signals <-chan *dbus.Signal) {
	for sig := range signals {
		switch sig.Name {
		case ifaceManager + ".JobRemoved": // (id, job, unit, result)
			if len(sig.Body) == 4 {
				unit, _ := sig.Body[2].(string)
				result, _ := sig.Body[3].(string)
				s.mu.Lock()
				if ch, ok := s.jobs[unit]; ok {
					ch <- result
					delete(s.jobs, unit)
				}
				s.mu.Unlock()
			}
		case "org.freedesktop.DBus.Properties.PropertiesChanged": // (interface, changed, invalidated)
			if len(sig.Body) < 2 || sig.Body[0] != ifaceUnit {
				continue
			}
			changed, _ := sig.Body[1].(map[string]dbus.Variant)
			state, _ := changed["ActiveState"].Value().(string)
			unit := unescapeBusPath(strings.TrimPrefix(string(sig.Path), unitPathPrefix))
			if !strings.HasPrefix(unit, "owneet-app-") || state == "" {
				continue
			}
			s.mu.Lock()
			wasActive := s.seen[unit]
			switch state {
			case "activating", "active", "deactivating", "reloading":
				s.seen[unit] = true
			default: // inactive, failed
				delete(s.seen, unit)
			}
			s.mu.Unlock()
			if wasActive && (state == "inactive" || state == "failed") {
				s.stopped(sig.Path, unit, state)
			}
		}
	}
}

// stopped reports a stopped unit; a failed one is reset so that systemd forgets it.
func (s *Systemd) stopped(path dbus.ObjectPath, unit, state string) {
	result := "success"
	if state == "failed" {
		if v, err := s.conn.Object(systemdService, path).GetProperty(ifaceService + ".Result"); err == nil {
			result, _ = v.Value().(string)
		}
		s.manager().Call(ifaceManager+".ResetFailedUnit", 0, unit)
	}
	s.exits <- Exit{Unit: unit, Result: result}
}

// unescapeBusPath decodes systemd's object path escaping (_XX for every other byte).
func unescapeBusPath(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '_' && i+2 < len(s) {
			if c, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(c))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

type property struct {
	Name  string
	Value dbus.Variant
}

type execCommand struct {
	Path          string
	Args          []string
	IgnoreFailure bool
}

// Start starts a transient service and waits until the program has been executed.
func (s *Systemd) Start(unit, description string, argv, env []string, workdir string) error {
	if workdir == "" {
		workdir, _ = os.UserHomeDir()
	}
	props := []property{
		{"Description", dbus.MakeVariant(description)},
		// exec: the start fails if the program cannot be executed.
		{"Type", dbus.MakeVariant("exec")},
		{"ExecStart", dbus.MakeVariant([]execCommand{{Path: argv[0], Args: argv}})},
		{"Environment", dbus.MakeVariant(env)},
		{"WorkingDirectory", dbus.MakeVariant(workdir)},
		// Closing stops every process of the app; after 10 s they are killed.
		{"KillMode", dbus.MakeVariant("control-group")},
		{"TimeoutStopUSec", dbus.MakeVariant(uint64(10 * time.Second / time.Microsecond))},
	}
	done := make(chan string, 1)
	s.mu.Lock()
	s.jobs[unit] = done
	s.mu.Unlock()
	aux := []struct {
		Name  string
		Props []property
	}{}
	if err := s.manager().Call(ifaceManager+".StartTransientUnit", 0, unit, "fail", props, aux).Err; err != nil {
		s.mu.Lock()
		delete(s.jobs, unit)
		s.mu.Unlock()
		return fmt.Errorf("cannot start %s: %w", unit, err)
	}
	select {
	case result := <-done:
		if result != "done" {
			return fmt.Errorf("cannot start %s: %s (journalctl --user -u %s)", unit, result, unit)
		}
		return nil
	case <-time.After(30 * time.Second):
		return fmt.Errorf("cannot start %s: timeout", unit)
	}
}

// Stop stops a unit: SIGTERM to all its processes, SIGKILL after the stop timeout.
func (s *Systemd) Stop(unit string) error {
	return s.manager().Call(ifaceManager+".StopUnit", 0, unit, "replace").Err
}

// Kill sends SIGKILL to every process of a unit.
func (s *Systemd) Kill(unit string) error {
	return s.manager().Call(ifaceManager+".KillUnit", 0, unit, "all", int32(9)).Err
}

// Running lists the active app units with their descriptions.
func (s *Systemd) Running() (map[string]string, error) {
	var units []struct {
		Name, Description, Load, Active, Sub, Following string
		Path                                            dbus.ObjectPath
		JobID                                           uint32
		JobType                                         string
		JobPath                                         dbus.ObjectPath
	}
	err := s.manager().Call(ifaceManager+".ListUnitsByPatterns", 0,
		[]string{"active", "activating"}, []string{"owneet-app-*.service"}).Store(&units)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range units {
		out[u.Name] = u.Description
		s.seen[u.Name] = true
	}
	return out, nil
}
