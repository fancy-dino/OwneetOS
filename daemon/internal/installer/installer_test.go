// SPDX-License-Identifier: GPL-3.0-or-later

package installer

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

const disksJSON = `{"disks":[
 {"path":"/dev/nvme0n1","serial":"S1","size":500107862016,"model":"Samsung SSD 980","usable":true,"reason":"","windows":true,"bitlocker":false,"linux":false,"partitions":[{"path":"/dev/nvme0n1p1","size":1,"fstype":"ntfs","label":"Volume"}]},
 {"path":"/dev/sda","serial":"S2","size":1000204886016,"model":"Crucial MX500","usable":true,"reason":"","windows":false,"bitlocker":false,"linux":false,"partitions":[]},
 {"path":"/dev/sdb","serial":"S3","size":16008609792,"model":"Kingston","usable":false,"reason":"too_small","windows":false,"bitlocker":false,"linux":false,"partitions":[]}]}`

// fakeUnits writes the backend's state when the unit is started
type fakeUnits struct {
	state   string
	started []string
	err     error
}

func (f *fakeUnits) Start(unit string) error {
	f.started = append(f.started, unit)
	if f.err != nil {
		return f.err
	}
	if f.state != "" {
		go func() {
			time.Sleep(20 * time.Millisecond)
			_ = os.WriteFile(stateFile, []byte(f.state), 0o644)
		}()
	}
	return nil
}

var stateFile string

func newManager(t *testing.T, units *fakeUnits, live bool) *Manager {
	dir := t.TempDir()
	stateFile = filepath.Join(dir, "install.json")
	return &Manager{
		Broker: events.NewBroker(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Units: units,
		RequestFile: filepath.Join(dir, "request"), StateFile: stateFile,
		Live: func() bool { return live }, Poll: 10 * time.Millisecond, Timeout: 2 * time.Second,
		DisksJSON: func() ([]byte, error) { return []byte(disksJSON), nil },
	}
}

func waitFor(t *testing.T, m *Manager, state string) Status {
	t.Helper()
	for i := 0; i < 200; i++ {
		if st := m.Status(); st.State == state {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("state %q never came: %+v", state, m.Status())
	return Status{}
}

func TestDisks(t *testing.T) {
	m := newManager(t, &fakeUnits{}, true)
	disks, err := m.Disks()
	if err != nil || len(disks) != 3 || !disks[0].Windows || disks[2].Usable || disks[2].Reason != "too_small" {
		t.Fatalf("Disks() = %+v, %v", disks, err)
	}
}

func TestStartWritesTheRequest(t *testing.T) {
	units := &fakeUnits{state: `{"state":"done","percent":100,"error":"","time":"x"}`}
	m := newManager(t, units, true)
	err := m.Start(Request{Disk: "/dev/sda", Size: 1000204886016, Serial: "S2", Name: "  Più\tConsole\n "})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(m.RequestFile)
	want := "OWNEET_INSTALL_DISK=/dev/sda\nOWNEET_INSTALL_SIZE=1000204886016\nOWNEET_INSTALL_SERIAL=S2\n" +
		"OWNEET_INSTALL_NAME=PiùConsole\nOWNEET_INSTALL_ERASE=/dev/sda\n"
	if string(data) != want {
		t.Errorf("request:\n%s\nwant:\n%s", data, want)
	}
	if len(units.started) != 1 || units.started[0] != Unit {
		t.Errorf("units started: %v", units.started)
	}
	waitFor(t, m, "done")
}

func TestStartRefuses(t *testing.T) {
	for name, c := range map[string]struct {
		req  Request
		live bool
		want error
	}{
		"not live":                {Request{Disk: "/dev/sda", Size: 1000204886016, Serial: "S2"}, false, ErrNotLive},
		"unknown disk":            {Request{Disk: "/dev/sdz", Size: 1, Serial: "S9"}, true, ErrDiskChanged},
		"another size":            {Request{Disk: "/dev/sda", Size: 1, Serial: "S2"}, true, ErrDiskChanged},
		"another serial":          {Request{Disk: "/dev/sda", Size: 1000204886016, Serial: "S9"}, true, ErrDiskChanged},
		"too small":               {Request{Disk: "/dev/sdb", Size: 16008609792, Serial: "S3"}, true, ErrNotUsable},
		"Windows without consent": {Request{Disk: "/dev/nvme0n1", Size: 500107862016, Serial: "S1"}, true, ErrWindowsNotConfirmed},
	} {
		units := &fakeUnits{}
		m := newManager(t, units, c.live)
		if err := m.Start(c.req); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
		if len(units.started) != 0 {
			t.Errorf("%s: a unit was started", name)
		}
		if _, err := os.Stat(m.RequestFile); err == nil {
			t.Errorf("%s: a request was written", name)
		}
	}
}

func TestWindowsConfirmed(t *testing.T) {
	m := newManager(t, &fakeUnits{state: `{"state":"failed","percent":0,"error":"copy_failed"}`}, true)
	if err := m.Start(Request{Disk: "/dev/nvme0n1", Size: 500107862016, Serial: "S1", EraseWindows: true}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(m.RequestFile)
	if !strings.Contains(string(data), "OWNEET_INSTALL_ERASE_WINDOWS=yes\n") || !strings.Contains(string(data), "OWNEET_INSTALL_NAME=OwneetOS\n") {
		t.Errorf("request: %s", data)
	}
	if st := waitFor(t, m, "failed"); st.Error != "copy_failed" {
		t.Errorf("status %+v", st)
	}
}

func TestOldStateIsNotThisInstallation(t *testing.T) {
	m := newManager(t, &fakeUnits{}, true)
	// a failed installation before "Try again"
	if err := os.WriteFile(stateFile, []byte(`{"state":"failed","percent":0,"error":"copy_failed"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	m.Timeout = 100 * time.Millisecond
	if err := m.Start(Request{Disk: "/dev/sda", Size: 1000204886016, Serial: "S2"}); err != nil {
		t.Fatal(err)
	}
	if st := m.Status(); st.State != "preparing" {
		t.Errorf("right after the start: %+v, want preparing", st)
	}
	if err := m.Start(Request{Disk: "/dev/sda", Size: 1000204886016, Serial: "S2"}); !errors.Is(err, ErrBusy) {
		t.Errorf("second start: %v, want ErrBusy", err)
	}
	// the unit never writes: failed when the wait is over
	if st := waitFor(t, m, "failed"); st.Error != "failed" {
		t.Errorf("gave up: %+v", st)
	}
}

func TestCleanName(t *testing.T) {
	for in, want := range map[string]string{"": "OwneetOS", "   ": "OwneetOS", "Salotto": "Salotto",
		strings.Repeat("a", 50): strings.Repeat("a", 40), "a\x00b": "ab"} {
		if got := CleanName(in); got != want {
			t.Errorf("CleanName(%q) = %q, want %q", in, got, want)
		}
	}
}

// The other internal disks prepared for games ([[step-6.7-preparing-other-disks]]): any size, named,
// never two the same name, never a name another disk keeps.
func TestGamesDisks(t *testing.T) {
	sdb := GamesDisk{Disk: "/dev/sdb", Size: 16008609792, Serial: "S3", Name: "Giochi"}
	units := &fakeUnits{state: `{"state":"done","percent":100,"error":"","time":"x"}`}
	m := newManager(t, units, true)
	if err := m.Start(Request{Disk: "/dev/sda", Size: 1000204886016, Serial: "S2", Games: []GamesDisk{sdb}, Folder: "Giochi"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(m.RequestFile)
	want := "OWNEET_INSTALL_GAMES_1_DISK=/dev/sdb\nOWNEET_INSTALL_GAMES_1_SIZE=16008609792\nOWNEET_INSTALL_GAMES_1_SERIAL=S3\n" +
		"OWNEET_INSTALL_GAMES_1_ERASE=/dev/sdb\nOWNEET_INSTALL_GAMES_1_NAME=Giochi\nOWNEET_INSTALL_GAMES_FOLDER=Giochi\n"
	if !strings.HasSuffix(string(data), want) {
		t.Errorf("request:\n%s\nwant at the end:\n%s", data, want)
	}
	waitFor(t, m, "done")

	nvme := GamesDisk{Disk: "/dev/nvme0n1", Size: 500107862016, Serial: "S1", Name: "Games 2"}
	named := func(g GamesDisk, name string) GamesDisk { g.Name = name; return g }
	confirmed := func(g GamesDisk) GamesDisk { g.EraseWindows = true; return g }
	sys := Request{Disk: "/dev/sda", Size: 1000204886016, Serial: "S2", Folder: "Games"}
	with := func(r Request, g ...GamesDisk) Request { r.Games = g; return r }
	for name, c := range map[string]struct {
		req  Request
		want error
	}{
		"Windows without consent": {with(sys, nvme), ErrWindowsNotConfirmed},
		"the system's disk":       {with(sys, GamesDisk{Disk: "/dev/sda", Size: 1000204886016, Serial: "S2", Name: "G"}), ErrDiskChanged},
		"another serial":          {with(sys, GamesDisk{Disk: "/dev/sdb", Size: 16008609792, Serial: "S9", Name: "G"}), ErrDiskChanged},
		"the same disk twice":     {with(sys, sdb, named(sdb, "Other")), ErrDiskChanged},
		"empty name":              {with(sys, named(sdb, "")), ErrBadName},
		"too long":                {with(sys, named(sdb, "Giochi giochi gio")), ErrBadName},
		"accents count two":       {with(sys, named(sdb, "Città dei giochi")), ErrBadName},
		"a slash":                 {with(sys, named(sdb, "a/b")), ErrBadName},
		"two the same":            {with(sys, named(sdb, "Giochi"), named(confirmed(nvme), "GIOCHI")), ErrNameTaken},
		"another disk keeps it":   {with(sys, named(sdb, "volume")), ErrNameTaken},
		"unknown folder":          {Request{Disk: "/dev/sda", Size: 1000204886016, Serial: "S2", Games: []GamesDisk{sdb}, Folder: "../x"}, ErrBadName},
	} {
		units := &fakeUnits{}
		m := newManager(t, units, true)
		if err := m.Start(c.req); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
		if len(units.started) != 0 {
			t.Errorf("%s: a unit was started", name)
		}
	}
	// the disk that kept "Volume" is erased itself (OwneetOS goes there): its name is free again
	m = newManager(t, &fakeUnits{}, true)
	m.Timeout = 50 * time.Millisecond
	err := m.Start(Request{Disk: "/dev/nvme0n1", Size: 500107862016, Serial: "S1", EraseWindows: true,
		Games: []GamesDisk{named(sdb, "Volume")}, Folder: "Games"})
	if err != nil {
		t.Errorf("a name the erased system disk kept: %v", err)
	}
}
