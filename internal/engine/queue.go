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
			if note := budgetHeld(budgets, cmp.Or(t.ProfileID, config.DefaultProfileID)); note != "" {
				if t.Note != note {
					t.Note, changed = note, true
				}
				continue
			}
			agent := e.taskAgent(ctx, *t)
			if until, ok := held[agent]; ok {
				note := fmt.Sprintf("%s is at its limit until %s", agent, until.Local().Format("Mon 15:04"))
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
	project := session.Project{Root: t.ProjectRoot}
	if p, err := e.store.ProjectByRoot(ctx, t.ProjectRoot); err == nil {
		project = p
	}
	prof, err := e.profileFor(project, cmp.Or(t.ProfileID, config.DefaultProfileID))
	if err != nil || prof.Harness == "" || prof.Harness == "builtin" {
		return ""
	}
	if p, err := e.preset(prof.Harness); err == nil {
		return p.Name
	}
	return prof.Harness
}

// agentsAtLimit names the agents whose account limit is at a warning or reached, with
// when it resets (GUI-028).
func (e *Engine) agentsAtLimit(ctx context.Context, now time.Time) map[string]time.Time {
	out := map[string]time.Time{}
	periods, err := e.Limits(ctx)
	if err != nil {
		return out
	}
	for _, p := range periods {
		if !p.ResetsAt.After(now) || (p.Warned == 0 && p.Refused == 0) {
			continue
		}
		if until, ok := out[p.Source]; !ok || p.ResetsAt.After(until) {
			out[p.Source] = p.ResetsAt
		}
	}
	return out
}

// startTask starts a task's session and sends its prompt.
func (e *Engine) startTask(ctx context.Context, t config.Task) (string, error) {
	ses, err := e.CreateSession(ctx, CreateSessionRequest{Title: t.Title, ProjectRoot: t.ProjectRoot, ProfileID: t.ProfileID, Worktree: t.Worktree})
	if err != nil {
		return "", err
	}
	e.SetTags(ctx, ses.ID, []string{QueuedTag})
	e.emit(ses.ID, event.Notice, event.NoticeData{Message: "Started from the queue."})
	if _, err := e.Prompt(ctx, ses.ID, t.Prompt); err != nil {
		return ses.ID, err
	}
	return ses.ID, nil
}
