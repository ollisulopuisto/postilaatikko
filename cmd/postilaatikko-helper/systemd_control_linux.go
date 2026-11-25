//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

func getAgentStatus(ctx context.Context) (helperAgentStatus, error) {
	status := helperAgentStatus{Supported: true}
	path, err := systemdUnitPath()
	if err != nil {
		return status, err
	}
	info, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return status, err
	}
	if err == nil {
		if !info.Mode().IsRegular() {
			return status, errors.New("helper user unit is not a regular file")
		}
		status.Installed = true
	}
	if !status.Installed {
		return status, nil
	}
	enabled, err := runSystemctl(ctx, "--user", "is-enabled", systemdServiceName)
	if err != nil {
		state := strings.TrimSpace(string(enabled))
		if !(systemctlExited(err, 1) && state == "disabled") && !(systemctlExited(err, 4) && state == "not-found") {
			return status, fmt.Errorf("inspect helper enable state: %s: %w", enabled, err)
		}
	}
	active, err := runSystemctl(ctx, "--user", "is-active", systemdServiceName)
	state := strings.TrimSpace(string(active))
	if err != nil {
		if systemctlExited(err, 3) && (state == "inactive" || state == "failed" || state == "activating" || state == "deactivating") {
			status.Running = state == "activating" || state == "deactivating"
			return status, nil
		}
		return status, fmt.Errorf("inspect helper running state: %s: %w", active, err)
	}
	status.Running = state == "active" || state == "reloading"
	return status, nil
}
func systemctlExited(err error, code int) bool {
	var exit interface{ ExitCode() int }
	return errors.As(err, &exit) && exit.ExitCode() == code
}
func startAgent(ctx context.Context) error {
	status, err := getAgentStatus(ctx)
	if err != nil {
		return err
	}
	if !status.Installed {
		return errors.New("helper user service is not installed")
	}
	if status.Running {
		return nil
	}
	if output, err := runSystemctl(ctx, "--user", "start", systemdServiceName); err != nil {
		return fmt.Errorf("start helper user service: %s: %w", output, err)
	}
	return nil
}
func stopAgent(ctx context.Context) error {
	status, err := getAgentStatus(ctx)
	if err != nil {
		return err
	}
	if !status.Running {
		return nil
	}
	if output, err := runSystemctl(ctx, "--user", "stop", systemdServiceName); err != nil {
		return fmt.Errorf("stop helper user service: %s: %w", output, err)
	}
	return nil
}
