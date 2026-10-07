// SPDX-License-Identifier: GPL-3.0-or-later

// Package config loads the daemon configuration: built-in defaults, then the optional file
// /etc/owneet/owneetd.json, then environment variables (OWNEETD_LOG_LEVEL).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// DefaultPath is the system-wide configuration file. It is optional.
const DefaultPath = "/etc/owneet/owneetd.json"

// Config holds every setting of owneetd.
type Config struct {
	// LogLevel is one of "debug", "info", "warn", "error".
	LogLevel string `json:"log_level"`
	// Socket is the path of the API socket. Empty means $XDG_RUNTIME_DIR/owneetd.sock.
	Socket string `json:"socket"`
	// ControllerDB is SDL_GameControllerDB, used to find the Guide button of generic controllers.
	ControllerDB string `json:"controller_db"`
	// KeyboardLayout is the XKB layout of the console session ("us", "it"): the virtual keyboard
	// needs it to type the right characters.
	KeyboardLayout string `json:"keyboard_layout"`
}

// Load returns the configuration from defaults, the file at path (if it exists) and the
// environment.
func Load(path string) (Config, error) {
	cfg := Config{LogLevel: "info", ControllerDB: "/usr/share/owneet/gamecontrollerdb.txt", KeyboardLayout: "us"}

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("%s: %w", path, err)
		}
	case errors.Is(err, fs.ErrNotExist):
		// no file: defaults
	default:
		return cfg, err
	}

	if v := os.Getenv("OWNEETD_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if cfg.Socket == "" {
		cfg.Socket, err = DefaultSocket()
		if err != nil {
			return cfg, err
		}
	}
	if _, err := cfg.SlogLevel(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// DefaultSocket is $XDG_RUNTIME_DIR/owneetd.sock.
func DefaultSocket() (string, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		return "", errors.New("XDG_RUNTIME_DIR is not set (owneetd runs in the console user's session)")
	}
	return filepath.Join(dir, "owneetd.sock"), nil
}

// SlogLevel converts LogLevel for log/slog.
func (c Config) SlogLevel() (slog.Level, error) {
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return slog.LevelInfo, fmt.Errorf("unknown log level %q (debug, info, warn, error)", c.LogLevel)
}
