package engine

import (
	"cmp"
	"context"
	"fmt"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/backlog"
	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// Reviews in a workflow (WFL-006): after each AFK issue, and on /review, a second
// agent on the workflow's review profile reads the changes, as HRN-018's review does:
// read-only, in plan mode, in the same worktree. Its findings reach no session by
// themselves; the user reads them with the issue in Needs attention.

// reviewIssue has the run's review profile review what an issue's session changed,
// and returns the review's session.
func (e *Engine) reviewIssue(ctx context.Context, r config.AFKRun, s config.AFKStep, sid string, w config.Workflow, env environment.Environment) (string, error) {
	b, _ := env.FS().ReadFile(s.Path)
	focus := config.Render(w.Review.Focus, map[string]string{"backlog": w.BacklogDir(), "issue": s.Path, "issueText": strings.TrimSpace(backlog.Body(b))})
	rev, err := e.RequestReview(ctx, ReviewRequest{SessionID: sid, ProfileID: r.ReviewProfileID, Label: s.Issue, Focus: strings.TrimSpace(focus),
		model: harness.OptionValue{ID: r.ReviewModelOption, Value: r.ReviewModel}, effort: harness.OptionValue{ID: r.ReviewEffortOption, Value: r.ReviewEffort}})
	if err != nil {
		return "", err
	}
	e.SetTags(ctx, rev.ID, []string{ReviewTag, AFKTag})
	return rev.ID, nil
}

// reviewStage has the workflow's review profile review the session (the review
// stage), with what followed the command as what to look at.
func (e *Engine) reviewStage(ctx context.Context, ses session.Session, w config.Workflow, input string) (StageResult, error) {
	res := StageResult{Kind: config.StageReview}
	project := e.sessionProject(ctx, ses)
	prof := ses.ProfileID
	if w.Review.Profile != "" {
		if _, err := e.profileFor(project, w.Review.Profile); err == nil {
			prof = w.Review.Profile
		}
	}
	rev, err := e.RequestReview(ctx, ReviewRequest{SessionID: ses.ID, ProfileID: cmp.Or(prof, config.DefaultProfileID), Focus: input})
	if err != nil {
		return res, err
	}
	res.SessionID = rev.ID
	res.Message = fmt.Sprintf("A second agent reviews this session's changes in %q. Its findings come here only when you ask, from the Diff tab.", rev.Title)
	e.emit(ses.ID, event.Notice, event.NoticeData{Message: res.Message})
	return res, nil
}
