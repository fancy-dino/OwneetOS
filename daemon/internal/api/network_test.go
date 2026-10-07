// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/fancy-dino/OwneetOS/daemon/internal/network"
)

type fakeNetwork struct {
	forgotten []string
	connected string
	wifiOn    bool
}

func (f *fakeNetwork) Status() network.Status {
	return network.Status{Available: true, State: "connected", Connectivity: "full",
		Wifi: network.WifiStatus{Present: true, Enabled: f.wifiOn, State: "connected", SSID: f.connected}}
}
func (f *fakeNetwork) Networks() ([]network.Network, error) {
	return []network.Network{{SSID: "Home", Strength: 80, Security: network.SecurityWPA, Known: true}}, nil
}
func (f *fakeNetwork) Scan() error { return nil }
func (f *fakeNetwork) Connect(_ context.Context, ssid, password string) error {
	switch {
	case ssid != "Home":
		return network.ErrNotFound
	case password != "" && password != "correct horse":
		return network.ErrWrongPassword
	}
	f.connected = ssid
	return nil
}
func (f *fakeNetwork) Disconnect() error { f.connected = ""; return nil }
func (f *fakeNetwork) Forget(ssid string) error {
	if ssid == "Unknown" {
		return network.ErrUnknownNetwork
	}
	f.forgotten = append(f.forgotten, ssid)
	return nil
}
func (f *fakeNetwork) SetWifiEnabled(on bool) error { f.wifiOn = on; return nil }

func TestNetworkUnavailable(t *testing.T) {
	_, c, _ := start(t)
	if code, errCode := post(t, c, "/v1/network/wifi/scan", ``); code != 503 || errCode != "network.unavailable" {
		t.Fatalf("got %d %s", code, errCode)
	}
}

func TestNetworkEndpoints(t *testing.T) {
	s, c, _ := start(t)
	fake := &fakeNetwork{}
	s.Network = fake

	resp, err := c.Get("http://owneetd/v1/network/wifi")
	if err != nil {
		t.Fatal(err)
	}
	var body struct{ Networks []network.Network }
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if len(body.Networks) != 1 || body.Networks[0].SSID != "Home" {
		t.Fatalf("networks = %+v", body.Networks)
	}

	if code, _ := post(t, c, "/v1/network/wifi/scan", ``); code != 202 {
		t.Fatalf("scan: %d", code)
	}
	if code, errCode := post(t, c, "/v1/network/wifi/connect", `{"ssid":"Home","password":"wrong one"}`); code != 422 || errCode != "network.wrong_password" {
		t.Fatalf("wrong password: %d %s", code, errCode)
	}
	if code, errCode := post(t, c, "/v1/network/wifi/connect", `{"ssid":"Elsewhere"}`); code != 404 || errCode != "network.not_found" {
		t.Fatalf("missing network: %d %s", code, errCode)
	}
	if code, errCode := post(t, c, "/v1/network/wifi/connect", `{"password":"x"}`); code != 400 || errCode != "bad_request" {
		t.Fatalf("no ssid: %d %s", code, errCode)
	}
	if code, _ := post(t, c, "/v1/network/wifi/connect", `{"ssid":"Home","password":"correct horse"}`); code != 204 || fake.connected != "Home" {
		t.Fatalf("connect: %d, connected %q", code, fake.connected)
	}
	if code, _ := post(t, c, "/v1/network/wifi/disconnect", ``); code != 204 || fake.connected != "" {
		t.Fatalf("disconnect: %d", code)
	}

	del := func(ssid string) int {
		req, _ := http.NewRequest(http.MethodDelete, "http://owneetd/v1/network/wifi/"+url.PathEscape(ssid), nil)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := del("My Wi-Fi/5G"); code != 204 || len(fake.forgotten) != 1 || fake.forgotten[0] != "My Wi-Fi/5G" {
		t.Fatalf("forget: %d %v", code, fake.forgotten)
	}
	if code := del("Unknown"); code != 404 {
		t.Fatalf("forget unknown: %d", code)
	}

	req, _ := http.NewRequest(http.MethodPut, "http://owneetd/v1/network/wifi/enabled", nil)
	req.Body = http.NoBody
	if resp, err := c.Do(req); err != nil || resp.StatusCode != 400 {
		t.Fatalf("enabled without body: %v %v", resp.StatusCode, err)
	}
}
