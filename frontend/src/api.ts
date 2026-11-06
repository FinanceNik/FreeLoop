// One import point for the generated bindings.
export { EngineService } from "../bindings/github.com/FinanceNik/Freeloop/cmd/freeloop";
export type { Budget, Colors, Command, Config, Defaults, Hook, Issue, MCPServer, Price, Profile, Schedule } from "../bindings/github.com/FinanceNik/Freeloop/internal/config/models";
export type { AgentSession, BudgetState, SessionOptionsReport, ActivityReport, ActivityRow, RegistryOption, RegistryPage, RegistryServer, AttentionItem, ChangedFile, Changes, ChecksInfo, CompareRun, ClaudeImport, ConfigPreview, ConfigRestore, DayCost, FileEntry, FileView, ImportItem, EnvironmentInfo, FileDiff, HarnessInfo, Issue as GitHubIssue, IssueBrief, MCPServerInfo, OptimizerCheck, OptimizerInfo, OptimizerStatus, PendingCommand, PolicyCheck, PolicyHook, PolicyOverview, PolicyQuery, PolicyScope, PresetInfo, ProjectInfo, ProjectSettings, ProviderInfo, PullRequestInfo, PullRequestText, ProjectScriptsInfo, ResourceRef, ReviewInfo, ScheduleInfo, UsageReport, UsageRow, TerminalInfo, TerminalOutput, UpdateInfo, SkillTest, SessionHits, SessionMCPContext, SkillCandidate, SkillInfo, SkillList, StartWaits } from "../bindings/github.com/FinanceNik/Freeloop/internal/engine/models";
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

export const MODES = [
  { id: "ask", name: "Ask", hint: "Asks before anything that changes files or runs a command, unless a rule decides." },
  { id: "accept_edits", name: "Accept edits", hint: "File edits in the project go ahead; commands still ask." },
  { id: "plan", name: "Plan", hint: "Only reads, and proposes a plan for you to approve." },
  { id: "auto", name: "Auto", hint: "The agent decides by itself what to run, for agents that have such a mode. What it does not ask about never reaches Freeloop." },
  // Short, as the other modes, so it fits the composer's control; what it means is in the hint.
  { id: "bypass", name: "Bypass", hint: "Skips permission prompts: nothing asks, except protected paths and deny rules." },
];

// A session never starts in these: the user chooses them in the session (SEC-001).
export const STARTING_MODES = MODES.filter((m) => m.id !== "bypass" && m.id !== "auto");

export function errorText(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === "object" && err && "message" in err) return String((err as { message: unknown }).message);
  return String(err);
}
