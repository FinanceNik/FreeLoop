package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
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

// GUI-029: the picture must be a PNG, however the call came.
func TestSaveUsagePicture(t *testing.T) {
	ctx := context.Background()
	e := openBinding(t, t.TempDir(), secrets.NewMemory())
	path := filepath.Join(t.TempDir(), "usage.png")
	png := "\x89PNG\r\n\x1a\nrest"
	if err := e.SaveUsagePicture(ctx, path, base64.StdEncoding.EncodeToString([]byte(png))); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != png {
		t.Errorf("wrote %q", b)
	}
	bad := filepath.Join(filepath.Dir(path), "bad")
	if err := e.SaveUsagePicture(ctx, bad, base64.StdEncoding.EncodeToString([]byte("#!/bin/sh"))); err == nil {
		t.Error("wrote something that is not a PNG")
	}
	if err := e.SaveUsagePicture(ctx, "relative.png", ""); err == nil {
		t.Error("took a relative path")
	}
	if _, err := os.Stat(bad); err == nil {
		t.Error("the refused file exists")
	}
}

// SEC-006: the trust prompt names what a repository brings for the agents, and the
// hooks trusting it would let Claude Code and Copilot CLI run.
func TestTrustPromptNamesAgentConfig(t *testing.T) {
	ctx := context.Background()
	e := openBinding(t, t.TempDir(), secrets.NewMemory())
	root := t.TempDir()
	write := func(name, text string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755)
		os.WriteFile(filepath.Join(root, name), []byte(text), 0o644)
	}
	write(".claude/settings.json", `{"permissions":{"allow":["Bash"]},"hooks":{"SessionStart":[],"PreToolUse":[]}}`)
	write(".codex/config.toml", "approval_policy = \"never\"\n")
	write("opencode.json", `{"theme":"x"}`)
	write(".github/copilot/settings.json", `{"defaultPermissionMode":"allow-all"}`)
	write(".github/hooks/check.json", `{"version":1,"hooks":{"preToolUse":[]}}`)
	write(".github/hooks/notes.md", "not a hook")
	got, err := e.ProjectSettings(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.AgentConfig) != 5 {
		t.Fatalf("%+v", got.AgentConfig)
	}
	c := got.AgentConfig[0]
	if c.Path != ".claude/settings.json" || !c.Allows || strings.Join(c.Hooks, ",") != "PreToolUse,SessionStart" {
		t.Errorf("claude: %+v", c)
	}
	if !got.AgentConfig[1].Allows || got.AgentConfig[2].Allows {
		t.Errorf("codex, opencode: %+v", got.AgentConfig[1:3])
	}
	if c := got.AgentConfig[3]; c.Path != ".github/copilot/settings.json" || !c.Allows || len(c.Hooks) != 0 {
		t.Errorf("copilot settings: %+v", c)
	}
	if c := got.AgentConfig[4]; c.Path != ".github/hooks/check.json" || strings.Join(c.Hooks, ",") != "preToolUse" {
		t.Errorf("copilot hooks: %+v", c)
	}
	if e, _ := e.ProjectSettings(ctx, t.TempDir()); len(e.AgentConfig) != 0 {
		t.Errorf("a plain folder: %+v", e.AgentConfig)
	}
}

// HRN-008: a restored providers.yaml naming another app's Keychain item does not get
// it read; the provider counts as missing its key.
func TestRestoreReadsOnlyTheAppsOwnItem(t *testing.T) {
	mem := secrets.NewMemory()
	other := secrets.Ref{Service: "Claude Code-credentials", Account: "me"}
	if err := mem.Set(other, "vendor token"); err != nil {
		t.Fatal(err)
	}
	e := openBinding(t, t.TempDir(), mem)
	got := e.missingKeys([]config.Provider{{ID: "p", Name: "Planted", APIKey: "keychain://" + other.Service + "/" + other.Account}})
	if len(got) != 1 || got[0] != "Planted" {
		t.Errorf("another app's item counted as this provider's key: %v", got)
	}
}
