// SPDX-License-Identifier: GPL-3.0-or-later

// Package player starts OwneetOS's player for the Videos and Music pages
// ([[step-5.6-local-videos-and-music]], [[local-media]]): mpv with the configuration of
// daemon/player (installed in /usr/share/owneet/player), as an app that runs one at a time like a
// game (apps.KindMedia: Guide and Resume as for games, one game, web app or player at a time).
//
// It writes the session the player's script draws from (the texts, colours, fonts and files the
// interface sends), follows what plays through mpv's control socket (the script's "owneet-state"
// messages, sent on as "player.changed"), and tells the script when the console is on screen over
// the player ("owneet-focus"): mpv reads the controller itself, through SDL, also while its window
// is not shown, so its buttons must do nothing then.
package player

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
	"github.com/fancy-dino/OwneetOS/daemon/internal/atomicfile"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// AppID is the player's id among the running apps.
const AppID = "player"

// Item is a video or a song of the list, with what the display shows.
type Item struct {
	Path   string  `json:"path"`
	Title  string  `json:"title"`
	Folder string  `json:"folder,omitempty"` // a video's folders, "Harbour Tales › Season 1"
	Artist string  `json:"artist,omitempty"`
	Album  string  `json:"album,omitempty"`
	Hue    float64 `json:"hue"` // a song's drawn art (no picture), as the console's
}

// Look is the console's palette, fonts, text size and controller glyphs.
type Look struct {
	Bg          string  `json:"bg"`
	Surface     string  `json:"surface"`
	Raised      string  `json:"raised"`
	Line        string  `json:"line"`
	Fg          string  `json:"fg"`
	Muted       string  `json:"muted"`
	Accent      string  `json:"accent"`
	OnAccent    string  `json:"onAccent"`
	DisplayFont string  `json:"displayFont"`
	TextFont    string  `json:"textFont"`
	TextScale   float64 `json:"textScale"`
	Glyphs      string  `json:"glyphs"` // xbox, ps, nintendo
}

// Request is the body of POST /v1/player.
type Request struct {
	Kind      string `json:"kind"` // "videos" or "music"
	Items     []Item `json:"items"`
	Index     int    `json:"index"`      // the one to play first
	FromStart bool   `json:"from_start"` // a video's saved position is forgotten first
	Autoplay  bool   `json:"autoplay"`
	// AudioLanguage and Subtitles are the console's preferred languages for videos (two- or
	// three-letter codes; Subtitles also "off"; "": none chosen yet), [[local-media]]
	AudioLanguage string `json:"audio_language"`
	Subtitles     string `json:"subtitles"`
	// Texts are the player's messages, already translated (placeholders such as {t} left in)
	Texts map[string]string `json:"texts"`
	// Languages names tracks by their two-letter codes; Codes turns three-letter ones into them
	Languages map[string]string `json:"languages"`
	Codes     map[string]string `json:"codes"`
	Look      Look              `json:"look"`
}

// session is the file owneet.lua reads.
type session struct {
	Kind          string            `json:"kind"`
	Mode          string            `json:"mode"` // gamescope or cage (Guide's prompt)
	Autoplay      bool              `json:"autoplay"`
	AudioLanguage string            `json:"audio_language"`
	Subtitles     string            `json:"subtitles"`
	Items         []Item            `json:"items"`
	Texts         map[string]string `json:"texts"`
	Languages     map[string]string `json:"languages"`
	Codes         map[string]string `json:"codes"`
	Look          Look              `json:"look"`
}

// State is the body of GET /v1/player and of the "player.changed" event: what plays.
type State struct {
	Open     bool    `json:"open"`
	Kind     string  `json:"kind,omitempty"`
	Index    int     `json:"index"`
	Count    int     `json:"count"`
	Path     string  `json:"path,omitempty"`
	Title    string  `json:"title,omitempty"`
	Folder   string  `json:"folder,omitempty"`
	Artist   string  `json:"artist,omitempty"`
	Album    string  `json:"album,omitempty"`
	Position float64 `json:"position"`
	Duration float64 `json:"duration"`
	Paused   bool    `json:"paused"`
	Autoplay bool    `json:"autoplay"`
	// The preferred languages, as the player last chose them (X, Menu)
	AudioLanguage string `json:"audio_language"`
	Subtitles     string `json:"subtitles"`
	// Error is a file that could not be played (the last one: the player closed)
	Error string `json:"error,omitempty"`
}

// Errors returned to the API (apps.ErrGameRunning and apps.ErrNoSession come through too).
var (
	ErrInvalid  = errors.New("invalid player request")
	ErrNotFound = errors.New("the file to play is not there")
)

// Apps is implemented by *apps.Manager.
type Apps interface {
	Status() apps.Status
	Launch(req apps.LaunchRequest) (apps.App, error)
	Close(id string) error
	Kill(id string) error
}

// Manager starts the player and follows it.
type Manager struct {
	Apps    Apps
	Broker  *events.Broker
	Log     *slog.Logger
	Session func() (apps.Session, error)
	// ConfigDir is mpv's configuration (mpv.conf, input.conf, owneet.lua)
	ConfigDir string
	// RuntimeDir keeps the session file and mpv's control socket ($XDG_RUNTIME_DIR/owneet)
	RuntimeDir string
	// WatchLaterDir keeps the videos' saved positions (~/.local/state/owneet/watch_later); the
	// interface reads them too
	WatchLaterDir string
	// Dial connects to mpv's control socket (a Unix socket by default)
	Dial func(path string) (net.Conn, error)
	// CloseWait is how long a player has to quit before another starts (10 s)
	CloseWait time.Duration

	mu    sync.Mutex
	state State
	items []Item
	conn  net.Conn
}

var (
	colourRE   = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	languageRE = regexp.MustCompile(`^([a-z]{2,3})?$`)
)

func validate(req *Request) error {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%w: "+format, append([]any{ErrInvalid}, a...)...)
	}
	if req.Kind != "videos" && req.Kind != "music" {
		return bad(`kind is "videos" or "music"`)
	}
	if len(req.Items) == 0 {
		return bad("no items")
	}
	if req.Index < 0 || req.Index >= len(req.Items) {
		return bad("index out of the list")
	}
	for _, it := range req.Items {
		if !filepath.IsAbs(it.Path) || filepath.Clean(it.Path) != it.Path || strings.ContainsAny(it.Path, "\n\x00") {
			return bad("every path is a clean absolute path")
		}
		if len(it.Title) > 500 || len(it.Folder) > 2000 || len(it.Artist) > 500 || len(it.Album) > 500 {
			return bad("text too long")
		}
	}
	if !languageRE.MatchString(req.AudioLanguage) || (req.Subtitles != "off" && !languageRE.MatchString(req.Subtitles)) {
		return bad(`languages are two or three letters (subtitles also "off")`)
	}
	if len(req.Texts) > 200 || len(req.Languages) > 500 || len(req.Codes) > 500 {
		return bad("too many texts")
	}
	for _, m := range []map[string]string{req.Texts, req.Languages, req.Codes} {
		for k, v := range m {
			if len(k) > 100 || len(v) > 500 {
				return bad("text too long")
			}
		}
	}
	l := req.Look
	for _, c := range []string{l.Bg, l.Surface, l.Raised, l.Line, l.Fg, l.Muted, l.Accent, l.OnAccent} {
		if !colourRE.MatchString(c) {
			return bad("colours are #RRGGBB")
		}
	}
	if l.Glyphs != "xbox" && l.Glyphs != "ps" && l.Glyphs != "nintendo" {
		return bad(`glyphs are "xbox", "ps" or "nintendo"`)
	}
	if l.TextScale < 0.5 || l.TextScale > 2 {
		return bad("text scale 0.5 to 2")
	}
	for _, f := range []string{l.DisplayFont, l.TextFont} {
		if f == "" || len(f) > 100 || strings.ContainsAny(f, `\{}`) {
			return bad("font names")
		}
	}
	return nil
}

// WatchLaterFile is mpv's file of a video's saved position: the MD5 of its path, in upper case.
func WatchLaterFile(dir, path string) string {
	sum := md5.Sum([]byte(path))
	return filepath.Join(dir, strings.ToUpper(hex.EncodeToString(sum[:])))
}

// Args is the mpv command: the player's configuration, the session's output (as
// owneet-placeholder: only the methods that fit the compositor; the last works without a GPU), the
// control socket, the session file, the saved positions, songs' profile, the list (a playlist
// file: a list of any length, never on the command line). Keep tools/preview-steps/player-driver.py
// alike.
func Args(configDir, runtimeDir, watchLaterDir, mode, kind string, index int) []string {
	argv := []string{"mpv", "--config-dir=" + configDir,
		"--input-ipc-server=" + filepath.Join(runtimeDir, "player.sock"),
		"--script-opts=owneet-session=" + filepath.Join(runtimeDir, "player.json"),
		"--watch-later-dir=" + watchLaterDir}
	if kind == "music" {
		argv = append(argv, "--profile=owneet-music")
	}
	if mode == "cage" {
		argv = append(argv, "--vo=gpu-next,gpu,wlshm", "--gpu-context=wayland,waylandvk")
	} else {
		argv = append(argv, "--vo=gpu-next,gpu,x11", "--gpu-context=x11egl,x11vk")
	}
	return append(argv, "--playlist="+filepath.Join(runtimeDir, "player.m3u"), "--playlist-start="+strconv.Itoa(index))
}

// playlist is the list for mpv: one absolute path a line (validate refuses line breaks in them).
func playlist(items []Item) []byte {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for _, it := range items {
		b.WriteString(it.Path)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func (m *Manager) running() bool {
	for _, a := range m.Apps.Status().Apps {
		if a.ID == AppID {
			return true
		}
	}
	return false
}

// closeAndWait closes the running player (it saves the video's position as it quits) and waits
// for it to end; a player that does not quit is killed.
func (m *Manager) closeAndWait() error {
	wait := m.CloseWait
	if wait == 0 {
		wait = 10 * time.Second
	}
	if err := m.Apps.Close(AppID); err != nil && !errors.Is(err, apps.ErrUnknownApp) {
		return err
	}
	if m.gone(wait) {
		return nil
	}
	m.Log.Warn("the player did not quit: killed")
	_ = m.Apps.Kill(AppID)
	if m.gone(2 * time.Second) {
		return nil
	}
	return errors.New("the player did not quit")
}

// gone waits until the player is no longer running, at most d.
func (m *Manager) gone(d time.Duration) bool {
	deadline := time.Now().Add(d)
	for m.running() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
	return true
}

// Open starts the player on a list, at one of its items. A player already open closes first
// (another video or song moves the player to it); a game or web app open: apps.ErrGameRunning.
func (m *Manager) Open(req Request) (State, error) {
	if err := validate(&req); err != nil {
		return State{}, err
	}
	first := req.Items[req.Index]
	if st, err := os.Stat(first.Path); err != nil || !st.Mode().IsRegular() {
		return State{}, fmt.Errorf("%w: %s", ErrNotFound, first.Path)
	}
	sess, err := m.Session()
	if err != nil {
		return State{}, apps.ErrNoSession
	}
	if m.running() {
		if err := m.closeAndWait(); err != nil {
			return State{}, err
		}
	}
	if req.FromStart {
		if err := os.Remove(WatchLaterFile(m.WatchLaterDir, first.Path)); err != nil && !os.IsNotExist(err) {
			m.Log.Warn("cannot forget the saved position", "err", err)
		}
	}
	if err := os.MkdirAll(m.RuntimeDir, 0o700); err != nil {
		return State{}, err
	}
	if err := os.MkdirAll(m.WatchLaterDir, 0o700); err != nil {
		return State{}, err
	}
	data, err := json.Marshal(session{Kind: req.Kind, Mode: sess.Mode, Autoplay: req.Autoplay,
		AudioLanguage: req.AudioLanguage, Subtitles: req.Subtitles, Items: req.Items,
		Texts: req.Texts, Languages: req.Languages, Codes: req.Codes, Look: req.Look})
	if err != nil {
		return State{}, err
	}
	if err := atomicfile.Write(filepath.Join(m.RuntimeDir, "player.json"), data, 0o600); err != nil {
		return State{}, err
	}
	if err := atomicfile.Write(filepath.Join(m.RuntimeDir, "player.m3u"), playlist(req.Items), 0o600); err != nil {
		return State{}, err
	}
	m.disconnect()
	argv := Args(m.ConfigDir, m.RuntimeDir, m.WatchLaterDir, sess.Mode, req.Kind, req.Index)
	if _, err := m.Apps.Launch(apps.LaunchRequest{ID: AppID, Name: first.Title, Kind: apps.KindMedia, Command: argv}); err != nil {
		return State{}, err
	}
	m.mu.Lock()
	m.items = req.Items
	m.state = State{Open: true, Kind: req.Kind, Count: len(req.Items), Autoplay: req.Autoplay,
		AudioLanguage: req.AudioLanguage, Subtitles: req.Subtitles}
	m.setIndex(req.Index, first.Path)
	st := m.state
	m.mu.Unlock()
	m.Log.Info("player started", "kind", req.Kind, "items", len(req.Items), "index", req.Index)
	m.Broker.Publish("player.changed", st)
	return st, nil
}

// setIndex: what the display shows of the item; callers hold m.mu.
func (m *Manager) setIndex(i int, path string) {
	m.state.Index, m.state.Path = i, path
	m.state.Title, m.state.Folder, m.state.Artist, m.state.Album = "", "", "", ""
	if i >= 0 && i < len(m.items) {
		it := m.items[i]
		m.state.Title, m.state.Folder, m.state.Artist, m.state.Album = it.Title, it.Folder, it.Artist, it.Album
	}
}

// Status is what plays (Open false when the player is not running).
func (m *Manager) Status() State {
	open := m.running()
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.state
	st.Open = open
	if !open {
		st.Paused = false
	}
	return st
}

// Run follows the player until stop is closed: its control socket while it runs (connected
// within half a second of its start, also after owneetd restarts), what is on screen, its end.
func (m *Manager) Run(stop <-chan struct{}) {
	evs, unsubscribe := m.Broker.Subscribe(32)
	defer unsubscribe()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			m.disconnect()
			return
		case ev, ok := <-evs:
			if !ok {
				return
			}
			data, _ := ev.Data.(map[string]string)
			switch ev.Type {
			case "focus.changed":
				m.sendFocus(data["focus"])
			case "app.exited":
				if data["id"] == AppID {
					m.ended()
				}
			}
		case <-tick.C:
			m.connect()
		}
	}
}

// ended: the player quit (B, a video's end with Autoplay off, the list's end, Close, Guide held
// in cage): its last state stays, closed.
func (m *Manager) ended() {
	m.disconnect()
	m.mu.Lock()
	m.state.Open, m.state.Paused = false, false
	st := m.state
	m.mu.Unlock()
	m.Broker.Publish("player.changed", st)
}

func (m *Manager) dial(path string) (net.Conn, error) {
	if m.Dial != nil {
		return m.Dial(path)
	}
	return net.DialTimeout("unix", path, time.Second)
}

func (m *Manager) connect() {
	m.mu.Lock()
	connected := m.conn != nil
	m.mu.Unlock()
	if connected || !m.running() {
		return
	}
	conn, err := m.dial(filepath.Join(m.RuntimeDir, "player.sock"))
	if err != nil {
		return // not ready yet: the next tick tries again
	}
	m.mu.Lock()
	if m.conn != nil {
		m.mu.Unlock()
		conn.Close()
		return
	}
	m.conn = conn
	if len(m.items) == 0 {
		m.readSession() // owneetd restarted while the player plays
	}
	m.mu.Unlock()
	// the script starts as if on screen: the console may be over it already
	m.sendFocus(m.Apps.Status().Focus)
	go m.follow(conn)
}

// readSession reads the list back from the session file; callers hold m.mu.
func (m *Manager) readSession() {
	data, err := os.ReadFile(filepath.Join(m.RuntimeDir, "player.json"))
	if err != nil {
		return
	}
	var s session
	if json.Unmarshal(data, &s) == nil {
		m.items = s.Items
		m.state.Kind, m.state.Count, m.state.Autoplay = s.Kind, len(s.Items), s.Autoplay
		m.state.AudioLanguage, m.state.Subtitles = s.AudioLanguage, s.Subtitles
	}
}

func (m *Manager) disconnect() {
	m.mu.Lock()
	conn := m.conn
	m.conn = nil
	m.mu.Unlock()
	if conn != nil {
		conn.Close()
	}
}

// ipcEvent is a line from mpv's control socket; the script's messages are "client-message".
type ipcEvent struct {
	Event string   `json:"event"`
	Args  []string `json:"args"`
}

// scriptState is the script's "owneet-state" message.
type scriptState struct {
	Index    int     `json:"index"`
	Path     string  `json:"path"`
	Position float64 `json:"position"`
	Duration float64 `json:"duration"`
	Paused   bool    `json:"paused"`
	Autoplay bool    `json:"autoplay"`
	Error    string  `json:"error"`
	Audio    string  `json:"audio_language"`
	Subs     string  `json:"subtitles"`
}

func (m *Manager) follow(conn net.Conn) {
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var ev ipcEvent
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Event != "client-message" || len(ev.Args) != 2 || ev.Args[0] != "owneet-state" {
			continue
		}
		var s scriptState
		if json.Unmarshal([]byte(ev.Args[1]), &s) != nil {
			continue
		}
		m.mu.Lock()
		if m.conn != conn {
			m.mu.Unlock()
			return
		}
		m.setIndex(s.Index, s.Path)
		m.state.Position, m.state.Duration, m.state.Paused = s.Position, s.Duration, s.Paused
		m.state.Autoplay, m.state.Error = s.Autoplay, s.Error
		m.state.AudioLanguage, m.state.Subtitles = s.Audio, s.Subs
		m.state.Open = true
		st := m.state
		m.mu.Unlock()
		m.Broker.Publish("player.changed", st)
	}
	m.mu.Lock()
	if m.conn == conn {
		m.conn = nil
	}
	m.mu.Unlock()
	conn.Close()
}

// sendFocus tells the script whether it is on screen: "yes" when the player is what the screen
// shows (or nothing is known), "no" when the console is over it.
func (m *Manager) sendFocus(focus string) {
	m.mu.Lock()
	conn := m.conn
	m.mu.Unlock()
	if conn == nil {
		return
	}
	on := "no"
	if focus == AppID || focus == "" {
		on = "yes"
	}
	line, _ := json.Marshal(map[string][]string{"command": {"script-message", "owneet-focus", on}})
	_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
	if _, err := conn.Write(append(line, '\n')); err != nil {
		m.Log.Debug("cannot reach the player", "err", err)
	}
}
