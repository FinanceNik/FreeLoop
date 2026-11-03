package config

import (
	"fmt"
	"os"
	"path/filepath"
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
