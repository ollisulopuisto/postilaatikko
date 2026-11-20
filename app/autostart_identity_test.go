//go:build linux

package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopAutostartInitializationIndependentOfWorkspace(t *testing.T) {
	for _, remote := range []bool{false, true} {
		t.Setenv("FLATPAK_ID", "")
		config := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", config)
		a := &App{remoteServiceMode: remote}
		a.initializeDesktopAutostart()
		if a.autostartMgr.IsEnabled() {
			t.Fatal("enabled absent startup", remote)
		}
		dir := filepath.Join(config, "autostart")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		old := filepath.Join(dir, "io.github.wesleiaqui.eternomail.desktop")
		if err := os.WriteFile(old, []byte("[Desktop Entry]\nType=Application\nX-GNOME-Autostart-enabled=true\n"), 0600); err != nil {
			t.Fatal(err)
		}
		a.initializeDesktopAutostart()
		if !a.autostartMgr.IsEnabled() {
			t.Fatal("legacy startup not migrated", remote)
		}
		if _, err := os.Stat(old); !os.IsNotExist(err) {
			t.Fatal("legacy duplicate remains", remote, err)
		}
	}
}
