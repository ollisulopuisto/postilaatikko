//go:build linux

package main

import (
	"context"
	"errors"

	"github.com/ollisulopuisto/postilaatikko/internal/alwayson"
)

// Linux service management is added in the subsequent INSTALL and CONTROL tasks.
// Keep these operations explicitly unsupported until those slices land.
func installAgent(_ context.Context, _, _, _, _, _, _ string, _ []string, _ bool, _ apiListenerConfig, _ alwayson.GmailPushConfig) error {
	return errors.New("Linux helper installation is not implemented yet; run the helper under your system's service manager")
}

func getAgentStatus(_ context.Context) (helperAgentStatus, error) {
	return helperAgentStatus{}, nil
}

func startAgent(_ context.Context) error {
	return errors.New("Linux helper management is not implemented yet; use your system's service manager")
}

func stopAgent(_ context.Context) error {
	return errors.New("Linux helper management is not implemented yet; use your system's service manager")
}

func uninstallAgent(_ context.Context) error {
	return errors.New("Linux helper removal is not implemented yet")
}
