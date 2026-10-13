// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/update"
)

// Update is implemented by *update.Manager (testers' updates from a USB stick, roadmap 4.2b).
type Update interface {
	Status() update.Status
	Check() error
	Apply() error
}

func (s *Server) updateAvailable(w http.ResponseWriter) bool {
	if s.Update == nil {
		WriteError(w, http.StatusServiceUnavailable, "update.unavailable",
			"updates are not available (no connection to the system bus)")
		return false
	}
	return true
}

func (s *Server) handleUpdate(w http.ResponseWriter, _ *http.Request) {
	if !s.updateAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.Update.Status())
}

// handleUpdateAction starts a check or an installation: 202, the result comes as "update.changed".
func (s *Server) handleUpdateAction(action func(Update) error) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if !s.updateAvailable(w) {
			return
		}
		err := action(s.Update)
		switch {
		case err == nil:
			w.WriteHeader(http.StatusAccepted)
		case errors.Is(err, update.ErrNotTester):
			WriteError(w, http.StatusConflict, "update.not_tester", err.Error())
		case errors.Is(err, update.ErrBusy):
			WriteError(w, http.StatusConflict, "update.busy", err.Error())
		default:
			s.Log.Warn("update failed to start", "err", err)
			WriteError(w, http.StatusBadGateway, "update.failed", err.Error())
		}
	}
}
