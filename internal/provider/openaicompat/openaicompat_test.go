package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/provider"
)

func server(t *testing.T, chunks []string, check func(body map[string]any, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			io.WriteString(w, `{"data":[{"id":"b-model"},{"id":"a-model"}]}`)
		case "/v1/chat/completions":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if check != nil {
				check(body, r)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			for _, c := range chunks {
				fmt.Fprintf(w, "data: %s\n\n", c)
			}
			io.WriteString(w, "data: [DONE]\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
}

func collect(t *testing.T, p *Provider, req provider.ChatRequest) ([]provider.EventType, provider.Event) {
	t.Helper()
	ch, err := p.Stream(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var types []provider.EventType
	var done provider.Event
	for ev := range ch {
		types = append(types, ev.Type)
		if ev.Type == provider.EventError {
			t.Fatal(ev.Err)
		}
		if ev.Type == provider.EventDone {
			done = ev
		}
	}
	return types, done
}

var req = provider.ChatRequest{
	Model:  "m",
	System: "sys",
	Messages: []provider.Message{
		{Role: provider.RoleUser, Parts: []provider.Part{{Type: provider.PartText, Text: "hi"}}},
		{Role: provider.RoleAssistant, Parts: []provider.Part{
			{Type: provider.PartThinking, Text: "secret reasoning", Provider: "anthropic"},
			{Type: provider.PartText, Text: "reading"},
			{Type: provider.PartToolUse, ToolCallID: "c0", ToolName: "read_file", Input: json.RawMessage(`{"path":"a"}`)},
		}},
		{Role: provider.RoleUser, Parts: []provider.Part{
			{Type: provider.PartToolResult, ToolCallID: "c0", Text: "contents"},
			{Type: provider.PartText, Text: "and then?"},
		}},
	},
	Tools: []provider.ToolSpec{{Name: "read_file", Description: "Read", InputSchema: map[string]any{"type": "object"}}},
}

// OpenAI's own shape: ids on the first delta, arguments streamed by index.
func TestStreamOpenAIShape(t *testing.T) {
	var sent map[string]any
	var auth, title string
	srv := server(t, []string{
		`{"choices":[{"delta":{"role":"assistant","content":"Let me "}}]}`,
		`{"choices":[{"delta":{"content":"look."}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"read_file","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"go.mod\"}"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"list_dir","arguments":"{}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"prompt_tokens_details":{"cached_tokens":60}}}`,
	}, func(body map[string]any, r *http.Request) {
		sent, auth, title = body, r.Header.Get("Authorization"), r.Header.Get("X-Title")
	})
	defer srv.Close()

	p := New(Options{ID: "or", Preset: "openrouter", BaseURL: srv.URL + "/v1", APIKey: "k"})
	types, done := collect(t, p, req)
	want := "text_delta,text_delta,tool_call_start,tool_args_delta,tool_args_delta,tool_call_start,tool_args_delta,done"
	if got := join(types); got != want {
		t.Errorf("events = %s\nwant      %s", got, want)
	}
	calls := done.Message.ToolCalls()
	if len(calls) != 2 || calls[0].ToolCallID != "call_a" || string(calls[0].Input) != `{"path":"go.mod"}` || calls[1].ToolName != "list_dir" {
		t.Fatalf("calls = %+v", calls)
	}
	if done.StopReason != provider.StopToolUse || done.Usage.InputTokens != 40 || done.Usage.CacheReadTokens != 60 || done.Usage.OutputTokens != 20 {
		t.Errorf("stop = %s usage = %+v", done.StopReason, done.Usage)
	}
	if auth != "Bearer k" || title != "Freeloop" {
		t.Errorf("headers: auth %q, X-Title %q", auth, title)
	}

	// History conversion: system first, no foreign reasoning, tool result
	// as a tool message, trailing text as a user message.
	msgs := sent["messages"].([]any)
	roles := []string{}
	for _, m := range msgs {
		roles = append(roles, m.(map[string]any)["role"].(string))
	}
	if strings.Join(roles, ",") != "system,user,assistant,tool,user" {
		t.Errorf("roles = %v", roles)
	}
	b, _ := json.Marshal(msgs)
	if strings.Contains(string(b), "secret reasoning") {
		t.Error("reasoning sent back")
	}
	if tm := msgs[3].(map[string]any); tm["tool_call_id"] != "c0" || tm["content"] != "contents" {
		t.Errorf("tool message = %v", tm)
	}
	if sent["stream_options"] == nil {
		t.Error("usage not requested")
	}
}

// Ollama /v1 and some others: no ids, every call at index 0, complete.
func TestStreamToleratesMissingIDsAndIndexReuse(t *testing.T) {
	srv := server(t, []string{
		`{"choices":[{"delta":{"reasoning_content":"thinking..."}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"read_file","arguments":"{\"path\":\"a\"}"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"x2","function":{"name":"read_file","arguments":"{\"path\":\"b\"}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}, nil)
	defer srv.Close()
	p := New(Options{Preset: "ollama-v1", BaseURL: srv.URL + "/v1"})
	_, done := collect(t, p, req)
	calls := done.Message.ToolCalls()
	if len(calls) != 2 {
		t.Fatalf("calls = %+v", calls)
	}
	if calls[0].ToolCallID == "" || calls[0].ToolCallID == calls[1].ToolCallID {
		t.Errorf("ids = %q, %q", calls[0].ToolCallID, calls[1].ToolCallID)
	}
	if string(calls[1].Input) != `{"path":"b"}` {
		t.Errorf("second call = %s", calls[1].Input)
	}
	if done.StopReason != provider.StopToolUse {
		t.Errorf("stop = %s; tool calls with finish_reason stop must still be tool_use", done.StopReason)
	}
	if done.Message.Parts[0].Type != provider.PartThinking || done.Message.Parts[0].Text != "thinking..." {
		t.Errorf("reasoning = %+v", done.Message.Parts[0])
	}
}

func TestListModelsAndErrors(t *testing.T) {
	srv := server(t, nil, nil)
	defer srv.Close()
	p := New(Options{Preset: "custom", BaseURL: srv.URL + "/v1"})
	models, err := p.ListModels(context.Background())
	if err != nil || len(models) != 2 || models[0].ID != "a-model" {
		t.Fatalf("models = %+v, %v", models, err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"message":"Incorrect API key provided"}}`)
	}))
	defer bad.Close()
	_, err = New(Options{Preset: "openai", BaseURL: bad.URL}).ListModels(context.Background())
	if err == nil || !strings.Contains(err.Error(), "API key") || !strings.Contains(err.Error(), "Incorrect") {
		t.Errorf("err = %v", err)
	}

	_, err = New(Options{Preset: "lmstudio", BaseURL: "http://127.0.0.1:1"}).ListModels(context.Background())
	if err == nil || !strings.Contains(err.Error(), "is the server running") {
		t.Errorf("connection refused err = %v", err)
	}
}

func TestCapabilitiesByPreset(t *testing.T) {
	if c := New(Options{Preset: "openai"}).Capabilities("gpt-x"); !c.Tools || c.Source != provider.CapsStatic {
		t.Errorf("openai caps = %+v", c)
	}
	if c := New(Options{Preset: "lmstudio"}).Capabilities("qwen"); c.Tools || c.Source != provider.CapsUnknown {
		t.Errorf("local caps = %+v", c)
	}
}

func TestProbe(t *testing.T) {
	yes := server(t, []string{`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"p","function":{"name":"ping","arguments":"{\"value\":\"ok\"}"}}]},"finish_reason":"tool_calls"}]}`}, nil)
	defer yes.Close()
	no := server(t, []string{`{"choices":[{"delta":{"content":"{\"name\": \"ping\"}"},"finish_reason":"stop"}]}`}, nil)
	defer no.Close()
	if ok, err := provider.ProbeTools(context.Background(), New(Options{BaseURL: yes.URL + "/v1"}), "m"); !ok || err != nil {
		t.Errorf("probe of a tool-calling model = %v, %v", ok, err)
	}
	if ok, err := provider.ProbeTools(context.Background(), New(Options{BaseURL: no.URL + "/v1"}), "m"); ok || err != nil {
		t.Errorf("probe of a model that writes the call as text = %v, %v", ok, err)
	}
}

func join(ts []provider.EventType) string {
	s := make([]string, len(ts))
	for i, t := range ts {
		s[i] = string(t)
	}
	return strings.Join(s, ",")
}

// PRV-011: the effort reaches OpenAI's reasoning models as
// reasoning_effort and OpenRouter as reasoning.effort, mapped onto low,
// medium and high; models and endpoints that do not take it get none.
func TestEffortInTheRequest(t *testing.T) {
	for _, c := range []struct {
		preset, model, effort string
		want                  map[string]any
	}{
		{"openai", "o3", "max", map[string]any{"reasoning_effort": "high"}},
		{"openai", "gpt-5-mini", "low", map[string]any{"reasoning_effort": "low"}},
		{"openai", "gpt-4o", "high", nil},
		{"openai", "gpt-5-chat-latest", "high", nil},
		{"openrouter", "anthropic/claude-x", "medium", map[string]any{"reasoning": map[string]any{"effort": "medium"}}},
		{"vllm", "qwen", "high", nil},
	} {
		var sent map[string]any
		srv := server(t, []string{`{"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`}, func(body map[string]any, r *http.Request) { sent = body })
		p := New(Options{ID: "x", Preset: c.preset, BaseURL: srv.URL + "/v1", APIKey: "k"})
		r := req
		r.Model, r.Effort = c.model, c.effort
		collect(t, p, r)
		srv.Close()
		got := map[string]any{}
		for _, k := range []string{"reasoning_effort", "reasoning"} {
			if v, ok := sent[k]; ok {
				got[k] = v
			}
		}
		if len(got) != len(c.want) || fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s %s %s: sent %v, want %v", c.preset, c.model, c.effort, got, c.want)
		}
		if caps := p.Capabilities(c.model); caps.Effort != (c.want != nil) {
			t.Errorf("%s %s: caps.Effort = %v", c.preset, c.model, caps.Effort)
		}
	}
}

// Review finding: a negative tool call index indexed the call list and
// crashed the app.
func TestStreamNegativeIndex(t *testing.T) {
	srv := server(t, []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":-1,"id":"a","function":{"name":"read_file","arguments":"{}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
	}, nil)
	defer srv.Close()
	_, done := collect(t, New(Options{Preset: "ollama-v1", BaseURL: srv.URL + "/v1"}), req)
	if calls := done.Message.ToolCalls(); len(calls) != 1 || calls[0].ToolCallID != "a" {
		t.Errorf("calls = %+v", calls)
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
	ch, err := New(Options{Preset: "custom", BaseURL: srv.URL + "/v1"}).Stream(context.Background(), provider.ChatRequest{Model: "m", Messages: []provider.Message{msg}})
	if err == nil {
		for range ch {
		}
	}
	for _, want := range []string{`"type":"image_url"`, `"url":"data:image/png;base64,UE5HREFUQQ=="`, `{"text":"what is this?","type":"text"}`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("request lacks %s:\n%s", want, body)
		}
	}
}
