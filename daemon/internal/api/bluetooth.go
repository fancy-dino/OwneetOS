// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/bluetooth"
)

// Bluetooth is implemented by *bluetooth.Manager.
type Bluetooth interface {
	Status() bluetooth.Status
	SetAutoPair(on bool, d time.Duration) error
	Connect(address string) error
	Disconnect(address string) error
	Forget(address string) error
}

// Auto-pair duration when the UI turns it on (default and maximum).
const (
	autoPairDefault = 120 * time.Second
	autoPairMax     = 600 * time.Second
)

var btAddress = regexp.MustCompile(`^[0-9A-Fa-f]{2}(:[0-9A-Fa-f]{2}){5}$`)

func (s *Server) bluetoothAvailable(w http.ResponseWriter) bool {
	if s.Bluetooth == nil {
		WriteError(w, http.StatusServiceUnavailable, "bluetooth.unavailable",
			"Bluetooth is not available (no connection to the system bus)")
		return false
	}
	return true
}

func (s *Server) bluetoothError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, bluetooth.ErrNoAdapter):
		WriteError(w, http.StatusConflict, "bluetooth.no_adapter", err.Error())
	case errors.Is(err, bluetooth.ErrUnknownDevice):
		WriteError(w, http.StatusNotFound, "bluetooth.unknown_device", err.Error())
	default:
		s.Log.Warn("bluetooth request failed", "err", err)
		WriteError(w, http.StatusBadGateway, "bluetooth.failed", err.Error())
	}
}

func (s *Server) handleBluetooth(w http.ResponseWriter, _ *http.Request) {
	if !s.bluetoothAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.Bluetooth.Status())
}

func (s *Server) handleBluetoothAutoPair(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
		Seconds int  `json:"seconds"`
	}
	if !s.bluetoothAvailable(w) || !decode(w, r, &body) {
		return
	}
	d := time.Duration(body.Seconds) * time.Second
	switch {
	case body.Seconds < 0 || d > autoPairMax:
		WriteError(w, http.StatusBadRequest, "bad_request", "seconds must be between 0 and 600")
		return
	case body.Seconds == 0:
		d = autoPairDefault
	}
	if err := s.Bluetooth.SetAutoPair(body.Enabled, d); err != nil {
		s.bluetoothError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleBluetoothDevice serves connect, disconnect and forget on /v1/bluetooth/devices/{address}.
func (s *Server) handleBluetoothDevice(action func(Bluetooth, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		addr := r.PathValue("address")
		if !s.bluetoothAvailable(w) {
			return
		}
		if !btAddress.MatchString(addr) {
			WriteError(w, http.StatusBadRequest, "bluetooth.invalid_address",
				"not a Bluetooth address (expected AA:BB:CC:DD:EE:FF): "+addr)
			return
		}
		if err := action(s.Bluetooth, strings.ToUpper(addr)); err != nil {
			s.bluetoothError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
