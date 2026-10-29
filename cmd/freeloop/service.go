package main

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// EngineService exposes the engine to the frontend. It is a thin adapter:
// each method forwards to the engine and adds nothing, except ChooseFolder,
// which is a native dialog.
type EngineService struct {
	eng *engine.Engine
	app *application.App
}

func (s *EngineService) ListSessions(ctx context.Context) ([]session.Session, error) {
	return s.eng.ListSessions(ctx)
}

func (s *EngineService) CreateSession(ctx context.Context, req engine.CreateSessionRequest) (session.Session, error) {
	return s.eng.CreateSession(ctx, req)
}

func (s *EngineService) Events(ctx context.Context, sessionID string, afterSeq int64) ([]event.Event, error) {
	return s.eng.Events(ctx, sessionID, afterSeq)
}

func (s *EngineService) Prompt(ctx context.Context, sessionID, text string) (engine.PromptResult, error) {
	return s.eng.Prompt(ctx, sessionID, text)
}

func (s *EngineService) Cancel(sessionID string) error {
	return s.eng.Cancel(sessionID)
}

func (s *EngineService) SetMode(ctx context.Context, sessionID, mode string) error {
	return s.eng.SetMode(ctx, sessionID, mode)
}

func (s *EngineService) Decide(req engine.DecideRequest) error {
	return s.eng.Decide(req)
}

func (s *EngineService) PendingPermissions() []permission.Request {
	return s.eng.PendingPermissions()
}

func (s *EngineService) ListProviders() []engine.ProviderInfo {
	return s.eng.ListProviders()
}

func (s *EngineService) SaveProvider(req engine.SaveProviderRequest) (engine.ProviderInfo, error) {
	return s.eng.SaveProvider(req)
}

func (s *EngineService) DeleteProvider(id string) error {
	return s.eng.DeleteProvider(id)
}

func (s *EngineService) TestProvider(ctx context.Context, id string) ([]provider.ModelInfo, error) {
	return s.eng.TestProvider(ctx, id)
}

func (s *EngineService) ProviderPresets() []engine.PresetInfo {
	return s.eng.ProviderPresets()
}

func (s *EngineService) ModelCapabilities(ctx context.Context, providerID, model string, probe bool) (provider.ModelCaps, error) {
	return s.eng.ModelCapabilities(ctx, providerID, model, probe)
}

func (s *EngineService) SetToolsOverride(providerID, model string, tools *bool) error {
	return s.eng.SetToolsOverride(providerID, model, tools)
}

func (s *EngineService) ListRules(sessionID string) []permission.Rule {
	return s.eng.ListRules(sessionID)
}

func (s *EngineService) AddRule(r permission.Rule) error {
	return s.eng.AddRule(r)
}

func (s *EngineService) DeleteRule(r permission.Rule) error {
	return s.eng.DeleteRule(r)
}

func (s *EngineService) ListProjects(ctx context.Context) ([]engine.ProjectInfo, error) {
	return s.eng.ListProjects(ctx)
}

func (s *EngineService) AddProject(ctx context.Context, root string) (session.Project, error) {
	return s.eng.AddProject(ctx, root)
}

func (s *EngineService) RelinkProject(ctx context.Context, id, root string) (session.Project, error) {
	return s.eng.RelinkProject(ctx, id, root)
}

func (s *EngineService) SetProjectEnvironment(ctx context.Context, projectID, envID string) error {
	return s.eng.SetProjectEnvironment(ctx, projectID, envID)
}

func (s *EngineService) ListEnvironments() []engine.EnvironmentInfo {
	return s.eng.ListEnvironments()
}

func (s *EngineService) SaveEnvironment(req engine.SaveEnvironmentRequest) (engine.EnvironmentInfo, error) {
	return s.eng.SaveEnvironment(req)
}

func (s *EngineService) DeleteEnvironment(id string) error {
	return s.eng.DeleteEnvironment(id)
}

func (s *EngineService) Config() config.Config {
	return s.eng.Config()
}

func (s *EngineService) ConfigIssues() []config.Issue {
	return s.eng.ConfigIssues()
}

// ChooseFolder shows a native folder picker and returns the chosen path, or
// "" when the user cancels.
func (s *EngineService) ChooseFolder() (string, error) {
	return s.app.Dialog.OpenFile().
		SetTitle("Choose a project folder").
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		PromptForSingleSelection()
}
