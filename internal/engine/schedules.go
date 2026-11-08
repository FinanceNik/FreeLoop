package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
)

// Scheduled and recurring sessions (SES-012). While the app runs, a due
// schedule starts a new session in its project, tagged "scheduled", and
// sends it the schedule's prompt. The session is like any other: every
// side effect goes through the permission broker, and one that needs the
// user waits in Needs attention, however long that takes (SEC-003).
// Runs missed while the app was closed make one run when it opens.

// ScheduledTag marks the sessions schedules start.
const ScheduledTag = "scheduled"

// ScheduleInfo is a schedule with when it runs next.
type ScheduleInfo struct {
	config.Schedule
	Next time.Time `json:"next,omitzero"`
}

// Schedules lists the schedules.
func (e *Engine) Schedules() ([]ScheduleInfo, error) {
	list, err := config.LoadSchedules(e.dataDir)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]ScheduleInfo, 0, len(list))
	for _, s := range list {
		info := ScheduleInfo{Schedule: s}
		if s.Enabled {
			last := s.LastRun
			if last.IsZero() {
				last = s.CreatedAt
			}
			info.Next = s.Next(last.In(now.Location()))
			// A missed run happens at the next check.
			if info.Next.Before(now) {
				info.Next = now
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// SaveSchedule adds a schedule, or replaces the one with its ID; what it
// ran keeps.
func (e *Engine) SaveSchedule(s config.Schedule) (config.Schedule, error) {
	s.Name, s.Prompt, s.At = strings.TrimSpace(s.Name), strings.TrimSpace(s.Prompt), strings.TrimSpace(s.At)
	if err := s.Validate(); err != nil {
		return config.Schedule{}, err
	}
	if permission.Mode(s.Mode).Unattended() {
		return config.Schedule{}, fmt.Errorf("a schedule cannot start sessions in %s mode", s.Mode)
	}
	e.schedules.Lock()
	defer e.schedules.Unlock()
	list, err := config.LoadSchedules(e.dataDir)
	if err != nil {
		return config.Schedule{}, err
	}
	i := slices.IndexFunc(list, func(x config.Schedule) bool { return s.ID != "" && x.ID == s.ID })
	if i < 0 {
		now := time.Now().UTC()
		s.ID = fmt.Sprintf("sch_%d", now.UnixNano())
		s.CreatedAt, s.LastRun, s.LastSession, s.LastError = now, time.Time{}, "", ""
		list = append(list, s)
	} else {
		old := list[i]
		s.CreatedAt, s.LastRun, s.LastSession, s.LastError = old.CreatedAt, old.LastRun, old.LastSession, old.LastError
		// Turned back on, it waits for its next time rather than running
		// at once for the times it was off.
		if s.Enabled && !old.Enabled {
			s.CreatedAt, s.LastRun = time.Now().UTC(), time.Time{}
		}
		list[i] = s
	}
	return s, config.SaveSchedules(e.dataDir, list)
}

// DeleteSchedule removes a schedule; the sessions it started stay.
func (e *Engine) DeleteSchedule(id string) error {
	e.schedules.Lock()
	defer e.schedules.Unlock()
	list, err := config.LoadSchedules(e.dataDir)
	if err != nil {
		return err
	}
	out := slices.DeleteFunc(list, func(s config.Schedule) bool { return s.ID == id })
	return config.SaveSchedules(e.dataDir, out)
}

// RunSchedule runs a schedule now, whether or not it is on, and returns
// the session it started.
func (e *Engine) RunSchedule(ctx context.Context, id string) (string, error) {
	e.schedules.Lock()
	defer e.schedules.Unlock()
	list, err := config.LoadSchedules(e.dataDir)
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(list, func(s config.Schedule) bool { return s.ID == id })
	if i < 0 {
		return "", errors.New("no such schedule")
	}
	sid, runErr := e.runSchedule(ctx, &list[i], time.Now())
	if err := config.SaveSchedules(e.dataDir, list); err != nil {
		log.Printf("engine: record schedule run: %v", err)
	}
	return sid, runErr
}

// runDue runs every schedule due at now.
func (e *Engine) runDue(ctx context.Context, now time.Time) {
	e.schedules.Lock()
	defer e.schedules.Unlock()
	list, err := config.LoadSchedules(e.dataDir)
	if err != nil {
		log.Printf("engine: schedules: %v", err)
		return
	}
	ran := false
	for i := range list {
		if list[i].Due(now) {
			e.runSchedule(ctx, &list[i], now)
			ran = true
		}
	}
	if ran {
		if err := config.SaveSchedules(e.dataDir, list); err != nil {
			log.Printf("engine: record schedule runs: %v", err)
		}
	}
}

// runSchedule starts one run and records it on s. A run that fails to
// start is recorded too, so it is not retried every check.
func (e *Engine) runSchedule(ctx context.Context, s *config.Schedule, now time.Time) (string, error) {
	s.LastRun, s.LastError = now.UTC(), ""
	if a := s.AFK; a != nil {
		// An AFK run, with what the user allowed when setting it up (WFL-005). It
		// refuses as from the dialog: a run going on, nothing ready, an uncommitted
		// backlog.
		_, err := e.StartAFK(ctx, AFKStart{ProjectRoot: s.ProjectRoot, WorkflowID: a.WorkflowID, ProfileID: s.ProfileID, ReviewProfileID: a.ReviewProfileID, Allow: a.Allow, StartedBy: fmt.Sprintf("the schedule %q", s.Name)})
		if err != nil {
			s.LastError = err.Error()
		}
		return "", err
	}
	ses, err := e.CreateSession(ctx, CreateSessionRequest{
		Title:       fmt.Sprintf("%s · %s", s.Name, now.Format("Jan 2 15:04")),
		ProjectRoot: s.ProjectRoot,
		ProfileID:   s.ProfileID,
		ProviderID:  s.ProviderID,
		Model:       s.Model,
		Mode:        s.Mode,
	})
	if err != nil {
		s.LastError = err.Error()
		return "", err
	}
	s.LastSession = ses.ID
	e.SetTags(ctx, ses.ID, []string{ScheduledTag})
	e.emit(ses.ID, event.Notice, event.NoticeData{Message: fmt.Sprintf("Started by the schedule %q.", s.Name)})
	if _, err := e.Prompt(ctx, ses.ID, s.Prompt); err != nil {
		s.LastError = err.Error()
		return ses.ID, err
	}
	return ses.ID, nil
}

// moveQueue brings the queue up to date, moves the AFK runs on, and starts what
// they queued.
func (e *Engine) moveQueue() {
	e.runQueue(e.scheduleCtx, time.Now())
	e.runAFK(e.scheduleCtx, time.Now())
	e.runQueue(e.scheduleCtx, time.Now())
}

// scheduleLoop checks for due schedules until Close, which waits for it
// before stopping sessions, so no run starts while the app quits.
func (e *Engine) scheduleLoop(interval time.Duration) {
	defer e.scheduling.Done()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		// A first check right away runs what was missed while closed.
		e.runDue(e.scheduleCtx, time.Now())
		e.moveQueue()
		e.checkBudgets(e.scheduleCtx, time.Now())
		for kicked := true; kicked; {
			select {
			case <-e.stopSchedules:
				return
			case <-t.C:
				kicked = false
			case <-e.afkKick:
				// A session of the queue's ended: move the queue and the runs on now.
				e.moveQueue()
			}
		}
	}
}
