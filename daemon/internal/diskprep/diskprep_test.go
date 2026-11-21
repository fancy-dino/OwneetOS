// SPDX-License-Identifier: GPL-3.0-or-later

package diskprep

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

const disksJSON = `{"disks":[
 {"path":"/dev/sda","serial":"S1","size":96000000000,"model":"Kingston","usb":false,"console":true,"owneetlog":false,"windows":false,"bitlocker":false,"linux":true,"partitions":[{"path":"/dev/sda2","size":1,"fstype":"btrfs","label":"OwneetOS","mountpoint":"/"}]},
 {"path":"/dev/sdb","serial":"S2","size":2000398934016,"model":"ST2000DM008","usb":false,"console":false,"owneetlog":false,"windows":true,"bitlocker":false,"linux":false,"partitions":[{"path":"/dev/sdb2","size":1,"fstype":"ntfs","label":"Volume","mountpoint":"/run/media/owneet/Volume"}]},
 {"path":"/dev/sdc","serial":"S3","size":62000000000,"model":"USB","usb":true,"console":false,"owneetlog":true,"windows":false,"bitlocker":false,"linux":false,"partitions":[{"path":"/dev/sdc1","size":1,"fstype":"vfat","label":"OWNEETLOG","mountpoint":"/run/owneet-testlog"}]},
 {"path":"/dev/sdd","serial":"S4","size":1000000000000,"model":"WD Elements","usb":true,"console":false,"owneetlog":false,"windows":false,"bitlocker":false,"linux":false,"partitions":[]}]}`

type fakeUnits struct {
	state   string
	file    string
	started []string
}

func (f *fakeUnits) Start(unit string) error {
	f.started = append(f.started, unit)
	if f.state != "" {
		go func() {
			time.Sleep(20 * time.Millisecond)
			_ = os.WriteFile(f.file, []byte(f.state), 0o644)
		}()
	}
	return nil
}

func newManager(t *testing.T, units *fakeUnits, live bool) *Manager {
	dir := t.TempDir()
	units.file = filepath.Join(dir, "prepare.json")
	return &Manager{
		Broker: events.NewBroker(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Units: units,
		RequestFile: filepath.Join(dir, "request"), StateFile: units.file,
		Live: func() bool { return live }, Poll: 10 * time.Millisecond, Timeout: 2 * time.Second,
		DisksJSON: func() ([]byte, error) { return []byte(disksJSON), nil },
	}
}

func TestPrepareWritesTheRequest(t *testing.T) {
	units := &fakeUnits{state: `{"state":"done","disk":"/dev/sdb","error":"","time":"x"}`}
	m := newManager(t, units, false)
	err := m.Start(Request{Disk: "/dev/sdb", Size: 2000398934016, Serial: "S2", Name: "Giochi", Folder: "Giochi", EraseWindows: true})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(m.RequestFile)
	want := "OWNEET_PREPARE_DISK=/dev/sdb\nOWNEET_PREPARE_SIZE=2000398934016\nOWNEET_PREPARE_SERIAL=S2\n" +
		"OWNEET_PREPARE_ERASE=/dev/sdb\nOWNEET_PREPARE_NAME=Giochi\nOWNEET_PREPARE_FOLDER=Giochi\nOWNEET_PREPARE_ERASE_WINDOWS=yes\n"
	if string(data) != want {
		t.Errorf("request:\n%s\nwant:\n%s", data, want)
	}
	if len(units.started) != 1 || units.started[0] != Unit {
		t.Errorf("units: %v", units.started)
	}
	for i := 0; i < 200 && m.Status().State != "done"; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if st := m.Status(); st.State != "done" || st.Disk != "/dev/sdb" {
		t.Errorf("status %+v", st)
	}
	// the disk keeps its own name, and a USB disk without anything on it is fine too
	m = newManager(t, &fakeUnits{}, false)
	m.Timeout = 50 * time.Millisecond
	if err := m.Start(Request{Disk: "/dev/sdb", Size: 2000398934016, Serial: "S2", Name: "VOLUME", Folder: "Games", EraseWindows: true}); err != nil {
		t.Errorf("its own name: %v", err)
	}
}

func TestPrepareRefuses(t *testing.T) {
	sdb := Request{Disk: "/dev/sdb", Size: 2000398934016, Serial: "S2", Name: "Giochi", Folder: "Giochi", EraseWindows: true}
	sdd := Request{Disk: "/dev/sdd", Size: 1000000000000, Serial: "S4", Name: "Giochi", Folder: "Games"}
	change := func(r Request, f func(*Request)) Request { f(&r); return r }
	for name, c := range map[string]struct {
		req  Request
		live bool
		want error
	}{
		"from the USB stick":      {sdb, true, ErrLive},
		"the console's own disk":  {Request{Disk: "/dev/sda", Size: 96000000000, Serial: "S1", Name: "G", Folder: "Games"}, false, ErrNotAllowed},
		"the OWNEETLOG stick":     {Request{Disk: "/dev/sdc", Size: 62000000000, Serial: "S3", Name: "G", Folder: "Games"}, false, ErrNotAllowed},
		"another serial":          {change(sdd, func(r *Request) { r.Serial = "S9" }), false, ErrDiskChanged},
		"unknown disk":            {change(sdd, func(r *Request) { r.Disk = "/dev/sdz" }), false, ErrDiskChanged},
		"Windows without consent": {change(sdb, func(r *Request) { r.EraseWindows = false }), false, ErrWindowsNotConfirmed},
		"another disk keeps it":   {change(sdd, func(r *Request) { r.Name = "volume" }), false, ErrNameTaken},
		"empty":                   {change(sdd, func(r *Request) { r.Name = "" }), false, ErrBadName},
		"17 bytes":                {change(sdd, func(r *Request) { r.Name = "Giochi giochi gio" }), false, ErrBadName},
		"a slash":                 {change(sdd, func(r *Request) { r.Name = "a/b" }), false, ErrBadName},
		"a dot first":             {change(sdd, func(r *Request) { r.Name = ".hidden" }), false, ErrBadName},
		"a new line":              {change(sdd, func(r *Request) { r.Name = "a\nb" }), false, ErrBadName},
		"spaces around":           {change(sdd, func(r *Request) { r.Name = " Giochi" }), false, ErrBadName},
		"unknown folder":          {change(sdd, func(r *Request) { r.Folder = "/etc" }), false, ErrBadName},
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

func TestCheckName(t *testing.T) {
	for _, ok := range []string{"Giochi", "Games 2", "Città", "1234567890123456", "Giochi di Marco"} {
		if CheckName(ok) != nil {
			t.Errorf("CheckName(%q) refused", ok)
		}
	}
}

func TestBusy(t *testing.T) {
	m := newManager(t, &fakeUnits{}, false)
	m.Timeout = 100 * time.Millisecond
	req := Request{Disk: "/dev/sdd", Size: 1000000000000, Serial: "S4", Name: "Giochi", Folder: "Games"}
	if err := m.Start(req); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(req); !errors.Is(err, ErrBusy) {
		t.Errorf("second start: %v", err)
	}
	for i := 0; i < 50 && m.Status().State != "failed"; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if st := m.Status(); st.State != "failed" || st.Error != "failed" {
		t.Errorf("the unit never wrote: %+v", st)
	}
}
