// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/live"
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
// 2.4 GB) is kept in RAM ([[steam-integration]], 2026-10-11): the rule of package live.
const SteamLiveMinRAM = live.MinRAM

// ErrSteamNeedsInstall: the live system has too little memory for Steam's files.
var ErrSteamNeedsInstall = errors.New("Steam needs the installed system: the live system has too little memory for its files")

// SteamRequest is the body of POST /v1/steam: open a page of Steam, run a game, or ask Steam to
// install or uninstall one (Steam shows its confirmation; [[step-4.2c-steam-without-its-interface]]).
type SteamRequest struct {
	Open      string `json:"open,omitempty"`      // "library", "store" or "storage"
	Run       string `json:"run,omitempty"`       // a Steam app id
	Install   string `json:"install,omitempty"`   // a Steam app id
	Uninstall string `json:"uninstall,omitempty"` // a Steam app id (also cancels its download)
	// Ask: the next start of this Steam app id asks the user first (a licence agreement to
	// accept): Steam's window is shown at once for it, not kept hidden
	Ask string `json:"ask,omitempty"`
	// Windows: a Steam app id whose Linux version does not start: its version for Windows is set
	// in Steam (Steam closed meanwhile) and the game started again (steamwindows.go)
	Windows string `json:"windows,omitempty"`
	Name    string `json:"name,omitempty"` // the game's name (shown while it runs)
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

// steamAllowed: always in the installed system; in the live system (live.Mode) with enough RAM.
func steamAllowed() bool { return live.Allowed(SteamLiveMinRAM) }

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

// steamGPUArgs: with NVIDIA's driver, Steam's pages drawn with the GPU (its web views) showed
// garbled, stale pictures inside gamescope (FrankenPC, GTX 1660 Super), and until 2026-10-15
// Steam drew them without the GPU ("-cef-disable-gpu"). Since the repaint overlay (repaint.go)
// they keep the GPU on NVIDIA too: no stale pictures came back with it on the FrankenPC (owner,
// 2026-10-15: keep it, turn it off if they come back). Test switch owneet.debug.steam_cef_gpu=off
// draws them without the GPU again. Games always keep the GPU.
func (m *Manager) steamGPUArgs() []string {
	if m.NVIDIA() && DebugOption("steam_cef_gpu") == "off" {
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

// steamStopLimit: how long Steam may take to close: its service's own stop limit (20 s, systemd.go),
// after which systemd ends it, plus a margin
const steamStopLimit = 25 * time.Second

// closingSteam remembers that Steam was asked to close, until it has ended (forget).
func (m *Manager) closingSteam(id string) {
	if id != SteamID {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.steamGone == nil {
		m.steamGone = make(chan struct{})
	}
}

// steamOSArgs: the options SteamOS-like sessions start Steam with (ChimeraOS: -steamos3 -steampal
// -steamdeck), undocumented by Valve, tried one at a time with the testers' switch
// owneet.debug.steam_os=os|pal|deck|all ([[steam-the-steamos-way]], [[game-compatibility-method]]): a
// game without controller support (BioShock, FrankenPC 2026-11-19) got Steam's gamepad layout, where
// SteamOS gives its keyboard-and-mouse ones.
func steamOSArgs(choice string) []string {
	switch choice {
	case "os":
		return []string{"-steamos3"}
	case "pal":
		return []string{"-steampal"}
	case "deck":
		return []string{"-steamdeck"}
	case "all":
		return []string{"-steamos3", "-steampal", "-steamdeck"}
	}
	return nil
}

// SteamProtonLogDir: where Proton writes its log of each Steam game (steam-APPID.log) with the
// testers' switch on. /tmp is in memory (nothing stays on the disk, [[open-items]] 3p) and Steam's
// container shares it; the test log copies the logs to the OWNEETLOG stick (owneet-testlog).
const SteamProtonLogDir = "/tmp/owneet-proton-logs"

// steamProtonLog: testers' switch owneet.debug.windows_log=on (boot menu, or owneet-debug.txt on the
// OWNEETLOG stick), the one of the console's Windows games (owneet-windows-run), turns Proton's log on
// for Steam's games too, with Proton's own channels ([[open-items]] 13, owner 2026-11-18: Cyberpunk
// 2077 ended a second after starting and nothing said why). Read when Steam starts.
func steamProtonLog() []string {
	if DebugOption("windows_log") != "on" {
		return nil
	}
	if err := os.MkdirAll(SteamProtonLogDir, 0o700); err != nil {
		return nil
	}
	// Proton's own channels without their traces of every exception and unwind (ELDEN RING: 155 MB
	// for one session, in memory): errors, a game's own messages (debugstr: Unreal's fatal errors),
	// the libraries loaded, and the exceptions' backtraces (warn+seh) are kept
	return []string{"PROTON_LOG=1", "PROTON_LOG_DIR=" + SteamProtonLogDir,
		"WINEDEBUG=+timestamp,+pid,+tid,+threadname,+debugstr,+loaddll,+mscoree,warn+seh"}
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
	for _, f := range []string{req.Open, req.Run, req.Install, req.Uninstall, req.Ask, req.Windows} {
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
	case appIDPattern.MatchString(req.Windows):
		m.mu.Lock()
		m.init()
		allowed := m.SteamAllowed
		game := false
		for _, a := range m.apps {
			game = game || a.Kind == KindGame
		}
		m.mu.Unlock()
		if !allowed() {
			return ErrSteamNeedsInstall
		}
		if game {
			return ErrGameRunning
		}
		go m.steamWindows(req)
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
	case req.Open == "storage":
		// Steam moves its own games: its storage settings ([[step-6.6-moving-games-and-media-between-disks]])
		url = "steam://settings/storage"
	}
	if url == "" {
		return fmt.Errorf("%w: one of open (library, store, storage), run, install, uninstall, ask, windows (a Steam app id)", ErrInvalid)
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
			if exclusive(a.Kind) {
				m.mu.Unlock()
				return ErrGameRunning
			}
		}
		if req.Name != "" {
			m.steamNames[req.Run] = req.Name
		}
	}
	_, running := m.apps[SteamID]
	dir, gone := m.SteamDir, m.steamGone
	m.mu.Unlock()
	if running && gone != nil {
		// Steam is still closing (asked to by the console): an address given to it now starts a
		// second Steam of its own, without the console's options (-gamepadui), outside the console's
		// service (FrankenPC, 2026-11-19: Steam closed and opened again within 4 s). Steam starts
		// again once it has ended, at most steamStopLimit later (its own service's stop limit).
		m.Log.Info("Steam is closing: it starts again once it has ended")
		select {
		case <-gone:
			running = false
		case <-time.After(steamStopLimit):
			m.Log.Warn("Steam did not end in time: the address goes to it", "limit", steamStopLimit)
		}
	}

	sess, err := m.Session()
	if err != nil {
		return ErrNoSession
	}
	// Steam's own windows are shown while it starts a game, as on SteamOS: its shader preparation, an
	// install script, a publisher's launcher, its questions ([[game-compatibility-method]], owner
	// 2026-11-18). The old way (hidden until the game's window) only with HideSteamLaunch: then not
	// when nobody has signed in yet or the game asks the user first (ask)
	m.mu.Lock()
	asks := req.Run != "" && req.Run == m.steamAskFor
	if req.Run != "" {
		m.steamAskFor = ""
	}
	hideLaunch := m.HideSteamLaunch
	m.mu.Unlock()
	hidden := req.Run != "" && hideLaunch != nil && hideLaunch() && steamAccount(dir) && !asks
	m.mu.Lock()
	m.steamHidden = hidden
	m.steamConfirm = nil
	if req.Run != "" { // its failure to start is looked for in Steam's log (steamwindows.go)
		offset := int64(0)
		if running {
			offset = steamLogSize(dir)
		}
		m.steamLaunch = &steamLaunch{appid: req.Run, at: time.Now(), offset: offset}
	}
	if id := req.Install + req.Uninstall; id != "" {
		flags, has := steamManifest(dir, id)
		m.steamConfirm = &steamConfirm{appid: id, flags: flags, has: has, at: time.Now()}
	}
	m.mu.Unlock()
	if !running {
		// Steam is closed: Steam Input on for every controller, once per account (steamlib.go)
		m.mu.Lock()
		stateDir, mountState := m.StateDir, m.MountState
		m.mu.Unlock()
		if accounts, err := steamInputOnce(dir, stateDir); err != nil {
			m.Log.Warn("cannot turn Steam Input on", "err", err)
		} else if len(accounts) > 0 {
			m.Log.Info("Steam Input turned on for every controller", "accounts", accounts)
		}
		// the disks prepared for games become Steam libraries (steamdisks.go)
		if libs, err := steamLibraryDisks(dir, mountState, os.Getuid()); err != nil {
			m.Log.Warn("cannot add the disks for games to Steam", "err", err)
		} else if len(libs) > 0 {
			m.Log.Info("disks for games added to Steam's libraries", "folders", libs)
		}
		args := append([]string{"-gamepadui"}, m.steamGPUArgs()...)
		args = append(args, steamOSArgs(DebugOption("steam_os"))...)
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
		// testers only, to compare: owneet.debug.steam_xalia=off starts Steam's games without Proton's
		// Xalia (BioShock, 2026-11-19: every button seemed to press Esc next to Steam's layout)
		if DebugOption("steam_xalia") == "off" {
			argv = append(append(append([]string{}, argv[:3]...), "PROTON_USE_XALIA=0"), argv[3:]...)
			m.Log.Info("test: Proton's Xalia off for Steam's games")
		}
		// testers only: owneet.debug.wine_focus_hack=off starts Steam's games with Wine's gamescope focus
		// handling off (WINE_IGNORE_FOCUS_HACK=0: no desktop made the foreground when the game's window
		// loses the focus), to see whether a game in exclusive full screen still draws black when the
		// console comes back to it (Paladins, FrankenPC 2026-11-23, [[open-items]] 20)
		if DebugOption("wine_focus_hack") == "off" {
			argv = append(append(append([]string{}, argv[:3]...), "WINE_IGNORE_FOCUS_HACK=0"), argv[3:]...)
			m.Log.Info("test: Wine's gamescope focus handling off for Steam's games")
		}
		if logEnv := steamProtonLog(); logEnv != nil {
			// env's own assignments, before the program: argv is env -u NAME PROGRAM ARGS…
			argv = append(append(append([]string{}, argv[:3]...), logEnv...), argv[3:]...)
			m.Log.Info("Proton's log on for Steam's games (testers)", "folder", SteamProtonLogDir)
		}
		if gamescope {
			m.mu.Lock()
			m.steamPending = url
			m.steamHold = !hidden && req.Run == "" && steamAccount(dir)
			m.steamPageOff = false
			m.steamLogFrom = steamInterfaceLogSize(dir)
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
	var phases map[string]string
	if running {
		m.steamContent.read(dir)
		phases = m.steamContent.phase
	} else {
		m.steamContent = steamContent{} // read again from the start when Steam runs again
	}
	downloads := steamDownloads(dir, m.steamSamples, phases, time.Now())
	m.steamMu.Unlock()
	return SteamStatus{Installed: err == nil, Allowed: allowed(), Prepared: prepared == nil, Running: running,
		Games: installedSteamGames(dir), Downloads: downloads, Away: steamAway(dir), Account: steamAccount(dir),
		Licences: steamLicences(dir), NVIDIA: m.NVIDIA()}
}

// steamInterfaceLog is Steam's log of its interface.
func steamInterfaceLog(dir string) string { return filepath.Join(dir, "logs", "webhelper.txt") }

func steamInterfaceLogSize(dir string) int64 {
	if st, err := os.Stat(steamInterfaceLog(dir)); err == nil {
		return st.Size()
	}
	return 0
}

// steamInterfaceUp is the line Steam writes when the last piece of its signed-in interface has loaded:
// in all 33 starts since it first appears in FrankenPC's log, 9 to 12 s after the interface starts there.
var steamInterfaceUp = regexp.MustCompile(`notificationtoasts_uid[0-9]+[^\n]*STARTUP LoadEnd`)

// steamInterfaceReady: that line came after `from`, where the log ended when Steam started. Steam
// starts a new log when it is too long: a smaller file is read from its start.
func steamInterfaceReady(dir string, from int64) bool {
	f, err := os.Open(steamInterfaceLog(dir))
	if err != nil {
		return false
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || st.Size() < from {
		from = 0
	}
	if _, err := f.Seek(from, io.SeekStart); err != nil {
		return false
	}
	data, err := io.ReadAll(f)
	return err == nil && steamInterfaceUp.Match(data)
}

// steamAskAfter: a safety limit ([[waiting-for-events]]). Steam's console log says when it needs the
// user, or when a launch ends (watchLaunch shows Steam at once then); a game asked of a hidden Steam
// whose launch the log has not named by this time is shown in Steam, which may be waiting (signed
// out, an address dropped). Not while Steam prepares the game's shaders (fossilize_replay, up to a
// minute or more the first time: FrankenPC, 2026-10-14): the limit starts again when that ends.
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

func (m *Manager) steamAskLater() { m.steamAskIn(steamAskAfter) }

func (m *Manager) steamAskIn(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.steamAsk != nil {
		m.steamAsk.Stop()
	}
	m.steamAsk = time.AfterFunc(d, m.steamAskNow)
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
	// Steam still works on the launch (its console log, steamwindows.go): it stays hidden, and
	// watchLaunch shows it as soon as the log says it needs the user or the launch has ended
	m.mu.Lock()
	launch := m.steamLaunch
	busy, task, told := false, "", true
	if launch != nil {
		busy, task = launch.state.busy(), launch.state.task
		told, launch.told = launch.told == task, task
	}
	if busy {
		m.steamHidden = true
	}
	m.mu.Unlock()
	if busy {
		if !told {
			m.Log.Info("Steam is preparing the game: still hidden", "task", task)
		}
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
	if running {
		m.watchLaunch()
		m.mu.Lock()
		unit := ""
		if s, ok := m.apps[SteamID]; ok {
			unit = s.unit
		}
		m.mu.Unlock()
		m.installScriptEnded(unit)
		m.pauseXalia()
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
// Steam's install script of a game's first start (reaper SteamLaunch AppId=N Install=1: the
// redistributables, before the game) is not the game: Steam's launch goes on after it (its shader
// cache next), and the game starts later (Batman: Arkham Knight, FrankenPC, 2026-11-13).
func reaperAppID(argv []string) string {
	if len(argv) < 3 || filepath.Base(argv[0]) != "reaper" {
		return ""
	}
	launch, id := false, ""
	for _, a := range argv[1:] {
		if a == "--" {
			break
		}
		switch {
		case a == "SteamLaunch":
			launch = true
		case a == "Install=1":
			return ""
		case strings.HasPrefix(a, "AppId=") && launch && id == "":
			if v := strings.TrimPrefix(a, "AppId="); appIDPattern.MatchString(v) {
				id = v
			}
		}
	}
	return id
}

// steamInstallScript: Steam's launcher of a game's install script ("reaper SteamLaunch AppId=N
// Install=1"), whose programs (the redistributables, a publisher's own installer) may open a window
// that waits for the user.
func steamInstallScript(argv []string) bool {
	if len(argv) < 3 || filepath.Base(argv[0]) != "reaper" {
		return false
	}
	launch := false
	for _, a := range argv[1:] {
		if a == "--" {
			break
		}
		switch {
		case a == "SteamLaunch":
			launch = true
		case a == "Install=1":
			return launch
		}
	}
	return false
}

// inInstallScript: the process comes from a game's install script (steamInstallScript).
func (m *Manager) inInstallScript(pid int) bool {
	for i := 0; i < 64 && pid > 1; i++ {
		if steamInstallScript(m.Procs.Cmdline(pid)) {
			return true
		}
		pid = m.Procs.Parent(pid)
	}
	return false
}

// installScriptWindow: a window of a game's install script came on screen. It waits for the user:
// Steam is shown if it was hidden for the launch (steamAskNow; Jedi: Fallen Order's EA install script,
// FrankenPC, 2026-11-18, [[open-items]] 13: an "Open" file window stayed hidden behind "Starting …" for
// 4 minutes), and the controller pointer is turned on for it by itself: such windows are publishers'
// installers made for a mouse (Red Dead Redemption 2's Rockstar Games Launcher installer, 2026-11-19;
// owner: option B, [[controller-input-map]]). The pointer holds for that window only.
func (m *Manager) installScriptWindow(pid int) {
	m.mu.Lock()
	hidden := m.steamHidden
	m.steamHidden = false
	pointerOn := m.PointerOn
	m.mu.Unlock()
	if hidden {
		m.Log.Info("a window of the game's install script: Steam shown", "pid", pid)
		if err := m.FocusApp(SteamID); err != nil {
			m.Log.Debug("cannot show Steam", "err", err)
		}
	}
	if pointerOn != nil {
		m.mu.Lock()
		already := m.installPointer
		m.mu.Unlock()
		if err := pointerOn(SteamID); err != nil {
			m.Log.Warn("cannot turn the pointer on for the install script's window", "err", err)
		} else {
			m.mu.Lock()
			m.installPointer = true
			m.mu.Unlock()
			if !already {
				m.Log.Info("a window of the game's install script: the pointer on for its windows", "pid", pid)
			}
		}
	}
}

// xaliaProgram: Proton's Xalia (it makes Windows launchers usable with a controller, run by Proton in
// Steam's games: "…/files/share/wine/../xalia/xalia.exe")
func xaliaProgram(argv []string) bool {
	return len(argv) > 0 && strings.HasSuffix(strings.ToLower(strings.ReplaceAll(argv[0], "\\", "/")), "/xalia.exe")
}

// PointerChanged: while the console's pointer is on, Proton's Xalia is paused (SIGSTOP), and it goes
// on when the pointer goes off (SIGCONT). Both read the same controller over the same window: Red Dead
// Redemption 2's Rockstar launcher had its checkbox ticked by the pointer's A and unticked by Xalia's
// (FrankenPC, 2026-11-19, owner). Xalia is left alone whenever the pointer is off (Jedi: Fallen Order's EA
// window went through with it).
func (m *Manager) PointerChanged(on bool) {
	m.mu.Lock()
	m.pointerIsOn = on
	m.mu.Unlock()
	if on {
		m.pauseXalia()
		return
	}
	m.mu.Lock()
	paused := m.xaliaPaused
	m.xaliaPaused = nil
	m.mu.Unlock()
	for pid := range paused {
		if err := m.Procs.Signal(pid, syscall.SIGCONT); err == nil {
			m.Log.Info("Proton's Xalia goes on: the pointer is off", "pid", pid)
		}
	}
}

// pauseXalia pauses every Xalia of Steam's games not paused yet (at the pointer's start, and each second
// while it is on: one started meanwhile)
func (m *Manager) pauseXalia() {
	m.mu.Lock()
	on := m.pointerIsOn
	unit := ""
	if s, ok := m.apps[SteamID]; ok {
		unit = s.unit
	}
	if m.xaliaPaused == nil {
		m.xaliaPaused = map[int]bool{}
	}
	m.mu.Unlock()
	if !on || unit == "" {
		return
	}
	for _, pid := range m.Procs.InUnit(unit) {
		m.mu.Lock()
		done := m.xaliaPaused[pid]
		m.mu.Unlock()
		if done || !xaliaProgram(m.Procs.Cmdline(pid)) {
			continue
		}
		if err := m.Procs.Signal(pid, syscall.SIGSTOP); err != nil {
			continue
		}
		m.mu.Lock()
		m.xaliaPaused[pid] = true
		m.mu.Unlock()
		m.Log.Info("Proton's Xalia paused while the console's pointer is on", "pid", pid)
	}
}

// installScriptEnded: the pointer turned on for a game's install script goes off once none of the
// script's programs is left (its windows all belong to it: an installer and its own file windows,
// owner 2026-11-19), whether Steam ran it for the console's launch or for its own; and as soon as
// none of its windows is shown, even while its programs run. The pointer pauses Proton's Xalia
// (PointerChanged), and Steam's install script did not end while Xalia was paused: The Fractured
// But Whole's started again right after the game stayed at "Starting…" with nothing of the script
// on screen, its window having shown for a few seconds, until Xalia went on (16 minutes at 09:20,
// ended 11 s after Xalia went on; again at 10:18 and 10:50; FrankenPC, 2026-11-23). A new window of
// the script turns the pointer on again (installScriptWindow).
func (m *Manager) installScriptEnded(unit string) {
	m.mu.Lock()
	on, off := m.installPointer, m.PointerOff
	m.mu.Unlock()
	if !on || unit == "" {
		return
	}
	running := false
	for _, pid := range m.Procs.InUnit(unit) {
		if m.inInstallScript(pid) {
			running = true
			break
		}
	}
	if running && m.installScriptShown() {
		return
	}
	m.mu.Lock()
	m.installPointer = false
	m.mu.Unlock()
	reason := "install script ended"
	if running {
		reason = "install script's windows gone"
		m.Log.Info("no window of the game's install script on screen: the pointer off")
	} else {
		m.Log.Info("the game's install script has ended: the pointer off")
	}
	if off != nil {
		off(reason)
	}
}

// installScriptShown: a window of a game's install script is shown (not Xalia's own: it is what
// the pointer pauses). True when it cannot be known.
func (m *Manager) installScriptShown() bool {
	sess, err := m.Session()
	if err != nil {
		return true
	}
	f := m.focus(sess)
	if f == nil {
		return true
	}
	pids, err := f.WindowPIDs()
	if err != nil {
		return true
	}
	for _, pid := range pids {
		if m.inInstallScript(pid) && !xaliaProgram(m.Procs.Cmdline(pid)) {
			return true
		}
	}
	return false
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
	hideLaunch := m.HideSteamLaunch
	m.mu.Unlock()
	behind := hideLaunch != nil && hideLaunch() // reads the testers' switch: outside the lock
	m.mu.Lock()
	if a, ok := m.apps[id]; ok { // listed meanwhile
		m.mu.Unlock()
		return a
	}
	// the old way only: Steam's window left out until the game's own window (steamBehind)
	app := &App{ID: id, Name: name, Kind: KindGame, Started: time.Now(), unit: unit, appID: m.newAppID(id), reaper: reaper,
		steamBehind: behind}
	m.apps[id] = app
	if m.steamAsk != nil { // the game started: Steam did not need the user
		m.steamAsk.Stop()
		m.steamAsk = nil
	}
	m.steamHidden = false
	if m.steamLaunch != nil && m.steamLaunch.appid == appid { // it started
		m.steamLaunch = nil
	}
	m.order = append(m.order, id)
	delete(m.steamNames, appid)
	m.mu.Unlock()

	m.Log.Info("Steam game started", "id", id, "name", name)
	m.Broker.Publish("app.started", map[string]string{"id": id, "name": name, "kind": KindGame})
	if sess, err := m.Session(); err == nil {
		if f := m.focus(sess); f != nil {
			if app.steamBehind {
				// the old way: Steam behind the game only once the game's window is there (checkWindows)
				m.show(f, []uint32{app.appID, HomeAppID})
			} else if steam := m.appIDOf(SteamID); steam != 0 {
				// Steam behind the game from its start: Steam's window (its "launching" page, a
				// question) stays on screen until the game's own window comes
				m.show(f, []uint32{app.appID, steam, HomeAppID})
			} else {
				m.show(f, []uint32{app.appID, HomeAppID})
			}
		}
		m.cageFocus(sess)
	}
	// the safety limit: Steam's log says when it waits for the user (watchLaunch); a game may take
	// long to open its window on a slow PC, so only this long without one shows Steam behind it
	time.AfterFunc(steamLaunchWatch, func() { m.steamGameNoWindow(app) })
	return app
}

// steamGameNoWindow: a Steam game started, still without a window, and Steam's log says it waits for
// the user (or steamLaunchWatch has passed without a word): Steam is shown behind the game again.
func (m *Manager) steamGameNoWindow(app *App) {
	m.mu.Lock()
	steam, running := m.apps[SteamID]
	wait := app.steamBehind && m.apps[app.ID] == app && running
	front := len(m.shown) > 0 && m.shown[0] == app.appID
	app.steamBehind = false
	m.mu.Unlock()
	if !wait || !front {
		return
	}
	m.Log.Info("the Steam game has no window yet: Steam may be waiting for the user, shown", "id", app.ID)
	if sess, err := m.Session(); err == nil {
		if f := m.focus(sess); f != nil {
			m.show(f, []uint32{app.appID, steam.appID, HomeAppID})
		}
	}
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
	m.leftOverLaunchers(found)
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
// Steam app id: that game) or in owneetd's (Steam reads them back from its windows' STEAM_GAME,
// which owneetd sets: e.g. when it resumes a running game; others are unknown). While Steam or one
// of its games is in front, owneetd shows what Steam asks for, in owneetd's app ids, but a Steam
// game in front stays in front when Steam does not name it (its window may change: Batman: Arkham
// Knight lost the screen to the home screen when its first window went, FrankenPC, 2026-11-13);
// otherwise (e.g. the home screen after Guide) owneetd puts its own choice back: Steam never takes
// the screen by itself.
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
		// a game whose window has not appeared yet keeps Steam's window out (steamBehind)
		behind := false
		for _, a := range m.apps {
			behind = behind || a.steamBehind
		}
		// the Steam game in front, kept there unless Steam names it
		var front *App
		for _, a := range m.apps {
			if a.reaper != 0 && a.appID == shown[0] {
				front = a
			}
		}
		named := false
		for _, id := range ids {
			own := m.appOfID(id)
			switch {
			case id == steamUIAppID || (own != nil && own.ID == SteamID):
				if !behind {
					add(steam.appID)
				}
			case own != nil && own.reaper != 0:
				named = named || own == front
				add(own.appID)
			default:
				if g, ok := m.apps[steamGamePrefix+strconv.FormatUint(uint64(id), 10)]; ok {
					named = named || g == front
					add(g.appID)
				}
			}
		}
		if front != nil && !named {
			want = append([]uint32{front.appID}, want...)
			seen[front.appID] = true
		}
		if !behind {
			add(steam.appID)
		}
		add(HomeAppID)
	}
	m.mu.Unlock()
	if want == nil {
		if len(shown) == 0 {
			shown = []uint32{HomeAppID}
		}
		m.Log.Info("something else chose what gamescope shows: put back", "asked", ids, "shown", shown)
		m.show(f, shown)
		m.screenChanged()
		return
	}
	m.Log.Info("Steam chose what gamescope shows", "asked", ids, "shown", want)
	m.show(f, want)
	m.screenChanged() // gamescope may already focus what Steam asked for: no notice of its own
}
