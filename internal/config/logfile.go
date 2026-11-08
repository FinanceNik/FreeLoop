package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// LogsDir holds the app's log, which stays on this Mac (NFR-PRIV-001).
const LogsDir = "logs"

const maxLogSize = 5 << 20

// LogPath is the current log file.
func LogPath(dir string) string { return filepath.Join(dir, LogsDir, "freeloop.log") }

// OpenLog opens the log for appending, moving an old one aside when it has grown.
func OpenLog(dir string) (*os.File, error) {
	path := LogPath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxLogSize {
		os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return f, nil
}

// LogFile is the log of an app that may run for weeks: past the size limit it moves
// the file aside and starts another, as a launch would.
type LogFile struct {
	mu   sync.Mutex
	dir  string
	f    *os.File
	size int64
	// OnRotate is told of each new file, for what writes to it directly.
	OnRotate func(*os.File)
}

// NewLogFile wraps a log OpenLog opened in dir.
func NewLogFile(dir string, f *os.File) *LogFile {
	l := &LogFile{dir: dir, f: f}
	if fi, err := f.Stat(); err == nil {
		l.size = fi.Size()
	}
	return l
}

func (l *LogFile) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.size > maxLogSize {
		// If it cannot move on, the old file is still better than no log.
		if next, err := OpenLog(l.dir); err == nil {
			l.f.Close()
			l.f, l.size = next, 0
			if l.OnRotate != nil {
				l.OnRotate(next)
			}
		}
	}
	n, err := l.f.Write(b)
	l.size += int64(n)
	return n, err
}

// LogTail returns the last n lines of the log, reaching into the file before the
// current one when the current one holds fewer, oldest first. A missing log is no
// lines. Only the end of each file is read, so a full log costs little.
func LogTail(dir string, n int) ([]string, error) {
	var lines []string
	for _, path := range []string{LogPath(dir), LogPath(dir) + ".1"} {
		if len(lines) >= n {
			break
		}
		got, err := tailLines(path, n-len(lines))
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("config: %w", err)
		}
		lines = append(got, lines...)
	}
	return lines, nil
}

// tailLines returns up to the last n lines of the file at path.
func tailLines(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	// Lines of the log are short; 400 bytes a line is plenty, and a longer line
	// is cut at the start of what was read.
	size := min(fi.Size(), int64(n)*400+1)
	buf := make([]byte, size)
	if _, err := f.ReadAt(buf, fi.Size()-size); err != nil && err != io.EOF {
		return nil, err
	}
	text := strings.TrimRight(string(buf), "\n")
	if text == "" {
		return nil, nil
	}
	all := strings.Split(text, "\n")
	if size < fi.Size() && len(all) > 1 {
		all = all[1:] // the first is likely cut
	}
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}
