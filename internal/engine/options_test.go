package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/session"
)

func TestAttachedFiles(t *testing.T) {
	got, err := attachedFiles([]FileInput{{Name: "/some/where/notes\n.md", Text: "hello"}}, 0)
	if err != nil || len(got) != 1 || got[0].Path != "notes.md" || got[0].URI != "attachment:notes.md" || got[0].Text != "hello" {
		t.Fatalf("got %+v, %v", got, err)
	}
	for name, in := range map[string][]FileInput{
		"not text":  {{Name: "a", Text: "x\x00y"}},
		"bad bytes": {{Name: "a", Text: "\xff\xfe"}},
		"no name":   {{Name: "", Text: "x"}},
		"too large": {{Name: "a", Text: strings.Repeat("x", maxMentionBytes+1)}},
		"too many":  make([]FileInput, maxMentions+1),
	} {
		if _, err := attachedFiles(in, 0); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if _, err := attachedFiles([]FileInput{{Name: "a", Text: "xx"}}, maxMentionTotal-1); err == nil {
		t.Error("a message over its limit was accepted")
	}
}

// A file reaches the model's prompt, and the transcript names it.
func TestPromptWithAttachedFile(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	sid := scriptedSession(t, e, t.TempDir())
	if _, err := e.PromptWith(ctx, sid, PromptRequest{Files: []FileInput{{Name: "notes.md", Text: "the note"}}}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, sid)
	evs, _ := e.Events(ctx, sid, 0)
	named := false
	for _, ev := range evs {
		if ev.Type == event.UserMessageChunk && strings.Contains(string(ev.Data), `"resource_link"`) && strings.Contains(string(ev.Data), "notes.md") {
			named = true
		}
	}
	if !named {
		t.Error("the transcript does not name the file")
	}
	if _, err := e.PromptWith(ctx, sid, PromptRequest{Files: []FileInput{{Name: "bin", Text: "\x00"}}}); err == nil {
		t.Error("a binary file was sent")
	}
}

func TestOpenProjectFolderUsesTheSessionsFolder(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	root := t.TempDir()
	sid := scriptedSession(t, e, root)
	if err := e.OpenProjectFolder(ctx, sid); err == nil {
		t.Error("a build with no way to open folders said it did")
	}
	marker := filepath.Join(t.TempDir(), "opened")
	e.folderArgv = func(path string) []string {
		return []string{"sh", "-c", `printf %s "$1" > "$2"`, "sh", path, marker}
	}
	if err := e.OpenProjectFolder(ctx, sid); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(marker)
	ses, _ := e.store.GetSession(ctx, sid)
	if err != nil || string(b) != ses.WorkRoot() {
		t.Errorf("opened %q, %v; want %q", b, err, ses.WorkRoot())
	}
	if err := e.OpenProjectFolder(ctx, "ses_none"); err == nil {
		t.Error("opened a folder for no session")
	}
}

// A rule added in a session lasts as long as it, across a restart, and removing it
// does too.
func TestSessionRulesLastAcrossARestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	sid := scriptedSession(t, e, t.TempDir())
	one := permission.Rule{Effect: permission.Deny, Tool: "shell", Command: "rm *"}
	two := permission.Rule{Effect: permission.Allow, Tool: "shell", Command: "go test *"}
	for _, r := range []permission.Rule{one, two} {
		if err := e.AddSessionRule(sid, r); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.AddSessionRule(sid, permission.Rule{Effect: permission.Allow}); err == nil {
		t.Error("a rule with no tool was accepted")
	}
	if err := e.DeleteSessionRule(sid, two); err != nil {
		t.Fatal(err)
	}
	if err := e.DeleteSessionRule(sid, two); err == nil {
		t.Error("removed a rule twice")
	}
	evs, _ := e.Events(ctx, sid, 0)
	e.rules.forget(sid)
	e.rules.restore(sid, evs)
	var kept []permission.Rule
	for _, r := range e.ListRules(sid) {
		if r.Scope == permission.ScopeSession {
			kept = append(kept, r)
		}
	}
	if len(kept) != 1 || !sameRule(kept[0], one) {
		t.Errorf("after a restart: %+v", kept)
	}
	if other := scriptedSession(t, e, t.TempDir()); len(e.ListRules(other)) != 0 {
		t.Errorf("another session has them: %+v", e.ListRules(other))
	}
}

func TestEffortIsTheSessions(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	sid := scriptedSession(t, e, t.TempDir())
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetSessionOption(ctx, sid, optionEffortID, "enormous"); err == nil {
		t.Error("an effort that does not exist was accepted")
	}
	if err := e.SetSessionOption(ctx, sid, optionEffortID, "low"); err != nil {
		t.Fatal(err)
	}
	ses, _ = e.store.GetSession(ctx, sid)
	if ses.Effort != "low" {
		t.Errorf("effort = %q", ses.Effort)
	}
	// Choosing what it already is records nothing.
	before, _ := e.Events(ctx, sid, 0)
	if err := e.SetSessionOption(ctx, sid, optionEffortID, "low"); err != nil {
		t.Fatal(err)
	}
	if after, _ := e.Events(ctx, sid, 0); len(after) != len(before) {
		t.Errorf("%d events, then %d", len(before), len(after))
	}
	if err := e.SetSessionOption(ctx, sid, "something else", "x"); err == nil {
		t.Error("an unknown option was accepted")
	}
	if err := e.SetSessionOption(ctx, sid, optionModelID, "no separator"); err == nil {
		t.Error("a malformed model was accepted")
	}
}

func TestBuiltinOptionsListTheModel(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	sid := scriptedSession(t, e, t.TempDir())
	report, err := e.SessionOptions(ctx, sid)
	if err != nil || len(report.Options) == 0 || report.Unknown {
		t.Fatalf("report = %+v, %v", report, err)
	}
	m := report.Options[0]
	ses, _ := e.store.GetSession(ctx, sid)
	if m.Category != harness.OptionModel || !m.Custom || m.Current != ses.ProviderID+modelSeparator+ses.Model {
		t.Errorf("model option = %+v", m)
	}
	found := false
	for _, c := range m.Choices {
		found = found || c.Value == m.Current
	}
	if !found {
		t.Errorf("the current model is not among %+v", m.Choices)
	}
}

// An agent that has not run yet cannot say; once it has, the transcript can.
func TestAgentOptionsComeFromItsLastReport(t *testing.T) {
	report := []byte(`{"configOptions":[
		{"id":"mode","name":"Mode","category":"mode","type":"select","currentValue":"default","options":[{"value":"bypassPermissions","name":"Bypass"}]},
		{"id":"fast","name":"Fast","type":"boolean","currentValue":true},
		{"id":"effort","name":"Effort","category":"thought_level","type":"select","currentValue":"high","options":[{"value":"low","name":"Low"},{"value":"high","name":"High"}]},
		{"id":"model","name":"Model","category":"model","type":"select","currentValue":"opus","options":[{"name":"Claude","options":[{"value":"opus","name":"Opus"}]}]}
	]}`)
	got := optionsInEvents([]event.Event{{Type: event.ConfigOptionUpdate, Data: json.RawMessage(`{"configOptions":[]}`)}, {Type: event.Notice, Data: json.RawMessage(`{}`)}, {Type: event.ConfigOptionUpdate, Data: report}})
	if len(got) != 2 || got[0].ID != "model" || got[1].ID != "effort" {
		t.Fatalf("options = %+v", got)
	}
	if got[0].Choices[0].Group != "Claude" || got[0].Current != "opus" {
		t.Errorf("model = %+v", got[0])
	}
	values := lastOptions([]event.Event{{Type: event.ConfigOptionUpdate, Data: report}})
	if len(values) != 2 || values[0] != (harness.OptionValue{ID: "model", Value: "opus"}) {
		t.Errorf("values = %+v", values)
	}
}

func TestExternalSessionWaitsToBeAsked(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, err := e.store.CreateSession(ctx, session.Session{Title: "x", HarnessID: "claude-code", ProjectRoot: t.TempDir(), Mode: "ask", Status: session.StatusIdle})
	if err != nil {
		t.Skip("cannot make a session this way: ", err)
	}
	report, err := e.SessionOptions(ctx, ses.ID)
	if err != nil || !report.Unknown || len(report.Options) != 0 {
		t.Errorf("report = %+v, %v", report, err)
	}
	e.mu.Lock()
	running := e.runtimes[ses.ID] != nil
	e.mu.Unlock()
	if running {
		t.Error("asking what it offers started the agent")
	}
}

// effortProvider takes an effort and remembers what it was asked for.
type effortProvider struct{ byPrompt }

func (p *effortProvider) Capabilities(string) provider.ModelCaps {
	return provider.ModelCaps{Source: provider.CapsStatic, Tools: true, Effort: true}
}

// The built-in agent's next model call uses the chosen effort, and so does the next
// run after a restart.
func TestBuiltinEffortReachesTheModel(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	p := &effortProvider{}
	e.mu.Lock()
	e.providers.built["e"] = &configuredProvider{cfg: config.Provider{ID: "e", Name: "E"}, p: p}
	e.mu.Unlock()
	root := t.TempDir()
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "e", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	report, err := e.SessionOptions(ctx, ses.ID)
	if err != nil || len(report.Options) != 2 || report.Options[1].Category != harness.OptionEffort || report.Options[1].Current != "high" {
		t.Fatalf("report = %+v, %v", report, err)
	}
	ch, cancel := e.Subscribe()
	defer cancel()
	stop := make(chan struct{})
	defer close(stop)
	approveAll(e, stop)
	run := func(file string) string {
		t.Helper()
		if _, err := e.Prompt(ctx, ses.ID, "write "+file+" x"); err != nil {
			t.Fatal(err)
		}
		waitIdle(t, ch, ses.ID)
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.requests[len(p.requests)-1].Effort
	}
	if got := run("a"); got != "high" {
		t.Errorf("default effort = %q", got)
	}
	if err := e.SetSessionOption(ctx, ses.ID, optionEffortID, "low"); err != nil {
		t.Fatal(err)
	}
	if got := run("b"); got != "low" {
		t.Errorf("a running session's effort = %q", got)
	}
	e.stopRuntime(ses.ID)
	if got := run("c"); got != "low" {
		t.Errorf("after the agent started again, effort = %q", got)
	}
	report, _ = e.SessionOptions(ctx, ses.ID)
	if report.Options[1].Current != "low" {
		t.Errorf("reported = %+v", report.Options[1])
	}
}
