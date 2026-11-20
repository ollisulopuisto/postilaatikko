package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetainedWindowsBrowserProfileForEveryMode(t *testing.T) {
	opts, err := stableWindowsOptions("windows", func() (string, error) { return "profile-root", nil })
	if err != nil || opts.WebviewUserDataPath != filepath.Join("profile-root", "eterno-mail.exe") {
		t.Fatalf("legacy profile lost: %#v %v", opts, err)
	}
	opts, err = stableWindowsOptions("linux", func() (string, error) { t.Fatal("non-Windows queried profile"); return "", nil })
	if err != nil || opts != nil {
		t.Fatal(opts, err)
	}
	for _, failure := range []func() (string, error){func() (string, error) { return "", errors.New("missing APPDATA") }, func() (string, error) { return "", nil }} {
		if _, err = stableWindowsOptions("windows", failure); err == nil {
			t.Fatal("profile failure silently fell back to a new profile")
		}
	}
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(source), "runWails(&options.App{") != 3 || strings.Contains(string(source), "wails.Run(") {
		t.Fatal("a window mode bypasses the retained profile")
	}
}
