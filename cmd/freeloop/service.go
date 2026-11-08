package main

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

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
// ExportSession, ExportConfig and PreviewConfigRestore, which show native dialogs,
// and OpenSkill, OpenFile, OpenHarness and OpenColors, which open a file in the
// user's text editor.
type EngineService struct {
	eng  *engine.Engine
	app  *application.App
	desk *desktop
	// logPath is the app's log file.
	logPath string

	// restoring is the backup last previewed. The page names only its
	// fingerprint: the user chose the file, and nothing else can.
	restoring atomic.Pointer[string]
}

// SetTags replaces a session's tags (SES-013) and returns them cleaned.
func (s *EngineService) SetTags(ctx context.Context, sessionID string, tags []string) ([]string, error) {
	return s.eng.SetTags(ctx, sessionID, tags)
}

// Schedules lists scheduled and recurring sessions (SES-012).
func (s *EngineService) Schedules() ([]engine.ScheduleInfo, error) { return s.eng.Schedules() }

// SaveSchedule adds or replaces a schedule.
func (s *EngineService) SaveSchedule(sc config.Schedule) (config.Schedule, error) {
	return s.eng.SaveSchedule(sc)
}

// DeleteSchedule removes a schedule; its sessions stay.
func (s *EngineService) DeleteSchedule(id string) error { return s.eng.DeleteSchedule(id) }

// RunSchedule runs a schedule now and returns the session it started.
func (s *EngineService) RunSchedule(ctx context.Context, id string) (string, error) {
	return s.eng.RunSchedule(ctx, id)
}

// ActivityReport returns what the agents did, for the analytics view (GUI-021).
func (s *EngineService) ActivityReport(ctx context.Context, days int) (engine.ActivityReport, error) {
	return s.eng.ActivityReport(ctx, days)
}

// AllowRepeated makes a rule from a question the user keeps answering (GUI-021).
func (s *EngineService) AllowRepeated(tool, command, path string) error {
	return s.eng.AllowRepeated(tool, command, path)
}

// UsageReport returns the usage rows the analytics view groups (PRV-008).
func (s *EngineService) UsageReport(ctx context.Context, days int) (engine.UsageReport, error) {
	return s.eng.UsageReport(ctx, days)
}

// Attachment returns one of a session's stored pictures (SES-015).
func (s *EngineService) Attachment(ctx context.Context, sessionID, name string) (engine.Attachment, error) {
	return s.eng.Attachment(ctx, sessionID, name)
}

// ProjectScripts returns a project's worktree scripts (SES-016).
func (s *EngineService) ProjectScripts(ctx context.Context, projectID string) (engine.ProjectScriptsInfo, error) {
	return s.eng.ProjectScripts(ctx, projectID)
}

// SetProjectScripts records the worktree scripts set in the app.
func (s *EngineService) SetProjectScripts(ctx context.Context, projectID string, w config.WorktreeScripts) error {
	return s.eng.SetProjectScripts(ctx, projectID, w)
}

// RetrySetup runs a worktree session's setup again.
func (s *EngineService) RetrySetup(ctx context.Context, sessionID string) error {
	return s.eng.RetrySetup(ctx, sessionID)
}

// SkipSetup lets a session whose setup failed start anyway.
func (s *EngineService) SkipSetup(ctx context.Context, sessionID string) error {
	return s.eng.SkipSetup(ctx, sessionID)
}

// RunWorktreeScript starts the session's run command in a terminal tab.
func (s *EngineService) RunWorktreeScript(ctx context.Context, sessionID string) (engine.TerminalInfo, error) {
	return s.eng.RunWorktreeScript(ctx, sessionID)
}

// PullRequestChecks returns the CI checks of the session's last pull
// request (SES-018).
func (s *EngineService) PullRequestChecks(ctx context.Context, sessionID string) (engine.ChecksInfo, error) {
	return s.eng.PullRequestChecks(ctx, sessionID)
}

// CheckFailures writes a message about the failing checks for the agent.
func (s *EngineService) CheckFailures(ctx context.Context, sessionID string) (string, error) {
	return s.eng.CheckFailures(ctx, sessionID)
}

// Issues lists a project's open GitHub issues (SES-017).
func (s *EngineService) Issues(ctx context.Context, root, search string) ([]engine.Issue, error) {
	return s.eng.Issues(ctx, root, search)
}

// IssueBrief writes one issue as a first message.
func (s *EngineService) IssueBrief(ctx context.Context, root string, number int) (engine.IssueBrief, error) {
	return s.eng.IssueBrief(ctx, root, number)
}

// CreateCompare sends one task to several profiles (HRN-016) and returns
// the tag that groups the runs.
func (s *EngineService) CreateCompare(ctx context.Context, req engine.CompareRequest) (string, error) {
	return s.eng.CreateCompare(ctx, req)
}

// Compare returns a comparison's runs.
func (s *EngineService) Compare(ctx context.Context, tag string) ([]engine.CompareRun, error) {
	return s.eng.Compare(ctx, tag)
}

// KeepCompare keeps one run and archives the others.
func (s *EngineService) KeepCompare(ctx context.Context, tag, sessionID string) error {
	return s.eng.KeepCompare(ctx, tag, sessionID)
}

// SetWebSearchKey keeps the search endpoint's key in the keyring (HRN-017).
func (s *EngineService) SetWebSearchKey(key string) error { return s.eng.SetWebSearchKey(key) }

// RequestReview sends a session's changes to another profile (HRN-018).
func (s *EngineService) RequestReview(ctx context.Context, req engine.ReviewRequest) (session.Session, error) {
	return s.eng.RequestReview(ctx, req)
}

// Reviews lists a session's reviews.
func (s *EngineService) Reviews(ctx context.Context, sessionID string) ([]engine.ReviewInfo, error) {
	return s.eng.Reviews(ctx, sessionID)
}

// ReviewFindings writes a reviewer's reply as a message.
func (s *EngineService) ReviewFindings(ctx context.Context, reviewerID string) (string, error) {
	return s.eng.ReviewFindings(ctx, reviewerID)
}

// PortOpen reports whether something listens on a port of this Mac (GUI-020).
func (s *EngineService) PortOpen(ctx context.Context, port int) bool {
	return s.eng.PortOpen(ctx, port)
}

// Workspace and SaveWorkspace keep the main window's panes (GUI-017).
func (s *EngineService) Workspace() config.Workspace { return s.eng.Workspace() }

func (s *EngineService) SaveWorkspace(w config.Workspace) error { return s.eng.SaveWorkspace(w) }

// OpenSessionWindow opens a session in a window of its own, or brings that
// window forward (GUI-015).
func (s *EngineService) OpenSessionWindow(ctx context.Context, sessionID string) error {
	ses, err := s.eng.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	return s.desk.openWindow(ses.ID, ses.Title)
}

// RedactText removes what the app knows to be secret, before the page stores a draft
// (SES-023).
func (s *EngineService) RedactText(text string) string { return s.eng.Redact(text) }

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

// OpenTerminal, SessionTerminals, TerminalScreen, WriteTerminal,
// ResizeTerminal and CloseTerminal drive the integrated terminal (GUI-012);
// output arrives on TerminalEventName.
func (s *EngineService) OpenTerminal(ctx context.Context, sessionID string, cols, rows int) (engine.TerminalInfo, error) {
	return s.eng.OpenTerminal(ctx, sessionID, cols, rows)
}

func (s *EngineService) SessionTerminals(sessionID string) []engine.TerminalInfo {
	return s.eng.SessionTerminals(sessionID)
}

func (s *EngineService) TerminalScreen(id string) (engine.TerminalOutput, error) {
	return s.eng.TerminalScreen(id)
}

func (s *EngineService) WriteTerminal(id, data string) error {
	return s.eng.WriteTerminal(id, []byte(data))
}

func (s *EngineService) ResizeTerminal(id string, cols, rows int) error {
	return s.eng.ResizeTerminal(id, cols, rows)
}

func (s *EngineService) CloseTerminal(id string) error {
	return s.eng.CloseTerminal(id)
}

// PullRequestInfo, SuggestPullRequest and CreatePullRequest open a pull
// request from the diff panel (SES-011).
func (s *EngineService) PullRequestInfo(ctx context.Context, sessionID string) (engine.PullRequestInfo, error) {
	return s.eng.PullRequestInfo(ctx, sessionID)
}

func (s *EngineService) SuggestPullRequest(ctx context.Context, sessionID string) (engine.PullRequestText, error) {
	return s.eng.SuggestPullRequest(ctx, sessionID)
}

func (s *EngineService) CreatePullRequest(ctx context.Context, req engine.CreatePullRequestRequest) (event.PullRequestData, error) {
	return s.eng.CreatePullRequest(ctx, req)
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

// SearchMCPRegistry returns a page of a server registry (MCP-009).
func (s *EngineService) SearchMCPRegistry(ctx context.Context, query, cursor string) (engine.RegistryPage, error) {
	return s.eng.SearchMCPRegistry(ctx, query, cursor)
}

func (s *EngineService) MCPRegistryAddress() string { return s.eng.MCPRegistryAddress() }

func (s *EngineService) SetMCPRegistryAddress(address string) error {
	return s.eng.SetMCPRegistryAddress(address)
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

// UpdateStatus and CheckForUpdate are the opt-in update check (OQ-015).
func (s *EngineService) UpdateStatus() engine.UpdateInfo { return s.eng.UpdateStatus() }

func (s *EngineService) CheckForUpdate(ctx context.Context) (engine.UpdateInfo, error) {
	return s.eng.CheckForUpdate(ctx)
}

// SignInMCPServer and SignOutMCPServer run and forget a remote server's
// OAuth sign-in (MCP-006).
func (s *EngineService) SignInMCPServer(ctx context.Context, id string) (engine.MCPServerInfo, error) {
	return s.eng.SignInMCPServer(ctx, id)
}

func (s *EngineService) SignOutMCPServer(id string) error {
	return s.eng.SignOutMCPServer(id)
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

// SessionOptions lists what can be changed in a session (PRV-014).
func (s *EngineService) SessionOptions(ctx context.Context, sessionID string) (engine.SessionOptionsReport, error) {
	return s.eng.SessionOptions(ctx, sessionID)
}

// SetSessionOption changes one of them.
func (s *EngineService) SetSessionOption(ctx context.Context, sessionID, id, value string) error {
	return s.eng.SetSessionOption(ctx, sessionID, id, value)
}

// LoadSessionOptions starts a session's agent, so it can say what it offers.
func (s *EngineService) LoadSessionOptions(ctx context.Context, sessionID string) (engine.SessionOptionsReport, error) {
	return s.eng.LoadSessionOptions(ctx, sessionID)
}

// OpenProjectFolder shows the session's folder in the file manager (GUI-024).
func (s *EngineService) OpenProjectFolder(ctx context.Context, sessionID string) error {
	return s.eng.OpenProjectFolder(ctx, sessionID)
}

// AddSessionRule adds a rule that lasts as long as the session (SEC-002).
func (s *EngineService) AddSessionRule(sessionID string, r permission.Rule) error {
	return s.eng.AddSessionRule(sessionID, r)
}

// DeleteSessionRule removes one of them.
func (s *EngineService) DeleteSessionRule(sessionID string, r permission.Rule) error {
	return s.eng.DeleteSessionRule(sessionID, r)
}

// SetModel switches a built-in session's model (PRV-006).
func (s *EngineService) SetModel(ctx context.Context, sessionID, providerID, model string) error {
	return s.eng.SetModel(ctx, sessionID, providerID, model)
}

// Limits returns the runs of each account limit agents have reported (GUI-028).
func (s *EngineService) Limits(ctx context.Context) ([]engine.LimitPeriod, error) {
	return s.eng.Limits(ctx)
}

// RecentUsage returns the last hours of usage, for the live graphs (GUI-023).
func (s *EngineService) RecentUsage(ctx context.Context, hours int) (engine.RecentUsageReport, error) {
	return s.eng.RecentUsage(ctx, hours)
}

// ShowLogs shows the app's log file in the file manager, for a bug report (NFR-PRIV-001).
func (s *EngineService) ShowLogs() error {
	return s.app.Env.OpenFileManager(s.logPath, true)
}

// ProblemReport is the text "Copy a report" puts on the clipboard: the version,
// this Mac, the session's agent and provider, and the log's end, redacted. It is
// sent nowhere.
func (s *EngineService) ProblemReport(ctx context.Context, sessionID string) (string, error) {
	return s.eng.ProblemReport(ctx, sessionID)
}

// Optimizers lists the token optimizers a session can use (HRN-019).
func (s *EngineService) Optimizers(ctx context.Context, sessionID string) ([]engine.OptimizerInfo, error) {
	return s.eng.Optimizers(ctx, sessionID)
}

// ListOptimizers, CheckOptimizer and SetDefaultOptimizer back the Settings page
// for token optimizers (HRN-019).
func (s *EngineService) ListOptimizers(ctx context.Context) []engine.OptimizerStatus {
	return s.eng.ListOptimizers(ctx)
}

func (s *EngineService) CheckOptimizer(ctx context.Context, id string) (engine.OptimizerCheck, error) {
	return s.eng.CheckOptimizer(ctx, id)
}

// ApproveOptimizer approves an optimizer file as the Settings page showed it. It
// is here and not on the socket: the user approves, not another process.
func (s *EngineService) ApproveOptimizer(ctx context.Context, id, fingerprint string) (config.Config, error) {
	return s.eng.ApproveOptimizer(ctx, id, fingerprint)
}

// PendingCommands lists the hooks and agent presets from files that wait for
// approval.
func (s *EngineService) PendingCommands() []engine.PendingCommand {
	return s.eng.PendingCommands()
}

// ApproveCommand approves a hook or preset as the Settings page showed it. It is
// here and not on the socket: the user approves, not another process.
func (s *EngineService) ApproveCommand(fingerprint string) (config.Config, error) {
	return s.eng.ApproveCommand(fingerprint)
}

// ApproveCommands approves all the Settings page showed, in one go. It is not on
// the socket either.
func (s *EngineService) ApproveCommands(fingerprints []string) (config.Config, error) {
	return s.eng.ApproveCommands(fingerprints)
}

func (s *EngineService) SetDefaultOptimizer(ctx context.Context, id string) (config.Config, error) {
	return s.eng.SetDefaultOptimizer(ctx, id)
}

// AgentSessions lists an external agent's own sessions (SES-020).
func (s *EngineService) AgentSessions(ctx context.Context, profileID, folder string) ([]engine.AgentSession, error) {
	return s.eng.AgentSessions(ctx, profileID, folder)
}

// ImportAgentSession imports one of an external agent's sessions.
func (s *EngineService) ImportAgentSession(ctx context.Context, req engine.ImportSessionRequest) (session.Session, error) {
	return s.eng.ImportAgentSession(ctx, req)
}

// InstallOptimizer downloads a token optimizer's program into the app's own folder,
// on the user's asking (HRN-019).
func (s *EngineService) InstallOptimizer(ctx context.Context, id string) (engine.OptimizerStatus, error) {
	return s.eng.InstallOptimizer(ctx, id)
}

// RemoveOptimizer deletes what InstallOptimizer downloaded.
func (s *EngineService) RemoveOptimizer(ctx context.Context, id string) error {
	return s.eng.RemoveOptimizer(ctx, id)
}

// TypeOptimizerInstall types a token optimizer's install command into a new
// terminal in the session, for the user to run (HRN-019).
func (s *EngineService) TypeOptimizerInstall(ctx context.Context, sessionID, id string) (engine.TerminalInfo, error) {
	return s.eng.TypeOptimizerInstall(ctx, sessionID, id)
}

// ListWorkflows, ApproveWorkflow, InstallWorkflowSkills and SetDefaultWorkflow
// back the Workflows page in Settings (WFL-001, WFL-003).
func (s *EngineService) ListWorkflows(ctx context.Context) []engine.WorkflowStatus {
	return s.eng.ListWorkflows(ctx)
}

func (s *EngineService) ApproveWorkflow(ctx context.Context, id, fingerprint string) (config.Config, error) {
	return s.eng.ApproveWorkflow(ctx, id, fingerprint)
}

func (s *EngineService) InstallWorkflowSkills(ctx context.Context, id string) ([]string, error) {
	return s.eng.InstallWorkflowSkills(ctx, id)
}

func (s *EngineService) SetDefaultWorkflow(ctx context.Context, id string) (config.Config, error) {
	return s.eng.SetDefaultWorkflow(ctx, id)
}

// Workflows lists the workflows a session can choose, SetWorkflow chooses one and
// RunStage invokes one of its stages (WFL-002).
func (s *EngineService) Workflows(ctx context.Context) []engine.WorkflowInfo {
	return s.eng.Workflows(ctx)
}

func (s *EngineService) SetWorkflow(ctx context.Context, sessionID, workflowID string) error {
	return s.eng.SetWorkflow(ctx, sessionID, workflowID)
}

// Backlogs lists trusted projects' backlogs for the Board, Backlog reads one and
// ArchiveBacklog moves what is finished out of it (WFL-004).
func (s *EngineService) Backlogs(ctx context.Context) ([]engine.BacklogInfo, error) {
	return s.eng.Backlogs(ctx)
}

func (s *EngineService) Backlog(ctx context.Context, root, workflowID string) (engine.BacklogInfo, error) {
	return s.eng.Backlog(ctx, root, workflowID)
}

func (s *EngineService) ArchiveBacklog(ctx context.Context, root, workflowID string) (engine.ArchiveResult, error) {
	return s.eng.ArchiveBacklog(ctx, root, workflowID)
}

// PrepareAFK says what an AFK run would do, StartAFK starts one, and the rest
// follow, pause, stop and QA them (WFL-005, WFL-006).
func (s *EngineService) PrepareAFK(ctx context.Context, root, workflowID string) (engine.AFKPlan, error) {
	return s.eng.PrepareAFK(ctx, root, workflowID)
}

func (s *EngineService) StartAFK(ctx context.Context, req engine.AFKStart) (config.AFKRun, error) {
	return s.eng.StartAFK(ctx, req)
}

func (s *EngineService) AFKRuns(ctx context.Context) ([]config.AFKRun, error) {
	return s.eng.AFKRuns(ctx)
}

func (s *EngineService) PauseAFK(ctx context.Context, id string, paused bool) (config.AFKRun, error) {
	return s.eng.PauseAFK(ctx, id, paused)
}

func (s *EngineService) StopAFK(ctx context.Context, id string) (config.AFKRun, error) {
	return s.eng.StopAFK(ctx, id)
}

func (s *EngineService) QADone(ctx context.Context, runID, issue string) error {
	return s.eng.QADone(ctx, runID, issue)
}

func (s *EngineService) RunStage(ctx context.Context, sessionID, command, input string) (engine.StageResult, error) {
	return s.eng.RunStage(ctx, sessionID, command, input)
}

// SetOptimizer switches a session's token optimizer (HRN-019).
// OptimizerSavings says what a session's optimizer reports having saved, or nil.
func (s *EngineService) OptimizerSavings(ctx context.Context, sessionID string) (*engine.OptimizerSavings, error) {
	return s.eng.CheckOptimizerSavings(ctx, sessionID)
}

// Queue returns the queue of tasks (SES-025).
func (s *EngineService) Queue() (config.Queue, error) { return s.eng.Queue() }

// AddTask puts a task at the end of the queue.
func (s *EngineService) AddTask(t config.Task) (config.Queue, error) { return s.eng.AddTask(t) }

// MoveTask moves a task by places in the queue.
func (s *EngineService) MoveTask(id string, by int) (config.Queue, error) {
	return s.eng.MoveTask(id, by)
}

// RemoveTask takes a task off the queue.
func (s *EngineService) RemoveTask(id string) (config.Queue, error) { return s.eng.RemoveTask(id) }

// SetQueue pauses or resumes the queue and sets how many tasks run at once.
func (s *EngineService) SetQueue(paused bool, limit int) (config.Queue, error) {
	return s.eng.SetQueue(paused, limit)
}

// HandOff continues a session on another profile in one step (HRN-020).
func (s *EngineService) HandOff(ctx context.Context, sessionID, profileID, reason string) (engine.HandOffResult, error) {
	return s.eng.HandOff(ctx, sessionID, profileID, reason, false)
}

func (s *EngineService) SetOptimizer(ctx context.Context, sessionID, optimizerID string) error {
	return s.eng.SetOptimizer(ctx, sessionID, optimizerID)
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
func (s *EngineService) MergeWorktree(ctx context.Context, sessionID string, confirmed bool) (engine.MergeResult, error) {
	return s.eng.MergeWorktree(ctx, sessionID, confirmed)
}

// MergeRisks lists the files a merge would bring in that can run commands.
func (s *EngineService) MergeRisks(ctx context.Context, sessionID string) ([]string, error) {
	return s.eng.MergeRisks(ctx, sessionID)
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

// SetContextLimit records a model's context limit, 0 to go back to what is detected.
func (s *EngineService) SetContextLimit(ctx context.Context, providerID, model string, limit int) error {
	return s.eng.SetContextLimit(ctx, providerID, model, limit)
}

func (s *EngineService) SetModelFlags(providerID, model string, favorite, hidden bool) error {
	return s.eng.SetModelFlags(providerID, model, favorite, hidden)
}

// ProjectFiles lists a session's project files for the @ picker.
func (s *EngineService) ProjectFiles(ctx context.Context, sessionID string) ([]string, error) {
	return s.eng.ProjectFiles(ctx, sessionID)
}

// Budgets returns every budget with what has been spent this period (PRV-017).
func (s *EngineService) Budgets(ctx context.Context) ([]engine.BudgetState, error) {
	return s.eng.Budgets(ctx)
}

// CheckPolicy says what the broker would do with a call, and why (SEC-018).
func (s *EngineService) CheckPolicy(ctx context.Context, q engine.PolicyQuery) (engine.PolicyCheck, error) {
	return s.eng.CheckPolicy(ctx, q)
}

// PolicyOverview lists what decides a call in a session or a project (SEC-018).
func (s *EngineService) PolicyOverview(ctx context.Context, q engine.PolicyQuery) (engine.PolicyOverview, error) {
	return s.eng.PolicyOverview(ctx, q)
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
	err := s.eng.DeleteSession(ctx, sessionID)
	// The session's own window, and its subagents', go with it.
	s.desk.closeGone(func(sid string) bool {
		_, err := s.eng.GetSession(context.Background(), sid)
		return err == nil
	})
	return err
}

// StartDemo opens the in-app demo (GUI-045): the existing one, or a new one that
// starts its scripted turn at once.
func (s *EngineService) StartDemo(ctx context.Context) (session.Session, error) {
	return s.eng.StartDemo(ctx)
}

// RemoveDemo deletes the demo session, its scratch project and the project record.
func (s *EngineService) RemoveDemo(ctx context.Context, sessionID string) error {
	err := s.eng.RemoveDemo(ctx, sessionID)
	s.desk.closeGone(func(sid string) bool {
		_, err := s.eng.GetSession(context.Background(), sid)
		return err == nil
	})
	return err
}

// BuiltinPrices are the vendors' list prices, after the user's own (PRV-008).
func (s *EngineService) BuiltinPrices() BuiltinPriceList {
	return BuiltinPriceList{AsOf: config.PricesAsOf, Prices: config.BuiltinPrices()}
}

// BuiltinPriceList is the built-in prices and the day they were copied.
type BuiltinPriceList struct {
	AsOf   string         `json:"asOf"`
	Prices []config.Price `json:"prices"`
}

func (s *EngineService) Config() config.Config {
	return s.eng.Config()
}

// ConfirmProviderAddress tells the engine a provider's changed address is the
// one its key is for.
func (s *EngineService) ConfirmProviderAddress(id string) error {
	return s.eng.ConfirmProviderAddress(id)
}

func (s *EngineService) Colors() config.Colors {
	return s.eng.Colors()
}

func (s *EngineService) SaveColors(c config.Colors) (config.Colors, error) {
	return s.eng.SaveColors(c)
}

// OpenColors opens the colors file in the app the user opens JSON with
// (GUI-022).
func (s *EngineService) OpenColors() error {
	path, err := s.eng.ColorsFile()
	if err != nil {
		return err
	}
	return s.eng.OpenInEditor(path)
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

// ExportUsage asks where to save the last days days of usage as CSV
// (PRV-008) and writes it; it returns the path, or "" when the user cancels.
func (s *EngineService) ExportUsage(ctx context.Context, days int) (string, error) {
	path, err := s.app.Dialog.SaveFile().
		SetMessage("Export token usage").
		SetFilename("freeloop-usage.csv").
		CanCreateDirectories(true).
		PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	return path, s.eng.ExportUsage(ctx, days, path)
}

// SaveUsagePicture asks where to save the cost summary's picture (GUI-029), which comes
// as base64 PNG, and writes it. It returns "" if the user cancels.
func (s *EngineService) SaveUsagePicture(ctx context.Context, data string) (string, error) {
	path, err := s.app.Dialog.SaveFile().
		SetMessage("Save the usage summary").
		SetFilename("freeloop-usage.png").
		CanCreateDirectories(true).
		PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	return path, s.eng.SaveUsagePicture(ctx, path, data)
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

// PreviewConfigRestore asks for a config backup and says what restoring it would
// do, changing nothing; nil when the user cancels. In a served build, which has no
// dialogs, FREELOOP_DEV_BACKUP names the file.
func (s *EngineService) PreviewConfigRestore(ctx context.Context) (*engine.ConfigPreview, error) {
	path := ""
	if servedBuild {
		path = devEnv("FREELOOP_DEV_BACKUP")
	} else {
		var err error
		path, err = s.app.Dialog.OpenFile().
			SetTitle("Restore Freeloop's configuration").
			CanChooseFiles(true).
			CanChooseDirectories(false).
			AddFilter("Freeloop config backup", "*.json").
			PromptForSingleSelection()
		if err != nil {
			return nil, err
		}
	}
	if path == "" {
		return nil, nil
	}
	preview, err := s.eng.PreviewConfig(ctx, path)
	if err != nil {
		return nil, err
	}
	s.restoring.Store(&path)
	return &preview, nil
}

// StartWaiting says what a profile's new sessions start without until approved.
func (s *EngineService) StartWaiting(profileID string) engine.StartWaits {
	return s.eng.StartWaiting(profileID)
}

// RestoreConfig restores the backup last previewed, if it is still the same file.
func (s *EngineService) RestoreConfig(ctx context.Context, fingerprint string) (*engine.ConfigRestore, error) {
	path := s.restoring.Load()
	if path == nil {
		return nil, errors.New("choose a backup first")
	}
	res, err := s.eng.RestorePreviewed(ctx, *path, fingerprint)
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
	return s.eng.OpenInEditor(path)
}

func (s *EngineService) ListFolder(ctx context.Context, sessionID, rel string) ([]engine.FileEntry, error) {
	return s.eng.ListFolder(ctx, sessionID, rel)
}

// ViewPDF returns a PDF of the session's folder for the viewer (GUI-035).
func (s *EngineService) ViewPDF(ctx context.Context, sessionID, rel string) (engine.PDFFile, error) {
	return s.eng.ViewPDF(ctx, sessionID, rel)
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
	return s.eng.OpenInEditor(path)
}

// OpenPDF shows a PDF of the session's folder in Preview (GUI-035).
func (s *EngineService) OpenPDF(ctx context.Context, sessionID, rel string) error {
	path, err := s.eng.FileOnHost(ctx, sessionID, rel)
	if err != nil {
		return err
	}
	return s.eng.OpenPDF(path)
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

func (s *EngineService) ScaffoldHarness(ctx context.Context, req engine.ScaffoldRequest) (engine.ScaffoldResult, error) {
	return s.eng.ScaffoldHarness(ctx, req)
}

// OpenHarness opens a custom harness's agent file in the user's editor
// (HRN-011).
func (s *EngineService) OpenHarness(id string) error {
	path := s.eng.HarnessEntry(id)
	if path == "" {
		return fmt.Errorf("the harness %s has no source folder", id)
	}
	return s.eng.OpenInEditor(path)
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
