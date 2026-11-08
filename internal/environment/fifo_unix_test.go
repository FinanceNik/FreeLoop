//go:build unix

package environment

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO must not hang a read.
func TestReadingAFIFODoesNotHang(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0o644); err != nil {
		t.Skip(err)
	}
	l, err := NewLocal(LocalOptions{Root: root, Guard: rootOnlyGuard{environmentCanonical(root)}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := l.FS().ReadFile("pipe")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a FIFO was read as a file")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reading a FIFO hung")
	}
}
