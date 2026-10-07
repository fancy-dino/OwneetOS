// SPDX-License-Identifier: GPL-3.0-or-later

package network

import (
	"errors"
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		flags, wpa, rsn uint32
		want            string
	}{
		{0, 0, 0, SecurityOpen},
		{apFlagPrivacy, 0, 0, SecurityWEP},
		{apFlagPrivacy, 0, keyMgmtPSK | 0x8 | 0x80, SecurityWPA}, // WPA2 personal
		{apFlagPrivacy, keyMgmtPSK, 0, SecurityWPA},              // WPA1
		{apFlagPrivacy, 0, keyMgmtPSK | keyMgmtSAE, SecurityWPA}, // WPA2/WPA3 transition
		{apFlagPrivacy, 0, keyMgmtSAE, SecurityWPA3},             // WPA3 only
		{apFlagPrivacy, 0, keyMgmt8021X, SecurityEnterprise},     // WPA2 enterprise
		{apFlagPrivacy, 0, keyMgmtSuiteB | keyMgmtSAE, SecurityEnterprise},
		{0, 0, keyMgmtOWE, SecurityOpen}, // Enhanced Open
	}
	for _, c := range cases {
		if got := Classify(c.flags, c.wpa, c.rsn); got != c.want {
			t.Errorf("Classify(%#x, %#x, %#x) = %s, want %s", c.flags, c.wpa, c.rsn, got, c.want)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	ok := []struct{ sec, pw string }{
		{SecurityWPA, "12345678"},
		{SecurityWPA, strings.Repeat("x", 63)},
		{SecurityWPA, strings.Repeat("aF", 32)}, // 64 hex digits: a raw key
		{SecurityWPA, "pass word!"},
		{SecurityWPA3, "short"},
		{SecurityOpen, ""},
	}
	for _, c := range ok {
		if err := ValidatePassword(c.sec, c.pw); err != nil {
			t.Errorf("ValidatePassword(%s, %q) = %v", c.sec, c.pw, err)
		}
	}
	bad := []struct{ sec, pw string }{
		{SecurityWPA, "1234567"},
		{SecurityWPA, strings.Repeat("x", 64)}, // 64 characters that are not hex
		{SecurityWPA, "pàssword"},              // not ASCII
		{SecurityWPA3, ""},
	}
	for _, c := range bad {
		if err := ValidatePassword(c.sec, c.pw); err == nil {
			t.Errorf("ValidatePassword(%s, %q) accepted", c.sec, c.pw)
		}
	}
}

func TestAggregate(t *testing.T) {
	aps := []AccessPoint{
		{SSID: "Home", Strength: 40, Security: SecurityWPA},
		{SSID: "Home", Strength: 80, Security: SecurityWPA}, // second access point, stronger
		{SSID: "Cafe", Strength: 90, Security: SecurityOpen},
		{SSID: "Neighbour", Strength: 95, Security: SecurityWPA3},
		{SSID: "", Strength: 99, Security: SecurityWPA}, // hidden: left out
		{SSID: "Office", Strength: 30, Security: SecurityEnterprise},
	}
	got := Aggregate(aps, map[string]bool{"Cafe": true, "Home": true}, "Home")
	want := []string{"Home", "Cafe", "Neighbour", "Office"} // connected, saved, then by signal
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, n := range got {
		if n.SSID != want[i] {
			t.Fatalf("order %d: got %s, want %s (%+v)", i, n.SSID, want[i], got)
		}
	}
	if got[0].Strength != 80 || !got[0].Connected || !got[0].Known {
		t.Errorf("Home = %+v", got[0])
	}
	if got[2].Known || got[2].Connected {
		t.Errorf("Neighbour = %+v", got[2])
	}
	if ap, ok := BestAccessPoint(aps, "Home"); !ok || ap.Strength != 80 {
		t.Errorf("BestAccessPoint = %+v %v", ap, ok)
	}
	if _, ok := BestAccessPoint(aps, "Nowhere"); ok {
		t.Error("BestAccessPoint found a missing network")
	}
}

func TestFailureError(t *testing.T) {
	if !errors.Is(FailureError(reasonNoSecrets), ErrWrongPassword) {
		t.Error("NO_SECRETS is not a wrong password")
	}
	if !errors.Is(FailureError(53), ErrConnectFailed) { // SSID_NOT_FOUND
		t.Error("other reasons must be a generic failure")
	}
}
