package ollama

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/provider"
)

// fake serves the endpoints the adapter uses, in the shapes Ollama 0.12+
// returns them.
func fake(t *testing.T, chat []string, sent *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			io.WriteString(w, `{"models":[{"name":"qwen3:8b","model":"qwen3:8b"},{"name":"llama3.2:3b","model":"llama3.2:3b"}]}`)
		case "/api/show":
			var b struct{ Model string }
			json.NewDecoder(r.Body).Decode(&b)
			switch b.Model {
			case "qwen3:8b":
				io.WriteString(w, `{"capabilities":["completion","tools","thinking"],"model_info":{"qwen3.context_length":40960}}`)
			case "gemma:2b":
				io.WriteString(w, `{"capabilities":["completion"],"model_info":{"gemma.context_length":8192}}`)
			case "old":
				io.WriteString(w, `{"model_info":{}}`)
			default:
				w.WriteHeader(http.StatusNotFound)
				io.WriteString(w, `{"error":"model 'nope' not found"}`)
			}
		case "/api/chat":
			if sent != nil {
				json.NewDecoder(r.Body).Decode(sent)
			}
			for _, line := range chat {
				io.WriteString(w, line+"\n")
			}
		}
	}))
}

func TestListAndDescribe(t *testing.T) {
	srv := fake(t, nil, nil)
	defer srv.Close()
	p := New(Options{BaseURL: srv.URL})
	ctx := context.Background()

	models, err := p.ListModels(ctx)
	if err != nil || len(models) != 2 || models[0].ID != "llama3.2:3b" {
		t.Fatalf("models = %+v, %v", models, err)
	}
	if c := p.Capabilities("qwen3:8b"); c.Source != provider.CapsUnknown {
		t.Errorf("caps before Describe = %+v", c)
	}
	c, err := p.Describe(ctx, "qwen3:8b")
	if err != nil || !c.Tools || !c.Reasoning || c.ContextWindow != 40960 || c.Source != provider.CapsReported {
		t.Errorf("qwen3 = %+v, %v", c, err)
	}
	if c, _ := p.Describe(ctx, "gemma:2b"); c.Tools || c.Source != provider.CapsReported {
		t.Errorf("gemma = %+v", c)
	}
	if c, _ := p.Describe(ctx, "old"); c.Source != provider.CapsUnknown {
		t.Errorf("old server without capabilities = %+v", c)
	}
	if _, err := p.Describe(ctx, "nope"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("unknown model err = %v", err)
	}
	if p.Capabilities("qwen3:8b").ContextWindow != 40960 {
		t.Error("Describe result not cached")
	}
}

func TestChatWithToolCallsAndThinking(t *testing.T) {
	var sent map[string]any
	srv := fake(t, []string{
		`{"message":{"role":"assistant","content":"","thinking":"I should read it."},"done":false}`,
		`{"message":{"role":"assistant","content":"Reading."},"done":false}`,
		`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"read_file","arguments":{"path":"go.mod"}}},{"function":{"index":1,"name":"list_dir","arguments":{}}}]},"done":false}`,
		`{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":321,"eval_count":45}`,
	}, &sent)
	defer srv.Close()
	p := New(Options{BaseURL: srv.URL})
	p.Describe(context.Background(), "qwen3:8b")

	ch, err := p.Stream(context.Background(), provider.ChatRequest{
		Model:  "qwen3:8b",
		System: "sys",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Parts: []provider.Part{{Type: provider.PartText, Text: "hi"}}},
			{Role: provider.RoleAssistant, Parts: []provider.Part{{Type: provider.PartToolUse, ToolCallID: "c9", ToolName: "grep", Input: json.RawMessage(`{"pattern":"x"}`)}}},
			{Role: provider.RoleUser, Parts: []provider.Part{{Type: provider.PartToolResult, ToolCallID: "c9", Text: "no matches"}}},
		},
		Tools: []provider.ToolSpec{{Name: "read_file", InputSchema: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var done provider.Event
	var types []string
	for ev := range ch {
		types = append(types, string(ev.Type))
		if ev.Type == provider.EventError {
			t.Fatal(ev.Err)
		}
		if ev.Type == provider.EventDone {
			done = ev
		}
	}
	if got := strings.Join(types, ","); got != "thinking_delta,text_delta,tool_call_start,tool_call_start,done" {
		t.Errorf("events = %s", got)
	}
	calls := done.Message.ToolCalls()
	if len(calls) != 2 || calls[0].ToolCallID == calls[1].ToolCallID || string(calls[0].Input) != `{"path":"go.mod"}` {
		t.Fatalf("calls = %+v", calls)
	}
	if done.StopReason != provider.StopToolUse || done.Usage.InputTokens != 321 || done.Usage.OutputTokens != 45 {
		t.Errorf("stop = %s usage = %+v", done.StopReason, done.Usage)
	}
	if sent["think"] != true {
		t.Error("think not requested for a thinking model")
	}
	msgs := sent["messages"].([]any)
	if tool := msgs[3].(map[string]any); tool["role"] != "tool" || tool["tool_name"] != "grep" {
		t.Errorf("tool result message = %v", tool)
	}
	if call := msgs[2].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any); call["arguments"].(map[string]any)["pattern"] != "x" {
		t.Errorf("tool call arguments not sent as an object: %v", call)
	}
}

func TestChatError(t *testing.T) {
	srv := fake(t, []string{`{"error":"model requires more system memory"}`}, nil)
	defer srv.Close()
	ch, _ := New(Options{BaseURL: srv.URL}).Stream(context.Background(), provider.ChatRequest{Model: "big"})
	var err error
	for ev := range ch {
		if ev.Type == provider.EventError {
			err = ev.Err
		}
	}
	if err == nil || !strings.Contains(err.Error(), "memory") {
		t.Errorf("err = %v", err)
	}
	_, err = New(Options{BaseURL: "http://127.0.0.1:1"}).ListModels(context.Background())
	if err == nil || !strings.Contains(err.Error(), "is it running") {
		t.Errorf("connection refused err = %v", err)
	}
}

// SES-015: a user message's picture goes out in this API's own shape.
func TestImagesInRequests(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ = io.ReadAll(r.Body)
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	msg := provider.Message{Role: provider.RoleUser, Parts: []provider.Part{
		{Type: provider.PartText, Text: "what is this?"},
		{Type: provider.PartImage, Image: &provider.Image{Name: "x.png", MediaType: "image/png", Data: []byte("PNGDATA")}},
	}}
	ch, err := New(Options{BaseURL: srv.URL}).Stream(context.Background(), provider.ChatRequest{Model: "m", Messages: []provider.Message{msg}})
	if err == nil {
		for range ch {
		}
	}
	for _, want := range []string{`"images":["UE5HREFUQQ=="]`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("request lacks %s:\n%s", want, body)
		}
	}
}
