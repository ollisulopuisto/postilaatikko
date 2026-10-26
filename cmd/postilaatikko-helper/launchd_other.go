//go:build !darwin

package main

import (
	"context"
	"errors"
)

func installAgent(_ context.Context, _, _, _, _, _, _ string) error {
	return errors.New("login-agent installation is available on macOS; run the helper under your system's service manager")
}

func uninstallAgent(_ context.Context) error {
	return errors.New("login-agent removal is available on macOS")
}
