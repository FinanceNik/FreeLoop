package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// SES-008: the Markdown export shows the user's message, the agent's reply
// and each tool call with its input, output and the edit as a diff; the
// JSON export holds every event; both are written where the user chose.
func TestExport(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	withScript(e, "s",
		scripted.Step{Text: "Writing it.", Calls: []scripted.Call{{Name: "write_file", Input: map[string]any{"path": "a.go", "content": "package a\n\nfunc A() int { return 1 }\n"}}}},
		scripted.Step{Calls: []scripted.Call{{Name: "edit_file", Input: map[string]any{"path": "a.go", "old_string": "return 1", "new_string": "return 2"}}}},
		scripted.Step{Text: "A now returns 2."},
	)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{Title: "Change A", ProjectRoot: t.TempDir(), ProviderID: "s", Model: "m", Mode: "accept_edits"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, ses.ID, "Make A return 2")
	waitIdle(t, ch, ses.ID)

	md, err := e.RenderExport(ctx, ses.ID, ExportMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Change A", "## You\n\nMake A return 2", "**Agent**\n\nWriting it.", "A now returns 2.", "```diff\n", "-func A() int { return 1 }", "+func A() int { return 2 }", "```json\n", `"path": "a.go"`} {
		if !strings.Contains(md, want) {
			t.Errorf("the Markdown export lacks %q:\n%s", want, md)
		}
	}
	js, err := e.RenderExport(ctx, ses.ID, ExportJSON)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Session struct{ ID string }
		Events  []json.RawMessage
	}
	evs, _ := e.Events(ctx, ses.ID, 0)
	if json.Unmarshal([]byte(js), &doc) != nil || doc.Session.ID != ses.ID || len(doc.Events) != len(evs) {
		t.Errorf("the JSON export has %d events for %d", len(doc.Events), len(evs))
	}

	path := filepath.Join(t.TempDir(), ExportFileName(ses.Title, ExportMarkdown))
	if err := e.ExportSession(ctx, ses.ID, ExportMarkdown, path); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); !strings.HasSuffix(path, "change-a.md") || !strings.Contains(string(b), "# Change A") {
		t.Errorf("wrote %s: %.80q", path, b)
	}
	if err := e.ExportSession(ctx, ses.ID, ExportMarkdown, filepath.Join(e.dataDir, "x.md")); err == nil {
		t.Error("an export was written into the app's data folder")
	}
}

func TestFenceOutlastsBackticks(t *testing.T) {
	if got := fence("", "a ``` b"); !strings.HasPrefix(got, "````\n") {
		t.Errorf("fence = %q", got)
	}
}
