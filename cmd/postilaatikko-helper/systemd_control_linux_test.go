//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type systemctlExit int

func (e systemctlExit) Error() string { return "systemctl exit" }
func (e systemctlExit) ExitCode() int { return int(e) }
func setupControl(t *testing.T, installed bool, enabled, active string) *[][]string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	if installed {
		path, _ := systemdUnitPath()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("unit"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var calls [][]string
	old := runSystemctl
	t.Cleanup(func() { runSystemctl = old })
	runSystemctl = func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		switch args[1] {
		case "is-enabled":
			if enabled == "disabled" {
				return []byte(enabled), systemctlExit(1)
			}
			if enabled == "not-found" {
				return []byte(enabled), systemctlExit(4)
			}
			return []byte(enabled), nil
		case "is-active":
			if active == "inactive" || active == "failed" || active == "activating" || active == "deactivating" {
				return []byte(active), systemctlExit(3)
			}
			return []byte(active), nil
		}
		return nil, nil
	}
	return &calls
}
func TestSystemdStatus(t *testing.T) {
	for _, tc := range []struct {
		name            string
		installed       bool
		enabled, active string
		want            helperAgentStatus
	}{
		{"active", true, "enabled", "active", helperAgentStatus{Supported: true, Installed: true, Running: true}},
		{"inactive", true, "disabled", "inactive", helperAgentStatus{Supported: true, Installed: true}},
		{"failed", true, "enabled", "failed", helperAgentStatus{Supported: true, Installed: true}},
		{"activating", true, "enabled", "activating", helperAgentStatus{Supported: true, Installed: true, Running: true}},
		{"missing", false, "not-found", "inactive", helperAgentStatus{Supported: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupControl(t, tc.installed, tc.enabled, tc.active)
			got, err := getAgentStatus(context.Background())
			if err != nil || got != tc.want {
				t.Fatalf("status=%+v err=%v", got, err)
			}
		})
	}
}
func TestSystemdControlCommands(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		calls := setupControl(t, true, "enabled", "inactive")
		if err := startAgent(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual((*calls)[len(*calls)-1], []string{"--user", "start", systemdServiceName}) {
			t.Fatalf("calls=%v", *calls)
		}
	})
	t.Run("stop", func(t *testing.T) {
		calls := setupControl(t, true, "enabled", "active")
		if err := stopAgent(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual((*calls)[len(*calls)-1], []string{"--user", "stop", systemdServiceName}) {
			t.Fatalf("calls=%v", *calls)
		}
	})
	t.Run("missing-start", func(t *testing.T) {
		setupControl(t, false, "not-found", "inactive")
		if err := startAgent(context.Background()); err == nil {
			t.Fatal("started uninstalled service")
		}
	})
}
func TestSystemdStatusCommandFailure(t *testing.T) {
	setupControl(t, true, "enabled", "active")
	runSystemctl = func(context.Context, ...string) ([]byte, error) { return nil, errors.New("no user bus") }
	if _, err := getAgentStatus(context.Background()); err == nil {
		t.Fatal("hid command failure")
	}
	if err := startAgent(context.Background()); err == nil {
		t.Fatal("hid start inspection failure")
	}
	if err := stopAgent(context.Background()); err == nil {
		t.Fatal("hid stop inspection failure")
	}
}
