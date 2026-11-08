package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// A trusted project's hooks (EXT-005, SEC-006): those in .freeloop/hooks.yaml
// and in its profiles run as one set, the one the user saw. Trusting the project
// approves the set as it is then, with a mark from approvalMark, which needs a
// key no file can read, as for the app's own files (approvals.go). A set that
// differs since, from a pull, a merge or anything else that writes the
// checkout, runs none of its hooks until the user approves it in Settings, where
// it waits with the app's other approvals; the session says so, since a guard
// may be among them. A project with no hooks needs nothing, and untrusting one
// ends the approval of what it holds.

// pendingProjectHooks is the PendingCommand kind of a project's hook set.
const pendingProjectHooks = "project_hooks"

// projectHookSet is a project's hooks as one set: hooks.yaml, then its profiles'.
func projectHookSet(pc projectConfig) []config.Hook {
	hooks := slices.Clone(pc.Hooks)
	for _, p := range pc.Profiles {
		for _, h := range p.Hooks {
			h.Source = p.Source
			hooks = append(hooks, h)
		}
	}
	return hooks
}

// projectHooksFingerprint is "" for a project with no hooks.
func projectHooksFingerprint(p session.Project, pc projectConfig) string {
	return config.ProjectHooksFingerprint(p.ID, p.Root, projectHookSet(pc))
}

// projectHooksApproved reports whether a project's hooks may run as they are.
func (e *Engine) projectHooksApproved(p session.Project, pc projectConfig) bool {
	fp := projectHooksFingerprint(p, pc)
	return fp == "" || e.commandApproved(fp)
}

// projectHooksRun is projectHooksApproved for a session's hooks, saying once per
// session and set when they do not run.
func (e *Engine) projectHooksRun(r *runtime, p session.Project, pc projectConfig) bool {
	fp := projectHooksFingerprint(p, pc)
	if fp == "" || e.commandApproved(fp) {
		return true
	}
	r.callsMu.Lock()
	seen := r.unapproved[fp]
	if r.unapproved == nil {
		r.unapproved = map[string]bool{}
	}
	r.unapproved[fp] = true
	r.callsMu.Unlock()
	if !seen {
		e.emit(r.sid, event.Notice, event.NoticeData{Message: fmt.Sprintf(
			"The hooks in %s are not the ones approved when the project was trusted, so none of them run until you approve them as they are now in Settings, Permissions. Until then a guard among them guards nothing: %s",
			filepath.Join(p.Root, ProjectDir), capText(describeHooks(projectHookSet(pc))))})
	}
	return false
}

// describeHooks is what a hook set runs, for an approval.
func describeHooks(hooks []config.Hook) string {
	parts := make([]string, 0, len(hooks))
	for _, h := range hooks {
		when := h.Event
		if h.Match != "" {
			when += " " + h.Match
		}
		parts = append(parts, fmt.Sprintf("%s: %s (%s)", when, h.Command, h.Source))
	}
	return strings.Join(parts, "; ")
}

// projectHookItems lists every trusted project's hook set, for PendingCommands.
func (e *Engine) projectHookItems() []PendingCommand {
	projects, err := e.store.ListProjects(context.Background())
	if err != nil {
		log.Printf("engine: projects' hooks: %v", err)
		return nil
	}
	var out []PendingCommand
	for _, p := range projects {
		if !p.Trusted {
			continue
		}
		pc := e.projectConfig(p)
		if fp := projectHooksFingerprint(p, pc); fp != "" {
			out = append(out, PendingCommand{Kind: pendingProjectHooks, Source: filepath.Join(p.Root, ProjectDir), Runs: describeHooks(projectHookSet(pc)), Fingerprint: fp})
		}
	}
	return out
}

// projectHooksMark is the mark approving a project's hooks as its files hold them
// now, read afresh; "" when it has none.
func (e *Engine) projectHooksMark(p session.Project) (string, error) {
	fp := projectHooksFingerprint(p, readProjectConfig(e, p))
	if fp == "" {
		return "", nil
	}
	return e.approvalMark(fp)
}

// approveProjectHooks approves a project's hooks as they are now, as trusting it does.
func (e *Engine) approveProjectHooks(p session.Project) error {
	mark, err := e.projectHooksMark(p)
	if err != nil || mark == "" {
		return err
	}
	return e.addApprovals([]string{mark})
}

// revokeProjectHooks ends the approval of a project's hooks as they are now.
func (e *Engine) revokeProjectHooks(p session.Project) error {
	mark, err := e.projectHooksMark(p)
	if err != nil || mark == "" || !slices.Contains(e.Config().CommandApprovals, mark) {
		return err
	}
	e.mu.Lock()
	e.cfg.CommandApprovals = slices.DeleteFunc(slices.Clone(e.cfg.CommandApprovals), func(m string) bool { return m == mark })
	e.mu.Unlock()
	_, err = e.saveSettings(e.Config(), nil)
	return err
}

// projectHooksAdoptedRef marks that a data folder's trusted projects had their
// hooks adopted, in the Keychain like commandsAdoptedRef.
func (e *Engine) projectHooksAdoptedRef() secrets.Ref {
	return secrets.Ref{Service: "freeloop", Account: "app/project-hooks-adopted/" + e.dataDir}
}

// adoptProjectHooks approves, once, the hooks trusted projects hold when the
// first version that asks about changed ones starts: they ran before it too.
func (e *Engine) adoptProjectHooks() {
	ref := e.projectHooksAdoptedRef()
	if _, err := e.secrets.Get(ref); !errors.Is(err, secrets.ErrNotFound) {
		return
	}
	if err := e.secrets.Set(ref, "1"); err != nil {
		log.Printf("engine: adopt projects' hooks: %v", err)
		return
	}
	var marks []string
	for _, c := range e.projectHookItems() {
		mark, err := e.approvalMark(c.Fingerprint)
		if err != nil {
			log.Printf("engine: adopt projects' hooks: %v", err)
			return
		}
		marks = append(marks, mark)
	}
	if err := e.addApprovals(marks); err != nil {
		log.Printf("engine: adopt projects' hooks: %v", err)
	}
}
