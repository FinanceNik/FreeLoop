package openairesponses

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/provider"
)

// A reasoning, text and tool-calling response in the shape the Responses
// API streams it.
var toolStream = []string{
	`{"type":"response.created","sequence_number":0,"response":{"id":"resp_1","object":"response","status":"in_progress","output":[]}}`,
	`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"rs_1","type":"reasoning","summary":[]}}`,
	`{"type":"response.reasoning_summary_text.delta","sequence_number":2,"item_id":"rs_1","output_index":0,"summary_index":0,"delta":"Need the file."}`,
	`{"type":"response.output_item.added","sequence_number":3,"output_index":1,"item":{"id":"msg_1","type":"message","role":"assistant","status":"in_progress","content":[]}}`,
	`{"type":"response.output_text.delta","sequence_number":4,"item_id":"msg_1","output_index":1,"content_index":0,"delta":"Reading it."}`,
	`{"type":"response.output_item.added","sequence_number":5,"output_index":2,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"read_file","arguments":"","status":"in_progress"}}`,
	`{"type":"response.function_call_arguments.delta","sequence_number":6,"item_id":"fc_1","output_index":2,"delta":"{\"path\":"}`,
	`{"type":"response.function_call_arguments.delta","sequence_number":7,"item_id":"fc_1","output_index":2,"delta":"\"go.mod\"}"}`,
	`{"type":"response.completed","sequence_number":8,"response":{"id":"resp_1","object":"response","status":"completed","output":[` +
		`{"id":"rs_1","type":"reasoning","summary":[{"type":"summary_text","text":"Need the file."}],"encrypted_content":"enc-abc"},` +
		`{"id":"msg_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Reading it.","annotations":[]}]},` +
		`{"id":"fc_1","type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"go.mod\"}","status":"completed"}` +
		`],"usage":{"input_tokens":120,"input_tokens_details":{"cached_tokens":100},"output_tokens":42,"output_tokens_details":{"reasoning_tokens":10},"total_tokens":162}}}`,
}

func sse(w http.ResponseWriter, events []string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, e := range events {
		var head struct{ Type string }
		json.Unmarshal([]byte(e), &head)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", head.Type, e)
	}
}

func TestStreamToolUse(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error":{"message":"Incorrect API key provided","type":"invalid_request_error"}}`)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		sse(w, toolStream)
	}))
	defer srv.Close()

	p := New(Options{ID: "o", BaseURL: srv.URL, APIKey: "test-key"})
	req := provider.ChatRequest{
		Model:    "gpt-5.5",
		System:   "sys",
		Effort:   "max",
		Messages: []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{{Type: provider.PartText, Text: "hi"}}}},
		Tools: []provider.ToolSpec{{Name: "read_file", Description: "Read", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"},
		}}},
	}
	done := collect(t, p, req)
	if done.StopReason != provider.StopToolUse {
		t.Errorf("stop = %s", done.StopReason)
	}
	if done.Usage != (provider.Usage{InputTokens: 20, OutputTokens: 42, CacheReadTokens: 100}) {
		t.Errorf("usage = %+v", done.Usage)
	}
	parts := done.Message.Parts
	if len(parts) != 3 || parts[0].Type != provider.PartThinking || parts[0].Text != "Need the file." || parts[1].Text != "Reading it." {
		t.Fatalf("parts = %+v", parts)
	}
	if c := parts[2]; c.Type != provider.PartToolUse || c.ToolCallID != "call_1" || c.ToolName != "read_file" || string(c.Input) != `{"path":"go.mod"}` {
		t.Errorf("call = %+v", c)
	}

	body := bodies[0]
	if body["instructions"] != "sys" || body["store"] != false || body["model"] != "gpt-5.5" {
		t.Errorf("body = %v", body)
	}
	if r, _ := body["reasoning"].(map[string]any); r["effort"] != "high" || r["summary"] != "auto" {
		t.Errorf("reasoning = %v", body["reasoning"])
	}
	if inc, _ := body["include"].([]any); !slices.Contains(inc, any("reasoning.encrypted_content")) {
		t.Errorf("include = %v", body["include"])
	}
	tool := body["tools"].([]any)[0].(map[string]any)
	if tool["type"] != "function" || tool["name"] != "read_file" || tool["strict"] != false {
		t.Errorf("tool = %v", tool)
	}

	// The next request sends the reasoning item back with its encrypted
	// content, the call and its result.
	req.Messages = append(req.Messages, *done.Message, provider.Message{Role: provider.RoleUser, Parts: []provider.Part{
		{Type: provider.PartToolResult, ToolCallID: "call_1", Text: "module x"},
	}})
	collect(t, p, req)
	input, _ := json.Marshal(bodies[1]["input"])
	for _, want := range []string{
		`{"content":"hi","role":"user"}`,
		`"encrypted_content":"enc-abc","id":"rs_1"`,
		`{"content":"Reading it.","role":"assistant"}`,
		`{"arguments":"{\"path\":\"go.mod\"}","call_id":"call_1","name":"read_file","type":"function_call"}`,
		`{"call_id":"call_1","output":"module x","type":"function_call_output"}`,
	} {
		if !strings.Contains(string(input), want) {
			t.Errorf("input lacks %s:\n%s", want, input)
		}
	}
}

func collect(t *testing.T, p *Provider, req provider.ChatRequest) provider.Event {
	t.Helper()
	ch, err := p.Stream(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var done provider.Event
	var types []provider.EventType
	for ev := range ch {
		types = append(types, ev.Type)
		switch ev.Type {
		case provider.EventDone:
			done = ev
		case provider.EventError:
			t.Fatal(ev.Err)
		}
	}
	want := []provider.EventType{provider.EventThinkingDelta, provider.EventTextDelta, provider.EventToolCallStart, provider.EventToolArgsDelta, provider.EventToolArgsDelta, provider.EventDone}
	if !slices.Equal(types, want) {
		t.Errorf("events = %v", types)
	}
	return done
}

// Errors name the cause and carry the status for the harness's retries.
func TestErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer bad":
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error":{"message":"Incorrect API key provided"}}`)
		default:
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"error":{"message":"slow down"}}`)
		}
	}))
	defer srv.Close()
	msgs := []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{{Type: provider.PartText, Text: "hi"}}}}

	ch, _ := New(Options{BaseURL: srv.URL, APIKey: "bad"}).Stream(context.Background(), provider.ChatRequest{Model: "gpt-5.5", Messages: msgs})
	ev := <-ch
	if ev.Type != provider.EventError || !strings.Contains(ev.Err.Error(), "rejected the API key") {
		t.Errorf("401: %+v", ev)
	}
	ch, _ = New(Options{BaseURL: srv.URL, APIKey: "ok"}).Stream(context.Background(), provider.ChatRequest{Model: "gpt-5.5", Messages: msgs})
	ev = <-ch
	var he *provider.HTTPError
	retry, wait := provider.Retryable(ev.Err)
	if !errors.As(ev.Err, &he) || he.Status != 429 || !retry || wait.Seconds() != 7 {
		t.Errorf("429: %+v, retry %v after %v", ev.Err, retry, wait)
	}
}

// The model list leaves out models that do not converse.
func TestListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"object":"list","data":[{"id":"gpt-5.5","object":"model"},{"id":"o4-mini","object":"model"},{"id":"text-embedding-3-large","object":"model"},{"id":"gpt-4o-realtime-preview","object":"model"},{"id":"dall-e-3","object":"model"}]}`)
	}))
	defer srv.Close()
	list, err := New(Options{BaseURL: srv.URL, APIKey: "k"}).ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range list {
		ids = append(ids, m.ID)
	}
	if !slices.Equal(ids, []string{"gpt-5.5", "o4-mini"}) {
		t.Errorf("models = %v", ids)
	}
}

// An error inside a stream is retried like its HTTP kind; a chat model
// gets no effort.
func TestStreamErrorsAndChatModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w, []string{
			`{"type":"response.created","sequence_number":0,"response":{"id":"r","object":"response","status":"in_progress","output":[]}}`,
			`{"type":"response.failed","sequence_number":1,"response":{"id":"r","object":"response","status":"failed","output":[],"error":{"code":"server_error","message":"The server had an error"}}}`,
		})
	}))
	defer srv.Close()
	ch, _ := New(Options{BaseURL: srv.URL, APIKey: "k"}).Stream(context.Background(), provider.ChatRequest{Model: "gpt-5.5",
		Messages: []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{{Type: provider.PartText, Text: "hi"}}}}})
	var last provider.Event
	for ev := range ch {
		last = ev
	}
	if retry, _ := provider.Retryable(last.Err); last.Type != provider.EventError || !retry {
		t.Errorf("server_error: %+v", last)
	}
	p := New(Options{APIKey: "k"})
	if p.Capabilities("gpt-5-chat-latest").Effort || !p.Capabilities("gpt-5.5").Effort {
		t.Error("effort offered to a chat model, or not to a reasoning one")
	}
}
