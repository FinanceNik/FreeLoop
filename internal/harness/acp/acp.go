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
	"io/fs"
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

// resourceNotFound is ACP's error code for a file or session that is not there.
const resourceNotFound = -32002

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
	// options are what the agent reports it can change (PRV-014).
	options []harness.Option
}

type approval struct {
	kind    permission.Kind
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
			s.restoreOptions(ctx)
			return s, nil
		}
	}
	ns, err := s.conn.NewSession(ctx, sdk.NewSessionRequest{Cwd: s.env.Root(), McpServers: s.mcpServers(), Meta: s.sessionMeta()})
	if err != nil {
		if len(init.AuthMethods) > 0 {
			how := "sign in with the agent's own command first (Freeloop does not handle its login)"
			if s.preset.Notes != "" {
				how = s.preset.Notes
			}
			err = fmt.Errorf("%w; %s", err, how)
		}
		return fail("new session", err)
	}
	s.id = ns.SessionId
	s.applyMode(ctx)
	s.reportOptions(ns.ConfigOptions)
	s.restoreOptions(ctx)
	return s, nil
}

// reportOptions takes what the agent said it can change, and records it, so the
// app knows the choices while the agent is not running.
func (s *session) reportOptions(options []sdk.SessionConfigOption) {
	if len(options) == 0 {
		return
	}
	raw, err := json.Marshal(options)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.options = harness.ParseOptions(raw)
	s.mu.Unlock()
	var m []any
	if json.Unmarshal(raw, &m) == nil {
		s.p.Emit(event.ConfigOptionUpdate, map[string]any{"configOptions": m})
	}
}

// Options implements harness.Configurable.
func (s *session) Options() []harness.Option {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.options)
}

// SetOption implements harness.Configurable. Only what the agent listed can be
// asked for, so an agent's mode or anything else it offers stays out of reach.
func (s *session) SetOption(ctx context.Context, id, value string) error {
	s.mu.Lock()
	i := slices.IndexFunc(s.options, func(o harness.Option) bool { return o.ID == id })
	listed := i >= 0 && slices.ContainsFunc(s.options[i].Choices, func(c harness.OptionChoice) bool { return c.Value == value })
	s.mu.Unlock()
	if !listed {
		return fmt.Errorf("%s does not offer %q for %q", s.preset.Name, value, id)
	}
	res, err := s.conn.SetSessionConfigOption(ctx, sdk.SetSessionConfigOptionRequest{ValueId: &sdk.SetSessionConfigOptionValueId{
		SessionId: s.id, ConfigId: sdk.SessionConfigId(id), Value: sdk.SessionConfigValueId(value),
	}})
	if err != nil {
		return fmt.Errorf("%s refused it: %w", s.preset.Name, err)
	}
	s.reportOptions(res.ConfigOptions)
	return nil
}

// restoreOptions puts back the choices from before the agent last stopped. One
// the agent no longer offers stays at what it chose by itself.
func (s *session) restoreOptions(ctx context.Context) {
	for _, want := range s.p.Options {
		s.mu.Lock()
		i := slices.IndexFunc(s.options, func(o harness.Option) bool { return o.ID == want.ID })
		same := i >= 0 && s.options[i].Current == want.Value
		s.mu.Unlock()
		if same || i < 0 {
			continue
		}
		if err := s.SetOption(ctx, want.ID, want.Value); err != nil {
			log.Printf("acp: put back option %s: %v", want.ID, err)
		}
	}
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
		loaded, err := s.conn.LoadSession(ctx, sdk.LoadSessionRequest{SessionId: sdk.SessionId(id), Cwd: s.env.Root(), McpServers: s.mcpServers(), Meta: s.sessionMeta()})
		s.mu.Lock()
		s.loading = false
		s.mu.Unlock()
		if err == nil {
			s.id = sdk.SessionId(id)
			s.reportOptions(loaded.ConfigOptions)
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
	if s.conn == nil {
		return // not started
	}
	if !ok {
		if m == permission.ModePlan {
			s.p.Emit(event.Notice, event.NoticeData{Message: s.preset.Name + " has no plan mode to switch to. What it asks the app is held read-only; tools of its own may still change files."})
		}
		return
	}
	if _, err := s.conn.SetSessionMode(ctx, sdk.SetSessionModeRequest{SessionId: s.id, ModeId: sdk.SessionModeId(id)}); err != nil {
		log.Printf("acp: set mode %s: %v", id, err)
		// What holds the agent back is its own mode, so this is the user's to know.
		s.p.Emit(event.Notice, event.NoticeData{Message: fmt.Sprintf("%s did not accept its %s mode (%v). What it asks the app still follows the session's mode; tools of its own may not.", s.preset.Name, id, err)})
	}
}

// modeLooseness orders the modes by what they let through without asking.
func modeLooseness(m permission.Mode) int {
	switch m {
	case permission.ModePlan:
		return 0
	case permission.ModeAcceptEdits:
		return 2
	case permission.ModeBypass:
		return 3
	}
	return 1
}

// agentReported takes the agent's own name for its mode. Several of the app's
// modes can share one of the agent's, so one that matches the session's mode
// changes nothing, and otherwise the tightest match counts. A mode the preset
// does not name is one the app has no terms for, such as one that approves by
// itself: the agent is told the session's mode again.
func (s *session) agentReported(id string) {
	if len(s.preset.Modes) == 0 {
		return
	}
	if s.preset.Modes[string(s.Mode())] == id {
		return
	}
	var found permission.Mode
	for _, m := range permission.Modes {
		if s.preset.Modes[string(m)] == id && (found == "" || modeLooseness(m) < modeLooseness(found)) {
			found = m
		}
	}
	if found != "" {
		s.agentMode(found)
		return
	}
	s.p.Emit(event.Notice, event.NoticeData{Message: fmt.Sprintf("%s switched itself to its %s mode, which this app has no name for; the session's mode stays %s.", s.preset.Name, id, s.Mode())})
	s.restoreMode()
}

// restoreMode tells the agent the session's mode, outside its notification.
func (s *session) restoreMode() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.applyMode(ctx)
	}()
}

// agentMode takes a mode the agent reports. The mode decides what the
// broker lets through, so an agent can tighten it and can never loosen it on
// its own (SEC-003): only after the user approved its switch_mode request,
// and never to bypass, which only the user sets and config.yaml must allow.
// A refused change is told back to the agent, which keeps to the app's mode.
func (s *session) agentMode(m permission.Mode) {
	s.mu.Lock()
	cur := s.mode
	asked := s.approved.kind == permission.KindSwitchMode && time.Now().Before(s.approved.until)
	loosens := modeLooseness(m) > modeLooseness(cur)
	refused := loosens && (!asked || m == permission.ModeBypass)
	if m == cur || refused {
		s.mu.Unlock()
		if refused {
			s.p.Emit(event.Notice, event.NoticeData{Message: fmt.Sprintf("The agent tried to change the mode from %s to %s on its own; the mode stays %s.", cur, m, cur)})
			s.restoreMode()
		}
		return
	}
	s.mode = m
	if loosens {
		s.approved = approval{}
	}
	s.mu.Unlock()
	s.p.Emit(event.CurrentModeUpdate, event.CurrentModeData{CurrentModeID: string(m)})
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
	for _, img := range p.Images {
		s.p.Emit(event.UserMessageChunk, event.MessageChunk{Content: event.ContentBlock{Type: "image", URI: event.AttachmentURI(img.Name), MimeType: img.MediaType}})
		blocks = append(blocks, sdk.ImageBlock(img.Base64(), img.MediaType))
	}
	// An agent that did not say it reads images gets none (SES-015).
	if len(p.Images) > 0 && !s.caps.PromptCapabilities.Image {
		err := fmt.Errorf("%s does not accept images; send the message again without them", s.preset.Name)
		s.p.Emit(event.Error, event.ErrorData{Message: err.Error()})
		s.p.Emit(event.TurnEnded, event.TurnEndedData{StopReason: string(harness.StopError)})
		s.p.Emit(event.SessionStatus, event.SessionStatusData{Status: "idle"})
		return harness.StopError, err
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
		a.err = harness.ErrAgentExited
		if tail := strings.TrimSpace(s.proc.Stderr()); tail != "" {
			a.err = fmt.Errorf("%w: %s", harness.ErrAgentExited, lastLine(tail))
		}
	}
	stop := harness.StopReason(a.res.StopReason)
	if a.err != nil {
		stop = harness.StopError
		a.err = s.explain(a.err)
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
	if t == event.AgentMessageChunk && len(s.preset.HideText) > 0 {
		if c, ok := m["content"].(map[string]any); ok {
			text, _ := c["text"].(string)
			if slices.ContainsFunc(s.preset.HideText, func(p string) bool { return strings.HasPrefix(text, p) }) {
				return nil
			}
		}
	}
	s.showOriginal(m)
	switch {
	case t == event.ConfigOptionUpdate:
		if raw, err := json.Marshal(m["configOptions"]); err == nil {
			s.mu.Lock()
			s.options = harness.ParseOptions(raw)
			s.mu.Unlock()
		}
	case t == event.UsageUpdate:
		return nil
	case t == event.UserMessageChunk:
		// The agent's own words must not show as the user's. A replay while
		// loading was already skipped above.
		return nil
	case t == event.CurrentModeUpdate:
		// The agent's mode, back in the app's terms when the preset maps it.
		id, _ := m["currentModeId"].(string)
		s.agentReported(id)
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

// showOriginal puts back what the model wrote in an update about a command a token
// optimizer rewrote, and says it did.
func (s *session) showOriginal(m map[string]any) {
	if s.p.Optimizer == nil {
		return
	}
	in, ok := m["rawInput"].(map[string]any)
	if !ok {
		return
	}
	command, _ := in["command"].(string)
	original, via, ok := s.p.Optimizer.Original(command)
	if !ok {
		return
	}
	in["command"] = original
	m["title"] = original
	m["via"] = via
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
	// A command a token optimizer rewrote is judged and shown as the model wrote it:
	// that is what the user decides about and what rules name. Without this the
	// agent would ask about every rewrite, read-only ones included.
	judged, readOnly, via := command, false, ""
	if s.p.Optimizer != nil && command != "" {
		if original, by, ok := s.p.Optimizer.Original(command); ok {
			judged, title, readOnly, via = original, original, permission.ReadOnlyCommand(original), by
			if in, ok := r.ToolCall.RawInput.(map[string]any); ok {
				shown := maps.Clone(in)
				shown["command"] = original
				input, _ = json.Marshal(shown)
			}
		}
	}
	d := s.p.Broker.Request(ctx, permission.Context{Mode: s.Mode(), Root: s.env.Root()}, permission.Request{
		SessionID: s.p.SessionID, ToolCallID: string(r.ToolCall.ToolCallId), Tool: toolName(string(kind)), Kind: kind,
		Title: title, Input: input, Paths: paths, Command: judged, Options: options, RewrittenReadOnly: readOnly,
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
		s.approved = approval{kind: kind, paths: paths, command: command, until: time.Now().Add(2 * time.Minute)}
		s.mu.Unlock()
		if via != "" {
			// The agent's own updates only ever name the command the model wrote.
			s.p.Emit(event.ToolCallUpdate, map[string]any{"toolCallId": string(r.ToolCall.ToolCallId), "via": via})
		}
		return pick(sdk.PermissionOptionKindAllowOnce, sdk.PermissionOptionKindAllowAlways)
	case permission.Denied:
		return pick(sdk.PermissionOptionKindRejectOnce, sdk.PermissionOptionKindRejectAlways)
	}
	return sdk.RequestPermissionResponse{Outcome: sdk.NewRequestPermissionOutcomeCancelled()}, nil
}

// covered reports whether a just-approved request of one of these kinds
// covers this path or command. An approval covers only its own kind of
// action, so a read allowed by policy never lets a write through; spend
// uses it up, so an edit's approval covers the one write it was for and a
// command's the one run.
func (s *session) covered(kinds []permission.Kind, path, command string, spend bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.approved
	if time.Now().After(a.until) || !slices.Contains(kinds, a.kind) {
		return false
	}
	if command != "" {
		ok := a.command != "" && strings.Join(strings.Fields(a.command), " ") == strings.Join(strings.Fields(command), " ")
		if ok && spend {
			s.approved.command = ""
		}
		return ok
	}
	if !slices.Contains(a.paths, path) {
		return false
	}
	if spend {
		s.approved.paths = slices.DeleteFunc(slices.Clone(a.paths), func(p string) bool { return p == path })
	}
	return true
}

// ask sends a file or terminal request the agent did not ask permission
// for to the broker, so no side effect bypasses it.
func (s *session) ask(ctx context.Context, kind permission.Kind, title, path, command string, contained permission.Containment) error {
	var paths []string
	if path != "" {
		paths = []string{path}
	}
	tool := toolName(string(kind))
	d := s.p.Broker.Request(ctx, permission.Context{Mode: s.Mode(), Root: s.env.Root()}, permission.Request{
		SessionID: s.p.SessionID, ToolCallID: fmt.Sprintf("acp-%d", time.Now().UnixNano()), Tool: tool, Kind: kind,
		Title: title, Paths: paths, Command: command, Contained: contained,
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
	if !s.covered([]permission.Kind{permission.KindRead, permission.KindEdit}, path, "", false) {
		if err := s.ask(ctx, permission.KindRead, "Read "+filepath.Base(path), path, "", ""); err != nil {
			return sdk.ReadTextFileResponse{}, err
		}
	}
	b, err := s.env.FS().ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if s.preset.MissingFileIsEmpty {
			return sdk.ReadTextFileResponse{}, nil
		}
		// In the protocol's terms: an agent can tell that from a failure.
		return sdk.ReadTextFileResponse{}, &sdk.RequestError{Code: resourceNotFound, Message: "Resource not found", Data: map[string]any{"path": r.Path}}
	}
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
	if !s.covered([]permission.Kind{permission.KindEdit, permission.KindDelete, permission.KindMove}, path, "", true) {
		if err := s.ask(ctx, permission.KindEdit, "Write "+filepath.Base(path), path, "", ""); err != nil {
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
	// What decides what runs is part of what is approved: PATH=./evil, or a folder
	// with its own Makefile, changes what the same words do.
	for _, v := range r.Env {
		line = v.Name + "=" + v.Value + " " + line
	}
	if r.Cwd != nil && *r.Cwd != "" {
		line = "cd " + *r.Cwd + " && " + line
	}
	// This command runs here, so the sandbox can hold it, unlike the ones an
	// agent runs itself after session/request_permission (ENV-009).
	var contained permission.Containment
	readOnly := false
	if environment.IsSandboxed(s.env) {
		contained = permission.ContainedProject
		if s.Mode() == permission.ModePlan {
			contained, readOnly = permission.ContainedReadOnly, true
		}
	}
	if !s.covered([]permission.Kind{permission.KindExecute}, "", line, true) {
		if err := s.ask(ctx, permission.KindExecute, "Run "+line, "", line, contained); err != nil {
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
	spec.ReadOnly = readOnly
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

// explain turns the protocol's "authentication required" (-32000) into
// what to do: the agent signs in by itself, with its own command, which the
// preset's notes name; Freeloop never handles the login (HRN-008).
func (s *session) explain(err error) error {
	var re *sdk.RequestError
	if !errors.As(err, &re) || re.Code != -32000 {
		return err
	}
	how := "Sign in with the agent's own command in a terminal."
	if s.preset.Notes != "" {
		how = s.preset.Notes
	}
	return signInError(fmt.Sprintf("%s needs you to sign in to its own account. %s Then send your message again.", s.preset.Name, how))
}

// signInError reads as its message and counts as harness.ErrSignIn.
type signInError string

func (e signInError) Error() string        { return string(e) }
func (e signInError) Is(target error) bool { return target == harness.ErrSignIn }
