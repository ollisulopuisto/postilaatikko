//go:build linux

package main

import (
	"context"
	"errors"
	"testing"
)

func TestLinuxBootstrapPauseWithoutUnitOrUserBus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	old := runSystemctl
	t.Cleanup(func() { runSystemctl = old })
	calls := 0
	runSystemctl = func(context.Context, ...string) ([]byte, error) {
		calls++
		return nil, errors.New("Failed to connect to bus: No medium found")
	}
	portChecks := 0
	paused, err := pauseServiceForMigrationWith("linux", getAgentStatus, stopAgent, func() error { portChecks++; return nil })
	if err != nil || paused || calls != 0 || portChecks != 1 {
		t.Fatalf("paused=%v err=%v systemctl=%d port=%d", paused, err, calls, portChecks)
	}
}
