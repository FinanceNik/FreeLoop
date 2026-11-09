package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// QueueFile holds the queue of tasks (SES-025). The app writes each task's state into
// it, so, like schedules, it is apart from the files the app watches.
const QueueFile = "queue.json"

// DefaultQueueLimit is how many of the queue's tasks run at once unless set.
const DefaultQueueLimit = 3

// Task states.
const (
	TaskWaiting = "waiting"
	TaskRunning = "running"
	TaskDone    = "done"
	TaskFailed  = "failed"
)

// Queue is tasks waiting to start as sessions when there is room.
type Queue struct {
	Paused bool `json:"paused"`
	// Limit is how many tasks run at once; DefaultQueueLimit when 0.
	Limit int    `json:"limit"`
	Tasks []Task `json:"tasks"`
}

// Task is one piece of work for the queue: a prompt for a new session.
type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Prompt      string `json:"prompt"`
	ProjectRoot string `json:"projectRoot"`
	ProfileID   string `json:"profileId,omitempty"`
	// Worktree gives the task's session its own worktree, so tasks in one repository
	// do not edit the same files.
	Worktree bool `json:"worktree"`
	// Set by an AFK run (WFL-005): the session's mode, environment and workflow,
	// shell commands allowed for it as session rules, tags beside "queued", the
	// session whose worktree it shares, and the run.
	Mode            string   `json:"mode,omitempty"`
	EnvironmentID   string   `json:"environmentId,omitempty"`
	Workflow        string   `json:"workflow,omitempty"`
	Allow           []string `json:"allow,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	ShareWorktreeOf string   `json:"shareWorktreeOf,omitempty"`
	// ModelOption and Model switch the session's model before its first message,
	// through the session's own model option, as its model list does: the built-in
	// agent's provider and model, or an external agent's own choice.
	ModelOption string    `json:"modelOption,omitempty"`
	Model       string    `json:"model,omitempty"`
	RunID       string    `json:"runId,omitempty"`
	State       string    `json:"state"`
	AddedAt     time.Time `json:"addedAt"`
	// Set once it started: its session, and what holds it back or went wrong.
	SessionID string    `json:"sessionId,omitempty"`
	StartedAt time.Time `json:"startedAt,omitempty"`
	Note      string    `json:"note,omitempty"`
}

// Validate reports the first thing a task cannot start without.
func (t Task) Validate() error {
	switch {
	case strings.TrimSpace(t.Prompt) == "":
		return errors.New("a task needs a prompt")
	case !filepath.IsAbs(t.ProjectRoot):
		return errors.New("a task needs a project folder")
	}
	return nil
}

// RunLimit is the queue's limit, with the default filled in.
func (q Queue) RunLimit() int {
	if q.Limit <= 0 {
		return DefaultQueueLimit
	}
	return q.Limit
}

// LoadQueue returns the queue, an empty one when the file is missing.
func LoadQueue(dir string) (Queue, error) {
	b, err := os.ReadFile(filepath.Join(dir, QueueFile))
	if errors.Is(err, os.ErrNotExist) {
		return Queue{Tasks: []Task{}}, nil
	}
	if err != nil {
		return Queue{}, err
	}
	var q Queue
	if err := json.Unmarshal(b, &q); err != nil {
		return Queue{}, fmt.Errorf("%s: %w", QueueFile, err)
	}
	if q.Tasks == nil {
		q.Tasks = []Task{}
	}
	return q, nil
}

// SaveQueue replaces the queue file.
func SaveQueue(dir string, q Queue) error {
	b, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, QueueFile), b)
}
