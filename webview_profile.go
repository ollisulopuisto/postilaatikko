package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// runWails retains the pre-rename WebView2 profile for all window modes. Wails'
// default is APPDATA plus the executable filename, including the .exe suffix.
func runWails(appOptions *options.App) error {
	windowsOptions, err := stableWindowsOptions(runtime.GOOS, os.UserConfigDir)
	if err != nil {
		return err
	}
	appOptions.Windows = windowsOptions
	return wails.Run(appOptions)
}

func stableWindowsOptions(goos string, configDir func() (string, error)) (*windows.Options, error) {
	if goos != "windows" {
		return nil, nil
	}
	root, err := configDir()
	if err != nil {
		return nil, fmt.Errorf("resolve retained Windows browser profile: %w", err)
	}
	if root == "" {
		return nil, fmt.Errorf("retained Windows browser profile root is empty")
	}
	return &windows.Options{WebviewUserDataPath: filepath.Join(root, "eterno-mail.exe")}, nil
}
