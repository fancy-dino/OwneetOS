// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsWithoutFile(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	t.Setenv("OWNEETD_LOG_LEVEL", "")
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != "info" || cfg.Socket != "/run/user/1000/owneetd.sock" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestFileThenEnvironment(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	path := filepath.Join(t.TempDir(), "owneetd.json")
	if err := os.WriteFile(path, []byte(`{"log_level":"warn","socket":"/tmp/x.sock"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OWNEETD_LOG_LEVEL", "debug")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != "debug" || cfg.Socket != "/tmp/x.sock" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestBadLogLevel(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	t.Setenv("OWNEETD_LOG_LEVEL", "loud")
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected an error for an unknown log level")
	}
}
