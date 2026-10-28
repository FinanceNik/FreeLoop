// Package harness defines what every harness offers the engine (HRN-001):
// the built-in loop and, from M2, external agents over ACP. The GUI never
// special-cases one; everything it shows comes from the events a session
// emits.
package harness

import (
	"context"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
)

// Info describes a harness.
type Info struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"` // builtin | acp
}

// Emitter records an event in the session's transcript; the store persists
// it before the UI sees it.
type Emitter func(typ event.Type, data any)

// Params is what a session runs with.
type Params struct {
	SessionID string
	Env       environment.Environment
	Broker    *permission.Broker
	Provider  provider.Provider
	Model     string
	Effort    string
	Mode      permission.Mode
	// History restores a built-in session's context after a restart.
	History []provider.Message
	Emit    Emitter
}

// Harness creates sessions.
type Harness interface {
	Info() Info
	NewSession(ctx context.Context, p Params) (Session, error)
}

// StopReason says why a prompt ended.
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"
	StopMaxTokens StopReason = "max_tokens"
	StopMaxTurns  StopReason = "max_turns"
	StopRefusal   StopReason = "refusal"
	StopCancelled StopReason = "cancelled"
	StopError     StopReason = "error"
)

// Session is one running conversation.
type Session interface {
	// Prompt runs one user turn to completion. Cancelling ctx stops it.
	Prompt(ctx context.Context, text string) (StopReason, error)
	Mode() permission.Mode
	// SetMode takes effect on the next tool call (GUI-007).
	SetMode(m permission.Mode)
}
