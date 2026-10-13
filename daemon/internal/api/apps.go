// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
)

// Apps is implemented by *apps.Manager.
type Apps interface {
	Status() apps.Status
	Launch(req apps.LaunchRequest) (apps.App, error)
	Close(id string) error
	// CloseAll closes every app and waits until they have ended, at most `wait`.
	CloseAll(wait time.Duration)
	Kill(id string) error
	FocusApp(id string) error
	Steam(req apps.SteamRequest) error
	SteamStatus() apps.SteamStatus
}

var appErrors = []struct {
	err    error
	status int
	code   string
}{
	{apps.ErrInvalid, http.StatusBadRequest, "apps.invalid"},
	{apps.ErrNotFound, http.StatusBadRequest, "apps.program_not_found"},
	{apps.ErrNoSession, http.StatusConflict, "apps.no_session"},
	{apps.ErrAlreadyRunning, http.StatusConflict, "apps.already_running"},
	{apps.ErrGameRunning, http.StatusConflict, "apps.game_running"},
	{apps.ErrUnknownApp, http.StatusNotFound, "apps.unknown_app"},
	{apps.ErrFocusUnsupported, http.StatusConflict, "apps.focus_unsupported"},
	{apps.ErrNoWindow, http.StatusConflict, "apps.no_window"},
	{apps.ErrSteamNeedsInstall, http.StatusConflict, "steam.needs_install"},
}

func (s *Server) appsAvailable(w http.ResponseWriter) bool {
	if s.Apps == nil {
		WriteError(w, http.StatusServiceUnavailable, "apps.unavailable",
			"apps cannot be started (no connection to the user's systemd)")
		return false
	}
	return true
}

func (s *Server) appsError(w http.ResponseWriter, err error) {
	for _, e := range appErrors {
		if errors.Is(err, e.err) {
			WriteError(w, e.status, e.code, err.Error())
			return
		}
	}
	s.Log.Warn("apps request failed", "err", err)
	WriteError(w, http.StatusBadGateway, "apps.failed", err.Error())
}

func (s *Server) handleApps(w http.ResponseWriter, _ *http.Request) {
	if !s.appsAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.Apps.Status())
}

func (s *Server) handleAppLaunch(w http.ResponseWriter, r *http.Request) {
	var req apps.LaunchRequest
	if !s.appsAvailable(w) || !decode(w, r, &req) {
		return
	}
	app, err := s.Apps.Launch(req)
	if err != nil {
		s.appsError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, app)
}

// handleAppAction serves close, kill and focus on /v1/apps/{id}/….
func (s *Server) handleAppAction(action func(Apps, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.appsAvailable(w) {
			return
		}
		if err := action(s.Apps, r.PathValue("id")); err != nil {
			s.appsError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleSteamStatus(w http.ResponseWriter, _ *http.Request) {
	if !s.appsAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.Apps.SteamStatus())
}

// handleSteam opens Steam's library or store, or runs a Steam game
// ([[step-4.2-steam-integration]]): 202, the game is listed in /v1/apps once Steam has started it.
func (s *Server) handleSteam(w http.ResponseWriter, r *http.Request) {
	var req apps.SteamRequest
	if !s.appsAvailable(w) || !decode(w, r, &req) {
		return
	}
	if err := s.Apps.Steam(req); err != nil {
		s.appsError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
