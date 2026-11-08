package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

func queueEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	e := openBinding(t, t.TempDir(), secrets.NewMemory())
	steps := make([]scripted.Step, 10)
	for i := range steps {
		steps[i] = scripted.Step{Text: "done"}
	}
	withScript(e, "s", steps...)
	if err := e.SaveProfileYAML("main", "name: Main\nprovider: s\nmodel: m\n"); err != nil {
		t.Fatal(err)
	}
	return e, t.TempDir()
}

func states(q config.Queue) []string {
	out := []string{}
	for _, t := range q.Tasks {
		out = append(out, t.State)
	}
	return out
}

// SES-025: tasks start under the queue's limit, the next once one is done, each as an
// ordinary session tagged queued.
func TestQueueStartsUnderItsLimit(t *testing.T) {
	ctx := context.Background()
	e, root := queueEngine(t)
	if _, err := e.SetQueue(false, 2); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"first", "second", "third"} {
		if _, err := e.AddTask(config.Task{Prompt: p, ProjectRoot: root, ProfileID: "main"}); err != nil {
			t.Fatal(err)
		}
	}
	ch, cancel := e.Subscribe()
	defer cancel()
	e.runQueue(ctx, time.Now())
	q, _ := e.Queue()
	if got := states(q); got[0] != "running" || got[1] != "running" || got[2] != "waiting" {
		t.Fatalf("after the first check: %v", got)
	}
	ses, _ := e.store.GetSession(ctx, q.Tasks[0].SessionID)
	if len(ses.Tags) != 1 || ses.Tags[0] != QueuedTag || ses.Title != "first" {
		t.Errorf("the session: %+v", ses)
	}
	waitIdle(t, ch, q.Tasks[0].SessionID, q.Tasks[1].SessionID)
	// The queue reads the stored status, written just after turn_ended.
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		a, _ := e.store.GetSession(ctx, q.Tasks[0].SessionID)
		b, _ := e.store.GetSession(ctx, q.Tasks[1].SessionID)
		if a.Status == "idle" && b.Status == "idle" {
			break
		}
	}
	e.runQueue(ctx, time.Now())
	q, _ = e.Queue()
	if got := states(q); got[0] != "done" || got[1] != "done" || got[2] != "running" {
		t.Errorf("after both finished: %v", got)
	}
}

// SES-025: a paused queue starts nothing, and a bad task fails without stopping others.
func TestQueuePausedAndFailing(t *testing.T) {
	ctx := context.Background()
	e, root := queueEngine(t)
	e.AddTask(config.Task{Prompt: "a", ProjectRoot: root, ProfileID: "main"})
	if _, err := e.SetQueue(true, 0); err != nil {
		t.Fatal(err)
	}
	e.runQueue(ctx, time.Now())
	if q, _ := e.Queue(); q.Tasks[0].State != "waiting" {
		t.Errorf("a paused queue started %v", states(q))
	}
	if _, err := e.AddTask(config.Task{Prompt: "", ProjectRoot: root}); err == nil {
		t.Error("a task without a prompt was taken")
	}
	e.AddTask(config.Task{Prompt: "b", ProjectRoot: root, ProfileID: "no-such-profile"})
	e.SetQueue(false, 0)
	e.runQueue(ctx, time.Now())
	q, _ := e.Queue()
	if got := states(q); got[0] != "running" || got[1] != "failed" || q.Tasks[1].Note == "" {
		t.Errorf("%v, %q", got, q.Tasks[1].Note)
	}
}

// SES-025: an agent at a limit warning holds its own tasks until the reset, and not
// the others'.
func TestQueueWaitsForAnAgentsLimit(t *testing.T) {
	ctx := context.Background()
	e, root := queueEngine(t)
	if err := e.SaveProfileYAML("cc", "name: CC\nharness: claude-code\n"); err != nil {
		t.Fatal(err)
	}
	other, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	resets := time.Now().Add(2 * time.Hour)
	e.emit(other.ID, event.LimitUpdate, event.LimitData{Source: "Claude Code", Windows: []event.LimitWindow{{ID: "five_hour", Label: "Session limit (5 hours)", UsedPercent: 92, ResetsAt: resets.Unix(), Status: "warning"}}})
	e.store.IndexAll(ctx)
	e.AddTask(config.Task{Prompt: "on claude", ProjectRoot: root, ProfileID: "cc"})
	e.AddTask(config.Task{Prompt: "built-in", ProjectRoot: root, ProfileID: "main"})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if held := e.agentsAtLimit(ctx, time.Now()); len(held) == 1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	e.runQueue(ctx, time.Now())
	q, _ := e.Queue()
	if got := states(q); got[0] != "waiting" || got[1] != "running" {
		t.Fatalf("%v", got)
	}
	if q.Tasks[0].Note == "" {
		t.Error("the held task does not say why")
	}
	// After the reset it starts.
	e.runQueue(ctx, resets.Add(time.Minute))
	if q, _ := e.Queue(); q.Tasks[0].State == "waiting" && q.Tasks[0].Note == "" {
		t.Errorf("after the reset: %v", states(q))
	}
}

// SES-025: a weekly limit's warning holds only from 95 %; a shorter window's
// warning holds at once, and a refusal always does.
func TestLimitHolds(t *testing.T) {
	now := time.Now()
	later := now.Add(30 * time.Hour)
	for _, c := range []struct {
		name string
		p    LimitPeriod
		want bool
	}{
		{"weekly warning at 86 %", LimitPeriod{Window: "seven_day", Label: "Weekly limit", Used: 86, Warned: 3, ResetsAt: later}, false},
		{"weekly warning at 95 %", LimitPeriod{Window: "seven_day", Label: "Weekly limit", Used: 95, Warned: 3, ResetsAt: later}, true},
		{"weekly by its label", LimitPeriod{Window: "w", Label: "Weekly limit", Used: 90, Warned: 1, ResetsAt: later}, false},
		{"weekly refused", LimitPeriod{Window: "seven_day", Used: 80, Refused: 1, ResetsAt: later}, true},
		{"5-hour warning", LimitPeriod{Window: "five_hour", Label: "5-hour limit", Used: 80, Warned: 1, ResetsAt: later}, true},
		{"no warning", LimitPeriod{Window: "five_hour", Used: 99, ResetsAt: later}, false},
		{"reset already", LimitPeriod{Window: "five_hour", Used: 99, Refused: 1, ResetsAt: now.Add(-time.Minute)}, false},
	} {
		if got := c.p.holds(now); got != c.want {
			t.Errorf("%s: holds = %v", c.name, got)
		}
	}
	h := limitHold{Label: "Weekly limit", Used: 96, Until: later}
	if note := h.note("Claude Code"); !strings.Contains(note, "Claude Code's weekly limit is at 96 % until") {
		t.Errorf("note = %q", note)
	}
}

// SES-025: tasks move within the list and leave it.
func TestQueueOrder(t *testing.T) {
	e, root := queueEngine(t)
	e.SetQueue(true, 0)
	for _, p := range []string{"a", "b", "c"} {
		e.AddTask(config.Task{Prompt: p, ProjectRoot: root})
	}
	q, _ := e.Queue()
	q, _ = e.MoveTask(q.Tasks[2].ID, -2)
	if q.Tasks[0].Title != "c" {
		t.Errorf("moved: %v", q.Tasks)
	}
	q, _ = e.RemoveTask(q.Tasks[1].ID)
	if len(q.Tasks) != 2 || q.Tasks[1].Title != "b" {
		t.Errorf("removed: %v", q.Tasks)
	}
	if _, err := e.SetQueue(false, 99); err == nil {
		t.Error("a limit of 99 was taken")
	}
}

// The schedule loop looks at the queue every 30 seconds; when nothing changes, it
// writes nothing, so the data folder only changes when something did.
func TestAnIdleQueueWritesNothing(t *testing.T) {
	e, _ := queueEngine(t)
	e.runQueue(context.Background(), time.Now())
	if _, err := os.Stat(filepath.Join(e.dataDir, config.QueueFile)); !os.IsNotExist(err) {
		t.Errorf("an empty queue was written: %v", err)
	}
}
