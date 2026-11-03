package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/session"
)

func TestRecapOf(t *testing.T) {
	said := func(typ event.Type, text string) event.Event {
		data, _ := json.Marshal(event.MessageChunk{Content: event.ContentBlock{Type: "text", Text: text}})
		return event.Event{Type: typ, Data: data}
	}
	ses := session.Session{Title: "t"}
	recap := recapOf(ses, []event.Event{said(event.UserMessageChunk, "fix the login"), said(event.AgentMessageChunk, "done, in auth.go")})
	for _, want := range []string{"fix the login", "done, in auth.go", "no memory"} {
		if !strings.Contains(recap, want) {
			t.Errorf("no %q in %q", want, recap)
		}
	}
	long := recapOf(ses, []event.Event{said(event.UserMessageChunk, "the very start"), said(event.AgentMessageChunk, strings.Repeat("line\n", 20000)+"the very end")})
	if len(long) > maxRecap+500 {
		t.Errorf("%d bytes", len(long))
	}
	if strings.Contains(long, "the very start") || !strings.Contains(long, "the very end") || !strings.Contains(long, "left out") {
		t.Errorf("kept the wrong end: %q", long[:200])
	}
}

// SES-005: an archived session keeps nothing outside its transcript. An optimizer's
// recall database holds full command output, beyond the transcript's redaction.
func TestArchiveRemovesSessionData(t *testing.T) {
	ctx := context.Background()
	e := openBinding(t, t.TempDir(), secrets.NewMemory())
	withScript(e, "s", scripted.Step{Text: "hi"})
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "s", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	dir := e.optimizerDir(ses, false)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "recall.db"), []byte("full output"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.ArchiveSession(ctx, ses.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the optimizer data is still there: %v", err)
	}
}
