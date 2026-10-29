// Package engine is the transport-agnostic API the GUI and, from v1, the
// JSON-RPC socket are thin adapters over. Requests and responses are plain
// types; live events arrive through Subscribe.
package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/session"
	"github.com/FinanceNik/Freeloop/internal/store"
)

// ErrNotFound is returned for an unknown session.
var ErrNotFound = store.ErrNotFound

// Options configures Open.
type Options struct {
	DataDir string
	Secrets secrets.Store
	// Home is the user's home directory, for the protected-path policy.
	// Default os.UserHomeDir().
	Home string
	// Dev offers the scripted demo provider.
	Dev bool
	// WatchInterval is how often the config files are checked for hand
	// edits (SET-001). Zero means one second; negative turns watching off.
	WatchInterval time.Duration
	// OnConfigChange is called after files changed on disk were reloaded.
	OnConfigChange func(ConfigChange)
}

// ConfigChange lists the config files that were reloaded, relative to the
// data directory.
type ConfigChange struct {
	Files []string `json:"files"`
}

// Engine is safe for concurrent use.
type Engine struct {
	dataDir string
	dev     bool
	store   *store.Store
	bus     *bus
	secrets secrets.Store
	broker  *permission.Broker
	rules   *ruleStore
	cfg     config.Config
	// issues are the current problems per source file group, so a reload
	// replaces its own and leaves the others.
	issues   map[string][]config.Issue
	onChange func(ConfigChange)
	// quit stops the config watcher; watching is separate from bg, which
	// deleting a session waits for.
	quit     chan struct{}
	watching sync.WaitGroup

	// bg tracks background work (git refreshes) that Close waits for.
	bg sync.WaitGroup

	mu        sync.Mutex
	envs      []config.Environment
	profiles  []config.Profile
	providers *providerRegistry
	runtimes  map[string]*runtime
	streams   map[string]*streamBuf
	known     []string // secret values to redact
}

// Open loads configuration and opens the store in opts.DataDir. Config
// problems do not fail Open; they are reported by ConfigIssues.
func Open(opts Options) (*Engine, error) {
	if opts.Secrets == nil {
		return nil, errors.New("engine: Secrets is required")
	}
	cfg, issues, err := config.Load(opts.DataDir)
	if err != nil {
		return nil, err
	}
	if opts.Home == "" {
		if opts.Home, err = os.UserHomeDir(); err != nil {
			return nil, err
		}
	}
	e := &Engine{
		dataDir:  opts.DataDir,
		dev:      opts.Dev,
		bus:      newBus(),
		secrets:  opts.Secrets,
		cfg:      cfg,
		issues:   map[string][]config.Issue{issueConfig: issues},
		onChange: opts.OnConfigChange,
		quit:     make(chan struct{}),
		runtimes: map[string]*runtime{},
		streams:  map[string]*streamBuf{},
	}
	e.store, err = store.Open(store.Options{Dir: opts.DataDir, OnAppend: e.bus.publish, Redact: e.redact})
	if err != nil {
		return nil, err
	}
	e.broker = permission.NewBroker(permission.NewPolicy(opts.Home, opts.DataDir), e.emit)
	e.rules = newRuleStore(opts.DataDir, func(issues []config.Issue) { e.setIssues(issueRules, issues) })
	e.rules.profileRules = e.profileRules
	e.broker.SetRules(e.rules, e.rules)
	e.loadEnvironments()
	e.loadProfiles()
	if err := e.loadProviders(); err != nil {
		e.store.Close()
		return nil, err
	}
	e.rememberStoredSecrets()
	e.recover(context.Background())
	if opts.WatchInterval >= 0 {
		interval := opts.WatchInterval
		if interval == 0 {
			interval = time.Second
		}
		e.watching.Add(1)
		go e.watch(interval)
	}
	return e, nil
}

// Close cancels running turns, closes every subscription and the store.
func (e *Engine) Close() error {
	e.mu.Lock()
	running := make([]*runtime, 0, len(e.runtimes))
	for _, r := range e.runtimes {
		running = append(running, r)
	}
	e.mu.Unlock()
	// Stopped outside e.mu: a finishing turn still records events.
	for _, r := range running {
		r.stop()
	}
	close(e.quit)
	e.watching.Wait()
	e.bg.Wait()
	e.bus.close()
	return e.store.Close()
}

// Config returns the loaded configuration.
func (e *Engine) Config() config.Config {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cfg
}

// SaveSettings replaces config.yaml (SET-004, GUI-011). Turning bypass mode
// off also takes any session out of it on its next mode change.
func (e *Engine) SaveSettings(c config.Config) (config.Config, error) {
	c.Version = 1
	if err := config.Save(e.dataDir, c); err != nil {
		return config.Config{}, err
	}
	e.mu.Lock()
	e.cfg = c
	e.mu.Unlock()
	return c, nil
}

// ConfigIssues returns the problems found while loading configuration.
func (e *Engine) ConfigIssues() []config.Issue {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := []config.Issue{}
	for _, k := range issueOrder {
		out = append(out, e.issues[k]...)
	}
	return out
}

// Issue groups, one per kind of config file.
const (
	issueConfig       = "config"
	issueProviders    = "providers"
	issueEnvironments = "environments"
	issueRules        = "rules"
	issueCapabilities = "capabilities"
	issueProfiles     = "profiles"
)

var issueOrder = []string{issueConfig, issueProviders, issueEnvironments, issueProfiles, issueRules, issueCapabilities}

func (e *Engine) setIssues(group string, issues []config.Issue) {
	e.mu.Lock()
	e.issues[group] = issues
	e.mu.Unlock()
}

// streamBuf holds back the tail of a session's streamed text while it could
// be the start of a known secret, so redaction sees a secret whole even when
// the model streams it across several chunks.
type streamBuf struct {
	mu   sync.Mutex
	typ  event.Type
	text string
}

func (e *Engine) streamFor(sid string) *streamBuf {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, ok := e.streams[sid]
	if !ok {
		b = &streamBuf{}
		e.streams[sid] = b
	}
	return b
}

// heldBack returns how many trailing bytes of text could begin a known
// secret.
func (e *Engine) heldBack(text string) int {
	e.mu.Lock()
	known := e.known
	e.mu.Unlock()
	hold := 0
	for _, k := range known {
		for l := min(len(k)-1, len(text)); l > hold; l-- {
			if strings.HasSuffix(text, k[:l]) {
				hold = l
				break
			}
		}
	}
	return hold
}

// emit appends an event to a session's transcript. Streamed agent text goes
// through the session's stream buffer; any other event first flushes it, so
// order is kept. Failures are logged; a harness cannot do anything useful
// about them.
func (e *Engine) emit(sid string, typ event.Type, data any) {
	b := e.streamFor(sid)
	b.mu.Lock()
	defer b.mu.Unlock()
	flush := func() {
		if b.text != "" {
			e.write(sid, b.typ, event.MessageChunk{Content: event.ContentBlock{Type: "text", Text: b.text}})
		}
		b.typ, b.text = "", ""
	}
	if mc, ok := data.(event.MessageChunk); ok && (typ == event.AgentMessageChunk || typ == event.AgentThoughtChunk) {
		if b.typ != typ {
			flush()
		}
		text := b.text + mc.Content.Text
		hold := e.heldBack(text)
		b.typ, b.text = typ, text[len(text)-hold:]
		if out := text[:len(text)-hold]; out != "" {
			e.write(sid, typ, event.MessageChunk{Content: event.ContentBlock{Type: "text", Text: out}})
		}
		return
	}
	flush()
	e.write(sid, typ, data)
}

func (e *Engine) write(sid string, typ event.Type, data any) {
	b, err := json.Marshal(data)
	if err == nil {
		_, err = e.store.Append(context.Background(), sid, typ, b)
	}
	if err != nil {
		log.Printf("engine: record %s for %s: %v", typ, sid, err)
	}
}

// rememberStoredSecrets adds every secret the app stores (provider keys,
// environment secrets) to the redaction list, so a key pasted into any
// session is redacted even before its provider or environment is used.
func (e *Engine) rememberStoredSecrets() {
	e.mu.Lock()
	var refs []string
	for _, c := range e.providers.configs {
		if c.APIKey != "" {
			refs = append(refs, c.APIKey)
		}
	}
	for _, env := range e.envs {
		for _, ref := range env.Secrets {
			refs = append(refs, ref)
		}
	}
	e.mu.Unlock()
	for _, s := range refs {
		ref, err := secrets.ParseRef(s)
		if err != nil {
			continue
		}
		if v, err := e.secrets.Get(ref); err == nil {
			e.remember(v)
		}
	}
}

// remember adds a secret value to the redaction list.
func (e *Engine) remember(secret string) {
	if len(secret) < 8 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, k := range e.known {
		if k == secret {
			return
		}
	}
	e.known = append(e.known, secret)
}

// redact replaces known secret values in event data, in raw and in
// JSON-escaped form.
func (e *Engine) redact(data json.RawMessage) json.RawMessage {
	e.mu.Lock()
	known := e.known
	e.mu.Unlock()
	for _, k := range known {
		esc, _ := json.Marshal(k)
		for _, form := range [][]byte{[]byte(k), esc[1 : len(esc)-1]} {
			if bytes.Contains(data, form) {
				data = bytes.ReplaceAll(data, form, []byte("[REDACTED]"))
			}
		}
	}
	return data
}

// ListSessions returns all sessions, most recently updated first.
func (e *Engine) ListSessions(ctx context.Context) ([]session.Session, error) {
	return e.store.ListSessions(ctx)
}

// CreateSessionRequest is the input of CreateSession.
type CreateSessionRequest struct {
	Title       string `json:"title"`
	ProjectRoot string `json:"projectRoot"`
	// EnvironmentID defaults to the project's default environment.
	EnvironmentID string `json:"environmentId"`
	// ProfileID defaults to the project's default profile, then to the
	// default profile; the profile fills in provider, model and mode when
	// they are empty (HRN-003, HRN-004).
	ProfileID  string `json:"profileId"`
	ProviderID string `json:"providerId"`
	Model      string `json:"model"`
	Mode       string `json:"mode"`
}

// CreateSession creates an idle session and records its initial status as
// the first transcript event. Bypass mode is never a new session's mode
// (SEC-001).
func (e *Engine) CreateSession(ctx context.Context, req CreateSessionRequest) (session.Session, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = defaultTitle
	}
	if req.ProjectRoot == "" {
		return session.Session{}, errors.New("choose a project folder")
	}
	project, err := e.AddProject(ctx, req.ProjectRoot)
	if err != nil {
		return session.Session{}, err
	}
	envID := req.EnvironmentID
	if envID == "" {
		envID = project.DefaultEnvironmentID
	}
	if envID == "" {
		envID = DefaultEnvironmentID
	}
	envCfg, err := e.environment(envID)
	if err != nil {
		return session.Session{}, err
	}
	profileID := req.ProfileID
	if profileID == "" {
		profileID = project.DefaultProfileID
	}
	if profileID == "" {
		profileID = e.Config().Defaults.ProfileID
	}
	prof, err := e.profile(profileID)
	if err != nil {
		return session.Session{}, err
	}
	if req.ProviderID == "" {
		req.ProviderID = prof.Provider
	}
	if req.ProviderID == "" {
		return session.Session{}, errors.New("choose a provider")
	}
	mode := permission.Mode(req.Mode)
	if mode == "" {
		mode = permission.Mode(prof.Mode)
	}
	if mode == "" {
		mode = permission.Mode(envCfg.DefaultMode)
	}
	if !mode.Valid() || mode == permission.ModeBypass {
		mode = permission.ModeAsk
	}
	p, err := e.provider(req.ProviderID)
	if err != nil {
		return session.Session{}, err
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = prof.Model
	}
	if model == "" {
		model = p.cfg.DefaultModel
	}
	if model == "" {
		return session.Session{}, errors.New("choose a model")
	}
	if err := e.requireTools(ctx, req.ProviderID, model); err != nil {
		return session.Session{}, err
	}
	// The base commit is recorded now so the session's changes stay
	// visible after unrelated commits (SES-006).
	var git event.GitStateData
	if env, err := e.localEnv(project.Root, nil); err == nil {
		git = gitState(ctx, env)
	}
	git.BaseCommit = git.Head
	ses, err := e.store.CreateSession(ctx, session.Session{
		Title: title, HarnessID: builtin.ID, ProjectID: project.ID, ProjectRoot: project.Root, EnvironmentID: envID,
		BaseCommit: git.BaseCommit, Branch: git.Branch, ProviderID: req.ProviderID, Model: model, Mode: string(mode), ProfileID: prof.ID,
	})
	if err != nil {
		return session.Session{}, err
	}
	data, _ := json.Marshal(event.SessionStatusData{Status: string(session.StatusIdle)})
	ev, err := e.store.Append(ctx, ses.ID, event.SessionStatus, data)
	if err != nil {
		return session.Session{}, err
	}
	if git.Repo {
		data, _ = json.Marshal(git)
		if ev, err = e.store.Append(ctx, ses.ID, event.GitState, data); err != nil {
			return session.Session{}, err
		}
	}
	ses.LastSeq, ses.UpdatedAt = ev.Seq, ev.TS
	return ses, nil
}

// AppendEventRequest is the input of AppendEvent.
type AppendEventRequest struct {
	SessionID string          `json:"sessionId"`
	Type      event.Type      `json:"type"`
	Data      json.RawMessage `json:"data"`
}

// AppendEvent persists an event to a session's transcript and then publishes
// it to subscribers. Harnesses are the normal producers; this is for tools
// and tests.
func (e *Engine) AppendEvent(ctx context.Context, req AppendEventRequest) (event.Event, error) {
	if req.SessionID == "" {
		return event.Event{}, errors.New("engine: sessionId is required")
	}
	ev, err := e.store.Append(ctx, req.SessionID, req.Type, req.Data)
	if err != nil {
		return event.Event{}, fmt.Errorf("engine: append to %s: %w", req.SessionID, err)
	}
	return ev, nil
}

// Events returns a session's events with seq greater than afterSeq. The GUI
// uses it for the initial load and to fill a gap in the live stream.
func (e *Engine) Events(ctx context.Context, sessionID string, afterSeq int64) ([]event.Event, error) {
	return e.store.Events(ctx, sessionID, afterSeq)
}

// Subscribe returns a channel of every event persisted from now on, in seq
// order per session, and a function that ends the subscription. A subscriber
// that falls behind loses events rather than slowing the engine; it notices
// the seq gap and catches up with Events.
func (e *Engine) Subscribe() (<-chan event.Event, func()) {
	return e.bus.subscribe(4096)
}
