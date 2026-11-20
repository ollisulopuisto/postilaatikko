package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoginIdentityMigrationOnlyWhenPreviouslyEnabled(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := &linuxAutostartManager{}
	if err := MigrateLegacyAutostart(m); err != nil {
		t.Fatal(err)
	}
	if m.IsEnabled() {
		t.Fatal("enabled without permission")
	}
	dir, _ := m.autostartDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, legacyDesktopIDs[0]+".desktop")
	if err := os.WriteFile(old, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateLegacyAutostart(m); err != nil {
		t.Fatal(err)
	}
	if !m.IsEnabled() {
		t.Fatal("existing startup was lost")
	}
	b, err := os.ReadFile(filepath.Join(dir, autostartFilename))
	if err != nil || !strings.Contains(string(b), "Name=Postilaatikko") {
		t.Fatal("new identity missing", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("legacy duplicate remains")
	}
	if err := m.Disable(); err != nil {
		t.Fatal(err)
	}
	if m.IsEnabled() {
		t.Fatal("disable failed")
	}
}
func TestFlatpakDoesNotMigrateHostAutostartSilently(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := &linuxAutostartManager{isFlatpak: true}
	if err := MigrateLegacyAutostart(m); err != nil {
		t.Fatal(err)
	}
}

func TestDisabledLegacyLoginEntriesRemainDisabled(t *testing.T) {
	for _, entry := range []string{"[Desktop Entry]\nHidden=true\n", "[Desktop Entry]\nX-GNOME-Autostart-enabled=false\n", "[Desktop Entry]\nHidden=false\nX-GNOME-Autostart-enabled=false\n"} {
		t.Run(entry, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			m := &linuxAutostartManager{}
			dir, _ := m.autostartDir()
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			old := filepath.Join(dir, legacyDesktopIDs[0]+".desktop")
			if err := os.WriteFile(old, []byte(entry), 0600); err != nil {
				t.Fatal(err)
			}
			if err := MigrateLegacyAutostart(m); err != nil {
				t.Fatal(err)
			}
			if m.IsEnabled() {
				t.Fatal("disabled startup became enabled")
			}
			if b, err := os.ReadFile(old); err != nil || string(b) != entry {
				t.Fatal("disabled legacy entry changed", err)
			}
		})
	}
}
