package environment

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// On Windows a path can lead elsewhere through a symbolic link or a junction, and a
// junction needs no privilege to make. Rather than refusing reparse points, which
// OneDrive's files on demand are too, a file is opened and then asked where it really
// is: if that is not the path the guard checked, a link on the way led somewhere else,
// such as a folder swapped for a junction since the check, and the file is closed
// unread or unwritten.

// openNoFollow opens path, failing if any part of it led through a link.
func openNoFollow(path string, flag int) (*os.File, error) {
	f, err := os.OpenFile(path, flag&^syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	if err := sameFinalPath(windows.Handle(f.Fd()), path); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// createNoFollow opens the file at real for writing, truncated, making it with any
// missing folders on the way. real is absolute and clean, and the path the guard
// checked. The file is opened without truncating, checked to be where real says,
// then truncated; a file made through a link is deleted again. An existing file must
// be a regular file (errNotRegular). Unlike on Unix, a folder made on the way through
// a junction put there since the check is made where the junction leads; the write
// itself still fails.
func createNoFollow(real string, newMode os.FileMode) (*os.File, error) {
	if !filepath.IsAbs(real) || filepath.Base(real) == "" {
		return nil, &fs.PathError{Op: "open", Path: real, Err: fs.ErrInvalid}
	}
	dir := filepath.Dir(real)
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		beforeMkdir()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	name, err := windows.UTF16PtrFromString(real)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: real, Err: err}
	}
	const share = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE
	created := false
	h, err := windows.CreateFile(name, windows.GENERIC_WRITE, share, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		attrs := uint32(windows.FILE_ATTRIBUTE_NORMAL)
		if newMode.Perm()&0o200 == 0 {
			attrs = windows.FILE_ATTRIBUTE_READONLY
		}
		// A file made here may have to be deleted again, if it was made through a link.
		h, err = windows.CreateFile(name, windows.GENERIC_WRITE|windows.DELETE, share, nil, windows.CREATE_NEW, attrs, 0)
		created = err == nil
	}
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: real, Err: err}
	}
	file := os.NewFile(uintptr(h), real)
	if err := sameFinalPath(h, real); err != nil {
		if created {
			deleteOnClose(h)
		}
		file.Close()
		return nil, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		file.Close()
		return nil, &fs.PathError{Op: "stat", Path: real, Err: err}
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		file.Close()
		return nil, errNotRegular
	}
	if err := file.Truncate(0); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

// sameFinalPath fails unless the open file h is at want, letter case aside. A drive
// mapped to a network share or made with subst has a final path on the share or
// folder it stands for, so want is also tried with its drive spelled that way.
func sameFinalPath(h windows.Handle, want string) error {
	got, err := finalPath(h)
	if err != nil {
		return &fs.PathError{Op: "open", Path: want, Err: err}
	}
	same := func(w string) bool { return strings.EqualFold(got, w) || FoldPath(got) == FoldPath(w) }
	if !same(want) && !same(onVolume(want)) {
		return fmt.Errorf("%s leads through a symbolic link or junction the check did not see, such as one put there since, to %s; it is not followed", want, got)
	}
	return nil
}

// GetFinalPathNameByHandle's flags, which x/sys/windows does not name.
const (
	fileNameNormalized = 0x0
	volumeNameDOS      = 0x0
)

// finalPath is where an open file is, as a drive or UNC path.
func finalPath(h windows.Handle) (string, error) {
	buf := make([]uint16, windows.MAX_PATH)
	for {
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), fileNameNormalized|volumeNameDOS)
		if err != nil {
			return "", err
		}
		if int(n) < len(buf) {
			return trimLongPrefix(windows.UTF16ToString(buf[:n])), nil
		}
		buf = make([]uint16, n)
	}
}

// onVolume spells p with its drive replaced by where the drive really is: Z:\a on a
// drive mapped to \\server\share is \\server\share\a. p itself when that cannot be told.
func onVolume(p string) string {
	vol := filepath.VolumeName(p)
	if vol == "" {
		return p
	}
	name, err := windows.UTF16PtrFromString(vol + `\`)
	if err != nil {
		return p
	}
	h, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return p
	}
	defer windows.CloseHandle(h)
	root, err := finalPath(h)
	if err != nil {
		return p
	}
	return strings.TrimRight(root, `\`) + p[len(vol):]
}

// trimLongPrefix turns \\?\C:\a into C:\a and \\?\UNC\host\share into \\host\share.
func trimLongPrefix(p string) string {
	if rest, ok := strings.CutPrefix(p, `\\?\UNC\`); ok {
		return `\\` + rest
	}
	return strings.TrimPrefix(p, `\\?\`)
}

// deleteOnClose marks an open file to be deleted when its handle closes.
func deleteOnClose(h windows.Handle) {
	del := uint32(1)
	windows.SetFileInformationByHandle(h, windows.FileDispositionInfo, (*byte)(unsafe.Pointer(&del)), uint32(unsafe.Sizeof(del)))
}

// errNotRegular is a write to something other than a regular file.
var errNotRegular = errors.New("not a regular file")

// beforeMkdir runs before each folder a write makes, for tests of that moment.
var beforeMkdir = func() {}
