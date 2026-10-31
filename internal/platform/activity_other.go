//go:build !darwin

package platform

// App Nap is a macOS feature; on Linux and Windows the manager still
// returns Activities so callers stay platform-neutral, they just assert
// nothing.
func newActivityBackend() activityBackend {
	return noopActivityBackend{}
}
