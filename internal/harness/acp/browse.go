package acp

import (
	"context"
	"errors"
	"fmt"
	"time"

	sdk "github.com/coder/acp-go-sdk"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
)

// Importing an agent's own sessions (SES-020) goes through the protocol, not
// the agent's files: session/list finds them, session/fork copies one so the
// original stays as it was, and session/load replays it. None of them calls a
// model.

// maxStoredSessions bounds a listing: an agent decides how many pages it sends.
const maxStoredSessions = 5000

// Browser is a running agent that only lists, copies and replays its sessions.
type Browser struct {
	s *session
}

// Browse starts the agent. The caller closes the Browser.
func (h Harness) Browse(ctx context.Context, env environment.Environment) (*Browser, error) {
	sp, ok := env.(environment.Spawner)
	if !ok {
		return nil, errors.New("acp: this environment cannot start processes")
	}
	s := &session{preset: h.Preset, env: env, browsing: true, terms: map[string]*terminal{}}
	ctx, cancel := context.WithTimeout(ctx, StartTimeout)
	defer cancel()
	if _, err := s.start(ctx, sp); err != nil {
		return nil, err
	}
	b := &Browser{s: s}
	if !s.caps.LoadSession || s.caps.SessionCapabilities.List == nil {
		b.Close()
		return nil, fmt.Errorf("%s cannot list and load its sessions", h.Preset.Name)
	}
	return b, nil
}

// Close ends the agent.
func (b *Browser) Close() { b.s.proc.Kill() }

// List returns the agent's sessions in folder, or every one it finds without
// asking for a folder when folder is empty.
func (b *Browser) List(ctx context.Context, folder string) ([]harness.StoredSession, error) {
	req := sdk.ListSessionsRequest{}
	if folder != "" {
		req.Cwd = &folder
	}
	var out []harness.StoredSession
	for len(out) < maxStoredSessions {
		res, err := b.s.conn.ListSessions(ctx, req)
		if err != nil {
			return out, fmt.Errorf("%s: list sessions: %w", b.s.preset.Name, err)
		}
		for _, i := range res.Sessions {
			st := harness.StoredSession{ID: string(i.SessionId), Folder: i.Cwd}
			if i.Title != nil {
				st.Title = *i.Title
			}
			if i.UpdatedAt != nil {
				st.UpdatedAt, _ = time.Parse(time.RFC3339Nano, *i.UpdatedAt)
			}
			out = append(out, st)
		}
		if res.NextCursor == nil || *res.NextCursor == "" {
			break
		}
		req.Cursor = res.NextCursor
	}
	return out, nil
}

// CanFork reports whether the agent can copy a session.
func (b *Browser) CanFork() bool { return b.s.caps.SessionCapabilities.Fork != nil }

// Fork copies a session in the agent's own store and returns the copy's ID.
func (b *Browser) Fork(ctx context.Context, id string) (string, error) {
	res, err := b.s.conn.UnstableForkSession(ctx, sdk.UnstableForkSessionRequest{SessionId: sdk.SessionId(id), Cwd: b.s.env.Root()})
	if err != nil {
		return "", fmt.Errorf("%s: copy the session: %w", b.s.preset.Name, err)
	}
	return string(res.SessionId), nil
}

// Replay loads a session and hands what the agent replays to emit, in order.
func (b *Browser) Replay(ctx context.Context, id string, emit func(event.Type, map[string]any)) error {
	s := b.s
	s.mu.Lock()
	s.replay = emit
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.replay = nil
		s.mu.Unlock()
	}()
	if _, err := s.conn.LoadSession(ctx, sdk.LoadSessionRequest{SessionId: sdk.SessionId(id), Cwd: s.env.Root(), McpServers: s.mcpServers()}); err != nil {
		return fmt.Errorf("%s: load the session: %w", s.preset.Name, err)
	}
	return nil
}
