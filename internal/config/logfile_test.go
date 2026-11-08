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
	// Windows deletes no open file, so the folder could not be cleaned up.
	t.Cleanup(func() { l.Close() })
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

// The end of the log reaches into the file before when the current one is short,
// and reads only the end of a long one.
func TestLogTailReadsAcrossARotation(t *testing.T) {
	dir := t.TempDir()
	if got, err := LogTail(dir, 5); err != nil || len(got) != 0 {
		t.Fatalf("no log = %v, %v", got, err)
	}
	if err := os.MkdirAll(dir+"/"+LogsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	var old strings.Builder
	old.WriteString(strings.Repeat("y", 1<<20) + "\n") // a long first line, cut when read
	for i := 1; i <= 100; i++ {
		old.WriteString("old " + strings.Repeat("o", i%7) + "\n")
	}
	os.WriteFile(LogPath(dir)+".1", []byte(old.String()), 0o600)
	os.WriteFile(LogPath(dir), []byte("new 1\nnew 2\nnew 3\n"), 0o600)
	got, err := LogTail(dir, 5)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "|") != "old o|old oo|new 1|new 2|new 3" {
		t.Errorf("LogTail = %q", got)
	}
	if got, _ := LogTail(dir, 2); strings.Join(got, "|") != "new 2|new 3" {
		t.Errorf("LogTail(2) = %q", got)
	}
}
