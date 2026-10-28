// Package environment is where a session's processes run and its files live
// (ENV-001). It is the only engine package allowed to start processes, and
// every file access a harness makes goes through it; internal/archtest
// enforces that. Each FileSystem call passes the Guard first, so protected
// paths hold for every caller, including external agents' fs/* requests.
package environment

import (
	"context"
	"errors"
	"path/filepath"
	"time"
)

// Kind is the kind of environment.
type Kind string

const (
	KindLocal Kind = "local"
)

// Environment is a place to run a session.
type Environment interface {
	ID() string
	Kind() Kind
	// Root is the project root as an absolute path inside the environment.
	Root() string
	// Resolve turns a path relative to Root (or absolute) into a clean
	// absolute path. It does not check policy.
	Resolve(path string) string
	Exec(ctx context.Context, spec ExecSpec) (ExecResult, error)
	FS() FileSystem
}

// ExecSpec describes one shell command.
type ExecSpec struct {
	Command string
	// Cwd defaults to Root.
	Cwd string
	// Env adds variables to the environment's own.
	Env map[string]string
	// Timeout defaults to DefaultTimeout and is capped at MaxTimeout.
	Timeout time.Duration
	// MaxOutput caps the combined output kept, in bytes. Default 30 KB.
	MaxOutput int
}

// ExecResult is a finished command.
type ExecResult struct {
	Output    string // stdout and stderr interleaved
	ExitCode  int
	TimedOut  bool
	Truncated bool
	Duration  time.Duration
}

const (
	DefaultTimeout   = 2 * time.Minute
	MaxTimeout       = 10 * time.Minute
	DefaultMaxOutput = 30 * 1024
)

// FileSystem is a session's view of files. Paths may be relative to Root.
type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	// WriteFile creates parent directories as needed.
	WriteFile(path string, data []byte) error
	Stat(path string) (FileInfo, error)
	ReadDir(path string) ([]FileInfo, error)
	// Glob matches pattern (with ** for any depth) under dir and returns
	// paths relative to Root, sorted, at most limit.
	Glob(ctx context.Context, dir, pattern string, limit int) ([]string, error)
	// Grep searches files under dir whose relative path matches include (all
	// files when empty) for the regular expression and returns at most limit
	// matches.
	Grep(ctx context.Context, dir, expr, include string, ignoreCase bool, limit int) ([]Match, error)
}

// FileInfo describes one file or directory.
type FileInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"` // relative to Root when inside it, else absolute
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

// Match is one grep hit.
type Match struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Guard is the path policy every FileSystem call checks first. Paths are
// absolute and clean.
type Guard interface {
	CheckRead(path string) error
	CheckWrite(path string) error
}

// ErrProtected wraps a Guard refusal.
var ErrProtected = errors.New("protected path")

// Spellings returns the clean form of p and, when it differs, its
// symlink-free form, so a path check holds whichever one a caller resolves
// to (on macOS /var is a link to /private/var, for example).
func Spellings(p string) []string {
	p = filepath.Clean(p)
	out := []string{p}
	if real, err := filepath.EvalSymlinks(p); err == nil && real != p {
		out = append(out, real)
	}
	return out
}
