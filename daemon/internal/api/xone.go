// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/xone"
)

// Xone is implemented by *xone.Manager (the Xbox wireless adapter's firmware,
// [[xbox-wireless-dongle]]).
type Xone interface {
	Status() xone.Status
	Install() error
}

func (s *Server) xoneAvailable(w http.ResponseWriter) bool {
	if s.Xone == nil {
		WriteError(w, http.StatusServiceUnavailable, "xone.unavailable",
			"the Xbox adapter's firmware cannot be installed (no connection to the system bus)")
		return false
	}
	return true
}

func (s *Server) handleXone(w http.ResponseWriter, _ *http.Request) {
	if !s.xoneAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.Xone.Status())
}

// handleXoneInstall starts the firmware's installation, only after the player has accepted
// Microsoft's terms on the console: 202, then "xone.changed".
func (s *Server) handleXoneInstall(w http.ResponseWriter, _ *http.Request) {
	if !s.xoneAvailable(w) {
		return
	}
	err := s.Xone.Install()
	switch {
	case err == nil:
		w.WriteHeader(http.StatusAccepted)
	case errors.Is(err, xone.ErrBusy):
		WriteError(w, http.StatusConflict, "xone.busy", err.Error())
	default:
		s.Log.Warn("Xbox adapter firmware: installation not started", "err", err)
		WriteError(w, http.StatusBadGateway, "xone.failed", err.Error())
	}
}
