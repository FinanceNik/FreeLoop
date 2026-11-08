package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AFKRunsFile holds the workflows' AFK runs (WFL-005). The app writes each run's
// state into it, so, like the queue, it is apart from the files the app watches.
const AFKRunsFile = "afkruns.json"

// AFK run states.
const (
	RunRunning  = "running"
	RunPaused   = "paused"
	RunFinished = "finished"
	RunStopped  = "stopped"
)

// AFK step states: an issue being worked on, being reviewed, waiting for the user's
// QA, QA'd, or one the run stopped at.
const (
	StepWorking   = "working"
	StepReviewing = "reviewing"
	StepQA        = "qa"
	StepQADone    = "qa_done"
	StepFailed    = "failed"
)

// AFKRun is one run of a workflow's unattended issues in a project: one fresh
// session per issue, one at a time, in one worktree of its own (OQ-040).
type AFKRun struct {
	ID          string `json:"id"`
	WorkflowID  string `json:"workflowId"`
	ProjectID   string `json:"projectId"`
	ProjectRoot string `json:"projectRoot"`
	Backlog     string `json:"backlog"`
	// The sessions' profile, the review's, their environment and mode.
	ProfileID       string `json:"profileId"`
	ReviewProfileID string `json:"reviewProfileId"`
	EnvironmentID   string `json:"environmentId"`
	Mode            string `json:"mode"`
	// Allow are the shell rule patterns the user allowed for the run's sessions
	// when starting it; they become each session's own rules.
	Allow     []string `json:"allow"`
	MaxIssues int      `json:"maxIssues"`
	State     string   `json:"state"`
	// Reason says why it finished or stopped.
	Reason string `json:"reason,omitempty"`
	// Anchor is the run's first session, whose worktree and branch the others share.
	Anchor   string    `json:"anchor,omitempty"`
	Worktree string    `json:"worktree,omitempty"`
	Branch   string    `json:"branch,omitempty"`
	Steps    []AFKStep `json:"steps"`
	// Started says how: from the Board, the palette, /afk or a schedule.
	StartedBy string    `json:"startedBy,omitempty"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt,omitzero"`
}

// AFKStep is one issue of a run.
type AFKStep struct {
	Issue     string `json:"issue"`
	Path      string `json:"path"`
	Title     string `json:"title"`
	TaskID    string `json:"taskId"`
	SessionID string `json:"sessionId,omitempty"`
	ReviewID  string `json:"reviewId,omitempty"`
	State     string `json:"state"`
	Note      string `json:"note,omitempty"`
	// Waiting says why its session has not started yet: the queue's reason, such
	// as an agent's limit, while its task waits.
	Waiting   string    `json:"waiting,omitempty"`
	Commit    string    `json:"commit,omitempty"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt,omitzero"`
}

// Active reports a run that may still start or follow a session.
func (r AFKRun) Active() bool { return r.State == RunRunning || r.State == RunPaused }

// Current is the step in progress, working or being reviewed; nil when none is.
func (r *AFKRun) Current() *AFKStep {
	if n := len(r.Steps); n > 0 && (r.Steps[n-1].State == StepWorking || r.Steps[n-1].State == StepReviewing) {
		return &r.Steps[n-1]
	}
	return nil
}

// AFKRuns are every run, oldest first.
type AFKRuns struct {
	Runs []AFKRun `json:"runs"`
}

// LoadAFKRuns returns the runs; none when the file is missing.
func LoadAFKRuns(dir string) (AFKRuns, error) {
	b, err := os.ReadFile(filepath.Join(dir, AFKRunsFile))
	if errors.Is(err, os.ErrNotExist) {
		return AFKRuns{Runs: []AFKRun{}}, nil
	}
	if err != nil {
		return AFKRuns{}, err
	}
	var r AFKRuns
	if err := json.Unmarshal(b, &r); err != nil {
		return AFKRuns{}, fmt.Errorf("%s: %w", AFKRunsFile, err)
	}
	if r.Runs == nil {
		r.Runs = []AFKRun{}
	}
	return r, nil
}

// SaveAFKRuns replaces the runs file.
func SaveAFKRuns(dir string, r AFKRuns) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, AFKRunsFile), b)
}
