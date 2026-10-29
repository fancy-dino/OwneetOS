// SPDX-License-Identifier: GPL-3.0-or-later

// owneet-bridge is the native-messaging host of the console's Brave extension
// ([[extension-design]]): Brave starts it for the extension and talks to it on stdin / stdout
// (JSON messages, each after its length as a 4-byte number); it passes them to owneetd over its
// Unix socket, and owneetd's events back. It ends when Brave closes its stdin.
//
// From the extension: {"type": "hello"}, {"type": "keyboard", "open": true, "kind": "email",
// "label": "…", "value": "…"}, {"type": "keyboard", "open": false}, {"type": "ask_close"}.
// To the extension: {"type": "look", "accent": "#…", "language": "…"}, {"type": "text", "text": "…"},
// {"type": "key", "key": "backspace"}, {"type": "keyboard_closed", "next": true}.
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/client"
	"github.com/fancy-dino/OwneetOS/daemon/internal/config"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
	"github.com/fancy-dino/OwneetOS/daemon/internal/version"
	"github.com/fancy-dino/OwneetOS/daemon/internal/webapps"
)

// maxMessage: Brave sends at most 64 MiB; the extension's messages are small.
const maxMessage = 1 << 20

// readMessage reads one native message.
func readMessage(r io.Reader) (map[string]any, error) {
	var n uint32
	if err := binary.Read(r, binary.NativeEndian, &n); err != nil {
		return nil, err
	}
	if n > maxMessage {
		return nil, fmt.Errorf("message too long: %d bytes", n)
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	var msg map[string]any
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// writer sends native messages; events and replies come from different goroutines.
type writer struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *writer) send(msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := binary.Write(w.w, binary.NativeEndian, uint32(len(data))); err != nil {
		return err
	}
	_, err = w.w.Write(data)
	return err
}

type bridge struct {
	http *http.Client
	out  *writer
	log  *slog.Logger
}

func (b *bridge) call(method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, "http://owneetd"+path, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return b.http.Do(req)
}

func (b *bridge) post(path string, body any) {
	resp, err := b.call(http.MethodPost, path, body)
	if err != nil {
		b.log.Warn("owneetd not reached", "path", path, "err", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		b.log.Warn("owneetd refused", "path", path, "status", resp.StatusCode, "body", strings.TrimSpace(string(data)))
	}
}

func (b *bridge) sendLook() {
	resp, err := b.call(http.MethodGet, "/v1/webapps/look", nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var look webapps.Look
	if resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&look) == nil {
		_ = b.out.send(map[string]any{"type": "look", "accent": look.Accent, "language": look.Language})
	}
}

// fromExtension handles one message of the extension.
func (b *bridge) fromExtension(msg map[string]any) {
	str := func(k string) string {
		s, _ := msg[k].(string)
		return s
	}
	switch str("type") {
	case "hello":
		b.sendLook()
	case "keyboard":
		open, _ := msg["open"].(bool)
		b.post("/v1/webapps/keyboard", webapps.Keyboard{Open: open, Kind: str("kind"), Label: str("label"), Value: str("value")})
	case "ask_close":
		b.post("/v1/webapps/ask-close", nil)
	default:
		b.log.Debug("unknown message from the extension", "type", str("type"))
	}
}

// forEvent turns an owneetd event into a message for the extension (nil: none).
func forEvent(ev events.Event) any {
	data, _ := json.Marshal(ev.Data)
	switch ev.Type {
	case "webapp.keyboard":
		var k webapps.Keyboard
		if json.Unmarshal(data, &k) == nil && !k.Open {
			return map[string]any{"type": "keyboard_closed", "next": k.Next}
		}
	case "webapp.text":
		var t webapps.Text
		if json.Unmarshal(data, &t) == nil {
			if t.Key != "" {
				return map[string]any{"type": "key", "key": t.Key}
			}
			return map[string]any{"type": "text", "text": t.Text}
		}
	}
	return nil
}

// followEvents passes owneetd's events to the extension, connecting again when owneetd restarts.
func (b *bridge) followEvents() {
	for {
		if err := b.streamEvents(); err != nil {
			b.log.Debug("event stream ended", "err", err)
		}
		time.Sleep(2 * time.Second)
	}
}

func (b *bridge) streamEvents() error {
	resp, err := b.call(http.MethodGet, "/v1/events", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var ev events.Event
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if msg := forEvent(ev); msg != nil {
			if err := b.out.send(msg); err != nil {
				os.Exit(0) // Brave went away
			}
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return errors.New("closed by owneetd")
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("owneet-bridge", version.Version)
		return
	}
	// Brave shows the host's standard error in its own log (the web app's journal)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	socket, err := config.DefaultSocket()
	if err != nil {
		log.Error("no owneetd socket", "err", err)
		os.Exit(1)
	}
	b := &bridge{http: client.New(socket), out: &writer{w: os.Stdout}, log: log}
	go b.followEvents()
	in := bufio.NewReader(os.Stdin)
	for {
		msg, err := readMessage(in)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Warn("stopped reading the extension", "err", err)
			}
			return
		}
		b.fromExtension(msg)
	}
}
