package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// bypassSession opens a session in bypass, in a trusted project, with the setting
// on. started says it has a running harness, not just a stored mode.
func bypassSession(t *testing.T, e *Engine, started bool) (session.Session, string) {
	t.Helper()
	ctx := context.Background()
	cfg := e.Config()
	cfg.Permissions.AllowBypass = true
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	withScript(e, "s", scripted.Step{Text: "hi"})
	root := t.TempDir()
	if _, err := e.SetProjectTrust(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetMode(ctx, ses.ID, "bypass"); err != nil {
		t.Fatal(err)
	}
	if started {
		ch, cancel := e.Subscribe()
		defer cancel()
		e.Prompt(ctx, ses.ID, "go")
		waitIdle(t, ch, ses.ID)
	}
	return ses, root
}

// openFor opens an engine the test closes itself, to open the same data again.
func openFor(t *testing.T, dir string, mem secrets.Store) *Engine {
	t.Helper()
	e, err := Open(Options{DataDir: dir, Secrets: mem, Home: dir, WatchInterval: -1, ScheduleInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func wantAsking(t *testing.T, e *Engine, sid string) {
	t.Helper()
	ctx := context.Background()
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if ses.Mode != "ask" {
		t.Errorf("the session is still in %s", ses.Mode)
	}
	e.mu.Lock()
	r := e.runtimes[sid]
	e.mu.Unlock()
	if r != nil {
		if m := r.hs.Mode(); m != "ask" {
			t.Errorf("its harness is still in %s", m)
		}
	}
	evs, _ := e.Events(ctx, sid, 0)
	for _, ev := range evs {
		if ev.Type == event.Notice && strings.Contains(string(ev.Data), "ypass") {
			return
		}
	}
	t.Error("the transcript does not say why the mode changed")
}

// SEC-001: turning bypass off ends it, in a session that is running and in one
// that only has the mode stored, rather than only refusing it from then on.
func TestTurningBypassOffEndsIt(t *testing.T) {
	for _, started := range []bool{true, false} {
		e := openBinding(t, t.TempDir(), secrets.NewMemory())
		ses, _ := bypassSession(t, e, started)
		cfg := e.Config()
		cfg.Permissions.AllowBypass = false
		if _, err := e.SaveSettings(cfg); err != nil {
			t.Fatal(err)
		}
		wantAsking(t, e, ses.ID)
	}
}

// SEC-001, SECURITY.md row 11: bypass is not available in an untrusted project,
// so taking the trust back takes bypass with it.
func TestRevokingTrustEndsBypass(t *testing.T) {
	for _, started := range []bool{true, false} {
		e := openBinding(t, t.TempDir(), secrets.NewMemory())
		ses, root := bypassSession(t, e, started)
		if _, err := e.SetProjectTrust(context.Background(), root, false); err != nil {
			t.Fatal(err)
		}
		wantAsking(t, e, ses.ID)
	}
}

// The setting can go while the app is closed, and a rebuilt index forgets trust.
func TestBypassDoesNotSurviveARestartWithoutTheSetting(t *testing.T) {
	dir, mem := t.TempDir(), secrets.NewMemory()
	e := openFor(t, dir, mem)
	ses, _ := bypassSession(t, e, false)
	cfg := e.Config()
	e.Close()
	cfg.Permissions.AllowBypass = false
	if err := config.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	again := openFor(t, dir, mem)
	defer again.Close()
	wantAsking(t, again, ses.ID)
}

// Whatever a sweep missed, a session does not start in a bypass that does not hold.
func TestASessionDoesNotStartInABypassThatDoesNotHold(t *testing.T) {
	ctx := context.Background()
	e := openBinding(t, t.TempDir(), secrets.NewMemory())
	ses, _ := bypassSession(t, e, false)
	e.mu.Lock()
	e.cfg.Permissions.AllowBypass = false
	e.mu.Unlock()
	if _, err := e.runtimeFor(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	wantAsking(t, e, ses.ID)
}

// Allowed and trusted, a session keeps the bypass it was put in across a restart.
func TestBypassThatHoldsIsLeftAlone(t *testing.T) {
	dir, mem := t.TempDir(), secrets.NewMemory()
	e := openFor(t, dir, mem)
	ses, _ := bypassSession(t, e, false)
	e.Close()
	again := openFor(t, dir, mem)
	defer again.Close()
	got, err := again.store.GetSession(context.Background(), ses.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != "bypass" {
		t.Errorf("the session is in %s", got.Mode)
	}
}
