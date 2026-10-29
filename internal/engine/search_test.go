package engine

import (
	"context"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/store"
)

// SES-007: a phrase from a built-in session's tool result and from its
// streamed reply is found, grouped under the session.
func TestSearchAcrossSessions(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	p := scripted.New("sp",
		scripted.Step{Calls: []scripted.Call{{Name: "write_file", Input: map[string]any{"path": "notes.txt", "content": "the migration needs a lighthouse flag\n"}}}},
		scripted.Step{Text: "Wrote the lighthouse note."})
	e.mu.Lock()
	e.providers.built["sp"] = &configuredProvider{cfg: config.Provider{ID: "sp", Name: "sp", Type: "scripted"}, p: p}
	e.mu.Unlock()
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "sp", Model: "m", Mode: "accept_edits"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, ses.ID, "Write down the flag")
	waitIdle(t, ch, ses.ID)

	res, err := e.Search(ctx, "lighthouse")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].SessionID != ses.ID || len(res[0].Hits) != 2 {
		t.Fatalf("results = %+v", res)
	}
	kinds := map[string]bool{}
	for _, h := range res[0].Hits {
		kinds[h.Kind] = true
	}
	if !kinds[store.HitTool] || !kinds[store.HitAgent] {
		t.Errorf("kinds = %v", kinds)
	}
	if res, _ := e.Search(ctx, `"needs a lighthouse flag"`); len(res) != 1 {
		t.Errorf("phrase = %+v", res)
	}
}
