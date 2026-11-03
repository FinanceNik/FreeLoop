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
	"strings"
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
	Paths   []string `json:"paths,omitempty"`
	Command string   `json:"command,omitempty"`
	// Contained says the caller will run this command in the sandbox, and so
	// it needs no approval (ENV-009). Only a caller that runs the command
	// itself can promise that; an ACP agent's own request never can.
	Contained Containment `json:"contained,omitempty"`
	// RewrittenReadOnly says the command is one the agent would not have asked
	// about before a token optimizer rewrote it, and that only reads (HRN-019). Only
	// a caller holding the original can say so, and Command is that original.
	RewrittenReadOnly bool `json:"rewrittenReadOnly,omitempty"`
	// LeavesSandbox asks to run a command outside the sandbox: no rule covers
	// it, so it always reaches the user.
	LeavesSandbox bool      `json:"leavesSandbox,omitempty"`
	Options       []Option  `json:"options"`
	Created       time.Time `json:"createdAt"`
	// Form is set when the request asks the user for input rather than
	// for permission: an MCP server's elicitation (MCP-008). It is always
	// put to the user; no rule, mode or hook answers it.
	Form *Form `json:"form,omitempty"`
}

// Containment is how a command is held in the sandbox.
type Containment string

const (
	// ContainedProject can write the project.
	ContainedProject Containment = "project"
	// ContainedReadOnly cannot, so plan mode can run it too.
	ContainedReadOnly Containment = "read_only"
)

// Form is an MCP server's request for input: fields described by a JSON
// schema, or a URL for the user to open.
type Form struct {
	Server  string          `json:"server"`
	Message string          `json:"message"`
	Schema  json.RawMessage `json:"schema,omitempty"`
	URL     string          `json:"url,omitempty"`
}

// FormOptions answer a form: send the answers, or decline.
var FormOptions = []Option{
	{ID: "accept", Name: "Send", Kind: "allow_once"},
	{ID: "decline", Name: "Decline", Kind: "reject_once"},
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
	// ActorHook is a pre_tool_use or permission_request hook (EXT-005).
	ActorHook Actor = "hook"
	// ActorSandbox ran a command because it cannot leave the sandbox (ENV-009).
	ActorSandbox Actor = "sandbox"
)

// Hooks runs the user's hooks for a request (EXT-005). Each returns a
// reason when a hook blocks the call, or "" to go on. Hooks can block but
// never approve.
type Hooks interface {
	// PreToolUse runs for a call the path policy and the deny rules let
	// through, before it is allowed or asked about.
	PreToolUse(ctx context.Context, pc Context, req Request) string
	// PermissionRequest runs before the user is asked.
	PermissionRequest(ctx context.Context, pc Context, req Request) string
}

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
	// Answers are a form's values (MCP-008). They go back to the asker but
	// not into the transcript, which records only the decision.
	Answers map[string]any `json:"-"`
}

// Emitter records an event in a session's transcript.
type Emitter func(sessionID string, typ event.Type, data any)

// Broker is safe for concurrent use.
type Broker struct {
	policy *Policy
	emit   Emitter
	rules  RuleSource
	sink   RuleSink
	hooks  Hooks

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

// SetHooks connects the hooks. Call it before the first Request.
func (b *Broker) SetHooks(h Hooks) { b.hooks = h }

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
	if req.Form != nil {
		// Input only the user can give.
		d, settled = Decision{}, false
	}
	// Hooks run only for calls that are not denied already: a deny wins
	// without running anything.
	if b.hooks != nil && req.Form == nil && (!settled || d.Outcome != Denied) {
		if why := b.hooks.PreToolUse(ctx, pc, req); why != "" {
			d, settled = Decision{Outcome: Denied, Actor: ActorHook, Reason: why}, true
		} else if !settled {
			if why := b.hooks.PermissionRequest(ctx, pc, req); why != "" {
				d, settled = Decision{Outcome: Denied, Actor: ActorHook, Reason: why}, true
			}
		}
	}
	if !settled {
		d = b.ask(ctx, pc, req)
	}
	d.RequestID, d.ToolCallID = req.ID, req.ToolCallID
	b.emit(req.SessionID, event.PermissionDecided, d)
	return d
}

// Hidden reports whether a deny rule refuses reading path, for leaving it
// out of search results: a search names a folder, and a deny rule on a
// file inside it would otherwise be read through grep. Rules that cannot
// be loaded hide everything.
func (b *Broker) Hidden(sessionID, root, path string) bool {
	if b.rules == nil {
		return false
	}
	rules, err := b.rules.Rules(sessionID)
	if err != nil {
		return true
	}
	for _, r := range rules {
		if r.Effect != Deny || r.Path == "" {
			continue
		}
		for _, req := range []Request{
			{Tool: "read_file", Kind: KindRead, Paths: []string{path}},
			{Tool: "grep", Kind: KindSearch, Paths: []string{path}},
			{Tool: "glob", Kind: KindSearch, Paths: []string{path}},
		} {
			if r.Matches(req, root) {
				return true
			}
		}
	}
	return false
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
	var rulesErr error
	if b.rules != nil {
		rules, rulesErr = b.rules.Rules(req.SessionID)
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
	// Rules that cannot be read may include deny rules: fail closed and
	// ask for everything except reads inside the project.
	if rulesErr != nil {
		if (req.Kind.reads() || req.Kind == KindThink) && !outside {
			return allow(ActorPolicy, "reads inside the project are allowed")
		}
		return Decision{}, false
	}

	// A rewritten command names its files only in its text, and a search can reach
	// them with no name at all, so a deny rule on a path cannot be checked against
	// it: whatever the rule's tool, the user decides instead (HRN-019, SEC-002).
	rewrittenReadOnly := req.Kind == KindExecute && req.RewrittenReadOnly &&
		!slices.ContainsFunc(rules, func(r Rule) bool { return r.Effect == Deny && r.Path != "" })
	rewrittenHidesPath := req.Kind == KindExecute && req.RewrittenReadOnly && !rewrittenReadOnly

	switch pc.Mode {
	case ModeBypass:
		// Leaving the sandbox asks every time, whatever the mode: a user who chose
		// a sandbox has not chosen to let the model walk out of it.
		if req.LeavesSandbox {
			return Decision{}, false
		}
		return allow(ActorMode, "bypass mode")
	case ModePlan:
		switch {
		case req.Kind == KindSwitchMode:
			return Decision{}, false
		case req.Kind.reads() || req.Kind == KindThink:
		case req.Kind == KindExecute && req.Contained == ContainedReadOnly:
			return allow(ActorSandbox, "it runs in the sandbox, read-only")
		case rewrittenReadOnly:
			return allow(ActorPolicy, "it only reads inside the project")
		case rewrittenHidesPath:
			return Decision{}, false
		default:
			return deny(ActorMode, "plan mode is read-only until the plan is approved")
		}
	}
	if req.Kind != KindSwitchMode && !req.LeavesSandbox {
		if r := match(Allow); r != nil {
			return byRule(r)
		}
	}
	if rewrittenReadOnly {
		return allow(ActorPolicy, "it only reads inside the project")
	}
	if req.Kind == KindExecute && req.Contained != "" {
		// Nothing else stands between the model and a deny rule here, and the rule
		// reads a line that may hide its command in an expansion.
		if !plain(req.Command) && slices.ContainsFunc(rules, func(r Rule) bool { return r.Effect == Deny && r.Tool == "shell" && r.Command != "" }) {
			return Decision{}, false
		}
		return allow(ActorSandbox, "it runs in the sandbox")
	}
	if pc.Mode == ModeAcceptEdits && req.Kind.writes() && len(req.Paths) > 0 && !outside && !b.policy.actedOn(pc.Root, req.Paths) {
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
	return b.DecideWith(requestID, optionID, reason, scope, nil)
}

// DecideWith is Decide with a form's answers, for a request with a Form.
func (b *Broker) DecideWith(requestID, optionID, reason string, scope Scope, answers map[string]any) error {
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
	if p.req.Form != nil && p.req.Options[i].Kind == "allow_once" {
		d.Answers = answers
	}
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
		r, ok := RuleFor(p.req, p.pc.Root, scope)
		if !ok {
			d.Reason = strings.TrimSpace(d.Reason + " Allowed once: one rule cannot cover this call, so none was saved.")
			break
		}
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
