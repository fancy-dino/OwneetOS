// SPDX-License-Identifier: GPL-3.0-or-later

// Package uinput creates virtual input devices through /dev/uinput: virtual gamepads for tests,
// and the virtual keyboard used by the on-screen keyboard (roadmap step 2.4).
package uinput

import (
	"bytes"
	"encoding/binary"
	"os"
	"syscall"

	"github.com/fancy-dino/OwneetOS/daemon/internal/evdev"
)

// Axis is an absolute axis of a virtual device.
type Axis struct {
	Code     int
	Min, Max int32
}

// Spec describes a virtual device.
type Spec struct {
	Name string
	ID   evdev.ID
	Keys []int
	Axes []Axis
	// Rel lists relative axes (REL_X, REL_Y, REL_WHEEL) for virtual mice.
	Rel []int
}

// Device is a virtual input device.
type Device struct {
	f *os.File
}

const (
	uiDevCreate  = 0x5501     // _IO('U', 1)
	uiDevDestroy = 0x5502     // _IO('U', 2)
	uiSetEvBit   = 0x40045564 // _IOW('U', 100, int)
	uiSetKeyBit  = 0x40045565 // _IOW('U', 101, int)
	uiSetRelBit  = 0x40045566 // _IOW('U', 102, int)
	uiSetAbsBit  = 0x40045567 // _IOW('U', 103, int)
	absCnt       = evdev.AbsMax + 1
	nameSize     = 80
	synReport    = 0
	eventRecSize = 24
)

// Create makes the virtual device. Needs write access to /dev/uinput.
func Create(spec Spec) (*Device, error) {
	f, err := os.OpenFile("/dev/uinput", os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	d := &Device{f: f}
	setup := func(fd uintptr) error {
		calls := [][2]uintptr{{uiSetEvBit, evdev.EvKey}, {uiSetEvBit, evdev.EvSyn}}
		if len(spec.Axes) > 0 {
			calls = append(calls, [2]uintptr{uiSetEvBit, evdev.EvAbs})
		}
		if len(spec.Rel) > 0 {
			calls = append(calls, [2]uintptr{uiSetEvBit, evdev.EvRel})
		}
		for _, rel := range spec.Rel {
			calls = append(calls, [2]uintptr{uiSetRelBit, uintptr(rel)})
		}
		for _, k := range spec.Keys {
			calls = append(calls, [2]uintptr{uiSetKeyBit, uintptr(k)})
		}
		for _, a := range spec.Axes {
			calls = append(calls, [2]uintptr{uiSetAbsBit, uintptr(a.Code)})
		}
		for _, c := range calls {
			if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, c[0], c[1]); e != 0 {
				return e
			}
		}
		return nil
	}
	if err := d.control(setup); err != nil {
		f.Close()
		return nil, err
	}

	// struct uinput_user_dev: name[80], input_id, ff_effects_max, absmax/absmin/absfuzz/absflat[64]
	var buf bytes.Buffer
	name := make([]byte, nameSize)
	copy(name, spec.Name)
	buf.Write(name)
	binary.Write(&buf, binary.LittleEndian, spec.ID)
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	var absmax, absmin, zero [absCnt]int32
	for _, a := range spec.Axes {
		absmax[a.Code], absmin[a.Code] = a.Max, a.Min
	}
	binary.Write(&buf, binary.LittleEndian, absmax)
	binary.Write(&buf, binary.LittleEndian, absmin)
	binary.Write(&buf, binary.LittleEndian, zero) // fuzz
	binary.Write(&buf, binary.LittleEndian, zero) // flat
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		return nil, err
	}
	if err := d.control(func(fd uintptr) error {
		if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, uiDevCreate, 0); e != 0 {
			return e
		}
		return nil
	}); err != nil {
		f.Close()
		return nil, err
	}
	return d, nil
}

func (d *Device) control(fn func(fd uintptr) error) error {
	rc, err := d.f.SyscallConn()
	if err != nil {
		return err
	}
	var inner error
	if err := rc.Control(func(fd uintptr) { inner = fn(fd) }); err != nil {
		return err
	}
	return inner
}

// Emit writes one event; call Sync to deliver a group of events.
func (d *Device) Emit(typ, code uint16, value int32) error {
	rec := make([]byte, eventRecSize)
	binary.LittleEndian.PutUint16(rec[16:], typ)
	binary.LittleEndian.PutUint16(rec[18:], code)
	binary.LittleEndian.PutUint32(rec[20:], uint32(value))
	_, err := d.f.Write(rec)
	return err
}

// Sync delivers the events emitted so far.
func (d *Device) Sync() error { return d.Emit(evdev.EvSyn, synReport, 0) }

// Press presses and releases a key or button.
func (d *Device) Press(code int) error {
	for _, v := range []int32{1, 0} {
		if err := d.Emit(evdev.EvKey, uint16(code), v); err != nil {
			return err
		}
		if err := d.Sync(); err != nil {
			return err
		}
	}
	return nil
}

// Close removes the virtual device.
func (d *Device) Close() error {
	d.control(func(fd uintptr) error {
		syscall.Syscall(syscall.SYS_IOCTL, fd, uiDevDestroy, 0)
		return nil
	})
	return d.f.Close()
}
