package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

func modelContexts(t *testing.T, e *Engine, sid string) []event.ModelContextData {
	t.Helper()
	evs, err := e.Events(context.Background(), sid, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []event.ModelContextData
	for _, ev := range evs {
		if ev.Type == event.ModelContext {
			var d event.ModelContextData
			json.Unmarshal(ev.Data, &d)
			out = append(out, d)
		}
	}
	return out
}

// NFR-PRIV-003: what a model call carries besides the conversation is in the transcript,
// once per change and kind of call: the system prompt with the project's instructions,
// the tools, the model; a secret in it is redacted; and the app's own calls for the
// session, such as a commit message, say what else they sent.
func TestWhatTheModelSeesIsRecorded(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	const secret = "ghp_planted0123456789abcdef"
	if _, err := e.SaveEnvironment(SaveEnvironmentRequest{Name: "Work", Secrets: map[string]string{"GITHUB_TOKEN": secret}}); err != nil {
		t.Fatal(err)
	}
	withScript(e, "s", scripted.Step{Text: "ok"})
	root := gitRepo(t)
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Use tabs. The deploy token is "+secret+".\n"), 0o644)
	if _, err := e.SetProjectTrust(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m", EnvironmentID: "work"})
	if err != nil {
		t.Fatal(err)
	}
	ch, cancel := e.Subscribe()
	defer cancel()
	turn := func(text string) {
		t.Helper()
		if _, err := e.Prompt(ctx, ses.ID, text); err != nil {
			t.Fatal(err)
		}
		waitIdle(t, ch, ses.ID)
	}

	turn("first")
	got := modelContexts(t, e, ses.ID)
	if len(got) != 1 {
		t.Fatalf("after the first turn: %d records", len(got))
	}
	first := got[0]
	names := func(d event.ModelContextData) []string {
		var n []string
		for _, tl := range d.Tools {
			n = append(n, tl.Name)
		}
		return n
	}
	if first.Purpose != "turn" || first.Model != "m" || first.ProviderID != "s" ||
		!strings.Contains(first.System, "built-in coding agent of Freeloop") || !strings.Contains(first.System, "Use tabs.") ||
		!slices.Contains(names(first), "write_file") || len(first.Tools[0].InputSchema) == 0 {
		t.Errorf("the first record: %+v", first)
	}
	if strings.Contains(first.System, secret) {
		t.Error("a secret in AGENTS.md was recorded as it is")
	}

	turn("second")
	if got := modelContexts(t, e, ses.ID); len(got) != 1 {
		t.Errorf("a turn that changed nothing was recorded again: %d records", len(got))
	}

	if err := e.SetMode(ctx, ses.ID, "plan"); err != nil {
		t.Fatal(err)
	}
	turn("third")
	got = modelContexts(t, e, ses.ID)
	if len(got) != 2 || !strings.Contains(got[1].System, "# Mode: plan") {
		t.Fatalf("after plan mode: %+v", got)
	}

	// The app's own call for the session: a commit message, from the session's diff.
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("changed\n"), 0o644)
	if _, err := e.SuggestCommitMessage(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	got = modelContexts(t, e, ses.ID)
	last := got[len(got)-1]
	if last.Purpose != "commit_message" || !strings.Contains(last.System, "git commit messages") || !strings.HasPrefix(last.Sent, "The session's diff (") || len(last.Tools) != 0 {
		t.Errorf("the commit message's record: %+v", last)
	}
}
