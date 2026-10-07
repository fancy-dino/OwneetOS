// SPDX-License-Identifier: GPL-3.0-or-later

// Package evdev reads Linux input devices (/dev/input/event*): identity, capabilities and events.
// Devices are opened without grabbing them, so games and the UI keep receiving their input.
package evdev

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"syscall"
	"unsafe"
)

// Event types and codes used by OwneetOS (linux/input-event-codes.h).
const (
	EvSyn = 0x00
	EvKey = 0x01
	EvRel = 0x02
	EvAbs = 0x03

	KeyHomepage     = 172
	BtnMisc         = 0x100
	BtnJoystick     = 0x120 // first joystick button (BTN_TRIGGER)
	BtnSouth        = 0x130 // BTN_A, first gamepad button
	BtnMode         = 0x13c // Guide / Xbox / PS / Home button
	BtnTriggerHappy = 0x2c0
	KeyMax          = 0x2ff

	AbsX     = 0x00
	AbsY     = 0x01
	AbsHat0X = 0x10
	AbsMax   = 0x3f
)

// ID is struct input_id.
type ID struct {
	Bustype, Vendor, Product, Version uint16
}

// Bus types (linux/input.h).
const (
	BusUSB       = 0x03
	BusBluetooth = 0x05
	BusVirtual   = 0x06
)

// Event is one input event.
type Event struct {
	Type  uint16
	Code  uint16
	Value int32
}

// Device is an open input device.
type Device struct {
	Path string
	Name string
	Phys string
	Uniq string
	ID   ID

	keys []byte
	abs  []byte
	f    *os.File
}

func ioc(dir, typ, nr, size uintptr) uintptr { return dir<<30 | size<<16 | typ<<8 | nr }

const iocRead = 2

func eviocg(nr, size uintptr) uintptr { return ioc(iocRead, 'E', nr, size) }

// Open opens path read-only (no grab) and reads the device identity and capabilities.
func Open(path string) (*Device, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	d := &Device{Path: path, f: f, keys: make([]byte, KeyMax/8+1), abs: make([]byte, AbsMax/8+1)}
	name := make([]byte, 256)
	phys := make([]byte, 256)
	uniq := make([]byte, 256)
	var id ID
	var ioErr error
	// Control keeps the file in non-blocking mode, so Close can interrupt a pending Read.
	err = d.withFd(func(fd uintptr) {
		if ioErr = ioctl(fd, eviocg(0x06, uintptr(len(name))), unsafe.Pointer(&name[0])); ioErr != nil {
			return
		}
		ioctl(fd, eviocg(0x07, uintptr(len(phys))), unsafe.Pointer(&phys[0])) // optional
		ioctl(fd, eviocg(0x08, uintptr(len(uniq))), unsafe.Pointer(&uniq[0])) // optional
		if ioErr = ioctl(fd, eviocg(0x02, unsafe.Sizeof(id)), unsafe.Pointer(&id)); ioErr != nil {
			return
		}
		if ioErr = ioctl(fd, eviocg(0x20+EvKey, uintptr(len(d.keys))), unsafe.Pointer(&d.keys[0])); ioErr != nil {
			return
		}
		ioErr = ioctl(fd, eviocg(0x20+EvAbs, uintptr(len(d.abs))), unsafe.Pointer(&d.abs[0]))
	})
	if err == nil {
		err = ioErr
	}
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	d.Name, d.Phys, d.Uniq, d.ID = cstring(name), cstring(phys), cstring(uniq), id
	return d, nil
}

func (d *Device) withFd(fn func(fd uintptr)) error {
	rc, err := d.f.SyscallConn()
	if err != nil {
		return err
	}
	return rc.Control(fn)
}

func ioctl(fd, req uintptr, arg unsafe.Pointer) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg)); e != 0 {
		return e
	}
	return nil
}

func cstring(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func bit(bits []byte, n int) bool {
	return n >= 0 && n/8 < len(bits) && bits[n/8]&(1<<(n%8)) != 0
}

// HasKey reports whether the device can send key/button code.
func (d *Device) HasKey(code int) bool { return bit(d.keys, code) }

// HasAbs reports whether the device has absolute axis code.
func (d *Device) HasAbs(code int) bool { return bit(d.abs, code) }

// Keys returns every key/button code of the device, ascending.
func (d *Device) Keys() []int {
	var out []int
	for c := 0; c <= KeyMax; c++ {
		if d.HasKey(c) {
			out = append(out, c)
		}
	}
	sort.Ints(out)
	return out
}

// SetCapabilities replaces the capability bitmaps (tests only).
func (d *Device) SetCapabilities(keys, abs []int) {
	d.keys = make([]byte, KeyMax/8+1)
	d.abs = make([]byte, AbsMax/8+1)
	for _, k := range keys {
		d.keys[k/8] |= 1 << (k % 8)
	}
	for _, a := range abs {
		d.abs[a/8] |= 1 << (a % 8)
	}
}

const eventSize = 24 // struct input_event on 64-bit Linux

// Read blocks until events arrive and returns them. It fails once the device is gone (ENODEV)
// or closed.
func (d *Device) Read() ([]Event, error) {
	buf := make([]byte, eventSize*64)
	n, err := d.f.Read(buf)
	if err != nil {
		return nil, err
	}
	return Decode(buf[:n]), nil
}

// Decode turns raw struct input_event records into events.
func Decode(buf []byte) []Event {
	events := make([]Event, 0, len(buf)/eventSize)
	for i := 0; i+eventSize <= len(buf); i += eventSize {
		rec := buf[i : i+eventSize]
		events = append(events, Event{
			Type:  binary.LittleEndian.Uint16(rec[16:]),
			Code:  binary.LittleEndian.Uint16(rec[18:]),
			Value: int32(binary.LittleEndian.Uint32(rec[20:])),
		})
	}
	return events
}

// Close releases the device; a pending Read returns an error.
func (d *Device) Close() error { return d.f.Close() }
