package engine

import (
	"crypto/hmac"
	"errors"
	"fmt"
	"log"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// Commands in the app's config files (SET-005, EXT-005, HRN-005): hooks in
// config.yaml and profiles, and agent presets. A file arrives without asking,
// from a restored backup or anything else that can write to the data folder, and
// its command would run with every session, outside the broker and any sandbox.
// So one runs only with a mark from approvalMark, which needs a key the file
// cannot read. The app adds the mark when the user writes the command in the app
// or approves it in Settings; a command edited in a file waits for approval. A
// project's own hooks need no mark: trusting the project lists them (SEC-006).

// Kinds of PendingCommand.
const (
	pendingHook  = "hook"
	pendingAgent = "agent"
)

// PendingCommand is a command from a file, as Settings shows it for approval.
type PendingCommand struct {
	Kind string `json:"kind"`
	// Source says where it is, Event is a hook's event, and Runs what would run.
	Source      string `json:"source"`
	Event       string `json:"event"`
	Runs        string `json:"runs"`
	Fingerprint string `json:"fingerprint"`
}

// fileCommands lists every hook and preset the app's own files define.
func (e *Engine) fileCommands() []PendingCommand {
	e.mu.Lock()
	cfg, profiles, presets := e.cfg, slices.Clone(e.profiles), slices.Clone(e.presets)
	e.mu.Unlock()
	var out []PendingCommand
	add := func(source string, hooks []config.Hook) {
		for _, h := range hooks {
			out = append(out, PendingCommand{Kind: pendingHook, Source: source, Event: h.Event, Runs: h.Command, Fingerprint: h.Fingerprint()})
		}
	}
	add(config.FileName, cfg.Hooks)
	for _, p := range profiles {
		add("profile "+p.Name, p.Hooks)
	}
	for _, p := range presets {
		if !p.Builtin {
			out = append(out, PendingCommand{Kind: pendingAgent, Source: presetFile(p), Runs: describePreset(p), Fingerprint: p.Fingerprint()})
		}
	}
	return out
}

func presetFile(p config.HarnessPreset) string {
	return filepath.Join(config.PresetsDir, p.ID+".yaml")
}

// describePreset is what approving a preset allows to run.
func describePreset(p config.HarnessPreset) string {
	quoted := func(s string) string {
		if strings.ContainsAny(s, " \t\n'\"") {
			return shellQuote(s)
		}
		return s
	}
	parts := []string{quoted(p.Command)}
	for _, a := range p.Args {
		parts = append(parts, quoted(a))
	}
	out := strings.Join(parts, " ")
	var vars []string
	for _, k := range slices.Sorted(maps.Keys(p.Env)) {
		vars = append(vars, k+"="+p.Env[k])
	}
	if len(vars) > 0 {
		out += ", with " + strings.Join(vars, ", ") + " set"
	}
	return out
}

// PendingCommands lists the commands from files waiting for the user's approval.
func (e *Engine) PendingCommands() []PendingCommand {
	out := []PendingCommand{}
	for _, c := range e.fileCommands() {
		if !e.commandApproved(c.Fingerprint) {
			out = append(out, c)
		}
	}
	return out
}

// commandApproved reports whether the user approved exactly this command.
func (e *Engine) commandApproved(fingerprint string) bool {
	mark, err := e.approvalMark(fingerprint)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(e.Config().CommandApprovals, func(m string) bool {
		return hmac.Equal([]byte(mark), []byte(m))
	})
}

// ApproveCommand approves a command as the user saw it, which must still be what
// a file says.
func (e *Engine) ApproveCommand(fingerprint string) (config.Config, error) {
	if !slices.ContainsFunc(e.PendingCommands(), func(c PendingCommand) bool { return c.Fingerprint == fingerprint }) {
		return config.Config{}, errors.New("that command changed or is gone; look at it again")
	}
	mark, err := e.approvalMark(fingerprint)
	if err != nil {
		return config.Config{}, err
	}
	return e.saveSettings(e.Config(), []string{mark})
}

// writtenMarks are the marks for hooks the user is saving in the app. A form
// sends back every hook it holds, so a hook still waiting for approval, in this
// list or in another, stays waiting: changing a setting or copying a profile
// does not approve what a backup brought.
func (e *Engine) writtenMarks(hooks []config.Hook) ([]string, error) {
	waiting := map[string]bool{}
	for _, c := range e.PendingCommands() {
		if c.Kind == pendingHook {
			waiting[c.Runs] = true
		}
	}
	var marks []string
	for _, h := range hooks {
		if waiting[h.Command] || e.commandApproved(h.Fingerprint()) {
			continue
		}
		mark, err := e.approvalMark(h.Fingerprint())
		if err != nil {
			return nil, err
		}
		marks = append(marks, mark)
	}
	return marks, nil
}

// addApprovals records marks in config.yaml.
func (e *Engine) addApprovals(marks []string) error {
	if len(marks) == 0 {
		return nil
	}
	_, err := e.saveSettings(e.Config(), marks)
	return err
}

// approvedHooks keeps the hooks the user approved and says once per session
// which did not run: one may be a guard the user counts on.
func (e *Engine) approvedHooks(r *runtime, hooks []config.Hook) []config.Hook {
	var out []config.Hook
	for _, h := range hooks {
		fp := h.Fingerprint()
		if e.commandApproved(fp) {
			out = append(out, h)
			continue
		}
		r.callsMu.Lock()
		seen := r.unapproved[fp]
		if r.unapproved == nil {
			r.unapproved = map[string]bool{}
		}
		r.unapproved[fp] = true
		r.callsMu.Unlock()
		if !seen {
			e.emit(r.sid, event.Notice, event.NoticeData{Message: fmt.Sprintf("A hook from %s does not run, because it has not been approved as it is now: %s. Approve it in Settings.", h.Source, capText(h.Command))})
		}
	}
	return out
}

// runnablePreset returns a preset whose command may run.
func (e *Engine) runnablePreset(id string) (config.HarnessPreset, error) {
	p, err := e.preset(id)
	if err != nil {
		return p, err
	}
	if why := e.presetBlocked(p); why != "" {
		return p, fmt.Errorf("%s cannot start: %s", p.Name, why)
	}
	return p, nil
}

// presetBlocked says why a preset's command may not run; "" when it may.
func (e *Engine) presetBlocked(p config.HarnessPreset) string {
	if p.Builtin || e.commandApproved(p.Fingerprint()) {
		return ""
	}
	return fmt.Sprintf("%s has not been approved as it is now; approve it in Settings", presetFile(p))
}

// pendingIssues lists what waits for approval among the problems the app shows.
func (e *Engine) pendingIssues() []config.Issue {
	var out []config.Issue
	for _, c := range e.PendingCommands() {
		out = append(out, config.Issue{File: c.Source, Message: fmt.Sprintf("%s does not run until you approve it in Settings", c.Runs)})
	}
	return out
}

// commandsAdoptedRef marks that a data folder's existing commands were adopted.
// It is in the Keychain because a backup can replace any config file, and per
// folder because a scratch one (FREELOOP_HOME) says nothing about the real one.
func (e *Engine) commandsAdoptedRef() secrets.Ref {
	return secrets.Ref{Service: "freeloop", Account: "app/commands-adopted/" + e.dataDir}
}

// adoptExistingCommands approves, once, what the files hold when the first
// version with these approvals starts, so the hooks and agents a user already has
// keep working. They ran before it too, so this trusts nothing new; a backup
// restored from then on finds the item there. The item is written first: if the
// rest fails the user is asked, which is the safe way to be wrong.
func (e *Engine) adoptExistingCommands() {
	ref := e.commandsAdoptedRef()
	if _, err := e.secrets.Get(ref); !errors.Is(err, secrets.ErrNotFound) {
		return
	}
	if err := e.secrets.Set(ref, "1"); err != nil {
		log.Printf("engine: adopt existing commands: %v", err)
		return
	}
	var marks []string
	for _, c := range e.fileCommands() {
		mark, err := e.approvalMark(c.Fingerprint)
		if err != nil {
			log.Printf("engine: adopt existing commands: %v", err)
			return
		}
		marks = append(marks, mark)
	}
	if err := e.addApprovals(marks); err != nil {
		log.Printf("engine: adopt existing commands: %v", err)
	}
}
