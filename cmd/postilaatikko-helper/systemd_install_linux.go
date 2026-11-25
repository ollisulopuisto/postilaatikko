//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/alwayson"
)

const systemdServiceName = "postilaatikko-helper.service"

var runSystemctl = func(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "systemctl", args...).CombinedOutput()
}

func systemdUnitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", systemdServiceName), nil
}
func installAgent(ctx context.Context, identity, data, queue, listen, cert, key string, allowedHosts []string, withSync bool, apiConfig apiListenerConfig, gmailPush alwayson.GmailPushConfig) error {
	if err := validateListener(listen, cert, key, allowedHosts); err != nil {
		return err
	}
	if err := apiConfig.validate(); err != nil {
		return err
	}
	if gmailPush.ProjectID != "" || gmailPush.Topic != "" || gmailPush.Subscription != "" || gmailPush.CredentialsFile != "" {
		if !withSync {
			return errors.New("Gmail Pub/Sub requires mailbox sync")
		}
		if err := gmailPush.Validate(); err != nil {
			return err
		}
	}
	path, err := systemdUnitPath()
	if err != nil {
		return err
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err = filepath.EvalSymlinks(binary)
	if err != nil {
		return err
	}
	args := []string{"serve", "--identity", identity, "--data", data, "--queue", queue, "--listen", listen, "--api-listen", apiConfig.address}
	if withSync {
		args = append(args, "--sync")
	}
	if gmailPush.ProjectID != "" {
		args = append(args, "--gmail-pubsub-project", gmailPush.ProjectID, "--gmail-pubsub-topic", gmailPush.Topic, "--gmail-pubsub-subscription", gmailPush.Subscription)
		if gmailPush.CredentialsFile != "" {
			args = append(args, "--gmail-pubsub-credentials", gmailPush.CredentialsFile)
		}
	}
	if cert != "" {
		args = append(args, "--tls-cert", cert, "--tls-key", key)
	}
	for _, host := range allowedHosts {
		args = append(args, "--allowed-host", host)
	}
	if apiConfig.cert != "" {
		args = append(args, "--api-tls-cert", apiConfig.cert, "--api-tls-key", apiConfig.key)
	}
	for _, host := range apiConfig.allowedHost {
		args = append(args, "--api-allowed-host", host)
	}
	return installSystemdUnit(ctx, path, renderSystemdUnit(binary, args))
}
func installSystemdUnit(ctx context.Context, path, unit string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create helper user unit (uninstall existing configuration before changing it): %w", err)
	}
	saved := false
	defer func() {
		_ = file.Close()
		if !saved {
			_ = os.Remove(path)
		}
	}()
	if _, err = file.WriteString(unit); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	saved = true
	for _, args := range [][]string{{"--user", "daemon-reload"}, {"--user", "enable", "--now", systemdServiceName}} {
		if output, err := runSystemctl(ctx, args...); err != nil {
			return fmt.Errorf("helper unit saved but systemctl %v failed: %s: %w", args, output, err)
		}
	}
	return nil
}
func uninstallAgent(ctx context.Context) error {
	path, err := systemdUnitPath()
	if err != nil {
		return err
	}
	return uninstallSystemdUnit(ctx, path)
}
func uninstallSystemdUnit(ctx context.Context, path string) error {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if output, err := runSystemctl(ctx, "--user", "disable", "--now", systemdServiceName); err != nil {
		return fmt.Errorf("stop and disable helper before removing unit: %s: %w", output, err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if output, err := runSystemctl(ctx, "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("helper unit removed but daemon-reload failed: %s: %w", output, err)
	}
	return nil
}
