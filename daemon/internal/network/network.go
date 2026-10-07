// SPDX-License-Identifier: GPL-3.0-or-later

// Package network manages wired and Wi-Fi networking through NetworkManager: status, Wi-Fi scan,
// connect with a password (typed on the on-screen keyboard), disconnect and forget.
package network

import (
	"errors"
	"sort"
	"strings"
)

// Status is the body of GET /v1/network.
type Status struct {
	// Available is false when NetworkManager is not running.
	Available    bool           `json:"available"`
	State        string         `json:"state"`        // connected, connecting, disconnected, asleep, unknown
	Connectivity string         `json:"connectivity"` // full, limited, portal, none, unknown
	Wifi         WifiStatus     `json:"wifi"`
	Ethernet     EthernetStatus `json:"ethernet"`
}

// WifiStatus describes the Wi-Fi adapter (the first one, if there are several).
type WifiStatus struct {
	Present bool `json:"present"`
	// Enabled is the software switch (settings); HardwareEnabled is false when a hardware switch
	// or the firmware blocks the radio.
	Enabled         bool   `json:"enabled"`
	HardwareEnabled bool   `json:"hardware_enabled"`
	State           string `json:"state"` // connected, connecting, disconnected, unavailable
	SSID            string `json:"ssid,omitempty"`
	Strength        *int   `json:"strength,omitempty"`
}

// EthernetStatus tells whether a cable connection exists and is up.
type EthernetStatus struct {
	Present   bool `json:"present"`
	Connected bool `json:"connected"`
}

// Network is a visible Wi-Fi network (all access points with the same name together).
type Network struct {
	SSID      string `json:"ssid"`
	Strength  int    `json:"strength"` // 0-100
	Security  string `json:"security"`
	Known     bool   `json:"known"` // saved: connecting needs no password
	Connected bool   `json:"connected"`
}

// Security of a Wi-Fi network.
const (
	SecurityOpen       = "open"
	SecurityWPA        = "wpa"  // WPA/WPA2 personal (WPA3 transition networks too)
	SecurityWPA3       = "wpa3" // WPA3 personal only (SAE)
	SecurityWEP        = "wep"
	SecurityEnterprise = "enterprise" // 802.1X: user name, certificates…
)

// Access point flags from NetworkManager (NM80211ApFlags, NM80211ApSecurityFlags).
const (
	apFlagPrivacy  = 0x1
	keyMgmtPSK     = 0x100
	keyMgmt8021X   = 0x200
	keyMgmtSAE     = 0x400
	keyMgmtOWE     = 0x800
	keyMgmtOWETM   = 0x1000
	keyMgmtSuiteB  = 0x2000
	keyMgmtEAPMask = keyMgmt8021X | keyMgmtSuiteB
)

// Classify returns the security of an access point from its NetworkManager flags.
func Classify(flags, wpaFlags, rsnFlags uint32) string {
	km := wpaFlags | rsnFlags
	switch {
	case km&keyMgmtEAPMask != 0:
		return SecurityEnterprise
	case km&keyMgmtPSK != 0:
		return SecurityWPA
	case km&keyMgmtSAE != 0:
		return SecurityWPA3
	case km&(keyMgmtOWE|keyMgmtOWETM) != 0:
		return SecurityOpen // "Enhanced Open": encrypted, but no password
	case flags&apFlagPrivacy != 0:
		return SecurityWEP
	}
	return SecurityOpen
}

// Supported tells whether OwneetOS can connect to a network with this security.
func Supported(security string) bool {
	return security == SecurityOpen || security == SecurityWPA || security == SecurityWPA3
}

// NeedsPassword tells whether connecting needs a password.
func NeedsPassword(security string) bool {
	return security != SecurityOpen
}

// Errors returned to the API.
var (
	ErrUnavailable         = errors.New("NetworkManager is not running")
	ErrNoWifi              = errors.New("no Wi-Fi adapter")
	ErrWifiDisabled        = errors.New("Wi-Fi is turned off")
	ErrNotFound            = errors.New("no visible Wi-Fi network with this name")
	ErrUnknownNetwork      = errors.New("this Wi-Fi network is not saved")
	ErrUnsupportedSecurity = errors.New("this Wi-Fi security is not supported (WEP or enterprise)")
	ErrPasswordRequired    = errors.New("this Wi-Fi network needs a password")
	ErrInvalidPassword     = errors.New("a WPA password has 8 to 63 characters, or 64 hexadecimal digits")
	ErrWrongPassword       = errors.New("wrong Wi-Fi password")
	ErrTimeout             = errors.New("the connection took too long")
	ErrConnectFailed       = errors.New("the connection failed")
)

// ValidatePassword checks a password before NetworkManager sees it.
func ValidatePassword(security, password string) error {
	switch security {
	case SecurityWPA:
		if len(password) == 64 && isHex(password) {
			return nil
		}
		if len(password) < 8 || len(password) > 63 {
			return ErrInvalidPassword
		}
		for _, r := range password {
			if r < 0x20 || r > 0x7e { // WPA passphrases are printable ASCII
				return ErrInvalidPassword
			}
		}
	case SecurityWPA3:
		if password == "" {
			return ErrPasswordRequired
		}
	}
	return nil
}

func isHex(s string) bool {
	return strings.Trim(strings.ToLower(s), "0123456789abcdef") == ""
}

// AccessPoint is one radio seen by the scan.
type AccessPoint struct {
	Path     string
	SSID     string
	Strength int
	Security string
}

// Aggregate groups access points by network name (hidden networks are left out), marks saved and
// connected networks, and sorts: connected first, then saved, then by signal.
func Aggregate(aps []AccessPoint, known map[string]bool, connected string) []Network {
	by := map[string]*Network{}
	for _, ap := range aps {
		if ap.SSID == "" {
			continue
		}
		n, ok := by[ap.SSID]
		if !ok {
			n = &Network{SSID: ap.SSID, Strength: -1, Known: known[ap.SSID], Connected: ap.SSID == connected}
			by[ap.SSID] = n
		}
		if ap.Strength > n.Strength {
			n.Strength, n.Security = ap.Strength, ap.Security
		}
	}
	out := make([]Network, 0, len(by))
	for _, n := range by {
		out = append(out, *n)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Connected != b.Connected {
			return a.Connected
		}
		if a.Known != b.Known {
			return a.Known
		}
		if a.Strength != b.Strength {
			return a.Strength > b.Strength
		}
		return a.SSID < b.SSID
	})
	return out
}

// BestAccessPoint returns the strongest access point of a network.
func BestAccessPoint(aps []AccessPoint, ssid string) (AccessPoint, bool) {
	best, found := AccessPoint{Strength: -1}, false
	for _, ap := range aps {
		if ap.SSID == ssid && ap.Strength > best.Strength {
			best, found = ap, true
		}
	}
	return best, found
}

// Device state reasons (NMDeviceStateReason) that mean the password was refused.
const (
	reasonNoSecrets            = 7
	reasonSupplicantDisconnect = 8
)

// FailureError turns the reason of a failed activation into an API error.
func FailureError(reason uint32) error {
	switch reason {
	case reasonNoSecrets, reasonSupplicantDisconnect:
		return ErrWrongPassword
	}
	return ErrConnectFailed
}
