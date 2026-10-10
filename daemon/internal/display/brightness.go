// SPDX-License-Identifier: GPL-3.0-or-later

package display

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// Brightness is shown only on screens that let the console change it (PROJECT_RULES.md, decision
// log 2026-10-09): a laptop's built-in screen (backlight, through logind) and monitors that
// accept DDC/CI commands (ddcutil). TVs usually do not.

// Backlight is a laptop screen's backlight in sysfs (/sys/class/backlight).
type Backlight struct {
	Name string
	dir  string
	max  int
}

// FindBacklight picks the backlight to use, as systemd does: firmware, then platform, then raw.
func FindBacklight(sysBacklight string) *Backlight {
	entries, _ := os.ReadDir(sysBacklight)
	var best *Backlight
	bestRank := 99
	for _, e := range entries {
		dir := filepath.Join(sysBacklight, e.Name())
		rank := map[string]int{"firmware": 0, "platform": 1, "raw": 2}[readTrim(filepath.Join(dir, "type"))]
		max, err := strconv.Atoi(readTrim(filepath.Join(dir, "max_brightness")))
		if err != nil || max <= 0 {
			continue
		}
		if best == nil || rank < bestRank {
			best, bestRank = &Backlight{Name: e.Name(), dir: dir, max: max}, rank
		}
	}
	return best
}

// Percent is the current brightness, 0-100.
func (b *Backlight) Percent() int {
	v, err := strconv.Atoi(readTrim(filepath.Join(b.dir, "brightness")))
	if err != nil {
		return 0
	}
	return (v*100 + b.max/2) / b.max
}

// raw turns a percentage into the device's scale; never fully dark (the screen must stay usable).
func (b *Backlight) raw(percent int) uint32 {
	v := percent * b.max / 100
	if floor := max(1, b.max/100); v < floor {
		v = floor
	}
	return uint32(v)
}

// LogindBacklight sets a backlight through logind (org.freedesktop.login1.Session.SetBrightness),
// which allows it to the user of the active session without root.
type LogindBacklight struct{ conn *dbus.Conn }

// NewLogindBacklight connects to the system bus.
func NewLogindBacklight() (*LogindBacklight, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	return &LogindBacklight{conn: conn}, nil
}

// Close disconnects from the bus.
func (l *LogindBacklight) Close() { l.conn.Close() }

// Set sets a backlight to a percentage.
func (l *LogindBacklight) Set(b *Backlight, percent int) error {
	session, err := l.session()
	if err != nil {
		return err
	}
	return l.conn.Object("org.freedesktop.login1", session).
		Call("org.freedesktop.login1.Session.SetBrightness", 0, "backlight", b.Name, b.raw(percent)).Err
}

// session is the console user's session on seat0 (owneetd is a user service, outside it).
func (l *LogindBacklight) session() (dbus.ObjectPath, error) {
	var sessions []struct {
		ID   string
		UID  uint32
		User string
		Seat string
		Path dbus.ObjectPath
	}
	err := l.conn.Object("org.freedesktop.login1", "/org/freedesktop/login1").
		Call("org.freedesktop.login1.Manager.ListSessions", 0).Store(&sessions)
	if err != nil {
		return "", err
	}
	uid := uint32(os.Getuid())
	for _, s := range sessions {
		if s.UID == uid && s.Seat == "seat0" {
			return s.Path, nil
		}
	}
	return "", errors.New("no session of this user on seat0")
}

// DDC talks to monitors with DDC/CI through ddcutil (VCP feature 0x10: brightness). It is slow
// (a command takes a fraction of a second, finding the monitors a few seconds), so the monitors
// are found in the background and a new value replaces one still waiting to be sent.
type DDC struct {
	Command string // ddcutil
	Log     interface{ Info(string, ...any) }

	mu       sync.Mutex
	found    bool
	finding  bool
	buses    map[string]int // screen id (e.g. "card1-DP-1") → I2C bus
	values   map[string]ddcValue
	queue    map[string]int // value waiting to be sent, per screen
	sending  bool
	onChange func()
}

type ddcValue struct{ cur, max int }

var (
	ddcBus  = regexp.MustCompile(`I2C bus:\s+/dev/i2c-([0-9]+)`)
	ddcDRM  = regexp.MustCompile(`DRM[ _]connector:\s+(card[0-9]+-\S+)`)
	ddcVCP  = regexp.MustCompile(`^VCP 10 C ([0-9]+) ([0-9]+)`)
	errNoDC = errors.New("this screen does not accept brightness commands")
)

func (d *DDC) run(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, d.Command, args...).CombinedOutput()
}

// Find looks for monitors in the background (again when the screens change); onChange is called
// once it knows more.
func (d *DDC) Find(onChange func()) {
	d.mu.Lock()
	if d.finding {
		d.mu.Unlock()
		return
	}
	d.finding, d.onChange = true, onChange
	d.mu.Unlock()
	go func() {
		buses := map[string]int{}
		values := map[string]ddcValue{}
		out, err := d.run("detect", "--terse")
		if err == nil {
			for _, block := range bytes.Split(out, []byte("\n\n")) {
				b, c := ddcBus.FindSubmatch(block), ddcDRM.FindSubmatch(block)
				if b == nil || c == nil || bytes.Contains(block, []byte("Invalid display")) {
					continue
				}
				bus, _ := strconv.Atoi(string(b[1]))
				if v, err := d.get(bus); err == nil {
					buses[string(c[1])], values[string(c[1])] = bus, v
				}
			}
		} else if d.Log != nil {
			d.Log.Info("ddcutil detect failed", "err", err)
		}
		if d.Log != nil {
			// what ddcutil said when it found nothing (e.g. no DDC/CI, no access to the I2C buses)
			said := strings.Join(strings.Fields(string(out)), " ")
			said = said[:min(len(said), 300)]
			d.Log.Info("monitors with DDC/CI brightness", "found", len(values), "ddcutil", said)
		}
		d.mu.Lock()
		d.buses, d.values, d.found, d.finding = buses, values, true, false
		cb := d.onChange
		d.mu.Unlock()
		if cb != nil {
			cb()
		}
	}()
}

func (d *DDC) get(bus int) (ddcValue, error) {
	out, err := d.run("--bus", strconv.Itoa(bus), "getvcp", "10", "--terse")
	if err != nil {
		return ddcValue{}, err
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if m := ddcVCP.FindStringSubmatch(strings.TrimSpace(sc.Text())); m != nil {
			cur, _ := strconv.Atoi(m[1])
			max, _ := strconv.Atoi(m[2])
			if max > 0 {
				return ddcValue{cur, max}, nil
			}
		}
	}
	return ddcValue{}, errNoDC
}

// Percent returns a screen's brightness, and whether it can be changed.
func (d *DDC) Percent(screen string) (int, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, ok := d.values[screen]
	if !ok {
		return 0, false
	}
	return (v.cur*100 + v.max/2) / v.max, true
}

// Set changes a screen's brightness (in the background; the last value wins).
func (d *DDC) Set(screen string, percent int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, ok := d.values[screen]
	if !ok {
		return errNoDC
	}
	if d.queue == nil {
		d.queue = map[string]int{}
	}
	value := max(1, percent*v.max/100)
	d.queue[screen] = value
	d.values[screen] = ddcValue{value, v.max}
	if !d.sending {
		d.sending = true
		go d.send()
	}
	return nil
}

func (d *DDC) send() {
	for {
		d.mu.Lock()
		var screen string
		var value int
		for s, v := range d.queue {
			screen, value = s, v
			break
		}
		if screen == "" {
			d.sending = false
			d.mu.Unlock()
			return
		}
		delete(d.queue, screen)
		bus := d.buses[screen]
		d.mu.Unlock()
		if _, err := d.run("--bus", strconv.Itoa(bus), "setvcp", "10", fmt.Sprint(value), "--noverify"); err != nil && d.Log != nil {
			d.Log.Info("ddcutil setvcp failed", "screen", screen, "err", err)
		}
	}
}
