// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakePreparer struct {
	dir string
	err error
}

func (f *fakePreparer) Prepare(_ context.Context, progress func(int)) (string, error) {
	progress(30)
	progress(70)
	return f.dir, f.err
}

func waitState(t *testing.T, m *Manager, want string) Status {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st := m.Status(); st.State == want {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("state %q, want %q", m.Status().State, want)
	return Status{}
}

func protonFolder(t *testing.T) string {
	dir := filepath.Join(t.TempDir(), "GE-Proton11-9-x86_64")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "proton"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestNothingWithoutAWindowsGame(t *testing.T) {
	m := &Manager{DataDir: t.TempDir(), Online: func() bool { return true }, Preparer: &fakePreparer{dir: protonFolder(t)}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	time.Sleep(50 * time.Millisecond)
	if st := m.Status(); st.State != StateNone || st.Needed {
		t.Fatalf("status %+v, want none", st)
	}
}

func TestReadyOnceNeededAndOnline(t *testing.T) {
	data := t.TempDir()
	proton := protonFolder(t)
	var online atomic.Bool
	m := &Manager{DataDir: data, Online: online.Load, Preparer: &fakePreparer{dir: proton}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	if err := m.SetNeeded(); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, StateWaiting)
	online.Store(true)
	m.Kick()
	st := waitState(t, m, StateReady)
	if st.Percent != 100 || !st.Needed {
		t.Fatalf("status %+v", st)
	}
	got, _ := os.ReadFile(filepath.Join(data, "proton"))
	if strings.TrimSpace(string(got)) != proton {
		t.Fatalf("proton file %q, want %q", got, proton)
	}
	// remembered: a new manager starts ready
	again := &Manager{DataDir: data}
	if st := again.Status(); st.State != StateReady {
		t.Fatalf("after a restart: %+v", st)
	}
}

func TestFailureIsReported(t *testing.T) {
	m := &Manager{DataDir: t.TempDir(), Online: func() bool { return true }, Preparer: &fakePreparer{err: errors.New("no network")}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	if err := m.SetNeeded(); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, StateFailed)
}

func TestFollowUmuLog(t *testing.T) {
	log := strings.Join([]string{
		"[umu.umu_proton:360] INFO: Downloading GE-Proton11-9-x86_64.sha512sum...",
		"[umu.umu_proton:414] INFO: Downloading GE-Proton11-9-x86_64.tar.gz...",
		"[umu.umu_proton:617] INFO: Extracting GE-Proton11-9-x86_64.tar.gz...",
		"[umu.umu_runtime:163] INFO: Downloading steamrt4 (4.0.20261007.263973), please wait...",
		"[umu.umu_runtime:419] INFO: steamrt4_platform_4.0.20261007.263973: mtree is OK",
		"[umu.umu_run:1036] DEBUG: PROTONPATH=/home/owneet/.local/share/Steam/compatibilitytools.d/GE-Proton11-9-x86_64",
		"[umu.umu_runtime:657] INFO: Running 'GE-Proton11-9-x86_64' using runtime 'steamrt4'",
		"never read",
	}, "\n")
	var parts []int
	u := &Umu{}
	proton, ready := u.follow(strings.NewReader(log), func(p int) { parts = append(parts, p) })
	if !ready || proton != "/home/owneet/.local/share/Steam/compatibilitytools.d/GE-Proton11-9-x86_64" {
		t.Fatalf("follow: %q %v", proton, ready)
	}
	if want := []int{1, 1, 2, 3, 4}; len(parts) != len(want) {
		t.Fatalf("parts %v, want %v", parts, want)
	}
}

func TestPercent(t *testing.T) {
	cases := []struct {
		part  int
		bytes int64
		want  int
	}{
		{0, 0, 0}, {1, 0, 0}, {1, protonBytes / 2, 30}, {1, protonBytes * 2, 60},
		{2, 0, 62}, {3, 0, 65}, {3, runtimeBytes, 97}, {4, 0, 98},
	}
	for _, c := range cases {
		if got := percentOf(c.part, c.bytes); got != c.want {
			t.Errorf("percentOf(%d, %d) = %d, want %d", c.part, c.bytes, got, c.want)
		}
	}
}
