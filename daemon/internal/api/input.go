// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/vinput"
)

// VirtualInput is implemented by *vinput.Input.
type VirtualInput interface {
	TypeText(text string) error
	PressKey(name string, modifiers []string) error
	MovePointer(dx, dy, wheel int) error
	Click(button string) error
	Layout() string
	SetLayout(name string) error
}

const maxBody = 64 << 10

// decode reads a JSON body into v; it writes the error response and returns false on failure.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func (s *Server) inputAvailable(w http.ResponseWriter) bool {
	if s.Input == nil {
		WriteError(w, http.StatusServiceUnavailable, "input.unavailable",
			"virtual input is not available (no access to /dev/uinput)")
		return false
	}
	return true
}

func (s *Server) inputError(w http.ResponseWriter, err error) {
	var uc *vinput.UnsupportedCharError
	switch {
	case errors.As(err, &uc):
		WriteError(w, http.StatusUnprocessableEntity, "input.unsupported_character", err.Error())
	case errors.Is(err, vinput.ErrUnknownKey):
		WriteError(w, http.StatusBadRequest, "input.unknown_key", err.Error())
	default:
		s.Log.Error("virtual input failed", "err", err)
		WriteError(w, http.StatusInternalServerError, "internal", err.Error())
	}
}

func (s *Server) handleInputText(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if !s.inputAvailable(w) || !decode(w, r, &body) {
		return
	}
	if err := s.Input.TypeText(body.Text); err != nil {
		s.inputError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleInputKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key       string   `json:"key"`
		Modifiers []string `json:"modifiers"`
	}
	if !s.inputAvailable(w) || !decode(w, r, &body) {
		return
	}
	if err := s.Input.PressKey(body.Key, body.Modifiers); err != nil {
		s.inputError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleInputPointer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DX    int `json:"dx"`
		DY    int `json:"dy"`
		Wheel int `json:"wheel"`
	}
	if !s.inputAvailable(w) || !decode(w, r, &body) {
		return
	}
	if err := s.Input.MovePointer(body.DX, body.DY, body.Wheel); err != nil {
		s.inputError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleInputClick(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Button string `json:"button"`
	}
	if !s.inputAvailable(w) || !decode(w, r, &body) {
		return
	}
	if err := s.Input.Click(body.Button); err != nil {
		s.inputError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetLayout(w http.ResponseWriter, _ *http.Request) {
	if !s.inputAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"layout": s.Input.Layout(), "supported": vinput.LayoutNames()})
}

func (s *Server) handlePutLayout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Layout string `json:"layout"`
	}
	if !s.inputAvailable(w) || !decode(w, r, &body) {
		return
	}
	if err := s.Input.SetLayout(body.Layout); err != nil {
		WriteError(w, http.StatusBadRequest, "input.unsupported_layout", err.Error())
		return
	}
	// A keyboard plugged in types with the same layout (Settings → Language)
	if s.Display != nil {
		if err := s.Display.SetKeyboardLayout(body.Layout); err != nil {
			s.Log.Warn("cannot set the layout of physical keyboards", "err", err)
		}
	}
	s.Broker.Publish("input.layout_changed", map[string]string{"layout": body.Layout})
	w.WriteHeader(http.StatusNoContent)
}
