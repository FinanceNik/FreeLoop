package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/acp"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/session"
	"github.com/FinanceNik/Freeloop/internal/store"
)

// runtime is a live session: its harness session and the prompt it is
// running, if any. Prompts sent while one runs wait in a queue and start at
// the next turn boundary (GUI-004).
type runtime struct {
	hs        harness.Session
	sid       string
	harnessID string
	// agentFiles fingerprints the folder of an agent its preset watches,
	// as it was when the agent started (HRN-011).
	agentFiles string
	projectID  string
	profileID  string
	// Tool calls in progress, for post_tool_use hooks (EXT-005).
	callsMu sync.Mutex
	calls   map[string]hookCall
	// Hooks the session was told do not run for want of approval (SET-005).
	unapproved map[string]bool
	env        environment.Environment
	// host is the session's folder on this Mac, where the app's own git
	// commands run even when env is a container.
	host *environment.Local
	// noCheckpoints is set once a snapshot failed (SES-014).
	noCheckpoints bool
	// optimizer rewrites a built-in session's commands (HRN-019).
	optimizer *sessionOptimizer

	// Git state, refreshed after tool calls and turns (SES-006).
	gitMu   sync.Mutex
	base    string
	lastGit event.GitStateData

	// jobs are the git refreshes in flight, which stopRuntime waits for.
	jobs sync.WaitGroup

	mu   sync.Mutex
	busy bool
	// stopped is set once stop began: nothing starts on this runtime
	// again, and a caller that fetched it earlier fetches a new one.
	stopped bool
	cancel  context.CancelFunc
	// closeEgress stops the session's proxy to its listed hosts (ENV-010).
	closeEgress func()
	// handoff goes with the session's first prompt (HRN-012).
	handoff *harness.Resource
	// turn is the running turn's context, which Stop cancels; hooks run
	// for the turn use it.
	turn  context.Context
	queue []queued
	done  chan struct{} // closed when the current prompt and queue finish
}

func (r *runtime) stop() {
	r.mu.Lock()
	r.stopped = true
	r.queue = nil
	if r.cancel != nil {
		r.cancel()
	}
	done := r.done
	r.mu.Unlock()
	if done != nil {
		<-done
	}
	if r.hs != nil {
		r.hs.Close()
	}
	if r.closeEgress != nil {
		r.closeEgress()
	}
	// A session's container stops with it and starts again on its next
	// prompt.
	if d, ok := r.env.(*environment.Docker); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		d.Stop(ctx)
		cancel()
	}
}

// runtimeFor returns the live session for sid, starting it on first use:
// environment, provider, restored history and the built-in harness.
func (e *Engine) runtimeFor(ctx context.Context, sid string) (*runtime, error) {
	e.mu.Lock()
	if r, ok := e.runtimes[sid]; ok {
		e.mu.Unlock()
		return r, nil
	}
	// One start per session at a time: a second caller waits for the
	// first and then finds its runtime, so a session's container starts
	// once and its session_start hooks run once.
	lock, _ := e.startLocks.LoadOrStore(sid, &sync.Mutex{})
	e.mu.Unlock()
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()
	e.mu.Lock()
	if r, ok := e.runtimes[sid]; ok {
		e.mu.Unlock()
		return r, nil
	}
	e.mu.Unlock()

	// A new worktree is set up before its session starts (SES-016).
	if err := e.waitSetup(ctx, sid); err != nil {
		return nil, err
	}
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			e.startLocks.Delete(sid) // no lock kept for an ID that is no session
		}
		return nil, err
	}
	var preset config.HarnessPreset
	var p *configuredProvider
	if ses.HarnessID == builtin.ID {
		if p, err = e.provider(ses.ProviderID); err != nil {
			return nil, err
		}
		if err := e.requireTools(ctx, ses.ProviderID, ses.Model); err != nil {
			return nil, err
		}
	} else if preset, err = e.runnablePreset(ses.HarnessID); err != nil {
		return nil, err
	}
	envID := ses.EnvironmentID
	if envID == "" {
		envID = DefaultEnvironmentID
	}
	envCfg, err := e.usableEnvironment(envID)
	if err != nil {
		return nil, err
	}
	project := e.sessionProject(ctx, ses)
	prof, err := e.profileFor(project, ses.ProfileID)
	if err != nil {
		return nil, err
	}
	e.startSessionServers(prof)
	effort := sessionEffort(ses, prof)
	vars, err := e.resolveEnv(envCfg)
	if err != nil {
		return nil, err
	}
	if ses.PortBase > 0 {
		vars["FREELOOP_PORT"] = strconv.Itoa(ses.PortBase)
	}
	if !environment.IsDir(ses.WorkRoot()) {
		return nil, missingRoot(ses)
	}
	host, err := e.localEnv(ses.WorkRoot(), vars)
	if err != nil {
		return nil, err
	}
	// An external agent that takes skills gets the session's (SKL-008).
	plugin, pluginDir := e.sessionPlugin(sid, preset, envCfg.Kind == string(environment.KindDocker), project, prof)
	var env environment.Environment = host
	closeEgress := func() {}
	if envCfg.Kind == string(environment.KindDocker) {
		if env, err = e.dockerEnv(ctx, ses, project, envCfg, vars, host, pluginDir); err != nil {
			return nil, err
		}
	} else if envCfg.SandboxOn() {
		sandboxed, closeIt, err := e.sandboxEnv(ctx, ses, envCfg, vars)
		if err != nil {
			return nil, err
		}
		if err := e.startSandbox(ctx, sid, sandboxed); err != nil {
			closeIt()
			return nil, err
		}
		env, closeEgress = sandboxed, closeIt
	}
	// A container started for a runtime that then fails to start stops
	// again instead of running on.
	ready := false
	defer func() {
		e.mu.Lock()
		inUse := e.runtimes[sid] != nil
		e.mu.Unlock()
		if !ready && !inUse {
			closeEgress()
		}
		if d, ok := env.(*environment.Docker); ok && !ready && !inUse {
			sctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			d.Stop(sctx)
		}
	}()
	events, err := e.store.Events(ctx, sid, 0)
	if err != nil {
		return nil, err
	}
	e.rules.restore(sid, events)
	r := &runtime{env: env, host: host, closeEgress: closeEgress, base: ses.BaseCommit, sid: sid, harnessID: ses.HarnessID, projectID: ses.ProjectID, profileID: ses.ProfileID, handoff: pendingHandoff(events)}
	if preset.Watch != "" {
		r.agentFiles = config.DirFingerprint(preset.Watch)
	}
	e.mu.Lock()
	e.starting[sid] = r
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		if e.starting[sid] == r {
			delete(e.starting, sid)
		}
		e.mu.Unlock()
	}()
	if why := e.runHooks(ctx, r, config.HookSessionStart, "", map[string]any{
		"harness": ses.HarnessID, "providerId": ses.ProviderID, "model": ses.Model, "profileId": ses.ProfileID, "mode": ses.Mode,
	}); why != "" {
		return nil, fmt.Errorf("a session_start hook stopped the session: %s", why)
	}
	lastCommands := []byte(`{"availableCommands":[]}`)
	for _, ev := range events {
		switch ev.Type {
		case event.GitState:
			json.Unmarshal(ev.Data, &r.lastGit)
		case event.AvailableCommandsUpdate:
			lastCommands = ev.Data
		}
	}
	commands, advertised := commandsOf(prof)
	if b, _ := json.Marshal(advertised); !bytes.Equal(b, lastCommands) {
		// The profile's commands changed since the session last ran.
		e.emit(sid, event.AvailableCommandsUpdate, advertised)
	}
	history := builtin.Restore(events)
	if n := builtin.LostMessages(history); n > 0 {
		e.emit(sid, event.Notice, event.NoticeData{Message: fmt.Sprintf("%d messages of this conversation could not be read from its saved record, so the model does not have them. The rest it still has.", n)})
	}
	params := harness.Params{
		SessionID:      sid,
		Env:            env,
		Broker:         e.broker,
		Model:          ses.Model,
		Effort:         effort,
		Options:        lastOptions(events),
		Mode:           permission.Mode(ses.Mode),
		History:        history,
		Attachment:     func(name string) ([]byte, error) { return e.store.Attachment(sid, name) },
		DisabledTools:  envCfg.DisabledTools,
		SystemPrompt:   prof.SystemPrompt,
		Tools:          prof.Tools,
		MaxTurns:       prof.MaxTurns,
		NoInstructions: prof.Instructions == "none",
		Commands:       commands,
		Trusted:        func() bool { return e.trusted(ses.ProjectID) },
		Web:            sessionWeb{e: e},
		MCP:            sessionMCP{e: e, sid: sid, projectID: ses.ProjectID, root: ses.ProjectRoot, profileID: ses.ProfileID},
		Skills:         sessionSkills{e: e, projectID: ses.ProjectID, root: ses.ProjectRoot, profileID: ses.ProfileID},
		SkillPlugin:    plugin,
		Emit: func(typ event.Type, data any) {
			e.emit(sid, typ, data)
			e.toolHooks(r, typ, data)
			if changesFiles(typ, data) {
				e.refreshGitLater(r)
			}
		},
	}
	// A subagent gets no task tool of its own (HRN-014).
	if ses.ParentID == "" {
		params.Subagents = sessionSubagents{e: e, sid: sid}
	}
	var hs harness.Session
	if p != nil {
		params.Provider = e.limit(p, sid)
		if f := e.fallbackOf(prof); f != nil {
			if fp, err := e.provider(f.Provider); err == nil {
				params.Fallback = &harness.Fallback{Provider: e.limit(fp, sid), Model: f.Model}
			} else {
				log.Printf("engine: the fallback of profile %s: %s", prof.ID, e.redactText(err.Error()))
			}
		}
		if caps, err := e.ModelCapabilities(ctx, ses.ProviderID, ses.Model, false); err == nil {
			params.ContextWindow = caps.ContextWindow
		}
		r.optimizer = &sessionOptimizer{e: e, sid: sid, env: env}
		params.Optimizer = r.optimizer
		hs, err = builtin.Harness{}.NewSession(ctx, params)
	} else {
		if e.agentTakesOptimizer(ses.HarnessID, envCfg) {
			r.optimizer = &sessionOptimizer{e: e, sid: sid, env: env, agent: true}
			params.Optimizer = r.optimizer
		}
		params.MCPServers = e.agentServers(prof)
		params.ExternalID = ses.ExternalID
		params.KeepTitle = ses.ImportedFrom != ""
		hs, err = acp.Harness{Preset: preset}.NewSession(ctx, params)
	}
	if err != nil {
		return nil, err
	}
	if rs, ok := hs.(harness.Resumable); ok && rs.ExternalID() != ses.ExternalID {
		if err := e.store.SetExternalID(ctx, sid, rs.ExternalID()); err != nil {
			log.Printf("engine: record the external session of %s: %v", sid, err)
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	// The container is in use either way: by this runtime, or by the one
	// that won the race, which shares it.
	ready = true
	if existing, ok := e.runtimes[sid]; ok { // another caller won the race
		hs.Close()
		return existing, nil
	}
	if e.closing {
		hs.Close()
		return nil, errClosing
	}
	r.hs = hs
	e.refreshGitLater(r)
	e.runtimes[sid] = r
	return r, nil
}

// changesFiles reports events after which the checkout may have changed: a
// finished tool call or a finished turn.
func changesFiles(typ event.Type, data any) bool {
	if typ == event.TurnEnded {
		return true
	}
	if typ != event.ToolCallUpdate {
		return false
	}
	m, ok := data.(map[string]any)
	return ok && m["status"] == "completed"
}

// refreshGitLater refreshes the git state in the background, counted in
// the runtime's jobs.
func (e *Engine) refreshGitLater(r *runtime) {
	r.jobs.Add(1)
	e.bg.Go(func() {
		defer r.jobs.Done()
		e.refreshGit(r)
	})
}

// refreshGit records the session's git state when it changed.
func (e *Engine) refreshGit(r *runtime) {
	r.gitMu.Lock()
	defer r.gitMu.Unlock()
	st := gitState(context.Background(), r.host, e.trusted(r.projectID))
	st.BaseCommit = r.base
	if st == r.lastGit || (!st.Repo && !r.lastGit.Repo) {
		return
	}
	r.lastGit = st
	e.emit(r.sid, event.GitState, st)
}

// errStopped is a message sent while its session was being stopped, by an
// archive, a delete or a restart of its agent.
var errStopped = errors.New("the session stopped while the message was sent; send it again")

// errClosing refuses to start a session while the app quits.
var errClosing = errors.New("Freeloop is quitting")

// PromptResult says whether the prompt started or waits for the current one.
type PromptResult struct {
	Queued bool `json:"queued"`
}

// Prompt sends a user message to a session. It returns at once; the turn
// runs in the background and reports through events.
func (e *Engine) Prompt(ctx context.Context, sid, text string) (PromptResult, error) {
	return e.PromptWith(ctx, sid, PromptRequest{Text: text})
}

// PromptRequest is a message with MCP resources attached (MCP-007).
type PromptRequest struct {
	Text      string        `json:"text"`
	Resources []ResourceRef `json:"resources"`
	// Images are pictures the user attached (SES-015).
	Images []ImageInput `json:"images"`
	// Files are text files the user attached from anywhere on the Mac (SES-019).
	Files []FileInput `json:"files"`
}

// queued is a prompt waiting for its turn. Resources are read when it
// runs; inline ones came with an MCP prompt.
type queued struct {
	text   string
	refs   []ResourceRef
	inline []harness.Resource
	// images were stored as the session's attachments when queued.
	images []provider.Image
	// noMentions is set for text the user did not type (an MCP prompt's, a
	// subagent's, a review's): its @ words attach no files.
	noMentions bool
}

// PromptWith sends a message with attached resources; see Prompt.
func (e *Engine) PromptWith(ctx context.Context, sid string, req PromptRequest) (PromptResult, error) {
	images, err := e.storeImages(ctx, sid, req.Images)
	if err != nil {
		return PromptResult{}, err
	}
	files, err := attachedFiles(req.Files, len(req.Text))
	if err != nil {
		return PromptResult{}, err
	}
	return e.enqueue(ctx, sid, queued{text: req.Text, refs: req.Resources, images: images, inline: files})
}

func (e *Engine) enqueue(ctx context.Context, sid string, q queued) (PromptResult, error) {
	text := q.text
	if text == "" && len(q.images) == 0 && len(q.inline) == 0 {
		return PromptResult{}, errors.New("empty message")
	}
	r, err := e.runtimeFor(ctx, sid)
	if errors.Is(err, store.ErrNotFound) {
		return PromptResult{}, err
	}
	if err != nil {
		e.emit(sid, event.Error, event.ErrorData{Message: err.Error()})
		e.emit(sid, event.SessionStatus, event.SessionStatusData{Status: string(session.StatusError)})
		return PromptResult{}, err
	}
	if ses, err := e.store.GetSession(ctx, sid); err == nil && ses.Title == defaultTitle {
		if t := titleFrom(text); t != "" {
			e.emit(sid, event.SessionInfoUpdate, map[string]string{"title": t})
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return PromptResult{}, errStopped
	}
	if r.busy {
		r.queue = append(r.queue, q)
		return PromptResult{Queued: true}, nil
	}
	r.busy = true
	r.done = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel, r.turn = cancel, ctx
	go e.run(r, q, ctx)
	return PromptResult{}, nil
}

// run works through a prompt and then the queue. Each prompt's cancel
// function is installed under r.mu together with taking the prompt, so a
// Cancel can never fall between the two.
func (e *Engine) run(r *runtime, q queued, ctx context.Context) {
	for {
		e.checkpoint(r)
		p := harness.Prompt{Text: q.text}
		if !q.noMentions {
			p = e.embedMentions(r.sid, r.env, q.text)
		}
		p.Images = q.images
		r.mu.Lock()
		if r.handoff != nil {
			p.Resources = append([]harness.Resource{*r.handoff}, p.Resources...)
			r.handoff = nil
		}
		r.mu.Unlock()
		p.Resources = append(p.Resources, q.inline...)
		p.Resources = append(p.Resources, e.readResources(ctx, r, q.refs)...)
		err := e.promptSafely(r, ctx, p)
		if err != nil {
			log.Printf("engine: prompt: %s", e.redactText(err.Error()))
		}
		r.mu.Lock()
		r.cancel()
		r.cancel = nil
		// The agent is gone. Drop the runtime, so the next message starts it again
		// and asks it to load the session (NFR-REL-002).
		if errors.Is(err, harness.ErrAgentExited) {
			dropped := len(r.queue)
			r.queue = nil
			r.busy = false
			close(r.done)
			r.mu.Unlock()
			msg := "The agent stopped. Your next message starts it again."
			if dropped > 0 {
				msg = fmt.Sprintf("The agent stopped, so %d queued messages were not sent. Your next message starts it again.", dropped)
			}
			e.emit(r.sid, event.Notice, event.NoticeData{Message: msg})
			e.bg.Go(func() { e.stopRuntime(r.sid) })
			return
		}
		// Queued prompts would fail the same way until the user signs in.
		if errors.Is(err, harness.ErrSignIn) && len(r.queue) > 0 {
			n := len(r.queue)
			r.queue = nil
			e.emit(r.sid, event.Notice, event.NoticeData{Message: fmt.Sprintf("%d queued %s not sent; send %s again after signing in.", n, plural(n, "message was", "messages were"), plural(n, "it", "them"))})
		}
		if len(r.queue) == 0 {
			r.busy = false
			close(r.done)
			r.mu.Unlock()
			return
		}
		q, r.queue = r.queue[0], r.queue[1:]
		ctx, r.cancel = context.WithCancel(context.Background())
		r.turn = ctx
		r.mu.Unlock()
	}
}

// promptSafely runs a prompt, and turns a panic in it into this session's error: a
// bug in one turn must not take down the app and every other session with it.
func (e *Engine) promptSafely(r *runtime, ctx context.Context, p harness.Prompt) (err error) {
	defer func() {
		if v := recover(); v != nil {
			log.Printf("engine: session %s panicked: %v\n%s", r.sid, v, debug.Stack())
			err = fmt.Errorf("internal error: %v", v)
			e.emit(r.sid, event.Error, map[string]string{"message": "Something went wrong inside Freeloop and this turn stopped. The details are in its log."})
			e.emit(r.sid, event.TurnEnded, map[string]string{"stopReason": string(harness.StopError)})
			e.emit(r.sid, event.SessionStatus, map[string]string{"status": string(session.StatusError)})
		}
	}()
	_, err = r.hs.Prompt(ctx, p)
	return err
}

// release ends something other than a turn that held the session busy (a
// rewind), starting the queued prompts if any.
func (e *Engine) release(r *runtime) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.queue) == 0 || r.stopped {
		r.queue = nil
		r.busy = false
		close(r.done)
		return
	}
	var q queued
	q, r.queue = r.queue[0], r.queue[1:]
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel, r.turn = cancel, ctx
	go e.run(r, q, ctx)
}

// Cancel stops a session's running turn and drops its queued prompts; the
// user asked for it.
func (e *Engine) Cancel(sid string) error {
	e.stopTurn(sid, "You stopped the turn.")
	return nil
}

// stopTurn cancels a session's running turn and drops its queued prompts,
// saying in the transcript why, since an agent reports every stop the same
// way.
func (e *Engine) stopTurn(sid, why string) {
	e.mu.Lock()
	r, ok := e.runtimes[sid]
	e.mu.Unlock()
	if !ok {
		return
	}
	r.mu.Lock()
	r.queue = nil
	running := r.cancel != nil
	if running {
		r.cancel()
	}
	r.mu.Unlock()
	if running {
		e.emit(sid, event.Notice, event.NoticeData{Message: why})
	}
}

// SetMode changes a session's permission mode; it applies from the next tool
// call. Bypass needs permissions.allowBypass in config.yaml (SEC-001).
func (e *Engine) SetMode(ctx context.Context, sid, mode string) error {
	m := permission.Mode(mode)
	if !m.Valid() {
		return fmt.Errorf("unknown mode %q", mode)
	}
	if m == permission.ModeBypass && !e.Config().Permissions.AllowBypass {
		return errors.New("bypass mode is off; turn it on in Settings to allow it")
	}
	if m == permission.ModeBypass && !e.commandApproved(config.BypassFingerprint()) {
		return errors.New("bypass mode was turned on in config.yaml and has not been approved; approve it in Settings")
	}
	if m == permission.ModeBypass {
		ses, err := e.store.GetSession(ctx, sid)
		if err != nil {
			return err
		}
		if !e.trusted(ses.ProjectID) {
			return errors.New("bypass mode is not available in a project you have not trusted")
		}
	}
	e.mu.Lock()
	r, ok := e.runtimes[sid]
	e.mu.Unlock()
	if ok {
		r.hs.SetMode(m)
		return nil
	}
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return err
	}
	if ses.Mode != mode {
		e.emit(sid, event.CurrentModeUpdate, event.CurrentModeData{CurrentModeID: mode})
	}
	return nil
}

// DecideRequest answers a pending permission request.
type DecideRequest struct {
	RequestID string `json:"requestId"`
	OptionID  string `json:"optionId"`
	Reason    string `json:"reason"`
	// Scope for "Allow always": session (default) or global.
	Scope string `json:"scope"`
	// Answers fill in a form request (MCP-008).
	Answers map[string]any `json:"answers,omitempty"`
}

// Decide answers a pending permission request.
func (e *Engine) Decide(req DecideRequest) error {
	return e.broker.DecideWith(req.RequestID, req.OptionID, req.Reason, permission.Scope(req.Scope), req.Answers)
}

// PendingPermissions returns every request waiting for the user, across
// sessions, oldest first.
func (e *Engine) PendingPermissions() []permission.Request {
	list := e.broker.Pending()
	for i, r := range list {
		// The transcript shows these redacted; so does this.
		list[i].Title, list[i].Command, list[i].Input = e.redactText(r.Title), e.redactText(r.Command), e.redact(r.Input)
	}
	return list
}

// recover closes out sessions the app left mid-turn: pending requests are
// recorded as cancelled, the open turn as ended, and the session as idle.
func (e *Engine) recover(ctx context.Context) {
	list, err := e.store.ListSessions(ctx)
	if err != nil {
		log.Printf("engine: recover: %v", err)
		return
	}
	for _, ses := range list {
		if ses.Status != session.StatusRunning && ses.Status != session.StatusWaitingPermission && ses.Status != session.StatusQueued {
			continue
		}
		events, err := e.store.Events(ctx, ses.ID, 0)
		if err != nil {
			continue
		}
		open := map[string]string{} // requestId -> toolCallId
		inTurn := false
		for _, ev := range events {
			var d struct {
				RequestID  string `json:"requestId"`
				ToolCallID string `json:"toolCallId"`
			}
			json.Unmarshal(ev.Data, &d)
			switch ev.Type {
			case event.PermissionRequested:
				open[d.RequestID] = d.ToolCallID
			case event.PermissionDecided:
				delete(open, d.RequestID)
			case event.TurnStarted:
				inTurn = true
			case event.TurnEnded:
				inTurn = false
			}
		}
		for id, call := range open {
			e.emit(ses.ID, event.PermissionDecided, permission.Decision{
				RequestID: id, ToolCallID: call, Outcome: permission.Cancelled, Actor: permission.ActorUser, Reason: "the app quit",
			})
		}
		if inTurn {
			e.emit(ses.ID, event.TurnEnded, event.TurnEndedData{StopReason: string(harness.StopCancelled)})
		}
		e.emit(ses.ID, event.SessionStatus, event.SessionStatusData{Status: string(session.StatusIdle)})
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
