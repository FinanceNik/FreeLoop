package acp

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// An account's limits, for an agent built on the Claude Agent SDK (GUI-028). The
// agent says how much of each it has used, since it is the one signed in. Only these
// numbers are read: Freeloop never sees the sign-in, which is what lets a subscription
// be used through it at all (HRN-008).

// sdkRateLimit is a rate_limit_event's rate_limit_info (checked with Claude Code
// through adapter 0.99.1). Utilization is a share of one in the windows, and matches
// what /usage prints as a percentage.
type sdkRateLimit struct {
	Status         string                    `json:"status"`
	UnifiedWindows map[string]sdkLimitWindow `json:"unifiedWindows"`
}

type sdkLimitWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    int64    `json:"resetsAt"`
	Status      string   `json:"status"`
}

// limitLabels names the windows in words. A window with no name here still shows.
var limitLabels = map[string]string{
	"five_hour": "Session limit (5 hours)",
	"seven_day": "Weekly limit",
}

func limitLabel(id string) string {
	if label, ok := limitLabels[id]; ok {
		return label
	}
	words := strings.ReplaceAll(id, "_", " ")
	return strings.ToUpper(words[:1]) + words[1:] + " limit"
}

// limitStatus puts the agent's word in the event's. An unknown one reads as allowed:
// nothing has been refused that the agent said so of.
func limitStatus(status string) string {
	switch status {
	case "allowed_warning":
		return "warning"
	case "rejected":
		return "rejected"
	}
	return "allowed"
}

// limitWindows are a report's windows, the shortest-lived first. One without a figure
// or a reset time says nothing that can be shown.
func limitWindows(r *sdkRateLimit) []event.LimitWindow {
	out := []event.LimitWindow{}
	for id, w := range r.UnifiedWindows {
		if id == "" || w.Utilization == nil || w.ResetsAt <= 0 {
			continue
		}
		out = append(out, event.LimitWindow{
			ID: id, Label: limitLabel(id), ResetsAt: w.ResetsAt, Status: limitStatus(w.Status),
			UsedPercent: min(100, max(0, *w.Utilization*100)),
		})
	}
	slices.SortFunc(out, func(a, b event.LimitWindow) int {
		if a.ResetsAt != b.ResetsAt {
			return int(a.ResetsAt - b.ResetsAt)
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// reportLimits records a report that differs from the last. The agent sends one with
// every model call, and they rarely change.
func (s *session) reportLimits(r *sdkRateLimit) {
	if r == nil {
		return
	}
	windows := limitWindows(r)
	if len(windows) == 0 {
		return
	}
	key, err := json.Marshal(windows)
	if err != nil {
		return
	}
	s.mu.Lock()
	same := s.lastLimits == string(key)
	s.lastLimits = string(key)
	s.mu.Unlock()
	if !same {
		s.p.Emit(event.LimitUpdate, event.LimitData{Source: s.preset.Name, Windows: windows})
	}
}
