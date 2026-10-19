// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/installs"
)

// Installs is implemented by *installs.Manager: games installed from their installers, on the
// console's disk ([[step-4.6-installers]]).
type Installs interface {
	Status() installs.Status
	Inspect(installer, kind string) (installs.Info, error)
	Install(req installs.Request) error
	Stop(dest string) error
	Uninstall(dest string) error
}

var installErrors = []struct {
	err    error
	status int
	code   string
}{
	{installs.ErrInvalid, http.StatusBadRequest, "installs.invalid"},
	{installs.ErrNoSpace, http.StatusConflict, "installs.no_space"},
	{installs.ErrExists, http.StatusConflict, "installs.exists"},
	{installs.ErrNotFound, http.StatusNotFound, "installs.not_found"},
	{installs.ErrInUse, http.StatusConflict, "installs.in_use"},
}

func (s *Server) installsError(w http.ResponseWriter, err error) {
	for _, e := range installErrors {
		if errors.Is(err, e.err) {
			WriteError(w, e.status, e.code, err.Error())
			return
		}
	}
	s.Log.Warn("install request failed", "err", err)
	WriteError(w, http.StatusInternalServerError, "installs.failed", err.Error())
}

func (s *Server) installsAvailable(w http.ResponseWriter) bool {
	if s.Installs == nil {
		WriteError(w, http.StatusServiceUnavailable, "installs.unavailable", "installing games is not available")
		return false
	}
	return true
}

// handleInstalls lists the installs, the one in progress first; changes come as
// "installs.changed", the end as "install.done" / "install.failed".
func (s *Server) handleInstalls(w http.ResponseWriter, _ *http.Request) {
	if s.Installs == nil {
		WriteJSON(w, http.StatusOK, installs.Status{Installs: []installs.Install{}})
		return
	}
	WriteJSON(w, http.StatusOK, s.Installs.Status())
}

// handleInstallInspect: {installer, kind} → {need, exact, free, licence}.
func (s *Server) handleInstallInspect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Installer string `json:"installer"`
		Kind      string `json:"kind"`
	}
	if !s.installsAvailable(w) || !decode(w, r, &req) {
		return
	}
	info, err := s.Installs.Inspect(req.Installer, req.Kind)
	if err != nil {
		s.installsError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, info)
}

// handleInstall queues an install (202).
func (s *Server) handleInstall(w http.ResponseWriter, r *http.Request) {
	var req installs.Request
	if !s.installsAvailable(w) || !decode(w, r, &req) {
		return
	}
	if err := s.Installs.Install(req); err != nil {
		s.installsError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

type destBody struct {
	Dest string `json:"dest"`
}

// handleInstallStop stops an install, or takes it out of the queue (204).
func (s *Server) handleInstallStop(w http.ResponseWriter, r *http.Request) {
	var req destBody
	if !s.installsAvailable(w) || !decode(w, r, &req) {
		return
	}
	if err := s.Installs.Stop(req.Dest); err != nil {
		s.installsError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleUninstall removes a game the console installed (204).
func (s *Server) handleUninstall(w http.ResponseWriter, r *http.Request) {
	var req destBody
	if !s.installsAvailable(w) || !decode(w, r, &req) {
		return
	}
	if err := s.Installs.Uninstall(req.Dest); err != nil {
		s.installsError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
