package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// Fork (SES-009): a new session that starts with the conversation up to a
// user message, so the user can take it another way. Its transcript begins
// with a copy of the source's events before that message (same seqs and
// times), from which the built-in harness rebuilds the context; the
// source's checkpoint store is cloned, so rewinding works in the fork too.
// The files are the folder's as they are now: a fork of a worktree session
// gets a worktree of its own, branched from the source's branch.

// ForkResult is the new session and the message it was forked before, for
// the composer.
type ForkResult struct {
	Session session.Session `json:"session"`
	Text    string          `json:"text"`
}

// ForkSession forks sid before the user message at seq.
func (e *Engine) ForkSession(ctx context.Context, sid string, seq int64) (ForkResult, error) {
	src, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return ForkResult{}, err
	}
	events, err := e.store.Events(ctx, sid, 0)
	if err != nil {
		return ForkResult{}, err
	}
	at := -1
	var text string
	for i, ev := range events {
		if ev.Seq == seq && ev.Type == event.UserMessageChunk {
			var c event.MessageChunk
			if json.Unmarshal(ev.Data, &c) == nil && c.Content.Type == "text" {
				at, text = i, c.Content.Text
			}
			break
		}
	}
	if at < 0 {
		return ForkResult{}, errors.New("there is no message to fork from there")
	}
	// The turn the message started began a little before it: its
	// checkpoint, turn_started and status come first and are left out.
	for at > 0 {
		switch events[at-1].Type {
		case event.Checkpoint, event.TurnStarted, event.SessionStatus:
			at--
			continue
		}
		break
	}

	ses := session.Session{
		Title: src.Title + " (fork)", HarnessID: src.HarnessID, ProjectID: src.ProjectID, ProjectRoot: src.ProjectRoot,
		EnvironmentID: src.EnvironmentID, BaseCommit: src.BaseCommit, Branch: src.Branch, ProviderID: src.ProviderID,
		Model: src.Model, Mode: src.Mode, ProfileID: src.ProfileID, Optimizer: src.Optimizer, Effort: src.Effort,
	}
	// Like any new session, a fork never starts in bypass (SEC-001).
	if permission.Mode(ses.Mode) == permission.ModeBypass {
		ses.Mode = string(permission.ModeAsk)
	}
	if src.Worktree != "" {
		project, err := e.store.GetProject(ctx, src.ProjectID)
		if err != nil {
			return ForkResult{}, err
		}
		path, branch, err := e.addWorktree(ctx, project, ses.Title, src.OwnBranch())
		if err != nil {
			return ForkResult{}, err
		}
		ses.Worktree, ses.WorktreeBranch, ses.Branch = path, branch, branch
	}
	fork, err := e.store.CreateSession(ctx, ses)
	if err != nil {
		if ses.Worktree != "" {
			e.removeWorktree(ctx, ses)
		}
		return ForkResult{}, err
	}
	copied := make([]event.Event, at)
	for i, ev := range events[:at] {
		ev.SID = fork.ID
		copied[i] = ev
	}
	if err := e.store.CopyEvents(ctx, fork.ID, copied); err != nil {
		e.store.DeleteSession(ctx, fork.ID)
		return ForkResult{}, err
	}
	// The pictures the copied messages show (SES-015).
	if err := e.store.CopyAttachments(src.ID, fork.ID); err != nil {
		log.Printf("engine: copy the images of %s to its fork: %v", src.ID, err)
	}
	// The copied events set the source's title and mode again; the fork
	// has its own.
	e.emit(fork.ID, event.SessionInfoUpdate, map[string]string{"title": ses.Title})
	copiedMode := ""
	for _, ev := range copied {
		if ev.Type == event.CurrentModeUpdate {
			var d event.CurrentModeData
			json.Unmarshal(ev.Data, &d)
			copiedMode = d.CurrentModeID
		}
	}
	if copiedMode != "" && copiedMode != ses.Mode {
		e.emit(fork.ID, event.CurrentModeUpdate, event.CurrentModeData{CurrentModeID: ses.Mode})
	}
	if environment.IsDir(e.checkpointStore(sid)) {
		if host, err := e.localEnv(src.WorkRoot(), nil); err == nil {
			gitRunOver(ctx, host, time.Minute, "file", "clone", "--quiet", "--bare", e.checkpointStore(sid), e.checkpointStore(fork.ID))
		}
	}
	e.emit(fork.ID, event.SessionStatus, event.SessionStatusData{Status: string(session.StatusIdle)})
	msg := fmt.Sprintf("Forked from %q before the message %q.", src.Title, trimForNotice(text))
	if src.HarnessID != builtin.ID {
		msg += " The external agent starts without the earlier conversation; only this transcript has it."
	}
	if src.Worktree != "" {
		msg += " The fork has its own worktree, branched from the source's committed work."
	}
	e.emit(fork.ID, event.Notice, event.NoticeData{Message: msg})
	fork, _ = e.store.GetSession(ctx, fork.ID)
	return ForkResult{Session: fork, Text: text}, nil
}

func trimForNotice(s string) string {
	if r := []rune(s); len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}
