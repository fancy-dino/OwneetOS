// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
	"github.com/fancy-dino/OwneetOS/daemon/internal/player"
)

// Player is implemented by *player.Manager ([[step-5.6-local-videos-and-music]]).
type Player interface {
	Open(req player.Request) (player.State, error)
	Status() player.State
}

// maxPlayerBody: a folder's or an album's list (player.MaxItems) with the player's texts
const maxPlayerBody = 4 << 20

func (s *Server) playerAvailable(w http.ResponseWriter) bool {
	if s.Player == nil {
		WriteError(w, http.StatusServiceUnavailable, "player.unavailable",
			"the player cannot be started (no connection to the user's systemd)")
		return false
	}
	return true
}

func (s *Server) handlePlayer(w http.ResponseWriter, _ *http.Request) {
	if !s.playerAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.Player.Status())
}

// handlePlayerOpen starts the player on a list (another one open closes first): 201 with what
// plays; its changes come as "player.changed", its end as "app.exited" (id "player").
func (s *Server) handlePlayerOpen(w http.ResponseWriter, r *http.Request) {
	if !s.playerAvailable(w) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPlayerBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req player.Request
	if err := dec.Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	st, err := s.Player.Open(req)
	switch {
	case err == nil:
		WriteJSON(w, http.StatusCreated, st)
	case errors.Is(err, player.ErrInvalid):
		WriteError(w, http.StatusBadRequest, "player.invalid", err.Error())
	case errors.Is(err, player.ErrNotFound):
		WriteError(w, http.StatusNotFound, "player.not_found", err.Error())
	case errors.Is(err, apps.ErrGameRunning), errors.Is(err, apps.ErrNoSession), errors.Is(err, apps.ErrNotFound):
		s.appsError(w, err)
	default:
		s.Log.Warn("the player could not start", "err", err)
		WriteError(w, http.StatusBadGateway, "player.failed", err.Error())
	}
}
