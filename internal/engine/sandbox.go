package engine

import (
	"cmp"
	"context"
	"path/filepath"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// The native sandbox (ENV-009, OQ-028): a local environment can confine its
// commands to the project, without Docker. What it reaches is decided here and
// enforced by Options.Confiner. The app's own git commands keep using the plain
// environment, under their own guards.

// SandboxedEnvironmentID is the sandboxed environment every install offers.
const SandboxedEnvironmentID = "sandboxed"

func sandboxedEnvironment() config.Environment {
	return config.Environment{ID: SandboxedEnvironmentID, Name: "Local, sandboxed", Kind: "local", Sandbox: &config.Sandbox{Enabled: true}}
}

// sandboxTools are the folders under home a confined command can read: what a
// login shell and the usual toolchains need. Everything else in home is closed,
// the shell's interactive startup files included, since keys are often kept in
// them, and npm's, where tokens are.
var sandboxTools = []string{
	".zshenv", ".zprofile", ".zlogin", ".gitconfig", ".gitignore_global", ".config/git",
	"go", ".nvm", ".cargo", ".rustup", ".pyenv", ".rbenv", ".asdf", ".volta", ".bun", ".deno", ".local/bin", ".local/share/mise",
	"Library/Developer", "Library/Python", "Library/pnpm",
}

// sandboxProtected are the project's files that something outside the sandbox
// reads and acts on: the app, git, editors and other agents. A command in the
// sandbox writing one could otherwise run with the user's full rights later.
var sandboxProtected = []string{
	ProjectDir, ".devcontainer", ".devcontainer.json", ".mcp.json", ".claude", ".envrc", ".vscode",
}

// sandboxEnv returns the session's environment confined as its settings say.
func (e *Engine) sandboxEnv(ctx context.Context, ses session.Session, cfg config.Environment, vars map[string]string) (*environment.Local, error) {
	root := ses.WorkRoot()
	host, err := e.localEnv(root, nil)
	if err != nil {
		return nil, err
	}
	root = host.Root()
	home := environment.Canonical(e.home)
	cacheBase := filepath.Join(home, "Library", "Caches", "Freeloop")
	cache, tmp, cacheEnv, err := environment.SandboxCaches(filepath.Join(cacheBase, "sandbox", cmp.Or(ses.ProjectID, "default")))
	if err != nil {
		return nil, err
	}
	c := environment.Confinement{
		Home: home, Write: []string{root}, Scratch: []string{cache, tmp, "/private/tmp"},
		NoAccess:   e.broker.Policy().NeverRead(),
		ReadAlways: []string{filepath.Join(environment.Canonical(e.dataDir), config.SkillsDir)},
		Network:    environment.Network(cmp.Or(cfg.Sandbox.Network, string(environment.NetworkLocalhost))),
	}
	for _, p := range sandboxTools {
		c.Read = append(c.Read, filepath.Join(home, filepath.FromSlash(p)))
	}
	for _, p := range cfg.Sandbox.Read {
		c.Read = append(c.Read, expandHome(home, p))
	}
	for _, p := range cfg.Sandbox.Write {
		c.Write = append(c.Write, expandHome(home, p))
	}
	for _, p := range sandboxProtected {
		c.Protected = append(c.Protected, filepath.Join(root, p))
	}
	c.Protected = append(c.Protected, e.gitProtected(ctx, host, &c)...)
	sid := ses.ID
	return environment.NewLocal(environment.LocalOptions{
		ID: cfg.ID, Root: root, Env: vars, Guard: e.broker.Policy().Guard(root),
		Sandbox: &environment.SandboxOptions{
			Confiner: e.confiner, Base: c, Env: cacheEnv,
			Probe: filepath.Join(cacheBase, ".sandbox-probe"),
			Live: func(c *environment.Confinement) {
				rules, err := e.rules.Rules(sid)
				if err != nil {
					return // the broker then asks about every command
				}
				read, write := permission.DeniedPathPatterns(rules, root)
				c.NoAccessPatterns = append(c.NoAccessPatterns, read...)
				c.ProtectedPatterns = append(c.ProtectedPatterns, write...)
			},
		},
	})
}

// gitProtected keeps a repository's config and hooks out of reach, as gitMounts
// does for a container. Git needs the rest of its directory to commit, which for
// a worktree is outside the project. With no repository, .git is closed entirely:
// one made by a command would be what the app's and the user's git then trust.
func (e *Engine) gitProtected(ctx context.Context, host *environment.Local, c *environment.Confinement) []string {
	root := host.Root()
	common, ok := git(ctx, host, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if !ok {
		return []string{filepath.Join(root, ".git")}
	}
	common = environment.Canonical(strings.TrimSpace(common))
	own, _ := git(ctx, host, "rev-parse", "--path-format=absolute", "--git-dir")
	own = environment.Canonical(strings.TrimSpace(own))
	if !environment.Within(common, root) {
		c.Write = append(c.Write, common)
	}
	out := []string{}
	for _, d := range []string{common, own} {
		for _, name := range []string{"config", "config.worktree", "hooks", "commondir", "gitdir"} {
			out = append(out, filepath.Join(d, name))
		}
	}
	// A worktree's .git is a file naming its directory.
	if own != filepath.Join(root, ".git") {
		out = append(out, filepath.Join(root, ".git"))
	}
	return out
}

func expandHome(home, p string) string {
	if p == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return environment.Canonical(p)
}

// startSandbox checks the sandbox holds before anything runs in it, and says so
// in the transcript. There is no running without it (ENV-009).
func (e *Engine) startSandbox(ctx context.Context, sid string, env *environment.Local) error {
	if err := env.CheckSandbox(ctx); err != nil {
		e.emit(sid, event.EnvHealth, map[string]string{"status": "unhealthy", "detail": err.Error()})
		return err
	}
	e.emit(sid, event.EnvHealth, map[string]string{"status": "healthy", "detail": "commands run in a sandbox, inside the project"})
	return nil
}
