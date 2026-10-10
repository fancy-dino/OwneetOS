// SPDX-License-Identifier: GPL-3.0-or-later

// owneetd is the OwneetOS system daemon. It runs as a systemd user service of the console user
// and serves its API on a Unix socket (docs/daemon-design.md).
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
	"github.com/fancy-dino/OwneetOS/daemon/internal/network"
	"github.com/fancy-dino/OwneetOS/daemon/internal/power"
	"github.com/fancy-dino/OwneetOS/daemon/internal/sdldb"
	"github.com/fancy-dino/OwneetOS/daemon/internal/version"
	"github.com/fancy-dino/OwneetOS/daemon/internal/vinput"
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
		// The Guide button belongs to the system (PROJECT_RULES.md section 9.1).
		pads.OnGuide = func(_ string, down bool) { appsMgr.Guide(down) }
	}
	bt.Controllers = func() int { return len(pads.List()) }
	go func() {
		if err := pads.Run(ctx); err != nil {
			log.Error("controller tracking stopped", "err", err)
		}
	}()

	var input api.VirtualInput
	if vin, err := vinput.New(cfg.KeyboardLayout); err != nil {
		log.Warn("virtual keyboard and mouse not available", "err", err)
	} else {
		defer vin.Close()
		input = vin
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
		Broker:       broker,
		Log:          log,
		Session:      appsMgr.Session,
		AppsOpen:     func() bool { return len(appsMgr.Status().Apps) > 0 },
		SysDRM:       "/sys/class/drm",
		SysBacklight: "/sys/class/backlight",
		PNPFile:      "/usr/share/hwdata/pnp.ids",
		Modes:        display.ModeFile{Path: filepath.Join(configDir, "owneet", "gamescope-modes")},
		Prefs:        display.Prefs{Path: filepath.Join(configDir, "owneet", "display.conf")},
		RestartFile:  filepath.Join(runtimeDir, "owneet", "restart"),
		ConfirmTime:  15 * time.Second,
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
	}
	log.Info("owneetd started", "version", version.Version, "socket", cfg.Socket, "controller_db_entries", db.Len())
	err = srv.Serve(ctx, ln)
	log.Info("owneetd stopped")
	return err
}
