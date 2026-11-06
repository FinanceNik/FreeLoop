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
	"github.com/FinanceNik/Freeloop/internal/permission"
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
	approvePending(t, e)
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
	approvePending(t, e)
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
		approvePending(t, e)
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

// A plan from an external agent (2026-11-06): the option the user clicks goes back as
// it is, a mode they chose that the app does not allow leaves the session asking, and
// a note sent back with "keep planning" reaches the agent as the next message.
func TestAnExternalAgentsPlan(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	exe, _ := os.Executable()
	os.MkdirAll(filepath.Join(dir, config.PresetsDir), 0o700)
	os.WriteFile(filepath.Join(dir, config.PresetsDir, "ref.yaml"),
		[]byte("name: Reference agent\ncommand: "+exe+"\nenv:\n  "+acptest.EnvVar+": \"1\"\nmodes:\n  plan: plan\n  ask: default\n  auto: auto\n  bypass: bypassPermissions\n"), 0o600)
	e.loadPresets()
	e.loadProfiles()
	approvePending(t, e)
	root := t.TempDir()
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "ref", Mode: "plan"})
	if err != nil {
		t.Fatal(err)
	}
	said := func() string {
		evs, _ := e.Events(ctx, ses.ID, 0)
		var b strings.Builder
		for _, ev := range evs {
			if ev.Type == event.AgentMessageChunk || ev.Type == event.ModeRefused || ev.Type == event.UserMessageChunk {
				b.Write(ev.Data)
			}
		}
		return b.String()
	}
	answer := func(option, note string) {
		t.Helper()
		waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested && ev.SID == ses.ID })
		for _, r := range e.PendingPermissions() {
			if r.SessionID == ses.ID {
				if err := e.Decide(DecideRequest{RequestID: r.ID, OptionID: option, Reason: note}); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
		t.Fatal("no request")
	}

	// Bypass is off here: the choice goes to the agent as made, and the session asks.
	if _, err := e.Prompt(ctx, ses.ID, "PLAN one"); err != nil {
		t.Fatal(err)
	}
	answer("bypassPermissions", "")
	waitIdle(t, ch, ses.ID)
	if !strings.Contains(said(), "chose: bypassPermissions") {
		t.Errorf("the agent did not get the option the user chose: %s", said())
	}
	if got, _ := e.store.GetSession(ctx, ses.ID); got.Mode != "ask" {
		t.Errorf("after a refused choice the mode is %q, not ask", got.Mode)
	}
	if !strings.Contains(said(), `"chosen":true`) {
		t.Errorf("the refusal did not say the user chose it: %s", said())
	}

	// Sent back with a note: the note follows as the user's next message.
	if err := e.SetMode(ctx, ses.ID, "plan"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Prompt(ctx, ses.ID, "PLAN two"); err != nil {
		t.Fatal(err)
	}
	answer("plan", "Add tests first.")
	// The note runs as a prompt of its own; in plan mode its edit is refused.
	waitFor(t, ch, func(ev event.Event) bool {
		return ev.Type == event.UserMessageChunk && ev.SID == ses.ID && strings.Contains(string(ev.Data), "My feedback on the plan")
	})
	waitIdle(t, ch, ses.ID)
	if !strings.Contains(said(), "My feedback on the plan:") || !strings.Contains(said(), "Add tests first.") {
		t.Errorf("the note did not reach the agent: %s", said())
	}
}

// HRN-021: an external agent's question reaches the user as a form on the agent's own
// call, and the answers go back to it. The app says it takes forms, without which
// Claude Code leaves its AskUserQuestion off.
func TestAnExternalAgentAsks(t *testing.T) {
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
	approvePending(t, e)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProfileID: "ref"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Prompt(ctx, ses.ID, "ASK"); err != nil {
		t.Fatal(err)
	}
	ev := waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested && ev.SID == ses.ID })
	var asked struct {
		RequestID  string           `json:"requestId"`
		ToolCallID string           `json:"toolCallId"`
		Form       *permission.Form `json:"form"`
	}
	json.Unmarshal(ev.Data, &asked)
	if asked.Form == nil || asked.Form.Message != "Which order?" || asked.Form.Server != "Reference agent" || !strings.Contains(string(asked.Form.Schema), "Paste all first") {
		t.Fatalf("the form = %+v", asked.Form)
	}
	if asked.ToolCallID != "q1" {
		t.Errorf("the form is on %q, not the agent's call", asked.ToolCallID)
	}
	if err := e.broker.DecideWith(asked.RequestID, "accept", "", "", map[string]any{"question_0": "One by one"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)
	evs, _ := e.Events(ctx, ses.ID, 0)
	var said strings.Builder
	for _, ev := range evs {
		if ev.Type == event.AgentMessageChunk {
			said.Write(ev.Data)
		}
	}
	if !strings.Contains(said.String(), `answer: {\"question_0\":\"One by one\"}`) {
		t.Errorf("the agent heard: %s", said.String())
	}
}
