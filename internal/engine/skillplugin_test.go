package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness/acp/acptest"
)

func skillFile(name, desc string) []byte {
	return []byte("---\nname: " + name + "\ndescription: " + desc + "\n---\n\nDo the " + name + " thing.\n")
}

// agentSaid is everything the agent of session sid said.
func agentSaid(t *testing.T, e *Engine, sid string) string {
	t.Helper()
	evs, _ := e.Events(context.Background(), sid, 0, 0)
	var said strings.Builder
	for _, ev := range evs {
		if ev.Type == event.AgentMessageChunk {
			var c event.MessageChunk
			json.Unmarshal(ev.Data, &c)
			said.WriteString(c.Content.Text + "\n")
		}
	}
	return said.String()
}

// SKL-008: an external agent whose preset takes a Claude Code plugin gets
// one with the session's enabled skills, built in the app's data folder:
// skills from the store and the project's .freeloop/ and .agents/, not
// those Claude Code finds itself (the project's .claude/skills and store
// skills imported from ~/.claude/skills), and not switched-off ones. An
// agent whose preset does not take skills gets none.
func TestExternalAgentSkills(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	exe, _ := os.Executable()
	os.MkdirAll(filepath.Join(dir, config.PresetsDir), 0o700)
	os.WriteFile(filepath.Join(dir, config.PresetsDir, "ref.yaml"),
		[]byte("name: Reference agent\ncommand: "+exe+"\nskills: claude-plugin\nenv:\n  "+acptest.EnvVar+": \"1\"\n"), 0o600)
	os.WriteFile(filepath.Join(dir, config.PresetsDir, "plain.yaml"),
		[]byte("name: Plain agent\ncommand: "+exe+"\nenv:\n  "+acptest.EnvVar+": \"1\"\n"), 0o600)
	e.loadPresets()
	e.loadProfiles()
	approvePending(t, e)

	now := time.Now().UTC()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(config.InstallSkill(dir, "release-notes", map[string][]byte{
		"SKILL.md": skillFile("release-notes", "Write release notes."), "scripts/collect.sh": []byte("#!/bin/sh\ngit log\n"),
	}, config.SkillProvenance{Source: "created in Freeloop", ImportedAt: now}))
	must(config.InstallSkill(dir, "pdf", map[string][]byte{"SKILL.md": skillFile("pdf", "PDFs.")},
		config.SkillProvenance{Source: filepath.Join(e.home, ".claude", "skills", "pdf"), ImportedAt: now}))
	must(config.InstallSkill(dir, "secret-sauce", map[string][]byte{"SKILL.md": skillFile("secret-sauce", "Off.")},
		config.SkillProvenance{Source: "created in Freeloop", ImportedAt: now}))
	must(e.SetSkillEnabled(ctx, "secret-sauce", "", false))

	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello"), 0o644)
	for d, name := range map[string]string{".agents/skills/lint": "lint", ".claude/skills/review": "review"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
		os.WriteFile(filepath.Join(root, d, "SKILL.md"), skillFile(name, "Project skill."), 0o644)
	}
	// A link from a skill to a file outside it is left out of the copy.
	private := filepath.Join(t.TempDir(), "private.txt")
	os.WriteFile(private, []byte("not for the container"), 0o600)
	os.Symlink(private, filepath.Join(root, ".agents/skills/lint/ref.md"))
	if _, err := e.SetProjectTrust(ctx, root, true); err != nil {
		t.Fatal(err)
	}

	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "ref", Mode: "accept_edits"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, ses.ID, "go")
	waitIdle(t, ch, ses.ID)
	plugin := config.SessionPluginDir(dir, ses.ID)
	if said := agentSaid(t, e, ses.ID); !strings.Contains(said, "plugin: "+plugin+"\n") {
		t.Fatalf("the agent was not given the plugin:\n%s", said)
	}
	var skills []string
	entries, _ := os.ReadDir(filepath.Join(plugin, "skills"))
	for _, de := range entries {
		if _, err := os.Stat(filepath.Join(plugin, "skills", de.Name(), "SKILL.md")); err == nil {
			skills = append(skills, de.Name())
		}
	}
	if strings.Join(skills, ",") != "lint,release-notes" {
		t.Errorf("the plugin's skills = %v", skills)
	}
	var manifest map[string]string
	b, _ := os.ReadFile(filepath.Join(plugin, ".claude-plugin", "plugin.json"))
	if json.Unmarshal(b, &manifest); manifest["name"] != "freeloop" {
		t.Errorf("plugin.json = %s", b)
	}
	if fi, err := os.Stat(filepath.Join(plugin, "skills", "release-notes", "scripts", "collect.sh")); err != nil || goruntime.GOOS != "windows" && fi.Mode()&0o100 == 0 {
		t.Errorf("the script = %v, %v", fi, err)
	}
	if _, err := os.Stat(filepath.Join(plugin, "skills", "lint", "ref.md")); err == nil {
		t.Error("a link out of the skill was copied")
	}
	if _, err := os.Stat(filepath.Join(plugin, "skills", "release-notes", config.ProvenanceFile)); err == nil {
		t.Error("the store's bookkeeping went into the plugin")
	}
	if strings.HasPrefix(plugin, root) || strings.Contains(plugin, ".claude") {
		t.Errorf("the plugin is at %s", plugin)
	}

	plain, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "plain", Mode: "accept_edits"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, plain.ID, "go")
	waitIdle(t, ch, plain.ID)
	if said := agentSaid(t, e, plain.ID); strings.Contains(said, "plugin:") {
		t.Errorf("an agent that takes no skills got some:\n%s", said)
	}

	// The plugin goes with its session.
	if err := e.DeleteSession(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plugin); !os.IsNotExist(err) {
		t.Errorf("the plugin outlived its session: %v", err)
	}
}
