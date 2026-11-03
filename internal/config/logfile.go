package config

import (
	"fmt"
	"os"
	"path/filepath"
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
