package anthropic

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

// A tool-calling response in the shape the Messages API streams it.
var toolUseStream = []string{
	`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],"stop_reason":null,"usage":{"input_tokens":120,"output_tokens":1,"cache_read_input_tokens":100,"cache_creation_input_tokens":0}}}`,
	`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Need the file."}}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-abc"}}`,
	`{"type":"content_block_stop","index":0}`,
	`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
	`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Reading it."}}`,
	`{"type":"content_block_stop","index":1}`,
	`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_1","name":"read_file","input":{}}}`,
	`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}`,
	`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"\"go.mod\"}"}}`,
	`{"type":"content_block_stop","index":2}`,
	`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":42}}`,
	`{"type":"message_stop"}`,
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
		if r.Header.Get("X-Api-Key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		sse(w, toolUseStream)
	}))
	defer srv.Close()

	p := New(Options{ID: "a", BaseURL: srv.URL, APIKey: "test-key"})
	req := provider.ChatRequest{
		Model:    "claude-opus-5-5",
		System:   "sys",
		Effort:   "high",
		Messages: []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{{Type: provider.PartText, Text: "hi"}}}},
		Tools: []provider.ToolSpec{{Name: "read_file", Description: "Read", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"},
		}}},
	}
	ch, err := p.Stream(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var types []provider.EventType
	var done provider.Event
	for ev := range ch {
		types = append(types, ev.Type)
		if ev.Type == provider.EventDone {
			done = ev
		}
		if ev.Type == provider.EventError {
			t.Fatal(ev.Err)
		}
	}
	want := "thinking_delta,text_delta,tool_call_start,tool_args_delta,tool_args_delta,done"
	if got := joinTypes(types); got != want {
		t.Errorf("events = %s\nwant      %s", got, want)
	}
	if done.StopReason != provider.StopToolUse {
		t.Errorf("stop = %s", done.StopReason)
	}
	if done.Usage.InputTokens != 120 || done.Usage.OutputTokens != 42 || done.Usage.CacheReadTokens != 100 {
		t.Errorf("usage = %+v", done.Usage)
	}
	calls := done.Message.ToolCalls()
	if len(calls) != 1 || calls[0].ToolCallID != "toolu_1" || string(calls[0].Input) != `{"path":"go.mod"}` {
		t.Fatalf("tool calls = %+v", calls)
	}

	// The request carried adaptive thinking, effort, eager tool streaming
	// and the system prompt.
	b := bodies[0]
	if th, _ := b["thinking"].(map[string]any); th["type"] != "adaptive" || th["display"] != "summarized" {
		t.Errorf("thinking = %v", b["thinking"])
	}
	if oc, _ := b["output_config"].(map[string]any); oc["effort"] != "high" {
		t.Errorf("output_config = %v", b["output_config"])
	}
	tool := b["tools"].([]any)[0].(map[string]any)
	if tool["eager_input_streaming"] != true {
		t.Errorf("tool = %v", tool)
	}

	// Sending the assistant message back keeps the thinking signature.
	req.Messages = append(req.Messages, *done.Message, provider.Message{Role: provider.RoleUser, Parts: []provider.Part{
		{Type: provider.PartToolResult, ToolCallID: "toolu_1", Text: "module x"},
	}})
	ch, _ = p.Stream(context.Background(), req)
	for range ch {
	}
	msgs := bodies[1]["messages"].([]any)
	asst := msgs[1].(map[string]any)["content"].([]any)
	th := asst[0].(map[string]any)
	if th["type"] != "thinking" || th["signature"] != "sig-abc" || th["thinking"] != "Need the file." {
		t.Errorf("thinking block sent back = %v", th)
	}
	if tu := asst[2].(map[string]any); tu["type"] != "tool_use" || tu["id"] != "toolu_1" {
		t.Errorf("tool_use sent back = %v", tu)
	}
	if tr := msgs[2].(map[string]any)["content"].([]any)[0].(map[string]any); tr["type"] != "tool_result" || tr["tool_use_id"] != "toolu_1" {
		t.Errorf("tool_result = %v", tr)
	}
}

func TestOtherProvidersThinkingIsDropped(t *testing.T) {
	msgs := toParams([]provider.Message{{Role: provider.RoleAssistant, Parts: []provider.Part{
		{Type: provider.PartThinking, Text: "x", Provider: "openai_compat"},
		{Type: provider.PartText, Text: "answer"},
	}}})
	b, _ := json.Marshal(msgs)
	if strings.Contains(string(b), "thinking") {
		t.Errorf("foreign thinking sent: %s", b)
	}
}

func TestReadableErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	}))
	defer srv.Close()
	p := New(Options{BaseURL: srv.URL, APIKey: "wrong"})
	_, err := p.ListModels(context.Background())
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("err = %v", err)
	}
}

func TestModern(t *testing.T) {
	for m, want := range map[string]bool{
		"claude-opus-5-5": true, "claude-sonnet-5-5": true, "claude-haiku-5-5": true, "claude-fable-5-1": true,
		"claude-opus-4-6": true, "claude-sonnet-4-6": true, "claude-haiku-4-5": false, "claude-sonnet-4-5": false,
	} {
		if modern(m) != want {
			t.Errorf("modern(%s) = %v", m, !want)
		}
	}
}

func joinTypes(ts []provider.EventType) string {
	s := make([]string, len(ts))
	for i, t := range ts {
		s[i] = string(t)
	}
	return strings.Join(s, ",")
}
