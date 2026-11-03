package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// screenFor collects one terminal's output from a subscription until it
// shows want, and reports whether the shell ended.
func screenFor(t *testing.T, ch <-chan TerminalOutput, id, want string) (string, bool) {
	t.Helper()
	var sb strings.Builder
	deadline := time.After(20 * time.Second)
	for {
		select {
		case out := <-ch:
			if out.ID != id {
				continue
			}
			sb.Write(out.Data)
			if out.Exited {
				return sb.String(), true
			}
			if want != "" && strings.Contains(sb.String(), want) {
				return sb.String(), false
			}
		case <-deadline:
			t.Fatalf("no %q from %s:\n%s", want, id, sb.String())
		}
	}
}

// GUI-012: a session's terminal runs in its folder with its environment's
// variables, streams to subscribers, keeps a scrollback for a view that
// attaches later, leaves the transcript alone, and ends with the session.
func TestSessionTerminal(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	os.MkdirAll(filepath.Join(dir, config.EnvironmentsDir), 0o700)
	os.WriteFile(filepath.Join(dir, config.EnvironmentsDir, "local.yaml"), []byte("name: Local\nkind: local\nenv:\n  GREETING: hi-from-env\n"), 0o600)
	e.loadEnvironments()
	approveFiles(t, e)
	withScript(e, "s", scripted.Step{Text: "done"})
	project := t.TempDir()
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: project, ProviderID: "s", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := e.Events(ctx, ses.ID, 0)

	out, cancel := e.SubscribeTerminals()
	defer cancel()
	info, err := e.OpenTerminal(ctx, ses.ID, 90, 20)
	if err != nil {
		t.Fatal(err)
	}
	if real, _ := filepath.EvalSymlinks(project); info.SessionID != ses.ID || info.Where != real {
		t.Errorf("info = %+v", info)
	}
	if list := e.SessionTerminals(ses.ID); len(list) != 1 || list[0].ID != info.ID {
		t.Errorf("terminals = %+v", list)
	}
	e.WriteTerminal(info.ID, []byte("echo \"[$(pwd)] $GREETING $(stty size)\"\n"))
	got, _ := screenFor(t, out, info.ID, "hi-from-env 20 90")
	if real, _ := filepath.EvalSymlinks(project); !strings.Contains(got, "["+real+"]") {
		t.Errorf("not in the project:\n%s", got)
	}
	if err := e.ResizeTerminal(info.ID, 0, 0); err == nil {
		t.Error("a 0×0 terminal was accepted")
	}
	if err := e.ResizeTerminal(info.ID, 132, 43); err != nil {
		t.Fatal(err)
	}
	e.WriteTerminal(info.ID, []byte("stty size\n"))
	screenFor(t, out, info.ID, "43 132")
	scr, err := e.TerminalScreen(info.ID)
	if err != nil || !strings.Contains(string(scr.Data), "hi-from-env") || scr.Exited {
		t.Errorf("screen = %q, %v", scr.Data, err)
	}

	// A shell that exits says so; closing forgets it.
	second, err := e.OpenTerminal(ctx, ses.ID, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	e.WriteTerminal(second.ID, []byte("exit 4\n"))
	if _, exited := screenFor(t, out, second.ID, ""); !exited {
		t.Error("the exit was not reported")
	}
	if scr, _ := e.TerminalScreen(second.ID); !scr.Exited || scr.ExitCode != 4 {
		t.Errorf("exited = %+v", scr)
	}
	e.CloseTerminal(second.ID)
	if _, err := e.TerminalScreen(second.ID); err == nil {
		t.Error("a closed terminal is still there")
	}

	// Nothing of it is in the transcript.
	after, _ := e.Events(ctx, ses.ID, 0)
	for _, ev := range after[len(before):] {
		if ev.Type != event.SessionStatus && ev.Type != event.GitState && ev.Type != event.EnvHealth {
			t.Errorf("the terminal recorded %s: %s", ev.Type, ev.Data)
		}
	}

	// Deleting another session does not wait for this one's shell.
	other, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "s", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	deleted := make(chan error, 1)
	go func() { deleted <- e.DeleteSession(ctx, other.ID) }()
	select {
	case err := <-deleted:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deleting a session waited for another session's shell")
	}

	// Archiving the session ends its terminal.
	if err := e.ArchiveSession(ctx, ses.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, exited := screenFor(t, out, info.ID, ""); !exited {
		t.Error("the terminal outlived its session")
	}
	if _, err := e.OpenTerminal(ctx, ses.ID, 80, 24); err == nil {
		t.Error("a terminal opened in an archived session")
	}
}

// GUI-012's acceptance: in a Docker session the terminal is in the
// container, at the session's folder.
func TestDockerTerminal(t *testing.T) {
	ctx := context.Background()
	docker, err := environment.FindDocker()
	if err != nil {
		t.Skip(err)
	}
	if err := hostRun(docker, "info"); err != nil {
		t.Skip("no Docker daemon is running")
	}
	cache, _ := os.UserCacheDir()
	root, err := os.MkdirTemp(cache, "freeloop-docker-terminal-")
	if err != nil {
		t.Skip(err)
	}
	defer os.RemoveAll(root)
	os.WriteFile(filepath.Join(root, "marker.txt"), []byte("x"), 0o644)

	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	os.MkdirAll(filepath.Join(dir, config.EnvironmentsDir), 0o700)
	os.WriteFile(filepath.Join(dir, config.EnvironmentsDir, "box.yaml"), []byte("name: Box\nkind: docker\nimage: alpine:3.20\n"), 0o600)
	e.loadEnvironments()
	approveFiles(t, e)
	withScript(e, "s", scripted.Step{Text: "done"})
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m", EnvironmentID: "box"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.DeleteSession(ctx, ses.ID)
	out, cancel := e.SubscribeTerminals()
	defer cancel()
	info, err := e.OpenTerminal(ctx, ses.ID, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(info.Where, "container") {
		t.Errorf("where = %s", info.Where)
	}
	e.WriteTerminal(info.ID, []byte("echo \"[$(pwd)] $(uname -s)\"; ls\n"))
	got, _ := screenFor(t, out, info.ID, "marker.txt")
	if !strings.Contains(got, "] Linux") {
		t.Errorf("not in the container:\n%s", got)
	}
}
