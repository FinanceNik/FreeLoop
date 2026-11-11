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
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// LocalOptions configures a local environment.
type LocalOptions struct {
	ID   string
	Root string
	// Shell runs commands as Shell[0] Shell[1:]... <command>. Default
	// LoginShell().Login: zsh -lc on a Mac (OQ-019).
	Shell []string
	Env   map[string]string
	Guard Guard
	// Sandbox, when set, confines every Exec (ENV-009).
	Sandbox *SandboxOptions
	// NoSSHAgent leaves SSH_AUTH_SOCK out of what commands, agents and terminals
	// inherit, so they cannot use the user's SSH keys.
	NoSSHAgent bool
}

// Local is a folder on this machine.
type Local struct {
	// lingering are process groups a finished command left running, by group, with
	// when their first process started; StopLingering ends them (NFR-REL-005).
	lingerMu  sync.Mutex
	lingering map[int]ledgerEntry

	id    string
	root  string
	shell []string
	env   map[string]string
	fs    *localFS
	// sandbox, when set, confines every Exec.
	sandbox *SandboxOptions
	// noSSHAgent leaves SSH_AUTH_SOCK out of what the environment's processes get.
	noSSHAgent bool
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
	root = Unaliased(root)
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
		shell = LoginShell().Login
	}
	l := &Local{id: opts.ID, root: root, shell: shell, env: opts.Env, sandbox: opts.Sandbox, noSSHAgent: opts.NoSSHAgent}
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
	r, root := FoldPath(l.Resolve(p)), FoldPath(l.root)
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
	var argv []string
	if len(spec.Argv) > 0 && spec.LoginShell {
		argv = append(append(append([]string{l.shell[0]}, l.shell[1:]...), `exec "$0" "$@"`), spec.Argv...)
	} else if len(spec.Argv) > 0 {
		argv = l.viaShellIfBatch(spec.Argv)
	} else {
		argv = append(append([]string{}, l.shell...), spec.Command)
	}
	argv, sandboxEnv, err := l.confined(spec, argv)
	if err != nil {
		return ExecResult{}, err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = cwd
	cmd.Env = l.baseEnv()
	for k, v := range l.env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	for k, v := range sandboxEnv {
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
		return ExecResult{}, fmt.Errorf("environment: start %s: %w", cmd.Path, startHint(err))
	}
	if err := joinGroup(cmd); err != nil {
		killProcessGroup(cmd)
		cmd.Wait()
		return ExecResult{}, fmt.Errorf("environment: start %s: %w", cmd.Path, err)
	}
	pgid := cmd.Process.Pid
	started := track(pgid, cmd.Path)
	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		// What the command put in the background runs on in its group until the
		// session stops (StopLingering): a dev server the agent started is meant to
		// outlive the command, not the session (NFR-REL-005).
		if groupAlive(pgid) {
			linger(pgid, started)
			l.lingerMu.Lock()
			if l.lingering == nil {
				l.lingering = map[int]ledgerEntry{}
			}
			l.lingering[pgid] = ledgerEntry{Pgid: pgid, Started: started, Lingering: time.Now()}
			l.lingerMu.Unlock()
		} else {
			untrack(pgid, started)
		}
		done <- err
	}()
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

// viaShellIfBatch runs argv through the shell when its program is a Windows batch
// file, as npm's npx.cmd and most shims installed by npm are. Windows starts a batch
// file through cmd.exe, which reads the arguments again by rules of its own, so an
// argument with & or | in it would run something else; Git Bash finds the shim's
// shell-script twin instead and passes the arguments as they are.
func (l *Local) viaShellIfBatch(argv []string) []string {
	if runtime.GOOS != "windows" || !isBatch(argv[0]) {
		return argv
	}
	return append(append(append([]string{}, l.shell...), `exec "$0" "$@"`), argv...)
}

// isBatch reports a program that is, or is found on PATH as, a .cmd or .bat file.
func isBatch(program string) bool {
	if p, err := exec.LookPath(program); err == nil {
		program = p
	}
	switch strings.ToLower(filepath.Ext(program)) {
	case ".cmd", ".bat":
		return true
	}
	return false
}

// StopLingering stops what this environment's finished commands left running in the
// background, such as a dev server an agent started. A session calls it when it stops,
// and so on quitting (NFR-REL-005).
func (l *Local) StopLingering() {
	l.lingerMu.Lock()
	groups := l.lingering
	l.lingering = nil
	l.lingerMu.Unlock()
	for pgid, e := range groups {
		// A group that ended by itself may have left its number to another since.
		if ProcessStarted == nil || leftover(e) {
			stopGroup(pgid)
		}
		untrack(pgid, e.Started)
	}
}

// waitDelay bounds how long Exec waits for output after the command exits.
var waitDelay = 2 * time.Second

// baseVars are the parts of the app's own environment a command inherits;
// everything else, such as keys the app was started with, stays out.
// Configured variables and secrets come on top (ENV-002, ENV-004).
var baseVars = []string{"PATH", "HOME", "USER", "LOGNAME", "SHELL", "TMPDIR", "LANG", "TERM", "TZ", "SSH_AUTH_SOCK"}

// windowsVars are what a program on Windows needs besides: without SYSTEMROOT, for
// one, the network does not work. Windows ignores the case of a variable's name.
var windowsVars = []string{
	"SYSTEMROOT", "SYSTEMDRIVE", "WINDIR", "COMSPEC", "PATHEXT", "USERPROFILE", "USERNAME", "USERDOMAIN",
	"HOMEDRIVE", "HOMEPATH", "APPDATA", "LOCALAPPDATA", "PROGRAMDATA", "PROGRAMFILES", "PROGRAMFILES(X86)",
	"PROGRAMW6432", "COMMONPROGRAMFILES", "COMMONPROGRAMFILES(X86)", "COMMONPROGRAMW6432", "TEMP", "TMP",
	"COMPUTERNAME", "NUMBER_OF_PROCESSORS", "PROCESSOR_ARCHITECTURE", "OS",
}

func baseEnv() []string {
	return inherited(runtime.GOOS, os.Environ())
}

// inherited picks from environ what a command on goos inherits.
func inherited(goos string, environ []string) []string {
	var env []string
	for _, kv := range environ {
		k, _, _ := strings.Cut(kv, "=")
		switch {
		case goos == "windows" && k != "" && (slices.Contains(baseVars, strings.ToUpper(k)) || slices.Contains(windowsVars, strings.ToUpper(k))):
			env = append(env, kv)
		case slices.Contains(baseVars, k) || strings.HasPrefix(k, "LC_"):
			env = append(env, kv)
		}
	}
	return env
}

// baseEnv is what a process the environment starts inherits from the app: without
// SSH_AUTH_SOCK when the environment keeps the SSH agent from them.
func (l *Local) baseEnv() []string {
	env := baseEnv()
	if l.noSSHAgent {
		env = slices.DeleteFunc(env, func(kv string) bool { return strings.HasPrefix(kv, "SSH_AUTH_SOCK=") })
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
// write through a symlinked file or directory is checked where it lands. On a Mac the
// data volume's name is dropped too, as it is another spelling of the same place.
func realTarget(abs string) string {
	dir, rest := abs, ""
	for {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			return Unaliased(filepath.Join(real, rest))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Unaliased(abs)
		}
		rest = filepath.Join(filepath.Base(dir), rest)
		dir = parent
	}
}

// MaxReadBytes caps what ReadFile returns.
const MaxReadBytes = 10 << 20

// openRead opens a file for reading exactly where the guard saw it. The path is
// resolved once, the result is checked, and that path is opened with no symbolic
// link followed anywhere in it, so a folder swapped for a link between the check
// and the open, by a process racing the tool call, fails instead of leading
// around the guard. Not blocking keeps a FIFO from hanging the open.
func (f *localFS) openRead(abs string) (*os.File, error) {
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	if real != abs {
		if err := f.guard.CheckRead(real); err != nil {
			return nil, err
		}
	}
	afterCheck()
	return openNoFollow(real, os.O_RDONLY|syscall.O_NONBLOCK)
}

func (f *localFS) ReadFile(p string) ([]byte, error) {
	abs := f.env.Resolve(p)
	if err := f.guard.CheckRead(abs); err != nil {
		return nil, err
	}
	file, err := f.openRead(abs)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", p)
	}
	if fi.Size() > MaxReadBytes {
		return nil, fmt.Errorf("%s is %d MB; files over %d MB are not read", p, fi.Size()>>20, MaxReadBytes>>20)
	}
	b, err := io.ReadAll(io.LimitReader(file, MaxReadBytes+1))
	if err == nil && len(b) > MaxReadBytes {
		return nil, fmt.Errorf("%s grew past %d MB while reading", p, MaxReadBytes>>20)
	}
	return b, err
}

func (f *localFS) WriteFile(p string, data []byte) error {
	return f.writeMode(p, data, 0o644)
}

// CopyFile copies the file at rel to the same path under dst, through both
// environments' guards, keeping it as private as it is here.
func (l *Local) CopyFile(dst *Local, rel string) error {
	b, err := l.fs.ReadFile(rel)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(l.Resolve(rel)); err == nil {
		mode = fi.Mode().Perm()
	}
	return dst.fs.writeMode(rel, b, mode)
}

// WritePrivate writes a file only its owner can read when it is new, for what
// holds a user's work or settings, such as an export.
func (l *Local) WritePrivate(p string, data []byte) error {
	return l.fs.writeMode(p, data, 0o600)
}

// writeMode writes a file, new ones with mode; an existing one keeps its own.
func (f *localFS) writeMode(p string, data []byte, newMode os.FileMode) error {
	abs := f.env.Resolve(p)
	if err := f.guard.CheckWrite(abs); err != nil {
		return err
	}
	// A symlink is written where it points, checked there. One whose
	// target does not exist is refused: it would be followed to a place
	// realTarget cannot see, such as a file outside the project a cloned
	// repository's link names.
	target := abs
	if fi, err := os.Lstat(abs); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		real, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return fmt.Errorf("%s is a symbolic link to a file that does not exist; it is not written through", p)
		}
		target = real
	}
	// Every symlink in the existing part is resolved and the result checked: that
	// is the path written, and missing folders are made along it.
	real := realTarget(target)
	if real != abs {
		if err := f.guard.CheckWrite(real); err != nil {
			return err
		}
	}
	afterCheck()
	// A link put in place since the check, here or in a folder on the way, is not
	// followed, and no folder is made through one.
	file, err := createNoFollow(real, newMode)
	if err != nil {
		if errors.Is(err, errNotRegular) {
			return fmt.Errorf("%s is not a regular file", p)
		}
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	return file.Close()
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

func (f *localFS) Grep(ctx context.Context, dir, expr, include string, ignoreCase bool, limit int, opts ...GrepOption) ([]Match, error) {
	var cfg grepConfig
	for _, o := range opts {
		o(&cfg)
	}
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
		if cfg.secrets != nil {
			if shown := f.env.rel(abs); secretPath(filepath.ToSlash(shown)) {
				*cfg.secrets = append(*cfg.secrets, shown)
				return false
			}
		}
		file, err := f.openRead(abs)
		if err != nil {
			return false
		}
		defer file.Close()
		sc := bufio.NewScanner(file)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for n := 1; sc.Scan(); n++ {
			// A file with Windows line endings keeps its \r out of the match, so foo$
			// finds foo at a line's end, and out of what is shown.
			line := bytes.TrimSuffix(sc.Bytes(), []byte("\r"))
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
