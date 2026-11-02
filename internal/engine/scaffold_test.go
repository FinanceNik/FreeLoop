package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// HRN-011: "New harness" writes the Python template and registers it; a
// session on it gets the agent's reply; when the agent's files change, an
// idle session's agent restarts on its next message.
func TestScaffoldHarness(t *testing.T) {
	if hostRun("python3", "--version") != nil {
		t.Skip("no python3")
	}
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	folder := t.TempDir()
	res, err := e.ScaffoldHarness(ctx, ScaffoldRequest{Name: "My Agent", Language: "python", Folder: folder})
	if err != nil {
		t.Fatal(err)
	}
	// The user asked for it here, so it waits for no approval.
	if got := e.PendingCommands(); len(got) != 0 {
		t.Errorf("pending = %+v", got)
	}
	if res.Preset.ID != "my-agent" || res.Preset.Watch != res.Dir || !strings.HasSuffix(res.Entry, "agent.py") {
		t.Fatalf("result = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(res.Dir, "test_agent.py")); err != nil {
		t.Error(err)
	}
	if _, err := e.ScaffoldHarness(ctx, ScaffoldRequest{Name: "My Agent", Language: "python", Folder: folder}); err == nil {
		t.Error("a second harness of the same name was made")
	}

	e.watchAgents() // records the folder
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProfileID: "my-agent"})
	if err != nil {
		t.Fatal(err)
	}
	if ses.HarnessID != "my-agent" {
		t.Fatalf("session harness = %s", ses.HarnessID)
	}
	if _, err := e.Prompt(ctx, ses.ID, "hello there"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)
	evs, _ := e.Events(ctx, ses.ID, 0)
	all, _ := json.Marshal(evs)
	if !strings.Contains(string(all), "You said: hello there") {
		t.Fatalf("the agent did not answer:\n%s", all)
	}

	b, _ := os.ReadFile(res.Entry)
	os.WriteFile(res.Entry, append(b, []byte("\n# changed\n")...), 0o644)
	e.watchAgents()
	e.mu.Lock()
	_, running := e.runtimes[ses.ID]
	e.mu.Unlock()
	evs, _ = e.Events(ctx, ses.ID, 0)
	if running || evs[len(evs)-1].Type != event.Notice {
		t.Errorf("after a change: running %v, last event %s", running, evs[len(evs)-1].Type)
	}
}
