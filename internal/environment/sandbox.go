package environment

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// The native sandbox (ENV-009). A local environment can run its commands under
// an operating-system confinement, so they reach the project and little else.
// The rules are the Confiner's to enforce, which lives with the platform's code
// (NFR-XPLAT-002). Agents and MCP servers the environment spawns stay outside
// it: only Exec is confined.

// Network says how far a confined command can connect.
type Network string

const (
	// NetworkLocalhost is this Mac's loopback only (OQ-028): tests and dev
	// servers work, and so does anything else listening here.
	NetworkLocalhost Network = "localhost"
	NetworkNone      Network = "none"
	// NetworkAll is for the user's own commands: setup, archive, run, hooks.
	NetworkAll Network = "all"
)

// Confinement is what a confined command may reach. Paths are absolute.
type Confinement struct {
	// Home is unreadable except for what is listed here.
	Home string
	// Write is the project: readable, and writable unless ReadOnly.
	Write []string
	// Scratch is always writable: temp files and the project's caches.
	Scratch []string
	// Protected are paths under Write that are never written, whether or not
	// they exist yet.
	Protected []string
	// Read are more readable places under Home, such as toolchains.
	Read []string
	// ProtectedPatterns are the same as regular expressions over absolute paths,
	// for deny rules with wildcards.
	ProtectedPatterns []string
	// NoAccess is neither readable nor writable, and wins over the above.
	NoAccess []string
	// NoAccessPatterns are regular expressions over absolute paths, for deny
	// rules with wildcards.
	NoAccessPatterns []string
	// ReadAlways stays readable even inside NoAccess: the app's skill store,
	// whose scripts the agent runs, in its otherwise closed data folder.
	ReadAlways []string
	Network    Network
	ReadOnly   bool
}

// Confiner wraps a command line so that it runs confined. An error means it
// must not run at all: there is no unconfined fallback.
type Confiner interface {
	Confine(c Confinement, argv []string) ([]string, error)
}

// SandboxOptions turns a local environment's Exec into confined commands.
type SandboxOptions struct {
	Confiner Confiner
	Base     Confinement
	// Env replaces, for confined commands only, the variables that name caches
	// and temp files, so nothing is written where the user's own tools read.
	Env map[string]string
	// Live adds what can change during a session: the deny rules.
	Live func(*Confinement)
	// Probe is a path a confined command must not be able to create.
	Probe string
}

// Sandboxed reports whether this environment confines its commands.
func (l *Local) Sandboxed() bool { return l.sandbox != nil }

// IsSandboxed reports whether env confines its commands.
func IsSandboxed(env Environment) bool {
	l, ok := env.(*Local)
	return ok && l.Sandboxed()
}

// confined wraps argv for spec, and says which variables go with it.
func (l *Local) confined(spec ExecSpec, argv []string) ([]string, map[string]string, error) {
	if l.sandbox == nil || spec.OutsideSandbox {
		return argv, nil, nil
	}
	if l.sandbox.Confiner == nil {
		return nil, nil, errors.New("this environment is sandboxed, and this system has no sandbox; the command did not run")
	}
	c := l.sandbox.Base
	if l.sandbox.Live != nil {
		c.NoAccessPatterns = slices.Clone(c.NoAccessPatterns)
		c.ProtectedPatterns = slices.Clone(c.ProtectedPatterns)
		l.sandbox.Live(&c)
	}
	if spec.Network != "" {
		c.Network = spec.Network
	}
	c.ReadOnly = spec.ReadOnly
	out, err := l.sandbox.Confiner.Confine(c, argv)
	if err != nil {
		return nil, nil, fmt.Errorf("the sandbox cannot run this command, so it did not run: %w", err)
	}
	return out, l.sandbox.Env, nil
}

// ConfineCommand returns a command line confined for a terminal that runs one
// command, and the variables that go with it; unchanged outside a sandbox.
func (l *Local) ConfineCommand(argv []string, network Network) ([]string, map[string]string, error) {
	return l.confined(ExecSpec{Network: network}, argv)
}

// CheckSandbox refuses a sandbox that does not hold: one the system will not
// apply, or that lets a write through. Nothing then runs (ENV-009).
func (l *Local) CheckSandbox(ctx context.Context) error {
	if l.sandbox == nil {
		return nil
	}
	if l.sandbox.Probe == "" {
		return errors.New("the sandbox has nowhere to test itself")
	}
	res, err := l.Exec(ctx, ExecSpec{Argv: []string{"/bin/sh", "-c", `: > "$1" 2>/dev/null && exit 9; exit 0`, "probe", l.sandbox.Probe}, MaxOutput: 4 << 10})
	if err != nil {
		return err
	}
	switch res.ExitCode {
	case 0:
		return nil
	case 9:
		os.Remove(l.sandbox.Probe)
		return errors.New("the sandbox let a write through that it must refuse, so commands will not run; the session needs another environment")
	}
	return fmt.Errorf("the sandbox would not start (exit %d): %s; commands will not run; the session needs another environment", res.ExitCode, res.Output)
}

// SandboxCaches makes the folders a project's confined commands keep caches and
// temp files in, and returns the variables that send them there. They are used
// only inside the sandbox: a command in it could poison a cache the user's own
// builds read, which would be a way out.
func SandboxCaches(dir string) (cache, tmp string, env map[string]string, err error) {
	cache, tmp = filepath.Join(dir, "cache"), filepath.Join(dir, "tmp")
	for _, d := range []string{cache, tmp} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return "", "", nil, fmt.Errorf("environment: %w", err)
		}
	}
	cache, tmp = Canonical(cache), Canonical(tmp)
	env = map[string]string{
		"TMPDIR":                  tmp,
		"XDG_CACHE_HOME":          cache,
		"GOCACHE":                 filepath.Join(cache, "go-build"),
		"npm_config_cache":        filepath.Join(cache, "npm"),
		"PIP_CACHE_DIR":           filepath.Join(cache, "pip"),
		"YARN_CACHE_FOLDER":       filepath.Join(cache, "yarn"),
		"HOMEBREW_CACHE":          filepath.Join(cache, "homebrew"),
		"CLANG_MODULE_CACHE_PATH": filepath.Join(cache, "clang"),
		"SWIFTPM_CACHE_DIR":       filepath.Join(cache, "swiftpm"),
	}
	return cache, tmp, maps.Clone(env), nil
}
