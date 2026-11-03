package acp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/acp/acptest"
	"github.com/FinanceNik/Freeloop/internal/permission"
)

// Review finding: a read the policy allowed without asking left an
// approval that let any number of writes to the file through, even in
// plan mode. An approval now covers only its own kind, and a write spends
// it.
func TestApprovalCoversOnlyItsKind(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	file := filepath.Join(root, "package.json")
	os.WriteFile(file, []byte("{}"), 0o644)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root)})
	if err != nil {
		t.Fatal(err)
	}
	broker := permission.NewBroker(policy, func(string, event.Type, any) {})
	s := &session{p: harness.Params{SessionID: "x", Broker: broker, Emit: func(event.Type, any) {}}, env: env, mode: permission.ModePlan, terms: map[string]*terminal{}}
	allow := []sdk.PermissionOption{{OptionId: "ok", Kind: sdk.PermissionOptionKindAllowOnce, Name: "ok"}}
	read := sdk.ToolKindRead
	if _, err := s.RequestPermission(context.Background(), sdk.RequestPermissionRequest{
		ToolCall: sdk.ToolCallUpdate{ToolCallId: "t1", Kind: &read, Locations: []sdk.ToolCallLocation{{Path: file}}},
		Options:  allow,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteTextFile(context.Background(), sdk.WriteTextFileRequest{Path: file, Content: "changed"}); err == nil {
		t.Error("a read's approval let a write through in plan mode")
	}
	if b, _ := os.ReadFile(file); string(b) != "{}" {
		t.Errorf("file = %q", b)
	}

	// An edit approval covers one write.
	s.mode = permission.ModeAcceptEdits
	edit := sdk.ToolKindEdit
	s.RequestPermission(context.Background(), sdk.RequestPermissionRequest{
		ToolCall: sdk.ToolCallUpdate{ToolCallId: "t2", Kind: &edit, Locations: []sdk.ToolCallLocation{{Path: file}}},
		Options:  allow,
	})
	if !s.covered([]permission.Kind{permission.KindEdit}, env.Resolve(file), "", true) {
		t.Fatal("the edit's approval does not cover its write")
	}
	if s.covered([]permission.Kind{permission.KindEdit}, env.Resolve(file), "", true) {
		t.Error("the edit's approval covered a second write")
	}
}

// shortener stands in for a token optimizer that rewrote one command.
type shortener struct{ made, original string }

func (o shortener) Rewrite(context.Context, string, bool) harness.Rewrite { return harness.Rewrite{} }
func (o shortener) Original(command string) (string, string, bool) {
	if command == o.made {
		return o.original, "Short", true
	}
	return "", "", false
}

// An agent asks about the command its hook gave it. The user and the rules decide
// about what the model wrote, and a read-only one does not need asking (HRN-019).
func TestRewrittenCommandIsJudgedAsWritten(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root)})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var asked []string
	broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) {
		if typ != event.PermissionRequested {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		b, _ := json.Marshal(data)
		asked = append(asked, string(b))
	})
	var updates []map[string]any
	emit := func(typ event.Type, data any) {
		if m, ok := data.(map[string]any); ok && typ == event.ToolCallUpdate {
			updates = append(updates, m)
		}
	}
	made := `export PATH=/x:"$PATH"; short git status`
	s := &session{p: harness.Params{SessionID: "x", Broker: broker, Emit: emit, Optimizer: shortener{made, "git status"}}, env: env, mode: permission.ModeAsk, terms: map[string]*terminal{}}
	options := []sdk.PermissionOption{
		{OptionId: "ok", Kind: sdk.PermissionOptionKindAllowOnce, Name: "ok"},
		{OptionId: "no", Kind: sdk.PermissionOptionKindRejectOnce, Name: "no"},
	}
	execute := sdk.ToolKindExecute
	title := made
	res, err := s.RequestPermission(context.Background(), sdk.RequestPermissionRequest{
		ToolCall: sdk.ToolCallUpdate{ToolCallId: "t1", Kind: &execute, Title: &title, RawInput: map[string]any{"command": made}},
		Options:  options,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome.Selected == nil || res.Outcome.Selected.OptionId != "ok" {
		t.Errorf("a read-only command was not allowed: %+v", res.Outcome)
	}
	if len(asked) != 0 {
		t.Errorf("a read-only command asked: %v", asked)
	}
	if len(updates) != 1 || updates[0]["via"] != "Short" || updates[0]["toolCallId"] != "t1" {
		t.Errorf("updates = %+v", updates)
	}

	// The same rewrite of a command that writes asks, and shows what the model wrote.
	s.p.Optimizer = shortener{made, "git push"}
	done := make(chan sdk.RequestPermissionResponse, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		res, _ := s.RequestPermission(ctx, sdk.RequestPermissionRequest{
			ToolCall: sdk.ToolCallUpdate{ToolCallId: "t2", Kind: &execute, Title: &title, RawInput: map[string]any{"command": made}},
			Options:  options,
		})
		done <- res
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(asked)
		mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a command that writes did not ask")
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	request := asked[0]
	mu.Unlock()
	if !strings.Contains(request, "git push") || strings.Contains(request, "short git") {
		t.Errorf("the request shows the rewrite: %s", request)
	}
	cancel()
	<-done
	if len(updates) != 1 {
		t.Errorf("a command that was not allowed got a note: %+v", updates)
	}

	// What an optimizer did not make is judged as it stands.
	other := "short git status"
	s.p.Optimizer = shortener{made, "git status"}
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go func() {
		s.RequestPermission(ctx2, sdk.RequestPermissionRequest{
			ToolCall: sdk.ToolCallUpdate{ToolCallId: "t3", Kind: &execute, Title: &other, RawInput: map[string]any{"command": other}},
			Options:  options,
		})
	}()
	deadline = time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(asked)
		mu.Unlock()
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a command no optimizer made was let through")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// An update naming a rewrite shows what the model wrote, and which optimizer ran it.
func TestShowOriginal(t *testing.T) {
	s := &session{p: harness.Params{Optimizer: shortener{"short ls", "ls"}}}
	m := map[string]any{"rawInput": map[string]any{"command": "short ls"}, "title": "short ls"}
	s.showOriginal(m)
	if m["title"] != "ls" || m["via"] != "Short" || m["rawInput"].(map[string]any)["command"] != "ls" {
		t.Errorf("m = %+v", m)
	}
	m = map[string]any{"rawInput": map[string]any{"command": "pwd"}, "title": "pwd"}
	s.showOriginal(m)
	if m["title"] != "pwd" || m["via"] != nil {
		t.Errorf("m = %+v", m)
	}
}

// Only what the agent listed can be chosen: its mode, which could loosen the app's
// permissions, stays out of reach even when it offers one (SEC-003).
func TestOnlyListedOptionsCanBeChosen(t *testing.T) {
	raw := []byte(`[{"id":"mode","category":"mode","type":"select","currentValue":"default","options":[{"value":"bypassPermissions","name":"Bypass"}]},
		{"id":"model","name":"Model","category":"model","type":"select","currentValue":"a","options":[{"value":"a","name":"A"}]}]`)
	s := &session{preset: config.HarnessPreset{Name: "Agent"}, options: harness.ParseOptions(raw)}
	if err := s.SetOption(context.Background(), "mode", "bypassPermissions"); err == nil {
		t.Error("the agent's mode could be set")
	}
	if err := s.SetOption(context.Background(), "model", "unlisted"); err == nil {
		t.Error("an unlisted model could be set")
	}
	if got := s.Options(); len(got) != 1 || got[0].ID != "model" {
		t.Errorf("options = %+v", got)
	}
}

// Stopping a turn ends what it was waiting on. The request's own context is the
// connection's and outlives the turn, so the card stayed and a later Allow still
// answered the agent.
func TestStoppingATurnEndsItsApprovals(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, _ := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root)})
	rec := &recorder{}
	broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
	s := &session{p: harness.Params{SessionID: "x", Broker: broker, Emit: rec.emit}, env: env, mode: permission.ModeAsk, terms: map[string]*terminal{}}
	turn, stop := context.WithCancel(context.Background())
	defer stop()
	s.mu.Lock()
	s.turn = turn
	s.mu.Unlock()

	got := make(chan sdk.RequestPermissionResponse, 1)
	go func() {
		res, err := s.RequestPermission(context.Background(), sdk.RequestPermissionRequest{
			ToolCall: sdk.ToolCallUpdate{ToolCallId: "t", Title: sdk.Ptr("Edit"), Kind: sdk.Ptr(sdk.ToolKindEdit)},
			Options:  []sdk.PermissionOption{{OptionId: "yes", Name: "Allow", Kind: sdk.PermissionOptionKindAllowOnce}},
		})
		if err != nil {
			t.Error(err)
		}
		got <- res
	}()
	for len(broker.Pending()) == 0 {
		time.Sleep(time.Millisecond)
	}
	stop()
	select {
	case res := <-got:
		if res.Outcome.Cancelled == nil {
			t.Errorf("outcome = %+v", res.Outcome)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the request outlived its turn")
	}
	if p := broker.Pending(); len(p) != 0 {
		t.Errorf("still waiting: %+v", p)
	}
}

// SES-003: an agent that cannot load its session gets the conversation in its next
// prompt, and only that one.
func TestAnAgentThatCannotLoadGetsARecap(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root), Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
	exe, _ := os.Executable()
	h := Harness{Preset: config.HarnessPreset{ID: "ref", Name: "Reference", Command: exe, Env: map[string]string{acptest.EnvVar: "1"}}}
	got, err := h.NewSession(context.Background(), harness.Params{
		SessionID: "ses", Env: env, Broker: broker, Mode: permission.ModeAsk, Emit: rec.emit,
		ExternalID: "old", Recap: func() string { return "the story so far" },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	s := got.(*session)
	if !strings.Contains(rec.text(event.Notice), "recap") {
		t.Errorf("notice: %s", rec.text(event.Notice))
	}
	if r := s.takeRecap(); r != "the story so far" {
		t.Errorf("recap = %q", r)
	}
	if r := s.takeRecap(); r != "" {
		t.Errorf("a second recap = %q", r)
	}
}
