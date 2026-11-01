// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/sysupdate"
)

// SystemUpdate is implemented by *sysupdate.Manager (updates of an installed OwneetOS,
// [[step-7.1-updates]]).
type SystemUpdate interface {
	Status() sysupdate.Status
	Check() error
	Install() error
	GoBack() error
	NoticeSeen()
}

func (s *Server) systemUpdateAvailable(w http.ResponseWriter) bool {
	if s.SystemUpdate == nil {
		WriteError(w, http.StatusServiceUnavailable, "system_update.unavailable",
			"updates are not available (no connection to the system bus)")
		return false
	}
	return true
}

func (s *Server) handleSystemUpdate(w http.ResponseWriter, _ *http.Request) {
	if !s.systemUpdateAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.SystemUpdate.Status())
}

// handleSystemUpdateAction starts a check, an installation or going back: 202, then
// "system-update.changed".
func (s *Server) handleSystemUpdateAction(action func(SystemUpdate) error) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if !s.systemUpdateAvailable(w) {
			return
		}
		err := action(s.SystemUpdate)
		switch {
		case err == nil:
			w.WriteHeader(http.StatusAccepted)
		case errors.Is(err, sysupdate.ErrNotInstalled):
			WriteError(w, http.StatusConflict, "system_update.not_installed", err.Error())
		case errors.Is(err, sysupdate.ErrBusy):
			WriteError(w, http.StatusConflict, "system_update.busy", err.Error())
		case errors.Is(err, sysupdate.ErrNothing):
			WriteError(w, http.StatusConflict, "system_update.nothing", err.Error())
		case errors.Is(err, sysupdate.ErrOnPrevious):
			WriteError(w, http.StatusConflict, "system_update.on_previous", err.Error())
		case errors.Is(err, sysupdate.ErrNoPrevious):
			WriteError(w, http.StatusConflict, "system_update.no_previous", err.Error())
		default:
			s.Log.Warn("system update failed to start", "err", err)
			WriteError(w, http.StatusBadGateway, "system_update.failed", err.Error())
		}
	}
}

// handleSystemUpdateNoticeSeen: the player has read that the console went back by itself.
func (s *Server) handleSystemUpdateNoticeSeen(w http.ResponseWriter, _ *http.Request) {
	if !s.systemUpdateAvailable(w) {
		return
	}
	s.SystemUpdate.NoticeSeen()
	w.WriteHeader(http.StatusNoContent)
}
