// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/installer"
)

// Installer is implemented by *installer.Manager (installing OwneetOS from the USB stick,
// [[step-6.4-installer-ui]]).
type Installer interface {
	Disks() ([]installer.Disk, error)
	Status() installer.Status
	Start(req installer.Request) error
}

func (s *Server) installerAvailable(w http.ResponseWriter) bool {
	if s.Installer == nil {
		WriteError(w, http.StatusServiceUnavailable, "installer.unavailable",
			"OwneetOS cannot be installed (no connection to the system bus)")
		return false
	}
	return true
}

func (s *Server) handleInstaller(w http.ResponseWriter, _ *http.Request) {
	if !s.installerAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.Installer.Status())
}

// handleInstallerDisks lists the internal disks the player can choose (never the USB stick).
func (s *Server) handleInstallerDisks(w http.ResponseWriter, _ *http.Request) {
	if !s.installerAvailable(w) {
		return
	}
	disks, err := s.Installer.Disks()
	if err != nil {
		s.Log.Warn("disks not listed", "err", err)
		WriteError(w, http.StatusBadGateway, "installer.failed", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"disks": disks})
}

// handleInstallerStart installs on the disk the player chose and confirmed (the final hold):
// {"disk", "size", "serial", "name", "erase_windows"}; 202, then "installer.changed".
func (s *Server) handleInstallerStart(w http.ResponseWriter, r *http.Request) {
	if !s.installerAvailable(w) {
		return
	}
	var req installer.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "request.invalid",
			`expected {"disk", "size", "serial", "name", "erase_windows"}`)
		return
	}
	err := s.Installer.Start(req)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusAccepted)
	case errors.Is(err, installer.ErrNotLive):
		WriteError(w, http.StatusConflict, "installer.not_live", err.Error())
	case errors.Is(err, installer.ErrBusy):
		WriteError(w, http.StatusConflict, "installer.busy", err.Error())
	case errors.Is(err, installer.ErrDiskChanged):
		WriteError(w, http.StatusConflict, "installer.disk_changed", err.Error())
	case errors.Is(err, installer.ErrNotUsable):
		WriteError(w, http.StatusConflict, "installer.not_usable", err.Error())
	case errors.Is(err, installer.ErrWindowsNotConfirmed):
		WriteError(w, http.StatusBadRequest, "installer.windows_not_confirmed", err.Error())
	default:
		s.Log.Warn("installation not started", "err", err)
		WriteError(w, http.StatusBadGateway, "installer.failed", err.Error())
	}
}
