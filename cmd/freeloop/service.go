package main

import (
	"context"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// EngineService exposes the engine to the frontend. It is a thin adapter:
// each method forwards to the engine and adds nothing.
type EngineService struct {
	eng *engine.Engine
}

func (s *EngineService) ListSessions(ctx context.Context) ([]session.Session, error) {
	return s.eng.ListSessions(ctx)
}

func (s *EngineService) CreateSession(ctx context.Context, req engine.CreateSessionRequest) (session.Session, error) {
	return s.eng.CreateSession(ctx, req)
}

func (s *EngineService) AppendEvent(ctx context.Context, req engine.AppendEventRequest) (event.Event, error) {
	return s.eng.AppendEvent(ctx, req)
}

func (s *EngineService) Events(ctx context.Context, sessionID string, afterSeq int64) ([]event.Event, error) {
	return s.eng.Events(ctx, sessionID, afterSeq)
}

func (s *EngineService) Config() config.Config {
	return s.eng.Config()
}

func (s *EngineService) ConfigIssues() []config.Issue {
	return s.eng.ConfigIssues()
}
