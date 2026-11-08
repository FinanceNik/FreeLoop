//go:build unix

package environment

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// ErrLocked is a data folder another copy of the app has open.
var ErrLocked = errors.New("another copy of Freeloop is using this data folder")

// LockFolder holds a lock on a folder until the process ends, however it ends:
// the system lets go of it. Two copies of the app on one data folder would write
// over each other's transcripts.
func LockFolder(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("environment: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("environment: %w", err)
	}
	return f, nil
}

// RedirectStderr sends the process's standard error to f, so a crash's stack trace
// lands in the log: a launched app's goes to /dev/null.
func RedirectStderr(f *os.File) error {
	// Dup2 is not on every system (Linux on arm64 has only dup3); unix.Dup2 is.
	return unix.Dup2(int(f.Fd()), 2)
}
