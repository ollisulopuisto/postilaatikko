//go:build linux

package main

import (
	"context"
	"errors"
	"github.com/ollisulopuisto/postilaatikko/internal/alwayson"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fakeSystemctl(t *testing.T, failAt int) *[][]string {
	t.Helper()
	var calls [][]string
	old := runSystemctl
	t.Cleanup(func() { runSystemctl = old })
	runSystemctl = func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		if len(calls) == failAt {
			return []byte("unavailable"), errors.New("exit1")
		}
		return nil, nil
	}
	return &calls
}
func TestInstallSystemdUnit(t *testing.T) {
	calls := fakeSystemctl(t, 0)
	path := filepath.Join(t.TempDir(), "systemd", "user", "postilaatikko-helper.service")
	if err := installSystemdUnit(context.Background(), path, "unit\n"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "unit\n" {
		t.Fatalf("unit=%q,%v", data, err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("file mode=%v", info.Mode())
	}
	info, _ = os.Stat(filepath.Dir(path))
	if info.Mode().Perm() != 0700 {
		t.Fatalf("directory mode=%v", info.Mode())
	}
	want := [][]string{{"--user", "daemon-reload"}, {"--user", "enable", "--now", systemdServiceName}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("calls=%v", *calls)
	}
	if err := installSystemdUnit(context.Background(), path, "replacement"); err == nil {
		t.Fatal("overwrote existing unit")
	}
	if len(*calls) != 2 {
		t.Fatal("called systemctl for existing unit")
	}
}
func TestInstallSystemdUnitDoesNotFollowSymlink(t *testing.T) {
	calls := fakeSystemctl(t, 0)
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	path := filepath.Join(dir, "service")
	if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := installSystemdUnit(context.Background(), path, "replace"); err == nil {
		t.Fatal("followed symlink")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "keep" || len(*calls) != 0 {
		t.Fatal("changed target or called runner")
	}
}
func TestInstallSystemdUnitReportsCommandFailure(t *testing.T) {
	for _, fail := range []int{1, 2} {
		t.Run(string(rune('0'+fail)), func(t *testing.T) {
			calls := fakeSystemctl(t, fail)
			path := filepath.Join(t.TempDir(), "unit.service")
			err := installSystemdUnit(context.Background(), path, "unit")
			if err == nil || !strings.Contains(err.Error(), "saved") {
				t.Fatalf("error=%v", err)
			}
			if len(*calls) != fail {
				t.Fatalf("calls=%v", *calls)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatal("lost saved unit after command failure")
			}
		})
	}
}
func TestUninstallSystemdUnit(t *testing.T) {
	calls := fakeSystemctl(t, 0)
	path := filepath.Join(t.TempDir(), "unit.service")
	if err := os.WriteFile(path, []byte("unit"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := uninstallSystemdUnit(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unit remains")
	}
	want := [][]string{{"--user", "disable", "--now", systemdServiceName}, {"--user", "daemon-reload"}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("calls=%v", *calls)
	}
}
func TestUninstallKeepsUnitWhenStopFails(t *testing.T) {
	fakeSystemctl(t, 1)
	path := filepath.Join(t.TempDir(), "unit.service")
	if err := os.WriteFile(path, []byte("unit"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := uninstallSystemdUnit(context.Background(), path); err == nil {
		t.Fatal("ignored stop failure")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("removed running service configuration")
	}
}

func TestLinuxInstallRejectsEncryptedFileBeforeSideEffects(t *testing.T) {
	calls := fakeSystemctl(t, 0)
	home := t.TempDir()
	t.Setenv("HOME", home)
	err := run([]string{"install-agent", "--identity-store", "encrypted-file", "--data", filepath.Join(home, "mail")})
	if err == nil || !strings.Contains(err.Error(), "keyring identities only") {
		t.Fatalf("error=%v", err)
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 || len(*calls) != 0 {
		t.Fatal("side effects before rejection")
	}
}
func TestLinuxInstallValidatesBeforeWriting(t *testing.T) {
	calls := fakeSystemctl(t, 0)
	home := t.TempDir()
	t.Setenv("HOME", home)
	err := installAgent(context.Background(), "id", "data", "queue", "not-an-address", "", "", nil, false, apiListenerConfig{address: "127.0.0.1:8027"}, alwayson.GmailPushConfig{})
	if err == nil {
		t.Fatal("accepted invalid listener")
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 || len(*calls) != 0 {
		t.Fatal("side effects before validation")
	}
}
func TestSystemdUnitPathUsesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path, err := systemdUnitPath()
	if err != nil || path != filepath.Join(home, ".config", "systemd", "user", systemdServiceName) {
		t.Fatalf("path=%q err=%v", path, err)
	}
}
