package linux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Updating the app in place on Linux (SET-008). An AppImage is one file, so a
// copy run as one is replaced by the release's: the engine has already
// checked that file against the release's signed list (SEC-021). A copy
// installed from the .deb belongs to the package manager and to root; it is
// not replaced, and says where the new package is.
//
// This file has no build constraint, unlike the rest of the package, so that
// its tests run on every system; it uses nothing of Linux but the APPIMAGE
// variable.

// UpdateKind says this copy updates from the AppImage, or why it cannot.
func UpdateKind() (string, error) {
	return updateKind(os.Getenv("APPIMAGE"))
}

func updateKind(appImage string) (string, error) {
	// The AppImage runtime sets APPIMAGE to the file it was started from.
	if appImage == "" {
		return "", errors.New("this copy was installed as a package, which only the package manager replaces; download the new .deb from the release's page")
	}
	probe, err := os.CreateTemp(filepath.Dir(appImage), ".freeloop-update-")
	if err != nil {
		return "", fmt.Errorf("the folder the AppImage is in (%s) cannot be written by you; download the release from its page", filepath.Dir(appImage))
	}
	probe.Close()
	return "appimage", os.Remove(probe.Name())
}

// ApplyUpdate replaces the AppImage this copy runs from with file and returns
// the command that starts it. run and scratch are not needed here.
func ApplyUpdate(_ context.Context, file, _, _ string, _ func(context.Context, []string) (string, error)) ([]string, error) {
	return applyUpdate(os.Getenv("APPIMAGE"), file)
}

func applyUpdate(appImage, file string) ([]string, error) {
	if _, err := updateKind(appImage); err != nil {
		return nil, err
	}
	// Beside the old one, so the rename that puts it in place stays on one
	// file system and the AppImage is at every moment whole: the old or the new.
	next := filepath.Join(filepath.Dir(appImage), "."+filepath.Base(appImage)+".update")
	if err := copyFile(file, next, 0o755); err != nil {
		os.Remove(next)
		return nil, err
	}
	// The running copy keeps the old file open; its name now holds the new one.
	if err := os.Rename(next, appImage); err != nil {
		os.Remove(next)
		return nil, fmt.Errorf("put the new AppImage in place: %w", err)
	}
	return []string{appImage, "--after-update"}, nil
}

func copyFile(from, to string, mode os.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// FinishUpdate has nothing to remove: the old AppImage went with the rename.
func FinishUpdate() {}
