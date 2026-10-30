// Package acp is the generic ACP client harness (HRN-005): it runs an
// external agent that speaks the Agent Client Protocol over stdio, maps its
// session updates one to one onto Freeloop's events, routes every permission
// request through the broker, and fulfils the agent's file and terminal
// requests through the session's environment, where the path policy holds.
// The agent is spawned with the environment's scrubbed variables only; a
// subscription agent such as Claude Code signs in by itself (HRN-008).
package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	sdk "github.com/coder/acp-go-sdk"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/permission"
)

// StartTimeout bounds starting the agent: spawning, initialize and
// session/new. npx may download the agent on first use.
const StartTimeout = 2 * time.Minute

// Harness runs the agent a preset names.
type Harness struct {
	Preset config.HarnessPreset
}

func (h Harness) Info() harness.Info {
	return harness.Info{ID: h.Preset.ID, Name: h.Preset.Name, Kind: "acp"}
}

type session struct {
	p      harness.Params
	preset config.HarnessPreset
	env    environment.Environment
	proc   environment.Process
	conn   *sdk.ClientSideConnection
	id     sdk.SessionId
	caps   sdk.AgentCapabilities

	mu   sync.Mutex
	mode permission.Mode
	// loading drops the history an agent replays on session/load: the
	// transcript has it already.
	loading bool
	// approved holds what the user or a rule just allowed for a permission
	// request, so the agent's own fs/terminal request for the same thing
	// does not ask twice.
	approved approval
	terms    map[string]*terminal
	next     int
}

type approval struct {
	paths   []string
	command string
	until   time.Time
}

// NewSession starts the agent, initializes it and opens a session in the
// project root with the session's MCP servers.
func (h Harness) NewSession(ctx context.Context, p harness.Params) (harness.Session, error) {
	sp, ok := p.Env.(environment.Spawner)
	switch {
	case !ok:
		return nil, errors.New("acp: this environment cannot start processes")
	case p.Broker == nil || p.Emit == nil:
		return nil, errors.New("acp: Broker and Emit are required")
	}
	mode := p.Mode
	if !mode.Valid() {
		mode = permission.ModeAsk
	}
	s := &session{p: p, preset: h.Preset, env: p.Env, mode: mode, terms: map[string]*terminal{}}
	proc, err := sp.Spawn(context.Background(), environment.SpawnSpec{
		Argv: append([]string{h.Preset.Command}, h.Preset.Args...), Env: h.Preset.Env, LoginShell: true,
	})
	if err != nil {
		return nil, err
	}
	s.proc = proc
	s.conn = sdk.NewClientSideConnection(s, proc.Stdin(), proc.Stdout())

	ctx, cancel := context.WithTimeout(ctx, StartTimeout)
	defer cancel()
	fail := func(step string, err error) (harness.Session, error) {
		proc.Kill()
		msg := fmt.Sprintf("%s: %s: %v", h.Preset.Name, step, err)
		if tail := strings.TrimSpace(proc.Stderr()); tail != "" {
			msg += " (" + lastLine(tail) + ")"
		}
		return nil, errors.New(msg)
	}
	init, err := s.conn.Initialize(ctx, sdk.InitializeRequest{
		ProtocolVersion:    sdk.ProtocolVersionNumber,
		ClientCapabilities: sdk.ClientCapabilities{Fs: sdk.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true}, Terminal: true},
		ClientInfo:         &sdk.Implementation{Name: "freeloop", Version: "0.1"},
	})
	if err != nil {
		return fail("initialize", err)
	}
	s.caps = init.AgentCapabilities
	if p.ExternalID != "" {
		if s.load(ctx, p.ExternalID) {
			s.applyMode(ctx)
			return s, nil
		}
	}
	ns, err := s.conn.NewSession(ctx, sdk.NewSessionRequest{Cwd: s.env.Root(), McpServers: s.mcpServers(), Meta: s.sessionMeta()})
	if err != nil {
		if len(init.AuthMethods) > 0 {
			err = fmt.Errorf("%w; sign in with the agent's own command first (Freeloop does not handle its login)", err)
		}
		return fail("new session", err)
	}
	s.id = ns.SessionId
	s.applyMode(ctx)
	return s, nil
}

// load continues the agent's session id (SES-003) and reports whether it
// could; the agent then knows the conversation so far. When it cannot, the
// session starts anew and the transcript says so.
func (s *session) load(ctx context.Context, id string) bool {
	why := "it cannot load earlier sessions"
	if s.caps.LoadSession {
		s.mu.Lock()
		s.loading = true
		s.mu.Unlock()
		_, err := s.conn.LoadSession(ctx, sdk.LoadSessionRequest{SessionId: sdk.SessionId(id), Cwd: s.env.Root(), McpServers: s.mcpServers(), Meta: s.sessionMeta()})
		s.mu.Lock()
		s.loading = false
		s.mu.Unlock()
		if err == nil {
			s.id = sdk.SessionId(id)
			return true
		}
		why = err.Error()
	}
	s.p.Emit(event.Notice, event.NoticeData{Message: fmt.Sprintf("%s started a new session: %s. It does not remember the conversation above.", s.preset.Name, why)})
	return false
}

// ExternalID is the agent's session ID.
func (s *session) ExternalID() string { return string(s.id) }

// mcpServers lists the session's servers in ACP's form: stdio always, HTTP
// when the agent supports it (MCP-004).
func (s *session) mcpServers() []sdk.McpServer {
	out := []sdk.McpServer{}
	for _, m := range s.p.MCPServers {
		switch m.Transport {
		case "http":
			if !s.caps.McpCapabilities.Http {
				continue
			}
			var hs []sdk.HttpHeader
			for _, k := range slices.Sorted(maps.Keys(m.Headers)) {
				hs = append(hs, sdk.HttpHeader{Name: k, Value: m.Headers[k]})
			}
			if hs == nil {
				hs = []sdk.HttpHeader{}
			}
			out = append(out, sdk.McpServer{Http: &sdk.McpServerHttpInline{Name: m.Name, Url: m.URL, Headers: hs, Type: "http"}})
		default:
			env := []sdk.EnvVariable{}
			for _, k := range slices.Sorted(maps.Keys(m.Env)) {
				env = append(env, sdk.EnvVariable{Name: k, Value: m.Env[k]})
			}
			args := m.Args
			if args == nil {
				args = []string{}
			}
			out = append(out, sdk.McpServer{Stdio: &sdk.McpServerStdio{Name: m.Name, Command: m.Command, Args: args, Env: env}})
		}
	}
	return out
}

// applyMode tells the agent the session's mode when the preset maps it.
func (s *session) applyMode(ctx context.Context) {
	s.mu.Lock()
	m := s.mode
	s.mu.Unlock()
	id, ok := s.preset.Modes[string(m)]
	if !ok {
		return
	}
	if _, err := s.conn.SetSessionMode(ctx, sdk.SetSessionModeRequest{SessionId: s.id, ModeId: sdk.SessionModeId(id)}); err != nil {
		log.Printf("acp: set mode %s: %v", id, err)
	}
}

func (s *session) Mode() permission.Mode {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode
}

func (s *session) SetMode(m permission.Mode) {
	s.mu.Lock()
	changed := s.mode != m
	s.mode = m
	s.mu.Unlock()
	if changed {
		s.p.Emit(event.CurrentModeUpdate, event.CurrentModeData{CurrentModeID: string(m)})
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.applyMode(ctx)
	}
}

func (s *session) Close() error {
	s.mu.Lock()
	for _, t := range s.terms {
		t.cancel()
	}
	s.mu.Unlock()
	s.proc.Kill()
	return nil
}

// Prompt sends one user turn. Cancelling ctx sends session/cancel and waits
// briefly for the agent to end the turn, as the protocol asks.
func (s *session) Prompt(ctx context.Context, p harness.Prompt) (harness.StopReason, error) {
	s.p.Emit(event.TurnStarted, struct{}{})
	s.p.Emit(event.SessionStatus, event.SessionStatusData{Status: "running"})
	s.p.Emit(event.UserMessageChunk, event.MessageChunk{Content: event.ContentBlock{Type: "text", Text: p.Text}})
	blocks := []sdk.ContentBlock{sdk.TextBlock(p.Text)}
	for _, r := range p.Resources {
		s.p.Emit(event.UserMessageChunk, event.MessageChunk{Content: event.ContentBlock{Type: "resource_link", URI: r.URI, Name: r.Path}})
		if s.caps.PromptCapabilities.EmbeddedContext {
			blocks = append(blocks, sdk.ResourceBlock(sdk.EmbeddedResourceResource{TextResourceContents: &sdk.TextResourceContents{Uri: r.URI, Text: r.Text}}))
		} else {
			blocks = append(blocks, sdk.TextBlock(harness.ResourceText(r)))
		}
	}
	type answer struct {
		res sdk.PromptResponse
		err error
	}
	done := make(chan answer, 1)
	go func() {
		res, err := s.conn.Prompt(context.Background(), sdk.PromptRequest{SessionId: s.id, Prompt: blocks})
		done <- answer{res, err}
	}()
	var a answer
	select {
	case a = <-done:
	case <-ctx.Done():
		s.conn.Cancel(context.Background(), sdk.CancelNotification{SessionId: s.id})
		select {
		case a = <-done:
		case <-time.After(10 * time.Second):
			a.res.StopReason = sdk.StopReasonCancelled
		}
	case <-s.conn.Done():
		a.err = errors.New("the agent exited")
		if tail := strings.TrimSpace(s.proc.Stderr()); tail != "" {
			a.err = fmt.Errorf("the agent exited: %s", lastLine(tail))
		}
	}
	stop := harness.StopReason(a.res.StopReason)
	if a.err != nil {
		stop = harness.StopError
		s.p.Emit(event.Error, event.ErrorData{Message: a.err.Error()})
	} else if ctx.Err() != nil {
		stop = harness.StopCancelled
	}
	if u := a.res.Usage; u != nil {
		d := event.UsageData{InputTokens: int64(u.InputTokens), OutputTokens: int64(u.OutputTokens)}
		if u.CachedReadTokens != nil {
			d.CacheReadTokens = int64(*u.CachedReadTokens)
		}
		if u.CachedWriteTokens != nil {
			d.CacheWriteTokens = int64(*u.CachedWriteTokens)
		}
		s.p.Emit(event.UsageUpdate, d)
	}
	s.p.Emit(event.TurnEnded, event.TurnEndedData{StopReason: string(stop)})
	status := "idle"
	if a.err != nil {
		status = "error"
	}
	s.p.Emit(event.SessionStatus, event.SessionStatusData{Status: status})
	return stop, a.err
}

// SessionUpdate records an update as the event of the same name: Freeloop's
// schema is a superset of ACP's (OQ-012). Context usage, which has another
// shape than the per-turn usage_update the app records, and variants newer
// than the schema are left out.
func (s *session) SessionUpdate(_ context.Context, n sdk.SessionNotification) error {
	s.mu.Lock()
	loading := s.loading
	s.mu.Unlock()
	if loading {
		return nil
	}
	b, err := json.Marshal(n.Update)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	typ, _ := m["sessionUpdate"].(string)
	delete(m, "sessionUpdate")
	delete(m, "_meta")
	t := event.Type(typ)
	switch {
	case t == event.UsageUpdate:
		return nil
	case t == event.CurrentModeUpdate:
		// The agent's mode, back in the app's terms when the preset maps it.
		id, _ := m["currentModeId"].(string)
		for app, agent := range s.preset.Modes {
			if agent == id {
				s.mu.Lock()
				s.mode = permission.Mode(app)
				s.mu.Unlock()
				s.p.Emit(event.CurrentModeUpdate, event.CurrentModeData{CurrentModeID: app})
			}
		}
		return nil
	case t == event.ToolCall:
		kind, _ := m["kind"].(string)
		m["tool"] = toolName(kind)
	case !t.Valid():
		return nil
	}
	s.p.Emit(t, m)
	return nil
}

// toolName maps an ACP tool kind onto the built-in harness's tool names, so
// rules written for those (shell(git *), edit_file paths) hold for external
// agents too.
func toolName(kind string) string {
	switch permission.Kind(kind) {
	case permission.KindExecute:
		return "shell"
	case permission.KindEdit, permission.KindDelete, permission.KindMove:
		return "edit_file"
	case permission.KindRead:
		return "read_file"
	case permission.KindSearch:
		return "grep"
	case "":
		return "acp:other"
	}
	return "acp:" + kind
}

// RequestPermission asks the broker (SEC-003) and answers with the agent's
// option of the decided kind.
func (s *session) RequestPermission(ctx context.Context, r sdk.RequestPermissionRequest) (sdk.RequestPermissionResponse, error) {
	kind := permission.KindOther
	if r.ToolCall.Kind != nil {
		kind = permission.Kind(*r.ToolCall.Kind)
	}
	title := string(r.ToolCall.ToolCallId)
	if r.ToolCall.Title != nil {
		title = *r.ToolCall.Title
	}
	var paths []string
	for _, l := range r.ToolCall.Locations {
		if l.Path != "" {
			paths = append(paths, s.env.Resolve(l.Path))
		}
	}
	command := ""
	if in, ok := r.ToolCall.RawInput.(map[string]any); ok {
		if c, ok := in["command"].(string); ok && kind == permission.KindExecute {
			command = c
		}
	}
	var options []permission.Option
	for _, o := range r.Options {
		options = append(options, permission.Option{ID: string(o.OptionId), Name: o.Name, Kind: string(o.Kind)})
	}
	input, _ := json.Marshal(r.ToolCall.RawInput)
	d := s.p.Broker.Request(ctx, permission.Context{Mode: s.Mode(), Root: s.env.Root()}, permission.Request{
		SessionID: s.p.SessionID, ToolCallID: string(r.ToolCall.ToolCallId), Tool: toolName(string(kind)), Kind: kind,
		Title: title, Input: input, Paths: paths, Command: command, Options: options,
	})
	pick := func(kinds ...sdk.PermissionOptionKind) (sdk.RequestPermissionResponse, error) {
		if d.OptionID != "" {
			return sdk.RequestPermissionResponse{Outcome: sdk.NewRequestPermissionOutcomeSelected(sdk.PermissionOptionId(d.OptionID))}, nil
		}
		for _, k := range kinds {
			for _, o := range r.Options {
				if o.Kind == k {
					return sdk.RequestPermissionResponse{Outcome: sdk.NewRequestPermissionOutcomeSelected(o.OptionId)}, nil
				}
			}
		}
		return sdk.RequestPermissionResponse{Outcome: sdk.NewRequestPermissionOutcomeCancelled()}, nil
	}
	switch d.Outcome {
	case permission.Allowed:
		s.mu.Lock()
		s.approved = approval{paths: paths, command: command, until: time.Now().Add(2 * time.Minute)}
		s.mu.Unlock()
		return pick(sdk.PermissionOptionKindAllowOnce, sdk.PermissionOptionKindAllowAlways)
	case permission.Denied:
		return pick(sdk.PermissionOptionKindRejectOnce, sdk.PermissionOptionKindRejectAlways)
	}
	return sdk.RequestPermissionResponse{Outcome: sdk.NewRequestPermissionOutcomeCancelled()}, nil
}

// covered reports whether a just-approved request covers this path or
// command, and uses the approval up for that path.
func (s *session) covered(path, command string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.approved
	if time.Now().After(a.until) {
		return false
	}
	if command != "" {
		return a.command != "" && strings.Join(strings.Fields(a.command), " ") == strings.Join(strings.Fields(command), " ")
	}
	return slices.Contains(a.paths, path)
}

// ask sends a file or terminal request the agent did not ask permission
// for to the broker, so no side effect bypasses it.
func (s *session) ask(ctx context.Context, kind permission.Kind, title, path, command string) error {
	var paths []string
	if path != "" {
		paths = []string{path}
	}
	tool := toolName(string(kind))
	d := s.p.Broker.Request(ctx, permission.Context{Mode: s.Mode(), Root: s.env.Root()}, permission.Request{
		SessionID: s.p.SessionID, ToolCallID: fmt.Sprintf("acp-%d", time.Now().UnixNano()), Tool: tool, Kind: kind,
		Title: title, Paths: paths, Command: command,
	})
	if d.Outcome != permission.Allowed {
		msg := "permission denied"
		if d.Reason != "" {
			msg += ": " + d.Reason
		}
		return errors.New(msg)
	}
	return nil
}

// ReadTextFile reads through the environment. Reads inside the project are
// allowed by policy; outside it the broker asks.
func (s *session) ReadTextFile(ctx context.Context, r sdk.ReadTextFileRequest) (sdk.ReadTextFileResponse, error) {
	path := s.env.Resolve(r.Path)
	if !s.covered(path, "") {
		if err := s.ask(ctx, permission.KindRead, "Read "+filepath.Base(path), path, ""); err != nil {
			return sdk.ReadTextFileResponse{}, err
		}
	}
	b, err := s.env.FS().ReadFile(path)
	if err != nil {
		return sdk.ReadTextFileResponse{}, err
	}
	content := string(b)
	if r.Line != nil || r.Limit != nil {
		lines := strings.Split(content, "\n")
		start := 0
		if r.Line != nil && *r.Line > 1 {
			start = min(*r.Line-1, len(lines))
		}
		end := len(lines)
		if r.Limit != nil && *r.Limit > 0 && start+*r.Limit < end {
			end = start + *r.Limit
		}
		content = strings.Join(lines[start:end], "\n")
	}
	return sdk.ReadTextFileResponse{Content: content}, nil
}

// WriteTextFile writes through the environment, asking the broker unless
// the user just approved a change to that file.
func (s *session) WriteTextFile(ctx context.Context, r sdk.WriteTextFileRequest) (sdk.WriteTextFileResponse, error) {
	path := s.env.Resolve(r.Path)
	if !s.covered(path, "") {
		if err := s.ask(ctx, permission.KindEdit, "Write "+filepath.Base(path), path, ""); err != nil {
			return sdk.WriteTextFileResponse{}, err
		}
	}
	return sdk.WriteTextFileResponse{}, s.env.FS().WriteFile(path, []byte(r.Content))
}

// terminal is one command the agent runs through the environment.
type terminal struct {
	cancel context.CancelFunc
	done   chan struct{}
	res    environment.ExecResult
	err    error
}

func (s *session) CreateTerminal(ctx context.Context, r sdk.CreateTerminalRequest) (sdk.CreateTerminalResponse, error) {
	line := strings.TrimSpace(r.Command + " " + strings.Join(r.Args, " "))
	if !s.covered("", line) {
		if err := s.ask(ctx, permission.KindExecute, "Run "+line, "", line); err != nil {
			return sdk.CreateTerminalResponse{}, err
		}
	}
	env := map[string]string{}
	for _, v := range r.Env {
		env[v.Name] = v.Value
	}
	spec := environment.ExecSpec{Argv: append([]string{r.Command}, r.Args...), Env: env, Timeout: environment.MaxTimeout}
	if len(r.Args) == 0 && strings.ContainsAny(r.Command, " ;&|") {
		spec = environment.ExecSpec{Command: r.Command, Env: env, Timeout: environment.MaxTimeout}
	}
	if r.Cwd != nil {
		spec.Cwd = *r.Cwd
	}
	if r.OutputByteLimit != nil && *r.OutputByteLimit > 0 {
		spec.MaxOutput = *r.OutputByteLimit
	}
	tctx, cancel := context.WithCancel(context.Background())
	t := &terminal{cancel: cancel, done: make(chan struct{})}
	go func() {
		t.res, t.err = s.env.Exec(tctx, spec)
		close(t.done)
	}()
	s.mu.Lock()
	s.next++
	id := fmt.Sprintf("term-%d", s.next)
	s.terms[id] = t
	s.mu.Unlock()
	return sdk.CreateTerminalResponse{TerminalId: id}, nil
}

func (s *session) terminal(id string) (*terminal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.terms[id]
	if !ok {
		return nil, fmt.Errorf("no terminal %s", id)
	}
	return t, nil
}

func exitStatus(t *terminal) *sdk.TerminalExitStatus {
	code := t.res.ExitCode
	return &sdk.TerminalExitStatus{ExitCode: &code}
}

func (s *session) TerminalOutput(_ context.Context, r sdk.TerminalOutputRequest) (sdk.TerminalOutputResponse, error) {
	t, err := s.terminal(r.TerminalId)
	if err != nil {
		return sdk.TerminalOutputResponse{}, err
	}
	select {
	case <-t.done:
		return sdk.TerminalOutputResponse{Output: t.res.Output, Truncated: t.res.Truncated, ExitStatus: exitStatus(t)}, nil
	default:
		return sdk.TerminalOutputResponse{}, nil
	}
}

func (s *session) WaitForTerminalExit(ctx context.Context, r sdk.WaitForTerminalExitRequest) (sdk.WaitForTerminalExitResponse, error) {
	t, err := s.terminal(r.TerminalId)
	if err != nil {
		return sdk.WaitForTerminalExitResponse{}, err
	}
	select {
	case <-t.done:
	case <-ctx.Done():
		return sdk.WaitForTerminalExitResponse{}, ctx.Err()
	}
	code := t.res.ExitCode
	return sdk.WaitForTerminalExitResponse{ExitCode: &code}, nil
}

func (s *session) KillTerminal(_ context.Context, r sdk.KillTerminalRequest) (sdk.KillTerminalResponse, error) {
	t, err := s.terminal(r.TerminalId)
	if err != nil {
		return sdk.KillTerminalResponse{}, err
	}
	t.cancel()
	return sdk.KillTerminalResponse{}, nil
}

func (s *session) ReleaseTerminal(_ context.Context, r sdk.ReleaseTerminalRequest) (sdk.ReleaseTerminalResponse, error) {
	t, err := s.terminal(r.TerminalId)
	if err != nil {
		return sdk.ReleaseTerminalResponse{}, err
	}
	t.cancel()
	s.mu.Lock()
	delete(s.terms, r.TerminalId)
	s.mu.Unlock()
	return sdk.ReleaseTerminalResponse{}, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

// sessionMeta is what session/new and session/load carry beyond the
// protocol: the session's skills as a local plugin, for an agent whose
// preset takes them that way (SKL-008). The plugin brings no MCP servers;
// the session's own reach the agent through the protocol.
func (s *session) sessionMeta() map[string]any {
	if s.preset.Skills != config.SkillsClaudePlugin || s.p.SkillPlugin == "" {
		return nil
	}
	return map[string]any{"claudeCode": map[string]any{"options": map[string]any{
		"plugins": []map[string]any{{"type": "local", "path": s.p.SkillPlugin, "skipMcpDiscovery": true}},
	}}}
}
