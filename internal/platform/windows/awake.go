//go:build windows

package windows

import (
	"runtime"
	"sync"

	"golang.org/x/sys/windows"
)

// Sessions from the phone (GUI-052) need the app running while the user is away.

var setThreadExecutionState = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadExecutionState")

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

// NoNap does nothing: Windows does not slow a minimised app the way App Nap does.
func NoNap(string) (release func()) { return func() {} }

// KeepAwake keeps the computer from sleeping when idle until release is called.
// The state belongs to a thread, so one is held for it.
func KeepAwake(string) (release func()) {
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		setThreadExecutionState.Call(esContinuous | esSystemRequired)
		<-done
		setThreadExecutionState.Call(esContinuous)
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}
