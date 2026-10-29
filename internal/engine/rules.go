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
	if len(issues) > 0 && s.report != nil {
		s.report(issues)
	}
	s.mod = mod
	switch {
	case errors.Is(err, config.ErrRulesInvalid):
		// Keep the valid rules, and the last good ones if nothing parsed.
		if rules != nil {
			s.global = rules
		}
		s.broken = fmt.Errorf("%s has errors; fix the file (see the problems listed in the app) before rules change: %w", config.RulesFileName, err)
	case err != nil:
		s.broken = err
		if s.report != nil {
			s.report([]config.Issue{{File: config.RulesFileName, Message: err.Error()}})
		}
	default:
		s.global, s.broken = rules, nil
	}
}

func (s *ruleStore) Rules(sid string) ([]permission.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reloadLocked(false)
	return append(slices.Clone(s.global), s.session[sid]...), s.broken
}

func (s *ruleStore) AddRule(sid string, r permission.Rule) error {
	if err := r.Validate(); err != nil {
		return err
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

// restore rebuilds a session's rules from its transcript.
func (s *ruleStore) restore(sid string, events []event.Event) {
	var rules []permission.Rule
	for _, ev := range events {
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

// ListRules returns the rules in force for a session, global first; with
// an empty session ID, only the global rules.
func (e *Engine) ListRules(sessionID string) []permission.Rule {
	rules, _ := e.rules.Rules(sessionID)
	if rules == nil {
		rules = []permission.Rule{}
	}
	return rules
}

// AddRule adds a global rule, as if written into rules.yaml.
func (e *Engine) AddRule(r permission.Rule) error {
	r.Scope = permission.ScopeGlobal
	return e.rules.AddRule("", r)
}

// DeleteRule removes a global rule from rules.yaml. Session rules last as
// long as their session.
func (e *Engine) DeleteRule(r permission.Rule) error {
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
