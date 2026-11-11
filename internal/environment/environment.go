// Package environment is where a session's processes run and its files live
// (ENV-001). It is the only engine package allowed to start processes, and
// every file access a harness makes goes through it; internal/archtest
// enforces that. Each FileSystem call passes the Guard first, so protected
// paths hold for every caller, including external agents' fs/* requests.
package environment

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	// Contains reports whether path resolves to a location inside Root.
	Contains(path string) bool
	Exec(ctx context.Context, spec ExecSpec) (ExecResult, error)
	FS() FileSystem
}

// ExecSpec describes one shell command.
type ExecSpec struct {
	Command string
	// Argv, when set, runs a program directly instead of Command through the
	// shell; the app uses it for its own git queries.
	Argv []string
	// LoginShell runs Argv through the environment's shell as
	// `exec "$0" "$@"`, so a program the user installed, such as gh,
	// resolves on their login PATH while the arguments stay as given.
	LoginShell bool
	// Cwd defaults to Root.
	Cwd string
	// Env adds variables to the environment's own.
	Env map[string]string
	// Timeout defaults to DefaultTimeout and is capped at MaxTimeout.
	Timeout time.Duration
	// MaxOutput caps the combined output kept, in bytes. Default 30 KB.
	MaxOutput int
	// Stdin, when set, is the command's standard input.
	Stdin []byte
	// SeparateStderr keeps standard error apart, in ExecResult.Stderr;
	// Output then holds standard output only (hooks, EXT-005).
	SeparateStderr bool
	// In a sandboxed environment (ENV-009): Network replaces its reach for the
	// user's own commands; ReadOnly also keeps the project from being written;
	// OutsideSandbox runs the command unconfined, once the user has approved
	// that.
	Network        Network
	ReadOnly       bool
	OutsideSandbox bool
}

// ExecResult is a finished command.
type ExecResult struct {
	Output    string // stdout and stderr interleaved, unless SeparateStderr
	Stderr    string
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

// GrepOption changes what a search looks at.
type GrepOption func(*grepConfig)

type grepConfig struct{ secrets *[]string }

// SkipSecrets leaves out the files whose names say they hold secrets (SecretPath) and
// appends their paths, as they are met, to *left, so a search over a folder shows what
// the user's yes to a search covered and says what it did not.
func SkipSecrets(left *[]string) GrepOption {
	return func(c *grepConfig) { c.secrets = left }
}

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
	Grep(ctx context.Context, dir, expr, include string, ignoreCase bool, limit int, opts ...GrepOption) ([]Match, error)
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

// IsDir reports whether path is an existing directory on this machine.
func IsDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// Canonical returns path absolute, clean and with symlinks resolved when it
// exists, the form project roots are stored in.
func Canonical(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return Unaliased(real)
	}
	return Unaliased(filepath.Clean(path))
}

// Within reports whether p is dir or inside it, both resolved like
// Canonical.
func Within(p, dir string) bool {
	rel, err := filepath.Rel(Canonical(dir), Canonical(p))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// Spellings returns the clean form of p and, when it differs, its
// symlink-free form, so a path check holds whichever one a caller resolves
// to (on macOS /var is a link to /private/var, for example).
//
// The existing part of p is resolved, so a protected directory that does
// not exist yet (a ~/.ssh created after the app started) is still covered
// under its resolved spelling.
func Spellings(p string) []string {
	p = filepath.Clean(p)
	out := []string{p}
	if real := realTarget(p); real != p {
		out = append(out, real)
	}
	return out
}
