package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The install script (scripts/install.sh, NFR-REL-006) leaves one Freeloop on the
// Mac, built from the checkout's commit. These tests run it in a throwaway repository
// with a fake build and a temporary home folder, so it cannot touch a real app or the
// real Trash.

const fakePlist = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleName</key><string>FreeloopTest</string>
<key>CFBundleExecutable</key><string>FreeloopTest</string>
<key>CFBundleIdentifier</key><string>test.freeloop.install</string>
</dict></plist>
`

// The fake build writes a signed bundle that says which commit it is from, as the
// real one does, unless told to fail or to say another commit.
const fakeBuild = `set -e
[ -z "${FAKE_FAIL:-}" ] || { echo "the build failed" >&2; exit 1; }
out=bin/FreeloopTest.app
rm -rf "$out"
mkdir -p "$out/Contents/MacOS"
cp /usr/bin/true "$out/Contents/MacOS/FreeloopTest"
cp build/darwin/Info.plist "$out/Contents/Info.plist"
c=$(git rev-parse --short=7 HEAD)
if [ -n "$(git status --porcelain)" ]; then c="$c+dirty"; fi
plutil -replace FreeloopCommit -string "${FAKE_STAMP:-$c}" "$out/Contents/Info.plist"
echo new > "$out/Contents/Resources-new"
codesign --force --deep --sign - "$out" >/dev/null 2>&1
`

type installRig struct {
	t        *testing.T
	repo     string
	home     string
	build    string
	scriptIn string
}

func newInstallRig(t *testing.T) *installRig {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("the install script is for a Mac")
	}
	root := t.TempDir()
	r := &installRig{t: t, repo: filepath.Join(root, "repo"), home: filepath.Join(root, "home")}
	r.write("repo/build/darwin/Info.plist", fakePlist)
	r.write("repo/.gitignore", "bin/\n")
	src, err := os.ReadFile("../../scripts/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	r.write("repo/scripts/install.sh", string(src))
	r.write("fakebuild.sh", fakeBuild)
	r.build = filepath.Join(root, "fakebuild.sh")
	os.MkdirAll(filepath.Join(r.home, ".Trash"), 0o755)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "first"}} {
		r.git(args...)
	}
	return r
}

func (r *installRig) write(rel, text string) {
	r.t.Helper()
	p := filepath.Join(filepath.Dir(r.repo), rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o755); err != nil {
		r.t.Fatal(err)
	}
}

func (r *installRig) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// run runs the script as a person would, with what the tests must set to stay inside
// the sandbox, and returns its output and whether it succeeded.
func (r *installRig) run(extra ...string) (string, bool) {
	r.t.Helper()
	cmd := exec.Command("sh", "scripts/install.sh")
	cmd.Dir = r.repo
	cmd.Env = append(os.Environ(), "HOME="+r.home, "INSTALL_BUILD=sh "+r.build, "INSTALL_SPOTLIGHT=0", "LSREGISTER=true", "OSASCRIPT=true", "INSTALL_QUIT_TRIES=2")
	cmd.Env = append(cmd.Env, extra...)
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

func (r *installRig) trashed() []string {
	r.t.Helper()
	entries, _ := os.ReadDir(filepath.Join(r.home, ".Trash"))
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func (r *installRig) exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func (r *installRig) commitOf(app string) string {
	r.t.Helper()
	out, err := exec.Command("plutil", "-extract", "FreeloopCommit", "raw", "-o", "-", filepath.Join(app, "Contents", "Info.plist")).Output()
	if err != nil {
		r.t.Fatalf("no commit in %s: %v", app, err)
	}
	return strings.TrimSpace(string(out))
}

// An old bundle is replaced by one made from this commit, and the old one is in the
// Trash, not gone.
func TestInstallReplacesTheOldBundle(t *testing.T) {
	r := newInstallRig(t)
	app := filepath.Join(r.repo, "bin", "FreeloopTest.app")
	os.MkdirAll(filepath.Join(app, "Contents"), 0o755)
	os.WriteFile(filepath.Join(app, "Contents", "marker-old"), []byte("old"), 0o644)
	out, ok := r.run()
	if !ok {
		t.Fatalf("install failed:\n%s", out)
	}
	if got, want := r.commitOf(app), r.git("rev-parse", "--short=7", "HEAD"); got != want {
		t.Errorf("the bundle is from %q, the checkout is at %q", got, want)
	}
	if r.exists(filepath.Join(app, "Contents", "marker-old")) || !r.exists(filepath.Join(app, "Contents", "Resources-new")) {
		t.Error("the old bundle is still in place")
	}
	trashed := r.trashed()
	if len(trashed) != 1 || !r.exists(filepath.Join(r.home, ".Trash", trashed[0], "Contents", "marker-old")) {
		t.Errorf("the old bundle is not in the Trash: %v", trashed)
	}
}

// With INSTALL_DIR the new bundle ends there and nowhere else: the checkout's bin has
// none, the one that was there is in the Trash, and so is a stray copy in the user's
// Applications folder.
func TestInstallLeavesOneCopy(t *testing.T) {
	r := newInstallRig(t)
	dir := filepath.Join(filepath.Dir(r.repo), "apps")
	old := filepath.Join(dir, "FreeloopTest.app", "Contents")
	stray := filepath.Join(r.home, "Applications", "FreeloopTest.app", "Contents")
	for _, p := range []string{old, stray, filepath.Join(r.repo, "bin", "FreeloopTest.app", "Contents")} {
		os.MkdirAll(p, 0o755)
		os.WriteFile(filepath.Join(p, "marker-old"), []byte("old"), 0o644)
	}
	out, ok := r.run("INSTALL_DIR=" + dir)
	if !ok {
		t.Fatalf("install failed:\n%s", out)
	}
	app := filepath.Join(dir, "FreeloopTest.app")
	if got, want := r.commitOf(app), r.git("rev-parse", "--short=7", "HEAD"); got != want {
		t.Errorf("the bundle is from %q, the checkout is at %q", got, want)
	}
	if r.exists(filepath.Join(app, "Contents", "marker-old")) {
		t.Error("the old bundle is still at the destination")
	}
	for _, gone := range []string{filepath.Join(r.repo, "bin", "FreeloopTest.app"), filepath.Dir(stray)} {
		if r.exists(gone) {
			t.Errorf("%s is still there", gone)
		}
	}
	if n := len(r.trashed()); n != 3 {
		t.Errorf("the Trash holds %d bundles, want the three that were replaced: %v\n%s", n, r.trashed(), out)
	}
	if !strings.Contains(out, "is at "+app) {
		t.Errorf("it does not say where the app is:\n%s", out)
	}
}

// A build that fails leaves the old bundle where it was, with nothing left in the Trash.
func TestInstallPutsTheOldBundleBackWhenTheBuildFails(t *testing.T) {
	r := newInstallRig(t)
	app := filepath.Join(r.repo, "bin", "FreeloopTest.app")
	os.MkdirAll(filepath.Join(app, "Contents"), 0o755)
	os.WriteFile(filepath.Join(app, "Contents", "marker-old"), []byte("old"), 0o644)
	out, ok := r.run("FAKE_FAIL=1")
	if ok {
		t.Fatalf("a failed build succeeded:\n%s", out)
	}
	if !r.exists(filepath.Join(app, "Contents", "marker-old")) || len(r.trashed()) != 0 {
		t.Errorf("the old bundle is not back: trash %v\n%s", r.trashed(), out)
	}
}

// A bundle that says it is from another commit than the checkout is not kept: the
// old one comes back, and the wrong one goes to the Trash.
func TestInstallRefusesABundleOfAnotherCommit(t *testing.T) {
	r := newInstallRig(t)
	app := filepath.Join(r.repo, "bin", "FreeloopTest.app")
	os.MkdirAll(filepath.Join(app, "Contents"), 0o755)
	os.WriteFile(filepath.Join(app, "Contents", "marker-old"), []byte("old"), 0o644)
	out, ok := r.run("FAKE_STAMP=deadbee")
	if ok || !strings.Contains(out, "deadbee") {
		t.Fatalf("a bundle of another commit was kept (ok=%v):\n%s", ok, out)
	}
	if !r.exists(filepath.Join(app, "Contents", "marker-old")) || r.exists(filepath.Join(app, "Contents", "Resources-new")) {
		t.Errorf("the old bundle is not back in place:\n%s", out)
	}
	if n := len(r.trashed()); n != 1 {
		t.Errorf("the Trash holds %d, want the wrong bundle only: %v", n, r.trashed())
	}
}

// Uncommitted changes are said in the stamp, and the bundle's matches.
func TestInstallStampsUncommittedChanges(t *testing.T) {
	r := newInstallRig(t)
	r.write("repo/change.txt", "not committed")
	out, ok := r.run()
	if !ok {
		t.Fatalf("install failed:\n%s", out)
	}
	if got := r.commitOf(filepath.Join(r.repo, "bin", "FreeloopTest.app")); !strings.HasSuffix(got, "+dirty") {
		t.Errorf("the stamp is %q, want it to say the files had changes", got)
	}
}

// Freeloop is quit before the build, never forced: one that stays open stops the
// install and keeps running, and the old bundle stays where it was.
func TestInstallDoesNotForceAnAppThatStaysOpen(t *testing.T) {
	r := newInstallRig(t)
	app := filepath.Join(r.repo, "bin", "FreeloopTest.app")
	exe := filepath.Join(filepath.Dir(r.repo), "open", "FreeloopTest.app", "Contents", "MacOS", "FreeloopTest")
	r.write("open/FreeloopTest.app/Contents/MacOS/FreeloopTest", "#!/bin/sh\nsleep 30\n")
	os.MkdirAll(filepath.Join(app, "Contents"), 0o755)
	os.WriteFile(filepath.Join(app, "Contents", "marker-old"), []byte("old"), 0o644)
	running := exec.Command(exe)
	if err := running.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { running.Process.Kill(); running.Wait() })
	out, ok := r.run()
	if ok || !strings.Contains(out, "still running") {
		t.Fatalf("it went on with the app open (ok=%v):\n%s", ok, out)
	}
	if running.ProcessState != nil {
		t.Error("the app was ended")
	}
	if !r.exists(filepath.Join(app, "Contents", "marker-old")) || len(r.trashed()) != 0 {
		t.Errorf("the old bundle was touched: %v", r.trashed())
	}
}

// Run from inside the app, which it would quit, it refuses.
func TestInstallRefusesToRunInsideTheApp(t *testing.T) {
	r := newInstallRig(t)
	wrapper := filepath.Join(filepath.Dir(r.repo), "inside", "FreeloopTest.app", "Contents", "MacOS", "FreeloopTest")
	r.write("inside/FreeloopTest.app/Contents/MacOS/FreeloopTest", "#!/bin/sh\n# Stays the parent, as the app is of a shell running in it.\nsh scripts/install.sh\n")
	cmd := exec.Command(wrapper)
	cmd.Dir = r.repo
	cmd.Env = append(os.Environ(), "HOME="+r.home, "INSTALL_BUILD=sh "+r.build, "INSTALL_SPOTLIGHT=0", "LSREGISTER=true", "OSASCRIPT=true")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "inside FreeloopTest") {
		t.Errorf("it ran inside the app (err=%v):\n%s", err, out)
	}
	if r.exists(filepath.Join(r.repo, "bin")) {
		t.Error("it built something")
	}
}
