//go:build linux

package environment

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// openNoFollow opens path, failing if any part of it is a symbolic link, so a
// folder swapped for a link after the guard looked cannot lead around it: what
// O_NOFOLLOW_ANY does on macOS. openat2 with RESOLVE_NO_SYMLINKS does it in one call
// (Linux 5.6 and later); an older kernel gets the same by opening one folder at a
// time, each relative to the last and without following a link.
func openNoFollow(path string, flag int) (*os.File, error) {
	how := unix.OpenHow{Flags: uint64(flag | unix.O_CLOEXEC), Resolve: unix.RESOLVE_NO_SYMLINKS}
	fd, err := unix.Openat2(unix.AT_FDCWD, path, &how)
	if errors.Is(err, unix.ENOSYS) {
		return openWalking(path, flag)
	}
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}

// openWalking opens path one folder at a time from the root, following no link.
func openWalking(path string, flag int) (*os.File, error) {
	if !filepath.IsAbs(path) {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrInvalid}
	}
	dir, base := filepath.Split(filepath.Clean(path))
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: "/", Err: err}
	}
	defer func() { unix.Close(fd) }()
	for _, name := range strings.Split(strings.Trim(dir, "/"), "/") {
		if name == "" {
			continue
		}
		next, err := openDirAt(fd, name)
		if err != nil {
			return nil, &os.PathError{Op: "open", Path: path, Err: err}
		}
		unix.Close(fd)
		fd = next
	}
	if base == "" {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrInvalid}
	}
	ffd, err := unix.Openat(fd, base, flag|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(ffd), path), nil
}
