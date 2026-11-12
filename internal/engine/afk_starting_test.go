package engine

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
)

// failSaves makes the engine's saves of the AFK runs fail while fail says so.
func failSaves(e *Engine, fail func(all config.AFKRuns) bool) {
	e.afkMu.Lock()
	defer e.afkMu.Unlock()
	e.afkSave = func(dir string, all config.AFKRuns) error {
		if fail(all) {
			return errors.New("disk full")
		}
		return config.SaveAFKRuns(dir, all)
	}
}

func startingStep(all config.AFKRuns) *config.AFKStep {
	for _, r := range all.Runs {
		for i := range r.Steps {
			if r.Steps[i].Starting {
				return &r.Steps[i]
			}
		}
	}
	return nil
}

// oneSessionPerStep checks each step of the run has its own session and, with a
// review, one review, and that the engine has no other AFK sessions.
func oneSessionPerStep(t *testing.T, e *Engine, run config.AFKRun, reviewed bool) {
	t.Helper()
	list, err := e.store.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var work, reviews int
	for _, s := range list {
		switch {
		case s.ParentID != "":
			reviews++
		case containsTag(s.Tags, AFKTag):
			work++
		}
	}
	want := len(run.Steps)
	if work != want {
		t.Errorf("%d working sessions for %d steps", work, want)
	}
	if reviewed && reviews != want {
		t.Errorf("%d reviews for %d steps", reviews, want)
	}
	for _, s := range run.Steps {
		if s.SessionID == "" || s.Starting || (reviewed && s.ReviewID == "") {
			t.Errorf("step %+v", s)
		}
	}
}

func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// WFL-005: a save of the step that fails stops the run with the reason and queues
// nothing.
func TestAFKRunStopsWhenTheStepCannotBeSaved(t *testing.T) {
	ctx := context.Background()
	e, root := afkEngine(t)
	failSaves(e, func(all config.AFKRuns) bool { return startingStep(all) != nil })
	run, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root, Allow: []string{"git add *", "git commit *"}})
	if err != nil {
		t.Fatal(err)
	}
	run = waitRun(t, e, run.ID)
	if run.State != config.RunStopped || !strings.Contains(run.Reason, "disk full") {
		t.Fatalf("run = %+v", run)
	}
	if len(run.Steps) != 0 {
		t.Errorf("steps = %+v", run.Steps)
	}
	if q, _ := e.Queue(); len(q.Tasks) != 0 {
		t.Errorf("queued: %+v", q.Tasks)
	}
	if list, _ := e.store.ListSessions(ctx); len(list) != 0 {
		t.Errorf("%d sessions started", len(list))
	}
}

// WFL-005, WFL-006: a save of the review step that fails makes no review.
func TestAFKRunStopsWhenTheReviewCannotBeSaved(t *testing.T) {
	ctx := context.Background()
	e, root := afkEngine(t)
	failSaves(e, func(all config.AFKRuns) bool {
		s := startingStep(all)
		return s != nil && s.State == config.StepReviewing
	})
	run, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root, Allow: []string{"git add *", "git commit *"}})
	if err != nil {
		t.Fatal(err)
	}
	run = waitRun(t, e, run.ID)
	if run.State != config.RunStopped || !strings.Contains(run.Reason, "disk full") || len(run.Steps) != 1 {
		t.Fatalf("run = %+v", run)
	}
	if list, _ := e.store.ListSessions(ctx); len(list) != 1 {
		t.Errorf("%d sessions, want the issue's alone", len(list))
	}
}

// freeze holds the schedule loop's passes until the returned function is called.
func freeze(e *Engine) func() {
	e.afkStep.Lock()
	return e.afkStep.Unlock
}

// WFL-005: a run reloaded with a step left Starting, whose task was never made,
// queues the issue once.
func TestAFKRunReloadedBeforeTheTaskWasMade(t *testing.T) {
	ctx := context.Background()
	e, root := afkEngine(t)
	thaw := freeze(e)
	run, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root, Allow: []string{"git add *", "git commit *"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.changeRun(run.ID, func(r *config.AFKRun) error {
		r.Steps = append(r.Steps, config.AFKStep{Issue: "001-store", Path: "backlog/issues/001-store.md", Title: "Store", State: config.StepWorking, Starting: true, StartedAt: time.Now().UTC()})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	thaw()
	run = waitRun(t, e, run.ID)
	if run.State != config.RunFinished || len(run.Steps) != 2 {
		t.Fatalf("run = %+v", run)
	}
	oneSessionPerStep(t, e, run, true)
}

// WFL-005: the task was made but its record lost (the save that cleared Starting
// failed): the run finds the task rather than queue the issue again.
func TestAFKRunReloadedAfterTheTaskWasMade(t *testing.T) {
	ctx := context.Background()
	e, root := afkEngine(t)
	var failed atomic.Int32
	failSaves(e, func(all config.AFKRuns) bool {
		// The save that records the task, once.
		for _, r := range all.Runs {
			for _, s := range r.Steps {
				if s.TaskID != "" && !s.Starting && s.SessionID == "" && s.State == config.StepWorking && failed.Add(1) == 1 {
					return true
				}
			}
		}
		return false
	})
	run, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root, Allow: []string{"git add *", "git commit *"}})
	if err != nil {
		t.Fatal(err)
	}
	run = waitRun(t, e, run.ID)
	if failed.Load() == 0 {
		t.Fatal("the save never failed")
	}
	if run.State != config.RunFinished || len(run.Steps) != 2 {
		t.Fatalf("run = %+v", run)
	}
	oneSessionPerStep(t, e, run, true)
}

// WFL-006: the review was made but its record lost: the run finds it.
func TestAFKRunReloadedAfterTheReviewWasMade(t *testing.T) {
	ctx := context.Background()
	e, root := afkEngine(t)
	var failed atomic.Int32
	failSaves(e, func(all config.AFKRuns) bool {
		for _, r := range all.Runs {
			for _, s := range r.Steps {
				if s.ReviewID != "" && !s.Starting && failed.Add(1) == 1 {
					return true
				}
			}
		}
		return false
	})
	run, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root, Allow: []string{"git add *", "git commit *"}})
	if err != nil {
		t.Fatal(err)
	}
	run = waitRun(t, e, run.ID)
	if failed.Load() == 0 {
		t.Fatal("the save never failed")
	}
	if run.State != config.RunFinished || len(run.Steps) != 2 {
		t.Fatalf("run = %+v", run)
	}
	oneSessionPerStep(t, e, run, true)
}

// WFL-006: the run was cut off before the review was made: it makes one.
func TestAFKRunReloadedBeforeTheReviewWasMade(t *testing.T) {
	ctx := context.Background()
	e, root := afkEngine(t)
	// No record of a review is saved until released, so the run stays at its first
	// review, its step saved as Starting.
	var released atomic.Bool
	failSaves(e, func(all config.AFKRuns) bool {
		for _, r := range all.Runs {
			for _, s := range r.Steps {
				if s.ReviewID != "" && !released.Load() {
					return true
				}
			}
		}
		return false
	})
	run, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root, Allow: []string{"git add *", "git commit *"}})
	if err != nil {
		t.Fatal(err)
	}
	var reviewID string
	for deadline := time.Now().Add(30 * time.Second); reviewID == ""; {
		list, _ := e.store.ListSessions(ctx)
		for _, s := range list {
			if s.ParentID != "" {
				reviewID = s.ID
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("no review")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Cut off: the review is gone, as if it had never been made.
	thaw := freeze(e)
	if err := e.DeleteSession(ctx, reviewID); err != nil {
		t.Fatal(err)
	}
	released.Store(true)
	thaw()
	run = waitRun(t, e, run.ID)
	if run.State != config.RunFinished || len(run.Steps) != 2 {
		t.Fatalf("run = %+v", run)
	}
	oneSessionPerStep(t, e, run, true)
}
