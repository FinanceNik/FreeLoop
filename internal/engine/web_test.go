package engine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/netguard"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// HRN-017: pages come back as text; private addresses, other schemes and
// binary files do not; local-only refuses.
func TestWebFetch(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, `<title>Docs</title><script>evil()</script><p>Hello &amp; welcome</p>`)
		case "/big":
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, strings.Repeat("x", maxPageText+10))
		case "/app":
			w.Header().Set("Content-Type", "application/zip")
			io.WriteString(w, "PK")
		case "/gone":
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	guarded := sessionWeb{e: e}
	for _, bad := range []string{srv.URL + "/page", "http://localhost/", "http://[::1]/", "http://169.254.169.254/latest/", "file:///etc/passwd", "ftp://example.com/", "https://user:pw@example.com/"} {
		if _, err := guarded.Fetch(ctx, bad); err == nil {
			t.Errorf("fetched %s", bad)
		}
	}
	w := sessionWeb{e: e, client: srv.Client()}
	p, err := w.Fetch(ctx, srv.URL+"/page")
	if err != nil || p.Title != "Docs" || p.Text != "Hello & welcome" || p.Truncated {
		t.Errorf("page = %+v, %v", p, err)
	}
	if p, err := w.Fetch(ctx, srv.URL+"/big"); err != nil || !p.Truncated || len(p.Text) != maxPageText {
		t.Errorf("big page: %d bytes, truncated %v, %v", len(p.Text), p.Truncated, err)
	}
	for _, path := range []string{"/app", "/gone"} {
		if _, err := w.Fetch(ctx, srv.URL+path); err == nil {
			t.Errorf("fetched %s", path)
		}
	}
	netguard.SetLocalOnly(true)
	defer netguard.SetLocalOnly(false)
	if _, err := w.Fetch(ctx, srv.URL+"/page"); err == nil || !strings.Contains(err.Error(), "nothing but this computer") {
		t.Errorf("local-only: %v", err)
	}
}

// web_search uses the configured endpoint, with the key from the Keychain.
func TestWebSearch(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	var gotQuery, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery, gotKey = r.URL.Query().Get("q"), r.Header.Get("X-Key")
		io.WriteString(w, `{"results":[{"title":"Go","url":"https://go.dev","content":"The language"}]}`)
	}))
	defer srv.Close()
	w := sessionWeb{e: e}
	if w.CanSearch() {
		t.Error("search offered without an endpoint")
	}
	cfg := e.Config()
	cfg.Web.SearchURL, cfg.Web.SearchHeader = srv.URL+"/search?format=json&q={query}", "X-Key"
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	if err := e.SetWebSearchKey("sekret-value"); err != nil {
		t.Fatal(err)
	}
	hits, err := w.Search(ctx, "go & rust")
	if err != nil || !w.CanSearch() || len(hits) != 1 || hits[0].URL != "https://go.dev" {
		t.Fatalf("hits = %+v, %v", hits, err)
	}
	if gotQuery != "go & rust" || gotKey != "sekret-value" {
		t.Errorf("query %q, key %q", gotQuery, gotKey)
	}
}

// Every fetch asks, "Allow always" covers that exact address only, plan
// mode refuses, and a to-do list asks nothing and reaches the transcript.
func TestWebToolsThroughTheBroker(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	url := "https://example.invalid/docs"
	p := scriptedProvider(e, "web",
		scripted.Step{Calls: []scripted.Call{{Name: "todo_write", Input: map[string]any{"items": []map[string]any{{"content": "Read the docs", "status": "in_progress"}}}}}},
		scripted.Step{Calls: []scripted.Call{{Name: "web_fetch", Input: map[string]any{"url": url}}}},
		scripted.Step{Text: "done"})
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "web", Model: "m", Mode: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, ses.ID, "go")
	ev := waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested })
	var req permission.Request
	json.Unmarshal(ev.Data, &req)
	if req.Tool != "web_fetch" || req.Command != url || req.Kind != permission.KindFetch {
		t.Fatalf("request = %+v", req)
	}
	if err := e.Decide(DecideRequest{RequestID: req.ID, OptionID: "allow_always"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)

	specs := p.Requests()[0].Tools
	names := []string{}
	for _, s := range specs {
		names = append(names, s.Name)
	}
	if !slices.Contains(names, "web_fetch") || !slices.Contains(names, "todo_write") || slices.Contains(names, "web_search") {
		t.Errorf("tools offered: %v", names)
	}
	evs, _ := e.Events(ctx, ses.ID, 0)
	var plan, rule string
	for _, ev := range evs {
		switch ev.Type {
		case event.Plan:
			plan = string(ev.Data)
		case event.PermissionDecided:
			rule = string(ev.Data)
		}
	}
	if !strings.Contains(plan, "Read the docs") || !strings.Contains(plan, "in_progress") {
		t.Errorf("plan event = %q", plan)
	}
	if !strings.Contains(rule, `"literal":true`) || !strings.Contains(rule, url) {
		t.Errorf("the saved rule is not that exact address: %s", rule)
	}
	r := permission.Rule{Effect: permission.Allow, Tool: "web_fetch", Command: url, Literal: true}
	if r.Matches(permission.Request{Tool: "web_fetch", Kind: permission.KindFetch, Command: url + "?secret=1"}, "/") {
		t.Error("the rule covers another address")
	}

	// Plan mode offers neither and refuses a call anyway.
	p2 := scriptedProvider(e, "webplan", scripted.Step{Calls: []scripted.Call{{Name: "web_fetch", Input: map[string]any{"url": url}}}}, scripted.Step{Text: "done"})
	planned, _ := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "webplan", Model: "m", Mode: "plan"})
	e.Prompt(ctx, planned.ID, "go")
	waitIdle(t, ch, planned.ID)
	for _, s := range p2.Requests()[0].Tools {
		if s.Name == "web_fetch" {
			t.Error("web_fetch offered in plan mode")
		}
	}
	evs, _ = e.Events(ctx, planned.ID, 0)
	for _, ev := range evs {
		if ev.Type == event.PermissionRequested {
			t.Error("plan mode asked instead of refusing")
		}
	}
}

func scriptedProvider(e *Engine, id string, steps ...scripted.Step) *scripted.Provider {
	p := scripted.New(id, steps...)
	e.mu.Lock()
	e.providers.built[id] = &configuredProvider{cfg: config.Provider{ID: id, Name: id, Type: "scripted"}, p: p}
	e.mu.Unlock()
	return p
}
