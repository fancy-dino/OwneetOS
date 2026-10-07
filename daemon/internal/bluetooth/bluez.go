// SPDX-License-Identifier: GPL-3.0-or-later

package bluetooth

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	bluezService  = "org.bluez"
	ifaceAdapter  = "org.bluez.Adapter1"
	ifaceDevice   = "org.bluez.Device1"
	ifaceBattery  = "org.bluez.Battery1"
	ifaceAgentMgr = "org.bluez.AgentManager1"
	ifaceAgent    = "org.bluez.Agent1"
	ifaceProps    = "org.freedesktop.DBus.Properties"
	ifaceObjMgr   = "org.freedesktop.DBus.ObjectManager"
	agentPath     = dbus.ObjectPath("/org/owneet/bluetooth/agent")

	// noStart: never ask D-Bus to start bluetoothd. systemd starts it when an adapter is present
	// (bluetooth.target); asking without an adapter only fills the journal with failed attempts.
	noStart = dbus.FlagNoAutoStart
)

// BlueZ is the real Backend, on the D-Bus system bus.
type BlueZ struct {
	conn    *dbus.Conn
	log     *slog.Logger
	changes chan struct{}

	mu      sync.Mutex
	adapter dbus.ObjectPath
	devices map[string]dbus.ObjectPath // address -> object path
}

// NewBlueZ connects to the system bus, registers the pairing agent (decisions delegated to
// policy) and starts listening for BlueZ changes. bluetoothd does not need to be running yet.
func NewBlueZ(log *slog.Logger, policy AgentPolicy) (*BlueZ, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("system bus: %w", err)
	}
	b := &BlueZ{conn: conn, log: log, changes: make(chan struct{}, 1), devices: map[string]dbus.ObjectPath{}}

	if err := conn.AddMatchSignal(dbus.WithMatchSender(bluezService)); err != nil {
		conn.Close()
		return nil, err
	}
	// bluetoothd may start after owneetd, or restart: register the agent again when it appears.
	if err := conn.AddMatchSignal(dbus.WithMatchSender("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, bluezService)); err != nil {
		conn.Close()
		return nil, err
	}
	if err := conn.Export(&agent{b: b, policy: policy}, agentPath, ifaceAgent); err != nil {
		conn.Close()
		return nil, err
	}
	signals := make(chan *dbus.Signal, 64)
	conn.Signal(signals)
	go func() {
		for sig := range signals {
			if sig.Name == "org.freedesktop.DBus.NameOwnerChanged" {
				if len(sig.Body) == 3 && sig.Body[2] != "" {
					b.registerAgent()
				}
			}
			select {
			case b.changes <- struct{}{}:
			default: // a refresh is already pending
			}
		}
	}()
	b.registerAgent()
	return b, nil
}

// registerAgent makes the pairing agent BlueZ's default agent.
func (b *BlueZ) registerAgent() {
	mgr := b.conn.Object(bluezService, "/org/bluez")
	if err := mgr.Call(ifaceAgentMgr+".RegisterAgent", noStart, agentPath, "NoInputNoOutput").Err; err != nil {
		var dbusErr dbus.Error
		if errors.As(err, &dbusErr) && (dbusErr.Name == "org.freedesktop.DBus.Error.NameHasNoOwner" ||
			dbusErr.Name == "org.freedesktop.DBus.Error.ServiceUnknown") {
			b.log.Info("bluetoothd is not running (no adapter?): the pairing agent registers when it starts")
		} else {
			b.log.Warn("cannot register the Bluetooth pairing agent", "err", err)
		}
		return
	}
	if err := mgr.Call(ifaceAgentMgr+".RequestDefaultAgent", noStart, agentPath).Err; err != nil {
		b.log.Warn("cannot become the default Bluetooth agent", "err", err)
		return
	}
	b.log.Info("Bluetooth pairing agent registered")
}

// Close disconnects from the bus.
func (b *BlueZ) Close() { b.conn.Close() }

// Changes signals BlueZ activity.
func (b *BlueZ) Changes() <-chan struct{} { return b.changes }

// Snapshot reads every adapter and device from BlueZ's object manager.
func (b *BlueZ) Snapshot() (State, error) {
	var objs map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	if err := b.conn.Object(bluezService, "/").Call(ifaceObjMgr+".GetManagedObjects", noStart).Store(&objs); err != nil {
		return State{}, err
	}
	var st State
	var adapter dbus.ObjectPath
	devices := map[string]dbus.ObjectPath{}
	for path, ifaces := range objs {
		if props, ok := ifaces[ifaceAdapter]; ok && (adapter == "" || path < adapter) {
			adapter = path
			st.Adapter = true
			st.Powered = boolProp(props, "Powered")
			st.Discovering = boolProp(props, "Discovering")
		}
	}
	for path, ifaces := range objs {
		props, ok := ifaces[ifaceDevice]
		if !ok || !strings.HasPrefix(string(path), string(adapter)+"/") {
			continue
		}
		d := Device{
			Address:    stringProp(props, "Address"),
			Name:       stringProp(props, "Alias"),
			Paired:     boolProp(props, "Paired"),
			Trusted:    boolProp(props, "Trusted"),
			Connected:  boolProp(props, "Connected"),
			Icon:       stringProp(props, "Icon"),
			Class:      uint32Prop(props, "Class"),
			Appearance: uint16Prop(props, "Appearance"),
		}
		if bat, ok := ifaces[ifaceBattery]; ok {
			if v, ok := bat["Percentage"].Value().(byte); ok {
				pct := int(v)
				d.Battery = &pct
			}
		}
		devices[strings.ToUpper(d.Address)] = path
		st.Devices = append(st.Devices, d)
	}
	b.mu.Lock()
	b.adapter, b.devices = adapter, devices
	b.mu.Unlock()
	return st, nil
}

func boolProp(p map[string]dbus.Variant, k string) bool     { v, _ := p[k].Value().(bool); return v }
func stringProp(p map[string]dbus.Variant, k string) string { v, _ := p[k].Value().(string); return v }
func uint32Prop(p map[string]dbus.Variant, k string) uint32 { v, _ := p[k].Value().(uint32); return v }
func uint16Prop(p map[string]dbus.Variant, k string) uint16 { v, _ := p[k].Value().(uint16); return v }

func (b *BlueZ) adapterObj() (dbus.BusObject, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.adapter == "" {
		return nil, ErrNoAdapter
	}
	return b.conn.Object(bluezService, b.adapter), nil
}

func (b *BlueZ) deviceObj(address string) (dbus.BusObject, dbus.ObjectPath, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	path, ok := b.devices[strings.ToUpper(address)]
	if !ok {
		return nil, "", ErrUnknownDevice
	}
	return b.conn.Object(bluezService, path), path, nil
}

// SetPowered turns the adapter on or off.
func (b *BlueZ) SetPowered(on bool) error {
	a, err := b.adapterObj()
	if err != nil {
		return err
	}
	return a.Call(ifaceProps+".Set", noStart, ifaceAdapter, "Powered", dbus.MakeVariant(on)).Err
}

// SetDiscovery starts or stops scanning for devices.
func (b *BlueZ) SetDiscovery(on bool) error {
	a, err := b.adapterObj()
	if err != nil {
		return err
	}
	if on {
		return a.Call(ifaceAdapter+".StartDiscovery", noStart).Err
	}
	return a.Call(ifaceAdapter+".StopDiscovery", noStart).Err
}

// Pair pairs with a device (an already paired device is not an error).
func (b *BlueZ) Pair(address string) error {
	d, _, err := b.deviceObj(address)
	if err != nil {
		return err
	}
	if err := d.Call(ifaceDevice+".Pair", noStart).Err; err != nil && !strings.Contains(err.Error(), "AlreadyExists") {
		return err
	}
	return nil
}

// SetTrusted marks a device as trusted, so it reconnects by itself later.
func (b *BlueZ) SetTrusted(address string, on bool) error {
	d, _, err := b.deviceObj(address)
	if err != nil {
		return err
	}
	return d.Call(ifaceProps+".Set", noStart, ifaceDevice, "Trusted", dbus.MakeVariant(on)).Err
}

// Connect connects a device.
func (b *BlueZ) Connect(address string) error {
	d, _, err := b.deviceObj(address)
	if err != nil {
		return err
	}
	return d.Call(ifaceDevice+".Connect", noStart).Err
}

// Disconnect disconnects a device.
func (b *BlueZ) Disconnect(address string) error {
	d, _, err := b.deviceObj(address)
	if err != nil {
		return err
	}
	return d.Call(ifaceDevice+".Disconnect", noStart).Err
}

// Remove forgets a device (unpairs it).
func (b *BlueZ) Remove(address string) error {
	_, path, err := b.deviceObj(address)
	if err != nil {
		return err
	}
	a, err := b.adapterObj()
	if err != nil {
		return err
	}
	return a.Call(ifaceAdapter+".RemoveDevice", noStart, path).Err
}

func (b *BlueZ) addressOf(path dbus.ObjectPath) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	for addr, p := range b.devices {
		if p == path {
			return addr
		}
	}
	return ""
}

// AgentPolicy decides on pairing requests (implemented by Manager).
type AgentPolicy interface {
	AllowPairing(address string) bool
	IsTrusted(address string) bool
}

// agent implements org.bluez.Agent1 with the "NoInputNoOutput" capability: there is nobody to
// type or compare a code, so requests are accepted only for gamepads while auto-pair is on.
type agent struct {
	b      *BlueZ
	policy AgentPolicy
}

var errRejected = dbus.NewError("org.bluez.Error.Rejected", []any{"rejected by OwneetOS"})

func (a *agent) allowed(dev dbus.ObjectPath) bool {
	if a.b.addressOf(dev) == "" {
		a.b.Snapshot() // the device may be new
	}
	addr := a.b.addressOf(dev)
	ok := addr != "" && a.policy.AllowPairing(addr)
	a.b.log.Info("bluetooth pairing request", "address", addr, "accepted", ok)
	return ok
}

func (a *agent) Release() *dbus.Error { return nil }
func (a *agent) Cancel() *dbus.Error  { return nil }

func (a *agent) RequestPinCode(dev dbus.ObjectPath) (string, *dbus.Error) {
	if a.allowed(dev) {
		return "0000", nil // legacy gamepads use 0000
	}
	return "", errRejected
}

func (a *agent) DisplayPinCode(dev dbus.ObjectPath, pin string) *dbus.Error { return nil }

func (a *agent) RequestPasskey(dev dbus.ObjectPath) (uint32, *dbus.Error) {
	if a.allowed(dev) {
		return 0, nil
	}
	return 0, errRejected
}

func (a *agent) DisplayPasskey(dev dbus.ObjectPath, passkey uint32, entered uint16) *dbus.Error {
	return nil
}

func (a *agent) RequestConfirmation(dev dbus.ObjectPath, passkey uint32) *dbus.Error {
	if a.allowed(dev) {
		return nil
	}
	return errRejected
}

func (a *agent) RequestAuthorization(dev dbus.ObjectPath) *dbus.Error {
	if a.allowed(dev) {
		return nil
	}
	return errRejected
}

func (a *agent) AuthorizeService(dev dbus.ObjectPath, uuid string) *dbus.Error {
	if addr := a.b.addressOf(dev); addr != "" && (a.policy.IsTrusted(addr) || a.policy.AllowPairing(addr)) {
		return nil
	}
	return errRejected
}
