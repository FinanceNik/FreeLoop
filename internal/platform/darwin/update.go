//go:build darwin

package darwin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Updating the app in place on a Mac (SET-008): the bundle the app runs from
// is replaced by the one in a release's disk image. The engine has already
// checked the image against the release's signed list (SEC-021); here the
// bundle inside is checked to be this app, of that version, validly signed,
// and by the same Apple team when this copy is signed by one.

// bundleID is the app's identifier, as build/darwin/Info.plist has it.
const bundleID = "io.github.financenik.freeloop"

// runner runs one of the system's programs and returns what it printed.
type runner = func(ctx context.Context, argv []string) (string, error)

// runningBundle is the .app the running program is in.
func runningBundle() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	return bundleOf(exe)
}

// bundleOf is the bundle of an executable at <bundle>.app/Contents/MacOS/<name>.
func bundleOf(exe string) (string, error) {
	macos := filepath.Dir(exe)
	bundle := filepath.Dir(filepath.Dir(macos))
	if filepath.Base(macos) != "MacOS" || filepath.Base(filepath.Dir(macos)) != "Contents" || !strings.HasSuffix(bundle, ".app") {
		return "", errors.New("this copy does not run from an app bundle, so there is nothing to replace; build it again from source")
	}
	return bundle, nil
}

// UpdateKind says this copy updates from the disk image, or why it cannot.
func UpdateKind() (string, error) {
	bundle, err := runningBundle()
	if err != nil {
		return "", err
	}
	return "dmg", replaceable(bundle)
}

// replaceable says whether the bundle can be swapped for another where it is.
func replaceable(bundle string) error {
	// macOS runs a downloaded app that was never moved from a read-only copy
	// at a random path; replacing that copy would change nothing.
	if strings.Contains(bundle, "/AppTranslocation/") {
		return errors.New("macOS runs this copy from a temporary place; move Freeloop to the Applications folder, open it from there, and update then")
	}
	if strings.HasPrefix(bundle, "/Volumes/") {
		return errors.New("this copy runs from a disk image; drag Freeloop to the Applications folder, open it from there, and update then")
	}
	// The swap renames within the bundle's folder, which must be writable.
	probe, err := os.MkdirTemp(filepath.Dir(bundle), ".freeloop-update-")
	if err != nil {
		return fmt.Errorf("the folder Freeloop is in (%s) cannot be written by you; download the release from its page", filepath.Dir(bundle))
	}
	return os.Remove(probe)
}

// staged and previous are the new bundle before the swap and the old one after
// it, beside the bundle so that each rename stays on one volume.
func staged(bundle string) string {
	return filepath.Join(filepath.Dir(bundle), ".Freeloop-update.app")
}
func previous(bundle string) string {
	return filepath.Join(filepath.Dir(bundle), ".Freeloop-previous.app")
}

// ApplyUpdate replaces the running bundle with the one in the disk image file,
// a release's of version, and returns the command that opens it.
func ApplyUpdate(ctx context.Context, file, version, scratch string, run runner) ([]string, error) {
	bundle, err := runningBundle()
	if err != nil {
		return nil, err
	}
	return ApplyUpdateTo(ctx, bundle, file, version, scratch, run)
}

// ApplyUpdateTo is ApplyUpdate for the bundle at a path, which a test names.
func ApplyUpdateTo(ctx context.Context, bundle, file, version, scratch string, run runner) (relaunch []string, err error) {
	if err := replaceable(bundle); err != nil {
		return nil, err
	}
	mount := filepath.Join(scratch, "image")
	if err := os.MkdirAll(mount, 0o700); err != nil {
		return nil, err
	}
	// Read-only, not shown in the Finder, and nothing in it opened.
	if _, err := run(ctx, []string{"/usr/bin/hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen", "-noverify", "-mountpoint", mount, file}); err != nil {
		return nil, fmt.Errorf("open the disk image: %w", err)
	}
	defer run(context.WithoutCancel(ctx), []string{"/usr/bin/hdiutil", "detach", "-force", mount})

	src := filepath.Join(mount, filepath.Base(bundle))
	if fi, err := os.Lstat(src); err != nil || !fi.IsDir() {
		// The release's bundle is Freeloop.app whatever this copy was renamed to.
		src = filepath.Join(mount, "Freeloop.app")
		if fi, err := os.Lstat(src); err != nil || !fi.IsDir() {
			return nil, errors.New("the disk image holds no Freeloop.app")
		}
	}
	if err := checkBundle(ctx, src, version, bundle, run); err != nil {
		return nil, err
	}

	stage, old := staged(bundle), previous(bundle)
	for _, leftover := range []string{stage, old} {
		if err := os.RemoveAll(leftover); err != nil {
			return nil, err
		}
	}
	// ditto copies a bundle with its signature, links and attributes.
	if _, err := run(ctx, []string{"/usr/bin/ditto", src, stage}); err != nil {
		os.RemoveAll(stage)
		return nil, fmt.Errorf("copy the new app: %w", err)
	}
	// The copy is what will run, so it is the copy that is checked last.
	if err := checkBundle(ctx, stage, version, bundle, run); err != nil {
		os.RemoveAll(stage)
		return nil, err
	}
	if err := os.Rename(bundle, old); err != nil {
		os.RemoveAll(stage)
		return nil, fmt.Errorf("move the old app aside: %w", err)
	}
	if err := os.Rename(stage, bundle); err != nil {
		// Back to how it was: the old app where it was.
		if back := os.Rename(old, bundle); back != nil {
			return nil, fmt.Errorf("put the new app in place: %w; the old one is at %s", err, old)
		}
		os.RemoveAll(stage)
		return nil, fmt.Errorf("put the new app in place: %w", err)
	}
	// -n: a new copy even while this one is still quitting.
	return []string{"/usr/bin/open", "-n", bundle, "--args", "--after-update"}, nil
}

// checkBundle says whether the bundle at path is this app at version, validly
// signed, and signed by the same team as the running copy when that one is
// signed by a team: a copy from a Developer ID release only ever becomes
// another Developer ID release of the same team.
func checkBundle(ctx context.Context, path, version, running string, run runner) error {
	if _, err := run(ctx, []string{"/usr/bin/codesign", "--verify", "--deep", "--strict", path}); err != nil {
		return fmt.Errorf("the new app's signature does not hold: %w", err)
	}
	plist := filepath.Join(path, "Contents", "Info.plist")
	id, err := run(ctx, []string{"/usr/bin/plutil", "-extract", "CFBundleIdentifier", "raw", "-o", "-", plist})
	if err != nil || strings.TrimSpace(id) != bundleID {
		return fmt.Errorf("the disk image's app is %q, not Freeloop", strings.TrimSpace(id))
	}
	got, err := run(ctx, []string{"/usr/bin/plutil", "-extract", "CFBundleShortVersionString", "raw", "-o", "-", plist})
	if want := plistVersion(version); err != nil || strings.TrimSpace(got) != want {
		return fmt.Errorf("the disk image's app is version %q, not %s", strings.TrimSpace(got), want)
	}
	have, err := team(ctx, running, run)
	if err != nil {
		return err
	}
	if have == "" {
		return nil
	}
	next, err := team(ctx, path, run)
	if err != nil {
		return err
	}
	if next != have {
		return fmt.Errorf("this copy is signed by Apple team %s and the new app is not, so it was not installed", have)
	}
	return nil
}

// plistVersion is a version as Info.plist holds it, which takes only numbers:
// 0.1.0 for 0.1.0-beta.1.
func plistVersion(version string) string {
	v, _, _ := strings.Cut(version, "-")
	return v
}

// team is the Apple team a bundle is signed by, empty for an ad-hoc signature.
func team(ctx context.Context, path string, run runner) (string, error) {
	out, err := run(ctx, []string{"/usr/bin/codesign", "--display", "--verbose=2", path})
	if err != nil {
		return "", fmt.Errorf("read the signature of %s: %w", filepath.Base(path), err)
	}
	return teamOf(out), nil
}

// teamOf reads the team from codesign's description of a signature.
func teamOf(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "TeamIdentifier="); ok && v != "not set" {
			return v
		}
	}
	return ""
}

// FinishUpdate removes the copy an update moved aside, once the new one runs.
func FinishUpdate() {
	if bundle, err := runningBundle(); err == nil {
		os.RemoveAll(previous(bundle))
		os.RemoveAll(staged(bundle))
	}
}
