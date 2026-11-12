package engine

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// delegator, as the parent, hands a task to a subagent and then repeats
// what came back; as the subagent (offered no task tool), it answers.
type delegator struct {
	mu        sync.Mutex
	childSeen []provider.ChatRequest
}

func (d *delegator) ID() string                                               { return "d" }
func (d *delegator) Type() provider.Type                                      { return provider.TypeScripted }
func (d *delegator) ListModels(context.Context) ([]provider.ModelInfo, error) { return nil, nil }
func (d *delegator) Capabilities(string) provider.ModelCaps {
	return provider.ModelCaps{Source: provider.CapsStatic, Tools: true}
}

func (d *delegator) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.Event, error) {
	parent := slices.ContainsFunc(req.Tools, func(t provider.ToolSpec) bool { return t.Name == "task" })
	last := req.Messages[len(req.Messages)-1].Parts[0]
	msg := provider.Message{Role: provider.RoleAssistant}
	stop := provider.StopEndTurn
	switch {
	case !parent:
		d.mu.Lock()
		d.childSeen = append(d.childSeen, req)
		d.mu.Unlock()
		msg.Parts = []provider.Part{{Type: provider.PartText, Text: "The project has 3 Go files."}}
	case last.Type == provider.PartToolResult:
		msg.Parts = []provider.Part{{Type: provider.PartText, Text: "The subagent says: " + last.Text}}
	default:
		in, _ := json.Marshal(map[string]string{"description": "Count Go files", "prompt": "Count the Go files in the project."})
		msg.Parts = []provider.Part{{Type: provider.PartToolUse, ToolCallID: "t1", ToolName: "task", Input: in}}
		stop = provider.StopToolUse
	}
	ch := make(chan provider.Event, 1)
	ch <- provider.Event{Type: provider.EventDone, Message: &msg, StopReason: stop}
	close(ch)
	return ch, nil
}

// HRN-014: the task tool runs a subagent in a session of its own, linked
// to the parent, whose final reply comes back as the tool's result; the
// subagent has no task tool; the transcript links to the subagent's.
func TestSubagents(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	d := &delegator{}
	e.mu.Lock()
	e.providers.built["d"] = &configuredProvider{cfg: config.Provider{ID: "d", Name: "d", Type: "scripted"}, p: d}
	e.mu.Unlock()
	parent, err := e.CreateSession(ctx, CreateSessionRequest{Title: "Lead", ProjectRoot: t.TempDir(), ProviderID: "d", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, parent.ID, "How many Go files are there?")
	waitIdle(t, ch, parent.ID)

	list, _ := e.ListSessions(ctx)
	i := slices.IndexFunc(list, func(s session.Session) bool { return s.ParentID == parent.ID })
	if i < 0 {
		t.Fatalf("no subagent session among %+v", list)
	}
	child := list[i]
	if child.Title != "Subagent: Count Go files" || child.ProjectRoot != parent.ProjectRoot {
		t.Errorf("child = %+v", child)
	}
	if len(d.childSeen) != 1 || slices.ContainsFunc(d.childSeen[0].Tools, func(t provider.ToolSpec) bool { return t.Name == "task" }) {
		t.Errorf("the subagent was offered the task tool, or ran %d times", len(d.childSeen))
	}
	evs, _ := e.Events(ctx, parent.ID, 0, 0)
	all, _ := json.Marshal(evs)
	if !strings.Contains(string(all), "freeloop://session/"+child.ID) || !strings.Contains(string(all), "The subagent says: The project has 3 Go files.") {
		t.Errorf("the parent's transcript:\n%s", all)
	}
	if !slices.ContainsFunc(evs, func(ev event.Event) bool {
		return ev.Type == event.PermissionDecided && strings.Contains(string(ev.Data), `"allowed"`)
	}) {
		t.Error("the task call was not decided by the broker")
	}
}

// HRN-014 with SES-010: a worktree session's subagents share its worktree,
// stop when their task returns, and go with it: archiving the session
// archives them and still removes the worktree, deleting it deletes them.
func TestSubagentsGoWithTheirSession(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	d := &delegator{}
	e.mu.Lock()
	e.providers.built["d"] = &configuredProvider{cfg: config.Provider{ID: "d", Name: "d", Type: "scripted"}, p: d}
	e.mu.Unlock()
	repo := gitRepo(t)
	if _, err := e.SetProjectTrust(ctx, repo, true); err != nil {
		t.Fatal(err)
	}
	parent, err := e.CreateSession(ctx, CreateSessionRequest{Title: "Lead", ProjectRoot: repo, ProviderID: "d", Model: "m", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, parent.ID, "How many Go files are there?")
	waitIdle(t, ch, parent.ID)
	children := e.subagentsOf(ctx, parent.ID)
	if len(children) != 1 || children[0].Worktree != parent.Worktree {
		t.Fatalf("children = %+v", children)
	}
	child := children[0]
	e.mu.Lock()
	_, live := e.runtimes[child.ID]
	e.mu.Unlock()
	if live {
		t.Error("the subagent's runtime outlived its task")
	}

	if err := e.ArchiveSession(ctx, parent.ID, true); err != nil {
		t.Fatal(err)
	}
	if c, _ := e.store.GetSession(ctx, child.ID); c.Status != session.StatusArchived {
		t.Errorf("the subagent is %s after its session was archived", c.Status)
	}
	if _, err := os.Stat(parent.Worktree); !os.IsNotExist(err) {
		t.Errorf("the worktree stayed: %v", err)
	}
	if err := e.ArchiveSession(ctx, parent.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := e.DeleteSession(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.GetSession(ctx, child.ID); err == nil {
		t.Error("the subagent outlived its deleted session")
	}
}
