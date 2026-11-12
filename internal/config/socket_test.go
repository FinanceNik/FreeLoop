package config

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// deepDir is a data folder whose socket path would be too long for a Unix socket.
func deepDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), strings.Repeat("deeper", 20))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// SEC-018: a data folder too deep for a socket path puts the socket in a
// folder made for this user, with mode 0700 and a name nobody can guess, never
// in a name derived from the data folder in a shared temporary folder.
func TestFallbackSocketIsInAPrivateFolder(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TEMP", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("XDG_RUNTIME_DIR", "")
	dir := deepDir(t)

	path, err := PrepareSocketPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(path) >= 104 {
		t.Errorf("%q is too long for a Unix socket (%d bytes)", path, len(path))
	}
	sum := sha256.Sum256([]byte(dir))
	guess := filepath.Join(os.TempDir(), "freeloop-"+hex.EncodeToString(sum[:6])+".sock")
	if path == guess || filepath.Dir(path) == filepath.Clean(os.TempDir()) {
		t.Errorf("the socket %q is in a shared folder under a name derived from the data folder", path)
	}
	fi, err := os.Lstat(filepath.Dir(path))
	if err != nil || !fi.IsDir() {
		t.Fatalf("the socket's folder: %v, %v", fi, err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o700 {
		t.Errorf("the socket's folder has mode %v, want 0700", fi.Mode().Perm())
	}
	// The command line and the hook find it from the data folder alone.
	if got := SocketPath(dir); got != path {
		t.Errorf("SocketPath = %q, want %q", got, path)
	}
	// Another data folder, or another start, gets another name.
	other, err := PrepareSocketPath(deepDir(t))
	if err != nil {
		t.Fatal(err)
	}
	again, err := PrepareSocketPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if other == path || again == path {
		t.Errorf("names repeat: %q %q %q", path, other, again)
	}
	if _, err := os.Stat(filepath.Dir(path)); err == nil {
		t.Errorf("the folder of the last start was left behind")
	}
}

// A data folder that is short enough keeps its socket inside it.
func TestShortDataFolderKeepsItsSocket(t *testing.T) {
	dir, err := os.MkdirTemp("", "fl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	want := filepath.Join(dir, SocketFileName)
	if len(want) >= 100 {
		t.Skip("the temporary folder is too deep for this test")
	}
	if got, err := PrepareSocketPath(dir); err != nil || got != want {
		t.Errorf("PrepareSocketPath = %q, %v", got, err)
	}
	if got := SocketPath(dir); got != want {
		t.Errorf("SocketPath = %q", got)
	}
}
