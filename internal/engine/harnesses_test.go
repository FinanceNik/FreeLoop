package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness/acp/acptest"
)

// HRN-004, HRN-005, MCP-004: a preset file adds an external harness with a
// profile of its own; its session runs the agent in the project, gets the
// session's MCP servers, and its edit is approved from the attention view.
func TestExternalHarnessSession(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	exe, _ := os.Executable()
	os.MkdirAll(filepath.Join(dir, config.PresetsDir), 0o700)
	os.WriteFile(filepath.Join(dir, config.PresetsDir, "ref.yaml"),
		[]byte("name: Reference agent\ncommand: "+exe+"\nenv:\n  "+acptest.EnvVar+": \"1\"\n"), 0o600)
	e.loadPresets()
	e.loadProfiles()
	if !slices.ContainsFunc(e.ListHarnesses(), func(h HarnessInfo) bool { return h.ID == "ref" && h.Kind == "acp" && !h.UsesProvider }) {
		t.Fatalf("harnesses = %+v", e.ListHarnesses())
	}
	if !slices.ContainsFunc(e.ListHarnesses(), func(h HarnessInfo) bool { return h.ID == config.ClaudeCodePresetID }) {
		t.Error("the Claude Code preset is missing")
	}

	info, err := e.SaveMCPServer(SaveMCPServerRequest{Server: config.MCPServer{ID: "echo", Transport: "stdio", Command: exe, Env: map[string]string{"FREELOOP_MCP_TEST_SERVER": "1"}, Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ApproveMCPServer(ctx, "echo", info.Fingerprint); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello"), 0o644)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "ref"})
	if err != nil {
		t.Fatal(err)
	}
	if ses.HarnessID != "ref" || ses.ProviderID != "" {
		t.Fatalf("session = %+v", ses)
	}
	if _, err := e.Prompt(ctx, ses.ID, "edit the notes"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested && ev.SID == ses.ID })
	items, _ := e.Attention(ctx)
	if len(items) != 1 || len(items[0].Requests) != 1 || items[0].Requests[0].Tool != "edit_file" {
		t.Fatalf("attention = %+v", items)
	}
	e.Decide(DecideRequest{RequestID: items[0].Requests[0].ID, OptionID: "yes"})
	waitIdle(t, ch, ses.ID)
	if b, _ := os.ReadFile(filepath.Join(root, "notes.txt")); string(b) != "hello edited" {
		t.Errorf("notes.txt = %q", b)
	}
	evs, _ := e.Events(ctx, ses.ID, 0)
	var said strings.Builder
	for _, ev := range evs {
		if ev.Type == event.AgentMessageChunk {
			said.Write(ev.Data)
		}
	}
	if !strings.Contains(said.String(), "mcp: echo="+exe) {
		t.Errorf("the agent did not get the MCP server: %s", said.String())
	}

	// The agent process ends with the session.
	if err := e.DeleteSession(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
}

// SES-003: after a restart an agent session is loaded again when the agent
// can, without recording the history it replays; otherwise it starts anew
// and the transcript says so.
func TestExternalSessionLoad(t *testing.T) {
	for _, canLoad := range []bool{true, false} {
		ctx := context.Background()
		dir := t.TempDir()
		exe, _ := os.Executable()
		os.MkdirAll(filepath.Join(dir, config.PresetsDir), 0o700)
		load := "0"
		if canLoad {
			load = "1"
		}
		os.WriteFile(filepath.Join(dir, config.PresetsDir, "ref.yaml"),
			[]byte("name: Reference agent\ncommand: "+exe+"\nenv:\n  "+acptest.EnvVar+": \"1\"\n  "+acptest.LoadVar+": \""+load+"\"\n"), 0o600)
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello"), 0o644)

		e := openTest(t, dir)
		ch, cancel := e.Subscribe()
		ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "ref", Mode: "accept_edits"})
		if err != nil {
			t.Fatal(err)
		}
		e.Prompt(ctx, ses.ID, "first")
		waitIdle(t, ch, ses.ID)
		cancel()
		if got, _ := e.store.GetSession(ctx, ses.ID); got.ExternalID != "s1" {
			t.Fatalf("external ID = %q", got.ExternalID)
		}
		e.Close()

		e = openTest(t, dir)
		ch, cancel = e.Subscribe()
		e.Prompt(ctx, ses.ID, "second")
		waitIdle(t, ch, ses.ID)
		cancel()
		evs, _ := e.Events(ctx, ses.ID, 0)
		var all strings.Builder
		for _, ev := range evs {
			all.WriteString(string(ev.Type) + " " + string(ev.Data) + "\n")
		}
		text := all.String()
		switch {
		case canLoad && (!strings.Contains(text, "loaded: s1") || strings.Contains(text, "replayed history")):
			t.Errorf("load: transcript =\n%s", text)
		case !canLoad && !strings.Contains(text, "started a new session"):
			t.Errorf("no load: transcript lacks the notice:\n%s", text)
		}
		e.Close()
	}
}

// A preset whose command is not installed says what to install.
func TestMissingAgentCommand(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	if m := e.missingCommand("definitely-not-a-command-freeloop"); !strings.Contains(m, "not on your login PATH") {
		t.Errorf("missing = %q", m)
	}
	if m := e.missingCommand("sh"); m != "" {
		t.Errorf("sh is reported missing: %q", m)
	}
	if m := e.missingCommand("it's"); !strings.Contains(m, "it's") {
		t.Errorf("quoting = %q", m)
	}
}

// An agent that is not signed in says so in plain words, with the preset's
// own sign-in note, and the prompts queued behind it are not sent.
func TestExternalAgentNotSignedIn(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	exe, _ := os.Executable()
	os.MkdirAll(filepath.Join(dir, config.PresetsDir), 0o700)
	os.WriteFile(filepath.Join(dir, config.PresetsDir, "ref.yaml"),
		[]byte("name: Reference agent\ncommand: "+exe+"\nnotes: Sign in once by running ref login in a terminal.\nenv:\n  "+acptest.EnvVar+": \"1\"\n  "+acptest.AuthVar+": \"1\"\n"), 0o600)
	e.loadPresets()
	e.loadProfiles()
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProfileID: "ref", Mode: "accept_edits"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Prompt(ctx, ses.ID, "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Prompt(ctx, ses.ID, "second"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)
	time.Sleep(200 * time.Millisecond)
	evs, _ := e.Events(ctx, ses.ID, 0)
	var errs, notices []string
	turns := 0
	for _, ev := range evs {
		var d struct{ Message string }
		json.Unmarshal(ev.Data, &d)
		switch ev.Type {
		case event.Error:
			errs = append(errs, d.Message)
		case event.Notice:
			notices = append(notices, d.Message)
		case event.TurnEnded:
			turns++
		}
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "Reference agent needs you to sign in") || !strings.Contains(errs[0], "ref login") || strings.Contains(errs[0], "-32000") {
		t.Errorf("errors = %q", errs)
	}
	if turns != 1 || !slices.ContainsFunc(notices, func(n string) bool { return strings.Contains(n, "1 queued message was not sent") }) {
		t.Errorf("turns %d, notices %q", turns, notices)
	}
}

// SES-015: an agent that says it reads images gets them as image blocks;
// one that does not is told nothing and the user is, before anything is
// sent.
func TestExternalAgentImages(t *testing.T) {
	for _, reads := range []bool{true, false} {
		ctx := context.Background()
		dir := t.TempDir()
		e := openTest(t, dir)
		ch, cancel := e.Subscribe()
		exe, _ := os.Executable()
		os.MkdirAll(filepath.Join(dir, config.PresetsDir), 0o700)
		flag := "0"
		if reads {
			flag = "1"
		}
		os.WriteFile(filepath.Join(dir, config.PresetsDir, "ref.yaml"),
			[]byte("name: Reference agent\ncommand: "+exe+"\nenv:\n  "+acptest.EnvVar+": \"1\"\n  "+acptest.ImageVar+": \""+flag+"\"\n"), 0o600)
		e.loadPresets()
		e.loadProfiles()
		ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProfileID: "ref", Mode: "plan"})
		if err != nil {
			t.Fatal(err)
		}
		png := ImageInput{MediaType: "image/png", Data: base64.StdEncoding.EncodeToString(tinyPNG)}
		if _, err := e.PromptWith(ctx, ses.ID, PromptRequest{Text: "look", Images: []ImageInput{png, png}}); err != nil {
			t.Fatal(err)
		}
		waitIdle(t, ch, ses.ID)
		evs, _ := e.Events(ctx, ses.ID, 0)
		var text, errs string
		for _, ev := range evs {
			switch ev.Type {
			case event.AgentMessageChunk:
				text += string(ev.Data)
			case event.Error:
				errs += string(ev.Data)
			}
		}
		if reads && !strings.Contains(text, "images: 2") {
			t.Errorf("the agent saw: %s (errors %s)", text, errs)
		}
		if !reads && (!strings.Contains(errs, "does not accept images") || strings.Contains(text, "mcp:")) {
			t.Errorf("without image support: text %q, errors %q", text, errs)
		}
		cancel()
		e.Close()
	}
}
