package engine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/netguard"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// SSH environments (ENV-008): a session's commands run on another machine, in a working
// copy kept in sync with the project here. The project stays on this Mac, so the app's
// git, diffs, checkpoints and file tools work as they do for a local folder. The other
// machine is not trusted: see internal/environment/sshsync.go and docs/SECURITY.md.
// The broker asks about commands there as it does here.

const (
	// sshStartTimeout covers the first copy of a project and its setup command.
	sshStartTimeout = 20 * time.Minute
	// sshListLimit bounds the files of a folder that is not a git repository.
	sshListLimit = 200_000
)

func isSSH(c config.Environment) bool { return c.Kind == string(environment.KindSSH) }

// sshDir holds the sockets of shared connections, private to the user. It is short,
// since a socket's path has a low limit.
func (e *Engine) sshDir() string {
	dir := filepath.Join(environment.Canonical(e.home), "Library", "Caches", "Freeloop", "ssh")
	if err := environment.PrivateCache(dir); err != nil {
		return ""
	}
	return dir
}

// sshSetAsideDir keeps what a session's sync replaced or deleted here; it goes with the
// session.
func (e *Engine) sshSetAsideDir(sid string) string {
	dir := filepath.Join(environment.Canonical(e.home), "Library", "Caches", "Freeloop", "ssh-replaced", sid)
	if err := environment.PrivateCache(dir); err != nil {
		return ""
	}
	return dir
}

// sshOptionsFor is how a session reaches its working copy.
func (e *Engine) sshOptionsFor(ses session.Session, cfg config.Environment, vars map[string]string, host *environment.Local) environment.SSHOptions {
	root := ses.WorkRoot()
	rel := func(p string) string { return filepath.Join(root, filepath.FromSlash(p)) }
	return environment.SSHOptions{
		ID: cfg.ID, Host: cfg.Host, RemoteDir: cfg.RemoteDir, SessionID: ses.ID, Root: root,
		Env: maps.Clone(vars), Setup: cfg.Setup, Health: cfg.Health, Guard: e.broker.Policy().Guard(root),
		ControlDir: e.sshDir(), ConfigFile: e.sshConfig,
		Allowed:   func() error { return e.sshAllowed(cfg) },
		ListFiles: func(ctx context.Context) ([]string, error) { return projectFiles(ctx, host) },
		Ignored:   func(ctx context.Context, paths []string) ([]string, error) { return ignoredFiles(ctx, host, paths) },
		NoRead:    func(p string) bool { return e.broker.ReadDenied(ses.ID, root, rel(p)) },
		NoWrite:   func(p string) bool { return e.broker.WriteDenied(ses.ID, root, rel(p)) },
		ActedOn:   permission.ActedOnLater, SetAsideDir: e.sshSetAsideDir(ses.ID),
		Notify: func(m string) { e.emit(ses.ID, event.Notice, event.NoticeData{Message: m}) },
	}
}

// sshAllowed refuses a machine that local-only mode rules out: anything but this one.
// Where a name leads comes from ssh itself, and one that goes through a proxy cannot be
// told.
func (e *Engine) sshAllowed(cfg config.Environment) error {
	if !netguard.LocalOnly() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	addr, err := environment.ResolvedSSHHost(ctx, cfg.Host, e.sshConfig)
	if err != nil {
		return fmt.Errorf("local-only mode is on: %w", err)
	}
	if !netguard.Local(addr) {
		return fmt.Errorf("local-only mode is on, and %s is at %s", cfg.Host, addr)
	}
	return nil
}

// sshEnv connects a session to its working copy, and records what it did.
func (e *Engine) sshEnv(ctx context.Context, ses session.Session, cfg config.Environment, vars map[string]string, host *environment.Local) (*environment.SSH, error) {
	env, err := environment.NewSSH(e.sshOptionsFor(ses, cfg, vars, host))
	if err != nil {
		return nil, err
	}
	sctx, cancel := context.WithTimeout(ctx, sshStartTimeout)
	defer cancel()
	did, err := env.Start(sctx)
	health := map[string]string{"status": "healthy", "host": cfg.Host}
	if did != "" {
		health["detail"] = did
	}
	if err != nil {
		health["status"], health["detail"] = "unhealthy", err.Error()
		e.emit(ses.ID, event.EnvHealth, health)
		return nil, fmt.Errorf("the %s environment cannot start: %w", cfg.Name, err)
	}
	e.emit(ses.ID, event.EnvHealth, health)
	return env, nil
}

// removeSSHCopy deletes a session's working copy when the session goes. A machine that
// cannot be reached keeps it, which the log says, and a session never fails to delete
// for it.
func (e *Engine) removeSSHCopy(ctx context.Context, ses session.Session) error {
	// Only an environment the user approved as it is now. Its file may have changed: no
	// connection goes to a machine the user did not choose.
	cfg, err := e.usableEnvironment(ses.EnvironmentID)
	if err != nil || !isSSH(cfg) {
		return nil
	}
	if err := e.sshAllowed(cfg); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// It needs a folder to be rooted in, and the session's may be gone.
	opts := environment.SSHOptions{ID: cfg.ID, Host: cfg.Host, RemoteDir: cfg.RemoteDir, SessionID: ses.ID, Root: "/", Guard: e.broker.Policy().Guard("/"),
		ControlDir: e.sshDir(), ConfigFile: e.sshConfig,
		ListFiles: func(context.Context) ([]string, error) { return nil, nil }}
	return environment.RemoveSSHCopy(ctx, opts)
}

// projectFiles lists what a project sends to a working copy. In a repository that is
// what git tracks or would track, so what it ignores stays here. Its git is the app's
// guarded one, which runs nothing a repository configures.
func projectFiles(ctx context.Context, host *environment.Local) ([]string, error) {
	repo, err := inRepository(ctx, host)
	if err != nil {
		return nil, err
	}
	if !repo {
		return host.FS().Glob(ctx, "", "**", sshListLimit)
	}
	// A repository git cannot read is not a folder of files: everything it ignores would
	// go with them.
	argv := append(append([]string{}, safeGit...), "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	res, err := host.Exec(ctx, environment.ExecSpec{Argv: argv, Env: gitEnv(nil), Timeout: 2 * time.Minute, MaxOutput: 128 << 20, SeparateStderr: true})
	switch {
	case err != nil:
		return nil, err
	case res.Truncated:
		return nil, errors.New("the project has more files than can be copied")
	case res.ExitCode != 0:
		return nil, fmt.Errorf("git cannot list the project's files, so nothing is copied: %s", strings.TrimSpace(res.Stderr))
	}
	return splitNul(res.Output), nil
}

// inRepository says whether a folder is in a git repository. Only git saying it is not
// counts as no: any other failure is an error, and nothing is decided on it.
func inRepository(ctx context.Context, host *environment.Local) (bool, error) {
	argv := append(append([]string{}, safeGit...), "rev-parse", "--is-inside-work-tree")
	res, err := host.Exec(ctx, environment.ExecSpec{Argv: argv, Env: gitEnv(nil), Timeout: time.Minute, SeparateStderr: true})
	if err != nil {
		return false, err
	}
	if res.ExitCode == 0 {
		return strings.TrimSpace(res.Output) == "true", nil
	}
	// A .git here that git rejects, such as a worktree's whose checkout has gone, is a
	// repository it cannot read. Taking it for a plain folder would send what it ignores.
	dotGit := filepath.Join(host.Root(), ".git")
	if strings.Contains(res.Stderr, "not a git repository") && !environment.IsDir(dotGit) && !environment.IsFile(dotGit) {
		return false, nil
	}
	return false, fmt.Errorf("git cannot read the project: %s", strings.TrimSpace(res.Stderr))
}

// ignoredFiles returns those of paths the project here ignores.
func ignoredFiles(ctx context.Context, host *environment.Local, paths []string) ([]string, error) {
	repo, err := inRepository(ctx, host)
	if err != nil {
		return nil, err
	}
	if !repo {
		return nil, nil
	}
	// A name is only a name: a file called :(bogus) is otherwise a pathspec git dies on,
	// and check-ignore has no literal mode. With ./ in front it is read as a path.
	var names strings.Builder
	for _, p := range paths {
		names.WriteString("./" + p + "\x00")
	}
	argv := append(append([]string{}, safeGit...), "check-ignore", "-z", "--stdin")
	res, err := host.Exec(ctx, environment.ExecSpec{Argv: argv, Env: gitEnv(nil), Timeout: time.Minute, MaxOutput: 128 << 20, Stdin: []byte(names.String()), SeparateStderr: true})
	switch {
	case err != nil:
		return nil, err
	case res.TimedOut, res.Truncated:
		return nil, errors.New("git did not finish")
	case res.ExitCode == 0:
		ignored := splitNul(res.Output)
		for i, p := range ignored {
			ignored[i] = strings.TrimPrefix(p, "./")
		}
		return ignored, nil
	case res.ExitCode == 1:
		return nil, nil // none of them
	}
	return nil, fmt.Errorf("git check-ignore: %s", strings.TrimSpace(res.Stderr))
}

func splitNul(s string) []string {
	var out []string
	for _, p := range strings.Split(s, "\x00") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
