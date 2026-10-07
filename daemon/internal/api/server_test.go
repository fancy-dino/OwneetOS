// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/client"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// start runs a server on a temporary socket and returns a client for it.
func start(t *testing.T) (*Server, *http.Client, string) {
	t.Helper()
	dir := t.TempDir()
	osRelease := filepath.Join(dir, "os-release")
	mode := filepath.Join(dir, "session-mode")
	os.WriteFile(osRelease, []byte("NAME=\"OwneetOS\"\nIMAGE_VERSION=2026.10.07\n"), 0o644)
	os.WriteFile(mode, []byte("gamescope\n"), 0o644)

	sock := filepath.Join(dir, "owneetd.sock")
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		Broker:          events.NewBroker(),
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		OSRelease:       osRelease,
		SessionModeFile: mode,
		Heartbeat:       50 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, ln) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Serve: %v", err)
		}
	})
	return s, client.New(sock), sock
}

func TestSocketIsPrivate(t *testing.T) {
	_, _, sock := start(t)
	info, err := os.Stat(sock)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("socket permissions %o, want 600", perm)
	}
}

func TestSecondInstanceRefused(t *testing.T) {
	_, _, sock := start(t)
	if _, err := Listen(sock); err == nil {
		t.Fatal("a second daemon could take over a socket in use")
	}
}

func TestStatus(t *testing.T) {
	_, c, _ := start(t)
	resp, err := c.Get("http://owneetd/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var st Status
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st.OwneetOSVersion != "2026.10.07" || st.SessionMode != "gamescope" || st.DaemonVersion == "" {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestControllersEmptyList(t *testing.T) {
	_, c, _ := start(t)
	resp, err := c.Get("http://owneetd/v1/controllers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(data)) != `{"controllers":[]}` {
		t.Fatalf("got %d %s", resp.StatusCode, data)
	}
}

func TestUnknownEndpointIsJSONError(t *testing.T) {
	_, c, _ := start(t)
	resp, err := c.Get("http://owneetd/v1/nope")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body ErrorBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound || body.Error.Code != "not_found" {
		t.Fatalf("got %d %+v", resp.StatusCode, body)
	}
}

func TestEventStream(t *testing.T) {
	s, c, _ := start(t)
	resp, err := c.Get("http://owneetd/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}

	// Wait until the stream is subscribed, then publish.
	deadline := time.Now().Add(2 * time.Second)
	for s.Broker.Subscribers() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	s.Broker.Publish("guide.pressed", map[string]string{"controller": "1"})

	sc := bufio.NewScanner(resp.Body)
	var gotEvent, gotData bool
	for sc.Scan() && !(gotEvent && gotData) {
		line := sc.Text()
		if line == "event: guide.pressed" {
			gotEvent = true
		}
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"type":"guide.pressed"`) {
			gotData = true
		}
	}
	if !gotEvent || !gotData {
		t.Fatalf("event not received (event=%v data=%v, err=%v)", gotEvent, gotData, sc.Err())
	}
}
