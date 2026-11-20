package platform

// AutostartManager manages application autostart on login.
type AutostartManager interface {
	// Enable creates the autostart entry.
	Enable() error
	// Disable removes the autostart entry.
	Disable() error
	// IsEnabled checks if autostart is currently configured.
	IsEnabled() bool
}

// MigrateLegacyAutostart rewrites only an existing legacy login entry. It never
// enables login startup for a user who did not previously enable it.
func MigrateLegacyAutostart(m AutostartManager) error {
	if legacy, ok := m.(interface{ migrateLegacy() error }); ok {
		return legacy.migrateLegacy()
	}
	return nil
}
