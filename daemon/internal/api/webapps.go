// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/webapps"
)

// WebApps is implemented by *webapps.Hub: the console's Brave extension (through owneet-bridge)
// and the interface meet there ([[extension-design]]).
type WebApps interface {
	SetLook(l webapps.Look)
	Look() webapps.Look
	SetKeyboard(k webapps.Keyboard) error
	KeyboardState() webapps.Keyboard
	KeyboardShown() error
	Type(t webapps.Text) error
	AskClose() error
}

func (s *Server) webAppsAvailable(w http.ResponseWriter) bool {
	if s.WebApps == nil {
		WriteError(w, http.StatusServiceUnavailable, "webapps.unavailable", "web apps are not available")
		return false
	}
	return true
}

func (s *Server) webAppsError(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, webapps.ErrInvalid):
		WriteError(w, http.StatusBadRequest, "webapps.invalid", err.Error())
	case errors.Is(err, webapps.ErrNoWebApp):
		WriteError(w, http.StatusConflict, "webapps.no_webapp", err.Error())
	case errors.Is(err, webapps.ErrKeyboardClosed):
		WriteError(w, http.StatusConflict, "webapps.keyboard_closed", err.Error())
	default:
		WriteError(w, http.StatusInternalServerError, "webapps.failed", err.Error())
	}
}

// handleWebAppsLook: the accent colour and language the extension draws with.
func (s *Server) handleWebAppsLook(w http.ResponseWriter, _ *http.Request) {
	if s.webAppsAvailable(w) {
		WriteJSON(w, http.StatusOK, s.WebApps.Look())
	}
}

func (s *Server) handleWebAppsKeyboardState(w http.ResponseWriter, _ *http.Request) {
	if s.webAppsAvailable(w) {
		WriteJSON(w, http.StatusOK, s.WebApps.KeyboardState())
	}
}

// handleWebAppsKeyboard opens (extension) or closes (interface or extension) the console's keyboard.
func (s *Server) handleWebAppsKeyboard(w http.ResponseWriter, r *http.Request) {
	if !s.webAppsAvailable(w) {
		return
	}
	var k webapps.Keyboard
	if decode(w, r, &k) {
		s.webAppsError(w, s.WebApps.SetKeyboard(k))
	}
}

// handleWebAppsKeyboardShown: the interface confirms that it shows the keyboard.
func (s *Server) handleWebAppsKeyboardShown(w http.ResponseWriter, _ *http.Request) {
	if s.webAppsAvailable(w) {
		s.webAppsError(w, s.WebApps.KeyboardShown())
	}
}

// handleWebAppsText: a key of the console's keyboard, typed into the web app's field.
func (s *Server) handleWebAppsText(w http.ResponseWriter, r *http.Request) {
	if !s.webAppsAvailable(w) {
		return
	}
	var t webapps.Text
	if decode(w, r, &t) {
		s.webAppsError(w, s.WebApps.Type(t))
	}
}

// handleWebAppsAskClose: B with nothing to go back to in the web app.
func (s *Server) handleWebAppsAskClose(w http.ResponseWriter, _ *http.Request) {
	if s.webAppsAvailable(w) {
		s.webAppsError(w, s.WebApps.AskClose())
	}
}
