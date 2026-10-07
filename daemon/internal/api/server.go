// SPDX-License-Identifier: GPL-3.0-or-later

// Package api serves the owneetd HTTP + JSON API on a Unix socket (docs/daemon-design.md).
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
	"github.com/fancy-dino/OwneetOS/daemon/internal/gamepad"
	"github.com/fancy-dino/OwneetOS/daemon/internal/version"
)

// Server is the API server.
type Server struct {
	Broker *events.Broker
	Log    *slog.Logger
	// OSRelease is read for the OwneetOS version (default /etc/os-release).
	OSRelease string
	// SessionModeFile holds "gamescope" or "cage", written by owneet-session.
	SessionModeFile string
	// Heartbeat is the interval of keep-alive comments on the event stream.
	Heartbeat time.Duration
	// Controllers lists the connected game controllers (nil: none).
	Controllers interface{ List() []gamepad.Controller }

	started time.Time
}

// Handler returns the HTTP handler with every v1 route.
func (s *Server) Handler() http.Handler {
	if s.started.IsZero() {
		s.started = time.Now()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", s.handleStatus)
	mux.HandleFunc("GET /v1/events", s.handleEvents)
	mux.HandleFunc("GET /v1/controllers", s.handleControllers)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, http.StatusNotFound, "not_found", "no such endpoint: "+r.Method+" "+r.URL.Path)
	})
	return mux
}

// Listen creates the Unix socket at path, readable and writable only by the current user.
// A stale socket left by a previous run is replaced; a socket still in use is an error.
func Listen(path string) (net.Listener, error) {
	if _, err := os.Stat(path); err == nil {
		if conn, err := net.DialTimeout("unix", path, time.Second); err == nil {
			conn.Close()
			return nil, fmt.Errorf("%s is in use: is owneetd already running?", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

// Serve runs the API on ln until ctx is cancelled, then shuts down gracefully.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		// Request contexts end with ctx, so open event streams close on shutdown.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

// Status is the body of GET /v1/status.
type Status struct {
	DaemonVersion   string `json:"daemon_version"`
	OwneetOSVersion string `json:"owneetos_version"`
	SessionMode     string `json:"session_mode"`
	UptimeSeconds   int64  `json:"uptime_seconds"`
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, Status{
		DaemonVersion:   version.Version,
		OwneetOSVersion: osReleaseValue(s.OSRelease, "IMAGE_VERSION"),
		SessionMode:     readTrimmed(s.SessionModeFile),
		UptimeSeconds:   int64(time.Since(s.started).Seconds()),
	})
}

func (s *Server) handleControllers(w http.ResponseWriter, _ *http.Request) {
	list := []gamepad.Controller{}
	if s.Controllers != nil {
		list = s.Controllers.List()
	}
	WriteJSON(w, http.StatusOK, map[string]any{"controllers": list})
}

// handleEvents streams events as Server-Sent Events until the client or the daemon goes away.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal", "streaming not supported")
		return
	}
	ch, cancel := s.Broker.Subscribe(64)
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := s.Heartbeat
	if heartbeat <= 0 {
		heartbeat = 15 * time.Second
	}
	tick := time.NewTicker(heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		case ev, open := <-ch:
			if !open {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				s.Log.Error("cannot encode event", "type", ev.Type, "err", err)
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
			flusher.Flush()
		}
	}
}

// WriteJSON writes v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ErrorBody is the JSON shape of every error. Code is stable and is what the UI translates;
// Message is English, for logs.
type ErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// WriteError writes an error response.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	var body ErrorBody
	body.Error.Code = code
	body.Error.Message = message
	WriteJSON(w, status, body)
}

func readTrimmed(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// osReleaseValue returns KEY from an os-release file ("" if missing).
func osReleaseValue(path, key string) string {
	if path == "" {
		path = "/etc/os-release"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if ok && k == key {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}
