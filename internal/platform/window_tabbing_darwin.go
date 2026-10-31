//go:build darwin && cgo

package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>

static BOOL postilaatikkoPrefersWindowTabs(BOOL fullscreen) {
	switch ([NSWindow userTabbingPreference]) {
	case NSWindowUserTabbingPreferenceAlways:
		return YES;
	case NSWindowUserTabbingPreferenceInFullScreen:
		return fullscreen;
	default:
		return NO;
	}
}
*/
import "C"

// PrefersWindowTabs follows macOS's System Settings > Desktop & Dock tab
// preference for the current window.
func PrefersWindowTabs(fullscreen bool) bool {
	return C.postilaatikkoPrefersWindowTabs(C.BOOL(fullscreen)) != 0
}
