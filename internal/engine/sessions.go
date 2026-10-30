package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// defaultTitle is what CreateSession names a session without a title; the
// first prompt replaces it (SES-004).
const defaultTitle = "New session"

// titleFrom derives a session title from its first message: the first line,
// whitespace collapsed, cut at a word boundary near 60 characters.
func titleFrom(text string) string {
	line := ""
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			line = strings.Join(strings.Fields(l), " ")
			break
		}
	}
	const max = 60
	if utf8.RuneCountInString(line) <= max {
		return line
	}
	r := []rune(line)[:max]
	cut := string(r)
	if i := strings.LastIndex(cut, " "); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:") + "…"
}

// RenameSession sets a session's title.
func (e *Engine) RenameSession(ctx context.Context, sid, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("a title cannot be empty")
	}
	if _, err := e.store.GetSession(ctx, sid); err != nil {
		return err
	}
	e.emit(sid, event.SessionInfoUpdate, map[string]string{"title": title})
	return nil
}

// stopRuntime cancels a session's turn and queue, waits for it, and forgets
// the live session.
func (e *Engine) stopRuntime(sid string) {
	e.mu.Lock()
	r, ok := e.runtimes[sid]
	delete(e.runtimes, sid)
	e.mu.Unlock()
	if ok {
		r.stop()
	}
}

// ArchiveSession hides a session from the default list (SES-005), stopping
// its turn first; archived false brings it back.
func (e *Engine) ArchiveSession(ctx context.Context, sid string, archived bool) error {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return err
	}
	status := session.StatusIdle
	if archived {
		// Its subagents go with it, first, since they share its folder.
		for _, c := range e.subagentsOf(ctx, sid) {
			if c.Status != session.StatusArchived {
				e.stopRuntime(c.ID)
				e.closeTerminals(c.ID)
				e.emit(c.ID, event.SessionStatus, event.SessionStatusData{Status: string(session.StatusArchived)})
			}
		}
		e.stopRuntime(sid)
		e.closeTerminals(sid)
		status = session.StatusArchived
		// A worktree goes with the archive unless it holds uncommitted
		// work; its branch and commits stay (SES-010).
		if ses.Worktree != "" {
			if ok, err := e.removeWorktree(ctx, ses); !ok {
				e.emit(sid, event.Notice, event.NoticeData{Message: fmt.Sprintf("The worktree %s was kept: %v", ses.Worktree, err)})
			}
		}
	} else if ses.Status != session.StatusArchived {
		return nil
	} else if err := e.restoreWorktree(ctx, ses); err != nil {
		return err
	}
	e.emit(sid, event.SessionStatus, event.SessionStatusData{Status: string(status)})
	return nil
}

// DeleteSession stops a session and removes its transcript and database
// rows (SES-005). The UI confirms first.
func (e *Engine) DeleteSession(ctx context.Context, sid string) error {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return err
	}
	// Its subagents go with it, first, since they share its folder.
	for _, c := range e.subagentsOf(ctx, sid) {
		if err := e.DeleteSession(ctx, c.ID); err != nil {
			log.Printf("engine: delete subagent %s of session %s: %v", c.ID, sid, err)
		}
	}
	e.stopRuntime(sid)
	e.closeTerminals(sid)
	if err := config.RemoveSessionPlugin(e.dataDir, sid); err != nil {
		log.Printf("engine: remove the skills of deleted session %s: %v", sid, err)
	}
	// Whatever the environment is now (it may have been deleted or changed
	// kind), a container the session had, and an image built for it, go.
	if _, err := environment.FindDocker(); err == nil {
		if err := environment.RemoveContainer(ctx, containerName(sid)); err != nil {
			log.Printf("engine: remove the container of deleted session %s: %v", sid, err)
		}
	}
	if ses.Worktree != "" {
		// Uncommitted work keeps the worktree; the branch always stays.
		if ok, err := e.removeWorktree(ctx, ses); !ok {
			log.Printf("engine: kept the worktree %s of deleted session %s: %v", ses.Worktree, sid, err)
		}
	}
	e.bg.Wait() // a git refresh must not recreate the transcript
	e.mu.Lock()
	delete(e.streams, sid)
	e.mu.Unlock()
	e.rules.forget(sid)
	return e.store.DeleteSession(ctx, sid)
}

// SetModel switches a built-in session to another model, on the same or
// another provider (PRV-006). The next turn uses it; a running turn
// finishes with the old one at its next model call boundary. The
// transcript records the switch.
func (e *Engine) SetModel(ctx context.Context, sid, providerID, model string) error {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return err
	}
	if ses.HarnessID != builtin.ID {
		return errors.New("only built-in sessions can switch models; an external agent chooses its own")
	}
	model = strings.TrimSpace(model)
	if providerID == "" {
		providerID = ses.ProviderID
	}
	if model == "" {
		return errors.New("choose a model")
	}
	if providerID == ses.ProviderID && model == ses.Model {
		return nil
	}
	p, err := e.provider(providerID)
	if err != nil {
		return err
	}
	if err := e.requireTools(ctx, providerID, model); err != nil {
		return err
	}
	e.mu.Lock()
	r, ok := e.runtimes[sid]
	e.mu.Unlock()
	if ok {
		if ms, ok := r.hs.(harness.ModelSetter); ok {
			window := 0
			if caps, err := e.ModelCapabilities(ctx, providerID, model, false); err == nil {
				window = caps.ContextWindow
			}
			ms.SetModel(e.limit(p, sid), model, window)
		}
	}
	e.emit(sid, event.ModelChanged, event.ModelChangedData{ProviderID: providerID, Model: model})
	return nil
}
