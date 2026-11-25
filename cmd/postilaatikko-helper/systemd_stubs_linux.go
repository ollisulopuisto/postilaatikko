//go:build linux

package main

import (
	"context"
	"errors"
)

// Linux service management is added in the subsequent INSTALL and CONTROL tasks.
// Keep these operations explicitly unsupported until those slices land.
func getAgentStatus(_ context.Context) (helperAgentStatus, error) {
	return helperAgentStatus{}, nil
}

func startAgent(_ context.Context) error {
	return errors.New("Linux helper management is not implemented yet; use your system's service manager")
}

func stopAgent(_ context.Context) error {
	return errors.New("Linux helper management is not implemented yet; use your system's service manager")
}
