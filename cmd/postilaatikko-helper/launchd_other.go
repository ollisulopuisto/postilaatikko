//go:build !darwin

package main

import (
	"context"
	"errors"

	"github.com/hkdb/aerion/internal/alwayson"
)

func installAgent(_ context.Context, _, _, _, _, _, _ string, _ bool, _ apiListenerConfig, _ alwayson.GmailPushConfig) error {
	return errors.New("login-agent installation is available on macOS; run the helper under your system's service manager")
}

func getAgentStatus(_ context.Context) (helperAgentStatus, error) {
	return helperAgentStatus{}, nil
}

func startAgent(_ context.Context) error {
	return errors.New("login-agent management is available on macOS; use your system's service manager")
}

func stopAgent(_ context.Context) error {
	return errors.New("login-agent management is available on macOS; use your system's service manager")
}

func uninstallAgent(_ context.Context) error {
	return errors.New("login-agent removal is available on macOS")
}
