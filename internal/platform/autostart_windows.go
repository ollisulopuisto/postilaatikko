package platform

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"

	"github.com/ollisulopuisto/postilaatikko/internal/logging"
)

const (
	registryKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	registryValue   = "Postilaatikko"
)

// windowsAutostartManager manages Windows autostart via the registry.
type windowsAutostartManager struct{}

// NewAutostartManager creates a new autostart manager.
func NewAutostartManager() AutostartManager {
	return &windowsAutostartManager{}
}

// Enable adds a registry entry to start Postilaatikko on login.
func (m *windowsAutostartManager) Enable() error {
	log := logging.WithComponent("autostart")

	key, _, err := registry.CreateKey(registry.CURRENT_USER, registryKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("failed to open registry key: %w", err)
	}
	defer key.Close()

	execPath := m.execCommand()
	value := fmt.Sprintf(`"%s"`, execPath)

	if err := key.SetStringValue(registryValue, value); err != nil {
		return fmt.Errorf("failed to set registry value: %w", err)
	}

	if err := key.DeleteValue("Aerion"); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("remove legacy autostart: %w", err)
	}
	log.Info().Str("value", value).Msg("Autostart enabled")
	return nil
}

// Disable removes the registry entry.
func (m *windowsAutostartManager) Disable() error {
	log := logging.WithComponent("autostart")

	key, err := registry.OpenKey(registry.CURRENT_USER, registryKeyPath, registry.SET_VALUE)
	if err != nil {
		// Key doesn't exist, nothing to disable
		return nil
	}
	defer key.Close()

	for _, name := range []string{registryValue, "Aerion"} {
		if err := key.DeleteValue(name); err != nil && err != registry.ErrNotExist {
			return err
		}
	}

	log.Info().Msg("Autostart disabled")
	return nil
}

// IsEnabled checks if the autostart registry entry exists.
func (m *windowsAutostartManager) IsEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	_, _, err = key.GetStringValue(registryValue)
	return err == nil
}

// execCommand returns the path to the current executable.
func (m *windowsAutostartManager) execCommand() string {
	exe, err := os.Executable()
	if err != nil {
		return "postilaatikko.exe"
	}
	return exe
}

func (m *windowsAutostartManager) migrateLegacy() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	_, _, err = key.GetStringValue("Aerion")
	key.Close()
	if err == nil {
		return m.Enable()
	}
	return nil
}
