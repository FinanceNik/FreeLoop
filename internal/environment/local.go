package environment

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// LocalOptions configures a local environment.
type LocalOptions struct {
	ID   string
	Root string
	// Shell runs commands as Shell[0] Shell[1:]... <command>. Default
	// zsh -lc (OQ-019).
	Shell []string
	Env   map[string]string
	Guard Guard
}

// Local is a folder on this machine.
type Local struct {
	id    string
	root  string
	shell []string
	env   map[string]string
	fs    *localFS
}

// NewLocal returns a local environment rooted at opts.Root, which must be an
// existing directory.
func NewLocal(opts LocalOptions) (*Local, error) {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, err
	}
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	fi, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("environment: %w", err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("environment: %s is not a directory", root)
	}
	if opts.Guard == nil {
		return nil, errors.New("environment: Guard is required")
	}
	shell := opts.Shell
	if len(shell) == 0 {
		shell = []string{"zsh", "-lc"}
	}
	l := &Local{id: opts.ID, root: root, shell: shell, env: opts.Env}
	l.fs = &localFS{env: l, guard: opts.Guard}
	return l, nil
}

func (l *Local) ID() string     { return l.id }
func (l *Local) Kind() Kind     { return KindLocal }
func (l *Local) Root() string   { return l.root }
func (l *Local) FS() FileSystem { return l.fs }

// Resolve returns where p really is: absolute, clean, and with symlinks in
// its existing part resolved, so the permission broker judges the file a
// call lands on and not the link that points there.
func (l *Local) Resolve(p string) string {
	if strings.HasPrefix(p, "~/") || p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(l.root, p)
	}
	return realTarget(filepath.Clean(p))
}

// Contains reports whether p resolves to a location inside Root. Like the
// permission policy it ignores case.
func (l *Local) Contains(p string) bool {
	r, root := strings.ToLower(l.Resolve(p)), strings.ToLower(l.root)
	return r == root || strings.HasPrefix(r, root+string(filepath.Separator))
}

// rel returns p relative to Root when it is inside Root, else p.
func (l *Local) rel(p string) string {
	r, err := filepath.Rel(l.root, p)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return p
	}
	return filepath.ToSlash(r)
}

func (l *Local) Exec(ctx context.Context, spec ExecSpec) (ExecResult, error) {
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	timeout = min(timeout, MaxTimeout)
	limit := spec.MaxOutput
	if limit <= 0 {
		limit = DefaultMaxOutput
	}
	cwd := l.root
	if spec.Cwd != "" {
		cwd = l.Resolve(spec.Cwd)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var cmd *exec.Cmd
	if len(spec.Argv) > 0 {
		cmd = exec.Command(spec.Argv[0], spec.Argv[1:]...)
	} else {
		args := append(append([]string{}, l.shell[1:]...), spec.Command)
		cmd = exec.Command(l.shell[0], args...)
	}
	cmd.Dir = cwd
	cmd.Env = baseEnv()
	for k, v := range l.env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	for k, v := range spec.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out := &capped{limit: limit}
	errOut := out
	if spec.SeparateStderr {
		errOut = &capped{limit: limit}
	}
	cmd.Stdout, cmd.Stderr = out, errOut
	if spec.Stdin != nil {
		cmd.Stdin = bytes.NewReader(spec.Stdin)
	}
	setProcessGroup(cmd)
	// A descendant that leaves the process group can keep the output pipe
	// open forever; stop waiting for it shortly after the command exits.
	cmd.WaitDelay = waitDelay

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return ExecResult{}, fmt.Errorf("environment: start %s: %w", cmd.Path, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		killProcessGroup(cmd)
		waitErr = <-done
	}
	res := ExecResult{Output: out.String(), Truncated: out.truncated, Duration: time.Since(start)}
	if errOut != out {
		res.Stderr = errOut.String()
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
	}
	var exitErr *exec.ExitError
	switch {
	case waitErr == nil:
	case errors.As(waitErr, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	default:
		return res, waitErr
	}
	if ctx.Err() != nil && !res.TimedOut {
		return res, ctx.Err() // cancelled by the caller
	}
	return res, nil
}

// waitDelay bounds how long Exec waits for output after the command exits.
var waitDelay = 2 * time.Second

// baseVars are the parts of the app's own environment a command inherits;
// everything else, such as keys the app was started with, stays out.
// Configured variables and secrets come on top (ENV-002, ENV-004).
var baseVars = []string{"PATH", "HOME", "USER", "LOGNAME", "SHELL", "TMPDIR", "LANG", "TERM", "TZ", "SSH_AUTH_SOCK"}

func baseEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if slices.Contains(baseVars, k) || strings.HasPrefix(k, "LC_") {
			env = append(env, kv)
		}
	}
	return env
}

// capped keeps the first limit bytes written to it.
type capped struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (c *capped) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if room := c.limit - c.buf.Len(); room < len(p) {
		c.buf.Write(p[:max(room, 0)])
		c.truncated = true
	} else {
		c.buf.Write(p)
	}
	return len(p), nil
}

func (c *capped) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

type localFS struct {
	env   *Local
	guard Guard
}

func (f *localFS) read(p string) (string, error) {
	abs := f.env.Resolve(p)
	if err := f.guard.CheckRead(abs); err != nil {
		return "", err
	}
	// A symlink must not lead around the guard.
	if real, err := filepath.EvalSymlinks(abs); err == nil && real != abs {
		if err := f.guard.CheckRead(real); err != nil {
			return "", err
		}
	}
	return abs, nil
}

// realTarget resolves symlinks in the longest existing prefix of abs, so a
// write through a symlinked file or directory is checked where it lands.
func realTarget(abs string) string {
	dir, rest := abs, ""
	for {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return abs
		}
		rest = filepath.Join(filepath.Base(dir), rest)
		dir = parent
	}
}

// MaxReadBytes caps what ReadFile returns.
const MaxReadBytes = 10 << 20

func (f *localFS) ReadFile(p string) ([]byte, error) {
	abs, err := f.read(p)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", p)
	}
	if fi.Size() > MaxReadBytes {
		return nil, fmt.Errorf("%s is %d MB; files over %d MB are not read", p, fi.Size()>>20, MaxReadBytes>>20)
	}
	file, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	b, err := io.ReadAll(io.LimitReader(file, MaxReadBytes+1))
	if err == nil && len(b) > MaxReadBytes {
		return nil, fmt.Errorf("%s grew past %d MB while reading", p, MaxReadBytes>>20)
	}
	return b, err
}

func (f *localFS) WriteFile(p string, data []byte) error {
	abs := f.env.Resolve(p)
	if err := f.guard.CheckWrite(abs); err != nil {
		return err
	}
	if real := realTarget(abs); real != abs {
		if err := f.guard.CheckWrite(real); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(abs); err == nil {
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", p)
		}
		mode = fi.Mode().Perm()
	}
	return os.WriteFile(abs, data, mode)
}

func (f *localFS) Stat(p string) (FileInfo, error) {
	abs, err := f.read(p)
	if err != nil {
		return FileInfo{}, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{Name: fi.Name(), Path: f.env.rel(abs), IsDir: fi.IsDir(), Size: fi.Size()}, nil
}

func (f *localFS) ReadDir(p string) ([]FileInfo, error) {
	abs, err := f.read(p)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	out := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		fi := FileInfo{Name: e.Name(), Path: f.env.rel(filepath.Join(abs, e.Name())), IsDir: e.IsDir()}
		if info, err := e.Info(); err == nil {
			fi.Size = info.Size()
		}
		out = append(out, fi)
	}
	return out, nil
}

// skipDirs are never descended into by Glob and Grep.
var skipDirs = map[string]bool{".git": true, "node_modules": true}

// walk visits regular files under dir, skipping skipDirs and paths the guard
// refuses, and calls fn with the absolute path and the path relative to dir.
func (f *localFS) walk(ctx context.Context, dir string, fn func(abs, rel string) (stop bool)) error {
	root, err := f.read(dir)
	if err != nil {
		return err
	}
	errStop := errors.New("stop")
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if p != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			if f.guard.CheckRead(p) != nil {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || f.guard.CheckRead(p) != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if fn(p, filepath.ToSlash(rel)) {
			return errStop
		}
		return nil
	})
	if errors.Is(err, errStop) {
		return nil
	}
	return err
}

func (f *localFS) Glob(ctx context.Context, dir, pattern string, limit int) ([]string, error) {
	if _, err := path.Match(strings.ReplaceAll(pattern, "**", "*"), ""); err != nil {
		return nil, fmt.Errorf("bad pattern %q: %w", pattern, err)
	}
	var out []string
	err := f.walk(ctx, dir, func(abs, rel string) bool {
		if MatchGlob(pattern, rel) {
			out = append(out, f.env.rel(abs))
		}
		return limit > 0 && len(out) >= limit
	})
	sort.Strings(out)
	return out, err
}

func (f *localFS) Grep(ctx context.Context, dir, expr, include string, ignoreCase bool, limit int) ([]Match, error) {
	if ignoreCase {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("bad regular expression: %w", err)
	}
	var out []Match
	err = f.walk(ctx, dir, func(abs, rel string) bool {
		if include != "" && !MatchGlob(include, rel) && !MatchGlob(include, path.Base(rel)) {
			return false
		}
		file, err := os.Open(abs)
		if err != nil {
			return false
		}
		defer file.Close()
		sc := bufio.NewScanner(file)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for n := 1; sc.Scan(); n++ {
			line := sc.Bytes()
			if n == 1 && bytes.IndexByte(line, 0) >= 0 {
				return false // binary
			}
			if re.Match(line) {
				text := string(line)
				if len(text) > 500 {
					text = text[:500] + "…"
				}
				out = append(out, Match{Path: f.env.rel(abs), Line: n, Text: text})
				if limit > 0 && len(out) >= limit {
					return true
				}
			}
		}
		return false
	})
	return out, err
}

// MatchGlob matches a slash-separated relative path against a pattern in
// which ** matches any number of path segments and the other wildcards are
// path.Match's.
func MatchGlob(pattern, name string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := 0; i <= len(name); i++ {
				if matchSegments(pat[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, _ := path.Match(pat[0], name[0]); !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}
