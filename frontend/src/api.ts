// One import point for the generated bindings.
export { EngineService } from "../bindings/github.com/FinanceNik/Freeloop/cmd/freeloop";
export type { Budget, Colors, Command, Config, Defaults, Hook, Issue, MCPServer, AFKRun, AFKStep, Price, Profile, Schedule, ScheduleAFK, WorkflowStage } from "../bindings/github.com/FinanceNik/Freeloop/internal/config/models";
export type { AFKCommand, AFKPlan, ProfileModels, QAItem, AgentSession, BudgetState, SessionOptionsReport, ActivityReport, ActivityRow, RegistryOption, RegistryPage, RegistryServer, AttentionItem, ArchiveResult, BacklogInfo, ChangedFile, Changes, ChecksInfo, CompareRun, ClaudeImport, ConfigPreview, ConfigRestore, DayCost, FileEntry, FileView, ImportItem, EnvironmentInfo, FileDiff, HarnessInfo, Issue as GitHubIssue, IssueBrief, MCPServerInfo, OptimizerCheck, OptimizerInfo, OptimizerStatus, PendingCommand, PolicyCheck, PolicyHook, PolicyOverview, PolicyQuery, PolicyScope, PresetInfo, ProjectInfo, ProjectSettings, ProviderInfo, PullRequestInfo, PullRequestText, ProjectScriptsInfo, ResourceRef, ReviewInfo, ScheduleInfo, UsageReport, UsageRow, TerminalInfo, TerminalOutput, UpdateInfo, SkillTest, SessionHits, SessionMCPContext, SkillCandidate, SkillInfo, SkillList, StageResult, StartWaits, WorkflowInfo, WorkflowSkill, WorkflowStatus } from "../bindings/github.com/FinanceNik/Freeloop/internal/engine/models";
export type { Request as PermissionRequest, Rule } from "../bindings/github.com/FinanceNik/Freeloop/internal/permission/models";
export type { ModelCaps, ModelInfo } from "../bindings/github.com/FinanceNik/Freeloop/internal/provider/models";
export type { Project, Session } from "../bindings/github.com/FinanceNik/Freeloop/internal/session/models";
export type { Option as SessionOption } from "../bindings/github.com/FinanceNik/Freeloop/internal/harness/models";
export type { Blocked } from "../bindings/github.com/FinanceNik/Freeloop/internal/netguard/models";

// Must match EventName, ConfigEventName, OpenSessionEventName,
// FilesDroppedEventName and TerminalEventName in
// cmd/freeloop/main.go.
export const EVENT_NAME = "freeloop:event";
export const CONFIG_EVENT_NAME = "freeloop:config";
export const OPEN_SESSION_EVENT_NAME = "freeloop:open-session";
export const FILES_DROPPED_EVENT_NAME = "freeloop:files-dropped";
export const TERMINAL_EVENT_NAME = "freeloop:terminal";

// The permission modes are the app's own and are named in the interface's language;
// they live with the slash commands that choose them.
export { MODES, STARTING_MODES, modeHint, modeName } from "./commands.ts";

export function errorText(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === "object" && err && "message" in err) return String((err as { message: unknown }).message);
  return String(err);
}
