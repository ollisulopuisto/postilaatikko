//go:build darwin

package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const agentLabel = "fi.postilaatikko.send-later"

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", agentLabel+".plist"), nil
}

func installAgent(ctx context.Context, identity, data, queue, listen, cert, key string) error {
	if err := validateListener(listen, cert, key); err != nil {
		return err
	}
	path, err := launchAgentPath()
	if err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return errors.New("Send Later login agent already exists; stop/uninstall it before changing its configuration")
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err = filepath.EvalSymlinks(binary)
	if err != nil {
		return err
	}
	arguments := []string{binary, "serve", "--identity", identity, "--data", data, "--queue", queue, "--listen", listen}
	if cert != "" {
		arguments = append(arguments, "--tls-cert", cert, "--tls-key", key)
	}
	var document bytes.Buffer
	document.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\"><dict>\n")
	document.WriteString("<key>Label</key><string>" + agentLabel + "</string>\n<key>ProgramArguments</key><array>\n")
	for _, argument := range arguments {
		document.WriteString("<string>")
		if err := xml.EscapeText(&document, []byte(argument)); err != nil {
			return err
		}
		document.WriteString("</string>\n")
	}
	document.WriteString("</array>\n<key>RunAtLoad</key><true/>\n<key>KeepAlive</key><true/>\n<key>ThrottleInterval</key><integer>30</integer>\n</dict></plist>\n")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(document.Bytes()); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	commandContext, done := context.WithTimeout(ctx, 10*time.Second)
	defer done()
	output, err := exec.CommandContext(commandContext, "launchctl", "bootstrap", "gui/"+strconv.Itoa(os.Getuid()), path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launch agent saved but bootstrap failed: %s: %w", string(output), err)
	}
	return nil
}

func uninstallAgent(ctx context.Context) error {
	path, err := launchAgentPath()
	if err != nil {
		return err
	}
	commandContext, done := context.WithTimeout(ctx, 10*time.Second)
	defer done()
	service := "gui/" + strconv.Itoa(os.Getuid()) + "/" + agentLabel
	if err := exec.CommandContext(commandContext, "launchctl", "print", service).Run(); err == nil {
		output, err := exec.CommandContext(commandContext, "launchctl", "bootout", service).CombinedOutput()
		if err != nil {
			return fmt.Errorf("stop helper agent before removing its configuration: %s: %w", string(output), err)
		}
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
