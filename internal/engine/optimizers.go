package engine

import (
	"context"
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
// does every failure, so an optimizer can make output shorter and nothing else.

const (
	rewriteTimeout = 2 * time.Second
	probeTimeout   = 10 * time.Second
	// probeRetry is how soon a missing program is looked for again while
	// commands run; choosing one always looks.
	probeRetry = time.Minute
)

// OptimizerInfo is a token optimizer as a session can use it.
type OptimizerInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
	// Reason says why it is not available, and what to do about it.
	Reason string `json:"reason"`
}

// Optimizers lists the token optimizers a session can use. It only looks for
// their programs: another program can have the same name, so nothing runs until
// the user chooses one.
func (e *Engine) Optimizers(ctx context.Context, sid string) ([]OptimizerInfo, error) {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	var out []OptimizerInfo
	for _, o := range e.optimizers {
		info := OptimizerInfo{ID: o.ID, Name: o.Name}
		switch {
		case ses.HarnessID != builtin.ID:
			info.Reason = externalAgentReason(o)
		case e.inContainer(ses):
			info.Available = true // only a command in it can tell
		case e.missingCommand(o.Program) != "":
			info.Reason = notInstalledReason(o)
		default:
			info.Available = true
		}
		out = append(out, info)
	}
	return out, nil
}

// SetOptimizer switches a session's token optimizer; "" turns it off. The next
// command uses it, in a running turn too.
func (e *Engine) SetOptimizer(ctx context.Context, sid, id string) error {
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
		if why := e.checkOptimizer(ctx, ses, o); why != "" {
			return errors.New(why)
		}
		name = o.Name
	}
	e.emit(sid, event.OptimizerChanged, event.OptimizerChangedData{Optimizer: id, Name: name})
	return nil
}

func (e *Engine) findOptimizer(id string) (config.Optimizer, bool) {
	i := slices.IndexFunc(e.optimizers, func(o config.Optimizer) bool { return o.ID == id })
	if i < 0 {
		return config.Optimizer{}, false
	}
	return e.optimizers[i], true
}

func externalAgentReason(o config.Optimizer) string {
	return fmt.Sprintf("%s is not available here: this agent runs its commands itself, so Freeloop cannot rewrite them.", o.Name)
}

func notInstalledReason(o config.Optimizer) string {
	return fmt.Sprintf("%s is not installed where this session's commands run. Install it with: %s", o.Name, o.Install)
}

// inContainer reports a session whose commands run in a container that is not
// up to be asked.
func (e *Engine) inContainer(ses session.Session) bool {
	cfg, err := e.environment(ses.EnvironmentID)
	return err == nil && cfg.Kind == string(environment.KindDocker)
}

// checkOptimizer tries the chosen optimizer where the session's commands run:
// in its running environment, else on the user's PATH. It returns why it cannot
// be used. A container that is not running cannot be asked; its first command
// finds out.
func (e *Engine) checkOptimizer(ctx context.Context, ses session.Session, o config.Optimizer) string {
	if ses.HarnessID != builtin.ID {
		return externalAgentReason(o)
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
	return probeOptimizer(ctx, env, o).reason
}

// optimizerDir is the session's own folder for what an optimizer keeps, in the
// caches a sandboxed command can write. A container has a session to itself and
// keeps it inside.
func (e *Engine) optimizerDir(ses session.Session) string {
	return filepath.Join(e.projectCache(ses.ProjectID), "cache", "optimizer", ses.ID)
}

// optimizerProbe is what looking for an optimizer's program found.
type optimizerProbe struct {
	path, reason string
	at           time.Time
}

// probeOptimizer finds the program the way the session's commands will and
// checks that it is the one meant, by what it does with a known command.
func probeOptimizer(ctx context.Context, env environment.Environment, o config.Optimizer) optimizerProbe {
	p := optimizerProbe{at: time.Now()}
	missing := notInstalledReason(o)
	res, err := env.Exec(ctx, environment.ExecSpec{Command: "command -v " + shellQuote(o.Program), Timeout: probeTimeout, MaxOutput: 4096, ReadOnly: true})
	if err != nil || res.ExitCode != 0 {
		p.reason = missing
		return p
	}
	// Startup files can print before the answer.
	lines := strings.Split(strings.TrimSpace(res.Output), "\n")
	path := strings.TrimSpace(lines[len(lines)-1])
	if !strings.HasPrefix(path, "/") {
		p.reason = missing
		return p
	}
	got, err := runRewrite(ctx, env, o, path, o.ProbeCommand)
	if err != nil || got != o.ProbeWant {
		p.reason = fmt.Sprintf("%s at %s does not rewrite commands as %s does, so it may be another program with the same name, or the session's environment may keep it from running.", o.Program, path, o.Name)
		return p
	}
	p.path = path
	return p
}

// runRewrite asks the program for a command's rewrite: "" when it has none, an
// error when it did not answer. Only standard output counts, since standard
// error carries its warnings.
func runRewrite(ctx context.Context, env environment.Environment, o config.Optimizer, path, command string) (string, error) {
	vars := map[string]string{}
	maps.Copy(vars, o.Env)
	maps.Copy(vars, o.RewriteEnv)
	argv := append(append([]string{path}, o.Rewrite...), command)
	res, err := env.Exec(ctx, environment.ExecSpec{Argv: argv, Env: vars, Timeout: rewriteTimeout, MaxOutput: 1 << 16, SeparateStderr: true, ReadOnly: true})
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
	s.mu.Lock()
	p, ok := s.probes[o.ID]
	s.mu.Unlock()
	if ok && !fresh && (p.reason == "" || time.Since(p.at) < probeRetry) {
		return p
	}
	p = probeOptimizer(ctx, s.env, o)
	s.mu.Lock()
	if s.probes == nil {
		s.probes = map[string]optimizerProbe{}
	}
	s.probes[o.ID] = p
	if p.reason == "" {
		delete(s.warned, o.ID)
	}
	s.mu.Unlock()
	return p
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
func (s *sessionOptimizer) Rewrite(ctx context.Context, command string) harness.Rewrite {
	out := harness.Rewrite{Command: command}
	ses, err := s.e.store.GetSession(ctx, s.sid)
	if err != nil || ses.Optimizer == "" {
		return out
	}
	o, ok := s.e.findOptimizer(ses.Optimizer)
	if !ok {
		return out
	}
	p := s.probe(ctx, o, false)
	if p.reason != "" {
		s.warn(o, p.reason)
		return out
	}
	out.Env = map[string]string{}
	maps.Copy(out.Env, o.Env)
	if s.env.Kind() != environment.KindDocker {
		// It keeps commands' full output here, and would make the folder open
		// to anyone on this Mac.
		dir := s.e.optimizerDir(ses)
		if err := environment.PrivateCache(dir); err != nil {
			s.warn(o, err.Error())
			return harness.Rewrite{Command: command}
		}
		for name, file := range o.DataEnv {
			out.Env[name] = filepath.Join(dir, file)
		}
	}
	rewritten, err := runRewrite(ctx, s.env, o, p.path, command)
	if err != nil {
		s.warn(o, "it did not answer in time.")
		return out
	}
	if rewritten != command && validRewrite(command, rewritten, o.Program) {
		out.Command, out.Via = rewritten, o.Name
	}
	return out
}

// leadingAssignments matches the VAR=value words before a command.
var leadingAssignments = regexp.MustCompile(`^(?:[A-Za-z_][A-Za-z0-9_]*=\S*\s+)*`)

// validRewrite reports whether rewritten is plainly the approved command with
// the optimizer's program in front of some of its parts: the same operators,
// redirections and substitutions in the same order, and every part that differs
// starting with the program. Its arguments may change (cat f becomes rtk read
// f), so it trusts the program with those, but a rewrite cannot chain, pipe or
// redirect anything the user did not approve, or hide a command inside another.
func validRewrite(original, rewritten, program string) bool {
	if rewritten == "" || strings.ContainsRune(rewritten, 0) {
		return false
	}
	before, opsBefore, ok := shellShape(original)
	if !ok {
		return false
	}
	after, opsAfter, ok := shellShape(rewritten)
	if !ok || !slices.Equal(opsBefore, opsAfter) || len(before) != len(after) {
		return false
	}
	for i := range before {
		if before[i] == after[i] {
			continue
		}
		prefix := leadingAssignments.FindString(after[i])
		if prefix != leadingAssignments.FindString(before[i]) {
			return false
		}
		rest := strings.TrimPrefix(after[i], prefix)
		if word, _, _ := strings.Cut(rest, " "); word != program {
			return false
		}
	}
	return true
}

// shellShape splits a command line at what the shell acts on outside quotes,
// returning the parts between and those operators in order, with substitutions
// inside double quotes among them. ok is false for an unbalanced quote.
func shellShape(cmd string) (parts, ops []string, ok bool) {
	var part strings.Builder
	cut := func(op string) {
		parts = append(parts, strings.TrimSpace(part.String()))
		part.Reset()
		ops = append(ops, op)
	}
	var quote byte
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			}
			part.WriteByte(c)
		case c == '\\' && i+1 < len(cmd):
			part.WriteByte(c)
			i++
			part.WriteByte(cmd[i])
		case quote == '"':
			if c == '"' {
				quote = 0
			}
			if c == '`' || c == '$' && i+1 < len(cmd) && cmd[i+1] == '(' {
				ops = append(ops, string(c))
			}
			part.WriteByte(c)
		case c == '\'' || c == '"':
			quote = c
			part.WriteByte(c)
		case c == '$' && i+1 < len(cmd) && cmd[i+1] == '(':
			cut("$(")
			i++
		case strings.IndexByte(";|&<>()`\n", c) >= 0:
			cut(string(c))
		default:
			part.WriteByte(c)
		}
	}
	if quote != 0 {
		return nil, nil, false
	}
	return append(parts, strings.TrimSpace(part.String())), ops, true
}
