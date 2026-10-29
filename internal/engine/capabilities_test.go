package engine

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

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
	for _, want := range []string{"anthropic", "ollama", "openai", "openrouter", "lmstudio", "llamacpp", "vllm", "ollama-v1", "custom"} {
		if !strings.Contains(strings.Join(ids, ","), want) {
			t.Errorf("preset %s missing from %v", want, ids)
		}
	}
	if _, err := e.SaveProvider(SaveProviderRequest{Name: "X", Type: "openai_compat", Preset: "custom"}); err == nil {
		t.Error("custom endpoint without a base URL accepted")
	}
}
