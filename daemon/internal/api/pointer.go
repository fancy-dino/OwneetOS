// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/pointer"
)

// Pointer is implemented by *pointer.Manager: the controller as a mouse for a running app's
// window ([[step-4.7-controller-pointer]]).
type Pointer interface {
	Status() pointer.Status
	On(app string) error
	Off(reason string)
}

// handlePointer: {on, app, reason}; changes come as "pointer.changed", the keyboard asked as
// "pointer.type" {app, reason: button / field}.
func (s *Server) handlePointer(w http.ResponseWriter, _ *http.Request) {
	if s.Pointer == nil {
		WriteJSON(w, http.StatusOK, pointer.Status{})
		return
	}
	WriteJSON(w, http.StatusOK, s.Pointer.Status())
}

// handlePointerSet: {"on": true, "app": id} turns it on for that app's next window (the interface
// then resumes the app); {"on": false} turns it off (204).
func (s *Server) handlePointerSet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On  bool   `json:"on"`
		App string `json:"app"`
	}
	if !decode(w, r, &body) {
		return
	}
	if s.Pointer == nil {
		WriteError(w, http.StatusServiceUnavailable, "pointer.unavailable", "the pointer is not available")
		return
	}
	if !body.On {
		s.Pointer.Off("user")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch err := s.Pointer.On(body.App); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, pointer.ErrUnknownApp):
		WriteError(w, http.StatusNotFound, "apps.unknown", err.Error())
	case errors.Is(err, pointer.ErrUnavailable):
		WriteError(w, http.StatusServiceUnavailable, "pointer.unavailable", err.Error())
	default:
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
	}
}
