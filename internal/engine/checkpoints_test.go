package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/provider"
)

// byPrompt writes the file the latest prompt names, then ends the turn.
type byPrompt struct {
	mu       sync.Mutex
	requests []provider.ChatRequest
}

func (b *byPrompt) ID() string                                               { return "byprompt" }
func (b *byPrompt) Type() provider.Type                                      { return provider.TypeScripted }
func (b *byPrompt) ListModels(context.Context) ([]provider.ModelInfo, error) { return nil, nil }
func (b *byPrompt) Capabilities(string) provider.ModelCaps {
	return provider.ModelCaps{Source: provider.CapsStatic, Tools: true}
}

func (b *byPrompt) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.Event, error) {
	b.mu.Lock()
	b.requests = append(b.requests, req)
	b.mu.Unlock()
	last := req.Messages[len(req.Messages)-1]
	msg := provider.Message{Role: provider.RoleAssistant, Parts: []provider.Part{{Type: provider.PartText, Text: "done"}}}
	stop := provider.StopEndTurn
	if last.Parts[0].Type != provider.PartToolResult {
		// "write <path> <content>"
		f := strings.Fields(last.Parts[0].Text)
		in, _ := json.Marshal(map[string]string{"path": f[1], "content": f[2] + "\n"})
		msg.Parts = []provider.Part{{Type: provider.PartToolUse, ToolCallID: "c" + f[1], ToolName: "write_file", Input: in}}
		stop = provider.StopToolUse
	}
	ch := make(chan provider.Event, 1)
	ch <- provider.Event{Type: provider.EventDone, Message: &msg, StopReason: stop}
	close(ch)
	return ch, nil
}

// SES-014: rewinding to before a turn brings its files back, removes what it
// added, leaves ignored files alone, and the next model call's context ends
// where the checkpoint was; the rewind keeps a snapshot of the files it
// replaced, and a restart rebuilds the rewound context.
func TestCheckpointsAndRewind(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	prov := &byPrompt{}
	e.mu.Lock()
	e.providers.built["bp"] = &configuredProvider{cfg: config.Provider{ID: "bp", Name: "bp", Type: "scripted"}, p: prov}
	e.mu.Unlock()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.txt\n"), 0o644)
	os.MkdirAll(filepath.Join(root, ".claude"), 0o755)
	os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte("{}\n"), 0o644)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "bp", Model: "m", Mode: "accept_edits"})
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return "<missing>"
		}
		return string(b)
	}
	e.Prompt(ctx, ses.ID, "write a.txt one")
	waitIdle(t, ch, ses.ID)
	e.Prompt(ctx, ses.ID, "write b.txt two")
	waitIdle(t, ch, ses.ID)
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("edited by hand\n"), 0o644)
	os.WriteFile(filepath.Join(root, "ignored.txt"), []byte("keep me\n"), 0o644)
	// A change the user makes in .claude/ after the checkpoint stays.
	os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte("{\"mine\": true}\n"), 0o644)

	evs, _ := e.Events(ctx, ses.ID, 0)
	var cps []event.Event
	for _, ev := range evs {
		if ev.Type == event.Checkpoint {
			cps = append(cps, ev)
		}
	}
	if len(cps) != 2 {
		t.Fatalf("checkpoints = %d", len(cps))
	}
	if err := e.Rewind(ctx, ses.ID, cps[1].Seq); err != nil {
		t.Fatal(err)
	}
	if read(".claude/settings.json") != "{\"mine\": true}\n" {
		t.Errorf("the rewind changed .claude/: %q", read(".claude/settings.json"))
	}
	if read("a.txt") != "one\n" || read("b.txt") != "<missing>" || read("ignored.txt") != "keep me\n" {
		t.Fatalf("after rewind: a = %q, b = %q, ignored = %q", read("a.txt"), read("b.txt"), read("ignored.txt"))
	}
	n := len(prov.requests)
	e.Prompt(ctx, ses.ID, "write c.txt three")
	waitIdle(t, ch, ses.ID)
	first := prov.requests[n]
	var cp event.CheckpointData
	json.Unmarshal(cps[1].Data, &cp)
	if len(first.Messages) != cp.HistoryLen+1 || strings.Contains(first.Messages[len(first.Messages)-1].Parts[0].Text, "b.txt") {
		t.Errorf("after the rewind the model saw %d messages, the checkpoint had %d", len(first.Messages), cp.HistoryLen)
	}

	// The rewind event names the snapshot of the files it replaced.
	evs, _ = e.Events(ctx, ses.ID, 0)
	var rw event.RewindData
	for _, ev := range evs {
		if ev.Type == event.Rewind {
			json.Unmarshal(ev.Data, &rw)
		}
	}
	if !rw.Context || rw.Saved == "" {
		t.Fatalf("rewind = %+v", rw)
	}
	// The transcript rebuilds the rewound context after a restart.
	e.mu.Lock()
	r := e.runtimes[ses.ID]
	e.mu.Unlock()
	live := r.hs.(History).HistoryLen()
	if got := len(builtin.Restore(evs)); got != live || live != cp.HistoryLen+4 {
		t.Errorf("restored %d messages, live %d, checkpoint %d", got, live, cp.HistoryLen)
	}
}

// Once a compaction (or an earlier rewind) has replaced the history, a
// checkpoint's length no longer points into it: rewinding restores the
// files and leaves the context alone instead of cutting the wrong messages.
func TestRewindAfterHistoryResetKeepsContext(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	prov := &byPrompt{}
	e.mu.Lock()
	e.providers.built["bp"] = &configuredProvider{cfg: config.Provider{ID: "bp", Name: "bp", Type: "scripted"}, p: prov}
	e.mu.Unlock()
	root := t.TempDir()
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "bp", Model: "m", Mode: "accept_edits"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, ses.ID, "write a.txt one")
	waitIdle(t, ch, ses.ID)
	e.Prompt(ctx, ses.ID, "write b.txt two")
	waitIdle(t, ch, ses.ID)
	e.mu.Lock()
	h := e.runtimes[ses.ID].hs.(History)
	e.mu.Unlock()
	// What a compaction does to the history: replace it, shorter.
	h.TruncateHistory(1)
	before := h.HistoryLen()

	evs, _ := e.Events(ctx, ses.ID, 0)
	var cps []event.Event
	for _, ev := range evs {
		if ev.Type == event.Checkpoint {
			cps = append(cps, ev)
		}
	}
	if err := e.Rewind(ctx, ses.ID, cps[1].Seq); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "b.txt")); err == nil {
		t.Error("the files were not restored")
	}
	if h.HistoryLen() != before {
		t.Errorf("history %d after the rewind, %d before", h.HistoryLen(), before)
	}
	evs, _ = e.Events(ctx, ses.ID, 0)
	if ev := evs[len(evs)-1]; ev.Type != event.Rewind || strings.Contains(string(ev.Data), `"context":true`) {
		t.Errorf("last event = %s %s", ev.Type, ev.Data)
	}
}

// SES-014: a project that ignores .claude or .freeloop still gets checkpoints. Naming
// an ignored folder in git add's exclusions is an error, even with everything else
// added, and that turned checkpoints off. Neither folder goes in, whatever the project
// ignores or takes back with a !, since a rewind must never change them.
func TestSnapshotLeavesOutClaudeWhateverIsIgnored(t *testing.T) {
	ctx := context.Background()
	for _, ignore := range []string{
		"",
		".claude\n",
		".claude/\n",
		".claude\n.freeloop\n",
		".claude/*\n",
		".claude/local.md\n",
		".claude/*\n!.claude/commands/\n",
		"/.claude/\n/.freeloop/\n",
	} {
		t.Run(strings.ReplaceAll(ignore, "\n", " "), func(t *testing.T) {
			e := openTest(t, t.TempDir())
			defer e.Close()
			root := t.TempDir()
			for path, text := range map[string]string{
				".claude/local.md":     "secret",
				".claude/commands/c":   "c",
				".freeloop/rules.yaml": "r",
				"a.txt":                "a",
				"build.log":            "l",
				".gitignore":           ignore + "*.log\n",
			} {
				full := filepath.Join(root, path)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			host, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: e.broker.Policy().Guard(root)})
			if err != nil {
				t.Fatal(err)
			}
			commit, err := e.snapshot(ctx, host, "ses_test", "checkpoint")
			if err != nil {
				t.Fatalf("snapshot: %v", err)
			}
			files, err := e.shadowGit(ctx, host, "ses_test", time.Minute, "ls-tree", "-r", "--name-only", commit)
			if err != nil {
				t.Fatal(err)
			}
			if files != ".gitignore\na.txt" {
				t.Errorf("the snapshot holds:\n%s", files)
			}
		})
	}
}
