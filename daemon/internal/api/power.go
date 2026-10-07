// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/power"
)

// Power is implemented by *power.Logind.
type Power interface {
	Status() power.Status
	Do(a power.Action) error
}

func (s *Server) powerAvailable(w http.ResponseWriter) bool {
	if s.Power == nil {
		WriteError(w, http.StatusServiceUnavailable, "power.unavailable",
			"power control is not available (no connection to the system bus)")
		return false
	}
	return true
}

func (s *Server) handlePower(w http.ResponseWriter, _ *http.Request) {
	if !s.powerAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.Power.Status())
}

// handlePowerAction answers 202 once logind has accepted the action: the reply may be the last
// thing the daemon sends.
func (s *Server) handlePowerAction(a power.Action) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if !s.powerAvailable(w) {
			return
		}
		err := s.Power.Do(a)
		switch {
		case err == nil:
			w.WriteHeader(http.StatusAccepted)
		case errors.Is(err, power.ErrUnsupported):
			WriteError(w, http.StatusConflict, "power.unsupported", err.Error())
		case errors.Is(err, power.ErrNotAllowed):
			WriteError(w, http.StatusForbidden, "power.not_allowed", err.Error())
		case errors.Is(err, power.ErrInhibited):
			WriteError(w, http.StatusConflict, "power.inhibited", err.Error())
		default:
			s.Log.Warn("power action failed", "action", a.Name, "err", err)
			WriteError(w, http.StatusBadGateway, "power.failed", err.Error())
		}
	}
}
