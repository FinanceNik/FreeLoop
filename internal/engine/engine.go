// Package engine is the transport-agnostic API the GUI and the JSON-RPC
// socket are thin adapters over. Requests and responses are plain
// types; live events arrive through Subscribe.
package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/mcp"
	"github.com/FinanceNik/Freeloop/internal/netguard"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/session"
	"github.com/FinanceNik/Freeloop/internal/store"
)

// ErrNotFound is returned for an unknown session.
var ErrNotFound = store.ErrNotFound

// Options configures Open.
type Options struct {
	// SSHConfig replaces ~/.ssh/config, for tests.
	SSHConfig string
	DataDir   string
	Secrets   secrets.Store
	// HookProgram is this program, for an external agent's hook to call back with
	// (HRN-019); without it agents cannot use a token optimizer.
	HookProgram string
	// Confiner enforces the native sandbox (ENV-009). Without one, a sandboxed
	// environment runs nothing.
	Confiner environment.Confiner
	// EditorArgv is the command that opens a file for editing, never running it.
	EditorArgv func(path string) []string
	// PDFArgv is the command that shows a PDF, never running it.
	PDFArgv func(path string) []string
	// FolderArgv is the command that shows a folder, never running what is in it.
	FolderArgv func(path string) []string
	// ProcessStarted says when a process started, so the commands, agents and
	// terminals a crash left running are stopped at the next start and nothing
	// else is (NFR-REL-005). Without it, none are recorded or stopped.
	ProcessStarted func(pid int) (time.Time, bool)
	// GroupMembers lists a process group's processes, so a group whose first process
	// ended is recognised and stopped too.
	GroupMembers func(pgid int) []int
	// SystemInfo describes this computer for a problem report (macOS version, model,
	// chip, memory). Without it the report says the system is unknown.
	SystemInfo func() string
	// Home is the user's home directory, for the protected-path policy.
	// Default os.UserHomeDir().
	Home string
	// Dev offers the scripted demo provider.
	Dev bool
	// WatchInterval is how often the config files are checked for hand
	// edits (SET-001). Zero means one second; negative turns watching off.
	WatchInterval time.Duration
	// ScheduleInterval is how often due schedules are looked for
	// (SES-012). Zero means thirty seconds; negative turns them off.
	ScheduleInterval time.Duration
	// OnConfigChange is called after files changed on disk were reloaded.
	OnConfigChange func(ConfigChange)
	// OnNotice receives notices (GUI-008) from a goroutine of its own.
	OnNotice func(Notice)
	// ReleaseAPI replaces GitHub's latest-release address for the update
	// check, in development builds only (OQ-015).
	ReleaseAPI string
	// GH replaces the GitHub CLI with a stand-in, in development builds
	// only, for trying pull requests, checks and issues without GitHub.
	GH string
}

// ConfigChange lists the config files that were reloaded, relative to the
// data directory.
type ConfigChange struct {
	Files []string `json:"files"`
}

// Engine is safe for concurrent use.
type Engine struct {
	// stuck follows each session for HRN-020.
	stuck stuckWatch
	// queueMu guards queue.json (SES-025); onQueue tells the interface it changed.
	queueMu sync.Mutex
	onQueue func()
	// onNotice sends a notice (GUI-008); budgets alerted holds which budget periods
	// have warned at which level, so each says so once (PRV-017).
	// contextSeen is what was last recorded of each session's kinds of model call
	// (NFR-PRIV-003), so only a change is written.
	contextMu     sync.Mutex
	contextSeen   map[string][32]byte
	onNotice      func(Notice)
	budgetsMu     sync.Mutex
	budgetsWarned map[string]string
	// importMu lets one session import at a time (SES-020).
	importMu sync.Mutex
	// hookProgram is what an agent's hook runs to reach the app (HRN-019).
	hookProgram string
	sshConfig   string
	// storageFailing is set while events cannot be saved, for the sessions in
	// storageStopped.
	storageFailing atomic.Bool
	storageStopped map[string]bool
	// optimizers are the token optimizers sessions can choose (HRN-019).
	optimizers []config.Optimizer
	workflows  []config.Workflow
	// afkMu guards afkruns.json, afkStep keeps one pass over the AFK runs at a time,
	// and afkKick asks the schedule loop for one now (WFL-005).
	afkMu    sync.Mutex
	afkStep  sync.Mutex
	afkKick  chan struct{}
	dataDir  string
	home     string
	confiner environment.Confiner
	dev      bool
	// gh is the GitHub CLI pull requests are opened with (SES-011); tests
	// point it at a fake.
	gh      string
	store   *store.Store
	bus     *bus
	secrets secrets.Store
	broker  *permission.Broker
	rules   *ruleStore
	cfg     config.Config
	colors  config.Colors
	// loginEnv is what shell startup files set, for commands in a sandbox.
	loginOnce sync.Once
	loginEnv  map[string]string
	// loginGitConfig is the user's name and email as a git config file's text.
	loginGitConfig string
	// editorArgv is the command that opens a file for editing.
	editorArgv func(path string) []string
	systemInfo func() string
	// pdfArgv is the command that shows a PDF.
	pdfArgv    func(path string) []string
	folderArgv func(path string) []string
	// modelLists remembers each provider's models for the session's model menu.
	modelLists map[string]modelList
	// keyAddresses holds, by provider, the address its key is for.
	keyAddresses map[string]string
	// approvalSecret signs MCP approvals; read from the Keychain when first needed.
	approvalOnce   sync.Mutex
	approvalSecret []byte
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
	// notifying is the notice goroutine, which ends when the bus closes.
	notifying sync.WaitGroup

	mu sync.Mutex
	// projects caches trusted projects' .freeloop/ settings, skillCache
	// the skills found per project.
	projects   projectCache
	skillCache skillCache
	commands   commandCache

	envs     []config.Environment
	profiles []config.Profile
	// mcpServers are mcp.yaml's servers; mcp runs them.
	mcpServers []config.MCPServer
	presets    []config.HarnessPreset
	mcp        *mcp.Manager
	providers  *providerRegistry
	runtimes   map[string]*runtime
	// terms are the open integrated terminals (GUI-012).
	terms terminals
	// openURL shows a page in the user's browser, for MCP sign-ins;
	// signIns are the sign-in pages being waited on, by server (MCP-006).
	openURL func(string) error
	signIns map[string]string
	// updates runs one background update check at a time; updateErr is
	// the last one's failure; releaseAPI replaces GitHub in tests.
	updates    onceReset
	updateErr  string
	releaseAPI string
	// schedules guards schedules.json and the runs it starts (SES-012);
	// stopSchedules ends the loop, and Close waits on scheduling.
	schedules     sync.Mutex
	stopSchedules chan struct{}
	scheduling    sync.WaitGroup
	// scheduleCtx is the context runs start under; Close cancels it, so a
	// run that is still starting does not hold up quitting.
	scheduleCtx    context.Context
	cancelSchedule context.CancelFunc
	// closing is set under mu when Close begins: no runtime starts after.
	closing bool
	// setups are worktree setups of this run (SES-016).
	setups setups
	// starting are runtimes being set up, so hooks find them while the
	// harness starts (an external agent's requests during session/load).
	starting map[string]*runtime
	// startLocks holds a *sync.Mutex per session that serialises starts.
	startLocks sync.Map
	streams    map[string]*streamBuf
	known      []string // secret values to redact
	knownForms [][]byte // and other spellings of them
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
	// Local-only mode applies to every connection the app makes from here
	// on (NFR-OFF-003).
	netguard.Install()
	netguard.SetLocalOnly(cfg.Network.LocalOnly)
	if opts.Home == "" {
		if opts.Home, err = os.UserHomeDir(); err != nil {
			return nil, err
		}
	}
	e := &Engine{
		gh:            "gh",
		hookProgram:   opts.HookProgram,
		sshConfig:     opts.SSHConfig,
		dataDir:       opts.DataDir,
		home:          opts.Home,
		dev:           opts.Dev,
		bus:           newBus(),
		secrets:       opts.Secrets,
		confiner:      opts.Confiner,
		editorArgv:    opts.EditorArgv,
		systemInfo:    opts.SystemInfo,
		pdfArgv:       opts.PDFArgv,
		folderArgv:    opts.FolderArgv,
		modelLists:    map[string]modelList{},
		cfg:           cfg,
		issues:        map[string][]config.Issue{issueConfig: issues},
		onChange:      opts.OnConfigChange,
		quit:          make(chan struct{}),
		stopSchedules: make(chan struct{}),
		afkKick:       make(chan struct{}, 1),
		runtimes:      map[string]*runtime{},
		starting:      map[string]*runtime{},
		streams:       map[string]*streamBuf{},
	}
	if opts.Dev {
		e.releaseAPI = opts.ReleaseAPI
		if opts.GH != "" {
			e.gh = opts.GH
		}
	}
	e.store, err = store.Open(store.Options{Dir: opts.DataDir, OnAppend: func(ev event.Event) { e.bus.publish(ev); e.observe(ev) }, Redact: e.redact})
	if err != nil {
		return nil, err
	}
	// What the last run left running, if it ended without stopping it, goes before
	// anything of this run starts (NFR-REL-005). The caller holds the data folder's
	// lock, so these are this folder's own.
	if opts.ProcessStarted != nil {
		environment.ProcessStarted, environment.GroupMembers = opts.ProcessStarted, opts.GroupMembers
		ledger := filepath.Join(opts.DataDir, "running.json")
		if stopped, err := environment.ReapLeftovers(ledger); err != nil {
			log.Printf("engine: what the last run left running: %v", err)
		} else if len(stopped) > 0 {
			log.Printf("engine: stopped %d left running by the last run: %s", len(stopped), strings.Join(stopped, ", "))
		}
		environment.UseLedger(ledger)
	}
	e.broker = permission.NewBroker(permission.NewPolicy(opts.Home, opts.DataDir), e.emit)
	e.rules = newRuleStore(opts.DataDir, func(issues []config.Issue) { e.setIssues(issueRules, issues) })
	e.rules.scopedRules = e.scopedRules
	e.rules.approved, e.rules.approve = e.ruleApproved, e.approveRule
	e.broker.SetRules(e.rules, e.rules)
	e.broker.SetHooks(engineHooks{e})
	e.loadColors()
	e.loadEnvironments()
	e.loadPresets()
	e.loadOptimizers()
	e.loadWorkflows()
	if why := e.store.Rebuilt(); why != "" {
		e.issues[issueRebuilt] = []config.Issue{{File: "freeloop.db", Message: why + " Sessions without their settings can be read but not continued, and projects need trusting again."}}
	}
	e.loadProfiles()
	// MCP servers run in a local environment at the user's home: they are
	// the app's, not one project's.
	if env, err := e.localEnv(e.home, nil); err == nil {
		e.mcp = mcp.New(env)
	} else {
		e.mcp = mcp.New(nil)
	}
	e.loadMCP()
	if err := e.loadProviders(); err != nil {
		e.store.Close()
		return nil, err
	}
	e.adoptExistingCommands()
	e.rememberStoredSecrets()
	e.recover(context.Background())
	e.onNotice = opts.OnNotice
	if opts.OnNotice != nil {
		events, _ := e.bus.subscribe(1024)
		e.notifying.Add(1)
		go e.notify(events, opts.OnNotice)
	}
	if opts.WatchInterval >= 0 {
		interval := opts.WatchInterval
		if interval == 0 {
			interval = time.Second
		}
		e.watching.Add(1)
		go e.watch(interval)
	}
	e.scheduleCtx, e.cancelSchedule = context.WithCancel(context.Background())
	if opts.ScheduleInterval >= 0 {
		interval := opts.ScheduleInterval
		if interval == 0 {
			interval = 30 * time.Second
		}
		e.scheduling.Add(1)
		go e.scheduleLoop(interval)
	}
	e.dropBypass(context.Background())
	return e, nil
}

// SetOpenURL sets how the engine shows a page in the user's browser.
func (e *Engine) SetOpenURL(f func(string) error) { e.openURL = f }

// Close cancels running turns, closes every subscription and the store.
func (e *Engine) Close() error {
	e.mu.Lock()
	e.closing = true
	e.mu.Unlock()
	e.cancelSchedule()
	close(e.stopSchedules)
	e.scheduling.Wait()
	e.mu.Lock()
	running := make([]*runtime, 0, len(e.runtimes))
	for _, r := range e.runtimes {
		running = append(running, r)
	}
	e.mu.Unlock()
	for _, r := range running {
		e.stopTurn(r.sid, "Stopped because Freeloop quit.")
	}
	// Stopped outside e.mu: a finishing turn still records events.
	for _, r := range running {
		r.stop()
	}
	e.closeTerminals("")
	e.terms.pumps.Wait()
	e.mcp.StopAll()
	close(e.quit)
	e.watching.Wait()
	e.bg.Wait()
	// An MCP server a background start registered after the first stop.
	e.mcp.StopAll()
	e.bus.close()
	e.notifying.Wait()
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
	marks, err := e.writtenMarks(settingsItems(c))
	if err != nil {
		return config.Config{}, err
	}
	return e.saveSettings(c, marks)
}

// saveSettings writes config.yaml with marks added to the approvals in force. The
// form's copy of those is ignored, since it can be older than an approval.
func (e *Engine) saveSettings(c config.Config, marks []string) (config.Config, error) {
	e.mu.Lock()
	broken := config.Unreadable(e.issues[issueConfig])
	e.mu.Unlock()
	if broken {
		return config.Config{}, fmt.Errorf("%s cannot be read, so it is not overwritten; fix or remove it first", config.FileName)
	}
	c.Version = 1
	// What is being typed in a mark's letters may not be letters yet.
	c.UI.SourceMarks, _ = config.CleanSourceMarks(c.UI.SourceMarks)
	e.mu.Lock()
	sameSearch := e.cfg.Web.SearchURL == c.Web.SearchURL
	c.CommandApprovals = slices.Clone(e.cfg.CommandApprovals)
	e.mu.Unlock()
	for _, m := range marks {
		if !slices.Contains(c.CommandApprovals, m) {
			c.CommandApprovals = append(c.CommandApprovals, m)
		}
	}
	// Off again, so a file turning one back on has to ask.
	for fingerprint, on := range map[string]bool{config.BypassFingerprint(): c.Permissions.AllowBypass, config.UpdatesFingerprint(): c.Updates.Check} {
		if mark, err := e.approvalMark(fingerprint); err == nil && !on {
			c.CommandApprovals = slices.DeleteFunc(c.CommandApprovals, func(m string) bool { return m == mark })
		}
	}
	if !sameSearch {
		if err := checkAddress("the search address", strings.ReplaceAll(c.Web.SearchURL, "{query}", "q")); err != nil {
			return config.Config{}, err
		}
	}
	if err := config.Save(e.dataDir, c); err != nil {
		return config.Config{}, err
	}
	e.mu.Lock()
	moved := originOf(e.cfg.Web.SearchURL) != originOf(c.Web.SearchURL)
	e.cfg = c
	e.mu.Unlock()
	e.dropBypass(context.Background())
	// Changed here, so the user means it; changed in the file, the key stays put.
	if moved && c.Web.SearchURL != "" {
		if err := e.rebindSearchKey(c.Web.SearchURL); err != nil {
			return config.Config{}, err
		}
	}
	netguard.SetLocalOnly(c.Network.LocalOnly)
	return c, nil
}

// BlockedConnections lists the connections local-only mode refused, newest
// last (NFR-OFF-003).
func (e *Engine) BlockedConnections() []netguard.Blocked {
	return netguard.Attempts()
}

// Redact removes what the app knows to be secret from a line for its log.
func (e *Engine) Redact(s string) string { return e.redactText(s) }

// ConfigIssues returns the problems found while loading configuration.
func (e *Engine) ConfigIssues() []config.Issue {
	e.mu.Lock()
	out := []config.Issue{}
	for _, k := range issueOrder {
		out = append(out, e.issues[k]...)
	}
	e.mu.Unlock()
	return append(out, e.pendingIssues()...)
}

// Issue groups, one per kind of config file.
const (
	issueConfig       = "config"
	issueProviders    = "providers"
	issueEnvironments = "environments"
	issueRules        = "rules"
	issueCapabilities = "capabilities"
	issueProfiles     = "profiles"
	issueMCP          = "mcp"
	issuePresets      = "presets"
	issueOptimizers   = "optimizers"
	issueWorkflows    = "workflows"
	issueStorage      = "storage"
	issueRebuilt      = "rebuilt"
	issueColors       = "colors"
	issueAddresses    = "addresses"
)

var issueOrder = []string{issueStorage, issueRebuilt, issueConfig, issueProviders, issueEnvironments, issuePresets, issueOptimizers, issueWorkflows, issueProfiles, issueMCP, issueRules, issueCapabilities, issueColors, issueAddresses}

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
		if errors.Is(err, store.ErrWrite) {
			e.storageFailed(sid, err)
		}
		return
	}
	if e.storageFailing.Load() {
		e.storageRecovered(sid)
	}
}

// storageFailed stops a session whose events cannot be saved, and says why. A turn
// that went on would change files and cost money with no record of it, and a lost
// event cannot be got back. The transcript's own events cannot carry this, so it
// goes where the app shows problems with its files.
func (e *Engine) storageFailed(sid string, err error) {
	e.mu.Lock()
	if e.storageStopped == nil {
		e.storageStopped = map[string]bool{}
	}
	first := !e.storageStopped[sid]
	e.storageStopped[sid] = true
	e.issues[issueStorage] = []config.Issue{{File: "sessions", Message: "Freeloop could not save to its data folder, so running turns were stopped: " + err.Error() + ". Free up disk space, then send the message again."}}
	e.mu.Unlock()
	e.storageFailing.Store(true)
	if first {
		e.bg.Go(func() {
			e.Cancel(sid)
			if e.onChange != nil {
				e.onChange(ConfigChange{})
			}
		})
	}
}

// storageRecovered clears the problem once this session's events save again.
func (e *Engine) storageRecovered(sid string) {
	e.mu.Lock()
	delete(e.storageStopped, sid)
	done := len(e.storageStopped) == 0
	if done {
		delete(e.issues, issueStorage)
	}
	e.mu.Unlock()
	if done {
		e.storageFailing.Store(false)
		e.bg.Go(func() {
			if e.onChange != nil {
				e.onChange(ConfigChange{})
			}
		})
	}
}

// rememberStoredSecrets adds every secret the app stores (provider keys,
// environment secrets) to the redaction list, so a key pasted into any
// session is redacted even before its provider or environment is used.
func (e *Engine) rememberStoredSecrets() {
	e.mu.Lock()
	// Only the items the app would use for these: a file naming another would
	// otherwise raise a Keychain prompt for it, in the app's name.
	var refs []secrets.Ref
	add := func(raw string, want secrets.Ref) {
		if ref, err := ownRef(raw, want); err == nil {
			refs = append(refs, ref)
		}
	}
	for _, c := range e.providers.configs {
		if c.APIKey != "" {
			add(c.APIKey, providerRef(c.ID))
		}
	}
	for _, env := range e.envs {
		for name, ref := range env.Secrets {
			add(ref, secretRef(env.ID, name))
		}
	}
	var oauth []string
	for _, s := range e.mcpServers {
		for name, ref := range s.Secrets {
			add(ref, mcpSecretRef(s.ID, name))
		}
		oauth = append(oauth, s.ID)
	}
	e.mu.Unlock()
	refs = append(refs, webSearchKey)
	for _, ref := range refs {
		if v, err := e.secrets.Get(ref); err == nil {
			_, key, _ := unbindSecret(v)
			e.remember(key)
		}
	}
	for _, id := range oauth {
		if v, err := e.secrets.Get(oauthRef(id)); err == nil {
			for _, secret := range oauthSecrets(v) {
				e.remember(secret)
			}
		}
	}
	e.learnKeyAddresses()
}

// learnKeyAddresses reads which address each provider's key is for, so a change
// made while the app was closed shows at once.
func (e *Engine) learnKeyAddresses() {
	e.mu.Lock()
	cfgs := slices.Clone(e.providers.configs)
	e.mu.Unlock()
	for _, c := range cfgs {
		if c.APIKey == "" {
			continue
		}
		if ref, err := ownRef(c.APIKey, providerRef(c.ID)); err == nil {
			if v, err := e.secrets.Get(ref); err == nil {
				if address, _, bound := unbindSecret(v); bound {
					e.noteKeyAddress(c.ID, address)
				}
			}
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
	e.knownForms = append(e.knownForms, encodedForms(secret)...)
}

// encodedForms are other spellings of a secret a command could print: curl -v
// shows a key base64-encoded in a Basic header, and URLs escape it. Base64 of a
// value inside something longer depends on where it starts, so the part that
// does not depend on its neighbours is taken at each of the three alignments.
func encodedForms(secret string) [][]byte {
	var out [][]byte
	add := func(s string) {
		if s != secret && len(s) >= 8 && !slices.ContainsFunc(out, func(b []byte) bool { return string(b) == s }) {
			out = append(out, []byte(s))
		}
	}
	add(url.QueryEscape(secret))
	add(url.PathEscape(secret))
	add(hex.EncodeToString([]byte(secret)))
	add(strings.ToUpper(hex.EncodeToString([]byte(secret))))
	for pad := range 3 {
		for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
			b := enc.EncodeToString(append(make([]byte, pad), secret...))
			// Characters touched by what comes before or after are not the secret's.
			start := (pad*8 + 5) / 6
			end := len(b) - 1
			if end-start >= 12 {
				add(b[start:end])
			}
		}
	}
	return out
}

// redact replaces known secret values in event data, in raw and in
// JSON-escaped form.
func (e *Engine) redact(data json.RawMessage) json.RawMessage {
	e.mu.Lock()
	known, forms := e.known, e.knownForms
	e.mu.Unlock()
	for _, k := range known {
		esc, _ := json.Marshal(k)
		for _, form := range [][]byte{[]byte(k), esc[1 : len(esc)-1]} {
			if bytes.Contains(data, form) {
				data = bytes.ReplaceAll(data, form, []byte("[REDACTED]"))
			}
		}
		data = redactCut(data, []byte(k))
	}
	for _, form := range forms {
		if bytes.Contains(data, form) {
			data = bytes.ReplaceAll(data, form, []byte("[REDACTED]"))
		}
	}
	return data
}

// redactText is redact for a string, for what goes to the log.
func (e *Engine) redactText(s string) string {
	return string(e.redact([]byte(s)))
}

// minCutSecret is the shortest piece of a secret recognized as one: long
// enough that it does not match a key format's common prefix alone.
const minCutSecret = 16

// redactCut replaces what is left of a secret some text limit cut in two, a
// title's or an output's: its start running up to a cut, its end running on
// from one. Those no longer match the whole value.
func redactCut(data, secret []byte) []byte {
	if len(secret) <= minCutSecret {
		return data
	}
	mark := []byte("[REDACTED]")
	for from := 0; ; {
		i := bytes.Index(data[from:], secret[:minCutSecret])
		if i < 0 {
			break
		}
		i += from
		n := minCutSecret
		for n < len(secret) && i+n < len(data) && data[i+n] == secret[n] {
			n++
		}
		data = slices.Concat(data[:i], mark, data[i+n:])
		from = i + len(mark)
	}
	end := secret[len(secret)-minCutSecret:]
	for from := 0; ; {
		i := bytes.Index(data[from:], end)
		if i < 0 {
			break
		}
		i += from
		start, n := i, len(secret)-minCutSecret
		for n > 0 && start > 0 && data[start-1] == secret[n-1] {
			start--
			n--
		}
		data = slices.Concat(data[:start], mark, data[i+minCutSecret:])
		from = start + len(mark)
	}
	return data
}

// ListSessions returns all sessions, most recently updated first.
func (e *Engine) ListSessions(ctx context.Context) ([]session.Session, error) {
	return e.store.ListSessions(ctx)
}

// GetSession returns one session.
func (e *Engine) GetSession(ctx context.Context, sid string) (session.Session, error) {
	return e.store.GetSession(ctx, sid)
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
	// Worktree gives the session its own git worktree and branch (SES-010).
	Worktree bool `json:"worktree"`
	// shareWorktree puts the session in an existing session's worktree,
	// for a profile switch (HRN-012) or a subagent (HRN-014).
	shareWorktree *session.Session
	// parent makes the session a subagent of another (HRN-014).
	parent string
	// Optimizer is the token optimizer to use, or off for none; empty takes
	// the profile's, then the global default (HRN-019). It is run once to see
	// that it works. optimizer is a parent's or source's choice, carried over
	// without that.
	Optimizer string `json:"optimizer"`
	optimizer string
	// Workflow is the session's workflow, or off for none; empty takes the
	// profile's, then the default (WFL-002).
	Workflow string `json:"workflow"`
}

// CreateSession creates an idle session and records its initial status as
// the first transcript event. Bypass mode is never a new session's mode
// (SEC-001).
func (e *Engine) CreateSession(ctx context.Context, req CreateSessionRequest) (session.Session, error) {
	// A title goes to the database without passing through an event.
	title := string(e.redact(json.RawMessage(strings.TrimSpace(req.Title))))
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
	envCfg, err := e.usableEnvironment(envID)
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
	prof, err := e.profileFor(project, profileID)
	if err != nil {
		return session.Session{}, err
	}
	mode := permission.Mode(req.Mode)
	if mode == "" {
		mode = permission.Mode(e.startMode(prof))
	}
	if mode == "" {
		mode = permission.Mode(envCfg.DefaultMode)
	}
	if !mode.Valid() || mode.Unattended() {
		mode = permission.ModeAsk
	}
	harnessID, model, planEffort := builtin.ID, "", ""
	if prof.Harness != "" && prof.Harness != builtin.ID {
		// An external agent brings its own model access (HRN-005).
		if _, err := e.runnablePreset(prof.Harness); err != nil {
			return session.Session{}, err
		}
		harnessID, req.ProviderID = prof.Harness, ""
		// The model it runs on, as the profile names it, prices the turns of an
		// agent that does not say (PRV-008).
		model = strings.TrimSpace(req.Model)
		if model == "" {
			model = strings.TrimSpace(prof.Model)
		}
	} else {
		// A workflow chosen for the session starts it on its model set's planning model,
		// unless a model was asked for (WFL-014).
		planning, planned := e.startingPlanning(req)
		if planned && req.ProviderID == "" && strings.TrimSpace(req.Model) == "" {
			req.ProviderID, req.Model = planning.Provider, planning.Model
		}
		if req.ProviderID == "" {
			req.ProviderID = prof.Provider
		}
		if req.ProviderID == "" {
			return session.Session{}, errors.New("choose a provider")
		}
		p, err := e.provider(req.ProviderID)
		if err != nil {
			return session.Session{}, err
		}
		model = strings.TrimSpace(req.Model)
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
		// On the planning model, as the new-session form fills it in, at its effort.
		if planned && planning.Effort != "" && req.ProviderID == planning.Provider && model == planning.Model {
			if caps, err := e.ModelCapabilities(ctx, req.ProviderID, model, false); err == nil && caps.Effort {
				planEffort = planning.Effort
			}
		}
	}
	optimizer, err := e.chooseOptimizer(ctx, req, prof, harnessID, envCfg)
	if err != nil {
		return session.Session{}, err
	}
	// The base commit is recorded now so the session's changes stay
	// visible after unrelated commits (SES-006).
	root, worktree, worktreeBranch := project.Root, "", ""
	if s := req.shareWorktree; s != nil && s.Worktree != "" {
		root, worktree, worktreeBranch = s.Worktree, s.Worktree, s.OwnBranch()
	} else if req.Worktree {
		path, branch, err := e.addWorktree(ctx, project, title, "")
		if err != nil {
			return session.Session{}, err
		}
		root, worktree, worktreeBranch = path, path, branch
	}
	// A session with its own worktree gets its own ports (SES-016).
	portBase := 0
	if worktree != "" && req.shareWorktree == nil {
		portBase = e.allocatePorts(ctx)
	}
	var git event.GitStateData
	if env, err := e.localEnv(root, nil); err == nil {
		git = gitState(ctx, env, project.Trusted)
	}
	git.BaseCommit = git.Head
	ses, err := e.store.CreateSession(ctx, session.Session{
		Optimizer: optimizer, Workflow: e.chooseWorkflow(req, prof), Title: title, HarnessID: harnessID, ProjectID: project.ID, ProjectRoot: project.Root, EnvironmentID: envID,
		BaseCommit: git.BaseCommit, Branch: git.Branch, ProviderID: req.ProviderID, Model: model, Mode: string(mode), ProfileID: prof.ID, Effort: planEffort,
		Worktree: worktree, WorktreeBranch: worktreeBranch, ParentID: req.parent, PortBase: portBase,
	})
	if err != nil {
		if worktree != "" {
			e.removeWorktree(ctx, session.Session{ProjectID: project.ID, Worktree: worktree})
		}
		return session.Session{}, err
	}
	if worktree != "" && req.shareWorktree == nil {
		e.startSetup(ses)
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
	if ses.Workflow != "" {
		w, _ := e.findWorkflow(ses.Workflow)
		data, _ = json.Marshal(event.WorkflowChangedData{Workflow: w.ID, Name: w.Name})
		if ev, err = e.store.Append(ctx, ses.ID, event.WorkflowChanged, data); err != nil {
			return session.Session{}, err
		}
	}
	if planEffort != "" {
		data, _ = json.Marshal(event.EffortChangedData{Effort: planEffort})
		if ev, err = e.store.Append(ctx, ses.ID, event.EffortChanged, data); err != nil {
			return session.Session{}, err
		}
	}
	// The profile's slash commands, as an ACP agent advertises its own
	// (GUI-004).
	if _, cmds := commandsOf(prof); len(cmds.AvailableCommands) > 0 {
		data, _ = json.Marshal(cmds)
		if ev, err = e.store.Append(ctx, ses.ID, event.AvailableCommandsUpdate, data); err != nil {
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
	start := time.Now()
	evs, err := e.store.Events(ctx, sessionID, afterSeq)
	// Opening a session once took over 30 s on CI and never here; a slow read says so
	// in the log, and a stall without one is in the page or the connection.
	if took := time.Since(start); took > time.Second {
		log.Printf("engine: reading %s's events after %d took %v (%d events)", sessionID, afterSeq, took.Round(time.Millisecond), len(evs))
	}
	return evs, err
}

// Subscribe returns a channel of every event persisted from now on, in seq
// order per session, and a function that ends the subscription. A subscriber
// that falls behind loses events rather than slowing the engine; it notices
// the seq gap and catches up with Events.
func (e *Engine) Subscribe() (<-chan event.Event, func()) {
	return e.bus.subscribe(4096)
}
