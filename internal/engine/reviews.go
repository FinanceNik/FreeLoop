package engine

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// Review by a second agent (HRN-018). The Diff tab sends a session's
// changes to another profile: a child session in plan mode, in the same
// worktree, so it can read the code but not change it. Its reply reaches
// the first session's message box only when the user asks.

// ReviewTag marks the sessions made to review another.
const ReviewTag = "review"

// maxReviewDiff caps the diff a reviewer is sent.
const maxReviewDiff = 120_000

// maxFindings caps a reviewer's reply in a message.
const maxFindings = 20 << 10

const reviewBrief = `Review the changes below, made by another agent. You are read-only: read any file you need, but do not plan, propose or make edits yourself.

Look for bugs, missed cases, security problems and changes that do not do what they seem to. Leave out style preferences. Report each problem as a numbered item: file and line, what is wrong, and why it matters. If you find nothing wrong, say so plainly. Your reply goes back to the agent that made the changes.

`

// ReviewRequest asks another profile to review a session's changes.
type ReviewRequest struct {
	SessionID string `json:"sessionId"`
	ProfileID string `json:"profileId"`
	// Label names the reviewer in the session's title.
	Label string `json:"label"`
	// Focus is what the user wants looked at, if anything.
	Focus string `json:"focus"`
}

// ReviewInfo is one review of a session.
type ReviewInfo struct {
	Session session.Session `json:"session"`
	Reply   string          `json:"reply"`
}

// RequestReview starts a review of the session's changes against its base
// commit and returns the reviewer's session.
func (e *Engine) RequestReview(ctx context.Context, req ReviewRequest) (session.Session, error) {
	env, ses, err := e.sessionGit(ctx, req.SessionID)
	if err != nil {
		return session.Session{}, err
	}
	if ses.ParentID != "" {
		return session.Session{}, errors.New("a subagent's or reviewer's session cannot be reviewed on its own")
	}
	if !e.trusted(ses.ProjectID) {
		return session.Session{}, errors.New(untrustedDiff)
	}
	if why := blocked(ctx, env); why != "" {
		return session.Session{}, errors.New(why)
	}
	base := ses.BaseCommit
	if base == "" {
		base = "HEAD"
	}
	diff, err := gitOK(ctx, env, "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--ignore-submodules=dirty", base)
	if err != nil {
		return session.Session{}, err
	}
	untracked, err := gitOK(ctx, env, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return session.Session{}, err
	}
	if strings.TrimSpace(diff) == "" && strings.TrimSpace(untracked) == "" {
		return session.Session{}, errors.New("there are no changes to review")
	}
	if len(diff) > maxReviewDiff {
		diff = diff[:maxReviewDiff] + "\n[The diff is longer; read the files for the rest.]"
	}

	project := e.sessionProject(ctx, ses)
	prof, err := e.profileFor(project, req.ProfileID)
	if err != nil {
		return session.Session{}, err
	}
	// An external agent's own tools do not pass through the app, so only its
	// own plan mode keeps it read-only, and the review promises that.
	if prof.Harness != "" && prof.Harness != builtin.ID {
		preset, err := e.runnablePreset(prof.Harness)
		if err != nil {
			return session.Session{}, err
		}
		if preset.Modes[string(permission.ModePlan)] == "" {
			return session.Session{}, fmt.Errorf("%s has no read-only mode the app can set, so it cannot review; add a plan entry to its preset's modes", preset.Name)
		}
	}
	// A profile without a model of its own reviews on the session's.
	create := CreateSessionRequest{
		Title: "Review: " + cmp.Or(strings.TrimSpace(req.Label), prof.Name), ProjectRoot: ses.ProjectRoot, EnvironmentID: ses.EnvironmentID,
		ProfileID: req.ProfileID, Mode: "plan", parent: ses.ID,
	}
	if prof.Provider == "" {
		create.ProviderID = ses.ProviderID
		if prof.Model == "" {
			create.Model = ses.Model
		}
	}
	if ses.Worktree != "" {
		create.shareWorktree = &ses
	}
	reviewer, err := e.CreateSession(ctx, create)
	if err != nil {
		return session.Session{}, err
	}
	e.SetTags(ctx, reviewer.ID, []string{ReviewTag})
	var b strings.Builder
	b.WriteString(reviewBrief)
	if f := strings.TrimSpace(req.Focus); f != "" {
		fmt.Fprintf(&b, "The user asks you to look especially at: %s\n\n", f)
	}
	if u := strings.TrimSpace(untracked); u != "" {
		fmt.Fprintf(&b, "New files not shown in the diff:\n%s\n\n", u)
	}
	b.WriteString(fence("diff", diff))
	// Whoever wrote the diff wrote this text, so its @ words attach nothing.
	if _, err := e.enqueue(ctx, reviewer.ID, queued{text: b.String(), noMentions: true}); err != nil {
		return reviewer, err
	}
	return reviewer, nil
}

// Reviews lists a session's reviews, newest first.
func (e *Engine) Reviews(ctx context.Context, sid string) ([]ReviewInfo, error) {
	children := e.subagentsOf(ctx, sid)
	out := []ReviewInfo{}
	for _, c := range children {
		if slices.Contains(c.Tags, ReviewTag) {
			out = append(out, ReviewInfo{Session: c, Reply: e.lastReply(ctx, c.ID)})
		}
	}
	slices.SortFunc(out, func(a, b ReviewInfo) int { return b.Session.CreatedAt.Compare(a.Session.CreatedAt) })
	return out, nil
}

// ReviewFindings writes a reviewer's reply as a message for the session it
// reviewed.
func (e *Engine) ReviewFindings(ctx context.Context, reviewerID string) (string, error) {
	r, err := e.store.GetSession(ctx, reviewerID)
	if err != nil {
		return "", err
	}
	if !slices.Contains(r.Tags, ReviewTag) {
		return "", errors.New("that session is not a review")
	}
	reply := e.fullReply(ctx, reviewerID)
	if reply == "" {
		return "", errors.New("the reviewer has not answered yet")
	}
	if len(reply) > maxFindings {
		reply = cutBytes(reply, maxFindings) + "\n[The review is longer; open the reviewer's session for the rest.]"
	}
	who := strings.TrimPrefix(r.Title, "Review: ")
	return fmt.Sprintf("A second agent (%s) reviewed your changes and reported the following. Check which of these are real problems, fix those, and say which you left and why.\n\n%s\n", who, reply), nil
}
