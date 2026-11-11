package environment

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Files the app downloads for voice input (GUI-050). Too large to hold in memory,
// the model is written as it arrives and kept only when it is what was pinned.

// ErrNotPinned is a download that is not the file the app knows.
var ErrNotPinned = errors.New("environment: the download is not the file Freeloop knows, so it was not kept")

// SaveDownload writes r to path, whole or not at all: into path.part first, then,
// when it is size bytes with the SHA-256 sha, renamed into place. progress, if not
// nil, is told how much has arrived. A part file left by a failed try is replaced.
func SaveDownload(path string, r io.Reader, size int64, sha string, progress func(int64)) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("environment: %w", err)
	}
	part := path + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("environment: %w", err)
	}
	h := sha256.New()
	w := io.MultiWriter(f, h)
	var got int64
	buf := make([]byte, 256<<10)
	for {
		n, rerr := r.Read(buf)
		if n > 0 {
			if got+int64(n) > size {
				rerr = ErrNotPinned
			} else if _, err := w.Write(buf[:n]); err != nil {
				rerr = err
			}
			got += int64(n)
			if progress != nil {
				progress(min(got, size))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(part)
			if errors.Is(rerr, ErrNotPinned) {
				return ErrNotPinned
			}
			return fmt.Errorf("environment: %w", rerr)
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(part)
		return fmt.Errorf("environment: %w", err)
	}
	if got != size || hex.EncodeToString(h.Sum(nil)) != sha {
		os.Remove(part)
		return ErrNotPinned
	}
	if err := os.Rename(part, path); err != nil {
		os.Remove(part)
		return fmt.Errorf("environment: %w", err)
	}
	return nil
}

// ToolFile is a file of a program the app downloaded: its name in the program's
// folder and either its bytes or, on Linux, the name of the file beside it that it
// links to, as a library's versioned names do.
type ToolFile struct {
	Name string
	Data []byte
	Link string
}

// InstallFolder makes dir hold exactly files, whole or not at all. Names and links
// are plain names: nothing reaches outside the folder.
func InstallFolder(dir string, files []ToolFile) error {
	names := map[string]bool{}
	for _, f := range files {
		names[f.Name] = true
	}
	for _, f := range files {
		if !plainName(f.Name) {
			return fmt.Errorf("environment: %q is not a file name", f.Name)
		}
		if f.Link != "" && (!plainName(f.Link) || !names[f.Link]) {
			return fmt.Errorf("environment: %s links to %q, which is not beside it", f.Name, f.Link)
		}
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return fmt.Errorf("environment: %w", err)
	}
	tmp := dir + ".tmp"
	os.RemoveAll(tmp)
	if err := os.Mkdir(tmp, 0o700); err != nil {
		return fmt.Errorf("environment: %w", err)
	}
	for _, f := range files {
		p := filepath.Join(tmp, f.Name)
		var err error
		if f.Link != "" {
			err = os.Symlink(f.Link, p)
		} else {
			err = os.WriteFile(p, f.Data, 0o700)
		}
		if err != nil {
			os.RemoveAll(tmp)
			return fmt.Errorf("environment: %w", err)
		}
	}
	os.RemoveAll(dir)
	if err := os.Rename(tmp, dir); err != nil {
		os.RemoveAll(tmp)
		return fmt.Errorf("environment: %w", err)
	}
	return nil
}

func plainName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\:`) && filepath.Base(name) == name
}
