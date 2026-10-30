package engine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

func openPlain(t *testing.T, dir string) *Engine {
	t.Helper()
	e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestOllamaToolSupportGatesSessions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		switch {
		case r.URL.Path == "/api/show" && strings.Contains(string(b), "qwen3"):
			io.WriteString(w, `{"capabilities":["completion","tools"]}`)
		case r.URL.Path == "/api/show":
			io.WriteString(w, `{"capabilities":["completion"]}`)
		case r.URL.Path == "/api/tags":
			io.WriteString(w, `{"models":[{"model":"qwen3:8b"},{"model":"gemma:2b"}]}`)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	e := openPlain(t, t.TempDir())
	defer e.Close()
	if _, err := e.SaveProvider(SaveProviderRequest{Name: "Ollama", Type: "ollama", BaseURL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	models, err := e.TestProvider(ctx, "ollama")
	if err != nil || len(models) != 2 {
		t.Fatalf("models = %v, %v", models, err)
	}
	root := t.TempDir()
	if _, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "ollama", Model: "qwen3:8b"}); err != nil {
		t.Errorf("tool-capable model rejected: %v", err)
	}
	_, err = e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "ollama", Model: "gemma:2b"})
	if err == nil || !strings.Contains(err.Error(), "no tool support") || !strings.Contains(err.Error(), "Use anyway") {
		t.Errorf("model without tools: err = %v", err)
	}
	on := true
	if err := e.SetToolsOverride("ollama", "gemma:2b", &on); err != nil {
		t.Fatal(err)
	}
	if caps, _ := e.ModelCapabilities(ctx, "ollama", "gemma:2b", false); caps.Source != provider.CapsOverride || !caps.Tools {
		t.Errorf("caps after override = %+v", caps)
	}
	if _, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "ollama", Model: "gemma:2b"}); err != nil {
		t.Errorf("override did not make the model selectable: %v", err)
	}
	if err := e.SetToolsOverride("ollama", "gemma:2b", nil); err != nil {
		t.Fatal(err)
	}
	if caps, _ := e.ModelCapabilities(ctx, "ollama", "gemma:2b", false); caps.Source != provider.CapsReported {
		t.Errorf("caps after clearing the override = %+v", caps)
	}
}

func TestLocalModelIsProbedOnceAndCached(t *testing.T) {
	var chats atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			chats.Add(1)
			io.WriteString(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"p","function":{"name":"ping","arguments":"{\"value\":\"ok\"}"}}]},"finish_reason":"tool_calls"}]}`+"\n\ndata: [DONE]\n\n")
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	dir := t.TempDir()
	e := openPlain(t, dir)
	info, err := e.SaveProvider(SaveProviderRequest{Name: "LM Studio", Type: "openai_compat", Preset: "lmstudio", BaseURL: srv.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if caps, _ := e.ModelCapabilities(ctx, info.ID, "qwen", false); caps.Source != provider.CapsUnknown || caps.Tools {
		t.Errorf("caps before probing = %+v", caps)
	}
	if _, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: info.ID, Model: "qwen"}); err != nil {
		t.Fatal(err)
	}
	if chats.Load() != 1 {
		t.Errorf("probe requests = %d, want 1", chats.Load())
	}
	e.Close()
	if b, _ := os.ReadFile(filepath.Join(dir, "capabilities.yaml")); !strings.Contains(string(b), "qwen") {
		t.Errorf("probe not cached: %s", b)
	}

	e = openPlain(t, dir)
	defer e.Close()
	caps, err := e.ModelCapabilities(ctx, info.ID, "qwen", true)
	if err != nil || caps.Source != provider.CapsProbe || !caps.Tools || chats.Load() != 1 {
		t.Errorf("after restart: caps = %+v, %v, probes = %d", caps, err, chats.Load())
	}
}

func TestPresets(t *testing.T) {
	e := openPlain(t, t.TempDir())
	defer e.Close()
	ids := []string{}
	for _, p := range e.ProviderPresets() {
		ids = append(ids, p.ID)
	}
	for _, want := range []string{"anthropic", "openai_responses", "gemini", "ollama", "openai", "openrouter", "lmstudio", "llamacpp", "vllm", "ollama-v1", "custom"} {
		if !strings.Contains(strings.Join(ids, ","), want) {
			t.Errorf("preset %s missing from %v", want, ids)
		}
	}
	if _, err := e.SaveProvider(SaveProviderRequest{Name: "X", Type: "openai_compat", Preset: "custom"}); err == nil {
		t.Error("custom endpoint without a base URL accepted")
	}

	// PRV-012: the native OpenAI and Gemini providers save and build, and
	// know their models' capabilities without a probe.
	for _, c := range []struct{ typ, model string }{{"openai_responses", "gpt-5.5"}, {"gemini", "gemini-3-pro"}} {
		info, err := e.SaveProvider(SaveProviderRequest{Name: c.typ, Type: c.typ, Preset: "ignored", APIKey: "test-key-" + c.typ})
		if err != nil || info.Type != c.typ || info.Preset != "" {
			t.Fatalf("save %s: %+v, %v", c.typ, info, err)
		}
		caps, err := e.ModelCapabilities(context.Background(), info.ID, c.model, false)
		if err != nil || caps.Source != provider.CapsStatic || !caps.Tools || !caps.Effort {
			t.Errorf("%s caps = %+v, %v", c.typ, caps, err)
		}
	}
}

// GUI-006: "Allow always" for a shell command creates a visible rule that
// approves the next identical call; session rules survive a restart.
func TestAllowAlwaysCreatesVisibleRules(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	ch, unsub := e.Subscribe()
	ses, _ := demoSession(t, e, "demo", "ask")
	e.Prompt(ctx, ses.ID, "go")

	answer := func(scope string) {
		req := waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested })
		var pr struct{ RequestID string }
		json.Unmarshal(req.Data, &pr)
		if err := e.Decide(DecideRequest{RequestID: pr.RequestID, OptionID: "allow_always", Scope: scope}); err != nil {
			t.Fatal(err)
		}
	}
	answer("session") // write_file freeloop-demo.txt
	answer("global")  // shell ls -la ... && cat ...
	waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.TurnEnded })

	rules := e.ListRules(ses.ID)
	if len(rules) != 2 || rules[0].Scope != "global" || rules[0].Tool != "shell" || rules[1].Scope != "session" || rules[1].Path != "freeloop-demo.txt" {
		t.Fatalf("rules = %+v", rules)
	}
	if len(e.ListRules("")) != 1 {
		t.Errorf("session rule visible without its session: %+v", e.ListRules(""))
	}
	b, _ := os.ReadFile(filepath.Join(dir, "rules.yaml"))
	if !strings.Contains(string(b), "ls -la freeloop-demo.txt") {
		t.Errorf("rules.yaml = %s", b)
	}

	// The second run asks nothing: both calls are approved by rules.
	e.Prompt(ctx, ses.ID, "again")
	end := waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.TurnEnded || ev.Type == event.PermissionRequested })
	if end.Type != event.TurnEnded {
		t.Fatalf("second run asked: %s", end.Data)
	}
	unsub()
	e.Close()

	// After a restart the session rule comes back from the transcript.
	e = openTest(t, dir)
	defer e.Close()
	if _, err := e.runtimeFor(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	if rules := e.ListRules(ses.ID); len(rules) != 2 {
		t.Errorf("rules after restart = %+v", rules)
	}
	if err := e.DeleteRule(rules[0]); err != nil {
		t.Fatal(err)
	}
	if rules := e.ListRules(ses.ID); len(rules) != 1 || rules[0].Scope != "session" {
		t.Errorf("rules after deleting the global one = %+v", rules)
	}
}

func TestHandEditedRulesAreReloaded(t *testing.T) {
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	os.WriteFile(filepath.Join(dir, "rules.yaml"), []byte("version: 1\nrules:\n  - {effect: deny, tool: shell, command: \"rm *\"}\n  - {effect: bogus, tool: x}\n"), 0o600)
	future := time.Now().Add(time.Second)
	os.Chtimes(filepath.Join(dir, "rules.yaml"), future, future)
	if rules := e.ListRules(""); len(rules) != 1 || rules[0].Effect != "deny" {
		t.Errorf("rules = %+v", rules)
	}
	found := false
	for _, i := range e.ConfigIssues() {
		found = found || strings.Contains(i.Field, "rules[1]")
	}
	if !found {
		t.Errorf("invalid rule not reported: %+v", e.ConfigIssues())
	}
}

// Review finding: a malformed rules.yaml dropped every rule and the next
// "Allow always" rewrote the file with only the new rule.
func TestBrokenRulesFileFailsClosedAndIsNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	good := "version: 1\nrules:\n  - {effect: deny, tool: shell, command: \"rm *\"}\n"
	os.WriteFile(filepath.Join(dir, "rules.yaml"), []byte(good), 0o600)
	bump := func() {
		future := time.Now().Add(time.Duration(len(e.ConfigIssues())+1) * time.Second)
		os.Chtimes(filepath.Join(dir, "rules.yaml"), future, future)
	}
	bump()
	if rules := e.ListRules(""); len(rules) != 1 {
		t.Fatalf("rules = %+v", rules)
	}

	broken := good + "  - {effect: deny, tool: shell, comand: typo}\n"
	os.WriteFile(filepath.Join(dir, "rules.yaml"), []byte(broken), 0o600)
	bump()
	if rules := e.ListRules(""); len(rules) != 1 || rules[0].Command != "rm *" {
		t.Errorf("last good rules not kept: %+v", rules)
	}
	if err := e.AddRule(permission.Rule{Effect: permission.Allow, Tool: "shell", Command: "ls"}); err == nil {
		t.Error("a rule was added while rules.yaml has errors")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "rules.yaml")); string(b) != broken {
		t.Errorf("rules.yaml was rewritten:\n%s", b)
	}
	if _, err := e.rules.Rules(""); err == nil {
		t.Error("the broker is not told that rules are broken")
	}
}
