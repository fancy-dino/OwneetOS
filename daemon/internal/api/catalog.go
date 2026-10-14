// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"net/http"

	"github.com/fancy-dino/OwneetOS/daemon/internal/catalog"
)

// Catalog is implemented by *catalog.Manager (the optional apps,
// [[step-5.0.0-optional-web-apps]]).
type Catalog interface {
	Status() catalog.Status
	Install(id string) error
	Remove(id string) error
	// Open opens an installed web app ([[step-5.1-brave-in-the-os]]).
	Open(id string) error
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
// like a game: one game or web app at a time.
func (s *Server) handleCatalogOpen(w http.ResponseWriter, r *http.Request) {
	if !s.catalogAvailable(w) {
		return
	}
	err := s.Catalog.Open(r.PathValue("id"))
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, catalog.ErrUnknown):
		WriteError(w, http.StatusNotFound, "apps.unknown", err.Error())
	case errors.Is(err, catalog.ErrNotInstalled):
		WriteError(w, http.StatusConflict, "apps.not_installed", err.Error())
	default:
		s.appsError(w, err)
	}
}
