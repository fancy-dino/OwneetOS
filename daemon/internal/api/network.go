// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/network"
)

// Network is implemented by *network.NM.
type Network interface {
	Status() network.Status
	Networks() ([]network.Network, error)
	Scan() error
	Connect(ctx context.Context, ssid, password string) error
	Disconnect() error
	Forget(ssid string) error
	SetWifiEnabled(on bool) error
}

func (s *Server) networkAvailable(w http.ResponseWriter) bool {
	if s.Network == nil {
		WriteError(w, http.StatusServiceUnavailable, "network.unavailable",
			"networking is not available (no connection to the system bus)")
		return false
	}
	return true
}

var networkErrors = []struct {
	err    error
	status int
	code   string
}{
	{network.ErrUnavailable, http.StatusServiceUnavailable, "network.unavailable"},
	{network.ErrNoWifi, http.StatusConflict, "network.no_wifi"},
	{network.ErrWifiDisabled, http.StatusConflict, "network.wifi_disabled"},
	{network.ErrNotFound, http.StatusNotFound, "network.not_found"},
	{network.ErrUnknownNetwork, http.StatusNotFound, "network.unknown_network"},
	{network.ErrUnsupportedSecurity, http.StatusUnprocessableEntity, "network.unsupported_security"},
	{network.ErrPasswordRequired, http.StatusUnprocessableEntity, "network.password_required"},
	{network.ErrInvalidPassword, http.StatusBadRequest, "network.invalid_password"},
	{network.ErrWrongPassword, http.StatusUnprocessableEntity, "network.wrong_password"},
	{network.ErrTimeout, http.StatusGatewayTimeout, "network.timeout"},
	{network.ErrConnectFailed, http.StatusBadGateway, "network.connect_failed"},
}

func (s *Server) networkError(w http.ResponseWriter, err error) {
	for _, e := range networkErrors {
		if errors.Is(err, e.err) {
			WriteError(w, e.status, e.code, err.Error())
			return
		}
	}
	s.Log.Warn("network request failed", "err", err)
	WriteError(w, http.StatusBadGateway, "network.failed", err.Error())
}

func (s *Server) handleNetwork(w http.ResponseWriter, _ *http.Request) {
	if !s.networkAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.Network.Status())
}

func (s *Server) handleWifiNetworks(w http.ResponseWriter, _ *http.Request) {
	if !s.networkAvailable(w) {
		return
	}
	list, err := s.Network.Networks()
	if err != nil {
		s.networkError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"networks": list})
}

func (s *Server) handleWifiScan(w http.ResponseWriter, _ *http.Request) {
	if !s.networkAvailable(w) {
		return
	}
	if err := s.Network.Scan(); err != nil {
		s.networkError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// handleWifiConnect waits for the result (up to network.ConnectTimeout). The password is never
// logged.
func (s *Server) handleWifiConnect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SSID     string `json:"ssid"`
		Password string `json:"password"`
	}
	if !s.networkAvailable(w) || !decode(w, r, &body) {
		return
	}
	if body.SSID == "" {
		WriteError(w, http.StatusBadRequest, "bad_request", "ssid is required")
		return
	}
	if err := s.Network.Connect(r.Context(), body.SSID, body.Password); err != nil {
		s.networkError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWifiDisconnect(w http.ResponseWriter, _ *http.Request) {
	if !s.networkAvailable(w) {
		return
	}
	if err := s.Network.Disconnect(); err != nil {
		s.networkError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWifiForget(w http.ResponseWriter, r *http.Request) {
	if !s.networkAvailable(w) {
		return
	}
	if err := s.Network.Forget(r.PathValue("ssid")); err != nil {
		s.networkError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWifiEnabled(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !s.networkAvailable(w) || !decode(w, r, &body) {
		return
	}
	if err := s.Network.SetWifiEnabled(body.Enabled); err != nil {
		s.networkError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
