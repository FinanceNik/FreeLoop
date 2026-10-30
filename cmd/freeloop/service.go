package main

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/netguard"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// EngineService exposes the engine to the frontend. It is a thin adapter:
// each method forwards to the engine and adds nothing, except ChooseFolder,
// ExportSession, ExportConfig and ImportConfig, which show native dialogs,
// and OpenSkill and OpenFile, which open a file in another app.
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

// Attention lists the sessions that need the user (GUI-002).
func (s *EngineService) Attention(ctx context.Context) ([]engine.AttentionItem, error) {
	return s.eng.Attention(ctx)
}

func (s *EngineService) DismissError(ctx context.Context, sessionID string) error {
	return s.eng.DismissError(ctx, sessionID)
}

// Changes, FileDiff, StageFiles, Commit, DiscardFile and
// SuggestCommitMessage feed the diff panel (GUI-005).
func (s *EngineService) Changes(ctx context.Context, sessionID string) (engine.Changes, error) {
	return s.eng.Changes(ctx, sessionID)
}

func (s *EngineService) FileDiff(ctx context.Context, sessionID, path string) (engine.FileDiff, error) {
	return s.eng.FileDiff(ctx, sessionID, path)
}

func (s *EngineService) StageFiles(ctx context.Context, sessionID string, paths []string, staged bool) error {
	return s.eng.StageFiles(ctx, sessionID, paths, staged)
}

func (s *EngineService) Commit(ctx context.Context, sessionID, message string) (string, error) {
	return s.eng.Commit(ctx, sessionID, message)
}

func (s *EngineService) DiscardFile(ctx context.Context, sessionID, path string) error {
	return s.eng.DiscardFile(ctx, sessionID, path)
}

func (s *EngineService) SuggestCommitMessage(ctx context.Context, sessionID string) (string, error) {
	return s.eng.SuggestCommitMessage(ctx, sessionID)
}

// ListProfiles, SaveProfile, DeleteProfile and SetProjectProfile manage
// harness profiles (HRN-003, HRN-004).
func (s *EngineService) ListProfiles() []config.Profile {
	return s.eng.ListProfiles()
}

func (s *EngineService) SaveProfile(p config.Profile) (config.Profile, error) {
	return s.eng.SaveProfile(p)
}

func (s *EngineService) DeleteProfile(id string) error {
	return s.eng.DeleteProfile(id)
}

func (s *EngineService) SetProjectProfile(ctx context.Context, projectID, profileID string) error {
	return s.eng.SetProjectProfile(ctx, projectID, profileID)
}

// ProjectSettings and SetProjectTrust handle project trust and .freeloop/
// settings (SEC-006, SET-002).
func (s *EngineService) ProjectSettings(ctx context.Context, root string) (engine.ProjectSettings, error) {
	return s.eng.ProjectSettings(ctx, root)
}

func (s *EngineService) SetProjectTrust(ctx context.Context, root string, trusted bool) (engine.ProjectInfo, error) {
	return s.eng.SetProjectTrust(ctx, root, trusted)
}

// SetProjectMuted turns a project's notifications off or on (GUI-008).
func (s *EngineService) SetProjectMuted(ctx context.Context, projectID string, muted bool) error {
	return s.eng.SetProjectMuted(ctx, projectID, muted)
}

// ListSkills and SetSkillEnabled feed the skills page (SKL-002).
func (s *EngineService) ListSkills(ctx context.Context, projectRoot string) (engine.SkillList, error) {
	return s.eng.ListSkills(ctx, projectRoot)
}

func (s *EngineService) SetSkillEnabled(ctx context.Context, name, projectID string, enabled bool) error {
	return s.eng.SetSkillEnabled(ctx, name, projectID, enabled)
}

// PreviewSkills, InstallSkills, RemoveSkill and ClaudeSkillsDir import and
// install skills into the app's store (SKL-003, SKL-005).
func (s *EngineService) PreviewSkills(ctx context.Context, src string) ([]engine.SkillCandidate, error) {
	return s.eng.PreviewSkills(ctx, src)
}

func (s *EngineService) InstallSkills(ctx context.Context, src string, names []string) ([]string, error) {
	return s.eng.InstallSkills(ctx, src, names)
}

func (s *EngineService) RemoveSkill(name string) error {
	return s.eng.RemoveSkill(name)
}

func (s *EngineService) ClaudeSkillsDir() string {
	return s.eng.ClaudeSkillsDir()
}

// The MCP page (MCP-001, MCP-005).
func (s *EngineService) ListMCPServers() []engine.MCPServerInfo {
	return s.eng.ListMCPServers()
}

func (s *EngineService) SaveMCPServer(req engine.SaveMCPServerRequest) (engine.MCPServerInfo, error) {
	return s.eng.SaveMCPServer(req)
}

func (s *EngineService) DeleteMCPServer(id string) error {
	return s.eng.DeleteMCPServer(id)
}

func (s *EngineService) ApproveMCPServer(ctx context.Context, id, fingerprint string) (engine.MCPServerInfo, error) {
	return s.eng.ApproveMCPServer(ctx, id, fingerprint)
}

func (s *EngineService) StartMCPServer(ctx context.Context, id string) error {
	return s.eng.StartMCPServer(ctx, id)
}

func (s *EngineService) StopMCPServer(id string) {
	s.eng.StopMCPServer(id)
}

// PreviewClaudeImport and ApplyClaudeImport import MCP servers and
// permission rules from Claude Code (MCP-002, SET-003).
func (s *EngineService) PreviewClaudeImport(ctx context.Context) (engine.ClaudeImport, error) {
	return s.eng.PreviewClaudeImport(ctx)
}

func (s *EngineService) ApplyClaudeImport(ctx context.Context, keys []string) (int, error) {
	return s.eng.ApplyClaudeImport(ctx, keys)
}

// ListHarnesses lists the built-in harness and the ACP agent presets
// (HRN-004, HRN-005).
func (s *EngineService) ListHarnesses() []engine.HarnessInfo {
	return s.eng.ListHarnesses()
}

// SetModel switches a built-in session's model (PRV-006).
func (s *EngineService) SetModel(ctx context.Context, sessionID, providerID, model string) error {
	return s.eng.SetModel(ctx, sessionID, providerID, model)
}

// ProfileYAML, CheckProfileYAML and SaveProfileYAML back the profile YAML
// editor (GUI-009).
func (s *EngineService) ProfileYAML(id string) (string, error) {
	return s.eng.ProfileYAML(id)
}

func (s *EngineService) CheckProfileYAML(id, text string) []config.Issue {
	return s.eng.CheckProfileYAML(id, text)
}

func (s *EngineService) SaveProfileYAML(id, text string) error {
	return s.eng.SaveProfileYAML(id, text)
}

// MergeWorktree and SetProjectWorktreeDefault handle session worktrees
// (SES-010).
func (s *EngineService) MergeWorktree(ctx context.Context, sessionID string) (engine.MergeResult, error) {
	return s.eng.MergeWorktree(ctx, sessionID)
}

func (s *EngineService) SetProjectWorktreeDefault(ctx context.Context, projectID, choice string) error {
	return s.eng.SetProjectWorktreeDefault(ctx, projectID, choice)
}

// Rewind restores a session to a checkpoint (SES-014).
func (s *EngineService) Rewind(ctx context.Context, sessionID string, seq int64) error {
	return s.eng.Rewind(ctx, sessionID, seq)
}

func (s *EngineService) ForkSession(ctx context.Context, sessionID string, seq int64) (engine.ForkResult, error) {
	return s.eng.ForkSession(ctx, sessionID, seq)
}

func (s *EngineService) SwitchProfile(ctx context.Context, sessionID, profileID string) (session.Session, error) {
	return s.eng.SwitchProfile(ctx, sessionID, profileID)
}

func (s *EngineService) Search(ctx context.Context, query string) ([]engine.SessionHits, error) {
	return s.eng.Search(ctx, query)
}

func (s *EngineService) Usage(ctx context.Context, days int) ([]engine.DayCost, error) {
	return s.eng.Usage(ctx, days)
}

func (s *EngineService) PromptWith(ctx context.Context, sessionID string, req engine.PromptRequest) (engine.PromptResult, error) {
	return s.eng.PromptWith(ctx, sessionID, req)
}

func (s *EngineService) SessionMCP(ctx context.Context, sessionID string) (engine.SessionMCPContext, error) {
	return s.eng.SessionMCP(ctx, sessionID)
}

func (s *EngineService) RunMCPPrompt(ctx context.Context, sessionID, server, name string, args map[string]string) (engine.PromptResult, error) {
	return s.eng.RunMCPPrompt(ctx, sessionID, server, name, args)
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

func (s *EngineService) SetModelFlags(providerID, model string, favorite, hidden bool) error {
	return s.eng.SetModelFlags(providerID, model, favorite, hidden)
}

// ProjectFiles lists a session's project files for the @ picker.
func (s *EngineService) ProjectFiles(ctx context.Context, sessionID string) ([]string, error) {
	return s.eng.ProjectFiles(ctx, sessionID)
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

func (s *EngineService) SaveSettings(c config.Config) (config.Config, error) {
	return s.eng.SaveSettings(c)
}

func (s *EngineService) RenameSession(ctx context.Context, sessionID, title string) error {
	return s.eng.RenameSession(ctx, sessionID, title)
}

func (s *EngineService) ArchiveSession(ctx context.Context, sessionID string, archived bool) error {
	return s.eng.ArchiveSession(ctx, sessionID, archived)
}

func (s *EngineService) DeleteSession(ctx context.Context, sessionID string) error {
	return s.eng.DeleteSession(ctx, sessionID)
}

func (s *EngineService) Config() config.Config {
	return s.eng.Config()
}

func (s *EngineService) ConfigIssues() []config.Issue {
	return s.eng.ConfigIssues()
}

func (s *EngineService) BlockedConnections() []netguard.Blocked {
	return s.eng.BlockedConnections()
}

// ExportSession asks where to save a session's export (SES-008) and writes
// it there; it returns the path, or "" when the user cancels.
func (s *EngineService) ExportSession(ctx context.Context, sessionID, format string) (string, error) {
	ses, err := s.eng.GetSession(ctx, sessionID)
	if err != nil {
		return "", err
	}
	path, err := s.app.Dialog.SaveFile().
		SetMessage("Export the session").
		SetFilename(engine.ExportFileName(ses.Title, format)).
		CanCreateDirectories(true).
		PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	return path, s.eng.ExportSession(ctx, sessionID, format, path)
}

// ExportConfig asks where to save a config backup (SET-005) and writes it;
// it returns the path, or "" when the user cancels.
func (s *EngineService) ExportConfig(ctx context.Context) (string, error) {
	path, err := s.app.Dialog.SaveFile().
		SetMessage("Back up Freeloop's configuration").
		SetFilename("freeloop-config.json").
		CanCreateDirectories(true).
		PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	return path, s.eng.ExportConfig(ctx, path)
}

// ImportConfig asks for a config backup and restores it; nil when the user
// cancels.
func (s *EngineService) ImportConfig(ctx context.Context) (*engine.ConfigRestore, error) {
	path, err := s.app.Dialog.OpenFile().
		SetTitle("Restore Freeloop's configuration").
		CanChooseFiles(true).
		CanChooseDirectories(false).
		AddFilter("Freeloop config backup", "*.json").
		PromptForSingleSelection()
	if err != nil || path == "" {
		return nil, err
	}
	res, err := s.eng.ImportConfig(ctx, path)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (s *EngineService) CreateSkill(name, description string) (string, error) {
	return s.eng.CreateSkill(name, description)
}

func (s *EngineService) TestSkill(ctx context.Context, req engine.SkillTestRequest) (engine.SkillTest, error) {
	return s.eng.TestSkill(ctx, req)
}

// OpenSkill opens a skill's SKILL.md in the app the user opens Markdown
// with (SKL-007).
func (s *EngineService) OpenSkill(name string) error {
	path := s.eng.SkillFile(name)
	if path == "" {
		return fmt.Errorf("the skill %s is not in Freeloop's skill folder", name)
	}
	return s.app.Browser.OpenFile(path)
}

func (s *EngineService) ListFolder(ctx context.Context, sessionID, rel string) ([]engine.FileEntry, error) {
	return s.eng.ListFolder(ctx, sessionID, rel)
}

func (s *EngineService) ViewFile(ctx context.Context, sessionID, rel string) (engine.FileView, error) {
	return s.eng.ViewFile(ctx, sessionID, rel)
}

// OpenFile opens a file of a session's folder in the app the user opens
// that kind of file with (GUI-013).
func (s *EngineService) OpenFile(ctx context.Context, sessionID, rel string) error {
	path, err := s.eng.FileOnHost(ctx, sessionID, rel)
	if err != nil {
		return err
	}
	return s.app.Browser.OpenFile(path)
}

func (s *EngineService) CheckSkillUpdate(ctx context.Context, name string) (engine.SkillCandidate, error) {
	return s.eng.CheckSkillUpdate(ctx, name)
}

func (s *EngineService) UpdateSkill(ctx context.Context, name string) error {
	return s.eng.UpdateSkill(ctx, name)
}

func (s *EngineService) RollbackSkill(name string) error {
	return s.eng.RollbackSkill(name)
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
