package main

import (
	"context"
	"errors"
	"testing"
)

func TestPauseServiceForMigrationPlatforms(t *testing.T) {
	for _, platform := range []string{"linux", "darwin", "windows", "freebsd"} {
		t.Run(platform, func(t *testing.T) {
			inspected, stopped, checked := 0, 0, 0
			paused, err := pauseServiceForMigrationWith(platform, func(context.Context) (helperAgentStatus, error) {
				inspected++
				return helperAgentStatus{Supported: true, Installed: true, Running: true}, nil
			}, func(context.Context) error { stopped++; return nil }, func() error { checked++; return nil })
			managed := platform == "linux" || platform == "darwin"
			if err != nil || paused != managed {
				t.Fatalf("paused=%v err=%v", paused, err)
			}
			if managed && (inspected != 1 || stopped != 1 || checked != 0) {
				t.Fatalf("calls %d/%d/%d", inspected, stopped, checked)
			}
			if !managed && (inspected != 0 || stopped != 0 || checked != 1) {
				t.Fatalf("calls %d/%d/%d", inspected, stopped, checked)
			}
		})
	}
}
func TestPauseServiceForMigrationFailsClosed(t *testing.T) {
	sentinel := errors.New("service failed")
	for _, tc := range []struct {
		name                         string
		status                       helperAgentStatus
		inspectErr, stopErr, portErr error
		wantPaused                   bool
		wantErr                      error
		wantStop                     int
	}{
		{name: "inspect", inspectErr: sentinel, wantErr: sentinel},
		{name: "stop", status: helperAgentStatus{Running: true}, stopErr: sentinel, wantErr: sentinel, wantStop: 1},
		{name: "inactive-port-clear", status: helperAgentStatus{Installed: true}},
		{name: "inactive-port-busy", status: helperAgentStatus{Installed: true}, portErr: sentinel, wantErr: sentinel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stops := 0
			paused, err := pauseServiceForMigrationWith("linux", func(context.Context) (helperAgentStatus, error) { return tc.status, tc.inspectErr }, func(context.Context) error { stops++; return tc.stopErr }, func() error { return tc.portErr })
			if paused != tc.wantPaused || !errors.Is(err, tc.wantErr) || stops != tc.wantStop {
				t.Fatalf("paused=%v err=%v stops=%d", paused, err, stops)
			}
		})
	}
}

func TestPauseServiceWithoutAgentNeedsNoUserBus(t *testing.T) {
	inspected, stopped, checked := 0, 0, 0
	paused, err := pauseServiceForMigrationWith("linux", func(context.Context) (helperAgentStatus, error) {
		inspected++
		return helperAgentStatus{Supported: true}, nil
	}, func(context.Context) error { stopped++; return errors.New("no bus") }, func() error { checked++; return nil })
	if err != nil || paused || inspected != 1 || stopped != 0 || checked != 1 {
		t.Fatalf("paused=%v err=%v calls=%d/%d/%d", paused, err, inspected, stopped, checked)
	}
}
