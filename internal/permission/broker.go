// Package permission is the single chokepoint for side effects (SEC-003).
// Every tool call from every harness asks the Broker, which applies, in
// order: the fixed path policy, deny rules, the session's mode, allow rules,
// the default policy for reads, and queues the rest for the user. Deny wins
// at every scope. Nothing is approved by timeout. Every decision is recorded
// in the transcript with its actor (SEC-007).
package permission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// Mode is a session's permission mode (SEC-001).
type Mode string

const (
	ModeAsk         Mode = "ask"
	ModeAcceptEdits Mode = "accept_edits"
	ModePlan        Mode = "plan"
	ModeBypass      Mode = "bypass"
)

// Modes lists every mode in display order.
var Modes = []Mode{ModeAsk, ModeAcceptEdits, ModePlan, ModeBypass}

func (m Mode) Valid() bool { return slices.Contains(Modes, m) }

// Kind is ACP's tool kind.
type Kind string

const (
	KindRead       Kind = "read"
	KindEdit       Kind = "edit"
	KindDelete     Kind = "delete"
	KindMove       Kind = "move"
	KindSearch     Kind = "search"
	KindExecute    Kind = "execute"
	KindThink      Kind = "think"
	KindFetch      Kind = "fetch"
	KindSwitchMode Kind = "switch_mode"
	KindOther      Kind = "other"
)

func (k Kind) writes() bool { return k == KindEdit || k == KindDelete || k == KindMove }
func (k Kind) reads() bool  { return k == KindRead || k == KindSearch }

// Option is one answer the user can give, shaped like ACP's PermissionOption.
type Option struct {
	ID   string `json:"optionId"`
	Name string `json:"name"`
	Kind string `json:"kind"` // allow_once | allow_always | reject_once | reject_always
}

// DefaultOptions are offered when a request names none.
var DefaultOptions = []Option{
	{ID: "allow_once", Name: "Allow once", Kind: "allow_once"},
	{ID: "allow_always", Name: "Allow always", Kind: "allow_always"},
	{ID: "reject_once", Name: "Deny", Kind: "reject_once"},
}

// Request is one tool call asking to run.
type Request struct {
	ID         string          `json:"requestId"`
	SessionID  string          `json:"sessionId"`
	ToolCallID string          `json:"toolCallId"`
	Tool       string          `json:"tool"`
	Kind       Kind            `json:"kind"`
	Title      string          `json:"title"`
	Input      json.RawMessage `json:"input,omitempty"`
	// Paths the call reads or writes, absolute and clean.
	Paths   []string  `json:"paths,omitempty"`
	Command string    `json:"command,omitempty"`
	Options []Option  `json:"options"`
	Created time.Time `json:"createdAt"`
}

// Context is the session state a request is judged in.
type Context struct {
	Mode Mode
	Root string
}

// Outcome of a request.
type Outcome string

const (
	Allowed   Outcome = "allowed"
	Denied    Outcome = "denied"
	Cancelled Outcome = "cancelled"
)

// Actor is who decided.
type Actor string

const (
	ActorUser          Actor = "user"
	ActorMode          Actor = "mode"
	ActorProtectedPath Actor = "protected_path"
	ActorPolicy        Actor = "policy"
	ActorRule          Actor = "rule"
)

// Decision is the broker's answer.
type Decision struct {
	RequestID  string  `json:"requestId"`
	ToolCallID string  `json:"toolCallId"`
	Outcome    Outcome `json:"outcome"`
	OptionID   string  `json:"optionId,omitempty"`
	Actor      Actor   `json:"actor"`
	Reason     string  `json:"reason,omitempty"`
	// Rule is the rule that decided, when Actor is rule.
	Rule *Rule `json:"rule,omitempty"`
	// Persisted is the rule "Allow always" created.
	Persisted *Rule `json:"persisted,omitempty"`
}

// Emitter records an event in a session's transcript.
type Emitter func(sessionID string, typ event.Type, data any)

// Broker is safe for concurrent use.
type Broker struct {
	policy *Policy
	emit   Emitter
	rules  RuleSource
	sink   RuleSink

	mu      sync.Mutex
	next    int
	pending map[string]*pending
}

type pending struct {
	req    Request
	pc     Context
	answer chan Decision
}

// NewBroker returns a broker that records decisions through emit.
func NewBroker(policy *Policy, emit Emitter) *Broker {
	return &Broker{policy: policy, emit: emit, pending: map[string]*pending{}}
}

// SetRules connects the rules in force and the store for "Allow always".
// Call it before the first Request.
func (b *Broker) SetRules(source RuleSource, sink RuleSink) {
	b.rules, b.sink = source, sink
}

// Policy returns the fixed path policy.
func (b *Broker) Policy() *Policy { return b.policy }

// Request decides req, asking the user when neither the path policy nor the
// mode settles it. It blocks until the user answers or ctx ends.
func (b *Broker) Request(ctx context.Context, pc Context, req Request) Decision {
	b.mu.Lock()
	b.next++
	req.ID = fmt.Sprintf("perm_%d_%d", time.Now().UnixMilli(), b.next)
	b.mu.Unlock()
	if len(req.Options) == 0 {
		req.Options = DefaultOptions
	}
	req.Created = time.Now().UTC()

	d, settled := b.evaluate(pc, req)
	if !settled {
		d = b.ask(ctx, pc, req)
	}
	d.RequestID, d.ToolCallID = req.ID, req.ToolCallID
	b.emit(req.SessionID, event.PermissionDecided, d)
	return d
}

// evaluate applies the path policy and the mode. It returns false when the
// user has to decide.
func (b *Broker) evaluate(pc Context, req Request) (Decision, bool) {
	deny := func(a Actor, why string) (Decision, bool) {
		return Decision{Outcome: Denied, Actor: a, Reason: why}, true
	}
	allow := func(a Actor, why string) (Decision, bool) {
		return Decision{Outcome: Allowed, Actor: a, Reason: why}, true
	}

	var rules []Rule
	if b.rules != nil {
		rules = b.rules.Rules(req.SessionID)
	}
	match := func(effect Effect) *Rule {
		for i := range rules {
			if rules[i].Effect == effect && rules[i].Matches(req, pc.Root) {
				return &rules[i]
			}
		}
		return nil
	}
	byRule := func(r *Rule) (Decision, bool) {
		outcome := Allowed
		if r.Effect == Deny {
			outcome = Denied
		}
		return Decision{Outcome: outcome, Actor: ActorRule, Reason: fmt.Sprintf("%s (%s rule)", r, r.Scope), Rule: r}, true
	}

	outside := false
	for _, p := range req.Paths {
		if why := b.policy.readRefusal(p); why != "" {
			return deny(ActorProtectedPath, why)
		}
		if req.Kind.writes() {
			if why := b.policy.writeRefusal(pc.Root, p); why != "" {
				return deny(ActorProtectedPath, why)
			}
		}
		if !b.policy.insideRoot(pc.Root, p) {
			outside = true
		}
	}

	if r := match(Deny); r != nil {
		return byRule(r)
	}

	switch pc.Mode {
	case ModeBypass:
		return allow(ActorMode, "bypass mode")
	case ModePlan:
		switch {
		case req.Kind == KindSwitchMode:
			return Decision{}, false
		case req.Kind.reads() || req.Kind == KindThink:
		default:
			return deny(ActorMode, "plan mode is read-only until the plan is approved")
		}
	}
	if req.Kind != KindSwitchMode {
		if r := match(Allow); r != nil {
			return byRule(r)
		}
	}
	if pc.Mode == ModeAcceptEdits && req.Kind.writes() && len(req.Paths) > 0 && !outside {
		return allow(ActorMode, "accept-edits mode allows edits inside the project")
	}

	if (req.Kind.reads() || req.Kind == KindThink) && !outside {
		return allow(ActorPolicy, "reads inside the project are allowed")
	}
	return Decision{}, false
}

func (b *Broker) ask(ctx context.Context, pc Context, req Request) Decision {
	p := &pending{req: req, pc: pc, answer: make(chan Decision, 1)}
	b.mu.Lock()
	b.pending[req.ID] = p
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.pending, req.ID)
		b.mu.Unlock()
	}()

	b.emit(req.SessionID, event.PermissionRequested, req)
	b.emit(req.SessionID, event.SessionStatus, event.SessionStatusData{Status: "waiting_permission"})
	defer b.emit(req.SessionID, event.SessionStatus, event.SessionStatusData{Status: "running"})

	select {
	case d := <-p.answer:
		return d
	case <-ctx.Done():
		return Decision{Outcome: Cancelled, Actor: ActorUser, Reason: "the turn was cancelled"}
	}
}

// ErrUnknownRequest is returned by Decide for a request that is not pending.
var ErrUnknownRequest = errors.New("permission: no pending request with that ID")

// Decide answers a pending request with one of its options. For an
// allow_always option the broker stores a rule for the call in scope
// (default session) before the call runs.
func (b *Broker) Decide(requestID, optionID, reason string, scope Scope) error {
	b.mu.Lock()
	p, ok := b.pending[requestID]
	b.mu.Unlock()
	if !ok {
		return ErrUnknownRequest
	}
	i := slices.IndexFunc(p.req.Options, func(o Option) bool { return o.ID == optionID })
	if i < 0 {
		return fmt.Errorf("permission: request %s has no option %q", requestID, optionID)
	}
	d := Decision{Outcome: Denied, OptionID: optionID, Actor: ActorUser, Reason: reason}
	switch p.req.Options[i].Kind {
	case "allow_once":
		d.Outcome = Allowed
	case "allow_always":
		d.Outcome = Allowed
		if scope == "" {
			scope = ScopeSession
		}
		if scope != ScopeSession && scope != ScopeGlobal {
			return fmt.Errorf("permission: unknown scope %q", scope)
		}
		if b.sink == nil {
			return errors.New("permission: rules cannot be saved")
		}
		r := RuleFor(p.req, p.pc.Root, scope)
		if err := b.sink.AddRule(p.req.SessionID, r); err != nil {
			return fmt.Errorf("permission: save rule: %w", err)
		}
		d.Persisted = &r
	}
	select {
	case p.answer <- d:
	default: // already answered
	}
	return nil
}

// Pending returns the requests waiting for the user, oldest first.
func (b *Broker) Pending() []Request {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Request, 0, len(b.pending))
	for _, p := range b.pending {
		out = append(out, p.req)
	}
	slices.SortFunc(out, func(a, b Request) int { return a.Created.Compare(b.Created) })
	return out
}
