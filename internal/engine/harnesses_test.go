package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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
