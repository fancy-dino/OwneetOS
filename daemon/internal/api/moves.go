// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/moves"
)

// Moves is implemented by *moves.Manager (moving games between disks,
// [[step-6.6-moving-games-and-media-between-disks]]).
type Moves interface {
	Status() moves.Status
	Start(req moves.Request) error
	Pause() error
	Resume() error
	Stop() error
}

func (s *Server) handleMoves(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, s.Moves.Status())
}

// handleMovesStart: {"games": [{"dir", "title"}], "to": "internal" or a disk's folder, "remove"};
// 202, then "moves.changed".
func (s *Server) handleMovesStart(w http.ResponseWriter, r *http.Request) {
	var req moves.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "request.invalid", `expected {"games": [{"dir", "title"}], "to", "remove"}`)
		return
	}
	s.movesReply(w, s.Moves.Start(req))
}

func (s *Server) movesReply(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusAccepted)
	case errors.Is(err, moves.ErrInvalid):
		WriteError(w, http.StatusBadRequest, "moves.invalid", err.Error())
	case errors.Is(err, moves.ErrNoSpace):
		WriteError(w, http.StatusConflict, "moves.no_space", err.Error())
	case errors.Is(err, moves.ErrBusy):
		WriteError(w, http.StatusConflict, "moves.busy", err.Error())
	case errors.Is(err, moves.ErrIdle):
		WriteError(w, http.StatusConflict, "moves.idle", err.Error())
	default:
		s.Log.Warn("moving games", "err", err)
		WriteError(w, http.StatusInternalServerError, "moves.failed", err.Error())
	}
}

// handleMovesPause, Resume, Stop: the copy waits while the player is asked "Stop?", goes on, or
// stops (the partial copy is removed).
func (s *Server) handleMovesPause(w http.ResponseWriter, _ *http.Request) {
	s.movesReply(w, s.Moves.Pause())
}
func (s *Server) handleMovesResume(w http.ResponseWriter, _ *http.Request) {
	s.movesReply(w, s.Moves.Resume())
}
func (s *Server) handleMovesStop(w http.ResponseWriter, _ *http.Request) {
	s.movesReply(w, s.Moves.Stop())
}
