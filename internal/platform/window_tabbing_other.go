//go:build !darwin || !cgo

package platform

// PrefersWindowTabs is false where AppKit's native tab preference is not
// available.
func PrefersWindowTabs(fullscreen bool) bool { return false }
