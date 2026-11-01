package engine

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// Worktree setup and run scripts (SES-016). A session with its own
// worktree starts without the files the repository ignores (an .env) and
// without installed dependencies. A trusted project can name files to copy
// from the main checkout and a setup command; the engine does both as soon
// as the worktree exists, and the session's first prompt waits until they
// are done. Each worktree session also gets a block of ports, so two dev
// servers never collide, and a run command started in a terminal tab.
// Commands run where the agent's own do, with the session environment's
// variables and secrets: on this Mac in the worktree, or in the session's
// container (scriptHost); an untrusted project runs nothing. Files are copied
// on the host, because the main checkout is not in the container.

// Ports: each worktree session gets portBlock ports from FirstPort up, the
// lowest block no session that is not archived holds.
const (
	FirstPort = 41000
	portBlock = 10
	maxBlocks = 500
)

// setupOutput is how much of the setup command's output an event keeps.
const setupOutput = 4000

// archiveTimeout bounds the archive command.
const archiveTimeout = time.Minute

// setupRun is a worktree setup in progress or done, while the app runs.
type setupRun struct {
	done    chan struct{}
	err     error
	skipped bool
}

type setups struct {
	mu   sync.Mutex
	runs map[string]*setupRun
}

// allocatePorts returns the first port of a free block.
func (e *Engine) allocatePorts(ctx context.Context) int {
	used, err := e.store.PortBases(ctx)
	if err != nil {
		log.Printf("engine: ports: %v", err)
	}
	taken := map[int]bool{}
	for _, p := range used {
		taken[p] = true
	}
	for i := range maxBlocks {
		if p := FirstPort + i*portBlock; !taken[p] {
			return p
		}
	}
	return 0
}

// ProjectScriptsInfo is a project's worktree scripts: those its
// .freeloop/worktree.yaml sets, those set in the app, and the ones that
// apply, which are none while the project is untrusted.
type ProjectScriptsInfo struct {
	Trusted   bool                   `json:"trusted"`
	Repo      config.WorktreeScripts `json:"repo"`
	RepoError string                 `json:"repoError,omitempty"`
	App       config.WorktreeScripts `json:"app"`
	Effective config.WorktreeScripts `json:"effective"`
}

// repoScripts reads a project's .freeloop/worktree.yaml, if any.
func (e *Engine) repoScripts(root string) (config.WorktreeScripts, error) {
	env, err := e.localEnv(root, nil)
	if err != nil {
		return config.WorktreeScripts{}, err
	}
	b, err := env.FS().ReadFile(path.Join(ProjectDir, config.WorktreeFileName))
	if err != nil {
		return config.WorktreeScripts{}, nil // no file
	}
	w, err := config.ParseWorktreeScripts(b)
	if err != nil {
		return config.WorktreeScripts{}, fmt.Errorf("%s/%s: %w", ProjectDir, config.WorktreeFileName, err)
	}
	return w, nil
}

// ProjectScripts returns a project's worktree scripts.
func (e *Engine) ProjectScripts(ctx context.Context, projectID string) (ProjectScriptsInfo, error) {
	p, err := e.store.GetProject(ctx, projectID)
	if err != nil {
		return ProjectScriptsInfo{}, err
	}
	info := ProjectScriptsInfo{Trusted: p.Trusted, Repo: config.WorktreeScripts{Copy: []string{}}, App: config.WorktreeScripts{Copy: []string{}}}
	if repo, err := e.repoScripts(p.Root); err != nil {
		info.RepoError = err.Error()
	} else if repo.Copy != nil {
		info.Repo = repo
	} else {
		repo.Copy = []string{}
		info.Repo = repo
	}
	if raw, err := e.store.ProjectScripts(ctx, projectID); err == nil && raw != "" {
		json.Unmarshal([]byte(raw), &info.App)
		if info.App.Copy == nil {
			info.App.Copy = []string{}
		}
	}
	if p.Trusted {
		info.Effective = info.Repo.Over(info.App)
	}
	if info.Effective.Copy == nil {
		info.Effective.Copy = []string{}
	}
	return info, nil
}

// SetProjectScripts records the worktree scripts set in the app for a
// project; they override its .freeloop/worktree.yaml field by field.
func (e *Engine) SetProjectScripts(ctx context.Context, projectID string, w config.WorktreeScripts) error {
	w.Setup, w.Run, w.Archive = strings.TrimSpace(w.Setup), strings.TrimSpace(w.Run), strings.TrimSpace(w.Archive)
	var copies []string
	for _, c := range w.Copy {
		if c = strings.TrimSpace(c); c != "" {
			copies = append(copies, c)
		}
	}
	w.Copy = copies
	if err := w.Validate(); err != nil {
		return err
	}
	raw := ""
	if !w.Empty() {
		b, _ := json.Marshal(w)
		raw = string(b)
	}
	return e.store.SetProjectScripts(ctx, projectID, raw)
}

// scriptsFor returns the scripts that apply to a session's worktree.
func (e *Engine) scriptsFor(ctx context.Context, ses session.Session) config.WorktreeScripts {
	if ses.Worktree == "" || ses.ProjectID == "" {
		return config.WorktreeScripts{}
	}
	info, err := e.ProjectScripts(ctx, ses.ProjectID)
	if err != nil {
		return config.WorktreeScripts{}
	}
	return info.Effective
}

// scriptEnv is what the scripts run with: the session environment's
// variables and secrets, and where they are.
func (e *Engine) scriptEnv(ses session.Session) map[string]string {
	vars := map[string]string{}
	envID := ses.EnvironmentID
	if envID == "" {
		envID = DefaultEnvironmentID
	}
	if c, err := e.environment(envID); err == nil {
		if v, err := e.resolveEnv(c); err == nil {
			vars = v
		}
	}
	vars["FREELOOP_ROOT_PATH"] = ses.ProjectRoot
	vars["FREELOOP_WORKTREE"] = ses.Worktree
	if ses.PortBase > 0 {
		vars["FREELOOP_PORT"] = strconv.Itoa(ses.PortBase)
	}
	return vars
}

// startSetup copies the files and runs the setup command of a new worktree
// session in the background; its first prompt waits for them.
func (e *Engine) startSetup(ses session.Session) {
	scripts := e.scriptsFor(context.Background(), ses)
	if len(scripts.Copy) == 0 && scripts.Setup == "" {
		return
	}
	run := &setupRun{done: make(chan struct{})}
	e.setups.mu.Lock()
	if e.setups.runs == nil {
		e.setups.runs = map[string]*setupRun{}
	}
	e.setups.runs[ses.ID] = run
	e.setups.mu.Unlock()
	e.bg.Go(func() {
		ctx, cancel := context.WithTimeout(e.scheduleCtx, dockerStartTimeout+environment.MaxTimeout+time.Minute)
		defer cancel()
		err := e.runSetup(ctx, ses, scripts)
		e.setups.mu.Lock()
		run.err = err
		e.setups.mu.Unlock()
		close(run.done)
	})
}

// runSetup does one setup and reports it.
func (e *Engine) runSetup(ctx context.Context, ses session.Session, scripts config.WorktreeScripts) error {
	d := event.WorktreeSetupData{Status: "running", Command: scripts.Setup}
	e.emit(ses.ID, event.WorktreeSetup, d)
	fail := func(err error) error {
		d.Status, d.Error = "failed", err.Error()
		e.emit(ses.ID, event.WorktreeSetup, d)
		return err
	}
	if len(scripts.Copy) > 0 {
		src, err := e.localEnv(ses.ProjectRoot, nil)
		if err != nil {
			return fail(err)
		}
		dst, err := e.localEnv(ses.Worktree, nil)
		if err != nil {
			return fail(err)
		}
		for _, c := range scripts.Copy {
			n, err := copyTree(src, dst, path.Clean(c), 0)
			switch {
			case errors.Is(err, errMissing):
				d.Missing = append(d.Missing, c)
			case err != nil:
				return fail(fmt.Errorf("copy %s: %w", c, err))
			case n > 0:
				d.Copied = append(d.Copied, c)
			}
		}
	}
	if scripts.Setup != "" {
		env, done, err := e.scriptHost(ctx, ses)
		if err != nil {
			return fail(err)
		}
		defer done()
		res, err := env.Exec(ctx, environment.ExecSpec{Command: scripts.Setup, Env: e.scriptEnv(ses), Timeout: environment.MaxTimeout, MaxOutput: 256 << 10})
		d.Output = tail(res.Output, setupOutput)
		switch {
		case err != nil:
			return fail(err)
		case res.TimedOut:
			return fail(fmt.Errorf("the setup command did not finish in %s", environment.MaxTimeout))
		case res.ExitCode != 0:
			return fail(fmt.Errorf("the setup command exited with %d", res.ExitCode))
		}
	}
	d.Status = "done"
	e.emit(ses.ID, event.WorktreeSetup, d)
	return nil
}

// waitSetup holds a session's start until its worktree setup ends, and
// refuses it after a failure the user has not chosen to ignore.
func (e *Engine) waitSetup(ctx context.Context, sid string) error {
	e.setups.mu.Lock()
	run := e.setups.runs[sid]
	e.setups.mu.Unlock()
	if run == nil {
		return nil
	}
	select {
	case <-run.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	e.setups.mu.Lock()
	defer e.setups.mu.Unlock()
	if run.err != nil && !run.skipped {
		return fmt.Errorf("the worktree setup failed (%v); retry it or start the session anyway", run.err)
	}
	return nil
}

// RetrySetup runs a worktree session's setup again.
func (e *Engine) RetrySetup(ctx context.Context, sid string) error {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return err
	}
	e.setups.mu.Lock()
	run := e.setups.runs[sid]
	e.setups.mu.Unlock()
	if run != nil {
		select {
		case <-run.done:
		default:
			return errors.New("the setup is still running")
		}
	}
	e.startSetup(ses)
	return nil
}

// containerStarted reports a session whose container has been started, and
// so may have changed its worktree.
func (e *Engine) containerStarted(ctx context.Context, sid string) bool {
	evs, err := e.store.Events(ctx, sid, 0)
	if err != nil {
		return true
	}
	return slices.ContainsFunc(evs, func(ev event.Event) bool { return ev.Type == event.EnvHealth })
}

// scriptHost returns where a session's setup and archive commands run, and
// what to call when they end. In a Docker environment that is the session's
// container, started for them and stopped again unless the session is using
// it. On the Mac they would run with the environment's secrets whatever a
// command in the container wrote into the worktree, such as a Makefile: a way
// out of the container. For the same reason they do not run on the Mac in a
// worktree a container has had, should its environment have changed since.
func (e *Engine) scriptHost(ctx context.Context, ses session.Session) (environment.Environment, func(), error) {
	envID := ses.EnvironmentID
	if envID == "" {
		envID = DefaultEnvironmentID
	}
	cfg, err := e.environment(envID)
	if err != nil {
		return nil, nil, err
	}
	if cfg.Kind != string(environment.KindDocker) {
		if e.containerStarted(ctx, ses.ID) {
			return nil, nil, errors.New("a container has had this worktree and the session's environment is no longer Docker, so this command does not run on this Mac")
		}
		host, err := e.localEnv(ses.Worktree, nil)
		return host, func() {}, err
	}
	d, err := e.sessionContainer(ctx, ses, cfg)
	if err != nil {
		return nil, nil, err
	}
	return d, func() {
		e.mu.Lock()
		inUse := e.runtimes[ses.ID] != nil || e.starting[ses.ID] != nil
		e.mu.Unlock()
		if !inUse {
			sctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			d.Stop(sctx)
		}
	}, nil
}

// SkipSetup lets a session whose setup failed start anyway.
func (e *Engine) SkipSetup(ctx context.Context, sid string) error {
	e.setups.mu.Lock()
	run := e.setups.runs[sid]
	if run != nil {
		run.skipped = true
	}
	e.setups.mu.Unlock()
	if run != nil {
		e.emit(sid, event.WorktreeSetup, event.WorktreeSetupData{Status: "skipped"})
	}
	return nil
}

// RunWorktreeScript starts a session's run command in a terminal tab.
func (e *Engine) RunWorktreeScript(ctx context.Context, sid string) (TerminalInfo, error) {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return TerminalInfo{}, err
	}
	run := e.scriptsFor(ctx, ses).Run
	if run == "" {
		return TerminalInfo{}, errors.New("the project has no run command; set one in its worktree scripts")
	}
	return e.openTerminal(ctx, sid, run)
}

// runArchiveScript runs the archive command before a worktree is removed;
// a failure is reported, never a reason to keep the worktree.
func (e *Engine) runArchiveScript(ctx context.Context, ses session.Session) {
	cmd := e.scriptsFor(ctx, ses).Archive
	if cmd == "" || !environment.IsDir(ses.Worktree) {
		return
	}
	notice := func(why, output string) {
		e.emit(ses.ID, event.Notice, event.NoticeData{Message: fmt.Sprintf("The archive command %s: %s", why, tail(output, 500))})
	}
	// A container that never started has nothing to clean up, and starting
	// one now could mean pulling its image.
	if c, err := e.environment(cmp.Or(ses.EnvironmentID, DefaultEnvironmentID)); err == nil && c.Kind == string(environment.KindDocker) && !e.containerStarted(ctx, ses.ID) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, archiveTimeout+time.Minute)
	defer cancel()
	env, done, err := e.scriptHost(ctx, ses)
	if err != nil {
		notice("did not run", err.Error())
		return
	}
	defer done()
	res, err := env.Exec(ctx, environment.ExecSpec{Command: cmd, Env: e.scriptEnv(ses), Timeout: archiveTimeout})
	switch {
	case err != nil:
		notice(err.Error(), res.Output)
	case res.TimedOut:
		notice("did not finish in a minute", res.Output)
	case res.ExitCode != 0:
		notice(fmt.Sprintf("exited with %d", res.ExitCode), res.Output)
	}
}

var errMissing = errors.New("missing")

// maxCopied bounds what one copy entry brings over.
const maxCopied = 2000

// copyTree copies a file, or a folder's files, from one checkout to the
// other, at most maxCopied files and maxDepth folders deep (a symlink loop
// ends there). It returns how many files it copied. A symlink leading out of
// the checkout is left out: a repository can hold one that names any file of
// the user's, and a copy would hand it to the agent without the prompt a read
// outside the project takes.
func copyTree(src, dst *environment.Local, rel string, n int) (int, error) {
	return copyDepth(src, dst, rel, n, 0)
}

const maxDepth = 12

func copyDepth(src, dst *environment.Local, rel string, n, depth int) (int, error) {
	fi, err := src.FS().Stat(rel)
	if err != nil || !src.Contains(rel) {
		if depth == 0 {
			return 0, errMissing
		}
		return n, nil
	}
	if !fi.IsDir {
		return n + 1, src.CopyFile(dst, rel)
	}
	if depth >= maxDepth {
		return n, nil
	}
	entries, err := src.FS().ReadDir(rel)
	if err != nil {
		return n, err
	}
	for _, de := range entries {
		if n >= maxCopied {
			return n, fmt.Errorf("more than %d files", maxCopied)
		}
		if n, err = copyDepth(src, dst, path.Join(rel, de.Name), n, depth+1); err != nil {
			return n, err
		}
	}
	return n, nil
}

// cutBytes cuts s to at most n bytes at a character boundary.
func cutBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}

// tail keeps the end of s.
func tail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := len(s) - max
	for cut < len(s) && s[cut]&0xC0 == 0x80 {
		cut++
	}
	return "…" + s[cut:]
}
