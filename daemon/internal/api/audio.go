// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/audio"
)

// Audio is implemented by *audio.Pulse.
type Audio interface {
	Status() audio.Status
	SetVolume(percent int) error
	SetMuted(muted bool) error
	SetOutput(id string) error
}

func (s *Server) audioAvailable(w http.ResponseWriter) bool {
	if s.Audio == nil {
		WriteError(w, http.StatusServiceUnavailable, "audio.unavailable", "audio is not available")
		return false
	}
	return true
}

func (s *Server) audioError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, audio.ErrUnavailable):
		WriteError(w, http.StatusServiceUnavailable, "audio.unavailable", err.Error())
	case errors.Is(err, audio.ErrNoOutput):
		WriteError(w, http.StatusConflict, "audio.no_output", err.Error())
	case errors.Is(err, audio.ErrUnknownOutput):
		WriteError(w, http.StatusNotFound, "audio.unknown_output", err.Error())
	default:
		s.Log.Warn("audio request failed", "err", err)
		WriteError(w, http.StatusBadGateway, "audio.failed", err.Error())
	}
}

func (s *Server) handleAudio(w http.ResponseWriter, _ *http.Request) {
	if !s.audioAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.Audio.Status())
}

func (s *Server) handleAudioVolume(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Volume *int `json:"volume"`
	}
	if !s.audioAvailable(w) || !decode(w, r, &body) {
		return
	}
	if body.Volume == nil || *body.Volume < 0 || *body.Volume > 100 {
		WriteError(w, http.StatusBadRequest, "bad_request", "volume must be between 0 and 100")
		return
	}
	if err := s.Audio.SetVolume(*body.Volume); err != nil {
		s.audioError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAudioMute(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Muted *bool `json:"muted"`
	}
	if !s.audioAvailable(w) || !decode(w, r, &body) {
		return
	}
	if body.Muted == nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "muted is required")
		return
	}
	if err := s.Audio.SetMuted(*body.Muted); err != nil {
		s.audioError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAudioOutput(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if !s.audioAvailable(w) || !decode(w, r, &body) {
		return
	}
	if err := s.Audio.SetOutput(body.ID); err != nil {
		s.audioError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
