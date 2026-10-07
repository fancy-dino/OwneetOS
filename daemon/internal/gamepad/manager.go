// SPDX-License-Identifier: GPL-3.0-or-later

// Package gamepad tracks the game controllers connected to the console: discovery and hot-plug
// (/dev/input), the Guide button (owned by owneetd, PROJECT_RULES.md section 9.1) and battery
// levels. Devices are read without grabbing them: games and the UI get their input as usual.
package gamepad

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/fancy-dino/OwneetOS/daemon/internal/evdev"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
	"github.com/fancy-dino/OwneetOS/daemon/internal/sdldb"
)

// Controller is what the API reports about a connected controller.
type Controller struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Brand      string `json:"brand"`      // xbox, playstation, nintendo, 8bitdo, steam, other
	Connection string `json:"connection"` // usb, bluetooth, dongle, other
	Vendor     string `json:"vendor_id"`
	Product    string `json:"product_id"`
	HasGuide   bool   `json:"has_guide"`
	// Battery is a percentage when the driver reports one; BatteryLevel is the coarse level
	// ("full", "high", "normal", "low", "critical") when only that is known.
	Battery      *int   `json:"battery,omitempty"`
	BatteryLevel string `json:"battery_level,omitempty"`
}

// Manager discovers controllers and publishes their events.
type Manager struct {
	Broker *events.Broker
	Log    *slog.Logger
	DB     *sdldb.DB
	// InputDir is /dev/input; SysInputDir is /sys/class/input (configurable for tests).
	InputDir    string
	SysInputDir string
	// BatteryInterval is how often battery levels are refreshed.
	BatteryInterval time.Duration
	// BluetoothName returns the name BlueZ knows for a Bluetooth address ("" if unknown). Bluetooth
	// LE controllers often have a generic input device name ("bluez-hog-device").
	BluetoothName func(address string) string
	// OnGuide is called when the Guide button goes down or up (after the events are published).
	OnGuide func(controller string, down bool)

	mu    sync.Mutex
	devs  map[string]*tracked // by device path
	ready chan struct{}
}

type tracked struct {
	dev       *evdev.Device
	info      Controller
	guideCode int  // -1 if unknown
	companion bool // not a gamepad, but may carry the Guide button of one (KEY_HOMEPAGE)
}

// Ready is closed after the first scan of /dev/input.
func (m *Manager) Ready() <-chan struct{} {
	m.init()
	return m.ready
}

func (m *Manager) init() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.devs == nil {
		m.devs = make(map[string]*tracked)
		m.ready = make(chan struct{})
	}
	if m.InputDir == "" {
		m.InputDir = "/dev/input"
	}
	if m.SysInputDir == "" {
		m.SysInputDir = "/sys/class/input"
	}
	if m.BatteryInterval == 0 {
		m.BatteryInterval = time.Minute
	}
}

// List returns the connected controllers, sorted by ID.
func (m *Manager) List() []Controller {
	m.init()
	m.mu.Lock()
	out := []Controller{}
	for _, t := range m.devs {
		if !t.companion {
			out = append(out, t.info)
		}
	}
	m.mu.Unlock()
	// The BlueZ name may become known only after the controller connected (first pairing).
	for i := range out {
		if out[i].Connection == "bluetooth" && m.BluetoothName != nil {
			if name := m.BluetoothName(out[i].ID); name != "" {
				out[i].Name = name
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Run watches /dev/input until ctx is cancelled.
func (m *Manager) Run(ctx context.Context) error {
	m.init()
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC | syscall.IN_NONBLOCK)
	if err != nil {
		return err
	}
	watch := os.NewFile(uintptr(fd), "inotify")
	defer watch.Close()
	if _, err := syscall.InotifyAddWatch(fd, m.InputDir, syscall.IN_CREATE|syscall.IN_ATTRIB|syscall.IN_DELETE); err != nil {
		return err
	}

	entries, _ := os.ReadDir(m.InputDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "event") {
			m.add(filepath.Join(m.InputDir, e.Name()))
		}
	}
	close(m.ready)

	go m.batteryLoop(ctx)
	go func() { <-ctx.Done(); watch.Close() }()

	buf := make([]byte, 64*1024)
	for {
		n, err := watch.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				m.closeAll()
				return nil
			}
			return err
		}
		for off := 0; off+syscall.SizeofInotifyEvent <= n; {
			ev := (*syscall.InotifyEvent)(unsafe.Pointer(&buf[off]))
			nameBytes := buf[off+syscall.SizeofInotifyEvent : off+syscall.SizeofInotifyEvent+int(ev.Len)]
			name := strings.TrimRight(string(nameBytes), "\x00")
			off += syscall.SizeofInotifyEvent + int(ev.Len)
			if !strings.HasPrefix(name, "event") {
				continue
			}
			path := filepath.Join(m.InputDir, name)
			if ev.Mask&syscall.IN_DELETE != 0 {
				m.remove(path)
			} else {
				// IN_CREATE, or IN_ATTRIB once udev has set the permissions.
				m.add(path)
			}
		}
	}
}

func (m *Manager) add(path string) {
	m.mu.Lock()
	_, known := m.devs[path]
	m.mu.Unlock()
	if known {
		return
	}
	dev, err := evdev.Open(path)
	if err != nil {
		if !errors.Is(err, os.ErrPermission) && !errors.Is(err, os.ErrNotExist) {
			m.Log.Debug("cannot open input device", "path", path, "err", err)
		}
		return
	}
	t := &tracked{dev: dev, guideCode: -1}
	switch {
	case IsGamepad(dev):
		t.guideCode = m.guideCode(dev)
		t.info = m.describe(dev, t.guideCode >= 0)
	case dev.HasKey(evdev.KeyHomepage) && dev.Uniq != "":
		t.companion = true
		t.guideCode = evdev.KeyHomepage
		t.info = Controller{ID: controllerID(dev)}
	default:
		dev.Close()
		return
	}

	m.mu.Lock()
	if _, dup := m.devs[path]; dup {
		m.mu.Unlock()
		dev.Close()
		return
	}
	m.devs[path] = t
	m.mu.Unlock()

	if !t.companion {
		m.refreshBattery(t)
		m.Log.Info("controller connected", "id", t.info.ID, "name", t.info.Name, "guide", t.info.HasGuide)
		m.Broker.Publish("controller.added", m.snapshot(path))
	}
	go m.read(path, t)
}

func (m *Manager) snapshot(path string) Controller {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.devs[path]; ok {
		return t.info
	}
	return Controller{}
}

func (m *Manager) read(path string, t *tracked) {
	for {
		evs, err := t.dev.Read()
		if err != nil {
			m.remove(path)
			return
		}
		for _, ev := range evs {
			// Value 1: pressed, 0: released, 2: autorepeat (ignored).
			if ev.Type != evdev.EvKey || int(ev.Code) != t.guideCode || ev.Value > 1 {
				continue
			}
			id := t.info.ID
			if t.companion && !m.hasController(id) {
				continue
			}
			down := ev.Value == 1
			if down {
				m.Log.Debug("guide pressed", "id", id)
				m.Broker.Publish("guide.pressed", map[string]string{"controller": id})
			} else {
				m.Broker.Publish("guide.released", map[string]string{"controller": id})
			}
			if m.OnGuide != nil {
				m.OnGuide(id, down)
			}
		}
	}
}

func (m *Manager) hasController(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.devs {
		if !t.companion && t.info.ID == id {
			return true
		}
	}
	return false
}

func (m *Manager) remove(path string) {
	m.mu.Lock()
	t, ok := m.devs[path]
	delete(m.devs, path)
	m.mu.Unlock()
	if !ok {
		return
	}
	t.dev.Close()
	if !t.companion {
		m.Log.Info("controller disconnected", "id", t.info.ID)
		m.Broker.Publish("controller.removed", map[string]string{"id": t.info.ID})
	}
}

func (m *Manager) closeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for path, t := range m.devs {
		t.dev.Close()
		delete(m.devs, path)
	}
}

// IsGamepad tells gamepads and joysticks apart from keyboards, mice, touchpads and the motion
// sensors of some controllers.
func IsGamepad(d *evdev.Device) bool {
	hasButtons := false
	for c := evdev.BtnJoystick; c < evdev.BtnJoystick+0x20; c++ { // joystick + gamepad buttons
		if d.HasKey(c) {
			hasButtons = true
			break
		}
	}
	hasAxes := d.HasAbs(evdev.AbsX) || d.HasAbs(evdev.AbsY) || d.HasAbs(evdev.AbsHat0X)
	return hasButtons && hasAxes
}

// guideCode returns the key code of the Guide button: BTN_MODE when the driver reports it,
// otherwise the "guide" entry of SDL_GameControllerDB, otherwise -1.
func (m *Manager) guideCode(d *evdev.Device) int {
	if d.HasKey(evdev.BtnMode) {
		return evdev.BtnMode
	}
	if mapping, ok := m.DB.Mapping(d.ID); ok {
		if code, ok := sdldb.ButtonCode(d.Keys(), mapping["guide"]); ok {
			return code
		}
	}
	return -1
}

func (m *Manager) describe(d *evdev.Device, hasGuide bool) Controller {
	c := Controller{
		ID:         controllerID(d),
		Name:       d.Name,
		Brand:      brand(d.ID.Vendor),
		Connection: m.connection(d),
		Vendor:     hex4(d.ID.Vendor),
		Product:    hex4(d.ID.Product),
		HasGuide:   hasGuide,
	}
	if c.Connection == "bluetooth" && d.Uniq != "" && m.BluetoothName != nil {
		if name := m.BluetoothName(d.Uniq); name != "" {
			c.Name = name
		}
	}
	return c
}

// controllerID is stable across reconnections when the device has a unique id (Bluetooth MAC).
func controllerID(d *evdev.Device) string {
	switch {
	case d.Uniq != "":
		return strings.ToLower(d.Uniq)
	case d.Phys != "":
		return d.Phys
	}
	return filepath.Base(d.Path)
}

func brand(vendor uint16) string {
	switch vendor {
	case 0x045e:
		return "xbox"
	case 0x054c:
		return "playstation"
	case 0x057e:
		return "nintendo"
	case 0x2dc8:
		return "8bitdo"
	case 0x28de:
		return "steam"
	}
	return "other"
}

func (m *Manager) connection(d *evdev.Device) string {
	if drv := m.driverName(d); strings.Contains(drv, "xone") {
		return "dongle"
	}
	switch d.ID.Bustype {
	case evdev.BusUSB:
		return "usb"
	case evdev.BusBluetooth:
		return "bluetooth"
	}
	return "other"
}

// driverName is the kernel driver bound to the device's parent (e.g. "xpad", "xone-gip-gamepad").
func (m *Manager) driverName(d *evdev.Device) string {
	link, err := filepath.EvalSymlinks(filepath.Join(m.sysDevice(d), "..", "driver"))
	if err != nil {
		return ""
	}
	return filepath.Base(link)
}

// sysDevice is /sys/class/input/eventN/device resolved to its real path.
func (m *Manager) sysDevice(d *evdev.Device) string {
	p, err := filepath.EvalSymlinks(filepath.Join(m.SysInputDir, filepath.Base(d.Path), "device"))
	if err != nil {
		return ""
	}
	return p
}

func hex4(v uint16) string { return strconv.FormatUint(uint64(v)|0x10000, 16)[1:] }

func (m *Manager) batteryLoop(ctx context.Context) {
	tick := time.NewTicker(m.BatteryInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			m.mu.Lock()
			var list []*tracked
			for _, t := range m.devs {
				if !t.companion {
					list = append(list, t)
				}
			}
			m.mu.Unlock()
			for _, t := range list {
				if m.refreshBattery(t) {
					m.Broker.Publish("controller.battery", t.snapshotInfo(&m.mu))
				}
			}
		}
	}
}

func (t *tracked) snapshotInfo(mu *sync.Mutex) Controller {
	mu.Lock()
	defer mu.Unlock()
	return t.info
}

// refreshBattery updates the battery fields; it reports whether they changed.
func (m *Manager) refreshBattery(t *tracked) bool {
	pct, level := ReadBattery(m.sysDevice(t.dev))
	m.mu.Lock()
	defer m.mu.Unlock()
	old := t.info
	t.info.Battery, t.info.BatteryLevel = pct, level
	return !samePct(old.Battery, pct) || old.BatteryLevel != level
}

func samePct(a, b *int) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

// ReadBattery looks for a power_supply next to the input device in sysfs (the controller's HID
// device, up to three levels above it) and returns its capacity and/or level.
func ReadBattery(sysDevice string) (*int, string) {
	if sysDevice == "" {
		return nil, ""
	}
	dir := sysDevice
	for i := 0; i < 4; i++ {
		supplies, _ := filepath.Glob(filepath.Join(dir, "power_supply", "*"))
		for _, s := range supplies {
			var pct *int
			if v, err := os.ReadFile(filepath.Join(s, "capacity")); err == nil {
				if n, err := strconv.Atoi(strings.TrimSpace(string(v))); err == nil {
					pct = &n
				}
			}
			level := ""
			if v, err := os.ReadFile(filepath.Join(s, "capacity_level")); err == nil {
				level = strings.ToLower(strings.TrimSpace(string(v)))
				if level == "unknown" {
					level = ""
				}
			}
			if pct != nil || level != "" {
				return pct, level
			}
		}
		dir = filepath.Dir(dir)
	}
	return nil, ""
}
