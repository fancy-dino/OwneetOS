// SPDX-License-Identifier: GPL-3.0-or-later

package apps

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ReadSession reads the session written by owneet-session-app ($XDG_RUNTIME_DIR/owneet/session-env).
func ReadSession(path string) (Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, err
	}
	defer f.Close()
	var s Session
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		switch k {
		case "OWNEET_HOME_PID":
			s.HomePID, _ = strconv.Atoi(v)
			continue
		case "OWNEET_SESSION_MODE":
			s.Mode = v
		case "DISPLAY":
			s.Display = v
		}
		s.Env = append(s.Env, k+"="+v)
	}
	if s.Mode == "" || s.HomePID == 0 {
		return Session{}, errors.New("incomplete session file")
	}
	// The home screen must still be running: the file of a stopped session is stale.
	if _, err := os.Stat(filepath.Join("/proc", strconv.Itoa(s.HomePID))); err != nil {
		return Session{}, errors.New("the console session stopped")
	}
	return s, sc.Err()
}

// UnitOfPID returns the systemd unit a process runs in, from /proc/PID/cgroup.
func UnitOfPID(pid int) string {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if path, ok := strings.CutPrefix(line, "0::"); ok {
			for _, part := range strings.Split(path, "/") {
				if strings.HasPrefix(part, "owneet-app-") && strings.HasSuffix(part, ".service") {
					return part
				}
			}
		}
	}
	return ""
}
