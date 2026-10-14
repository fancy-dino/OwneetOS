// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Steam ([[step-4.2-steam-integration]]) runs as the app "steam". It is started without
// GAMESCOPE_WAYLAND_DISPLAY: with it, Steam takes over gamescope's control of what is on screen and
// its own window stays hidden ([[owneetd-design]]). A game started by Steam runs inside Steam's
// unit, under Steam's launcher ("reaper SteamLaunch AppId=N -- …"): owneetd lists it as the game
// "steam:N", tags its windows with an app id of its own and closes it without closing Steam.

// SteamID is the id of the Steam app.
const SteamID = "steam"

const steamGamePrefix = "steam:"

// steamUIAppID is the code Steam uses for its own window when it writes what gamescope shows.
const steamUIAppID = 769

// SteamLiveMinRAM is the memory Steam needs in the live system, where its first download (about
// 2.4 GB) is kept in RAM: 12 GB of RAM, as the kernel reports it ([[steam-integration]],
// 2026-10-11).
const SteamLiveMinRAM = 11 << 30

// ErrSteamNeedsInstall: the live system has too little memory for Steam's files.
var ErrSteamNeedsInstall = errors.New("Steam needs the installed system: the live system has too little memory for its files")

// SteamRequest is the body of POST /v1/steam: open a page of Steam, run a game, or ask Steam to
// install or uninstall one (Steam shows its confirmation; [[step-4.2c-steam-without-its-interface]]).
type SteamRequest struct {
	Open      string `json:"open,omitempty"`      // "library" or "store"
	Run       string `json:"run,omitempty"`       // a Steam app id
	Install   string `json:"install,omitempty"`   // a Steam app id
	Uninstall string `json:"uninstall,omitempty"` // a Steam app id (also cancels its download)
	// Ask: the next start of this Steam app id asks the user first (a licence agreement to
	// accept): Steam's window is shown at once for it, not kept hidden
	Ask  string `json:"ask,omitempty"`
	Name string `json:"name,omitempty"` // the game's name (shown while it runs)
	// The console's language tag ("it", "pt-BR"…): Steam starts in the same language
	Language string `json:"language,omitempty"`
}

// steamLanguages are Steam's names for the languages it offers, by language tag.
var steamLanguages = map[string]string{
	"ar": "arabic", "bg": "bulgarian", "cs": "czech", "da": "danish", "de": "german",
	"el": "greek", "en": "english", "es": "spanish", "es-419": "latam", "fi": "finnish",
	"fr": "french", "hu": "hungarian", "id": "indonesian", "it": "italian", "ja": "japanese",
	"ko": "koreana", "nb": "norwegian", "nl": "dutch", "no": "norwegian", "pl": "polish",
	"pt": "portuguese", "pt-BR": "brazilian", "ro": "romanian", "ru": "russian",
	"sv": "swedish", "th": "thai", "tr": "turkish", "uk": "ukrainian", "vi": "vietnamese",
	"zh-CN": "schinese", "zh-Hans": "schinese", "zh-TW": "tchinese", "zh-Hant": "tchinese",
}

// steamLanguage is Steam's name for a language tag ("pt-BR" → brazilian, "it-CH" → italian),
// "" when Steam does not offer it (Steam then keeps its own choice).
func steamLanguage(tag string) string {
	if name, ok := steamLanguages[tag]; ok {
		return name
	}
	base, _, _ := strings.Cut(tag, "-")
	return steamLanguages[base]
}

// SteamStatus is the body of GET /v1/steam.
type SteamStatus struct {
	Installed bool `json:"installed"` // the steam program is there
	Allowed   bool `json:"allowed"`   // false in a live system with too little memory
	Prepared  bool `json:"prepared"`  // Steam has started once (no long first download)
	Running   bool `json:"running"`
	// Games are the Steam app ids installed completely, on every library folder present
	Games []string `json:"games"`
	// Downloads: the games Steam is installing or updating, the one it works on first (4.2c)
	Downloads []SteamDownload `json:"downloads"`
	// Away: games installed on a disk that is not connected, with the disk's name ("" unknown)
	Away map[string]string `json:"away"`
	// Account: someone has signed in to Steam on this computer (else Steam asks first, online)
	Account bool `json:"account"`
	// Licences: when Steam last wrote the account's licences (Unix time): a game bought since
	// the interface read its games makes it read them again
	Licences int64 `json:"licences"`
	// NVIDIA: Steam's window may show display errors (the interface says so before showing it)
	NVIDIA bool `json:"nvidia"`
}

// Procs reads and signals processes (/proc by default).
type Procs interface {
	InUnit(unit string) []int
	Cmdline(pid int) []string
	Parent(pid int) int
	Signal(pid int, sig syscall.Signal) error
}

type procFS struct{}

func (procFS) InUnit(unit string) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var pids []int
	for _, e := range entries {
		if pid, err := strconv.Atoi(e.Name()); err == nil && UnitOfPID(pid) == unit {
			pids = append(pids, pid)
		}
	}
	return pids
}

func (procFS) Cmdline(pid int) []string {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil || len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
}

func (procFS) Parent(pid int) int {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0
	}
	// "PID (name) STATE PPID …": the name may contain spaces and parentheses
	s := string(data)
	fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
	if len(fields) < 2 {
		return 0
	}
	ppid, _ := strconv.Atoi(fields[1])
	return ppid
}

func (procFS) Signal(pid int, sig syscall.Signal) error { return syscall.Kill(pid, sig) }

// steamAllowed: always in the installed system; in the live system (/run/archiso) with enough RAM.
func steamAllowed() bool {
	if _, err := os.Stat("/run/archiso"); err != nil {
		return true
	}
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "MemTotal:"); ok {
			kb, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(rest), " kB"), 10, 64)
			return kb*1024 >= SteamLiveMinRAM
		}
	}
	return false
}

// forward hands a steam:// address to the Steam already running (the command returns at once).
func forward(argv, env []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), env...)
	return cmd.Run()
}

// nvidiaDriver tells whether NVIDIA's driver is loaded (owneet-gpu-select chose it at boot).
func nvidiaDriver() bool {
	_, err := os.Stat("/sys/module/nvidia_drm")
	return err == nil
}

// steamGPUArgs: with NVIDIA's driver, Steam's pages drawn with the GPU (its web views) show
// garbled, stale pictures inside gamescope (FrankenPC, GTX 1660 Super); Steam draws them
// without the GPU instead, as when "GPU accelerated rendering in web views" is turned off.
// Games keep the GPU.
func (m *Manager) steamGPUArgs() []string {
	if m.NVIDIA() {
		return []string{"-cef-disable-gpu"}
	}
	return nil
}

// steamStop closes Steam cleanly: Steam killed with its processes (e.g. at shutdown) checks
// every one of its files at the next start (7 s on the FrankenPC). Steam's own remote control
// asks the running Steam to exit (it never starts Steam, unlike the steam command), then the stop
// waits for Steam's main process; the rest of the unit is stopped afterwards.
func (m *Manager) steamStop() []string {
	remote := filepath.Join(m.SteamDir, "ubuntu12_32", "steam-runtime", "amd64", "usr", "bin", "steam-runtime-steam-remote")
	return []string{"/bin/sh", "-c",
		`"$0" -shutdown >/dev/null 2>&1 || exit 0; while kill -0 "$MAINPID" 2>/dev/null; do sleep 0.5; done`,
		remote}
}

// forwardSteam hands a steam:// address to the Steam already running.
func (m *Manager) forwardSteam(url string, sess Session) error {
	argv, err := m.steamArgv(url)
	if err != nil {
		return err
	}
	if err := m.Forward(argv, sess.Env); err != nil {
		m.Log.Warn("cannot reach Steam", "url", url, "err", err)
		return fmt.Errorf("cannot reach Steam: %w", err)
	}
	return nil
}

// steamArgv starts the steam program without gamescope's control socket (see above).
func (m *Manager) steamArgv(args ...string) ([]string, error) {
	path, err := m.LookPath("steam")
	if err != nil {
		return nil, fmt.Errorf("%w: steam", ErrNotFound)
	}
	return append([]string{"env", "-u", "GAMESCOPE_WAYLAND_DISPLAY", path}, args...), nil
}

var appIDPattern = regexp.MustCompile(`^[0-9]{1,10}$`)

// Steam opens Steam's library or store, runs a Steam game, or asks Steam to install or
// uninstall one; Steam starts if needed. Running a game keeps Steam's window hidden
// ([[step-4.2c-steam-without-its-interface]]): it is shown only if Steam seems to need the user
// (no game after steamAskAfter, or nobody has signed in yet). Its pages and confirmations are
// shown; after a confirmation, the home screen comes back once Steam has acted on it.
func (m *Manager) Steam(req SteamRequest) error {
	var url string
	given := 0
	for _, f := range []string{req.Open, req.Run, req.Install, req.Uninstall, req.Ask} {
		if f != "" {
			given++
		}
	}
	switch {
	case given != 1:
		url = ""
	case appIDPattern.MatchString(req.Ask):
		// the next start of this game asks the user first (a licence agreement): Steam is
		// shown at once then, not after steamAskAfter
		m.mu.Lock()
		m.steamAskFor = req.Ask
		m.mu.Unlock()
		return nil
	case appIDPattern.MatchString(req.Run):
		url = "steam://rungameid/" + req.Run
	case appIDPattern.MatchString(req.Install):
		url = "steam://install/" + req.Install
	case appIDPattern.MatchString(req.Uninstall):
		url = "steam://uninstall/" + req.Uninstall
	case req.Open == "library":
		url = "steam://open/games"
	case req.Open == "store":
		url = "steam://store"
	}
	if url == "" {
		return fmt.Errorf("%w: one of open (library, store), run, install, uninstall, ask (a Steam app id)", ErrInvalid)
	}
	m.mu.Lock()
	m.init()
	allowed := m.SteamAllowed
	m.mu.Unlock()
	if !allowed() {
		return ErrSteamNeedsInstall
	}

	m.mu.Lock()
	if req.Run != "" {
		for _, a := range m.apps {
			if a.Kind == KindGame {
				m.mu.Unlock()
				return ErrGameRunning
			}
		}
		if req.Name != "" {
			m.steamNames[req.Run] = req.Name
		}
	}
	_, running := m.apps[SteamID]
	dir := m.SteamDir
	m.mu.Unlock()

	sess, err := m.Session()
	if err != nil {
		return ErrNoSession
	}
	// Steam's window stays hidden while it starts a game, unless nobody has signed in yet or the
	// game asks the user first (ask)
	m.mu.Lock()
	asks := req.Run != "" && req.Run == m.steamAskFor
	if req.Run != "" {
		m.steamAskFor = ""
	}
	m.mu.Unlock()
	hidden := req.Run != "" && steamAccount(dir) && !asks
	m.mu.Lock()
	m.steamHidden = hidden
	m.steamConfirm = nil
	if id := req.Install + req.Uninstall; id != "" {
		flags, has := steamManifest(dir, id)
		m.steamConfirm = &steamConfirm{appid: id, flags: flags, has: has, at: time.Now()}
	}
	m.mu.Unlock()
	if !running {
		args := append([]string{"-gamepadui"}, m.steamGPUArgs()...)
		if lang := steamLanguage(req.Language); lang != "" {
			args = append(args, "-language", lang)
		}
		// Steam started on an address keeps it as its first page: Back from there leaves Steam
		// loading for ever (FrankenPC). In gamescope Steam starts on its own home page and gets
		// the address once its window is shown (checkWindows); cage shows no windows to owneetd.
		gamescope := m.focus(sess) != nil
		if !gamescope {
			args = append(args, url)
		}
		argv, err := m.steamArgv(args...)
		if err != nil {
			return err
		}
		if gamescope {
			m.mu.Lock()
			m.steamPending = url
			m.mu.Unlock()
		}
		_, err = m.Launch(LaunchRequest{ID: SteamID, Name: "Steam", Kind: KindApp, Command: argv, Stop: m.steamStop()})
		return err
	}
	if err := m.forwardSteam(url, sess); err != nil {
		return err
	}
	if hidden {
		m.steamAskLater()
		return nil
	}
	// Steam's window comes to the front (a game's window takes over once it appears)
	if err := m.FocusApp(SteamID); errors.Is(err, ErrNoWindow) {
		if f := m.focus(sess); f != nil {
			m.show(f, []uint32{m.appIDOf(SteamID), HomeAppID})
		}
	}
	return nil
}

func (m *Manager) appIDOf(id string) uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.apps[id]; ok {
		return a.appID
	}
	return 0
}

// SteamStatus tells the interface what opening Steam will do.
func (m *Manager) SteamStatus() SteamStatus {
	m.mu.Lock()
	m.init()
	_, running := m.apps[SteamID]
	allowed, dir := m.SteamAllowed, m.SteamDir
	m.mu.Unlock()
	_, err := m.LookPath("steam")
	_, prepared := os.Stat(filepath.Join(dir, "ubuntu12_32", "steam"))
	m.steamMu.Lock()
	if m.steamSamples == nil {
		m.steamSamples = map[string]*steamSample{}
	}
	downloads := steamDownloads(dir, m.steamSamples, time.Now())
	m.steamMu.Unlock()
	return SteamStatus{Installed: err == nil, Allowed: allowed(), Prepared: prepared == nil, Running: running,
		Games: installedSteamGames(dir), Downloads: downloads, Away: steamAway(dir), Account: steamAccount(dir),
		Licences: steamLicences(dir), NVIDIA: m.NVIDIA()}
}

// steamAskAfter: a game asked of a hidden Steam that has not started by then probably waits for
// the user (a licence to accept, a cloud conflict, launch options): Steam is shown. Not while
// Steam prepares the game's shaders (fossilize_replay, up to a minute or more the first time:
// FrankenPC, 2026-10-14): the wait starts again when that ends.
const steamAskAfter = 20 * time.Second

// steamPreparingShaders: Steam is preparing a game's shaders (its fossilize_replay processes).
func (m *Manager) steamPreparingShaders(unit string) bool {
	for _, pid := range m.Procs.InUnit(unit) {
		if argv := m.Procs.Cmdline(pid); len(argv) > 0 && strings.HasPrefix(filepath.Base(argv[0]), "fossilize_replay") {
			return true
		}
	}
	return false
}

// steamConfirm is an installation or uninstallation Steam asks the user to confirm.
type steamConfirm struct {
	appid string
	flags int64 // the app's StateFlags before
	has   bool  // it had an appmanifest before
	at    time.Time
}

// steamConfirmWait: after this long without Steam acting, the confirmation is forgotten (cancelled).
const steamConfirmWait = 10 * time.Minute

func (m *Manager) steamAskLater() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.steamAsk != nil {
		m.steamAsk.Stop()
	}
	m.steamAsk = time.AfterFunc(steamAskAfter, m.steamAskNow)
}

func (m *Manager) steamAskNow() {
	m.mu.Lock()
	m.steamAsk = nil
	m.steamHidden = false
	game := false
	for _, a := range m.apps {
		game = game || a.reaper != 0
	}
	steam, running := m.apps[SteamID]
	m.mu.Unlock()
	if game || !running {
		return
	}
	if m.steamPreparingShaders(steam.unit) {
		m.Log.Info("Steam is preparing the game's shaders: still hidden")
		m.mu.Lock()
		m.steamHidden = true
		m.mu.Unlock()
		m.steamAskLater()
		return
	}
	m.Log.Info("no Steam game yet: Steam may be waiting for the user, shown")
	if err := m.FocusApp(SteamID); err != nil {
		m.Log.Debug("cannot show Steam", "err", err)
	}
}

// watchSteam (every second, while Steam runs, and once after it stopped): sends "steam.changed" with the status
// when Steam's games or downloads change, and brings the home screen back once Steam has acted on
// a confirmation it showed.
func (m *Manager) watchSteam() {
	m.mu.Lock()
	m.init()
	_, running := m.apps[SteamID]
	confirm, dir := m.steamConfirm, m.SteamDir
	m.mu.Unlock()
	m.steamMu.Lock()
	was := m.steamRunning // once more after Steam stopped: its downloads stopped too
	m.steamRunning = running
	m.steamMu.Unlock()
	if !running && !was && confirm == nil {
		return
	}
	st := m.SteamStatus()
	key, _ := json.Marshal([]any{st.Games, st.Downloads, st.Away, st.Running})
	m.steamMu.Lock()
	changed := string(key) != m.steamLast
	m.steamLast = string(key)
	m.steamMu.Unlock()
	if changed {
		m.Broker.Publish("steam.changed", st)
	}
	if confirm == nil {
		return
	}
	flags, has := steamManifest(dir, confirm.appid)
	acted := has != confirm.has || flags != confirm.flags
	if !acted && time.Since(confirm.at) < steamConfirmWait {
		return
	}
	m.mu.Lock()
	if m.steamConfirm == confirm {
		m.steamConfirm = nil
	}
	m.mu.Unlock()
	if acted {
		m.Log.Info("Steam acted on its confirmation: home screen", "appid", confirm.appid)
		m.backHome()
	}
}

// backHome shows the home screen if Steam is on screen (its window is not used any more).
func (m *Manager) backHome() {
	sess, err := m.Session()
	if err != nil || !m.steamOnScreen(sess) {
		return
	}
	f := m.focus(sess)
	if f == nil {
		return
	}
	m.mu.Lock()
	m.lastApp = ""
	m.mu.Unlock()
	if err := m.show(f, []uint32{HomeAppID}); err == nil {
		m.focusChanged("home")
	}
}

var manifestFlags = regexp.MustCompile(`(?m)^\s*"StateFlags"\s+"([0-9]+)"`)

// steamLibraries: Steam's folder and the library folders it lists.
func steamLibraries(dir string) []string {
	libraries := []string{dir}
	if data, err := os.ReadFile(filepath.Join(dir, "steamapps", "libraryfolders.vdf")); err == nil {
		for _, match := range libraryPath.FindAllStringSubmatch(string(data), -1) {
			if match[1] != dir {
				libraries = append(libraries, match[1])
			}
		}
	}
	return libraries
}

// installedSteamGames lists the app ids installed completely (StateFlags bit 4), sorted.
func installedSteamGames(dir string) []string {
	ids := []string{}
	seen := map[string]bool{}
	for _, lib := range steamLibraries(dir) {
		files, _ := filepath.Glob(filepath.Join(lib, "steamapps", "appmanifest_*.acf"))
		for _, f := range files {
			id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "appmanifest_"), ".acf")
			data, err := os.ReadFile(f)
			match := manifestFlags.FindSubmatch(data)
			if err != nil || match == nil || seen[id] {
				continue
			}
			if flags, _ := strconv.Atoi(string(match[1])); flags&4 != 0 {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	sort.Strings(ids)
	return ids
}

// reaperAppID returns the Steam app id of Steam's launcher process ("reaper SteamLaunch AppId=N").
func reaperAppID(argv []string) string {
	if len(argv) < 3 || filepath.Base(argv[0]) != "reaper" {
		return ""
	}
	launch := false
	for _, a := range argv[1:] {
		if a == "--" {
			break
		}
		if a == "SteamLaunch" {
			launch = true
		}
		if id, ok := strings.CutPrefix(a, "AppId="); ok && launch && appIDPattern.MatchString(id) {
			return id
		}
	}
	return ""
}

// reaperOf returns the Steam game a process belongs to: its app id and launcher, if any.
func (m *Manager) reaperOf(pid int) (string, int) {
	for i := 0; i < 64 && pid > 1; i++ {
		if id := reaperAppID(m.Procs.Cmdline(pid)); id != "" {
			return id, pid
		}
		pid = m.Procs.Parent(pid)
	}
	return "", 0
}

func (m *Manager) descends(pid, ancestor int) bool {
	for i := 0; i < 64 && pid > 1; i++ {
		pid = m.Procs.Parent(pid)
		if pid == ancestor {
			return true
		}
	}
	return false
}

var manifestName = regexp.MustCompile(`(?m)^\s*"name"\s+"([^"]+)"`)
var libraryPath = regexp.MustCompile(`(?m)^\s*"path"\s+"([^"]+)"`)

// steamGameName: the name the interface gave, else the game's manifest, else "Steam".
func (m *Manager) steamGameName(appid string) string {
	m.mu.Lock()
	name, dir := m.steamNames[appid], m.SteamDir
	m.mu.Unlock()
	if name != "" {
		return name
	}
	for _, lib := range steamLibraries(dir) {
		data, err := os.ReadFile(filepath.Join(lib, "steamapps", "appmanifest_"+appid+".acf"))
		if match := manifestName.FindStringSubmatch(string(data)); err == nil && match != nil {
			return match[1]
		}
	}
	return "Steam"
}

// steamGame returns the running Steam game with this app id, listing it when it is new.
func (m *Manager) steamGame(appid string, reaper int, unit string) *App {
	id := steamGamePrefix + appid
	m.mu.Lock()
	if a, ok := m.apps[id]; ok {
		m.mu.Unlock()
		return a
	}
	m.mu.Unlock()
	name := m.steamGameName(appid)
	m.mu.Lock()
	if a, ok := m.apps[id]; ok { // listed meanwhile
		m.mu.Unlock()
		return a
	}
	app := &App{ID: id, Name: name, Kind: KindGame, Started: time.Now(), unit: unit, appID: m.newAppID(id), reaper: reaper}
	m.apps[id] = app
	if m.steamAsk != nil { // the game started: Steam did not need the user
		m.steamAsk.Stop()
		m.steamAsk = nil
	}
	m.steamHidden = false
	m.order = append(m.order, id)
	steam := uint32(0)
	if s, ok := m.apps[SteamID]; ok {
		steam = s.appID
	}
	delete(m.steamNames, appid)
	m.mu.Unlock()

	m.Log.Info("Steam game started", "id", id, "name", name)
	m.Broker.Publish("app.started", map[string]string{"id": id, "name": name, "kind": KindGame})
	if sess, err := m.Session(); err == nil {
		if f := m.focus(sess); f != nil {
			m.show(f, []uint32{app.appID, steam, HomeAppID})
		}
		m.cageFocus(sess)
	}
	return app
}

// scanSteam lists the games running inside Steam and notices the ones that ended.
func (m *Manager) scanSteam() {
	m.mu.Lock()
	m.init()
	unit := ""
	if s, ok := m.apps[SteamID]; ok {
		unit = s.unit
	}
	m.mu.Unlock()
	if unit == "" {
		return
	}
	found := map[string]int{}
	for _, pid := range m.Procs.InUnit(unit) {
		if id := reaperAppID(m.Procs.Cmdline(pid)); id != "" {
			if old, ok := found[id]; !ok || pid < old {
				found[id] = pid
			}
		}
	}
	for appid, reaper := range found {
		m.steamGame(appid, reaper, unit)
	}
	m.mu.Lock()
	var gone []*App
	for _, a := range m.apps {
		if a.reaper != 0 {
			if pid, ok := found[strings.TrimPrefix(a.ID, steamGamePrefix)]; !ok || pid != a.reaper {
				gone = append(gone, a)
			}
		}
	}
	m.mu.Unlock()
	for _, a := range gone {
		m.ended(a, "success")
	}
}

// signalGame signals every process of a Steam game (not Steam itself); with SIGKILL the launcher too.
func (m *Manager) signalGame(a *App, sig syscall.Signal) {
	for _, pid := range m.Procs.InUnit(a.unit) {
		if m.descends(pid, a.reaper) || (sig == syscall.SIGKILL && pid == a.reaper) {
			m.Procs.Signal(pid, sig)
		}
	}
}

// closeSteamGame asks a Steam game to close, and force-closes it after 10 seconds.
func (m *Manager) closeSteamGame(a *App, force bool) {
	if force {
		m.Log.Info("force-closing Steam game", "id", a.ID)
		m.signalGame(a, syscall.SIGKILL)
		return
	}
	m.Log.Info("closing Steam game", "id", a.ID)
	m.signalGame(a, syscall.SIGTERM)
	time.AfterFunc(10*time.Second, func() {
		m.mu.Lock()
		still := m.apps[a.ID] == a
		m.mu.Unlock()
		if still {
			m.signalGame(a, syscall.SIGKILL)
		}
	})
}

// steamRequest: Steam wrote what gamescope should show, in its own codes (769: its own window; a
// Steam app id: that game; others are unknown). While Steam or one of its games is in front,
// owneetd shows what Steam asks for, in owneetd's app ids; otherwise (e.g. the home screen after
// Guide) owneetd puts its own choice back: Steam never takes the screen by itself.
func (m *Manager) steamRequest(ids []uint32) {
	sess, err := m.Session()
	if err != nil {
		return
	}
	f := m.focus(sess)
	if f == nil {
		return
	}
	m.mu.Lock()
	m.init()
	steam, running := m.apps[SteamID]
	shown := append([]uint32(nil), m.shown...)
	steamFront := false
	if running && len(shown) > 0 {
		steamFront = shown[0] == steam.appID
		for _, a := range m.apps {
			steamFront = steamFront || (a.reaper != 0 && a.appID == shown[0])
		}
	}
	var want []uint32
	if steamFront {
		seen := map[uint32]bool{}
		add := func(id uint32) {
			if !seen[id] {
				seen[id] = true
				want = append(want, id)
			}
		}
		for _, id := range ids {
			if id == steamUIAppID {
				add(steam.appID)
			} else if g, ok := m.apps[steamGamePrefix+strconv.FormatUint(uint64(id), 10)]; ok {
				add(g.appID)
			}
		}
		add(steam.appID)
		add(HomeAppID)
	}
	m.mu.Unlock()
	if want == nil {
		if len(shown) == 0 {
			shown = []uint32{HomeAppID}
		}
		m.Log.Debug("something else chose what gamescope shows: put back", "asked", ids, "shown", shown)
		m.show(f, shown)
		return
	}
	m.Log.Debug("Steam chose what gamescope shows", "asked", ids, "shown", want)
	m.show(f, want)
}
