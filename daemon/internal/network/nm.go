// SPDX-License-Identifier: GPL-3.0-or-later

package network

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

const (
	nmService     = "org.freedesktop.NetworkManager"
	nmPath        = dbus.ObjectPath("/org/freedesktop/NetworkManager")
	settingsPath  = dbus.ObjectPath("/org/freedesktop/NetworkManager/Settings")
	ifaceNM       = "org.freedesktop.NetworkManager"
	ifaceDevice   = "org.freedesktop.NetworkManager.Device"
	ifaceWireless = "org.freedesktop.NetworkManager.Device.Wireless"
	ifaceAP       = "org.freedesktop.NetworkManager.AccessPoint"
	ifaceActive   = "org.freedesktop.NetworkManager.Connection.Active"
	ifaceSettings = "org.freedesktop.NetworkManager.Settings"
	ifaceConn     = "org.freedesktop.NetworkManager.Settings.Connection"
	ifaceProps    = "org.freedesktop.DBus.Properties"

	// noStart: owneetd never starts NetworkManager itself.
	noStart = dbus.FlagNoAutoStart

	deviceTypeEthernet = 1
	deviceTypeWifi     = 2

	deviceStateActivated = 100
	deviceStateFailed    = 120

	activeStateActivated   = 2
	activeStateDeactivated = 4
)

// ConnectTimeout bounds a Wi-Fi connection attempt (association, password check, DHCP).
const ConnectTimeout = 60 * time.Second

// NM talks to NetworkManager on the D-Bus system bus.
type NM struct {
	conn   *dbus.Conn
	log    *slog.Logger
	broker *events.Broker

	// connectMu: one connection attempt at a time.
	connectMu sync.Mutex

	mu        sync.Mutex
	last      Status
	lastScan  int64
	scanKnown bool
}

type device struct {
	Path      dbus.ObjectPath
	Interface string
	Type      uint32
	State     uint32
	ActiveAP  dbus.ObjectPath
	LastScan  int64
}

// NewNM connects to the system bus. NetworkManager does not need to be running yet.
func NewNM(log *slog.Logger, broker *events.Broker) (*NM, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("system bus: %w", err)
	}
	if err := conn.AddMatchSignal(dbus.WithMatchSender(nmService)); err != nil {
		conn.Close()
		return nil, err
	}
	return &NM{conn: conn, log: log, broker: broker}, nil
}

// Close disconnects from the bus.
func (n *NM) Close() { n.conn.Close() }

// Run publishes network.changed and wifi.scan_done events until ctx ends.
func (n *NM) Run(ctx context.Context) {
	signals := make(chan *dbus.Signal, 128)
	n.conn.Signal(signals)
	defer n.conn.RemoveSignal(signals)
	n.refresh()
	// NetworkManager sends bursts of signals: refresh at most every 300 ms.
	var timer <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-signals:
			if timer == nil {
				timer = time.After(300 * time.Millisecond)
			}
		case <-timer:
			timer = nil
			n.refresh()
		}
	}
}

func (n *NM) refresh() {
	st := n.Status()
	var scan int64
	if dev, err := n.wifiDevice(); err == nil {
		scan = dev.LastScan
	}
	n.mu.Lock()
	changed := !reflect.DeepEqual(st, n.last)
	scanned := n.scanKnown && scan != n.lastScan
	n.last, n.lastScan, n.scanKnown = st, scan, true
	n.mu.Unlock()
	if changed {
		n.broker.Publish("network.changed", st)
	}
	if scanned {
		n.broker.Publish("wifi.scan_done", nil)
	}
}

func (n *NM) getAll(path dbus.ObjectPath, iface string) (map[string]dbus.Variant, error) {
	var props map[string]dbus.Variant
	err := n.conn.Object(nmService, path).Call(ifaceProps+".GetAll", noStart, iface).Store(&props)
	return props, err
}

func u32(p map[string]dbus.Variant, k string) uint32 { v, _ := p[k].Value().(uint32); return v }
func str(p map[string]dbus.Variant, k string) string { v, _ := p[k].Value().(string); return v }
func boolean(p map[string]dbus.Variant, k string) bool {
	v, _ := p[k].Value().(bool)
	return v
}
func objPath(p map[string]dbus.Variant, k string) dbus.ObjectPath {
	v, _ := p[k].Value().(dbus.ObjectPath)
	return v
}

func stateName(s uint32) string {
	switch {
	case s >= 50: // NM_STATE_CONNECTED_LOCAL, _SITE, _GLOBAL
		return "connected"
	case s == 40:
		return "connecting"
	case s == 20 || s == 30:
		return "disconnected"
	case s == 10:
		return "asleep"
	}
	return "unknown"
}

func connectivityName(c uint32) string {
	return [...]string{"unknown", "none", "portal", "limited", "full"}[min(c, 4)]
}

func wifiStateName(s uint32) string {
	switch {
	case s == deviceStateActivated:
		return "connected"
	case s >= 40 && s < deviceStateActivated: // prepare … secondaries
		return "connecting"
	case s < 30: // unmanaged, unavailable (radio off)
		return "unavailable"
	}
	return "disconnected"
}

// Status reports NetworkManager's state, the Wi-Fi adapter and the wired connection.
func (n *NM) Status() Status {
	props, err := n.getAll(nmPath, ifaceNM)
	if err != nil {
		return Status{State: "unknown", Connectivity: "unknown", Wifi: WifiStatus{State: "unavailable"}}
	}
	st := Status{
		Available:    true,
		State:        stateName(u32(props, "State")),
		Connectivity: connectivityName(u32(props, "Connectivity")),
		Wifi: WifiStatus{
			Enabled:         boolean(props, "WirelessEnabled"),
			HardwareEnabled: boolean(props, "WirelessHardwareEnabled"),
			State:           "unavailable",
		},
	}
	devs, _ := n.devices()
	for _, d := range devs {
		switch d.Type {
		case deviceTypeEthernet:
			st.Ethernet.Present = true
			st.Ethernet.Connected = st.Ethernet.Connected || d.State == deviceStateActivated
		case deviceTypeWifi:
			if st.Wifi.Present {
				continue // only the first adapter
			}
			st.Wifi.Present = true
			st.Wifi.State = wifiStateName(d.State)
			if ap, err := n.accessPoint(d.ActiveAP); err == nil && d.State == deviceStateActivated {
				strength := ap.Strength
				st.Wifi.SSID, st.Wifi.Strength = ap.SSID, &strength
			}
		}
	}
	return st
}

// devices lists Ethernet and Wi-Fi devices, sorted by interface name.
func (n *NM) devices() ([]device, error) {
	var paths []dbus.ObjectPath
	if err := n.conn.Object(nmService, nmPath).Call(ifaceNM+".GetDevices", noStart).Store(&paths); err != nil {
		return nil, ErrUnavailable
	}
	var out []device
	for _, p := range paths {
		props, err := n.getAll(p, ifaceDevice)
		if err != nil {
			continue
		}
		d := device{Path: p, Interface: str(props, "Interface"), Type: u32(props, "DeviceType"), State: u32(props, "State")}
		if d.Type != deviceTypeEthernet && d.Type != deviceTypeWifi {
			continue
		}
		if d.Type == deviceTypeWifi {
			if w, err := n.getAll(p, ifaceWireless); err == nil {
				d.ActiveAP = objPath(w, "ActiveAccessPoint")
				d.LastScan, _ = w["LastScan"].Value().(int64)
			}
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Interface < out[j].Interface })
	return out, nil
}

func (n *NM) wifiDevice() (device, error) {
	devs, err := n.devices()
	if err != nil {
		return device{}, err
	}
	for _, d := range devs {
		if d.Type == deviceTypeWifi {
			return d, nil
		}
	}
	return device{}, ErrNoWifi
}

func (n *NM) accessPoint(p dbus.ObjectPath) (AccessPoint, error) {
	if p == "" || p == "/" {
		return AccessPoint{}, ErrNotFound
	}
	props, err := n.getAll(p, ifaceAP)
	if err != nil {
		return AccessPoint{}, err
	}
	ssid, _ := props["Ssid"].Value().([]byte)
	strength, _ := props["Strength"].Value().(byte)
	return AccessPoint{
		Path:     string(p),
		SSID:     string(ssid),
		Strength: int(strength),
		Security: Classify(u32(props, "Flags"), u32(props, "WpaFlags"), u32(props, "RsnFlags")),
	}, nil
}

func (n *NM) accessPoints(dev device) ([]AccessPoint, error) {
	var paths []dbus.ObjectPath
	if err := n.conn.Object(nmService, dev.Path).Call(ifaceWireless+".GetAllAccessPoints", noStart).Store(&paths); err != nil {
		return nil, err
	}
	var out []AccessPoint
	for _, p := range paths {
		if ap, err := n.accessPoint(p); err == nil {
			out = append(out, ap)
		}
	}
	return out, nil
}

// savedWifi returns the saved Wi-Fi client connections by network name (access point and ad-hoc
// connections, e.g. a hotspot, are left out).
func (n *NM) savedWifi() (map[string][]dbus.ObjectPath, error) {
	var paths []dbus.ObjectPath
	if err := n.conn.Object(nmService, settingsPath).Call(ifaceSettings+".ListConnections", noStart).Store(&paths); err != nil {
		return nil, ErrUnavailable
	}
	out := map[string][]dbus.ObjectPath{}
	for _, p := range paths {
		var s map[string]map[string]dbus.Variant
		if err := n.conn.Object(nmService, p).Call(ifaceConn+".GetSettings", noStart).Store(&s); err != nil {
			continue
		}
		if str(s["connection"], "type") != "802-11-wireless" {
			continue
		}
		if mode := str(s["802-11-wireless"], "mode"); mode != "" && mode != "infrastructure" {
			continue
		}
		ssid, _ := s["802-11-wireless"]["ssid"].Value().([]byte)
		if len(ssid) > 0 {
			out[string(ssid)] = append(out[string(ssid)], p)
		}
	}
	return out, nil
}

// Networks lists the visible Wi-Fi networks.
func (n *NM) Networks() ([]Network, error) {
	dev, err := n.wifiDevice()
	if err != nil {
		return nil, err
	}
	aps, err := n.accessPoints(dev)
	if err != nil {
		return nil, err
	}
	saved, err := n.savedWifi()
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for ssid := range saved {
		known[ssid] = true
	}
	connected := ""
	if ap, err := n.accessPoint(dev.ActiveAP); err == nil && dev.State == deviceStateActivated {
		connected = ap.SSID
	}
	return Aggregate(aps, known, connected), nil
}

func (n *NM) wifiOn() error {
	props, err := n.getAll(nmPath, ifaceNM)
	if err != nil {
		return ErrUnavailable
	}
	if !boolean(props, "WirelessEnabled") || !boolean(props, "WirelessHardwareEnabled") {
		return ErrWifiDisabled
	}
	return nil
}

// Scan asks for a new Wi-Fi scan; wifi.scan_done follows.
func (n *NM) Scan() error {
	dev, err := n.wifiDevice()
	if err != nil {
		return err
	}
	if err := n.wifiOn(); err != nil {
		return err
	}
	err = n.conn.Object(nmService, dev.Path).Call(ifaceWireless+".RequestScan", noStart, map[string]dbus.Variant{}).Err
	if err != nil && strings.Contains(err.Error(), "not allowed") {
		return nil // a scan is already running or just finished
	}
	return err
}

// Connect connects to a Wi-Fi network and waits for the result. Without a password, a saved
// network is reused; with one, a new saved network replaces the old one only if it works.
func (n *NM) Connect(ctx context.Context, ssid, password string) error {
	n.connectMu.Lock()
	defer n.connectMu.Unlock()

	dev, err := n.wifiDevice()
	if err != nil {
		return err
	}
	if err := n.wifiOn(); err != nil {
		return err
	}
	aps, err := n.accessPoints(dev)
	if err != nil {
		return err
	}
	ap, ok := BestAccessPoint(aps, ssid)
	if !ok {
		return ErrNotFound
	}
	if !Supported(ap.Security) {
		return ErrUnsupportedSecurity
	}
	saved, err := n.savedWifi()
	if err != nil {
		return err
	}
	old := saved[ssid]

	nm := n.conn.Object(nmService, nmPath)
	var created, active dbus.ObjectPath
	if password == "" && len(old) > 0 {
		err = nm.Call(ifaceNM+".ActivateConnection", noStart, old[0], dev.Path, dbus.ObjectPath(ap.Path)).Store(&active)
	} else {
		if NeedsPassword(ap.Security) {
			if password == "" {
				return ErrPasswordRequired
			}
			if err := ValidatePassword(ap.Security, password); err != nil {
				return err
			}
		}
		err = nm.Call(ifaceNM+".AddAndActivateConnection", noStart, connectionSettings(ssid, ap.Security, password),
			dev.Path, dbus.ObjectPath(ap.Path)).Store(&created, &active)
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrConnectFailed, err)
	}
	n.log.Info("connecting to Wi-Fi", "ssid", ssid, "security", ap.Security, "saved", created == "")
	n.broker.Publish("network.connecting", map[string]string{"ssid": ssid})

	if err := n.waitActivated(ctx, dev.Path, active); err != nil {
		if created != "" { // do not keep a network saved with a wrong password
			n.deleteConnection(created)
		}
		n.log.Warn("Wi-Fi connection failed", "ssid", ssid, "err", err)
		n.broker.Publish("network.connect_failed", map[string]string{"ssid": ssid, "error": errorCode(err)})
		return err
	}
	if created != "" {
		for _, p := range old {
			n.deleteConnection(p)
		}
	}
	n.log.Info("connected to Wi-Fi", "ssid", ssid)
	return nil
}

// connectionSettings is a saved Wi-Fi client connection. The password is stored by
// NetworkManager in /etc/NetworkManager/system-connections (readable by root only).
func connectionSettings(ssid, security, password string) map[string]map[string]dbus.Variant {
	s := map[string]map[string]dbus.Variant{
		"connection": {
			"id":          dbus.MakeVariant(ssid),
			"type":        dbus.MakeVariant("802-11-wireless"),
			"autoconnect": dbus.MakeVariant(true),
		},
		"802-11-wireless": {
			"ssid": dbus.MakeVariant([]byte(ssid)),
			"mode": dbus.MakeVariant("infrastructure"),
		},
	}
	switch security {
	case SecurityWPA:
		s["802-11-wireless-security"] = map[string]dbus.Variant{
			"key-mgmt": dbus.MakeVariant("wpa-psk"), "psk": dbus.MakeVariant(password)}
	case SecurityWPA3:
		s["802-11-wireless-security"] = map[string]dbus.Variant{
			"key-mgmt": dbus.MakeVariant("sae"), "psk": dbus.MakeVariant(password)}
	}
	return s
}

// waitActivated waits until the activation succeeds, fails, or times out. The reason of a
// failure comes from the device's StateChanged signal: the failed state lasts a fraction of a
// millisecond, too short to be seen by reading properties.
func (n *NM) waitActivated(ctx context.Context, dev, active dbus.ObjectPath) error {
	signals := make(chan *dbus.Signal, 64)
	n.conn.Signal(signals)
	defer n.conn.RemoveSignal(signals)
	deadline := time.NewTimer(ConnectTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	var reason uint32
	record := func(sig *dbus.Signal) {
		// StateChanged(new, old, reason)
		if sig.Path == dev && sig.Name == ifaceDevice+".StateChanged" && len(sig.Body) == 3 {
			if state, _ := sig.Body[0].(uint32); state == deviceStateFailed {
				reason, _ = sig.Body[2].(uint32)
			}
		}
	}
	// failed reads the signals still queued (the failure may be among them) and returns the error.
	failed := func() error {
		grace := time.After(200 * time.Millisecond)
		for {
			select {
			case sig := <-signals:
				record(sig)
			case <-grace:
				return FailureError(reason)
			}
		}
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			n.conn.Object(nmService, nmPath).Call(ifaceNM+".DeactivateConnection", noStart, active)
			return ErrTimeout
		case sig := <-signals:
			record(sig)
			continue
		case <-tick.C:
		}
		props, err := n.getAll(active, ifaceActive)
		if err != nil { // the active connection is gone: it failed
			return failed()
		}
		switch u32(props, "State") {
		case activeStateActivated:
			return nil
		case activeStateDeactivated:
			return failed()
		}
	}
}

func (n *NM) deleteConnection(p dbus.ObjectPath) {
	if err := n.conn.Object(nmService, p).Call(ifaceConn+".Delete", noStart).Err; err != nil {
		n.log.Warn("cannot delete a saved network", "path", p, "err", err)
	}
}

// Disconnect disconnects the Wi-Fi adapter (it does not reconnect by itself until the next
// Connect).
func (n *NM) Disconnect() error {
	dev, err := n.wifiDevice()
	if err != nil {
		return err
	}
	err = n.conn.Object(nmService, dev.Path).Call(ifaceDevice+".Disconnect", noStart).Err
	if err != nil && strings.Contains(err.Error(), "NotActive") {
		return nil
	}
	return err
}

// Forget deletes a saved network (disconnecting from it if needed).
func (n *NM) Forget(ssid string) error {
	saved, err := n.savedWifi()
	if err != nil {
		return err
	}
	paths := saved[ssid]
	if len(paths) == 0 {
		return ErrUnknownNetwork
	}
	for _, p := range paths {
		if err := n.conn.Object(nmService, p).Call(ifaceConn+".Delete", noStart).Err; err != nil {
			return err
		}
	}
	n.log.Info("Wi-Fi network forgotten", "ssid", ssid)
	n.broker.Publish("network.forgotten", map[string]string{"ssid": ssid})
	return nil
}

// SetWifiEnabled turns the Wi-Fi radio on or off.
func (n *NM) SetWifiEnabled(on bool) error {
	err := n.conn.Object(nmService, nmPath).Call(ifaceProps+".Set", noStart, ifaceNM, "WirelessEnabled", dbus.MakeVariant(on)).Err
	var dbusErr dbus.Error
	if errors.As(err, &dbusErr) && strings.HasSuffix(dbusErr.Name, ".ServiceUnknown") {
		return ErrUnavailable
	}
	return err
}

// errorCode is the API error code of a connection failure, for events.
func errorCode(err error) string {
	switch {
	case errors.Is(err, ErrWrongPassword):
		return "network.wrong_password"
	case errors.Is(err, ErrTimeout):
		return "network.timeout"
	}
	return "network.connect_failed"
}
