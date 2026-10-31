package permission

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
)

type recorder struct {
	mu     sync.Mutex
	events []event.Type
}

func (r *recorder) emit(_ string, typ event.Type, _ any) {
	r.mu.Lock()
	r.events = append(r.events, typ)
	r.mu.Unlock()
}

func setup(t *testing.T) (*Broker, *recorder, string, string) {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	rec := &recorder{}
	return NewBroker(NewPolicy(home, filepath.Join(home, "Library", "Application Support", "freeloop")), rec.emit), rec, home, root
}

// decide runs Request and, if it waits for the user, answers with option.
func decide(t *testing.T, b *Broker, pc Context, req Request, option string) Decision {
	t.Helper()
	got := make(chan Decision, 1)
	go func() { got <- b.Request(context.Background(), pc, req) }()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case d := <-got:
			return d
		case <-deadline:
			t.Fatal("request neither settled nor queued")
		default:
		}
		if p := b.Pending(); len(p) == 1 {
			if option == "" {
				t.Fatalf("request %+v was queued, expected it settled", req)
			}
			if err := b.Decide(p[0].ID, option, "", ""); err != nil {
				t.Fatal(err)
			}
			return <-got
		}
		time.Sleep(time.Millisecond)
	}
}

func TestModeTable(t *testing.T) {
	b, _, _, root := setup(t)
	write := Request{SessionID: "s", Tool: "write_file", Kind: KindEdit, Paths: []string{filepath.Join(root, "a.go")}}
	shell := Request{SessionID: "s", Tool: "shell", Kind: KindExecute, Command: "make test"}
	read := Request{SessionID: "s", Tool: "read_file", Kind: KindRead, Paths: []string{filepath.Join(root, "a.go")}}

	for _, c := range []struct {
		mode   Mode
		req    Request
		answer string // "" means it must settle without asking
		want   Outcome
		actor  Actor
	}{
		{ModeAsk, read, "", Allowed, ActorPolicy},
		{ModeAsk, write, "allow_once", Allowed, ActorUser},
		{ModeAsk, shell, "reject_once", Denied, ActorUser},
		{ModeAcceptEdits, write, "", Allowed, ActorMode},
		{ModeAcceptEdits, shell, "allow_once", Allowed, ActorUser},
		{ModePlan, write, "", Denied, ActorMode},
		{ModePlan, shell, "", Denied, ActorMode},
		{ModePlan, read, "", Allowed, ActorPolicy},
		{ModeBypass, write, "", Allowed, ActorMode},
		{ModeBypass, shell, "", Allowed, ActorMode},
	} {
		d := decide(t, b, Context{Mode: c.mode, Root: root}, c.req, c.answer)
		if d.Outcome != c.want || d.Actor != c.actor {
			t.Errorf("%s %s: got %s by %s, want %s by %s", c.mode, c.req.Tool, d.Outcome, d.Actor, c.want, c.actor)
		}
	}
}

func TestProtectedPathsHoldInBypass(t *testing.T) {
	b, _, home, root := setup(t)
	pc := Context{Mode: ModeBypass, Root: root}
	for _, req := range []Request{
		{Tool: "write_file", Kind: KindEdit, Paths: []string{filepath.Join(home, ".ssh", "config")}},
		{Tool: "read_file", Kind: KindRead, Paths: []string{filepath.Join(home, ".ssh", "id_ed25519")}},
		{Tool: "write_file", Kind: KindEdit, Paths: []string{filepath.Join(root, ".git", "config")}},
		// A project's .freeloop/ can add hooks, which run commands.
		{Tool: "write_file", Kind: KindEdit, Paths: []string{filepath.Join(root, ".freeloop", "hooks.yaml")}},
		{Tool: "edit_file", Kind: KindEdit, Paths: []string{filepath.Join(root, ".freeloop", "profiles", "p.yaml")}},
		{Tool: "write_file", Kind: KindEdit, Paths: []string{filepath.Join(home, "elsewhere.txt")}},
		{Tool: "read_file", Kind: KindRead, Paths: []string{filepath.Join(home, "Library", "Application Support", "freeloop", "config.yaml")}},
	} {
		d := decide(t, b, pc, req, "")
		if d.Outcome != Denied || d.Actor != ActorProtectedPath {
			t.Errorf("%v: got %s by %s", req.Paths, d.Outcome, d.Actor)
		}
	}
}

func TestReadOutsideRootAsksExceptInBypass(t *testing.T) {
	b, _, home, root := setup(t)
	req := Request{Tool: "read_file", Kind: KindRead, Paths: []string{filepath.Join(home, ".zshrc")}}
	if d := decide(t, b, Context{Mode: ModeAcceptEdits, Root: root}, req, "allow_once"); d.Actor != ActorUser {
		t.Errorf("accept-edits read outside root decided by %s", d.Actor)
	}
	if d := decide(t, b, Context{Mode: ModeBypass, Root: root}, req, ""); d.Outcome != Allowed {
		t.Errorf("bypass read outside root: %s", d.Outcome)
	}
}

func TestPlanApprovalAsks(t *testing.T) {
	b, _, _, root := setup(t)
	req := Request{Tool: "present_plan", Kind: KindSwitchMode, Options: []Option{
		{ID: "approve_ask", Name: "Approve", Kind: "allow_once"},
		{ID: "keep_planning", Name: "Keep planning", Kind: "reject_once"},
	}}
	d := decide(t, b, Context{Mode: ModePlan, Root: root}, req, "approve_ask")
	if d.Outcome != Allowed || d.OptionID != "approve_ask" {
		t.Errorf("got %+v", d)
	}
}

func TestCancelResolvesPendingAsCancelled(t *testing.T) {
	b, rec, _, root := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan Decision, 1)
	go func() {
		got <- b.Request(ctx, Context{Mode: ModeAsk, Root: root}, Request{SessionID: "s", Tool: "shell", Kind: KindExecute})
	}()
	for len(b.Pending()) == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if d := <-got; d.Outcome != Cancelled {
		t.Errorf("outcome = %s", d.Outcome)
	}
	if len(b.Pending()) != 0 {
		t.Error("request still pending after cancel")
	}
	want := []event.Type{event.PermissionRequested, event.SessionStatus, event.SessionStatus, event.PermissionDecided}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.events) != len(want) {
		t.Fatalf("events = %v", rec.events)
	}
	for i := range want {
		if rec.events[i] != want[i] {
			t.Errorf("events = %v, want %v", rec.events, want)
		}
	}
}

func TestDecideErrors(t *testing.T) {
	b, _, _, _ := setup(t)
	if err := b.Decide("nope", "allow_once", "", ""); !errors.Is(err, ErrUnknownRequest) {
		t.Errorf("err = %v", err)
	}
}

func TestGuard(t *testing.T) {
	_, _, home, root := setup(t)
	g := NewPolicy(home, filepath.Join(home, "appdata")).Guard(root)
	if err := g.CheckRead(filepath.Join(home, ".aws", "credentials")); !errors.Is(err, environment.ErrProtected) {
		t.Errorf("CheckRead ~/.aws: %v", err)
	}
	if err := g.CheckRead(filepath.Join(home, ".zshrc")); err != nil {
		t.Errorf("CheckRead ~/.zshrc: %v (outside-root reads are the broker's call)", err)
	}
	if err := g.CheckWrite(filepath.Join(root, "x.go")); err != nil {
		t.Errorf("CheckWrite inside root: %v", err)
	}
	if err := g.CheckWrite(filepath.Join(root, ".git", "HEAD")); !errors.Is(err, environment.ErrProtected) {
		t.Errorf("CheckWrite .git: %v", err)
	}
	if err := g.CheckWrite(filepath.Join(root+"-sibling", "x")); !errors.Is(err, environment.ErrProtected) {
		t.Errorf("CheckWrite sibling dir with root as prefix: %v", err)
	}
}

// memRules is a RuleSource and RuleSink for tests.
type memRules struct {
	global  []Rule
	session map[string][]Rule
	err     error
}

func (m *memRules) Rules(sid string) ([]Rule, error) {
	return append(append([]Rule{}, m.global...), m.session[sid]...), m.err
}

func (m *memRules) AddRule(sid string, r Rule) error {
	if r.Scope == ScopeGlobal {
		m.global = append(m.global, r)
		return nil
	}
	if m.session == nil {
		m.session = map[string][]Rule{}
	}
	m.session[sid] = append(m.session[sid], r)
	return nil
}

func TestRulesInEvaluation(t *testing.T) {
	b, _, _, root := setup(t)
	rules := &memRules{}
	b.SetRules(rules, rules)
	gitStatus := Request{SessionID: "s", Tool: "shell", Kind: KindExecute, Command: "git status"}
	write := Request{SessionID: "s", Tool: "write_file", Kind: KindEdit, Paths: []string{filepath.Join(root, "a.go")}}

	// No rule: ask.
	if d := decide(t, b, Context{Mode: ModeAsk, Root: root}, gitStatus, "reject_once"); d.Actor != ActorUser {
		t.Errorf("without rules: %+v", d)
	}

	// Allow always (session scope) creates a rule; the next identical call
	// is approved by it.
	d := decide(t, b, Context{Mode: ModeAsk, Root: root}, gitStatus, "allow_always")
	if d.Outcome != Allowed || d.Persisted == nil || d.Persisted.Command != "git status" || d.Persisted.Scope != ScopeSession {
		t.Fatalf("allow always = %+v", d)
	}
	d = decide(t, b, Context{Mode: ModeAsk, Root: root}, gitStatus, "")
	if d.Outcome != Allowed || d.Actor != ActorRule || d.Rule == nil {
		t.Errorf("second call = %+v", d)
	}
	// The session rule does not leak into another session.
	other := gitStatus
	other.SessionID = "other"
	if d := decide(t, b, Context{Mode: ModeAsk, Root: root}, other, "reject_once"); d.Actor != ActorUser {
		t.Errorf("session rule applied to another session: %+v", d)
	}

	// Deny wins over allow, across scopes, and even in bypass mode.
	rules.global = append(rules.global, Rule{Effect: Deny, Tool: "shell", Command: "git *", Scope: ScopeGlobal})
	for _, mode := range []Mode{ModeAsk, ModeBypass} {
		if d := decide(t, b, Context{Mode: mode, Root: root}, gitStatus, ""); d.Outcome != Denied || d.Actor != ActorRule {
			t.Errorf("%s: deny rule lost to %+v", mode, d)
		}
	}

	// An allow rule cannot lift plan mode's read-only rule.
	rules.global = []Rule{{Effect: Allow, Tool: "write_file", Scope: ScopeGlobal}}
	if d := decide(t, b, Context{Mode: ModePlan, Root: root}, write, ""); d.Outcome != Denied || d.Actor != ActorMode {
		t.Errorf("plan mode with an allow rule = %+v", d)
	}
	if d := decide(t, b, Context{Mode: ModeAsk, Root: root}, write, ""); d.Outcome != Allowed || d.Actor != ActorRule {
		t.Errorf("ask mode with an allow rule = %+v", d)
	}
	// Nor protected paths.
	git := write
	git.Paths = []string{filepath.Join(root, ".git", "config")}
	if d := decide(t, b, Context{Mode: ModeAsk, Root: root}, git, ""); d.Actor != ActorProtectedPath {
		t.Errorf("allow rule opened a protected path: %+v", d)
	}
}

func TestAllowAlwaysGlobalScope(t *testing.T) {
	b, _, _, root := setup(t)
	rules := &memRules{}
	b.SetRules(rules, rules)
	got := make(chan Decision, 1)
	go func() {
		got <- b.Request(context.Background(), Context{Mode: ModeAsk, Root: root}, Request{SessionID: "s", Tool: "shell", Kind: KindExecute, Command: "make test"})
	}()
	for len(b.Pending()) == 0 {
		time.Sleep(time.Millisecond)
	}
	if err := b.Decide(b.Pending()[0].ID, "allow_always", "", "planet"); err == nil {
		t.Error("unknown scope accepted")
	}
	if err := b.Decide(b.Pending()[0].ID, "allow_always", "", ScopeGlobal); err != nil {
		t.Fatal(err)
	}
	if d := <-got; d.Persisted == nil || d.Persisted.Scope != ScopeGlobal || len(rules.global) != 1 {
		t.Errorf("decision = %+v, global rules = %+v", d, rules.global)
	}
}

// Review finding: protected paths were compared case-sensitively, but APFS
// opens .GIT as .git and ~/.SSH as ~/.ssh.
func TestProtectedPathsIgnoreCase(t *testing.T) {
	b, _, home, root := setup(t)
	for _, req := range []Request{
		{Tool: "write_file", Kind: KindEdit, Paths: []string{filepath.Join(root, ".GIT", "hooks", "pre-commit")}},
		{Tool: "write_file", Kind: KindEdit, Paths: []string{filepath.Join(root, ".Git", "config")}},
		{Tool: "read_file", Kind: KindRead, Paths: []string{filepath.Join(home, ".SSH", "id_ed25519")}},
		{Tool: "read_file", Kind: KindRead, Paths: []string{filepath.Join(home, "library", "keychains", "login.keychain-db")}},
	} {
		for _, mode := range []Mode{ModeAcceptEdits, ModeBypass} {
			if d := decide(t, b, Context{Mode: mode, Root: root}, req, ""); d.Actor != ActorProtectedPath {
				t.Errorf("%s %v: %s by %s", mode, req.Paths, d.Outcome, d.Actor)
			}
		}
	}
	g := NewPolicy(home, filepath.Join(home, "appdata")).Guard(root)
	if err := g.CheckWrite(filepath.Join(root, ".GIT", "config")); err == nil {
		t.Error("guard allowed a write to .GIT/config")
	}
}

// Review finding: unreadable rules (which may hold deny rules) must not let
// modes or allow rules auto-approve.
func TestUnreadableRulesFailClosed(t *testing.T) {
	b, _, _, root := setup(t)
	rules := &memRules{global: []Rule{{Effect: Allow, Tool: "shell", Scope: ScopeGlobal}}, err: errors.New("rules.yaml: bad YAML")}
	b.SetRules(rules, rules)
	write := Request{SessionID: "s", Tool: "write_file", Kind: KindEdit, Paths: []string{filepath.Join(root, "a.go")}}
	shell := Request{SessionID: "s", Tool: "shell", Kind: KindExecute, Command: "ls"}
	read := Request{SessionID: "s", Tool: "read_file", Kind: KindRead, Paths: []string{filepath.Join(root, "a.go")}}
	for _, mode := range []Mode{ModeAcceptEdits, ModeBypass} {
		if d := decide(t, b, Context{Mode: mode, Root: root}, write, "allow_once"); d.Actor != ActorUser {
			t.Errorf("%s write with broken rules decided by %s", mode, d.Actor)
		}
		if d := decide(t, b, Context{Mode: mode, Root: root}, shell, "allow_once"); d.Actor != ActorUser {
			t.Errorf("%s shell with broken rules decided by %s", mode, d.Actor)
		}
	}
	if d := decide(t, b, Context{Mode: ModeAsk, Root: root}, read, ""); d.Outcome != Allowed {
		t.Errorf("read inside the root = %+v", d)
	}
}

// Recheck finding: the policy only resolved protected directories that
// existed when it was built.
func TestProtectedDirectoryCreatedLaterIsCovered(t *testing.T) {
	home := t.TempDir() // under /var, a link to /private/var on macOS
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	policy := NewPolicy(home, filepath.Join(home, "appdata"))
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	os.WriteFile(filepath.Join(home, ".ssh", "id"), []byte("SECRET"), 0o600)
	env, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root)})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(home, ".ssh", "id"), filepath.Join(home, ".SSH", "id")} {
		if b, err := env.FS().ReadFile(p); err == nil {
			t.Errorf("read %s through the guard: %q", p, b)
		}
	}
}

type fakeHooks struct {
	mu        sync.Mutex
	pre, perm string // reasons to return
	preCalls  int
	permCalls int
}

func (h *fakeHooks) PreToolUse(_ context.Context, _ Context, _ Request) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.preCalls++
	return h.pre
}

func (h *fakeHooks) PermissionRequest(_ context.Context, _ Context, _ Request) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.permCalls++
	return h.perm
}

// EXT-005: a pre_tool_use hook blocks even an allowed call; a call already
// denied runs no hook; the permission_request hook runs only before the
// user would be asked; hooks never approve.
func TestHooksBlockButNeverApprove(t *testing.T) {
	b, _, home, root := setup(t)
	h := &fakeHooks{pre: "no shell on Fridays"}
	b.SetHooks(h)
	write := Request{SessionID: "s", Tool: "write_file", Kind: KindEdit, Paths: []string{filepath.Join(root, "a.go")}}
	shell := Request{SessionID: "s", Tool: "shell", Kind: KindExecute, Command: "make test"}
	accept := Context{Mode: ModeAcceptEdits, Root: root}

	if d := decide(t, b, accept, write, ""); d.Outcome != Denied || d.Actor != ActorHook || d.Reason != "no shell on Fridays" {
		t.Errorf("allowed write with a blocking hook = %+v", d)
	}
	protected := Request{SessionID: "s", Tool: "write_file", Kind: KindEdit, Paths: []string{filepath.Join(home, ".ssh", "config")}}
	before := h.preCalls
	if d := decide(t, b, accept, protected, ""); d.Actor != ActorProtectedPath || h.preCalls != before {
		t.Errorf("protected write = %+v, hooks ran %d times", d, h.preCalls-before)
	}

	h.pre, h.perm = "", "needs review"
	if d := decide(t, b, accept, write, ""); d.Outcome != Allowed || h.permCalls != 0 {
		t.Errorf("allowed write = %+v, permission hook ran %d times", d, h.permCalls)
	}
	if d := decide(t, b, accept, shell, ""); d.Outcome != Denied || d.Actor != ActorHook || d.Reason != "needs review" {
		t.Errorf("asked shell with a permission hook = %+v", d)
	}
	h.perm = ""
	if d := decide(t, b, accept, shell, "allow_once"); d.Outcome != Allowed || d.Actor != ActorUser {
		t.Errorf("hooks that pass still leave the decision to the user: %+v", d)
	}
}

// Review finding: path deny rules matched case-sensitively on a
// case-insensitive volume, and a search over the folder read the file
// anyway. Searches now leave out what a deny rule hides.
func TestDenyRulesIgnoreCaseAndHideFromSearch(t *testing.T) {
	b, _, _, root := setup(t)
	rules := &memRules{global: []Rule{{Effect: Deny, Tool: "*", Path: "secrets/**", Scope: ScopeGlobal}}}
	b.SetRules(rules, rules)
	for _, p := range []string{"secrets/key.txt", "SECRETS/key.txt", "Secrets/KEY.txt"} {
		d := b.Request(context.Background(), Context{Mode: ModeAsk, Root: root}, Request{SessionID: "s", Tool: "read_file", Kind: KindRead, Paths: []string{filepath.Join(root, p)}})
		if d.Outcome != Denied {
			t.Errorf("read %s: %s", p, d.Outcome)
		}
		if !b.Hidden("s", root, filepath.Join(root, p)) {
			t.Errorf("%s not hidden from searches", p)
		}
	}
	if b.Hidden("s", root, filepath.Join(root, "src", "main.go")) {
		t.Error("an unrelated file is hidden")
	}
}
