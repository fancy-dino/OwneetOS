// SPDX-License-Identifier: GPL-3.0-or-later

package evdev

import (
	"encoding/binary"
	"testing"
)

func record(typ, code uint16, value int32) []byte {
	b := make([]byte, eventSize)
	binary.LittleEndian.PutUint16(b[16:], typ)
	binary.LittleEndian.PutUint16(b[18:], code)
	binary.LittleEndian.PutUint32(b[20:], uint32(value))
	return b
}

func TestDecode(t *testing.T) {
	buf := append(record(EvKey, BtnMode, 1), record(EvAbs, AbsX, -32768)...)
	buf = append(buf, 0xff) // trailing partial record is ignored
	got := Decode(buf)
	if len(got) != 2 {
		t.Fatalf("got %d events", len(got))
	}
	if got[0] != (Event{EvKey, BtnMode, 1}) || got[1] != (Event{EvAbs, AbsX, -32768}) {
		t.Fatalf("unexpected events %+v", got)
	}
}

func TestCapabilities(t *testing.T) {
	var d Device
	d.SetCapabilities([]int{BtnSouth, BtnMode, BtnTriggerHappy}, []int{AbsX, AbsHat0X})
	if !d.HasKey(BtnMode) || d.HasKey(BtnJoystick) || !d.HasAbs(AbsHat0X) || d.HasAbs(AbsY) {
		t.Fatal("capability bits wrong")
	}
	keys := d.Keys()
	if len(keys) != 3 || keys[0] != BtnSouth || keys[2] != BtnTriggerHappy {
		t.Fatalf("keys %v", keys)
	}
}
