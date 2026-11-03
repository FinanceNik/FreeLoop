package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
)

// ruleStore holds the rules in force (SEC-002): global rules from
// rules.yaml, reloaded when the file changes, and per-session rules, which
// live in each session's transcript as the persisted rule of a
// permission_decided event.
type ruleStore struct {
	dir    string
	report func([]config.Issue)

	mu      sync.Mutex
	global  []permission.Rule
	mod     time.Time
	session map[string][]permission.Rule
	// scopedRules returns a session's profile and project rules, and an
	// error that makes the broker fail closed; called without mu held.
	scopedRules func(sid string) ([]permission.Rule, error)
	// approved says whether an allow rule from a file may apply, and approve
	// records that the user added one (SET-005); called without mu held.
	approved func(permission.Rule) bool
	approve  func(permission.Rule) error
	// broken is set while rules.yaml has errors: the broker fails closed and
	// the file is not rewritten, so no hand-written rule is lost.
	broken error
}

func newRuleStore(dir string, report func([]config.Issue)) *ruleStore {
	s := &ruleStore{dir: dir, report: report, session: map[string][]permission.Rule{}}
	s.mu.Lock()
	s.reloadLocked(true)
	s.mu.Unlock()
	return s
}

// reloadLocked rereads rules.yaml when its modification time changed.
func (s *ruleStore) reloadLocked(force bool) {
	mod := config.RulesModTime(s.dir)
	if !force && mod.Equal(s.mod) {
		return
	}
	rules, issues, err := config.LoadRules(s.dir)
	s.mod = mod
	// Every reload reports its issues, none included, so fixed problems
	// clear (SET-001).
	defer func() {
		if s.report != nil {
			s.report(issues)
		}
	}()
	switch {
	case errors.Is(err, config.ErrRulesInvalid):
		// Keep the valid rules, and the last good ones if nothing parsed.
		if rules != nil {
			s.global = rules
		}
		s.broken = fmt.Errorf("%s has errors; fix the file (see the problems listed in the app) before rules change: %w", config.RulesFileName, err)
	case err != nil:
		s.broken = err
		issues = append(issues, config.Issue{File: config.RulesFileName, Message: err.Error()})
	default:
		s.global, s.broken = rules, nil
	}
}

// Rules returns the rules in force for sid: global, then its profile's and
// its project's, then its own. An allow rule from a file the user has not
// approved is not among them; a deny rule needs no approval.
func (s *ruleStore) Rules(sid string) ([]permission.Rule, error) {
	rules, err := s.all(sid)
	return slices.DeleteFunc(rules, s.pending), err
}

// pending reports an allow rule from a file waiting for approval. A session's
// own came from the user answering, and a project's can only deny.
func (s *ruleStore) pending(r permission.Rule) bool {
	return r.Effect == permission.Allow && r.Scope != permission.ScopeSession && s.approved != nil && !s.approved(r)
}

// fileRules returns what rules.yaml says.
func (s *ruleStore) fileRules() []permission.Rule {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reloadLocked(false)
	return slices.Clone(s.global)
}

// all returns the rules for sid whether in force or not.
func (s *ruleStore) all(sid string) ([]permission.Rule, error) {
	var scoped []permission.Rule
	var scopedErr error
	if sid != "" && s.scopedRules != nil {
		scoped, scopedErr = s.scopedRules(sid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reloadLocked(false)
	out := append(slices.Clone(s.global), scoped...)
	if s.broken != nil {
		scopedErr = s.broken
	}
	return append(out, s.session[sid]...), scopedErr
}

func (s *ruleStore) AddRule(sid string, r permission.Rule) error {
	if err := r.Validate(); err != nil {
		return err
	}
	// The user asked for this one. Before the file, so it never waits.
	if r.Scope == permission.ScopeGlobal && r.Effect == permission.Allow && s.approve != nil {
		if err := s.approve(r); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.Scope {
	case permission.ScopeSession:
		r.Source = "session " + sid
		s.session[sid] = append(s.session[sid], r)
		return nil
	case permission.ScopeGlobal:
		s.reloadLocked(false)
		if s.broken != nil {
			return s.broken
		}
		if slices.ContainsFunc(s.global, func(g permission.Rule) bool { return sameRule(g, r) }) {
			return nil
		}
		next := append(slices.Clone(s.global), r)
		if err := config.SaveRules(s.dir, next); err != nil {
			return err
		}
		s.reloadLocked(true)
		return nil
	}
	return errors.New("unknown rule scope")
}

func sameRule(a, b permission.Rule) bool {
	return a.Effect == b.Effect && a.Tool == b.Tool && a.Command == b.Command && a.Path == b.Path && a.Literal == b.Literal
}

// reload rereads rules.yaml if it changed on disk.
func (s *ruleStore) reload() {
	s.mu.Lock()
	s.reloadLocked(false)
	s.mu.Unlock()
}

// forget drops a deleted session's rules.
func (s *ruleStore) forget(sid string) {
	s.mu.Lock()
	delete(s.session, sid)
	s.mu.Unlock()
}

// restore rebuilds a session's rules from its transcript.
func (s *ruleStore) restore(sid string, events []event.Event) {
	var rules []permission.Rule
	for _, ev := range events {
		if ev.Type == event.SessionRuleChanged {
			var d sessionRuleChange
			if json.Unmarshal(ev.Data, &d) != nil {
				continue
			}
			if d.Removed {
				rules = slices.DeleteFunc(rules, func(r permission.Rule) bool { return sameRule(r, d.Rule) })
			} else {
				d.Rule.Scope, d.Rule.Source = permission.ScopeSession, "session "+sid
				rules = append(rules, d.Rule)
			}
			continue
		}
		if ev.Type != event.PermissionDecided {
			continue
		}
		var d permission.Decision
		if json.Unmarshal(ev.Data, &d) != nil || d.Persisted == nil || d.Persisted.Scope != permission.ScopeSession {
			continue
		}
		r := *d.Persisted
		r.Source = "session " + sid
		rules = append(rules, r)
	}
	s.mu.Lock()
	s.session[sid] = rules
	s.mu.Unlock()
}

// sessionRuleChange is the payload of session_rule_changed.
type sessionRuleChange struct {
	Rule    permission.Rule `json:"rule"`
	Removed bool            `json:"removed,omitempty"`
}

// deleteSession removes a rule from a session and reports whether it had it.
func (s *ruleStore) deleteSession(sid string, r permission.Rule) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	before := len(s.session[sid])
	s.session[sid] = slices.DeleteFunc(slices.Clone(s.session[sid]), func(g permission.Rule) bool { return sameRule(g, r) })
	return len(s.session[sid]) != before
}

// ListRules returns the rules of a session, global first; with an empty session
// ID, only the global rules. One waiting for approval says so, and can be deleted.
func (e *Engine) ListRules(sessionID string) []permission.Rule {
	rules, _ := e.rules.all(sessionID)
	if rules == nil {
		rules = []permission.Rule{}
	}
	for i := range rules {
		rules[i].Pending = e.rules.pending(rules[i])
	}
	return rules
}

// AddRule adds a global rule, as if written into rules.yaml.
func (e *Engine) AddRule(r permission.Rule) error {
	r.Scope = permission.ScopeGlobal
	return e.rules.AddRule("", r)
}

// DeleteRule removes a global rule from rules.yaml, and with it the approval of
// an allow rule, so a file bringing it back has to ask. Session rules last as
// long as their session.
func (e *Engine) DeleteRule(r permission.Rule) error {
	if err := e.deleteRule(r); err != nil {
		return err
	}
	if r.Effect != permission.Allow {
		return nil
	}
	return e.revokeRule(r)
}

func (e *Engine) deleteRule(r permission.Rule) error {
	s := e.rules
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reloadLocked(false)
	if s.broken != nil {
		return s.broken
	}
	i := slices.IndexFunc(s.global, func(g permission.Rule) bool { return sameRule(g, r) })
	if i < 0 {
		return errors.New("no such global rule")
	}
	if err := config.SaveRules(s.dir, slices.Delete(slices.Clone(s.global), i, i+1)); err != nil {
		return err
	}
	s.reloadLocked(true)
	return nil
}
