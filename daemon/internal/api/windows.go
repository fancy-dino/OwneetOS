// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/windows"
)

// Windows is implemented by *windows.Manager: what Windows games need, downloaded once
// ([[step-4.5-windows-games-proton]]).
type Windows interface {
	Status() windows.Status
	SetNeeded() error
}

// handleWindows reports the preparation: {state, percent, needed}; changes come as "windows.changed".
func (s *Server) handleWindows(w http.ResponseWriter, _ *http.Request) {
	if s.Windows == nil {
		WriteJSON(w, http.StatusOK, windows.Status{State: windows.StateNone})
		return
	}
	WriteJSON(w, http.StatusOK, s.Windows.Status())
}

// handleWindowsNeeded: the library has a Windows game; the preparation starts as soon as the
// console is online (204).
func (s *Server) handleWindowsNeeded(w http.ResponseWriter, _ *http.Request) {
	if s.Windows == nil {
		WriteError(w, http.StatusServiceUnavailable, "windows.unavailable", "Windows games are not available")
		return
	}
	if err := s.Windows.SetNeeded(); err != nil {
		s.Log.Warn("cannot record that Windows games are needed", "err", err)
		WriteError(w, http.StatusInternalServerError, "windows.failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
