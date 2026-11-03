package engine

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/netguard"
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
// the shell's startup files and git's own config included, since keys and tokens
// are often kept in them. What they set reaches commands through loginVars,
// without what looks like a secret.
var sandboxTools = []string{
	".gitignore_global",
	"go", ".nvm", ".cargo", ".rustup", ".pyenv", ".rbenv", ".asdf", ".volta", ".bun", ".deno", ".local/bin", ".local/share/mise",
	"Library/Developer", "Library/Python", "Library/pnpm",
}

// sandboxProtected are the project's files that something outside the sandbox
// reads and acts on: the app, git, editors and other agents. A command in the
// sandbox writing one could otherwise run with the user's full rights later.
var sandboxProtected = append([]string{ProjectDir}, permission.ActedOnLater...)

// projectCache is the project's folder in the app's caches, where a sandboxed
// command can write.
func (e *Engine) projectCache(projectID string) string {
	return filepath.Join(environment.Canonical(e.home), "Library", "Caches", "Freeloop", "sandbox", cmp.Or(projectID, "default"))
}

// sandboxEnv returns the session's environment confined as its settings say.
func (e *Engine) sandboxEnv(ctx context.Context, ses session.Session, cfg config.Environment, vars map[string]string) (*environment.Local, func(), error) {
	root := ses.WorkRoot()
	host, err := e.localEnv(root, nil)
	if err != nil {
		return nil, nil, err
	}
	root = host.Root()
	home := environment.Canonical(e.home)
	cacheBase := filepath.Join(home, "Library", "Caches", "Freeloop")
	cache, tmp, cacheEnv, err := environment.SandboxCaches(e.projectCache(ses.ProjectID))
	if err != nil {
		return nil, nil, err
	}
	c := environment.Confinement{
		Home: home, Write: []string{root}, Scratch: []string{cache, tmp},
		NoAccess:   e.broker.Policy().NeverRead(),
		ReadAlways: []string{filepath.Join(environment.Canonical(e.dataDir), config.SkillsDir)},
		Network:    environment.Network(cmp.Or(cfg.Sandbox.Network, string(environment.NetworkLocalhost))),
	}
	// The sandbox still lets commands reach only this Mac; the listed hosts are
	// through a proxy here, which refuses the rest (ENV-010).
	closeEgress := func() {}
	var networkEnv map[string]string
	reach := ""
	switch cfg.Sandbox.Network {
	case "none":
		reach = "nothing, not even localhost"
	case "hosts":
		c.Network, c.Certificates = environment.NetworkLocalhost, true
		sid := ses.ID
		egress, err := netguard.StartEgress(cfg.Sandbox.Hosts, func(h string) {
			e.emit(sid, event.Notice, event.NoticeData{Message: fmt.Sprintf("A command tried to reach %s, which the sandbox's list of hosts does not include. Add it in the environment's settings to allow it.", h)})
		})
		if err != nil {
			return nil, nil, err
		}
		e.remember(egress.Token())
		closeEgress = egress.Close
		networkEnv = environment.ProxyEnv(egress.URL())
		// Fetching means writing where a package manager keeps its downloads,
		// which the user's own builds read, so this project gets its own. Go can
		// still take what the user already has, without the network.
		networkEnv["GOMODCACHE"] = filepath.Join(cache, "go-mod")
		networkEnv["GOPROXY"] = "file://" + filepath.Join(home, "go", "pkg", "mod", "cache", "download") + ",https://proxy.golang.org"
		networkEnv["GOFLAGS"] = "-modcacherw"
		networkEnv["CARGO_HOME"] = filepath.Join(cache, "cargo")
		reach = "localhost and these hosts, through a proxy the usual tools pick up by themselves: " + strings.Join(cfg.Sandbox.Hosts, ", ")
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
	// At any depth: git runs what a nested repository's config names when it
	// starts there, and a nested project's settings become active when opened.
	quoted := regexp.QuoteMeta(root)
	c.ProtectedPatterns = append(c.ProtectedPatterns, "^"+quoted+"/.+/\\.git(/.*)?$", "^"+quoted+"/.+/\\."+regexp.QuoteMeta(strings.TrimPrefix(ProjectDir, "."))+"(/.*)?$")
	c.ProtectedPatterns = append(c.ProtectedPatterns, bareRepositoryPatterns(quoted)...)
	sid := ses.ID
	allVars := e.loginVars(ctx, host)
	// The user's name, as a global config of its own: a variable would beat what
	// the repository says. A command can change this copy, which only it reads.
	if e.loginGitConfig != "" {
		own := filepath.Join(cache, "gitconfig")
		if err := environment.WriteCacheFile(own, []byte(e.loginGitConfig)); err != nil {
			return nil, nil, err
		}
		allVars["GIT_CONFIG_GLOBAL"] = own
	}
	maps.Copy(allVars, vars)
	env, err := environment.NewLocal(environment.LocalOptions{
		ID: cfg.ID, Root: root, Env: allVars, Guard: e.broker.Policy().Guard(root),
		// No startup files: they may hold keys, so they are closed, and what they
		// set came through loginVars.
		Shell: []string{"zsh", "-fc"},
		Sandbox: &environment.SandboxOptions{
			Confiner: e.confiner, Base: c, Env: cacheEnv, NetworkEnv: networkEnv, Reach: reach,
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
	if err != nil {
		closeEgress()
		return nil, nil, err
	}
	return env, closeEgress, nil
}

// bareRepositoryPatterns close a file named HEAD anywhere in the project except
// inside .git. A folder with HEAD, objects and refs is a repository to git, whose
// config it obeys. So a command could move .git/HEAD aside, which git itself has
// to be able to replace, build one in the project, and have the user's own git
// run what its config names, unconfined. HEAD is the rarest of the three names.
// The case varies because APFS opens head as HEAD.
func bareRepositoryPatterns(quotedRoot string) []string {
	const head = "[Hh][Ee][Aa][Dd]"
	// A first folder that is not .git. One character per bracket: the sandbox's
	// regular expressions do not read [^./] as the two characters it says.
	const first = `([^.][^/]*|\.[^g][^/]*|\.g[^i][^/]*|\.gi[^t][^/]*|\.git[^/]+|\.|\.g|\.gi)`
	return []string{"^" + quotedRoot + "/" + head + "$", "^" + quotedRoot + "/" + first + "/(.*/)?" + head + "$"}
}

// gitProtected keeps a repository's config and hooks out of reach, as gitMounts
// does for a container, and stops the folders that hold them being moved aside,
// edited and moved back. Git needs the rest of its directory to commit, which
// for a worktree is outside the project. With no repository, or one whose common
// directory is not where it belongs, .git is closed entirely: one made by a
// command would be what the app's and the user's git then trust, and a planted
// .git/commondir must not choose what the sandbox can write.
func (e *Engine) gitProtected(ctx context.Context, host *environment.Local, c *environment.Confinement) []string {
	root := host.Root()
	common, ok := git(ctx, host, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if !ok || movedCommonDir(ctx, host) {
		return []string{filepath.Join(root, ".git")}
	}
	common = environment.Canonical(strings.TrimSpace(common))
	own, _ := git(ctx, host, "rev-parse", "--path-format=absolute", "--git-dir")
	own = environment.Canonical(strings.TrimSpace(own))
	if plantedGitFile(host, own) {
		return []string{filepath.Join(root, ".git")}
	}
	if !environment.Within(common, root) {
		c.Write = append(c.Write, common)
	}
	out := []string{filepath.Join(common, "modules")}
	worktrees := filepath.Join(common, "worktrees")
	c.NoMove = append(c.NoMove, common, own, worktrees)
	for _, d := range []string{common, own} {
		for _, name := range gitActive {
			out = append(out, filepath.Join(d, name))
		}
	}
	// Git runs the hooks it names, wherever that is.
	if hooks, ok := git(ctx, host, "config", "--local", "--get", "core.hooksPath"); ok {
		if hooks = strings.TrimSpace(hooks); hooks != "" {
			if !filepath.IsAbs(hooks) {
				hooks = filepath.Join(root, hooks)
			}
			out = append(out, filepath.Clean(hooks))
		}
	}
	if own == common {
		// The main checkout has no business in its linked worktrees' folders.
		out = append(out, worktrees)
	} else {
		// A worktree session writes its own folder, not the others'.
		siblings := "^" + regexp.QuoteMeta(worktrees) + "/[^/]+"
		c.ProtectedPatterns = append(c.ProtectedPatterns, siblings+"/("+strings.Join(gitActive, "|")+")(/.*)?$")
		c.NoMovePatterns = append(c.NoMovePatterns, siblings+"$")
	}
	// A worktree's .git is a file naming its directory.
	if own != filepath.Join(root, ".git") {
		out = append(out, filepath.Join(root, ".git"))
	}
	return out
}

// gitActive are the names in a git directory that decide what git runs.
var gitActive = []string{"config", "config.worktree", "hooks", "commondir", "gitdir"}

// loginVars are what the user's shell startup files set, for commands in a
// sandbox, which cannot read those files. A login shell outside the sandbox reads
// them once, with everything else the app runs that way, and what looks like a
// secret stays out: such a command needs no approval, so a prompt-injected
// printenv would otherwise hand it to the model. Git's own config is closed too,
// so it gets only the user's name from it.
func (e *Engine) loginVars(ctx context.Context, host *environment.Local) map[string]string {
	e.loginOnce.Do(func() {
		e.loginEnv = map[string]string{}
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		const marker = "__FREELOOP_ENV__"
		res, err := host.Exec(ctx, environment.ExecSpec{
			Argv: []string{"zsh", "-lc", "print -r -- " + marker + "; /usr/bin/env"},
			Env:  map[string]string{"HOME": e.home},
		})
		if err != nil || res.ExitCode != 0 {
			return
		}
		_, out, ok := strings.Cut(res.Output, marker+"\n")
		if !ok {
			return
		}
		for _, line := range strings.Split(out, "\n") {
			name, value, ok := strings.Cut(line, "=")
			if ok && envName.MatchString(name) && keepLoginVar(name, value) {
				e.loginEnv[name] = value
			}
		}
		e.loginGitConfig = ""
		for _, key := range []string{"name", "email"} {
			if v, ok := git(ctx, host, "config", "--global", "--get", "user."+key); ok && strings.TrimSpace(v) != "" {
				e.loginGitConfig += "\t" + key + " = " + strconv.Quote(strings.TrimSpace(v)) + "\n"
			}
		}
		if e.loginGitConfig != "" {
			e.loginGitConfig = "[user]\n" + e.loginGitConfig
		}
		e.loginEnv["GIT_CONFIG_GLOBAL"] = "/dev/null"
	})
	return maps.Clone(e.loginEnv)
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// keepLoginVar reports a variable a sandboxed command may have. The ones the
// app sets itself, the ones that change what programs load, and secrets stay out.
func keepLoginVar(name, value string) bool {
	switch name {
	case "HOME", "PWD", "OLDPWD", "SHLVL", "_", "TMPDIR", "USER", "LOGNAME", "SHELL", "SSH_AUTH_SOCK", "ZDOTDIR", "BASH_ENV", "ENV",
		"NODE_OPTIONS", "PYTHONSTARTUP", "RUBYOPT", "PERL5OPT", "GIT_SSH", "GIT_SSH_COMMAND", "GIT_ASKPASS", "GIT_EXTERNAL_DIFF", "GIT_PAGER", "PAGER", "EDITOR", "VISUAL":
		return false
	}
	if strings.HasPrefix(name, "DYLD_") || strings.HasPrefix(name, "LD_") || strings.HasPrefix(name, "FREELOOP_") {
		return false
	}
	return !secretValue(name, value) && !secretShaped.MatchString(value) && !longToken(value)
}

// longToken reports an unbroken run of key characters, with both letters and
// digits, long enough to be a key.
func longToken(value string) bool {
	if len(value) < 32 || strings.ContainsAny(value, "/:. ") {
		return false
	}
	return strings.IndexFunc(value, unicode.IsDigit) >= 0 && strings.IndexFunc(value, unicode.IsLetter) >= 0
}

// secretShaped is a value that looks like a key whatever its variable is called:
// the prefixes of the common services, and a long run of letters and digits (see
// longToken), which a path or a setting is not.
var secretShaped = regexp.MustCompile(`^(sk|pk|rk)[-_]|^(sk_live|sk_test|xox[a-z]|ghp|gho|ghu|ghs|ghr|github_pat|glpat|hf|npm|pypi|AKIA|ASIA|AIza|ya29|dop_v1|shpat|SG)[-_.A-Za-z0-9]{10,}|^eyJ[-_A-Za-z0-9]{10,}\.`)

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
