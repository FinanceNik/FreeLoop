package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// optimizerNote is what the model is told when an optimizer comes on (HRN-019),
// once, on its next message. A rewrite is invisible to it, so without this an agent
// asked about the program looks for it, finds nothing on the PATH, and reports it
// as missing, or starts typing the program's name itself.
func (e *Engine) optimizerNote(ctx context.Context, r *runtime) string {
	if r.optimizer == nil {
		return ""
	}
	ses, err := e.store.GetSession(ctx, r.sid)
	if err != nil {
		return ""
	}
	if ses.Optimizer == r.toldOptimizer {
		return ""
	}
	r.toldOptimizer = ses.Optimizer
	o, ok := e.findOptimizer(ses.Optimizer)
	if !ok {
		return ""
	}
	return fmt.Sprintf("Note from Freeloop, not from the user: %s is on for this session. Freeloop runs your simple shell commands through it to shorten their output, and may show fewer lines than the tool would. It is not on your PATH: do not look for it, install it or type its name. Write commands as usual. One command without a pipe or a multi-line script is what it can shorten; say so if the user asks whether it is working.", o.Name)
}

// OptimizerSavings is what a session's optimizer reports having saved.
type OptimizerSavings struct {
	Commands int   `json:"commands"`
	Input    int64 `json:"input"`
	Output   int64 `json:"output"`
	Saved    int64 `json:"saved"`
}

// CheckOptimizerSavings asks a running session's optimizer what it has saved, or
// returns nil when it cannot say: no such report, nothing running, nothing yet.
func (e *Engine) CheckOptimizerSavings(ctx context.Context, sid string) (*OptimizerSavings, error) {
	e.mu.Lock()
	r := e.runtimes[sid]
	e.mu.Unlock()
	if r == nil || r.optimizer == nil {
		return nil, nil
	}
	return r.optimizer.savings(ctx)
}

func (s *sessionOptimizer) savings(ctx context.Context) (*OptimizerSavings, error) {
	ses, err := s.e.store.GetSession(ctx, s.sid)
	if err != nil {
		return nil, err
	}
	o, ok := s.e.findOptimizer(ses.Optimizer)
	if !ok || len(o.Gain) == 0 || s.env.Kind() == environment.KindDocker {
		return nil, nil
	}
	p := s.probe(ctx, o, false)
	if p.path == "" {
		return nil, nil
	}
	// Its own data, and no agent's settings, as for a rewrite.
	vars := map[string]string{"HOME": "/nonexistent"}
	maps.Copy(vars, o.Env)
	dir := s.e.optimizerDir(ses, environment.IsSandboxed(s.env))
	for name, file := range o.DataEnv {
		vars[name] = filepath.Join(dir, file)
	}
	res, err := s.env.Exec(ctx, environment.ExecSpec{Argv: append([]string{p.path}, o.Gain...), Env: vars, Cwd: neutralDirFor(s.env), Timeout: rewriteTimeout, MaxOutput: 1 << 16})
	if err != nil {
		return nil, err
	}
	if res.TimedOut || res.Truncated || res.ExitCode != 0 {
		return nil, nil
	}
	saved, err := parseGain([]byte(res.Output))
	if err != nil {
		return nil, nil // No data yet reads as text, not as a failure worth showing.
	}
	return saved, nil
}

// parseGain reads RTK's JSON report, past any warning printed before it.
func parseGain(out []byte) (*OptimizerSavings, error) {
	i := bytes.IndexByte(out, '{')
	if i < 0 {
		return nil, errors.New("no report")
	}
	var r struct {
		Summary *struct {
			Commands int   `json:"total_commands"`
			Input    int64 `json:"total_input"`
			Output   int64 `json:"total_output"`
			Saved    int64 `json:"total_saved"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(out[i:], &r); err != nil {
		return nil, err
	}
	if r.Summary == nil {
		return nil, errors.New("no summary")
	}
	return &OptimizerSavings{Commands: r.Summary.Commands, Input: r.Summary.Input, Output: r.Summary.Output, Saved: r.Summary.Saved}, nil
}
