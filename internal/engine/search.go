package engine

import (
	"context"

	"github.com/FinanceNik/Freeloop/internal/store"
)

// SessionHits are one session's search results, best first (SES-007).
type SessionHits struct {
	SessionID string      `json:"sessionId"`
	Hits      []store.Hit `json:"hits"`
}

// hitsPerSession caps how many results one session shows.
const hitsPerSession = 3

// Search finds query in every session's messages and tool calls and groups
// the results by session, sessions with the best match first.
func (e *Engine) Search(ctx context.Context, query string) ([]SessionHits, error) {
	hits, err := e.store.Search(ctx, query, 150, hitsPerSession)
	if err != nil {
		return nil, err
	}
	out := []SessionHits{}
	at := map[string]int{}
	for _, h := range hits {
		i, ok := at[h.SessionID]
		if !ok {
			i = len(out)
			at[h.SessionID] = i
			out = append(out, SessionHits{SessionID: h.SessionID})
		}
		if len(out[i].Hits) < hitsPerSession {
			out[i].Hits = append(out[i].Hits, h)
		}
	}
	return out, nil
}
