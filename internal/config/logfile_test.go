package config

import (
	"os"
	"strings"
	"testing"
)

// An app left running for weeks keeps its log to a size, not only at launch.
func TestLogFileMovesOnWhileRunning(t *testing.T) {
	dir := t.TempDir()
	f, err := OpenLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	l := NewLogFile(dir, f)
	rotated := 0
	l.OnRotate = func(*os.File) { rotated++ }
	line := strings.Repeat("x", 1<<20)
	for range 7 {
		if _, err := l.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	if rotated != 1 {
		t.Errorf("rotated %d times", rotated)
	}
	old, err := os.Stat(LogPath(dir) + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if old.Size() <= maxLogSize {
		t.Errorf("the old log holds %d bytes", old.Size())
	}
	now, err := os.Stat(LogPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if now.Size() >= maxLogSize {
		t.Errorf("the new log holds %d bytes", now.Size())
	}
}
