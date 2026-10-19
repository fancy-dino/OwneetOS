// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/catalog"
	"github.com/fancy-dino/OwneetOS/daemon/internal/webapps"
)

// Catalog is implemented by *catalog.Manager (the optional apps,
// [[step-5.0.0-optional-web-apps]]).
type Catalog interface {
	Status() catalog.Status
	Install(id string) error
	Remove(id string) error
	// Open opens an installed web app ([[step-5.1-brave-in-the-os]]).
	Open(id string) error
	// The user's consent to Google's terms for Widevine ([[step-5.4-netflix]]): asked before a
	// web app opens, with Google's live terms page to read.
	Widevine() catalog.WidevineStatus
	AcceptWidevine() error
	OpenTerms(language string) error
}

func (s *Server) catalogAvailable(w http.ResponseWriter) bool {
	if s.Catalog == nil {
		WriteError(w, http.StatusServiceUnavailable, "apps.unavailable",
			"optional apps are not available (no connection to the system bus)")
		return false
	}
	return true
}

func (s *Server) handleCatalog(w http.ResponseWriter, _ *http.Request) {
	if !s.catalogAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.Catalog.Status())
}

// handleCatalogAction installs or removes a web app: 202, the outcome comes as
// "apps.catalog.changed".
func (s *Server) handleCatalogAction(action func(Catalog, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.catalogAvailable(w) {
			return
		}
		err := action(s.Catalog, r.PathValue("id"))
		switch {
		case err == nil:
			w.WriteHeader(http.StatusAccepted)
		case errors.Is(err, catalog.ErrUnknown):
			WriteError(w, http.StatusNotFound, "apps.unknown", err.Error())
		case errors.Is(err, catalog.ErrBusy):
			WriteError(w, http.StatusConflict, "apps.busy", err.Error())
		case errors.Is(err, catalog.ErrNeedsInstall):
			WriteError(w, http.StatusConflict, "apps.needs_install", err.Error())
		case errors.Is(err, catalog.ErrNoSpace):
			WriteError(w, http.StatusConflict, "apps.no_space", err.Error())
		default:
			s.Log.Warn("optional app failed to start", "err", err)
			WriteError(w, http.StatusBadGateway, "apps.failed", err.Error())
		}
	}
}

// handleCatalogOpen opens an installed web app, or brings it back when it is open (204). It runs
// like a game: one game or web app at a time. The body may carry the look for the extension:
// {"accent": "#FF8A5B", "language": "it"}.
func (s *Server) handleCatalogOpen(w http.ResponseWriter, r *http.Request) {
	if !s.catalogAvailable(w) {
		return
	}
	// Optional body: the look the console's extension draws with ([[extension-design]])
	if r.ContentLength != 0 {
		var look webapps.Look
		if !decode(w, r, &look) {
			return
		}
		if s.WebApps != nil {
			s.WebApps.SetLook(look)
		}
	}
	err := s.Catalog.Open(r.PathValue("id"))
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, catalog.ErrUnknown):
		WriteError(w, http.StatusNotFound, "apps.unknown", err.Error())
	case errors.Is(err, catalog.ErrNotInstalled):
		WriteError(w, http.StatusConflict, "apps.not_installed", err.Error())
	case errors.Is(err, catalog.ErrWidevineConsent):
		WriteError(w, http.StatusConflict, "apps.widevine_consent", err.Error())
	default:
		s.appsError(w, err)
	}
}

// handleWidevine: whether opening a web app must first ask for the consent to Google's terms.
func (s *Server) handleWidevine(w http.ResponseWriter, _ *http.Request) {
	if !s.catalogAvailable(w) {
		return
	}
	WriteJSON(w, http.StatusOK, s.Catalog.Widevine())
}

// handleWidevineAccept records the user's consent (204).
func (s *Server) handleWidevineAccept(w http.ResponseWriter, _ *http.Request) {
	if !s.catalogAvailable(w) {
		return
	}
	if err := s.Catalog.AcceptWidevine(); err != nil {
		s.Log.Warn("Widevine consent not saved", "err", err)
		WriteError(w, http.StatusInternalServerError, "apps.consent_not_saved", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleWidevineTerms opens Google's terms page like a web app (204). Optional body:
// {"language": "it"}.
func (s *Server) handleWidevineTerms(w http.ResponseWriter, r *http.Request) {
	if !s.catalogAvailable(w) {
		return
	}
	var body struct {
		Language string `json:"language"`
	}
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	if err := s.Catalog.OpenTerms(body.Language); err != nil {
		s.appsError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
