// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/diskprep"
)

// DiskPrep is implemented by *diskprep.Manager (a disk prepared for games from Settings → Disks,
// [[step-6.7-preparing-other-disks]]).
type DiskPrep interface {
	Disks() ([]diskprep.Disk, error)
	Status() diskprep.Status
	Start(req diskprep.Request) error
}

func (s *Server) diskPrepAvailable(w http.ResponseWriter) bool {
	if s.DiskPrep == nil {
		WriteError(w, http.StatusServiceUnavailable, "disks.unavailable",
			"disks cannot be prepared (no connection to the system bus)")
		return false
	}
	return true
}

// handleDisks lists every disk of the PC, with what is on it and whether it is the console's own.
func (s *Server) handleDisks(w http.ResponseWriter, _ *http.Request) {
	if !s.diskPrepAvailable(w) {
		return
	}
	disks, err := s.DiskPrep.Disks()
	if err != nil {
		s.Log.Warn("disks not listed", "err", err)
		WriteError(w, http.StatusBadGateway, "disks.failed", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"disks": disks})
}

func (s *Server) handleDiskPrepStatus(w http.ResponseWriter, _ *http.Request) {
	if !s.diskPrepAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.DiskPrep.Status())
}

// handleDiskPrep erases and prepares for games the disk the player chose, named and confirmed (the
// final hold): {"disk", "size", "serial", "name", "folder", "erase_windows"}; 202, then
// "disks.prepare.changed".
func (s *Server) handleDiskPrep(w http.ResponseWriter, r *http.Request) {
	if !s.diskPrepAvailable(w) {
		return
	}
	var req diskprep.Request
	if !decode(w, r, &req) {
		return
	}
	err := s.DiskPrep.Start(req)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusAccepted)
	case errors.Is(err, diskprep.ErrLive):
		WriteError(w, http.StatusConflict, "disks.live", err.Error())
	case errors.Is(err, diskprep.ErrBusy):
		WriteError(w, http.StatusConflict, "disks.busy", err.Error())
	case errors.Is(err, diskprep.ErrDiskChanged):
		WriteError(w, http.StatusConflict, "disks.disk_changed", err.Error())
	case errors.Is(err, diskprep.ErrNotAllowed):
		WriteError(w, http.StatusForbidden, "disks.not_allowed", err.Error())
	case errors.Is(err, diskprep.ErrWindowsNotConfirmed):
		WriteError(w, http.StatusBadRequest, "disks.windows_not_confirmed", err.Error())
	case errors.Is(err, diskprep.ErrBadName):
		WriteError(w, http.StatusBadRequest, "disks.bad_name", err.Error())
	case errors.Is(err, diskprep.ErrNameTaken):
		WriteError(w, http.StatusConflict, "disks.name_taken", err.Error())
	default:
		s.Log.Warn("disk preparation not started", "err", err)
		WriteError(w, http.StatusBadGateway, "disks.failed", err.Error())
	}
}
