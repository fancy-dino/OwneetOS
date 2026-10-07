// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"testing"

	"github.com/fancy-dino/OwneetOS/daemon/internal/power"
)

type fakePower struct{ done []string }

func (f *fakePower) Status() power.Status { return power.Status{CanShutdown: true, CanRestart: true} }
func (f *fakePower) Do(a power.Action) error {
	if a == power.Suspend {
		return power.ErrUnsupported
	}
	f.done = append(f.done, a.Name)
	return nil
}

func TestPowerEndpoints(t *testing.T) {
	s, c, _ := start(t)
	if code, errCode := post(t, c, "/v1/power/shutdown", ``); code != 503 || errCode != "power.unavailable" {
		t.Fatalf("without logind: %d %s", code, errCode)
	}
	fake := &fakePower{}
	s.Power = fake
	if code, _ := post(t, c, "/v1/power/restart", ``); code != 202 {
		t.Fatalf("restart: %d", code)
	}
	if code, _ := post(t, c, "/v1/power/shutdown", ``); code != 202 {
		t.Fatalf("shutdown: %d", code)
	}
	if code, errCode := post(t, c, "/v1/power/suspend", ``); code != 409 || errCode != "power.unsupported" {
		t.Fatalf("suspend: %d %s", code, errCode)
	}
	if code, _ := post(t, c, "/v1/power/hibernate", ``); code != 404 {
		t.Fatalf("hibernate: %d", code)
	}
	if len(fake.done) != 2 || fake.done[0] != "restart" || fake.done[1] != "shutdown" {
		t.Fatalf("done = %v", fake.done)
	}
}
