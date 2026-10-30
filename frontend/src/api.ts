// One import point for the generated bindings.
export { EngineService } from "../bindings/github.com/FinanceNik/Freeloop/cmd/freeloop";
export type { Command, Config, Hook, Issue, MCPServer, Price, Profile } from "../bindings/github.com/FinanceNik/Freeloop/internal/config/models";
export type { AttentionItem, ChangedFile, Changes, ClaudeImport, ConfigRestore, DayCost, FileEntry, FileView, ImportItem, EnvironmentInfo, FileDiff, HarnessInfo, MCPServerInfo, PresetInfo, ProjectInfo, ProjectSettings, ProviderInfo, PullRequestInfo, PullRequestText, ResourceRef, TerminalInfo, TerminalOutput, SkillTest, SessionHits, SessionMCPContext, SkillCandidate, SkillInfo, SkillList } from "../bindings/github.com/FinanceNik/Freeloop/internal/engine/models";
export type { Request as PermissionRequest, Rule } from "../bindings/github.com/FinanceNik/Freeloop/internal/permission/models";
export type { ModelCaps, ModelInfo } from "../bindings/github.com/FinanceNik/Freeloop/internal/provider/models";
export type { Project, Session } from "../bindings/github.com/FinanceNik/Freeloop/internal/session/models";
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
  { id: "ask", name: "Ask" },
  { id: "accept_edits", name: "Accept edits" },
  { id: "plan", name: "Plan" },
  { id: "bypass", name: "Bypass" },
];

export function errorText(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === "object" && err && "message" in err) return String((err as { message: unknown }).message);
  return String(err);
}
