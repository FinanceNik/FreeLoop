package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// HRN-012: switching profile starts a new session told about the old one;
// its first turn carries the handoff, so the model can answer about work
// done before; the old session says where it continued.
func TestSwitchProfile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	// The scripted provider answers by the number of assistant messages
	// sent, so the one-message summary request gets the first step.
	withScript(e, "s1", scripted.Step{Text: "You fixed the tokenizer bug in parse.go; the tests pass."})
	other := scripted.New("s2", scripted.Step{Text: "We fixed the tokenizer."})
	e.mu.Lock()
	e.providers.built["s2"] = &configuredProvider{cfg: config.Provider{ID: "s2", Name: "s2", Type: "scripted"}, p: other}
	e.mu.Unlock()
	os.MkdirAll(filepath.Join(dir, config.ProfilesDir), 0o700)
	os.WriteFile(filepath.Join(dir, config.ProfilesDir, "reviewer.yaml"), []byte("name: Reviewer\nprovider: s2\nmodel: m2\n"), 0o600)
	e.loadProfiles()

	root := t.TempDir()
	src, err := e.CreateSession(ctx, CreateSessionRequest{Title: "Parser", ProjectRoot: root, ProviderID: "s1", Model: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, src.ID, "Fix the tokenizer bug")
	waitIdle(t, ch, src.ID)

	ses, err := e.SwitchProfile(ctx, src.ID, "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if ses.ID == src.ID || ses.Title != "Parser · Reviewer" || ses.ProfileID != "reviewer" || ses.ProviderID != "s2" || ses.ProjectRoot != src.ProjectRoot {
		t.Fatalf("new session = %+v", ses)
	}
	e.Prompt(ctx, ses.ID, "What did we fix?")
	waitIdle(t, ch, ses.ID)
	req := other.Requests()[0]
	var sent strings.Builder
	for _, p := range req.Messages[len(req.Messages)-1].Parts {
		sent.WriteString(p.Text)
	}
	if !strings.Contains(sent.String(), `name="Handoff from Parser"`) || !strings.Contains(sent.String(), "tokenizer bug in parse.go") || !strings.Contains(sent.String(), "What did we fix?") {
		t.Errorf("the first turn sent %q", sent.String())
	}
	// The handoff goes with the first prompt only.
	e.Prompt(ctx, ses.ID, "And then?")
	waitIdle(t, ch, ses.ID)
	if n := len(other.Requests()); n != 2 {
		t.Fatalf("calls = %d", n)
	}
	last := other.Requests()[1].Messages
	if p := last[len(last)-1].Parts; len(p) != 1 {
		t.Errorf("the second prompt carried %d parts", len(p))
	}
	evs, _ := e.Events(ctx, src.ID, 0)
	if ev := evs[len(evs)-1]; ev.Type != event.Notice || !strings.Contains(string(ev.Data), "Reviewer") {
		t.Errorf("the old session ends with %s %s", ev.Type, ev.Data)
	}
}

// Without a built-in model on either side, the end of the transcript is
// the handoff.
func TestHandoffWithoutAModel(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	long := strings.Repeat("x", handoffExcerpt) + "THE END"
	text, usage, generated := e.handoffSummary(context.Background(), session.Session{HarnessID: "claude-code"}, "claude-code", "", "", long)
	if generated || usage != nil || !strings.HasSuffix(text, "THE END") || len(text) > handoffExcerpt+100 {
		t.Errorf("handoff = %d bytes, generated %v", len(text), generated)
	}
}
