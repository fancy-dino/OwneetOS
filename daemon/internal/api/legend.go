// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/legend"
)

// Legend is implemented by *legend.Manager ([[step-5.6a-command-legend]]).
type Legend interface {
	Handle(req legend.Request) error
	Status() legend.State
}

func (s *Server) legendAvailable(w http.ResponseWriter) bool {
	if s.Legend == nil {
		WriteError(w, http.StatusServiceUnavailable, "legend.unavailable", "the command legend is not available")
		return false
	}
	return true
}

func (s *Server) handleLegend(w http.ResponseWriter, _ *http.Request) {
	if !s.legendAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.Legend.Status())
}

// handleLegendAction: View in the web app ("toggle"), its screen ("screen"), the interface's close
// ("close"); 204, the change comes as "legend.changed".
func (s *Server) handleLegendAction(w http.ResponseWriter, r *http.Request) {
	var req legend.Request
	if !s.legendAvailable(w) || !decode(w, r, &req) {
		return
	}
	switch err := s.Legend.Handle(req); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, legend.ErrInvalid):
		WriteError(w, http.StatusBadRequest, "legend.invalid", err.Error())
	case errors.Is(err, legend.ErrNoApp):
		WriteError(w, http.StatusConflict, "legend.no_app", err.Error())
	default:
		WriteError(w, http.StatusBadGateway, "legend.failed", err.Error())
	}
}
