package permission

// Verdict says what the broker would do with a call, and why, without the call
// being made (SEC-018).
type Verdict struct {
	// Outcome is allowed or denied, or "ask" when the user would be asked.
	Outcome string `json:"outcome"`
	Actor   Actor  `json:"actor,omitempty"`
	Reason  string `json:"reason"`
	// Rule is the rule that decided, when one did.
	Rule *Rule `json:"rule,omitempty"`
	// Overruled are rules that match the call but did not decide it: an allow
	// rule a deny rule or the mode beat, or a deny rule that lost to nothing
	// because a protected path came first.
	Overruled []Rule `json:"overruled,omitempty"`
	// RulesUnreadable says some rules could not be read, so the broker fails closed.
	RulesUnreadable bool `json:"rulesUnreadable,omitempty"`
}

// Ask is a Verdict's outcome when the user would decide.
const Ask = "ask"

// Explain judges req under rules exactly as Request would, through the same code,
// but records nothing, asks no one and runs no hook: hooks can only deny, and are
// the caller's to mention.
func (b *Broker) Explain(pc Context, req Request, rules []Rule, rulesErr error) Verdict {
	if len(req.Options) == 0 {
		req.Options = DefaultOptions
	}
	d, settled := b.decide(pc, req, rules, rulesErr)
	v := Verdict{Outcome: Ask, Actor: d.Actor, Reason: d.Reason, Rule: d.Rule, RulesUnreadable: rulesErr != nil}
	switch {
	case req.Form != nil:
		v = Verdict{Outcome: Ask, Reason: "a request for input always goes to the user"}
	case settled:
		v.Outcome = string(d.Outcome)
	case rulesErr != nil:
		v.Reason = "some rules could not be read, so the user is asked"
	case req.LeavesSandbox:
		v.Reason = "leaving the sandbox always asks"
	case req.Kind == KindSwitchMode:
		v.Reason = "changing the mode always asks"
	default:
		v.Reason = "no rule or mode decides it, so the user is asked"
	}
	for i := range rules {
		if rules[i].Matches(req, pc.Root) && (v.Rule == nil || rules[i] != *v.Rule) {
			v.Overruled = append(v.Overruled, rules[i])
		}
	}
	return v
}
