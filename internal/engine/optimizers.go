package engine

import (
	"cmp"
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// Token optimizers (HRN-019): the broker approves a shell command as the model
// wrote it, and only then is the optimizer asked for a cheaper form of it. A
// rewrite that is not plainly the same command runs as the original instead, as
// does every failure. Only a plain command line is rewritten at all: where
// output feeds a pipe, a file or a substitution, shortening it would change what
// the command does.

const (
	rewriteTimeout = 2 * time.Second
	probeTimeout   = 10 * time.Second
	// maxRewriteInput keeps a huge command from failing as an argument.
	maxRewriteInput = 64 << 10
	// neutralDir is where the program is asked and looked for: a project's
	// files, its .claude/settings.json among them, must not change the answer.
	neutralDir = "/"
	// probeRetry is how soon a missing program is looked for again while
	// commands run; choosing one always looks.
	probeRetry = time.Minute
)

func (e *Engine) loadOptimizers() {
	list, issues := config.LoadOptimizers(e.dataDir)
	e.mu.Lock()
	e.optimizers = list
	e.issues[issueOptimizers] = issues
	e.mu.Unlock()
}

func (e *Engine) optimizerList() []config.Optimizer {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.optimizers)
}

func (e *Engine) findOptimizer(id string) (config.Optimizer, bool) {
	list := e.optimizerList()
	i := slices.IndexFunc(list, func(o config.Optimizer) bool { return o.ID == id })
	if i < 0 {
		return config.Optimizer{}, false
	}
	return list[i], true
}

// OptimizerStatus is a token optimizer as the Settings page shows it. Looking
// for it runs nothing: another program can have the same name.
type OptimizerStatus struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Source is its file; empty for a built-in one.
	Source  string `json:"source"`
	Install string `json:"install"`
	// Path is where the program is on the user's PATH; empty when it is not.
	Path string `json:"path"`
	// NeedsApproval is set for a file the user has not approved as it is now;
	// Runs says what it would run, and Fingerprint is what to approve.
	NeedsApproval bool   `json:"needsApproval"`
	Runs          string `json:"runs"`
	Fingerprint   string `json:"fingerprint"`
}

// ListOptimizers lists the token optimizers and where their programs are.
func (e *Engine) ListOptimizers(ctx context.Context) []OptimizerStatus {
	env, err := e.localEnv(e.home, nil)
	var out []OptimizerStatus
	for _, o := range e.optimizerList() {
		st := OptimizerStatus{ID: o.ID, Name: o.Name, Source: o.Source, Install: o.Install, NeedsApproval: !e.optimizerApproved(o), Runs: describeOptimizer(o), Fingerprint: o.Fingerprint()}
		if err == nil {
			st.Path = locateProgram(ctx, env, o.Program)
		}
		out = append(out, st)
	}
	return out
}

// describeOptimizer says what an optimizer runs, for the user to approve.
func describeOptimizer(o config.Optimizer) string {
	out := strings.Join(append([]string{o.Program}, o.Rewrite...), " ") + " <command>, then what it answers"
	if len(o.Version) > 0 {
		out += ", and " + strings.Join(append([]string{o.Program}, o.Version...), " ")
	}
	out += ", for " + strings.Join(slices.Sorted(maps.Keys(o.Rewrites)), ", ")
	names := slices.Sorted(maps.Keys(o.Env))
	names = append(names, slices.Sorted(maps.Keys(o.DataEnv))...)
	if len(names) > 0 {
		out += ", with " + strings.Join(names, ", ") + " set"
	}
	return out
}

// optimizerApproved reports whether an optimizer may run: every built-in one,
// and a file the user approved as it is now. A file arrives without asking (from
// a restored backup, or from anything that can write to the data folder) and
// would run on every command of every session that has it as its default.
func (e *Engine) optimizerApproved(o config.Optimizer) bool {
	if o.Source == "" {
		return true
	}
	mark, err := e.approvalMark(o.Fingerprint())
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(mark), []byte(e.Config().OptimizerApprovals[o.ID]))
}

// ApproveOptimizer approves an optimizer file as the user saw it, which must
// still be what the file says.
func (e *Engine) ApproveOptimizer(ctx context.Context, id, fingerprint string) (config.Config, error) {
	o, ok := e.findOptimizer(id)
	if !ok {
		return config.Config{}, fmt.Errorf("no token optimizer %q", id)
	}
	if o.Fingerprint() != fingerprint {
		return config.Config{}, fmt.Errorf("the file of %s changed; look at it again", o.Name)
	}
	mark, err := e.approvalMark(fingerprint)
	if err != nil {
		return config.Config{}, err
	}
	c := e.Config()
	approvals := map[string]string{}
	maps.Copy(approvals, c.OptimizerApprovals)
	approvals[id] = mark
	c.OptimizerApprovals = approvals
	return e.SaveSettings(c)
}

func notApprovedReason(o config.Optimizer) string {
	return fmt.Sprintf("%s comes from %s and has not been approved as it is now. Approve it in Settings, under Token optimizers.", o.Name, o.Source)
}

// OptimizerCheck is what running a token optimizer's program found.
type OptimizerCheck struct {
	OK      bool   `json:"ok"`
	Path    string `json:"path"`
	Version string `json:"version"`
	// Reason says why it cannot be used, and what to do about it.
	Reason string `json:"reason"`
}

// CheckOptimizer runs a token optimizer's program, on the user's asking.
func (e *Engine) CheckOptimizer(ctx context.Context, id string) (OptimizerCheck, error) {
	o, ok := e.findOptimizer(id)
	if !ok {
		return OptimizerCheck{}, fmt.Errorf("no token optimizer %q", id)
	}
	env, err := e.localEnv(e.home, nil)
	if err != nil {
		return OptimizerCheck{}, err
	}
	p := e.probeOptimizer(ctx, env, o, false)
	return OptimizerCheck{OK: p.reason == "", Path: p.path, Version: p.version, Reason: p.reason}, nil
}

// SetDefaultOptimizer sets the optimizer of new sessions; "" or off is none.
// The one chosen has to work, as when choosing one for a session.
func (e *Engine) SetDefaultOptimizer(ctx context.Context, id string) (config.Config, error) {
	if id == config.OptimizerOff {
		id = ""
	}
	if id != "" {
		check, err := e.CheckOptimizer(ctx, id)
		if err != nil {
			return config.Config{}, err
		}
		if !check.OK {
			return config.Config{}, errors.New(check.Reason)
		}
	}
	c := e.Config()
	c.Defaults.Optimizer = id
	return e.SaveSettings(c)
}

// chooseOptimizer decides a new session's optimizer: the one carried over, the
// one asked for, the profile's, then the global default. Only the one asked for
// is tried here. A default that does not work is the first command's to report,
// and a session still starts.
func (e *Engine) chooseOptimizer(ctx context.Context, req CreateSessionRequest, prof config.Profile, harnessID string, envCfg config.Environment) (string, error) {
	id := cmp.Or(req.optimizer, req.Optimizer, prof.Optimizer, e.Config().Defaults.Optimizer)
	if id == config.OptimizerOff || id == "" {
		return "", nil
	}
	asked := req.optimizer == "" && req.Optimizer != ""
	if harnessID != builtin.ID {
		if asked {
			return "", errors.New("only built-in sessions can use a token optimizer; an external agent runs its commands itself")
		}
		return "", nil
	}
	if !asked {
		return id, nil
	}
	o, ok := e.findOptimizer(id)
	if !ok {
		return "", fmt.Errorf("no token optimizer %q", id)
	}
	if !e.optimizerApproved(o) {
		return "", errors.New(notApprovedReason(o))
	}
	if envCfg.Kind == string(environment.KindDocker) {
		return id, nil
	}
	check, err := e.CheckOptimizer(ctx, id)
	if err != nil {
		return "", err
	}
	if !check.OK {
		return "", errors.New(check.Reason)
	}
	return id, nil
}

// OptimizerInfo is a token optimizer as a session can use it.
type OptimizerInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
	// Reason says why it is not available, and Install what to type about it,
	// when typing can help.
	Reason  string `json:"reason"`
	Install string `json:"install"`
}

// Optimizers lists the token optimizers a session can use. It only looks for
// their programs: another program can have the same name, so nothing runs until
// the user chooses one.
func (e *Engine) Optimizers(ctx context.Context, sid string) ([]OptimizerInfo, error) {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	env, err := e.localEnv(e.home, nil)
	if err != nil {
		return nil, err
	}
	var out []OptimizerInfo
	for _, o := range e.optimizerList() {
		info := OptimizerInfo{ID: o.ID, Name: o.Name}
		switch {
		case ses.HarnessID != builtin.ID:
			info.Reason = externalAgentReason(o)
		case !e.optimizerApproved(o):
			info.Reason = notApprovedReason(o)
		case e.inContainer(ses):
			info.Available = true // only a command in it can tell
		case locateProgram(ctx, env, o.Program) == "":
			info.Reason, info.Install = notInstalledReason(o), o.Install
		default:
			info.Available = true
		}
		out = append(out, info)
	}
	return out, nil
}

// SetOptimizer switches a session's token optimizer; "" or off turns it off.
// The next command uses it, in a running turn too.
func (e *Engine) SetOptimizer(ctx context.Context, sid, id string) error {
	if id == config.OptimizerOff {
		id = ""
	}
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return err
	}
	if ses.Optimizer == id {
		return nil
	}
	name := ""
	if id != "" {
		o, ok := e.findOptimizer(id)
		if !ok {
			return fmt.Errorf("no token optimizer %q", id)
		}
		if why := e.checkForSession(ctx, ses, o); why != "" {
			return errors.New(why)
		}
		name = o.Name
	}
	e.emit(sid, event.OptimizerChanged, event.OptimizerChangedData{Optimizer: id, Name: name})
	return nil
}

// TypeOptimizerInstall opens a terminal in the session with the optimizer's
// install command typed, for the user to read and run.
func (e *Engine) TypeOptimizerInstall(ctx context.Context, sid, id string) (TerminalInfo, error) {
	o, ok := e.findOptimizer(id)
	if !ok {
		return TerminalInfo{}, fmt.Errorf("no token optimizer %q", id)
	}
	if !e.optimizerApproved(o) {
		return TerminalInfo{}, errors.New(notApprovedReason(o))
	}
	if o.Install == "" {
		return TerminalInfo{}, fmt.Errorf("%s does not say how to install it", o.Name)
	}
	info, err := e.OpenTerminal(ctx, sid, 80, 24)
	if err != nil {
		return TerminalInfo{}, err
	}
	// No newline: Validate holds it to one line, and the user presses return.
	if err := e.WriteTerminal(info.ID, []byte(o.Install)); err != nil {
		return TerminalInfo{}, err
	}
	return info, nil
}

func externalAgentReason(o config.Optimizer) string {
	return fmt.Sprintf("%s is not available here: this agent runs its commands itself, so Freeloop cannot rewrite them.", o.Name)
}

func notInstalledReason(o config.Optimizer) string {
	if o.Install == "" {
		return fmt.Sprintf("%s is not installed where this session's commands run.", o.Name)
	}
	return fmt.Sprintf("%s is not installed where this session's commands run. Install it with: %s", o.Name, o.Install)
}

// inContainer reports a session whose commands run in a container, which only
// one of its commands can look into.
func (e *Engine) inContainer(ses session.Session) bool {
	cfg, err := e.environment(ses.EnvironmentID)
	return err == nil && cfg.Kind == string(environment.KindDocker)
}

// checkForSession tries the chosen optimizer where the session's commands run:
// in its running environment, else on the user's PATH. It returns why it cannot
// be used. A container that is not running cannot be asked; its first command
// finds out.
func (e *Engine) checkForSession(ctx context.Context, ses session.Session, o config.Optimizer) string {
	if ses.HarnessID != builtin.ID {
		return externalAgentReason(o)
	}
	if !e.optimizerApproved(o) {
		return notApprovedReason(o)
	}
	e.mu.Lock()
	r := e.runtimes[ses.ID]
	e.mu.Unlock()
	if r != nil && r.optimizer != nil {
		return r.optimizer.probe(ctx, o, true).reason
	}
	if e.inContainer(ses) {
		return ""
	}
	env, err := e.localEnv(e.home, nil)
	if err != nil {
		return err.Error()
	}
	return e.probeOptimizer(ctx, env, o, false).reason
}

// optimizerDir is the session's own folder for what an optimizer keeps, which
// holds commands' full output. A confined command can reach only the project's
// caches, so an unconfined session's stays out of them, or a sandboxed session of
// the same project could read what ran outside, or plant a link for it to write
// through. Sandboxed sessions of a project can reach each other's, as they can
// reach each other's work. A container has a session to itself and keeps it
// inside.
func (e *Engine) optimizerDir(ses session.Session, sandboxed bool) string {
	if sandboxed {
		return filepath.Join(e.projectCache(ses.ProjectID), "cache", "optimizer", ses.ID)
	}
	return filepath.Join(environment.Canonical(e.home), "Library", "Caches", "Freeloop", "optimizer", ses.ID)
}

// optimizerProbe is what looking for an optimizer's program found.
type optimizerProbe struct {
	path, version, reason string
	at                    time.Time
}

// locateProgram finds a program the way a command would, without running it;
// "" when there is none.
func locateProgram(ctx context.Context, env environment.Environment, program string) string {
	res, err := env.Exec(ctx, environment.ExecSpec{Command: "command -v " + shellQuote(program), Cwd: neutralDir, Timeout: probeTimeout, MaxOutput: 4096, ReadOnly: true})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	// Startup files can print before the answer.
	lines := strings.Split(strings.TrimSpace(res.Output), "\n")
	path := strings.TrimSpace(lines[len(lines)-1])
	if !strings.HasPrefix(path, "/") {
		return "" // an alias or a function, which the rewrite could not run
	}
	return path
}

// probeOptimizer finds the program the way the session's commands will and
// checks that it is the one meant, by what it does with a known command. With
// inProject set, env is the session's, and a program inside its folder is refused
// unrun: the repository would choose what runs, confined or not.
func (e *Engine) probeOptimizer(ctx context.Context, env environment.Environment, o config.Optimizer, inProject bool) optimizerProbe {
	p := optimizerProbe{at: time.Now()}
	if !e.optimizerApproved(o) {
		p.reason = notApprovedReason(o)
		return p
	}
	path := locateProgram(ctx, env, o.Program)
	if path == "" {
		p.reason = notInstalledReason(o)
		return p
	}
	if inProject && env.Contains(path) {
		p.reason = fmt.Sprintf("%s was found inside the project, at %s, which the repository could change, so it is not used.", o.Program, path)
		return p
	}
	got, err := runRewrite(ctx, env, o, path, o.ProbeCommand)
	if err != nil || got != o.ProbeWant {
		p.reason = fmt.Sprintf("%s at %s does not rewrite commands as %s does, so it may be another program with the same name, or the session's environment may keep it from running.", o.Program, path, o.Name)
		return p
	}
	p.path = path
	if len(o.Version) > 0 {
		if res, err := env.Exec(ctx, environment.ExecSpec{Argv: append([]string{path}, o.Version...), Env: o.Env, Cwd: neutralDir, Timeout: probeTimeout, MaxOutput: 4096, ReadOnly: true}); err == nil && res.ExitCode == 0 {
			p.version = strings.TrimSpace(res.Output)
		}
	}
	return p
}

// runRewrite asks the program for a command's rewrite: "" when it has none, an
// error when it did not answer. Only standard output counts, since standard
// error carries its warnings.
func runRewrite(ctx context.Context, env environment.Environment, o config.Optimizer, path, command string) (string, error) {
	// Without a home it reads no agent's settings: what may run is the broker's
	// business.
	vars := map[string]string{}
	maps.Copy(vars, o.Env)
	vars["HOME"] = "/nonexistent"
	argv := append(append([]string{path}, o.Rewrite...), command)
	res, err := env.Exec(ctx, environment.ExecSpec{Argv: argv, Env: vars, Cwd: neutralDir, Timeout: rewriteTimeout, MaxOutput: 1 << 16, SeparateStderr: true, ReadOnly: true})
	if err != nil {
		return "", err
	}
	if res.TimedOut || res.Truncated {
		return "", errors.New("no answer in time")
	}
	if !slices.Contains(o.RewriteExits, res.ExitCode) {
		return "", nil
	}
	return strings.TrimSpace(res.Output), nil
}

// sessionOptimizer is harness.CommandOptimizer for one running session.
type sessionOptimizer struct {
	e   *Engine
	sid string
	env environment.Environment

	mu     sync.Mutex
	probes map[string]optimizerProbe
	warned map[string]bool
}

func (s *sessionOptimizer) probe(ctx context.Context, o config.Optimizer, fresh bool) optimizerProbe {
	// An edited file is another optimizer: what was checked no longer counts.
	key := o.Fingerprint()
	s.mu.Lock()
	p, ok := s.probes[key]
	s.mu.Unlock()
	if ok && !fresh && (p.reason == "" || time.Since(p.at) < probeRetry) {
		return p
	}
	p = s.e.probeOptimizer(ctx, s.env, o, true)
	// A stopped turn found nothing out.
	if ctx.Err() != nil {
		return p
	}
	s.mu.Lock()
	if s.probes == nil {
		s.probes = map[string]optimizerProbe{}
	}
	s.probes[key] = p
	if p.reason == "" {
		delete(s.warned, o.ID)
	}
	s.mu.Unlock()
	return p
}

// fail stops asking a program that did not answer for a while, so one that hangs
// costs its time limit once a minute and not on every command.
func (s *sessionOptimizer) fail(o config.Optimizer, why string) {
	s.mu.Lock()
	if s.probes == nil {
		s.probes = map[string]optimizerProbe{}
	}
	s.probes[o.Fingerprint()] = optimizerProbe{reason: why, at: time.Now()}
	s.mu.Unlock()
	s.warn(o, why)
}

// warn tells the user once, not at every command, that an optimizer is not
// doing its work.
func (s *sessionOptimizer) warn(o config.Optimizer, why string) {
	s.mu.Lock()
	seen := s.warned[o.ID]
	if s.warned == nil {
		s.warned = map[string]bool{}
	}
	s.warned[o.ID] = true
	s.mu.Unlock()
	if !seen {
		s.e.emit(s.sid, event.Notice, event.NoticeData{Message: fmt.Sprintf("Commands are running without %s: %s", o.Name, why)})
	}
}

// Rewrite implements harness.CommandOptimizer.
func (s *sessionOptimizer) Rewrite(ctx context.Context, command string, raw bool) harness.Rewrite {
	out := harness.Rewrite{Command: command}
	ses, err := s.e.store.GetSession(ctx, s.sid)
	if err != nil || ses.Optimizer == "" {
		return out
	}
	o, ok := s.e.findOptimizer(ses.Optimizer)
	if !ok {
		s.warn(config.Optimizer{ID: ses.Optimizer, Name: ses.Optimizer}, "there is no token optimizer with that name.")
		return out
	}
	// Checked every time: an approval ends when the file changes.
	if !s.e.optimizerApproved(o) {
		s.warn(o, notApprovedReason(o))
		return out
	}
	p := s.probe(ctx, o, false)
	if ctx.Err() != nil {
		return out
	}
	if p.reason != "" {
		s.warn(o, p.reason)
		return out
	}
	out.Env = map[string]string{}
	maps.Copy(out.Env, o.Env)
	if s.env.Kind() != environment.KindDocker {
		// It keeps commands' full output here, and would make the folder open
		// to anyone on this Mac.
		dir := s.e.optimizerDir(ses, environment.IsSandboxed(s.env))
		if err := environment.PrivateCache(dir); err != nil {
			s.warn(o, err.Error())
			return harness.Rewrite{Command: command}
		}
		for name, file := range o.DataEnv {
			out.Env[name] = filepath.Join(dir, file)
		}
	}
	if raw || !rewritable(command, o) {
		return out
	}
	rewritten, err := runRewrite(ctx, s.env, o, p.path, command)
	if err != nil {
		// A stopped turn is not the program's failing.
		if ctx.Err() == nil {
			s.fail(o, "it did not answer in time.")
		}
		return harness.Rewrite{Command: command}
	}
	if rewritten != command && validRewrite(command, rewritten, o) {
		out.Command, out.Via = rewritten, o.Name
	}
	return out
}

// rewritable reports whether the program is asked about a command at all: a plain
// line, with a command it may rewrite, without anything it should leave alone.
func rewritable(command string, o config.Optimizer) bool {
	if len(command) > maxRewriteInput {
		return false
	}
	parts, _, ok := splitCommands(command)
	if !ok {
		return false
	}
	if !slices.ContainsFunc(parts, func(p string) bool {
		word, _, _ := strings.Cut(p, " ")
		_, ok := o.Rewrites[word]
		return ok
	}) {
		return false
	}
	for _, part := range parts {
		words := strings.Fields(part)
		for _, c := range o.LeaveAloneCommands {
			if inOrder(strings.Fields(c), words) {
				return false
			}
		}
		for _, word := range words {
			word = strings.NewReplacer("'", "", `"`, "", `\`, "").Replace(word)
			for _, w := range o.LeaveAlone {
				if word == w || strings.HasPrefix(word, w+"=") {
					return false
				}
			}
		}
	}
	return true
}

// inOrder reports whether want appears in words, in order, with others between.
func inOrder(want, words []string) bool {
	for _, w := range words {
		if len(want) > 0 && w == want[0] {
			want = want[1:]
		}
	}
	return len(want) == 0
}

// newWord is what a rewrite may add for a program it renames: head -5 f becomes
// rtk read f --head-lines 5. An option or a number, and no file or expansion.
var newWord = regexp.MustCompile(`^(-[A-Za-z-]+|[0-9]+)$`)

// validRewrite reports whether rewritten is plainly the approved command with the
// program in front of the commands it may rewrite. The same commands stay in the
// same order, joined the same way, and each that differs is exactly the original
// after the program's name. Where the program has its own word for a command, as
// read for cat, its other words come from the original, or are options or numbers.
// So a rewrite cannot add a command, run another tool, or change what it works on.
func validRewrite(original, rewritten string, o config.Optimizer) bool {
	if rewritten == "" || strings.ContainsRune(rewritten, 0) {
		return false
	}
	before, opsBefore, ok := splitCommands(original)
	if !ok {
		return false
	}
	after, opsAfter, ok := splitCommands(rewritten)
	if !ok || !slices.Equal(opsBefore, opsAfter) || len(before) != len(after) {
		return false
	}
	for i := range before {
		if before[i] == after[i] {
			continue
		}
		tool, _, _ := strings.Cut(before[i], " ")
		word, ok := o.Rewrites[tool]
		if !ok {
			return false
		}
		if word == tool {
			if after[i] != o.Program+" "+before[i] {
				return false
			}
			continue
		}
		rest, ok := strings.CutPrefix(after[i], o.Program+" "+word+" ")
		if !ok {
			return false
		}
		kept := strings.Fields(before[i])[1:]
		for _, w := range strings.Fields(rest) {
			if !slices.Contains(kept, w) && !newWord.MatchString(w) {
				return false
			}
		}
	}
	return true
}

// splitCommands splits a plain command line at what separates its commands: ;,
// &&, ||, & and newlines, outside quotes. ok is false for anything else the shell
// would act on, or that is easy to read two ways, in which case the line is not
// rewritten:
//   - a pipe, a redirection or a substitution feeds a command's output onward, and
//     shortened output would change the result (git diff | git apply);
//     2>&1 is the one exception, since it only joins the output being read;
//   - parentheses also cover zsh's process substitution and glob qualifiers, which
//     run code;
//   - a comment hides quotes from the shell: rtk x #' then a line, then #' runs
//     that line;
//   - $'...' takes \' as a quote inside it, and ” is one under zsh's RC_QUOTES,
//     so where such a string ends depends on more than can be told from here.
func splitCommands(cmd string) (parts, ops []string, ok bool) {
	var part strings.Builder
	cut := func(op string) {
		parts = append(parts, strings.TrimSpace(part.String()))
		part.Reset()
		ops = append(ops, op)
	}
	wordStart := func(i int) bool {
		return i == 0 || strings.IndexByte(" \t\n;&", cmd[i-1]) >= 0
	}
	var quote byte
	for i := 0; i < len(cmd); i++ {
		c, rest := cmd[i], cmd[i:]
		switch {
		case quote == '\'':
			part.WriteByte(c)
			if c == '\'' {
				if strings.HasPrefix(rest, "''") {
					return nil, nil, false
				}
				quote = 0
			}
		case c == '\\':
			part.WriteByte(c)
			if i+1 < len(cmd) {
				i++
				part.WriteByte(cmd[i])
			}
		case quote == '"':
			if c == '`' || strings.HasPrefix(rest, "$(") {
				return nil, nil, false
			}
			if c == '"' {
				quote = 0
			}
			part.WriteByte(c)
		case strings.HasPrefix(rest, "$'"), strings.HasPrefix(rest, "$("), c == '#' && wordStart(i):
			return nil, nil, false
		case c == '\'' || c == '"':
			quote = c
			part.WriteByte(c)
		case strings.HasPrefix(rest, "2>&1") && wordStart(i) && (len(rest) == 4 || strings.IndexByte(" \t\n;&", rest[4]) >= 0):
			part.WriteString("2>&1")
			i += 3
		case strings.HasPrefix(rest, "&&"), strings.HasPrefix(rest, "||"):
			cut(rest[:2])
			i++
		case c == ';' || c == '&' || c == '\n':
			cut(string(c))
		case strings.IndexByte("|<>()`", c) >= 0:
			return nil, nil, false
		default:
			part.WriteByte(c)
		}
	}
	if quote != 0 {
		return nil, nil, false
	}
	return append(parts, strings.TrimSpace(part.String())), ops, true
}
