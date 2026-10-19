// SPDX-License-Identifier: GPL-3.0-or-later

// owneetd is the OwneetOS system daemon. It runs as a systemd user service of the console user
// and serves its API on a Unix socket ([[owneetd-design]]).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/api"
	"github.com/fancy-dino/OwneetOS/daemon/internal/apps"
	"github.com/fancy-dino/OwneetOS/daemon/internal/audio"
	"github.com/fancy-dino/OwneetOS/daemon/internal/bluetooth"
	"github.com/fancy-dino/OwneetOS/daemon/internal/config"
	"github.com/fancy-dino/OwneetOS/daemon/internal/display"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
	"github.com/fancy-dino/OwneetOS/daemon/internal/gamepad"
	"github.com/fancy-dino/OwneetOS/daemon/internal/installs"
	"github.com/fancy-dino/OwneetOS/daemon/internal/network"
	"github.com/fancy-dino/OwneetOS/daemon/internal/pointer"
	"github.com/fancy-dino/OwneetOS/daemon/internal/power"
	"github.com/fancy-dino/OwneetOS/daemon/internal/sdldb"
	"github.com/fancy-dino/OwneetOS/daemon/internal/update"
	"github.com/fancy-dino/OwneetOS/daemon/internal/version"
	"github.com/fancy-dino/OwneetOS/daemon/internal/vinput"
	"github.com/fancy-dino/OwneetOS/daemon/internal/windows"
)

func main() {
	configPath := flag.String("config", config.DefaultPath, "configuration file (optional)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("owneetd", version.Version)
		return
	}
	if err := run(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, "owneetd:", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	level, _ := cfg.SlogLevel()
	// stderr goes to the journal: journalctl --user -u owneetd
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	ln, err := api.Listen(cfg.Socket)
	if err != nil {
		return err
	}
	defer os.Remove(cfg.Socket)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	broker := events.NewBroker()

	db, dbErr := sdldb.Load(cfg.ControllerDB)
	if dbErr != nil {
		log.Warn("controller database not loaded: generic controllers may lack a Guide button", "err", dbErr)
	}
	bt := &bluetooth.Manager{
		Broker:                    broker,
		Log:                       log,
		AutoPairWithoutController: cfg.BluetoothAutoPairWithoutController,
	}
	runtimeDir := filepath.Dir(cfg.Socket)
	appsMgr := &apps.Manager{
		Broker: broker,
		Log:    log,
		Session: func() (apps.Session, error) {
			return apps.ReadSession(filepath.Join(runtimeDir, "owneet", "session-env"))
		},
		FocusFor: apps.GamescopeFocus(log),
		UnitOf:   apps.UnitOfPID,
	}
	var appsAPI api.Apps
	if units, err := apps.NewSystemd(log); err != nil {
		log.Warn("games and apps cannot be started", "err", err)
	} else {
		defer units.Close()
		appsMgr.Units = units
		appsAPI = appsMgr
		go appsMgr.Run(ctx.Done())
	}

	pads := &gamepad.Manager{Broker: broker, Log: log, DB: db, BluetoothName: bt.Name}
	if appsAPI != nil {
		// The Guide button belongs to the system ([[controller-input-map]]).
		pads.OnGuide = func(_ string, down bool) { appsMgr.Guide(down) }
	}
	bt.Controllers = func() int { return len(pads.List()) }
	go func() {
		if err := pads.Run(ctx); err != nil {
			log.Error("controller tracking stopped", "err", err)
		}
	}()

	// The virtual keyboard and mouse: made as soon as /dev/uinput can be opened (the session may
	// not be active yet when owneetd starts)
	var input api.VirtualInput
	vin, err := vinput.NewRetrying(ctx, cfg.KeyboardLayout, 3*time.Second, log)
	if err != nil {
		log.Warn("virtual keyboard and mouse not available", "err", err)
	} else {
		defer vin.Close()
		input = vin
		select {
		case <-vin.Ready():
		default:
			log.Info("virtual keyboard and mouse: waiting for access to /dev/uinput")
		}
	}

	var btAPI api.Bluetooth
	if backend, err := bluetooth.NewBlueZ(log, bt); err != nil {
		log.Warn("Bluetooth not available", "err", err)
	} else {
		defer backend.Close()
		bt.Backend = backend
		btAPI = bt
		go bt.Run(ctx)
	}

	var netAPI api.Network
	if nm, err := network.NewNM(log, broker); err != nil {
		log.Warn("networking not available", "err", err)
	} else {
		defer nm.Close()
		netAPI = nm
		go nm.Run(ctx)
	}

	sound := &audio.Pulse{Log: log, Broker: broker}
	go sound.Run(ctx)

	var powerAPI api.Power
	if logind, err := power.New(log, broker); err != nil {
		log.Warn("power control not available", "err", err)
	} else {
		defer logind.Close()
		powerAPI = logind
		go logind.Run(ctx)
	}

	// Settings → Display: files shared with owneet-session (it starts gamescope with them)
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	screen := &display.Manager{
		Broker:            broker,
		Log:               log,
		Session:           appsMgr.Session,
		AppsOpen:          func() bool { return len(appsMgr.Status().Apps) > 0 },
		SysDRM:            "/sys/class/drm",
		SysBacklight:      "/sys/class/backlight",
		PNPFile:           "/usr/share/hwdata/pnp.ids",
		Modes:             display.ModeFile{Path: filepath.Join(configDir, "owneet", "gamescope-modes")},
		Prefs:             display.Prefs{Path: filepath.Join(configDir, "owneet", "display.conf")},
		RestartFile:       filepath.Join(runtimeDir, "owneet", "restart"),
		ConfirmTime:       15 * time.Second,
		ScreenConfirmTime: 30 * time.Second,
	}
	if bl, err := display.NewLogindBacklight(); err == nil {
		defer bl.Close()
		screen.Backlight = bl
	}
	if _, err := exec.LookPath("ddcutil"); err == nil {
		screen.DDC = &display.DDC{Command: "ddcutil", Log: log}
	}
	if helper, err := display.NewSystemdHelper(); err == nil {
		defer helper.Close()
		screen.Helper = helper
	}
	go screen.Run(ctx)

	// Testers' updates from a USB stick ([[step-4.2b-updates-from-a-usb-stick]])
	var updateAPI api.Update
	if starter, err := update.NewSystemdStarter(); err == nil {
		defer starter.Close()
		updateAPI = &update.Manager{Broker: broker, Log: log, Units: starter,
			StateFile: "/run/owneet/update.json", TesterMark: "/etc/owneet/tester"}
	}

	// Windows games ([[step-4.5-windows-games-proton]]): Proton and its runtime, downloaded once
	// by umu-launcher when the library has a Windows game and the console is online
	home, _ := os.UserHomeDir()
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = filepath.Join(home, ".cache")
	}
	winData := filepath.Join(home, ".local", "share", "owneet", "windows")
	winMgr := &windows.Manager{Broker: broker, Log: log, DataDir: winData}
	if netAPI != nil {
		winMgr.Online = func() bool { return netAPI.Status().State == "connected" }
	}
	if umu, err := exec.LookPath("umu-run"); err == nil {
		winMgr.Preparer = &windows.Umu{Command: umu, PrefixDir: filepath.Join(winData, "prepare"),
			CacheDir: filepath.Join(cacheDir, "umu")}
	} else {
		log.Warn("Windows games not available: umu-run not found")
	}
	go winMgr.Run(ctx)

	// Games installed from their installers ([[step-4.6-installers]]), on the console's disk;
	// Windows installers run in the console's session (its DISPLAY), never shown
	instMgr := &installs.Manager{Broker: broker, Log: log, Home: home,
		StateDir: filepath.Join(home, ".config", "owneet"), WindowsRun: "/usr/lib/owneet/owneet-windows-run",
		Env: func() []string {
			if sess, err := appsMgr.Session(); err == nil {
				return sess.Env
			}
			return nil
		}}
	go instMgr.Run(ctx)

	// The controller as a mouse for an app's window ([[step-4.7-controller-pointer]]): the
	// virtual mouse, the controllers read without a grab, gamescope's X server for what is on
	// screen and the pointer's shape
	var pointerAPI api.Pointer
	if vin != nil && appsAPI != nil {
		pointerAPI = &pointer.Manager{Broker: broker, Log: log, Mouse: vin, Apps: appsMgr,
			Screen: func() (pointer.Screen, error) {
				sess, err := appsMgr.Session()
				if err != nil {
					return nil, err
				}
				return pointer.DialX(sess.Display)
			},
			Pads: pointer.ReadPads(db, log)}
	}

	srv := &api.Server{
		Broker:          broker,
		Log:             log,
		SessionModeFile: filepath.Join(runtimeDir, "owneet", "session-mode"),
		Controllers:     pads,
		Input:           input,
		Bluetooth:       btAPI,
		Network:         netAPI,
		Audio:           sound,
		Power:           powerAPI,
		Apps:            appsAPI,
		Display:         screen,
		Update:          updateAPI,
		Windows:         winMgr,
		Installs:        instMgr,
		Pointer:         pointerAPI,
	}
	log.Info("owneetd started", "version", version.Version, "socket", cfg.Socket, "controller_db_entries", db.Len())
	err = srv.Serve(ctx, ln)
	log.Info("owneetd stopped")
	return err
}
