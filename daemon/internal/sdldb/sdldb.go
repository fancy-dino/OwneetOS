// SPDX-License-Identifier: GPL-3.0-or-later

// Package sdldb reads SDL_GameControllerDB (https://github.com/mdqinc/SDL_GameControllerDB) to
// find which button is the Guide button on controllers whose kernel driver does not report it
// as BTN_MODE.
package sdldb

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/fancy-dino/OwneetOS/daemon/internal/evdev"
)

// DB holds the Linux mappings: SDL GUID -> mapping fields.
type DB struct {
	entries map[string]map[string]string
}

// GUID returns the SDL2 GUID of a Linux input device: bus, vendor, product and version as
// little-endian 16-bit words, each followed by two zero bytes.
func GUID(id evdev.ID) string {
	le := func(v uint16) string { return fmt.Sprintf("%02x%02x", v&0xff, v>>8) }
	return le(id.Bustype) + "0000" + le(id.Vendor) + "0000" + le(id.Product) + "0000" + le(id.Version) + "0000"
}

// Load reads a gamecontrollerdb.txt file.
func Load(path string) (*DB, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

// Parse reads mappings, keeping the Linux ones.
func Parse(r io.Reader) (*DB, error) {
	db := &DB{entries: make(map[string]map[string]string)}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(strings.TrimSuffix(line, ","), ",")
		if len(fields) < 3 {
			continue
		}
		m := make(map[string]string)
		for _, f := range fields[2:] {
			if k, v, ok := strings.Cut(f, ":"); ok {
				m[k] = v
			}
		}
		if m["platform"] != "Linux" {
			continue
		}
		m["name"] = fields[1]
		db.entries[strings.ToLower(fields[0])] = m
	}
	return db, sc.Err()
}

// Len is the number of Linux mappings (0 for a nil database).
func (db *DB) Len() int {
	if db == nil {
		return 0
	}
	return len(db.entries)
}

// Mapping returns the mapping fields for a device: exact GUID first, then ignoring the version.
func (db *DB) Mapping(id evdev.ID) (map[string]string, bool) {
	if db == nil {
		return nil, false
	}
	if m, ok := db.entries[GUID(id)]; ok {
		return m, true
	}
	id.Version = 0
	m, ok := db.entries[GUID(id)]
	return m, ok
}

// ButtonCode converts an SDL button reference ("b10") into the device's evdev key code. SDL
// numbers buttons from BTN_JOYSTICK upwards, then the codes below BTN_JOYSTICK.
func ButtonCode(keys []int, ref string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimPrefix(ref, "b"))
	if !strings.HasPrefix(ref, "b") || err != nil || n < 0 {
		return 0, false
	}
	var order []int
	for _, k := range keys {
		if k >= evdev.BtnJoystick {
			order = append(order, k)
		}
	}
	for _, k := range keys {
		if k < evdev.BtnJoystick {
			order = append(order, k)
		}
	}
	if n >= len(order) {
		return 0, false
	}
	return order[n], true
}
