//go:build darwin

package darwin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// realRun runs the system's programs, as the engine's runner does.
func realRun(ctx context.Context, argv []string) (string, error) {
	out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %s", filepath.Base(argv[0]), strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// fakeApp writes an ad-hoc signed bundle that says what it is.
func fakeApp(t *testing.T, path, id, version, says string) {
	t.Helper()
	macos := filepath.Join(path, "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>` + id + `</string>
<key>CFBundleShortVersionString</key><string>` + version + `</string>
<key>CFBundleExecutable</key><string>Freeloop</string>
<key>CFBundlePackageType</key><string>APPL</string>
</dict></plist>`
	os.WriteFile(filepath.Join(path, "Contents", "Info.plist"), []byte(plist), 0o644)
	os.WriteFile(filepath.Join(macos, "Freeloop"), []byte("#!/bin/sh\necho "+says+"\n"), 0o755)
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", path).CombinedOutput(); err != nil {
		t.Fatalf("codesign: %v\n%s", err, out)
	}
}

// image makes a disk image holding the bundle at app, as make dmg does.
func image(t *testing.T, app string) string {
	t.Helper()
	dmg := filepath.Join(t.TempDir(), "Freeloop.dmg")
	src := filepath.Join(t.TempDir(), "src")
	os.MkdirAll(src, 0o755)
	if out, err := exec.Command("/usr/bin/ditto", app, filepath.Join(src, "Freeloop.app")).CombinedOutput(); err != nil {
		t.Fatalf("ditto: %v\n%s", err, out)
	}
	if out, err := exec.Command("/usr/bin/hdiutil", "create", "-volname", "Freeloop", "-srcfolder", src, "-ov", "-format", "UDZO", "-quiet", dmg).CombinedOutput(); err != nil {
		t.Skipf("hdiutil cannot make a disk image here: %v\n%s", err, out)
	}
	return dmg
}

func says(t *testing.T, bundle string) string {
	t.Helper()
	out, err := exec.Command(filepath.Join(bundle, "Contents", "MacOS", "Freeloop")).Output()
	if err != nil {
		t.Fatalf("run %s: %v", bundle, err)
	}
	return strings.TrimSpace(string(out))
}

// SET-008: the bundle is replaced by the release's, with the system's own
// programs, and what is not this app at that version stays out.
func TestApplyUpdate(t *testing.T) {
	ctx := context.Background()
	apps := t.TempDir()
	bundle := filepath.Join(apps, "Freeloop.app")
	fakeApp(t, bundle, bundleID, "0.1.0", "old")

	next := filepath.Join(t.TempDir(), "Freeloop.app")
	fakeApp(t, next, bundleID, "0.2.0", "new")
	dmg := image(t, next)

	// A release that says it is another version, or another app, is refused and
	// the running copy is left as it was.
	for name, tc := range map[string]struct{ id, version, ask string }{
		"another version": {bundleID, "0.3.0", "0.2.0"},
		"another app":     {"com.example.other", "0.2.0", "0.2.0"},
	} {
		other := filepath.Join(t.TempDir(), "Freeloop.app")
		fakeApp(t, other, tc.id, tc.version, "other")
		if _, err := ApplyUpdateTo(ctx, bundle, image(t, other), tc.ask, t.TempDir(), realRun); err == nil {
			t.Errorf("%s was installed", name)
		}
		if got := says(t, bundle); got != "old" {
			t.Fatalf("after %s the app says %q", name, got)
		}
	}

	// A bundle changed after it was signed.
	broken := filepath.Join(t.TempDir(), "Freeloop.app")
	fakeApp(t, broken, bundleID, "0.2.0", "new")
	os.WriteFile(filepath.Join(broken, "Contents", "MacOS", "Freeloop"), []byte("#!/bin/sh\necho changed\n"), 0o755)
	if _, err := ApplyUpdateTo(ctx, bundle, image(t, broken), "0.2.0", t.TempDir(), realRun); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Errorf("a bundle changed after signing: %v", err)
	}
	if got := says(t, bundle); got != "old" {
		t.Fatalf("after a broken bundle the app says %q", got)
	}

	// The release itself. A pre-release's bundle carries the plain number.
	scratch := t.TempDir()
	relaunch, err := ApplyUpdateTo(ctx, bundle, dmg, "0.2.0-beta.1", scratch, realRun)
	if err != nil {
		t.Fatal(err)
	}
	if got := says(t, bundle); got != "new" {
		t.Errorf("the app in place says %q", got)
	}
	if want := []string{"/usr/bin/open", "-n", bundle, "--args", "--after-update"}; strings.Join(relaunch, " ") != strings.Join(want, " ") {
		t.Errorf("relaunch = %v", relaunch)
	}
	if got := says(t, previous(bundle)); got != "old" {
		t.Errorf("the copy moved aside says %q", got)
	}
	if _, err := os.Stat(staged(bundle)); err == nil {
		t.Error("the staged copy was left behind")
	}
	// The image is let go of.
	if entries, _ := os.ReadDir(filepath.Join(scratch, "image")); len(entries) != 0 {
		t.Errorf("the disk image is still mounted: %v", entries)
	}
	if out, err := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", bundle).CombinedOutput(); err != nil {
		t.Errorf("the app in place is not validly signed: %s", out)
	}
}

func TestReplaceable(t *testing.T) {
	for _, path := range []string{
		"/private/var/folders/xx/T/AppTranslocation/ABCD/d/Freeloop.app",
		"/Volumes/Freeloop/Freeloop.app",
	} {
		if err := replaceable(path); err == nil {
			t.Errorf("%s can be replaced", path)
		}
	}
	locked := filepath.Join(t.TempDir(), "locked")
	os.MkdirAll(locked, 0o500)
	defer os.Chmod(locked, 0o700)
	if err := replaceable(filepath.Join(locked, "Freeloop.app")); err == nil && os.Geteuid() != 0 {
		t.Error("a bundle in a folder that cannot be written can be replaced")
	}
	if err := replaceable(filepath.Join(t.TempDir(), "Freeloop.app")); err != nil {
		t.Error(err)
	}
}

func TestBundleOf(t *testing.T) {
	if got, err := bundleOf("/Applications/Freeloop.app/Contents/MacOS/Freeloop"); err != nil || got != "/Applications/Freeloop.app" {
		t.Errorf("bundle = %q, %v", got, err)
	}
	if _, err := bundleOf("/Users/x/freeloop/bin/Freeloop"); err == nil {
		t.Error("a bare binary has a bundle")
	}
}

func TestTeamOf(t *testing.T) {
	if got := teamOf("Executable=/x\nAuthority=Developer ID Application: A B (ABCDE12345)\nTeamIdentifier=ABCDE12345\n"); got != "ABCDE12345" {
		t.Errorf("team = %q", got)
	}
	if got := teamOf("Signature=adhoc\nTeamIdentifier=not set\n"); got != "" {
		t.Errorf("an ad-hoc signature's team = %q", got)
	}
}

// A copy signed by a team takes only that team's app.
func TestCheckBundleTeam(t *testing.T) {
	run := func(running, next string) runner {
		return func(_ context.Context, argv []string) (string, error) {
			switch {
			case argv[0] == "/usr/bin/plutil" && argv[2] == "CFBundleIdentifier":
				return bundleID + "\n", nil
			case argv[0] == "/usr/bin/plutil":
				return "0.2.0\n", nil
			case argv[1] == "--display" && argv[len(argv)-1] == "/running.app":
				return "TeamIdentifier=" + running + "\n", nil
			case argv[1] == "--display":
				return "TeamIdentifier=" + next + "\n", nil
			}
			return "", nil
		}
	}
	ctx := context.Background()
	if err := checkBundle(ctx, "/new.app", "0.2.0", "/running.app", run("ABCDE12345", "ABCDE12345")); err != nil {
		t.Errorf("the same team: %v", err)
	}
	if err := checkBundle(ctx, "/new.app", "0.2.0", "/running.app", run("ABCDE12345", "not set")); err == nil {
		t.Error("an ad-hoc app replaced a team's")
	}
	if err := checkBundle(ctx, "/new.app", "0.2.0", "/running.app", run("ABCDE12345", "ZZZZZ99999")); err == nil {
		t.Error("another team's app replaced a team's")
	}
	// From ad hoc to a team's is the move to signed releases.
	if err := checkBundle(ctx, "/new.app", "0.2.0", "/running.app", run("not set", "ABCDE12345")); err != nil {
		t.Errorf("ad hoc to a team: %v", err)
	}
}
