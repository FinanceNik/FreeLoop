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
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// craftedBundle writes a backup file holding the given config files.
func craftedBundle(t *testing.T, files map[string]string) string {
	t.Helper()
	b, err := json.Marshal(config.Bundle{Format: config.BundleFormat, Version: 1, Exported: time.Now(), Files: files})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "backup.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// startedSession creates a session of a profile and sends it a message, so
// whatever runs when a session starts has run. The agent's own start can fail.
func startedSession(t *testing.T, e *Engine, profile string) string {
	t.Helper()
	ctx := context.Background()
	e.mu.Lock()
	e.providers.built["ap"] = &configuredProvider{cfg: config.Provider{ID: "ap", Name: "ap", Type: "scripted"}, p: scripted.New("ap", scripted.Step{Text: "done"})}
	e.mu.Unlock()
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "ap", Model: "m", ProfileID: profile, Mode: "default"})
	if err != nil {
		t.Logf("create session of %q: %v", profile, err)
		return ""
	}
	ch, cancel := e.Subscribe()
	defer cancel()
	if _, err := e.Prompt(ctx, ses.ID, "go"); err != nil {
		t.Logf("prompt in %q: %v", profile, err)
		return ses.ID
	}
	waitIdle(t, ch, ses.ID)
	return ses.ID
}

// SET-005: what a backup brings runs nothing until the user approves it.
func TestRestoredCommandsDoNotRun(t *testing.T) {
	ctx := context.Background()
	marks := t.TempDir()
	global, profile, agent := filepath.Join(marks, "global"), filepath.Join(marks, "profile"), filepath.Join(marks, "agent")
	backup := craftedBundle(t, map[string]string{
		config.FileName:                           "version: 1\nhooks:\n  - event: session_start\n    command: touch " + global + "\n",
		config.ProfilesDir + "/planted.yaml":      "name: Planted\nhooks:\n  - event: session_start\n    command: touch " + profile + "\n",
		config.PresetsDir + "/planted-agent.yaml": "name: Planted agent\ncommand: /bin/sh\nargs: [\"-c\", \"touch " + agent + "\"]\n",
	})
	e := openTest(t, t.TempDir())
	defer e.Close()
	if _, err := e.ImportConfig(ctx, backup); err != nil {
		t.Fatal(err)
	}
	startedSession(t, e, "")
	if exists(global) {
		t.Error("a hook from a restored config.yaml ran")
	}
	startedSession(t, e, "planted")
	if exists(profile) {
		t.Error("a hook from a restored profile ran")
	}
	startedSession(t, e, "planted-agent")
	if exists(agent) {
		t.Error("the command of a restored preset ran")
	}
}

// A restored preset cannot take the place of one that ships with the app, and a
// mark in the backup, even one that looks right, approves nothing.
func TestRestoredPresetAndMarksAreNotTrusted(t *testing.T) {
	ctx := context.Background()
	marker := filepath.Join(t.TempDir(), "ran")
	other := openTest(t, t.TempDir())
	defer other.Close()
	hook := config.Hook{Event: config.HookSessionStart, Command: "touch " + marker}
	foreign, err := other.approvalMark(hook.Fingerprint())
	if err != nil {
		t.Fatal(err)
	}
	backup := craftedBundle(t, map[string]string{
		config.FileName: "version: 1\ncommandApprovals: [" + foreign + "]\nhooks:\n  - event: session_start\n    command: touch " + marker + "\n",
		config.PresetsDir + "/" + config.ClaudeCodePresetID + ".yaml": "name: Claude Code\ncommand: /bin/sh\nargs: [\"-c\", \"touch " + marker + "\"]\n",
	})
	e := openTest(t, t.TempDir())
	defer e.Close()
	res, err := e.ImportConfig(ctx, backup)
	if err != nil {
		t.Fatal(err)
	}
	if res.NeedApproval != 2 {
		t.Errorf("need approval = %d", res.NeedApproval)
	}
	startedSession(t, e, "")
	startedSession(t, e, config.ClaudeCodePresetID)
	if exists(marker) {
		t.Error("a restored command ran on a mark made elsewhere, or in place of a built-in preset")
	}
	i := slices.IndexFunc(e.ListHarnesses(), func(h HarnessInfo) bool { return h.ID == config.ClaudeCodePresetID })
	if i < 0 || !strings.Contains(e.ListHarnesses()[i].Missing, "approve") {
		t.Errorf("harnesses = %+v", e.ListHarnesses())
	}
}

// Approving runs a command from then on, as it is: a change to the file ends it.
func TestApprovingACommand(t *testing.T) {
	ctx := context.Background()
	marks := t.TempDir()
	hookRan, agentRan := filepath.Join(marks, "hook"), filepath.Join(marks, "agent")
	backup := craftedBundle(t, map[string]string{
		config.FileName: "version: 1\nhooks:\n  - event: session_start\n    command: touch " + hookRan + "\n",
		config.PresetsDir + "/planted-agent.yaml": "name: Planted agent\ncommand: /bin/sh\nargs: [\"-c\", \"touch " + agentRan + "\"]\n",
	})
	e := openTest(t, t.TempDir())
	defer e.Close()
	if _, err := e.ImportConfig(ctx, backup); err != nil {
		t.Fatal(err)
	}
	pending := e.PendingCommands()
	if len(pending) != 2 || len(e.ConfigIssues()) != 2 {
		t.Fatalf("pending = %+v, issues = %+v", pending, e.ConfigIssues())
	}
	sid := startedSession(t, e, "")
	if !slices.ContainsFunc(noticesOf(t, e, sid), func(m string) bool { return strings.Contains(m, "touch "+hookRan) }) {
		t.Errorf("no notice of the hook that did not run: %q", noticesOf(t, e, sid))
	}
	if _, err := e.ApproveCommand("not what the file says"); err == nil {
		t.Error("approved a fingerprint no file has")
	}
	for _, c := range pending {
		if _, err := e.ApproveCommand(c.Fingerprint); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.PendingCommands(); len(got) != 0 {
		t.Fatalf("still pending: %+v", got)
	}
	startedSession(t, e, "")
	if !exists(hookRan) {
		t.Error("an approved hook did not run")
	}
	startedSession(t, e, "planted-agent")
	if !exists(agentRan) {
		t.Error("an approved preset did not run")
	}

	// The same file with another command is another approval.
	os.Remove(hookRan)
	changed := filepath.Join(marks, "changed")
	c := e.Config()
	text := "version: 1\ncommandApprovals: [" + strings.Join(c.CommandApprovals, ", ") + "]\nhooks:\n  - event: session_start\n    command: touch " + changed + "\n"
	if err := os.WriteFile(filepath.Join(e.dataDir, config.FileName), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	e.reload([]string{config.FileName})
	startedSession(t, e, "")
	if exists(changed) {
		t.Error("an edited hook ran on the old approval")
	}
}

// What the user writes in the app runs without asking; saving a form or copying
// a profile does not approve what is waiting.
func TestHooksWrittenInTheApp(t *testing.T) {
	ctx := context.Background()
	marks := t.TempDir()
	typed, inProfile, inYAML, planted := filepath.Join(marks, "typed"), filepath.Join(marks, "profile"), filepath.Join(marks, "yaml"), filepath.Join(marks, "planted")
	e := openTest(t, t.TempDir())
	defer e.Close()

	c := e.Config()
	c.Hooks = []config.Hook{{Event: config.HookSessionStart, Command: "touch " + typed}}
	if _, err := e.SaveSettings(c); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SaveProfile(config.Profile{ID: "mine", Name: "Mine", Hooks: []config.Hook{{Event: config.HookSessionStart, Command: "touch " + inProfile}}}); err != nil {
		t.Fatal(err)
	}
	if err := e.SaveProfileYAML("typed", "name: Typed\nhooks:\n  - event: session_start\n    command: touch "+inYAML+"\n"); err != nil {
		t.Fatal(err)
	}
	if got := e.PendingCommands(); len(got) != 0 {
		t.Fatalf("what the user wrote waits for approval: %+v", got)
	}
	startedSession(t, e, "mine")
	startedSession(t, e, "typed")
	for _, f := range []string{typed, inProfile, inYAML} {
		if !exists(f) {
			t.Errorf("%s did not run", filepath.Base(f))
		}
	}

	// A form holding a hook a backup brought sends it back.
	backup := craftedBundle(t, map[string]string{
		config.FileName:                      "version: 1\nhooks:\n  - event: session_start\n    command: touch " + planted + "\n",
		config.ProfilesDir + "/planted.yaml": "name: Planted\nhooks:\n  - event: session_start\n    command: touch " + planted + "2\n",
	})
	if _, err := e.ImportConfig(ctx, backup); err != nil {
		t.Fatal(err)
	}
	c = e.Config()
	c.UI.FontSize = 15
	c.CommandApprovals = nil
	if _, err := e.SaveSettings(c); err != nil {
		t.Fatal(err)
	}
	p, err := e.profile("planted")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	p.ID, p.Name = "copy", "Copy"
	if _, err := e.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	text, err := e.ProfileYAML("planted")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SaveProfileYAML("planted", text); err != nil {
		t.Fatal(err)
	}
	startedSession(t, e, "planted")
	startedSession(t, e, "copy")
	if exists(planted) || exists(planted+"2") {
		t.Error("saving a form approved a hook a backup brought")
	}
	// The hook in config.yaml, and the one in each of the two profiles.
	if got := e.PendingCommands(); len(got) != 3 {
		t.Errorf("pending = %+v", got)
	}
}

// What a Mac has when this version first starts stays approved, once: a backup
// restored afterwards is asked about, across restarts too.
func TestExistingCommandsAreAdoptedOnce(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	marks := t.TempDir()
	old, agent, planted := filepath.Join(marks, "old"), filepath.Join(marks, "agent"), filepath.Join(marks, "planted")
	text := "version: 1\nhooks:\n  - event: session_start\n    command: touch " + old + "\n"
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.SavePreset(dir, config.HarnessPreset{ID: "mine", Name: "Mine", Command: "/bin/sh", Args: []string{"-c", "touch " + agent}}); err != nil {
		t.Fatal(err)
	}
	store := secrets.NewMemory()
	open := func() *Engine {
		e, err := Open(Options{DataDir: dir, Secrets: store, Home: t.TempDir(), Dev: true})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	e := open()
	if got := e.PendingCommands(); len(got) != 0 {
		t.Fatalf("existing commands wait for approval: %+v", got)
	}
	startedSession(t, e, "")
	if !exists(old) {
		t.Error("an existing hook stopped running")
	}
	startedSession(t, e, "mine")
	if !exists(agent) {
		t.Error("an existing preset stopped running")
	}
	backup := craftedBundle(t, map[string]string{
		config.ProfilesDir + "/planted.yaml": "name: Planted\nhooks:\n  - event: session_start\n    command: touch " + planted + "\n",
	})
	if _, err := e.ImportConfig(ctx, backup); err != nil {
		t.Fatal(err)
	}
	e.Close()
	e = open()
	defer e.Close()
	if got := e.PendingCommands(); len(got) != 1 {
		t.Errorf("after a restart, pending = %+v", got)
	}
	startedSession(t, e, "planted")
	if exists(planted) {
		t.Error("a restart adopted a restored hook")
	}
}

// A scratch data folder opened first does not use up the real one's adoption.
func TestAdoptionIsPerDataFolder(t *testing.T) {
	store := secrets.NewMemory()
	scratch, err := Open(Options{DataDir: t.TempDir(), Secrets: store, Home: t.TempDir(), Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	scratch.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte("version: 1\nhooks:\n  - event: session_start\n    command: \"true\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e, err := Open(Options{DataDir: dir, Secrets: store, Home: t.TempDir(), Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if got := e.PendingCommands(); len(got) != 0 {
		t.Errorf("pending = %+v", got)
	}
}

// A project's hooks are covered by trusting the project, not by these approvals.
func TestTrustedProjectHooksNeedNoApproval(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	root := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	if err := os.MkdirAll(filepath.Join(root, ProjectDir), 0o700); err != nil {
		t.Fatal(err)
	}
	text := "hooks:\n  - event: session_start\n    command: touch " + marker + "\n"
	if err := os.WriteFile(filepath.Join(root, ProjectDir, config.HooksFileName), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SetProjectTrust(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	e.providers.built["ap"] = &configuredProvider{cfg: config.Provider{ID: "ap", Name: "ap", Type: "scripted"}, p: scripted.New("ap", scripted.Step{Text: "done"})}
	e.mu.Unlock()
	ch, cancel := e.Subscribe()
	defer cancel()
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "ap", Model: "m", Mode: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Prompt(ctx, ses.ID, "go"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)
	if !exists(marker) {
		t.Error("a trusted project's hook did not run")
	}
}

// approvePending approves what tests put in place as files.
func approvePending(t *testing.T, e *Engine) {
	t.Helper()
	for _, c := range e.PendingCommands() {
		if _, err := e.ApproveCommand(c.Fingerprint); err != nil {
			t.Fatal(err)
		}
	}
}

func noticesOf(t *testing.T, e *Engine, sid string) []string {
	t.Helper()
	evs, _ := e.Events(context.Background(), sid, 0)
	var out []string
	for _, ev := range evs {
		if ev.Type == event.Notice {
			var d event.NoticeData
			json.Unmarshal(ev.Data, &d)
			out = append(out, d.Message)
		}
	}
	return out
}
