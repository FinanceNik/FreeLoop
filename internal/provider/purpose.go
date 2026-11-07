package provider

import "context"

// What a model call is for, carried in its context so the engine can record what each
// kind of call sends (NFR-PRIV-003). A call that says nothing is a turn.
const (
	PurposeTurn          = "turn"
	PurposeCompaction    = "compaction"
	PurposeCommitMessage = "commit_message"
	PurposePullRequest   = "pull_request"
	PurposeHandoff       = "handoff"
)

type purposeKey struct{}

type purpose struct{ name, sent string }

// WithPurpose marks ctx's model calls as being for name; sent says in words what goes
// with the call besides its system prompt and tools, such as "The session's diff".
func WithPurpose(ctx context.Context, name, sent string) context.Context {
	return context.WithValue(ctx, purposeKey{}, purpose{name, sent})
}

// PurposeOf returns what ctx's model calls are for, and what they send besides.
func PurposeOf(ctx context.Context) (name, sent string) {
	if p, ok := ctx.Value(purposeKey{}).(purpose); ok && p.name != "" {
		return p.name, p.sent
	}
	return PurposeTurn, ""
}
