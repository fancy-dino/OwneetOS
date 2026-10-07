// SPDX-License-Identifier: GPL-3.0-or-later

// Package power shuts down, restarts and suspends the console through systemd-logind, which allows
// it to the active local session without a password (polkit).
package power

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

const (
	logindService = "org.freedesktop.login1"
	logindPath    = dbus.ObjectPath("/org/freedesktop/login1")
	ifaceManager  = "org.freedesktop.login1.Manager"
)

// Status is the body of GET /v1/power.
type Status struct {
	CanShutdown bool `json:"can_shutdown"`
	CanRestart  bool `json:"can_restart"`
	CanSuspend  bool `json:"can_suspend"`
}

// Errors returned to the API.
var (
	ErrUnsupported = errors.New("this computer does not support it")
	ErrNotAllowed  = errors.New("not allowed")
	ErrInhibited   = errors.New("blocked by a running program")
)

// Action is a power action: the logind method and its Can… check.
type Action struct {
	Name   string // shutdown, restart, suspend (logs and events)
	Method string
	Check  string
}

// The power actions.
var (
	Shutdown = Action{"shutdown", "PowerOff", "CanPowerOff"}
	Restart  = Action{"restart", "Reboot", "CanReboot"}
	Suspend  = Action{"suspend", "Suspend", "CanSuspend"}
)

// CheckError turns logind's answer to Can… ("yes", "no", "challenge", "na") into an error.
func CheckError(answer string) error {
	switch answer {
	case "yes":
		return nil
	case "na":
		return ErrUnsupported
	}
	return ErrNotAllowed // "no", or "challenge": a password would be needed
}

// Logind talks to systemd-logind on the system bus.
type Logind struct {
	conn   *dbus.Conn
	log    *slog.Logger
	broker *events.Broker
}

// New connects to the system bus and listens for logind's sleep and shutdown signals.
func New(log *slog.Logger, broker *events.Broker) (*Logind, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("system bus: %w", err)
	}
	for _, member := range []string{"PrepareForSleep", "PrepareForShutdown"} {
		if err := conn.AddMatchSignal(dbus.WithMatchSender(logindService),
			dbus.WithMatchInterface(ifaceManager), dbus.WithMatchMember(member)); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return &Logind{conn: conn, log: log, broker: broker}, nil
}

// Close disconnects from the bus.
func (l *Logind) Close() { l.conn.Close() }

// Run publishes power.suspending, power.resumed and power.shutting_down until ctx ends.
func (l *Logind) Run(ctx context.Context) {
	signals := make(chan *dbus.Signal, 16)
	l.conn.Signal(signals)
	defer l.conn.RemoveSignal(signals)
	for {
		select {
		case <-ctx.Done():
			return
		case sig := <-signals:
			if len(sig.Body) != 1 {
				continue
			}
			start, _ := sig.Body[0].(bool)
			switch {
			case sig.Name == ifaceManager+".PrepareForSleep" && start:
				l.log.Info("suspending")
				l.broker.Publish("power.suspending", nil)
			case sig.Name == ifaceManager+".PrepareForSleep":
				l.log.Info("resumed from suspend")
				l.broker.Publish("power.resumed", nil)
			case sig.Name == ifaceManager+".PrepareForShutdown" && start:
				l.broker.Publish("power.shutting_down", nil)
			}
		}
	}
}

func (l *Logind) can(a Action) error {
	var answer string
	if err := l.conn.Object(logindService, logindPath).Call(ifaceManager+"."+a.Check, 0).Store(&answer); err != nil {
		return err
	}
	return CheckError(answer)
}

// Status tells which actions are possible.
func (l *Logind) Status() Status {
	return Status{
		CanShutdown: l.can(Shutdown) == nil,
		CanRestart:  l.can(Restart) == nil,
		CanSuspend:  l.can(Suspend) == nil,
	}
}

// Do starts a power action. It returns once logind has accepted it.
func (l *Logind) Do(a Action) error {
	if err := l.can(a); err != nil {
		return err
	}
	l.log.Info("power action requested", "action", a.Name)
	// interactive=false: never ask for a password (nobody could type it).
	err := l.conn.Object(logindService, logindPath).Call(ifaceManager+"."+a.Method, 0, false).Err
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "inhibit") {
		return fmt.Errorf("%w: %v", ErrInhibited, err)
	}
	return err
}
