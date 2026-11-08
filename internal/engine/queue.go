package engine

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// A queue of tasks (SES-025). Each check of the schedule loop starts waiting tasks,
// first in the list first, while fewer than the queue's limit are running and the
// task's agent is not at a limit warning, whose reset it then waits for. A started task
// is an ordinary session tagged "queued": every tool call asks as usual, and one waiting
// for an approval keeps its place among the running.

// QueuedTag marks the sessions the queue starts.
const QueuedTag = "queued"

// Queue returns the queue.
func (e *Engine) Queue() (config.Queue, error) {
	e.queueMu.Lock()
	defer e.queueMu.Unlock()
	return config.LoadQueue(e.dataDir)
}

// change loads the queue, lets fn change it and saves it.
func (e *Engine) changeQueue(fn func(q *config.Queue) error) (config.Queue, error) {
	e.queueMu.Lock()
	defer e.queueMu.Unlock()
	q, err := config.LoadQueue(e.dataDir)
	if err != nil {
		return q, err
	}
	was, _ := json.Marshal(q)
	if err := fn(&q); err != nil {
		return q, err
	}
	// The schedule loop looks every 30 seconds; a queue it left as it was stays unwritten.
	if now, _ := json.Marshal(q); bytes.Equal(was, now) {
		return q, nil
	}
	return q, config.SaveQueue(e.dataDir, q)
}

// AddTask puts a task at the end of the queue.
func (e *Engine) AddTask(t config.Task) (config.Queue, error) {
	if err := t.Validate(); err != nil {
		return config.Queue{}, err
	}
	t.ID = fmt.Sprintf("task_%d", time.Now().UnixNano())
	t.State, t.AddedAt, t.SessionID, t.Note = config.TaskWaiting, time.Now().UTC(), "", ""
	if strings.TrimSpace(t.Title) == "" {
		t.Title = firstLine(t.Prompt, 60)
	}
	return e.changeQueue(func(q *config.Queue) error {
		q.Tasks = append(q.Tasks, t)
		return nil
	})
}

func firstLine(s string, n int) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if len(s) > n {
		s = s[:n] + "…"
	}
	return s
}

// MoveTask moves a waiting task by by places among the tasks.
func (e *Engine) MoveTask(id string, by int) (config.Queue, error) {
	return e.changeQueue(func(q *config.Queue) error {
		i := slices.IndexFunc(q.Tasks, func(t config.Task) bool { return t.ID == id })
		if i < 0 {
			return errors.New("no such task")
		}
		j := min(max(i+by, 0), len(q.Tasks)-1)
		t := q.Tasks[i]
		q.Tasks = slices.Insert(slices.Delete(q.Tasks, i, i+1), j, t)
		return nil
	})
}

// RemoveTask takes a task off the queue; a session it started stays.
func (e *Engine) RemoveTask(id string) (config.Queue, error) {
	return e.changeQueue(func(q *config.Queue) error {
		q.Tasks = slices.DeleteFunc(q.Tasks, func(t config.Task) bool { return t.ID == id })
		return nil
	})
}

// SetQueue pauses or resumes the queue and sets how many tasks run at once.
func (e *Engine) SetQueue(paused bool, limit int) (config.Queue, error) {
	if limit < 0 || limit > 20 {
		return config.Queue{}, errors.New("the queue runs 1 to 20 tasks at once")
	}
	return e.changeQueue(func(q *config.Queue) error {
		q.Paused, q.Limit = paused, limit
		return nil
	})
}

// runQueue brings running tasks' states up to date and starts what there is room for.
func (e *Engine) runQueue(ctx context.Context, now time.Time) {
	changed := false
	q, err := e.changeQueue(func(q *config.Queue) error {
		running := 0
		for i := range q.Tasks {
			t := &q.Tasks[i]
			if t.State != config.TaskRunning {
				continue
			}
			if state := e.taskState(ctx, t.SessionID); state != t.State {
				t.State, changed = state, true
				continue
			}
			running++
		}
		if q.Paused {
			return nil
		}
		held := e.agentsAtLimit(ctx, now)
		budgets, _ := e.budgetStates(ctx, now)
		for i := range q.Tasks {
			t := &q.Tasks[i]
			if t.State != config.TaskWaiting || running >= q.RunLimit() {
				continue
			}
			if note := budgetHeld(budgets, e.taskProfile(ctx, *t).ID); note != "" {
				if t.Note != note {
					t.Note, changed = note, true
				}
				continue
			}
			agent := e.taskAgent(ctx, *t)
			if h, ok := held[agent]; ok {
				note := h.note(agent)
				if t.Note != note {
					t.Note, changed = note, true
				}
				continue
			}
			sid, err := e.startTask(ctx, *t)
			t.SessionID, t.StartedAt, t.Note, changed = sid, now.UTC(), "", true
			if err != nil {
				t.State, t.Note = config.TaskFailed, err.Error()
				continue
			}
			t.State = config.TaskRunning
			running++
		}
		return nil
	})
	if err != nil {
		log.Printf("engine: queue: %v", err)
		return
	}
	if changed {
		e.notifyQueue(q)
	}
}

// notifyQueue tells the interface that the queue changed.
func (e *Engine) notifyQueue(q config.Queue) {
	if e.onQueue != nil {
		e.onQueue()
	}
}

// taskState is what a started task's session says about it: running until a turn has
// ended with the session idle, done then or once archived, failed in error.
func (e *Engine) taskState(ctx context.Context, sid string) string {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return config.TaskFailed
	}
	switch ses.Status {
	case session.StatusArchived:
		return config.TaskDone
	case session.StatusError:
		return config.TaskFailed
	case session.StatusIdle:
		evs, err := e.store.Events(ctx, sid, 0)
		if err == nil && slices.ContainsFunc(evs, func(ev event.Event) bool { return ev.Type == event.TurnEnded }) {
			return config.TaskDone
		}
	}
	return config.TaskRunning
}

// taskAgent names the agent a task's session would run, as account limits name it.
func (e *Engine) taskAgent(ctx context.Context, t config.Task) string {
	prof := e.taskProfile(ctx, t)
	if prof.Harness == "" || prof.Harness == "builtin" {
		return ""
	}
	if p, err := e.preset(prof.Harness); err == nil {
		return p.Name
	}
	return prof.Harness
}

// taskProfile is the profile a task's session will have, chosen as CreateSession
// chooses it: the task's, else the project's default, else the app's.
func (e *Engine) taskProfile(ctx context.Context, t config.Task) config.Profile {
	project := session.Project{Root: t.ProjectRoot}
	if p, err := e.store.ProjectByRoot(ctx, environment.Canonical(t.ProjectRoot)); err == nil {
		project = p
	}
	prof, err := e.profileFor(project, cmp.Or(t.ProfileID, project.DefaultProfileID, e.Config().Defaults.ProfileID))
	if err != nil {
		return config.Profile{ID: cmp.Or(t.ProfileID, config.DefaultProfileID)}
	}
	return prof
}

// weeklyHoldPercent is how full a weekly limit gets before the queue holds its
// agent's tasks. A weekly warning comes days before the reset, and holding from it
// would leave most of the week's allowance unused; a shorter window's warning holds
// at once, since its reset is hours away (SES-025).
const weeklyHoldPercent = 95

// limitHold is why an agent's tasks wait: the limit holding them and its reset.
type limitHold struct {
	Label string
	Used  float64
	Until time.Time
}

// note says it in words for the queue and the AFK run.
func (h limitHold) note(agent string) string {
	return fmt.Sprintf("%s's %s is at %.0f %% until %s", agent, strings.ToLower(cmp.Or(h.Label, "limit")), h.Used, h.Until.Local().Format("Mon 15:04"))
}

// weeklyLimit reports a limit window that runs for a week.
func weeklyLimit(p LimitPeriod) bool {
	return p.Window == "seven_day" || strings.Contains(strings.ToLower(p.Label), "week")
}

// holds reports whether a limit period holds its agent's tasks now: once refused,
// or at a warning, which for a weekly limit counts only from weeklyHoldPercent.
func (p LimitPeriod) holds(now time.Time) bool {
	switch {
	case !p.ResetsAt.After(now):
		return false
	case p.Refused > 0:
		return true
	case p.Warned == 0:
		return false
	case weeklyLimit(p):
		return p.Used >= weeklyHoldPercent
	}
	return true
}

// agentsAtLimit names the agents whose account limit holds their tasks, with the
// limit and when it resets (GUI-028). The one resetting last wins.
func (e *Engine) agentsAtLimit(ctx context.Context, now time.Time) map[string]limitHold {
	out := map[string]limitHold{}
	periods, err := e.Limits(ctx)
	if err != nil {
		return out
	}
	for _, p := range periods {
		if !p.holds(now) {
			continue
		}
		if h, ok := out[p.Source]; !ok || p.ResetsAt.After(h.Until) {
			out[p.Source] = limitHold{Label: p.Label, Used: p.Used, Until: p.ResetsAt}
		}
	}
	return out
}

// limitNote says why a profile's tasks would wait for its agent's limit now, or "".
func (e *Engine) limitNote(ctx context.Context, root, profileID string) string {
	agent := e.taskAgent(ctx, config.Task{ProjectRoot: root, ProfileID: profileID})
	if h, ok := e.agentsAtLimit(ctx, time.Now())[agent]; ok && agent != "" {
		return h.note(agent)
	}
	return ""
}

// startTask starts a task's session and sends its prompt.
func (e *Engine) startTask(ctx context.Context, t config.Task) (string, error) {
	req := CreateSessionRequest{Title: t.Title, ProjectRoot: t.ProjectRoot, ProfileID: t.ProfileID, Worktree: t.Worktree, Mode: t.Mode, EnvironmentID: t.EnvironmentID, Workflow: t.Workflow}
	if t.ShareWorktreeOf != "" {
		anchor, err := e.store.GetSession(ctx, t.ShareWorktreeOf)
		if err != nil || anchor.Worktree == "" || !environment.IsDir(anchor.Worktree) {
			return "", errors.New("the worktree it was to share is gone")
		}
		req.shareWorktree, req.Worktree = &anchor, false
	}
	ses, err := e.CreateSession(ctx, req)
	if err != nil {
		return "", err
	}
	e.SetTags(ctx, ses.ID, append([]string{QueuedTag}, t.Tags...))
	// What the user allowed for an AFK run's sessions when starting it (WFL-005).
	for _, r := range sessionRules(t.Allow) {
		if err := e.AddSessionRule(ses.ID, r); err != nil {
			return ses.ID, err
		}
	}
	note := "Started from the queue."
	if t.RunID != "" {
		note = "Started by an AFK run: it works on one issue unattended. Requests wait for you in Needs attention."
	}
	e.emit(ses.ID, event.Notice, event.NoticeData{Message: note})
	// A run's prompt quotes the backlog, which agents wrote: its @ words attach nothing.
	if _, err := e.enqueue(ctx, ses.ID, queued{text: t.Prompt, noMentions: t.RunID != ""}); err != nil {
		return ses.ID, err
	}
	return ses.ID, nil
}
