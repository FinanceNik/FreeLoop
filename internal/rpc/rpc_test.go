package rpc

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// EXT-004: a session created and prompted through the socket is the
// engine's like any other (what the GUI subscribes to sees it at once);
// its approval can be answered through the API; a second app cannot serve
// the same data folder.
func TestHeadless(t *testing.T) {
	e, err := engine.Open(engine.Options{DataDir: t.TempDir(), Secrets: secrets.NewMemory(), Home: t.TempDir(), Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	// Socket paths are short on macOS (104 bytes); the test's temp folder
	// can be longer. Windows has no /tmp, and its temporary folder is short.
	base := "/tmp"
	if runtime.GOOS == "windows" {
		base = os.TempDir()
	}
	dir, err := os.MkdirTemp(base, "fl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "s.sock")
	srv, err := Listen(e, sock)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	defer srv.Close()
	if _, err := Listen(e, sock); err == nil {
		t.Error("a second server took the socket")
	}

	gui, cancel := e.Subscribe()
	defer cancel()
	c, err := Dial(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var ses session.Session
	if err := c.Call("sessions.create", engine.CreateSessionRequest{Title: "From the CLI", ProjectRoot: t.TempDir(), ProviderID: engine.DemoProviderID, Mode: "accept_edits"}, &ses); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-gui:
		if ev.SID != ses.ID {
			t.Errorf("the GUI saw %s first", ev.SID)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the GUI's subscription saw nothing")
	}
	var optimizers []engine.OptimizerInfo
	if err := c.Call("sessions.optimizers", map[string]any{"id": ses.ID}, &optimizers); err != nil || len(optimizers) == 0 {
		t.Errorf("sessions.optimizers = %+v, %v", optimizers, err)
	}
	if err := c.Call("sessions.setOptimizer", map[string]any{"id": ses.ID, "optimizer": "no-such-optimizer"}, nil); err == nil {
		t.Error("switched to an unknown optimizer")
	}
	if err := c.Call("sessions.prompt", map[string]any{"id": ses.ID, "text": "go"}, nil); err != nil {
		t.Fatal(err)
	}

	// Follow the turn: answer its approval, stop at its end.
	var after int64
	ended, approved := false, map[string]bool{}
	for deadline := time.Now().Add(20 * time.Second); !ended && time.Now().Before(deadline); {
		var evs []event.Event
		if err := c.Call("sessions.wait", map[string]any{"id": ses.ID, "after": after, "timeoutMs": 2000}, &evs); err != nil {
			t.Fatal(err)
		}
		for _, ev := range evs {
			after = ev.Seq
			ended = ended || ev.Type == event.TurnEnded
		}
		var pending []permission.Request
		c.Call("approvals.list", nil, &pending)
		for _, r := range pending {
			// A request stays listed for a moment after its decision.
			if r.SessionID == ses.ID && !approved[r.ID] {
				if err := c.Call("approvals.decide", map[string]any{"requestId": r.ID, "optionId": "allow_once"}, nil); err != nil {
					t.Fatal(err)
				}
				approved[r.ID] = true
			}
		}
	}
	if !ended || len(approved) == 0 {
		t.Fatalf("turn ended %v, approved %v", ended, approved)
	}
	var list []session.Session
	c.Call("sessions.list", nil, &list)
	if !slices.ContainsFunc(list, func(s session.Session) bool { return s.ID == ses.ID && s.Title == "From the CLI" }) {
		t.Errorf("sessions = %+v", list)
	}
	if err := c.Call("no.such", nil, nil); err == nil {
		t.Error("an unknown method answered")
	}
}

// Closing the server ends connected clients, one waiting for events
// included, so quitting the app never hangs on them.
func TestCloseEndsClients(t *testing.T) {
	e, err := engine.Open(engine.Options{DataDir: t.TempDir(), Secrets: secrets.NewMemory(), Home: t.TempDir(), Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	sock := filepath.Join(t.TempDir(), "s.sock")
	srv, err := Listen(e, sock)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	idle, err := Dial(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer idle.Close()
	waiting, err := Dial(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer waiting.Close()
	var ses session.Session
	if err := waiting.Call("sessions.create", engine.CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: engine.DemoProviderID}, &ses); err != nil {
		t.Fatal(err)
	}
	go waiting.Call("sessions.wait", map[string]any{"id": ses.ID, "after": 1 << 30, "timeoutMs": 300000}, nil)
	time.Sleep(100 * time.Millisecond)
	closed := make(chan error, 1)
	go func() { closed <- srv.Close() }()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close waited for connected clients")
	}
}
