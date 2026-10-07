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
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/fancy-dino/OwneetOS/daemon/internal/api"
	"github.com/fancy-dino/OwneetOS/daemon/internal/config"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
	"github.com/fancy-dino/OwneetOS/daemon/internal/gamepad"
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
	pads := &gamepad.Manager{Broker: broker, Log: log, DB: db}
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

	srv := &api.Server{
		Broker:          broker,
		Log:             log,
		SessionModeFile: filepath.Join(filepath.Dir(cfg.Socket), "owneet", "session-mode"),
		Controllers:     pads,
		Input:           input,
	}
	log.Info("owneetd started", "version", version.Version, "socket", cfg.Socket, "controller_db_entries", db.Len())
	err = srv.Serve(ctx, ln)
	log.Info("owneetd stopped")
	return err
}
