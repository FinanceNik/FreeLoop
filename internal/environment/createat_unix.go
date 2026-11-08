//go:build unix

package environment

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// createNoFollow opens the file at real for writing, truncated, making it with
// newMode and any missing folders on the way. real is absolute and clean, and the
// path the guard checked. It is walked one folder at a time from the root, each
// opened relative to the one before without following a symbolic link, each
// missing one made relative to its open parent, and the file opened relative to
// the last. So nothing is made or written anywhere but where the check looked: a
// folder swapped for a link by a process racing the tool call, a sandboxed
// command, makes the write fail rather than land through it, before or after any
// folder is made (security review, 2026-11-03). An existing file keeps its mode
// and must be a regular file (errNotRegular).
func createNoFollow(real string, newMode os.FileMode) (*os.File, error) {
	dir, base := filepath.Split(real)
	if !filepath.IsAbs(real) || base == "" {
		return nil, &fs.PathError{Op: "open", Path: real, Err: fs.ErrInvalid}
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: "/", Err: err}
	}
	defer func() { unix.Close(fd) }()
	at := "/"
	for _, name := range strings.Split(strings.Trim(dir, "/"), "/") {
		if name == "" {
			continue
		}
		at = filepath.Join(at, name)
		next, err := openDirAt(fd, name)
		if errors.Is(err, unix.ENOENT) {
			beforeMkdir()
			if err := unix.Mkdirat(fd, name, 0o755); err != nil && !errors.Is(err, unix.EEXIST) {
				return nil, &fs.PathError{Op: "mkdir", Path: at, Err: err}
			}
			next, err = openDirAt(fd, name)
		}
		if err != nil {
			var st unix.Stat_t
			if errors.Is(err, unix.ELOOP) || unix.Fstatat(fd, name, &st, unix.AT_SYMLINK_NOFOLLOW) == nil && st.Mode&unix.S_IFMT == unix.S_IFLNK {
				return nil, fmt.Errorf("%s is a symbolic link the check did not see, such as one put there since; it is not followed: %w", at, err)
			}
			return nil, &fs.PathError{Op: "open", Path: at, Err: err}
		}
		unix.Close(fd)
		fd = next
	}

	var st unix.Stat_t
	switch err := unix.Fstatat(fd, base, &st, unix.AT_SYMLINK_NOFOLLOW); {
	case err == nil:
		if st.Mode&unix.S_IFMT != unix.S_IFREG {
			return nil, errNotRegular
		}
	case !errors.Is(err, unix.ENOENT):
		return nil, &fs.PathError{Op: "stat", Path: real, Err: err}
	}
	// Not blocking keeps a FIFO put in place since the look from hanging the
	// open; the file is checked again once open.
	ffd, err := unix.Openat(fd, base, unix.O_WRONLY|unix.O_CREAT|unix.O_TRUNC|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, uint32(newMode.Perm()))
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: real, Err: err}
	}
	if err := unix.SetNonblock(ffd, false); err != nil {
		unix.Close(ffd)
		return nil, &fs.PathError{Op: "open", Path: real, Err: err}
	}
	file := os.NewFile(uintptr(ffd), real)
	fi, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		file.Close()
		return nil, errNotRegular
	}
	return file, nil
}

// openDirAt opens the folder name in the folder dirfd, failing if it is a
// symbolic link or not a folder.
func openDirAt(dirfd int, name string) (int, error) {
	for {
		fd, err := unix.Openat(dirfd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		return fd, err
	}
}

// errNotRegular is a write to something other than a regular file.
var errNotRegular = errors.New("not a regular file")

// beforeMkdir runs before each folder a write makes, for tests of that moment.
var beforeMkdir = func() {}
