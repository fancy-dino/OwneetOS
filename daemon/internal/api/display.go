// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/display"
)

// Display is implemented by *display.Manager (Settings → Display).
type Display interface {
	Status() display.Status
	SetScreen(id string) error
	SetMode(mode string) error
	Confirm() error
	Revert() error
	SetBrightness(percent int) error
	SetKeyboardLayout(layout string) error
}

var displayErrors = []struct {
	err    error
	status int
	code   string
}{
	{display.ErrNotGamescope, http.StatusConflict, "display.not_gamescope"},
	{display.ErrUnknownScreen, http.StatusNotFound, "display.unknown_screen"},
	{display.ErrUnknownMode, http.StatusBadRequest, "display.unknown_mode"},
	{display.ErrAppsOpen, http.StatusConflict, "display.apps_open"},
	{display.ErrNotPending, http.StatusConflict, "display.not_pending"},
	{display.ErrNoBrightness, http.StatusConflict, "display.no_brightness"},
}

func (s *Server) displayAvailable(w http.ResponseWriter) bool {
	if s.Display == nil {
		WriteError(w, http.StatusServiceUnavailable, "display.unavailable", "display settings are not available")
		return false
	}
	return true
}

func (s *Server) displayError(w http.ResponseWriter, err error) {
	for _, e := range displayErrors {
		if errors.Is(err, e.err) {
			WriteError(w, e.status, e.code, err.Error())
			return
		}
	}
	s.Log.Warn("display change failed", "err", err)
	WriteError(w, http.StatusBadGateway, "display.failed", err.Error())
}

func (s *Server) handleDisplay(w http.ResponseWriter, _ *http.Request) {
	if !s.displayAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.Display.Status())
}

func (s *Server) handleDisplayScreen(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if !s.displayAvailable(w) || !decode(w, r, &body) {
		return
	}
	s.displayResult(w, s.Display.SetScreen(body.ID))
}

func (s *Server) handleDisplayMode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string `json:"mode"`
	}
	if !s.displayAvailable(w) || !decode(w, r, &body) {
		return
	}
	s.displayResult(w, s.Display.SetMode(body.Mode))
}

func (s *Server) handleDisplayBrightness(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Value *int `json:"value"`
	}
	if !s.displayAvailable(w) || !decode(w, r, &body) {
		return
	}
	if body.Value == nil || *body.Value < 0 || *body.Value > 100 {
		WriteError(w, http.StatusBadRequest, "bad_request", "value must be between 0 and 100")
		return
	}
	s.displayResult(w, s.Display.SetBrightness(*body.Value))
}

func (s *Server) handleDisplayAction(action func(Display) error) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if !s.displayAvailable(w) {
			return
		}
		s.displayResult(w, action(s.Display))
	}
}

func (s *Server) displayResult(w http.ResponseWriter, err error) {
	if err != nil {
		s.displayError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
