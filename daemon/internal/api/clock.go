// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/clock"
)

// Clock is implemented by *clock.Manager (the time zone, [[step-6.1-first-boot-flow]]).
type Clock interface {
	Status() clock.Status
	SetZone(zone string) error
}

func (s *Server) clockAvailable(w http.ResponseWriter) bool {
	if s.Clock == nil {
		WriteError(w, http.StatusServiceUnavailable, "time.unavailable",
			"the time zone cannot be read or set (no connection to the system bus)")
		return false
	}
	return true
}

func (s *Server) handleTime(w http.ResponseWriter, _ *http.Request) {
	if !s.clockAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.Clock.Status())
}

// handleTimeZones tells how far zones are from UTC now: ?zone=Europe/Rome&zone=America/Lima, for
// the first start's list of cities (it works without a system bus).
func (s *Server) handleTimeZones(w http.ResponseWriter, r *http.Request) {
	zones := r.URL.Query()["zone"]
	if len(zones) > 100 {
		WriteError(w, http.StatusBadRequest, "request.invalid", "at most 100 zones")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"zones": clock.Offsets(zones, time.Now())})
}

// handleTimeZone sets the time zone: {"zone": "Europe/Rome"}.
func (s *Server) handleTimeZone(w http.ResponseWriter, r *http.Request) {
	if !s.clockAvailable(w) {
		return
	}
	var body struct {
		Zone string `json:"zone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "request.invalid", "expected {\"zone\": \"Area/City\"}")
		return
	}
	err := s.Clock.SetZone(body.Zone)
	switch {
	case err == nil:
		WriteJSON(w, http.StatusOK, s.Clock.Status())
	case errors.Is(err, clock.ErrUnknownZone):
		WriteError(w, http.StatusBadRequest, "time.unknown_zone", err.Error())
	default:
		s.Log.Warn("time zone not set", "zone", body.Zone, "err", err)
		WriteError(w, http.StatusBadGateway, "time.failed", err.Error())
	}
}
