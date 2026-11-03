package engine

import (
	"context"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// GUI-027: a stored status with nothing behind it does not hold up quitting, and
// a session in the middle of something does.
func TestCheckQuit(t *testing.T) {
	ctx := context.Background()
	e := openBinding(t, t.TempDir(), secrets.NewMemory())
	withScript(e, "s", scripted.Step{Text: "hi"})
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "s", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if c := e.CheckQuit(ctx); !c.Empty() {
		t.Errorf("an idle session: %+v", c)
	}
	if _, err := e.store.Append(ctx, ses.ID, "session_status", []byte(`{"status":"running"}`)); err != nil {
		t.Fatal(err)
	}
	if c := e.CheckQuit(ctx); !c.Empty() {
		t.Errorf("a status left by a crash: %+v", c)
	}
	if _, err := e.runtimeFor(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	if c := e.CheckQuit(ctx); c.Working != 1 {
		t.Errorf("a session at work: %+v", c)
	}
}
