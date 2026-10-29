package engine

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// Review finding: redaction ran per streamed chunk, so a secret the model
// streamed across two chunks stayed in the transcript.
func TestSecretSplitAcrossChunksIsRedacted(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	const secret = "ghp_abcdef0123456789"
	e.remember(secret)
	ses, _ := demoSession(t, e, "", "")
	for _, part := range []string{"the token is ghp_ab", "cdef01234", "56789 and more", " text"} {
		e.emit(ses.ID, event.AgentMessageChunk, event.MessageChunk{Content: event.ContentBlock{Type: "text", Text: part}})
	}
	e.emit(ses.ID, event.TurnEnded, event.TurnEndedData{StopReason: "end_turn"})

	evs, _ := e.Events(ctx, ses.ID, 0)
	var text strings.Builder
	for _, ev := range evs {
		var mc event.MessageChunk
		if ev.Type == event.AgentMessageChunk && json.Unmarshal(ev.Data, &mc) == nil {
			text.WriteString(mc.Content.Text)
		}
	}
	if got := text.String(); got != "the token is [REDACTED] and more text" {
		t.Errorf("streamed text = %q", got)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "sessions", ses.ID, "events.jsonl"))
	if strings.Contains(string(b), "cdef01234") {
		t.Error("a piece of the secret is in the transcript")
	}
	if evs[len(evs)-1].Type != event.TurnEnded {
		t.Error("held-back text was not flushed before the next event")
	}
}

// Review finding: a provider key was only redacted after its provider had
// been used in this run of the app.
func TestStoredKeysAreRedactedFromStartup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	mem := secrets.NewMemory()
	e, _ := Open(Options{DataDir: dir, Secrets: mem, Home: t.TempDir(), Dev: true})
	if _, err := e.SaveProvider(SaveProviderRequest{Name: "Anthropic", Type: "anthropic", APIKey: "sk-ant-startup-0123456789"}); err != nil {
		t.Fatal(err)
	}
	e.Close()

	e, _ = Open(Options{DataDir: dir, Secrets: mem, Home: t.TempDir(), Dev: true})
	defer e.Close()
	ses, _ := demoSession(t, e, "", "")
	e.AppendEvent(ctx, AppendEventRequest{SessionID: ses.ID, Type: event.UserMessageChunk,
		Data: json.RawMessage(`{"content":{"type":"text","text":"my key: sk-ant-startup-0123456789"}}`)})
	b, _ := os.ReadFile(filepath.Join(dir, "sessions", ses.ID, "events.jsonl"))
	if strings.Contains(string(b), "sk-ant-startup") {
		t.Error("a stored key reached the transcript")
	}
}

// Review finding: a repository's .git/config could make the app's own git
// calls run a command.
func TestGitCallsDoNotRunRepositoryCommands(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	marker := filepath.Join(t.TempDir(), "ran")
	setConfig := func(key, value string) {
		cmd := exec.Command("git", "config", key, value)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git config: %v %s", err, out)
		}
	}
	setConfig("core.fsmonitor", "touch "+marker+"; false")
	e := openTest(t, t.TempDir())
	defer e.Close()
	env, err := e.localEnv(repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed"), 0o644)
	if st := gitState(ctx, env); st.Dirty != 1 || st.Branch != "main" {
		t.Errorf("state = %+v", st)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("core.fsmonitor ran")
	}

	setConfig("filter.evil.clean", "touch "+marker)
	os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("* filter=evil\n"), 0o644)
	st := gitState(ctx, env)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a content filter ran")
	}
	if st.Dirty != -1 || st.Branch != "main" || st.Head == "" {
		t.Errorf("state with filters = %+v; want branch and HEAD, dirty unknown", st)
	}
}
