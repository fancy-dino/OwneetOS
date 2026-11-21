// SPDX-License-Identifier: GPL-3.0-or-later

// Package diskprep prepares a disk other than the console's own for games, from Settings → Disks
// ([[step-6.7-preparing-other-disks]]): the disks (owneet-prepare-disk disks, any user), the player's
// request written to /run/owneet-prepare/request (the console user's folder; root reads it as plain
// text, key by key), and the root unit owneet-prepare-disk.service, which polkit lets the console user
// start. Its progress comes from /run/owneet/prepare.json. Nothing is erased without the player's
// confirmation of that very disk, and of Windows or BitLocker on it ([[user-data]]). The disk gets the
// name the player chose: ext4's label, never another disk's name. From the USB stick only the
// installer prepares disks.
package diskprep

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// Unit is the root unit that prepares the disk of the request.
const Unit = "owneet-prepare-disk.service"

// MaxNameBytes is the longest name a disk can get: ext4's label.
const MaxNameBytes = 16

// Folders are the names the Games folder may get, by the console's language ([[games-on-disk]]).
var Folders = []string{"Games", "Giochi"}

// Partition is a partition of a disk, as owneet-prepare-disk lists it.
type Partition struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	FSType     string `json:"fstype"`
	Label      string `json:"label"`
	Mountpoint string `json:"mountpoint"`
}

// Disk is a disk of the PC, as owneet-prepare-disk lists it. Console (the console's own disk or the
// USB stick it started from) and OwneetLog (the testers' stick) can never be prepared.
type Disk struct {
	Path       string      `json:"path"`
	Serial     string      `json:"serial"`
	Size       int64       `json:"size"` // bytes
	Model      string      `json:"model"`
	USB        bool        `json:"usb"`
	Console    bool        `json:"console"`
	OwneetLog  bool        `json:"owneetlog"`
	Windows    bool        `json:"windows"`
	BitLocker  bool        `json:"bitlocker"`
	Linux      bool        `json:"linux"`
	Partitions []Partition `json:"partitions"`
}

// Status is the body of GET /v1/disks/prepare.
type Status struct {
	// State: "" (nothing started), preparing, erasing, formatting, done, failed
	State string `json:"state"`
	Disk  string `json:"disk,omitempty"`
	// Error (when failed): the backend's code (in_use, format_failed, …), shown in the player's words
	Error string `json:"error,omitempty"`
}

// Request is what the player chose: the disk as it was listed (path, size, serial), its new name,
// the Games folder's name, and the confirmation that Windows or BitLocker on it may be erased.
type Request struct {
	Disk         string `json:"disk"`
	Size         int64  `json:"size"`
	Serial       string `json:"serial"`
	Name         string `json:"name"`
	Folder       string `json:"folder"`
	EraseWindows bool   `json:"erase_windows"`
}

// Errors returned to the API.
var (
	ErrLive                = errors.New("from the USB stick, disks are prepared by the installer")
	ErrBusy                = errors.New("a disk is already being prepared")
	ErrDiskChanged         = errors.New("the disk is not the one that was listed")
	ErrNotAllowed          = errors.New("this disk cannot be prepared (the console's own disk or its USB stick)")
	ErrWindowsNotConfirmed = errors.New("erasing Windows or BitLocker was not confirmed")
	ErrBadName             = errors.New("the name is empty, longer than 16 bytes, or has a \"/\"")
	ErrNameTaken           = errors.New("another disk has this name")
)

// CheckName tells whether a disk may get this name: one line of printable characters, no "/",
// not starting with ".", 1 to 16 bytes (ext4's label; accented letters take two).
func CheckName(name string) error {
	if name == "" || len(name) > MaxNameBytes || strings.Contains(name, "/") || strings.HasPrefix(name, ".") ||
		strings.TrimSpace(name) != name {
		return ErrBadName
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return ErrBadName
		}
	}
	return nil
}

// NameTaken tells whether a file system of another disk has this name, letters in any case; the disks
// in skip are left out (they are erased too).
func NameTaken(name string, disks []Disk, skip ...string) bool {
	for _, d := range disks {
		left := false
		for _, s := range skip {
			left = left || s == d.Path
		}
		if left {
			continue
		}
		for _, p := range d.Partitions {
			if p.Label != "" && strings.EqualFold(p.Label, name) {
				return true
			}
		}
	}
	return false
}

// Starter starts a systemd unit of the system manager.
type Starter interface {
	Start(unit string) error
}

// Manager lists the disks and prepares one.
type Manager struct {
	Broker      *events.Broker
	Log         *slog.Logger
	Units       Starter
	Command     string      // /usr/lib/owneet/owneet-prepare-disk
	RequestFile string      // /run/owneet-prepare/request
	StateFile   string      // /run/owneet/prepare.json
	Live        func() bool // started from the USB stick
	Poll        time.Duration
	Timeout     time.Duration
	// DisksJSON runs "owneet-prepare-disk disks" (tests replace it)
	DisksJSON func() ([]byte, error)

	mu      sync.Mutex
	running bool
	started time.Time
	gaveUp  bool
}

// Disks lists the disks of the PC.
func (m *Manager) Disks() ([]Disk, error) {
	run := m.DisksJSON
	if run == nil {
		run = func() ([]byte, error) { return exec.Command(m.Command, "disks").Output() }
	}
	out, err := run()
	if err != nil {
		return nil, fmt.Errorf("listing the disks: %w", err)
	}
	var list struct {
		Disks []Disk `json:"disks"`
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("listing the disks: %w", err)
	}
	if list.Disks == nil {
		list.Disks = []Disk{}
	}
	return list.Disks, nil
}

func (m *Manager) read() (Status, time.Time) {
	var st Status
	info, err := os.Stat(m.StateFile)
	if err != nil {
		return st, time.Time{}
	}
	if data, err := os.ReadFile(m.StateFile); err == nil {
		_ = json.Unmarshal(data, &st)
	}
	if st.State != "failed" {
		st.Error = ""
	}
	return st, info.ModTime()
}

// Status tells how the preparation goes; a state written before this one started is not this one's.
func (m *Manager) Status() Status {
	st, written := m.read()
	m.mu.Lock()
	defer m.mu.Unlock()
	if !written.After(m.started) {
		if m.running {
			return Status{State: "preparing"}
		}
		if m.gaveUp {
			return Status{State: "failed", Error: "failed"}
		}
		if !m.started.IsZero() {
			return Status{}
		}
	}
	return st
}

// Start prepares the disk the player chose, named and confirmed; "disks.prepare.changed" follows it.
func (m *Manager) Start(req Request) error {
	if m.Live != nil && m.Live() {
		return ErrLive
	}
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return ErrBusy
	}
	m.running = true
	m.gaveUp = false
	m.started = time.Now()
	m.mu.Unlock()
	if err := m.start(req); err != nil {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
		return err
	}
	m.Log.Info("preparing a disk for games", "disk", req.Disk, "name", req.Name)
	m.Broker.Publish("disks.prepare.changed", m.Status())
	go m.watch()
	return nil
}

func (m *Manager) start(req Request) error {
	if err := CheckName(req.Name); err != nil {
		return err
	}
	known := false
	for _, f := range Folders {
		known = known || f == req.Folder
	}
	if !known {
		return ErrBadName
	}
	disks, err := m.Disks()
	if err != nil {
		return err
	}
	var disk *Disk
	for i := range disks {
		if disks[i].Path == req.Disk {
			disk = &disks[i]
		}
	}
	// the same disk the player saw: path, size and serial (the backend checks them again)
	if disk == nil || disk.Size != req.Size || disk.Serial != req.Serial {
		return ErrDiskChanged
	}
	if disk.Console || disk.OwneetLog {
		return ErrNotAllowed
	}
	if (disk.Windows || disk.BitLocker) && !req.EraseWindows {
		return ErrWindowsNotConfirmed
	}
	if NameTaken(req.Name, disks, disk.Path) {
		return ErrNameTaken
	}
	lines := []string{
		"OWNEET_PREPARE_DISK=" + disk.Path,
		"OWNEET_PREPARE_SIZE=" + strconv.FormatInt(disk.Size, 10),
		"OWNEET_PREPARE_SERIAL=" + disk.Serial,
		// the player confirmed that this very disk will be erased (the final hold)
		"OWNEET_PREPARE_ERASE=" + disk.Path,
		"OWNEET_PREPARE_NAME=" + req.Name,
		"OWNEET_PREPARE_FOLDER=" + req.Folder,
	}
	if disk.Windows || disk.BitLocker {
		lines = append(lines, "OWNEET_PREPARE_ERASE_WINDOWS=yes")
	}
	for _, l := range lines {
		if strings.ContainsAny(l, "\n\r") {
			return ErrDiskChanged
		}
	}
	tmp := m.RequestFile + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing the request: %w", err)
	}
	if err := os.Rename(tmp, m.RequestFile); err != nil {
		return fmt.Errorf("writing the request: %w", err)
	}
	return m.Units.Start(Unit)
}

// watch follows the unit's progress until it is done or failed.
func (m *Manager) watch() {
	poll, timeout := m.Poll, m.Timeout
	if poll == 0 {
		poll = time.Second
	}
	if timeout == 0 {
		timeout = 31 * time.Minute // the unit gives up after 30
	}
	deadline := time.Now().Add(timeout)
	last := Status{State: "preparing"}
	for time.Now().Before(deadline) {
		time.Sleep(poll)
		st := m.Status()
		if st != last {
			m.Broker.Publish("disks.prepare.changed", st)
			last = st
		}
		if st.State == "done" || st.State == "failed" {
			m.finish(false)
			return
		}
	}
	m.finish(true)
}

func (m *Manager) finish(gaveUp bool) {
	m.mu.Lock()
	m.running = false
	m.gaveUp = gaveUp
	m.mu.Unlock()
	st := m.Status()
	m.Log.Info("disk preparation finished", "state", st.State, "error", st.Error)
	m.Broker.Publish("disks.prepare.changed", st)
}
