//go:build !linux

package environment

import (
	"os"
	"runtime"
	"syscall"
)

// openNoFollow opens path, failing if any part of it is a symbolic link, so a
// folder swapped for a link after the guard looked cannot lead around it. On macOS
// that is O_NOFOLLOW_ANY, which cannot be combined with O_NOFOLLOW; elsewhere
// O_NOFOLLOW covers only the last part, and a system needs its own way when it is
// supported (NFR-XPLAT-003; Linux has one in nofollow_linux.go).
func openNoFollow(path string, flag int) (*os.File, error) {
	if runtime.GOOS == "darwin" {
		return os.OpenFile(path, flag|0x20000000, 0)
	}
	return os.OpenFile(path, flag|syscall.O_NOFOLLOW, 0)
}
