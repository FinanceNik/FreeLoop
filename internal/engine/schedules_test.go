package engine

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
)

// A due schedule starts one tagged session with its prompt, records the
// run, and does not run again until its next time (SES-012).
func TestSchedules(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	root := t.TempDir()
	if _, err := e.SaveSchedule(config.Schedule{Name: "Nightly", Prompt: "check", ProjectRoot: root, Repeat: config.RepeatDaily, At: "09:00", Mode: "bypass"}); err == nil {
		t.Error("a bypass schedule was saved")
	}
	s, err := e.SaveSchedule(config.Schedule{Name: " Nightly ", Prompt: "check the build", ProjectRoot: root, ProviderID: DemoProviderID, Repeat: config.RepeatDaily, At: time.Now().Add(12 * time.Hour).Format("15:04"), Mode: "ask", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if s.ID == "" || s.Name != "Nightly" {
		t.Fatalf("saved = %+v", s)
	}

	e.runDue(ctx, s.CreatedAt.Add(time.Minute))
	if n := len(scheduledSessions(t, e)); n != 0 {
		t.Fatalf("ran before its time: %d sessions", n)
	}
	later := s.CreatedAt.Add(25 * time.Hour)
	e.runDue(ctx, later)
	e.runDue(ctx, later.Add(time.Minute))
	got := scheduledSessions(t, e)
	if len(got) != 1 {
		t.Fatalf("sessions after the run = %d", len(got))
	}
	// The turn writes the prompt from its own goroutine.
	var prompt, notice bool
	for deadline := time.Now().Add(3 * time.Second); !prompt && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		evs, _ := e.Events(ctx, got[0], 0)
		for _, ev := range evs {
			prompt = prompt || ev.Type == event.UserMessageChunk && strings.Contains(string(ev.Data), "check the build")
			notice = notice || ev.Type == event.Notice && strings.Contains(string(ev.Data), `the schedule \"Nightly\"`)
		}
	}
	if !prompt || !notice {
		t.Errorf("prompt %v, notice %v", prompt, notice)
	}
	list, _ := e.Schedules()
	if len(list) != 1 || list[0].LastSession != got[0] || !list[0].LastRun.Equal(later.UTC()) || list[0].LastError != "" {
		t.Errorf("recorded = %+v", list)
	}

	// Run now works while the schedule is off; a missing folder is an
	// error recorded on it.
	s.Enabled = false
	e.SaveSchedule(s)
	if sid, err := e.RunSchedule(ctx, s.ID); err != nil || sid == "" {
		t.Errorf("run now = %q, %v", sid, err)
	}
	s.ProjectRoot = root + "/gone"
	e.SaveSchedule(s)
	if _, err := e.RunSchedule(ctx, s.ID); err == nil {
		t.Error("ran in a missing folder")
	}
	if list, _ := e.Schedules(); list[0].LastError == "" {
		t.Error("the failure was not recorded")
	}
	if err := e.DeleteSchedule(s.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := e.Schedules(); len(list) != 0 {
		t.Errorf("after delete: %+v", list)
	}
}

func scheduledSessions(t *testing.T, e *Engine) []string {
	t.Helper()
	list, err := e.store.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range list {
		if slices.Contains(s.Tags, ScheduledTag) {
			out = append(out, s.ID)
		}
	}
	return out
}
