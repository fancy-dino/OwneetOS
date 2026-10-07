// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/bluetooth"
)

type fakeBluetooth struct {
	adapter  bool
	autoPair time.Duration
	calls    []string
}

func (f *fakeBluetooth) Status() bluetooth.Status {
	return bluetooth.Status{Adapter: f.adapter, Powered: f.adapter, Devices: []bluetooth.Device{
		{Address: "AA:BB:CC:DD:EE:01", Name: "Xbox Wireless Controller", Paired: true, Gamepad: true},
	}}
}

func (f *fakeBluetooth) SetAutoPair(on bool, d time.Duration) error {
	if !f.adapter {
		return bluetooth.ErrNoAdapter
	}
	f.autoPair = d
	return nil
}

func (f *fakeBluetooth) act(what, addr string) error {
	if addr != "AA:BB:CC:DD:EE:01" {
		return bluetooth.ErrUnknownDevice
	}
	f.calls = append(f.calls, what+" "+addr)
	return nil
}
func (f *fakeBluetooth) Connect(a string) error    { return f.act("connect", a) }
func (f *fakeBluetooth) Disconnect(a string) error { return f.act("disconnect", a) }
func (f *fakeBluetooth) Forget(a string) error     { return f.act("forget", a) }

func TestBluetoothUnavailable(t *testing.T) {
	_, c, _ := start(t)
	resp, err := c.Get("http://owneetd/v1/bluetooth")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatalf("status %d, want 503", resp.StatusCode)
	}
}

func TestBluetoothStatusAndActions(t *testing.T) {
	s, c, _ := start(t)
	bt := &fakeBluetooth{adapter: true}
	s.Bluetooth = bt

	resp, err := c.Get("http://owneetd/v1/bluetooth")
	if err != nil {
		t.Fatal(err)
	}
	var st bluetooth.Status
	json.NewDecoder(resp.Body).Decode(&st)
	resp.Body.Close()
	if !st.Adapter || len(st.Devices) != 1 || !st.Devices[0].Gamepad {
		t.Fatalf("status = %+v", st)
	}

	if code, _ := post(t, c, "/v1/bluetooth/auto-pair", `{"enabled":true}`); code != 204 || bt.autoPair != autoPairDefault {
		t.Fatalf("auto-pair: status %d, duration %v", code, bt.autoPair)
	}
	if code, errCode := post(t, c, "/v1/bluetooth/auto-pair", `{"enabled":true,"seconds":601}`); code != 400 || errCode != "bad_request" {
		t.Fatalf("too long auto-pair: %d %s", code, errCode)
	}
	if code, _ := post(t, c, "/v1/bluetooth/devices/aa:bb:cc:dd:ee:01/connect", ``); code != 204 {
		t.Fatalf("connect: status %d", code)
	}
	if code, errCode := post(t, c, "/v1/bluetooth/devices/AA:BB:CC:DD:EE:02/disconnect", ``); code != 404 || errCode != "bluetooth.unknown_device" {
		t.Fatalf("unknown device: %d %s", code, errCode)
	}
	if code, errCode := post(t, c, "/v1/bluetooth/devices/nonsense/connect", ``); code != 400 || errCode != "bluetooth.invalid_address" {
		t.Fatalf("bad address: %d %s", code, errCode)
	}
	req, _ := http.NewRequest(http.MethodDelete, "http://owneetd/v1/bluetooth/devices/AA:BB:CC:DD:EE:01", nil)
	resp, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("forget: status %d", resp.StatusCode)
	}
	want := []string{"connect AA:BB:CC:DD:EE:01", "forget AA:BB:CC:DD:EE:01"}
	if len(bt.calls) != 2 || bt.calls[0] != want[0] || bt.calls[1] != want[1] {
		t.Fatalf("calls = %v, want %v", bt.calls, want)
	}

	bt.adapter = false
	if code, errCode := post(t, c, "/v1/bluetooth/auto-pair", `{"enabled":true}`); code != 409 || errCode != "bluetooth.no_adapter" {
		t.Fatalf("no adapter: %d %s", code, errCode)
	}
}
