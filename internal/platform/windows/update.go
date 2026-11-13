package windows

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Updating the app in place on Windows (SET-008). Installed or unpacked, a
// copy is one exe in a folder the user owns (the installer puts it under
// %LOCALAPPDATA%\Programs and asks for no administrator), so both are
// replaced the same way: by the exe in the release's portable zip, which the
// engine has already checked against the release's signed list (SEC-021).
// Windows will not overwrite a running program but lets it be renamed, so the
// old exe is moved aside and removed when the new copy starts.
//
// This file has no build constraint, so its tests run on every system.

// exeName is the program in the zip's one folder, as build/windows/Taskfile.yml
// packs it; the licenses beside it are replaced too.
const exeName = "Freeloop.exe"

var besideExe = []string{"THIRD_PARTY_NOTICES.txt", "LICENSE.txt"}

// maxExe bounds what is unpacked: the exe is about a twentieth of it.
const maxExe = 1 << 30

// UpdateKind says this copy updates from the portable zip, or why it cannot.
func UpdateKind() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return "zip", writable(filepath.Dir(exe))
}

func writable(dir string) error {
	probe, err := os.CreateTemp(dir, ".freeloop-update-")
	if err != nil {
		return fmt.Errorf("the folder Freeloop is in (%s) cannot be written by you; download the release from its page", dir)
	}
	probe.Close()
	return os.Remove(probe.Name())
}

// ApplyUpdate replaces the running exe with the one in the zip file, a
// release's of version, and returns the command that starts it.
func ApplyUpdate(_ context.Context, file, version, _ string, _ func(context.Context, []string) (string, error)) ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	relaunch, err := applyUpdate(exe, file)
	if err == nil {
		recordVersion(filepath.Dir(exe), version)
	}
	return relaunch, err
}

func applyUpdate(exe, file string) ([]string, error) {
	dir := filepath.Dir(exe)
	if err := writable(dir); err != nil {
		return nil, err
	}
	z, err := zip.OpenReader(file)
	if err != nil {
		return nil, fmt.Errorf("open the release's zip: %w", err)
	}
	defer z.Close()
	// Entries by their name within the zip's one folder; a name that would
	// leave it is never looked up.
	entries := map[string]*zip.File{}
	for _, f := range z.File {
		if name := path.Base(strings.ReplaceAll(f.Name, `\`, "/")); !f.FileInfo().IsDir() {
			entries[name] = f
		}
	}
	if entries[exeName] == nil {
		return nil, fmt.Errorf("the release's zip holds no %s", exeName)
	}
	next, old := exe+".new", exe+".old"
	if err := unpack(entries[exeName], next); err != nil {
		os.Remove(next)
		return nil, err
	}
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		os.Remove(next)
		return nil, fmt.Errorf("move the old program aside: %w", err)
	}
	if err := os.Rename(next, exe); err != nil {
		// Back to how it was.
		if back := os.Rename(old, exe); back != nil {
			return nil, fmt.Errorf("put the new program in place: %w; the old one is at %s", err, old)
		}
		os.Remove(next)
		return nil, fmt.Errorf("put the new program in place: %w", err)
	}
	// The licenses follow the program; a failure here leaves the old ones.
	for _, name := range besideExe {
		if f := entries[name]; f != nil {
			if err := unpack(f, filepath.Join(dir, name+".new")); err == nil {
				os.Rename(filepath.Join(dir, name+".new"), filepath.Join(dir, name))
			}
			os.Remove(filepath.Join(dir, name+".new"))
		}
	}
	return []string{exe, "--after-update"}, nil
}

func unpack(f *zip.File, to string) error {
	if f.UncompressedSize64 > maxExe {
		return errors.New("the release's zip holds a file larger than any release's")
	}
	in, err := f.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(in, maxExe)); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// FinishUpdate removes the program an update moved aside, once the new one runs.
func FinishUpdate() {
	if exe, err := os.Executable(); err == nil {
		os.Remove(exe + ".old")
		os.Remove(exe + ".new")
	}
}
