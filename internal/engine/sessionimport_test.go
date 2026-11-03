package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness/acp/acptest"
)

// importAgent adds a preset for the reference agent; extra are more of its variables.
func importAgent(t *testing.T, e *Engine, dir, id string, extra ...string) {
	t.Helper()
	exe, _ := os.Executable()
	env := "  " + acptest.EnvVar + `: "1"` + "\n"
	for _, name := range extra {
		env += "  " + name + `: "1"` + "\n"
	}
	os.MkdirAll(filepath.Join(dir, config.PresetsDir), 0o700)
	if err := os.WriteFile(filepath.Join(dir, config.PresetsDir, id+".yaml"), []byte("name: Reference agent\ncommand: "+exe+"\nenv:\n"+env), 0o600); err != nil {
		t.Fatal(err)
	}
	e.loadPresets()
	e.loadProfiles()
	approvePending(t, e)
}

func eventsOf(t *testing.T, e *Engine, sid string) []event.Event {
	t.Helper()
	evs, err := e.Events(context.Background(), sid, 0)
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func TestAgentSessionsList(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	importAgent(t, e, dir, "ref", acptest.ImportVar)
	root := t.TempDir()
	got, err := e.AgentSessions(ctx, "ref", root)
	if err != nil {
		t.Fatal(err)
	}
	// Every page, newest first, and a removed folder is said.
	if len(got) != 3 || got[0].ID != "old-2" || got[1].ID != "old-1" || got[2].ID != "gone" {
		t.Fatalf("sessions = %+v", got)
	}
	if got[0].Title != "Second session" || got[0].Folder != root || got[0].Missing || !got[2].Missing {
		t.Errorf("sessions = %+v", got)
	}
	// With no folder it also asks about the projects here.
	if _, err := e.AddProject(ctx, root); err != nil {
		t.Fatal(err)
	}
	all, err := e.AgentSessions(ctx, "ref", "")
	if err != nil || len(all) != 3 {
		t.Errorf("no folder: %+v, %v", all, err)
	}

	if _, err := e.AgentSessions(ctx, "", root); err == nil {
		t.Error("listed with no agent chosen")
	}
	importAgent(t, e, dir, "plain")
	if _, err := e.AgentSessions(ctx, "plain", root); err == nil || !strings.Contains(err.Error(), "cannot list") {
		t.Errorf("an agent that cannot list: %v", err)
	}
}

// SES-020: the agent's copy is replayed into a new session, which then continues it.
// A replay can ask for nothing and touch nothing.
func TestImportAgentSession(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	importAgent(t, e, dir, "ref", acptest.ImportVar)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello"), 0o644)

	ses, err := e.ImportAgentSession(ctx, ImportSessionRequest{ProfileID: "ref", Folder: root, ID: "old-1", Title: "  First\n session  "})
	if err != nil {
		t.Fatal(err)
	}
	if ses.Title != "First session" || ses.ExternalID != "fork-of-old-1" || ses.ImportedFrom != "ref:old-1" || ses.HarnessID != "ref" {
		t.Errorf("session = %+v", ses)
	}
	evs := eventsOf(t, e, ses.ID)
	if last := evs[len(evs)-1]; last.Type != event.SessionImported || !strings.Contains(string(last.Data), "fork-of-old-1") {
		t.Errorf("last event = %s %s", last.Type, last.Data)
	}
	var transcript strings.Builder
	statuses := map[string]string{}
	var picture string
	for _, ev := range evs {
		transcript.WriteString(string(ev.Type) + " " + string(ev.Data) + "\n")
		var d struct {
			ToolCallID string `json:"toolCallId"`
			Status     string `json:"status"`
			Content    struct {
				Type string `json:"type"`
				URI  string `json:"uri"`
			} `json:"content"`
		}
		json.Unmarshal(ev.Data, &d)
		if ev.Type == event.ToolCall || ev.Type == event.ToolCallUpdate {
			statuses[d.ToolCallID] = d.Status
		}
		if ev.Type == event.UserMessageChunk && d.Content.Type == "image" {
			picture = d.Content.URI
		}
	}
	text := transcript.String()
	for _, want := range []string{"first question from fork-of-old-1", "first answer", `\n\nsecond answer`, "cat a.txt", "contents", "not imported", `"tool":"shell"`, "Imported from Reference agent", "continues a copy"} {
		if !strings.Contains(text, want) {
			t.Errorf("the transcript lacks %q:\n%s", want, text)
		}
	}
	for _, bad := range []string{"iVBORw0KGgo", "permission was granted", "a terminal was granted", "A title the replay must not set", `permission_requested`} {
		if strings.Contains(text, bad) {
			t.Errorf("the transcript holds %q:\n%s", bad, text)
		}
	}
	for _, f := range []string{"replay-wrote-this.txt", "replay-ran-this"} {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			t.Errorf("the replay made %s", f)
		}
	}
	if statuses["done"] != "completed" || statuses["open"] != "failed" {
		t.Errorf("statuses = %+v", statuses)
	}
	// The picture is an attachment, and its bytes are there.
	data, err := e.store.Attachment(ses.ID, strings.TrimPrefix(picture, "attachment:"))
	if picture == "" || err != nil || len(data) == 0 {
		t.Errorf("picture %q: %d bytes, %v", picture, len(data), err)
	}
	// None of it counts as activity of today.
	if got := foldActivity(evs, 0); len(got) != 0 {
		t.Errorf("activity = %+v", got)
	}

	// A second import finds the first, and the list says so.
	again, err := e.ImportAgentSession(ctx, ImportSessionRequest{ProfileID: "ref", Folder: root, ID: "old-1"})
	if err != nil || again.ID != ses.ID {
		t.Errorf("again = %+v, %v", again, err)
	}
	list, err := e.AgentSessions(ctx, "ref", root)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if (s.SessionID == ses.ID) != (s.ID == "old-1") {
			t.Errorf("listed = %+v", s)
		}
	}

	// With no title given, it takes the agent's.
	named, err := e.ImportAgentSession(ctx, ImportSessionRequest{ProfileID: "ref", Folder: root, ID: "old-2"})
	if err != nil || named.Title != "Second session" {
		t.Errorf("named = %+v, %v", named, err)
	}

	// It continues the copy, and the replay of that load is not recorded twice.
	before := strings.Count(text, "first question")
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(time.Millisecond):
			}
			for _, r := range e.PendingPermissions() {
				e.Decide(DecideRequest{RequestID: r.ID, OptionID: "yes"})
			}
		}
	}()
	if _, err := e.Prompt(ctx, ses.ID, "next"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)
	var after strings.Builder
	for _, ev := range eventsOf(t, e, ses.ID) {
		after.WriteString(string(ev.Data))
	}
	if got := strings.Count(after.String(), "first question"); got != before {
		t.Errorf("the conversation is recorded %d times, not %d", got, before)
	}
	if got, _ := e.store.GetSession(ctx, ses.ID); got.Title != "First session" {
		t.Errorf("the agent renamed it: %q", got.Title)
	}
	if !strings.Contains(after.String(), "loaded: fork-of-old-1") {
		t.Errorf("it did not continue the copy:\n%s", after.String())
	}
}

func TestImportAgentSessionRefuses(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	importAgent(t, e, dir, "ref", acptest.ImportVar)
	root := t.TempDir()
	count := func() int {
		all, err := e.store.ListSessions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return len(all)
	}
	for name, req := range map[string]ImportSessionRequest{
		"no agent":    {Folder: root, ID: "old-1"},
		"no session":  {ProfileID: "ref", Folder: root},
		"gone folder": {ProfileID: "ref", Folder: "/nonexistent/folder", ID: "old-1"},
	} {
		if _, err := e.ImportAgentSession(ctx, req); err == nil {
			t.Errorf("%s: imported", name)
		}
	}
	if n := count(); n != 0 {
		t.Errorf("%d sessions after refusals", n)
	}
	// A failure leaves no half a session.
	if _, err := e.ImportAgentSession(ctx, ImportSessionRequest{ProfileID: "ref", Folder: root, ID: "broken"}); err == nil {
		t.Error("imported what the agent could not copy")
	}
	if n := count(); n != 0 {
		t.Errorf("%d sessions after a failure", n)
	}
	// An agent that cannot list is told apart from one that failed.
	importAgent(t, e, dir, "plain", acptest.LoadVar)
	if _, err := e.ImportAgentSession(ctx, ImportSessionRequest{ProfileID: "plain", Folder: root, ID: "old-1"}); err == nil || !strings.Contains(err.Error(), "cannot list") {
		t.Errorf("an agent that cannot list: %v", err)
	}
	if n := count(); n != 0 {
		t.Errorf("%d sessions", n)
	}
}

// A session imported before the database was lost is still known as imported.
func TestImportedFromSurvivesRecovery(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	importAgent(t, e, dir, "ref", acptest.ImportVar)
	ses, err := e.ImportAgentSession(ctx, ImportSessionRequest{ProfileID: "ref", Folder: t.TempDir(), ID: "old-1"})
	if err != nil {
		t.Fatal(err)
	}
	e.Close()
	for _, name := range []string{"freeloop.db", "freeloop.db-wal", "freeloop.db-shm"} {
		os.Remove(filepath.Join(dir, name))
	}
	e = openTest(t, dir)
	defer e.Close()
	got, err := e.store.GetSession(ctx, ses.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ImportedFrom != "ref:old-1" {
		t.Errorf("recovered = %+v", got)
	}
}
