// SPDX-License-Identifier: GPL-3.0-or-later

// Package installer drives the installation of OwneetOS from the live USB stick
// ([[step-6.4-installer-ui]]) with the backend of [[step-6.3-installer-backend]]: the internal
// disks (owneet-install disks, any user), the player's request written to
// /run/owneet-install/request (the console user's folder; root reads it as plain text, key by key),
// and the root unit owneet-install.service, which polkit lets the console user start. Its progress
// comes from /run/owneet/install.json. Nothing is erased without the player's confirmation of that
// very disk, and of Windows or BitLocker on it ([[user-data]]).
package installer

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

	"github.com/fancy-dino/OwneetOS/daemon/internal/diskprep"
	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// Unit is the root unit that installs on the disk of the request.
const Unit = "owneet-install.service"

// DefaultName is the console's name when the player keeps it.
const DefaultName = "OwneetOS"

// Partition is a partition of a disk, as owneet-install lists it.
type Partition struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	FSType string `json:"fstype"`
	Label  string `json:"label"`
}

// Disk is an internal disk, as owneet-install lists it (never the USB stick, never USB disks).
type Disk struct {
	Path       string      `json:"path"`
	Serial     string      `json:"serial"`
	Size       int64       `json:"size"` // bytes
	Model      string      `json:"model"`
	Usable     bool        `json:"usable"`
	Reason     string      `json:"reason"` // why not usable: too_small
	Windows    bool        `json:"windows"`
	BitLocker  bool        `json:"bitlocker"`
	Linux      bool        `json:"linux"`
	Partitions []Partition `json:"partitions"`
}

// Status is the body of GET /v1/installer.
type Status struct {
	// State: "" (nothing started), preparing, partitioning, copying, setting_up, boot, games, done, failed
	State   string `json:"state"`
	Percent int    `json:"percent"`
	// Error (when failed): the backend's code (copy_failed, not_uefi, …), shown in the player's words
	Error string `json:"error,omitempty"`
}

// Request is what the player chose: the disk as it was listed (path, size, serial), the console's
// name, and the confirmation that Windows or BitLocker on it may be erased; the other internal disks
// to prepare for games, each named ([[step-6.7-preparing-other-disks]]), and the Games folder's name.
type Request struct {
	Disk         string      `json:"disk"`
	Size         int64       `json:"size"`
	Serial       string      `json:"serial"`
	Name         string      `json:"name"`
	EraseWindows bool        `json:"erase_windows"`
	Games        []GamesDisk `json:"games"`
	Folder       string      `json:"folder"`
}

// GamesDisk is another internal disk to erase and prepare for games, as it was listed.
type GamesDisk struct {
	Disk         string `json:"disk"`
	Size         int64  `json:"size"`
	Serial       string `json:"serial"`
	Name         string `json:"name"`
	EraseWindows bool   `json:"erase_windows"`
}

// MaxGamesDisks is how many other disks one installation prepares at most.
const MaxGamesDisks = 8

// Errors returned to the API.
var (
	ErrNotLive             = errors.New("OwneetOS is installed only from its USB stick")
	ErrBusy                = errors.New("an installation is already running")
	ErrDiskChanged         = errors.New("the disk is not the one that was listed")
	ErrNotUsable           = errors.New("the disk cannot hold OwneetOS")
	ErrWindowsNotConfirmed = errors.New("erasing Windows or BitLocker was not confirmed")
	ErrBadName             = diskprep.ErrBadName
	ErrNameTaken           = diskprep.ErrNameTaken
)

// Starter starts a systemd unit of the system manager.
type Starter interface {
	Start(unit string) error
}

// Manager lists the disks and runs the installation.
type Manager struct {
	Broker      *events.Broker
	Log         *slog.Logger
	Units       Starter
	Command     string      // /usr/lib/owneet/owneet-install
	RequestFile string      // /run/owneet-install/request
	StateFile   string      // /run/owneet/install.json
	Live        func() bool // started from the USB stick
	Poll        time.Duration
	Timeout     time.Duration
	// DisksJSON runs "owneet-install disks" (tests replace it)
	DisksJSON func() ([]byte, error)

	mu      sync.Mutex
	running bool
	started time.Time
	gaveUp  bool // the unit never wrote its end in time: failed
}

// Disks lists the internal disks.
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

// Status tells how the installation goes. A state written before this installation started (a
// failed one before "Try again") is not this one's.
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
	}
	return st
}

// CleanName makes the console's name safe for the request: one line of printable characters, at
// most 40, "OwneetOS" when empty.
func CleanName(name string) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.TrimSpace(name) {
		if !unicode.IsPrint(r) || n == 40 {
			continue
		}
		b.WriteRune(r)
		n++
	}
	if clean := strings.TrimSpace(b.String()); clean != "" {
		return clean
	}
	return DefaultName
}

// Start installs on the disk the player chose and confirmed; "installer.changed" follows it.
func (m *Manager) Start(req Request) error {
	if m.Live != nil && !m.Live() {
		return ErrNotLive
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
	m.Log.Info("installation started", "disk", req.Disk)
	m.Broker.Publish("installer.changed", m.Status())
	go m.watch()
	return nil
}

func (m *Manager) start(req Request) error {
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
	if !disk.Usable {
		return ErrNotUsable
	}
	if (disk.Windows || disk.BitLocker) && !req.EraseWindows {
		return ErrWindowsNotConfirmed
	}
	lines := []string{
		"OWNEET_INSTALL_DISK=" + disk.Path,
		"OWNEET_INSTALL_SIZE=" + strconv.FormatInt(disk.Size, 10),
		"OWNEET_INSTALL_SERIAL=" + disk.Serial,
		"OWNEET_INSTALL_NAME=" + CleanName(req.Name),
		// the player confirmed that this very disk will be erased (the final hold)
		"OWNEET_INSTALL_ERASE=" + disk.Path,
	}
	if disk.Windows || disk.BitLocker {
		lines = append(lines, "OWNEET_INSTALL_ERASE_WINDOWS=yes")
	}
	games, err := gamesLines(req, disks)
	if err != nil {
		return err
	}
	lines = append(lines, games...)
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

// gamesLines checks the disks for games as the backend will (each one listed as it was, not the
// system's disk, confirmed, named with a name no other disk keeps) and gives their request lines.
func gamesLines(req Request, disks []Disk) ([]string, error) {
	if len(req.Games) == 0 {
		return nil, nil
	}
	if len(req.Games) > MaxGamesDisks {
		return nil, ErrDiskChanged
	}
	known := false
	for _, f := range diskprep.Folders {
		known = known || f == req.Folder
	}
	if !known {
		return nil, ErrBadName
	}
	// the names other disks keep: every disk but the erased ones
	listed := make([]diskprep.Disk, 0, len(disks))
	for _, d := range disks {
		parts := make([]diskprep.Partition, 0, len(d.Partitions))
		for _, p := range d.Partitions {
			parts = append(parts, diskprep.Partition{Path: p.Path, Label: p.Label})
		}
		listed = append(listed, diskprep.Disk{Path: d.Path, Partitions: parts})
	}
	erased := []string{req.Disk}
	for _, g := range req.Games {
		erased = append(erased, g.Disk)
	}
	var lines []string
	seen := map[string]bool{}
	for i, g := range req.Games {
		var disk *Disk
		for j := range disks {
			if disks[j].Path == g.Disk {
				disk = &disks[j]
			}
		}
		if disk == nil || disk.Path == req.Disk || seen[disk.Path] || disk.Size != g.Size || disk.Serial != g.Serial {
			return nil, ErrDiskChanged
		}
		seen[disk.Path] = true
		if (disk.Windows || disk.BitLocker) && !g.EraseWindows {
			return nil, ErrWindowsNotConfirmed
		}
		if err := diskprep.CheckName(g.Name); err != nil {
			return nil, err
		}
		for _, other := range req.Games[:i] {
			if strings.EqualFold(other.Name, g.Name) {
				return nil, ErrNameTaken
			}
		}
		if diskprep.NameTaken(g.Name, listed, erased...) {
			return nil, ErrNameTaken
		}
		n := "OWNEET_INSTALL_GAMES_" + strconv.Itoa(i+1) + "_"
		lines = append(lines, n+"DISK="+disk.Path, n+"SIZE="+strconv.FormatInt(disk.Size, 10), n+"SERIAL="+disk.Serial,
			n+"ERASE="+disk.Path, n+"NAME="+g.Name)
		if disk.Windows || disk.BitLocker {
			lines = append(lines, n+"ERASE_WINDOWS=yes")
		}
	}
	return append(lines, "OWNEET_INSTALL_GAMES_FOLDER="+req.Folder), nil
}

// watch follows the unit's progress until it is done or failed.
func (m *Manager) watch() {
	poll, timeout := m.Poll, m.Timeout
	if poll == 0 {
		poll = time.Second
	}
	if timeout == 0 {
		timeout = 61 * time.Minute // the unit gives up after 60
	}
	deadline := time.Now().Add(timeout)
	last := Status{State: "preparing"}
	for time.Now().Before(deadline) {
		time.Sleep(poll)
		st := m.Status()
		if st != last {
			m.Broker.Publish("installer.changed", st)
			last = st
		}
		if st.State == "done" || st.State == "failed" {
			m.finish(false)
			return
		}
	}
	m.finish(true)
}

// finish ends the installation; gaveUp: the unit never wrote its end in time (it did not run, or
// it was stopped).
func (m *Manager) finish(gaveUp bool) {
	m.mu.Lock()
	m.running = false
	m.gaveUp = gaveUp
	m.mu.Unlock()
	st := m.Status()
	m.Log.Info("installation finished", "state", st.State, "error", st.Error)
	m.Broker.Publish("installer.changed", st)
}
