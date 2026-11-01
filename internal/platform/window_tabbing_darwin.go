//go:build darwin && cgo

package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>

// The Go boundary uses int: C BOOL is _Bool on Apple silicon but signed
// char on Intel, so converting a Go bool directly compiles on one
// architecture and not the other.
static int postilaatikkoPrefersWindowTabs(int fullscreen) {
	switch ([NSWindow userTabbingPreference]) {
	case NSWindowUserTabbingPreferenceAlways:
		return 1;
	case NSWindowUserTabbingPreferenceInFullScreen:
		return fullscreen;
	default:
		return 0;
	}
}
*/
import "C"

// PrefersWindowTabs follows macOS's System Settings > Desktop & Dock tab
// preference for the current window.
func PrefersWindowTabs(fullscreen bool) bool {
	fs := C.int(0)
	if fullscreen {
		fs = 1
	}
	return C.postilaatikkoPrefersWindowTabs(fs) != 0
}
