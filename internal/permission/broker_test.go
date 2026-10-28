package permission

import (
	"context"
	"errors"
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
			if err := b.Decide(p[0].ID, option, ""); err != nil {
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
	if err := b.Decide("nope", "allow_once", ""); !errors.Is(err, ErrUnknownRequest) {
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
