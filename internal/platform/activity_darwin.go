//go:build darwin

package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation

#import <Foundation/Foundation.h>

// beginActivity holds an NSProcessInfo activity assertion and returns it
// retained, so it survives autoreleasepool drainage until endActivity.
static void* beginActivity(unsigned long long options, const char* reason) {
	@autoreleasepool {
		id activity = [[NSProcessInfo processInfo]
			beginActivityWithOptions:(NSActivityOptions)options
			reason:[NSString stringWithUTF8String:reason]];
		return (void*)CFBridgingRetain(activity);
	}
}

static void endActivity(void* token) {
	@autoreleasepool {
		id activity = (id)CFBridgingRelease(token);
		[[NSProcessInfo processInfo] endActivity:activity];
	}
}
*/
import "C"

import "unsafe"

// darwinActivityBackend asserts through NSProcessInfo, the API Apple
// documents for keeping a specific operation out of App Nap (Energy
// Efficiency Guide for Mac Apps, "Prioritize Work at the App Level").
type darwinActivityBackend struct{}

func (darwinActivityBackend) begin(options ActivityOptions, reason string) (release func()) {
	cReason := C.CString(reason)
	token := C.beginActivity(C.ulonglong(options), cReason)
	C.free(unsafe.Pointer(cReason))
	var once bool
	return func() {
		if once {
			return
		}
		once = true
		C.endActivity(token)
	}
}

func newActivityBackend() activityBackend {
	return darwinActivityBackend{}
}
