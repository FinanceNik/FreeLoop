package linux

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// SET-008: an AppImage is replaced by the release's, whole, and stays a program.
func TestApplyUpdate(t *testing.T) {
	dir := t.TempDir()
	appImage := filepath.Join(dir, "Freeloop-0.1.0-linux-amd64.AppImage")
	os.WriteFile(appImage, []byte("old"), 0o755)
	file := filepath.Join(t.TempDir(), "Freeloop-0.2.0-linux-amd64.AppImage")
	os.WriteFile(file, []byte("new"), 0o600)

	relaunch, err := applyUpdate(appImage, file)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(appImage); string(b) != "new" {
		t.Errorf("the AppImage holds %q", b)
	}
	// Windows, where these tests run too, has no such bit.
	if fi, _ := os.Stat(appImage); runtime.GOOS != "windows" && fi.Mode().Perm()&0o111 == 0 {
		t.Errorf("the AppImage is not a program: %v", fi.Mode())
	}
	if len(relaunch) != 2 || relaunch[0] != appImage || relaunch[1] != "--after-update" {
		t.Errorf("relaunch = %v", relaunch)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("left beside the AppImage: %v", entries)
	}

	// A download that is gone leaves the AppImage as it was.
	if _, err := applyUpdate(appImage, filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing file was installed")
	}
	if b, _ := os.ReadFile(appImage); string(b) != "new" {
		t.Errorf("after a failure the AppImage holds %q", b)
	}
}

func TestUpdateKind(t *testing.T) {
	if _, err := updateKind(""); err == nil || !strings.Contains(err.Error(), ".deb") {
		t.Errorf("a packaged copy: %v", err)
	}
	if kind, err := updateKind(filepath.Join(t.TempDir(), "Freeloop.AppImage")); err != nil || kind != "appimage" {
		t.Errorf("an AppImage: %q, %v", kind, err)
	}
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		locked := filepath.Join(t.TempDir(), "locked")
		os.MkdirAll(locked, 0o500)
		defer os.Chmod(locked, 0o700)
		if _, err := updateKind(filepath.Join(locked, "Freeloop.AppImage")); err == nil {
			t.Error("an AppImage in a folder that cannot be written can be replaced")
		}
	}
}
