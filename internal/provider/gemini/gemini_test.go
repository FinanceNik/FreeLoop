package gemini

import (
	"context"
	"encoding/base64"
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

var sig = base64.StdEncoding.EncodeToString([]byte("sig-abc"))

// A thinking, text and tool-calling response in the shape the Gemini API
// streams it; the call carries a thought signature and no ID.
var toolStream = []string{
	`{"candidates":[{"content":{"role":"model","parts":[{"text":"Need the file.","thought":true}]}}]}`,
	`{"candidates":[{"content":{"role":"model","parts":[{"text":"Reading "}]}}]}`,
	`{"candidates":[{"content":{"role":"model","parts":[{"text":"it."}]}}]}`,
	`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"read_file","args":{"path":"go.mod"}},"thoughtSignature":"` + sig + `"}]},"finishReason":"STOP"}],` +
		`"usageMetadata":{"promptTokenCount":120,"cachedContentTokenCount":100,"candidatesTokenCount":30,"thoughtsTokenCount":12,"totalTokenCount":162}}`,
}

func TestStreamToolUse(t *testing.T) {
	var bodies []map[string]any
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Goog-Api-Key") != "test-key" {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT"}}`)
			return
		}
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, e := range toolStream {
			fmt.Fprintf(w, "data: %s\n\n", e)
		}
	}))
	defer srv.Close()

	p := New(Options{ID: "g", BaseURL: srv.URL, APIKey: "test-key"})
	req := provider.ChatRequest{
		Model:    "gemini-3-pro",
		System:   "sys",
		Effort:   "low",
		Messages: []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{{Type: provider.PartText, Text: "hi"}}}},
		Tools: []provider.ToolSpec{{Name: "read_file", Description: "Read", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"},
		}}},
	}
	done := collect(t, p, req)
	if !strings.HasPrefix(paths[0], "/v1beta/models/gemini-3-pro:streamGenerateContent") || !strings.Contains(paths[0], "alt=sse") {
		t.Errorf("path = %s", paths[0])
	}
	if done.StopReason != provider.StopToolUse {
		t.Errorf("stop = %s", done.StopReason)
	}
	if done.Usage != (provider.Usage{InputTokens: 20, OutputTokens: 42, CacheReadTokens: 100}) {
		t.Errorf("usage = %+v", done.Usage)
	}
	parts := done.Message.Parts
	if len(parts) != 3 || parts[0].Type != provider.PartThinking || parts[1].Text != "Reading it." {
		t.Fatalf("parts = %+v", parts)
	}
	call := parts[2]
	if call.Type != provider.PartToolUse || call.ToolName != "read_file" || string(call.Input) != `{"path":"go.mod"}` || !strings.HasPrefix(call.ToolCallID, madeUp) || call.Provider != Type {
		t.Errorf("call = %+v", call)
	}

	body := bodies[0]
	if s, _ := json.Marshal(body["systemInstruction"]); !strings.Contains(string(s), `"text":"sys"`) {
		t.Errorf("system = %s", s)
	}
	gc, _ := json.Marshal(body["generationConfig"])
	if !strings.Contains(string(gc), `"includeThoughts":true`) || !strings.Contains(string(gc), `"thinkingLevel":"LOW"`) {
		t.Errorf("generationConfig = %s", gc)
	}
	if tools, _ := json.Marshal(body["tools"]); !strings.Contains(string(tools), `"name":"read_file"`) || !strings.Contains(string(tools), `"parametersJsonSchema"`) {
		t.Errorf("tools = %s", tools)
	}

	// The next request sends the call back with its signature and the
	// result under the function's name, without the made-up ID.
	req.Messages = append(req.Messages, *done.Message, provider.Message{Role: provider.RoleUser, Parts: []provider.Part{
		{Type: provider.PartToolResult, ToolCallID: call.ToolCallID, Text: "module x"},
	}})
	collect(t, p, req)
	contents, _ := json.Marshal(bodies[1]["contents"])
	for _, want := range []string{
		`{"parts":[{"text":"hi"}],"role":"user"}`,
		`{"text":"Reading it."}`,
		`{"functionCall":{"args":{"path":"go.mod"},"name":"read_file"},"thoughtSignature":"` + sig + `"}`,
		`{"functionResponse":{"name":"read_file","response":{"output":"module x"}}}`,
	} {
		if !strings.Contains(string(contents), want) {
			t.Errorf("contents lack %s:\n%s", want, contents)
		}
	}
	if strings.Contains(string(contents), "Need the file.") {
		t.Error("thought text was sent back")
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
		case provider.EventUsage:
			// A running total, in the same terms as the final count (PRV-008).
			if ev.Usage != (provider.Usage{InputTokens: 20, OutputTokens: 42, CacheReadTokens: 100}) {
				t.Errorf("reported usage = %+v", ev.Usage)
			}
		case provider.EventDone:
			done = ev
		case provider.EventError:
			t.Fatal(ev.Err)
		}
	}
	want := []provider.EventType{provider.EventThinkingDelta, provider.EventTextDelta, provider.EventTextDelta, provider.EventUsage, provider.EventToolCallStart, provider.EventToolArgsDelta, provider.EventDone}
	if !slices.Equal(types, want) {
		t.Errorf("events = %v", types)
	}
	return done
}

// Errors name the cause and carry the status for the harness's retries;
// without a key nothing is sent.
func TestErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("X-Goog-Api-Key") == "bad" {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT"}}`)
			return
		}
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"error":{"code":429,"message":"Resource exhausted","status":"RESOURCE_EXHAUSTED"}}`)
	}))
	defer srv.Close()
	req := provider.ChatRequest{Model: "gemini-3-pro", Messages: []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{{Type: provider.PartText, Text: "hi"}}}}}

	ch, _ := New(Options{BaseURL: srv.URL, APIKey: "bad"}).Stream(context.Background(), req)
	if ev := <-ch; ev.Type != provider.EventError || !strings.Contains(ev.Err.Error(), "rejected the API key") {
		t.Errorf("bad key: %+v", ev)
	}
	ch, _ = New(Options{BaseURL: srv.URL, APIKey: "ok"}).Stream(context.Background(), req)
	ev := <-ch
	var he *provider.HTTPError
	if retry, _ := provider.Retryable(ev.Err); !errors.As(ev.Err, &he) || he.Status != 429 || !retry {
		t.Errorf("429: %+v", ev.Err)
	}
	t.Setenv("GEMINI_API_KEY", "from-the-environment")
	if _, err := New(Options{BaseURL: srv.URL}).Stream(context.Background(), req); err == nil || !strings.Contains(err.Error(), "needs an API key") {
		t.Errorf("without a key: %v", err)
	}
}

func TestListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"models":[`+
			`{"name":"models/gemini-3-pro","displayName":"Gemini 3 Pro","supportedGenerationMethods":["generateContent","countTokens"]},`+
			`{"name":"models/gemini-2.5-flash","displayName":"Gemini 2.5 Flash","supportedGenerationMethods":["generateContent"]},`+
			`{"name":"models/gemini-embedding-001","supportedGenerationMethods":["embedContent"]},`+
			`{"name":"models/imagen-4","supportedGenerationMethods":["predict"]}]}`)
	}))
	defer srv.Close()
	list, err := New(Options{BaseURL: srv.URL, APIKey: "k"}).ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range list {
		ids = append(ids, m.ID+"="+m.DisplayName)
	}
	if !slices.Equal(ids, []string{"gemini-3-pro=Gemini 3 Pro", "gemini-2.5-flash=Gemini 2.5 Flash"}) {
		t.Errorf("models = %v", ids)
	}
}

// Calls without an ID get one unique across responses, so a session's
// history never repeats one; thinking budgets stay in each model's range.
func TestMadeUpIDsAndBudgets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"read_file","args":{}}}]},"finishReason":"STOP"}]}`)
	}))
	defer srv.Close()
	p := New(Options{BaseURL: srv.URL, APIKey: "k"})
	req := provider.ChatRequest{Model: "gemini-3-pro", Messages: []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{{Type: provider.PartText, Text: "hi"}}}}}
	var ids []string
	for range 2 {
		ch, err := p.Stream(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		for ev := range ch {
			if ev.Type == provider.EventDone {
				ids = append(ids, ev.Message.Parts[0].ToolCallID)
			}
		}
	}
	if len(ids) != 2 || ids[0] == ids[1] {
		t.Errorf("ids = %v", ids)
	}
	for model, want := range map[string]int32{"gemini-2.5-flash": 24576, "gemini-2.5-pro": 32768} {
		for _, e := range []string{"xhigh", "max"} {
			if b := thinking(model, e).ThinkingBudget; b == nil || *b != want {
				t.Errorf("%s %s budget = %v", model, e, b)
			}
		}
	}
}

// Review finding: a null candidate or part crashed the app, the first
// inside the SDK; both now fail or skip without a panic.
func TestNullCandidatesAndParts(t *testing.T) {
	for _, body := range []string{
		`{"candidates":[{"content":{"role":"model","parts":[null,{"text":"ok"}]},"finishReason":"STOP"}]}`,
		`{"candidates":[null]}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: %s\n\n", body)
		}))
		ch, err := New(Options{BaseURL: srv.URL, APIKey: "k"}).Stream(context.Background(), provider.ChatRequest{Model: "gemini-3-pro",
			Messages: []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{{Type: provider.PartText, Text: "hi"}}}}})
		if err != nil {
			t.Fatal(err)
		}
		var last provider.Event
		for ev := range ch {
			last = ev
		}
		if last.Type != provider.EventDone && last.Type != provider.EventError {
			t.Errorf("%s: last event %+v", body, last)
		}
		srv.Close()
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
	ch, err := New(Options{BaseURL: srv.URL, APIKey: "k"}).Stream(context.Background(), provider.ChatRequest{Model: "gemini-3-pro", Messages: []provider.Message{msg}})
	if err == nil {
		for range ch {
		}
	}
	for _, want := range []string{`"inlineData":{"data":"UE5HREFUQQ==","mimeType":"image/png"}`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("request lacks %s:\n%s", want, body)
		}
	}
}
