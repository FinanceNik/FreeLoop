package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/acp/acptest"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// panicking is a harness session with a bug in it.
type panicking struct{}

func (panicking) Prompt(context.Context, harness.Prompt) (harness.StopReason, error) {
	panic("a bug")
}
func (panicking) Mode() permission.Mode   { return permission.ModeAsk }
func (panicking) SetMode(permission.Mode) {}
func (panicking) Close() error            { return nil }

// A bug in one turn must not take down the app and every other session.
func TestAPanicEndsOnlyItsTurn(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, _ := demoSession(t, e, "s", "ask")
	err := e.promptSafely(&runtime{sid: ses.ID, hs: panicking{}}, ctx, harness.Prompt{Text: "go"})
	if err == nil {
		t.Fatal("no error from a panic")
	}
	evs, _ := e.Events(ctx, ses.ID, 0)
	types := []event.Type{}
	for _, ev := range evs {
		types = append(types, ev.Type)
	}
	for _, want := range []event.Type{event.Error, event.TurnEnded, event.SessionStatus} {
		if !slices.Contains(types, want) {
			t.Errorf("no %s: %v", want, types)
		}
	}
	if got, _ := e.GetSession(ctx, ses.ID); got.Status != "error" {
		t.Errorf("status %s", got.Status)
	}
}

// NFR-REL-002: a crashed agent does not leave its session unusable.
func TestAnExternalAgentIsStartedAgainAfterItExits(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e, ses := referenceAgentSession(t, dir)
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	e.Prompt(ctx, ses.ID, "one")
	stopEarlyIn(t, e, ch, ses.ID)
	e.mu.Lock()
	first := e.runtimes[ses.ID]
	e.mu.Unlock()
	if first == nil {
		t.Fatal("no runtime")
	}
	// The agent's process is gone.
	first.hs.Close()
	if _, err := e.Prompt(ctx, ses.ID, "two"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		evs, _ := e.Events(ctx, ses.ID, 0)
		if slices.ContainsFunc(evs, func(ev event.Event) bool {
			return ev.Type == event.Notice && strings.Contains(string(ev.Data), "starts it again")
		}) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no word that the agent stopped")
		}
		time.Sleep(20 * time.Millisecond)
	}
	for time.Now().Before(deadline) {
		e.mu.Lock()
		_, still := e.runtimes[ses.ID]
		e.mu.Unlock()
		if !still {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := e.Prompt(ctx, ses.ID, "three"); err != nil {
		t.Fatalf("the next message: %v", err)
	}
	stopEarlyIn(t, e, ch, ses.ID)
	e.mu.Lock()
	again := e.runtimes[ses.ID]
	e.mu.Unlock()
	if again == nil || again == first {
		t.Error("the agent was not started again")
	}
}

// stopEarlyIn answers what a turn asks until it ends.
func stopEarlyIn(t *testing.T, e *Engine, ch <-chan event.Event, sid string) {
	t.Helper()
	stop := make(chan struct{})
	defer close(stop)
	approveAll(e, stop)
	waitIdle(t, ch, sid)
}

// A session keeps what a rebuilt index needs beside its transcript.
func TestASessionRecordsItsSettings(t *testing.T) {
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	ses, _ := demoSession(t, e, "Named", "ask")
	b, err := os.ReadFile(filepath.Join(dir, "sessions", ses.ID, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ HarnessID, ProjectRoot string }
	if json.Unmarshal(b, &got); got.ProjectRoot != ses.ProjectRoot || got.HarnessID == "" {
		t.Errorf("record: %s", b)
	}
}

// referenceAgentSession is a session on the reference ACP agent.
func referenceAgentSession(t *testing.T, dir string) (*Engine, session.Session) {
	t.Helper()
	exe, _ := os.Executable()
	os.MkdirAll(filepath.Join(dir, config.PresetsDir), 0o700)
	os.WriteFile(filepath.Join(dir, config.PresetsDir, "ref.yaml"),
		[]byte("name: Reference agent\ncommand: "+exe+"\nenv:\n  "+acptest.EnvVar+": \"1\"\n"), 0o600)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello"), 0o644)
	e := openTest(t, dir)
	approvePending(t, e)
	ses, err := e.CreateSession(context.Background(), CreateSessionRequest{ProjectRoot: root, ProfileID: "ref", Mode: "accept_edits"})
	if err != nil {
		t.Fatal(err)
	}
	return e, ses
}
