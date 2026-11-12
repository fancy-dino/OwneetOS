// SPDX-License-Identifier: GPL-3.0-or-later

package player

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

type fakeApps struct {
	mu        sync.Mutex
	running   map[string]bool
	focus     string
	launched  []apps.LaunchRequest
	closed    []string
	launchErr error
	stubborn  bool // Close does not end the player
}

func (f *fakeApps) Status() apps.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := apps.Status{Focus: f.focus}
	for id := range f.running {
		st.Apps = append(st.Apps, apps.App{ID: id})
	}
	return st
}

func (f *fakeApps) Launch(req apps.LaunchRequest) (apps.App, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.launchErr != nil {
		return apps.App{}, f.launchErr
	}
	if f.running == nil {
		f.running = map[string]bool{}
	}
	f.running[req.ID] = true
	f.launched = append(f.launched, req)
	return apps.App{ID: req.ID, Name: req.Name, Kind: req.Kind}, nil
}

func (f *fakeApps) Close(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, id)
	if !f.stubborn {
		delete(f.running, id)
	}
	return nil
}

func (f *fakeApps) Kill(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.running, id)
	return nil
}

func look() Look {
	return Look{Bg: "#0E1424", Surface: "#172036", Raised: "#222D4A", Line: "#2E3A5C", Fg: "#EEF1F8",
		Muted: "#8F9AB5", Accent: "#FF8A5B", OnAccent: "#1E0E06", DisplayFont: "Bricolage Grotesque",
		TextFont: "Lexend", TextScale: 1, Glyphs: "xbox"}
}

// setup: two episodes on disk, a manager with a fake apps manager in a cage session
func setup(t *testing.T) (*Manager, *fakeApps, Request) {
	t.Helper()
	dir := t.TempDir()
	season := filepath.Join(dir, "Videos", "Harbour Tales", "Season 1")
	if err := os.MkdirAll(season, 0o755); err != nil {
		t.Fatal(err)
	}
	var items []Item
	for _, name := range []string{"Episode 1.mkv", "Episode 2.mkv"} {
		p := filepath.Join(season, name)
		if err := os.WriteFile(p, []byte("video"), 0o644); err != nil {
			t.Fatal(err)
		}
		items = append(items, Item{Path: p, Title: strings.TrimSuffix(name, ".mkv"), Folder: "Harbour Tales › Season 1"})
	}
	fa := &fakeApps{focus: "home"}
	m := &Manager{Apps: fa, Broker: events.NewBroker(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Session:   func() (apps.Session, error) { return apps.Session{Mode: "cage"}, nil },
		ConfigDir: "/usr/share/owneet/player", RuntimeDir: filepath.Join(dir, "run"),
		WatchLaterDir: filepath.Join(dir, "watch_later"), CloseWait: 300 * time.Millisecond}
	req := Request{Kind: "videos", Items: items, Index: 1, Autoplay: true,
		Texts: map[string]string{"player.resumed": "Resumed at {t}"}, Languages: map[string]string{"en": "English"},
		Codes: map[string]string{"eng": "en"}, Look: look()}
	return m, fa, req
}

func TestArgs(t *testing.T) {
	got := Args("/cfg", "/run/user/1000/owneet", "/home/o/.local/state/owneet/watch_later", "gamescope", "videos", 2)
	want := []string{"mpv", "--config-dir=/cfg", "--input-ipc-server=/run/user/1000/owneet/player.sock",
		"--script-opts=owneet-session=/run/user/1000/owneet/player.json",
		"--watch-later-dir=/home/o/.local/state/owneet/watch_later",
		"--vo=gpu-next,gpu,x11", "--gpu-context=x11egl,x11vk", "--playlist=/run/user/1000/owneet/player.m3u", "--playlist-start=2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("videos in gamescope:\n got %q\nwant %q", got, want)
	}
	got = Args("/cfg", "/r", "/w", "cage", "music", 0)
	want = []string{"mpv", "--config-dir=/cfg", "--input-ipc-server=/r/player.sock", "--script-opts=owneet-session=/r/player.json",
		"--watch-later-dir=/w", "--profile=owneet-music", "--vo=gpu-next,gpu,wlshm", "--gpu-context=wayland,waylandvk",
		"--playlist=/r/player.m3u", "--playlist-start=0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("music in cage:\n got %q\nwant %q", got, want)
	}
}

func TestOpenStartsThePlayer(t *testing.T) {
	m, fa, req := setup(t)
	events, stop := m.Broker.Subscribe(4)
	defer stop()
	// "Watch from the start": the saved position of the first file played is forgotten
	if err := os.MkdirAll(m.WatchLaterDir, 0o700); err != nil {
		t.Fatal(err)
	}
	saved := WatchLaterFile(m.WatchLaterDir, req.Items[1].Path)
	other := WatchLaterFile(m.WatchLaterDir, req.Items[0].Path)
	for _, f := range []string{saved, other} {
		if err := os.WriteFile(f, []byte("start=95\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	req.FromStart = true
	req.AudioLanguage, req.Subtitles = "it", "off"
	st, err := m.Open(req)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Open || st.Index != 1 || st.Count != 2 || st.Title != "Episode 2" || st.Folder != "Harbour Tales › Season 1" || !st.Autoplay ||
		st.AudioLanguage != "it" || st.Subtitles != "off" {
		t.Fatalf("state: %+v", st)
	}
	if _, err := os.Stat(saved); !os.IsNotExist(err) {
		t.Fatal("the saved position of the video watched from the start is still there")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("another video's saved position was removed")
	}
	if len(fa.launched) != 1 {
		t.Fatalf("launched %d times", len(fa.launched))
	}
	l := fa.launched[0]
	if l.ID != AppID || l.Kind != apps.KindMedia || l.Name != "Episode 2" {
		t.Fatalf("launch: %+v", l)
	}
	want := Args(m.ConfigDir, m.RuntimeDir, m.WatchLaterDir, "cage", "videos", 1)
	if !reflect.DeepEqual(l.Command, want) {
		t.Fatalf("command:\n got %q\nwant %q", l.Command, want)
	}
	// the list, in a playlist file (any length, never on the command line)
	list, err := os.ReadFile(filepath.Join(m.RuntimeDir, "player.m3u"))
	if err != nil || string(list) != "#EXTM3U\n"+req.Items[0].Path+"\n"+req.Items[1].Path+"\n" {
		t.Fatalf("playlist %q %v", list, err)
	}
	// the session file the script reads: private, with the session's mode and what was sent
	path := filepath.Join(m.RuntimeDir, "player.json")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("session file: %v %v", info, err)
	}
	var s session
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	if s.Kind != "videos" || s.Mode != "cage" || !s.Autoplay || len(s.Items) != 2 || s.Texts["player.resumed"] != "Resumed at {t}" ||
		s.Codes["eng"] != "en" || s.Look.Accent != "#FF8A5B" || s.AudioLanguage != "it" || s.Subtitles != "off" {
		t.Fatalf("session: %+v", s)
	}
	select {
	case ev := <-events:
		if ev.Type != "player.changed" || ev.Data.(State).Title != "Episode 2" {
			t.Fatalf("event: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no player.changed")
	}
}

func TestOpenRefuses(t *testing.T) {
	m, fa, req := setup(t)
	bad := map[string]func(r *Request){
		"kind":     func(r *Request) { r.Kind = "games" },
		"no items": func(r *Request) { r.Items = nil },
		"index":    func(r *Request) { r.Index = 2 },
		"relative": func(r *Request) { r.Items[0].Path = "Videos/a.mkv" },
		"unclean":  func(r *Request) { r.Items[0].Path = "/media/../etc/passwd" },
		"colour":   func(r *Request) { r.Look.Bg = "red" },
		"glyphs":   func(r *Request) { r.Look.Glyphs = "sega" },
		"font":     func(r *Request) { r.Look.TextFont = `Lexend\b1` },
		"scale":    func(r *Request) { r.Look.TextScale = 0 },
		"audio":    func(r *Request) { r.AudioLanguage = "Italian" },
		"subs":     func(r *Request) { r.Subtitles = "Off" },
	}
	for name, change := range bad {
		r := req
		r.Items = append([]Item(nil), req.Items...)
		change(&r)
		if _, err := m.Open(r); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// the file to play is gone (its disk unplugged)
	r := req
	r.Items = append([]Item(nil), req.Items...)
	r.Items[1].Path = filepath.Join(filepath.Dir(req.Items[1].Path), "gone.mkv")
	if _, err := m.Open(r); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing file: %v", err)
	}
	// a game is open: apps refuses, one at a time
	fa.launchErr = apps.ErrGameRunning
	if _, err := m.Open(req); !errors.Is(err, apps.ErrGameRunning) {
		t.Errorf("game running: %v", err)
	}
	if len(fa.launched) != 0 {
		t.Error("launched anyway")
	}
}

// A list of any length: a folder of 10,000 songs opens (sub-step 5 had a cap of 5,000)
func TestOpenLongList(t *testing.T) {
	m, fa, req := setup(t)
	first := req.Items[0]
	req.Kind, req.Index = "music", 0
	req.Items = []Item{first}
	for i := 1; i < 10000; i++ {
		req.Items = append(req.Items, Item{Path: filepath.Join(filepath.Dir(first.Path), "song.flac"), Title: "A song"})
	}
	st, err := m.Open(req)
	if err != nil || st.Count != 10000 || len(fa.launched) != 1 {
		t.Fatalf("10,000 songs: %+v %v", st, err)
	}
	list, _ := os.ReadFile(filepath.Join(m.RuntimeDir, "player.m3u"))
	if n := strings.Count(string(list), "\n"); n != 10001 {
		t.Fatalf("playlist lines %d", n)
	}
}

// Another video or song while the player is open: the player closes (its position saved by mpv),
// then starts on the new one; one that does not quit is killed
func TestOpenMovesThePlayer(t *testing.T) {
	m, fa, req := setup(t)
	if _, err := m.Open(req); err != nil {
		t.Fatal(err)
	}
	req.Index = 0
	if _, err := m.Open(req); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fa.closed, []string{AppID}) || len(fa.launched) != 2 {
		t.Fatalf("closed %v, launched %d", fa.closed, len(fa.launched))
	}
	fa.stubborn = true
	if _, err := m.Open(req); err != nil {
		t.Fatal(err)
	}
	if len(fa.launched) != 3 {
		t.Fatalf("launched %d", len(fa.launched))
	}
}

// The player's control socket: what plays comes as "player.changed"; the console over the player
// (or not) is told to the script; its end closes the state
func TestFollowsThePlayer(t *testing.T) {
	m, fa, req := setup(t)
	mpv := make(chan net.Conn, 1)
	m.Dial = func(string) (net.Conn, error) {
		a, b := net.Pipe()
		mpv <- b
		return a, nil
	}
	if _, err := m.Open(req); err != nil {
		t.Fatal(err)
	}
	evs, unsubscribe := m.Broker.Subscribe(16)
	defer unsubscribe()
	stop := make(chan struct{})
	defer close(stop)
	go m.Run(stop)

	var conn net.Conn
	select {
	case conn = <-mpv:
	case <-time.After(2 * time.Second):
		t.Fatal("no connection to the player")
	}
	lines := bufio.NewScanner(conn)
	focus := func() string {
		t.Helper()
		if !lines.Scan() {
			t.Fatal("nothing sent to the player")
		}
		var cmd struct{ Command []string }
		if err := json.Unmarshal(lines.Bytes(), &cmd); err != nil || len(cmd.Command) != 3 || cmd.Command[1] != "owneet-focus" {
			t.Fatalf("sent %s", lines.Bytes())
		}
		return cmd.Command[2]
	}
	if got := focus(); got != "no" { // the home screen is on screen: the buttons do nothing yet
		t.Fatalf("first focus %q", got)
	}
	changed := func() State {
		t.Helper()
		for {
			select {
			case ev := <-evs:
				if ev.Type == "player.changed" {
					return ev.Data.(State)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("no player.changed")
			}
		}
	}
	msg, _ := json.Marshal(map[string]any{"event": "client-message", "args": []string{"owneet-state",
		`{"index":0,"path":"` + req.Items[0].Path + `","position":95.5,"duration":2465,"paused":true,"autoplay":false,"error":"","audio_language":"it","subtitles":"off"}`}})
	go func() {
		conn.Write([]byte(`{"event":"file-loaded"}` + "\n"))
		conn.Write(append(msg, '\n'))
	}()
	st := changed()
	if st.Index != 0 || st.Title != "Episode 1" || st.Position != 95.5 || !st.Paused || st.Autoplay || !st.Open ||
		st.AudioLanguage != "it" || st.Subtitles != "off" {
		t.Fatalf("state %+v", st)
	}
	if got := m.Status(); got.Title != "Episode 1" || !got.Open {
		t.Fatalf("status %+v", got)
	}
	m.Broker.Publish("focus.changed", map[string]string{"focus": AppID})
	if got := focus(); got != "yes" {
		t.Fatalf("focus on the player: %q", got)
	}
	m.Broker.Publish("focus.changed", map[string]string{"focus": "home"})
	if got := focus(); got != "no" {
		t.Fatalf("focus on the console: %q", got)
	}
	fa.Kill(AppID)
	m.Broker.Publish("app.exited", map[string]string{"id": AppID})
	if st := changed(); st.Open || st.Title != "Episode 1" {
		t.Fatalf("after the end %+v", st)
	}
}
