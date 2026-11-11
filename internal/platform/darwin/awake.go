//go:build darwin

package darwin

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework IOKit
#import <Foundation/Foundation.h>
#import <IOKit/pwr_mgt/IOPMLib.h>

// fl_begin_activity keeps App Nap from slowing the app while it is hidden, without
// keeping the Mac awake. The token is retained until fl_end_activity.
static void *fl_begin_activity(const char *reason) {
	id token = [[NSProcessInfo processInfo] beginActivityWithOptions:NSActivityUserInitiatedAllowingIdleSystemSleep
		reason:[NSString stringWithUTF8String:reason]];
	return (__bridge_retained void *)token;
}

static void fl_end_activity(void *token) {
	id t = (__bridge_transfer id)token;
	[[NSProcessInfo processInfo] endActivity:t];
}

// fl_prevent_sleep keeps the Mac from sleeping when idle; 0 when it could not.
static unsigned int fl_prevent_sleep(const char *reason) {
	IOPMAssertionID id = 0;
	CFStringRef r = CFStringCreateWithCString(kCFAllocatorDefault, reason, kCFStringEncodingUTF8);
	IOReturn ok = IOPMAssertionCreateWithName(kIOPMAssertionTypePreventUserIdleSystemSleep, kIOPMAssertionLevelOn, r, &id);
	CFRelease(r);
	return ok == kIOReturnSuccess ? id : 0;
}

static void fl_allow_sleep(unsigned int id) {
	IOPMAssertionRelease(id);
}
*/
import "C"

import (
	"sync"
	"unsafe"
)

// Sessions from the phone (GUI-052) need the app running while its window is
// hidden and the user away.

// NoNap keeps App Nap from slowing the app until release is called; the Mac may
// still sleep.
func NoNap(reason string) (release func()) {
	r := C.CString(reason)
	defer C.free(unsafe.Pointer(r))
	token := C.fl_begin_activity(r)
	var once sync.Once
	return func() { once.Do(func() { C.fl_end_activity(token) }) }
}

// KeepAwake keeps the Mac from sleeping when idle until release is called. A
// closed lid still sleeps it unless it is on power with an external display.
func KeepAwake(reason string) (release func()) {
	r := C.CString(reason)
	defer C.free(unsafe.Pointer(r))
	id := C.fl_prevent_sleep(r)
	var once sync.Once
	return func() {
		once.Do(func() {
			if id != 0 {
				C.fl_allow_sleep(id)
			}
		})
	}
}
