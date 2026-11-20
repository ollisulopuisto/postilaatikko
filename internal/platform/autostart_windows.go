package platform

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"

	"github.com/ollisulopuisto/postilaatikko/internal/logging"
)

const (
	registryKeyPath        = `Software\Microsoft\Windows\CurrentVersion\Run`
	registryValue          = "Postilaatikko"
	startupApprovedKeyPath = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
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

	// Explicit Enable clears a prior Task Manager disablement for this value.
	approval, err := registry.OpenKey(registry.CURRENT_USER, startupApprovedKeyPath, registry.SET_VALUE)
	if err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("open startup approval: %w", err)
	}
	if err == nil {
		deleteErr := approval.DeleteValue(registryValue)
		closeErr := approval.Close()
		if deleteErr != nil && deleteErr != registry.ErrNotExist {
			return fmt.Errorf("enable startup approval: %w", deleteErr)
		}
		if closeErr != nil {
			return closeErr
		}
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
	if err != nil {
		return false
	}
	enabled, err := startupApprovalEnabled(registryValue)
	return err == nil && enabled
}

// execCommand returns the path to the current executable.
func (m *windowsAutostartManager) execCommand() string {
	exe, err := os.Executable()
	if err != nil {
		return "postilaatikko.exe"
	}
	return exe
}

// startupApprovalEnabled checks Task Manager's separate per-value setting.
func startupApprovalEnabled(name string) (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, startupApprovedKeyPath, registry.QUERY_VALUE)
	if err == registry.ErrNotExist {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("open startup approval: %w", err)
	}
	defer key.Close()
	data, _, err := key.GetBinaryValue(name)
	if err == registry.ErrNotExist {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read startup approval: %w", err)
	}
	return windowsStartupApprovalEnabled(data)
}

func (m *windowsAutostartManager) migrateLegacy() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryKeyPath, registry.QUERY_VALUE)
	if err == registry.ErrNotExist {
		return nil
	}
	if err != nil {
		return err
	}
	defer key.Close()
	// An existing replacement, enabled or disabled, is already the user's choice.
	if _, _, err = key.GetStringValue(registryValue); err == nil {
		return nil
	}
	if err != registry.ErrNotExist {
		return err
	}
	if _, _, err = key.GetStringValue("Aerion"); err == registry.ErrNotExist {
		return nil
	}
	if err != nil {
		return err
	}
	enabled, err := startupApprovalEnabled("Aerion")
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	return m.Enable()
}
