package environment

import (
	"runtime"
	"syscall"
)

// noFollow makes an open fail if the path holds a symbolic link. On macOS that
// is O_NOFOLLOW_ANY, for any part of the path, which cannot be combined with
// O_NOFOLLOW; elsewhere O_NOFOLLOW covers only the last part, and other systems
// need their own way when they are supported (NFR-XPLAT-003).
var noFollow = func() int {
	if runtime.GOOS == "darwin" {
		return 0x20000000
	}
	return syscall.O_NOFOLLOW
}()

// afterCheck runs between a path's check and its open, for tests of that gap.
var afterCheck = func() {}
