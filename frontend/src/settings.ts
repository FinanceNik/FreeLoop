import { t, type PlainKey } from "./i18n.ts";
import { pressed, type Keys } from "./shortcuts.ts";

// The settings area: the configuration pages, grouped, behind one sidebar
// entry. The pages share a column listing them, like a system settings
// window; the sidebar keeps its room for sessions.

// The app's own settings, once one long General page, are pages of their own.
export type GeneralPage = "appearance" | "keyboard" | "sessions" | "costs" | "permissions" | "network" | "data";

export type SettingsPage = "providers" | "profiles" | "skills" | "mcp" | "optimizers" | "workflows" | "rules" | "environments" | "schedules" | GeneralPage;

// The group's title and each page's label are catalogue keys (GUI-048).
export interface SettingsGroup {
  title: PlainKey;
  pages: { id: SettingsPage; label: PlainKey; icon: string }[];
}

export const SETTINGS_GROUPS: SettingsGroup[] = [
  {
    title: "settings.group.models",
    pages: [
      { id: "providers", label: "settings.page.providers", icon: "providers" },
      { id: "profiles", label: "settings.page.profiles", icon: "profiles" },
    ],
  },
  {
    title: "settings.group.agents",
    pages: [
      { id: "skills", label: "settings.page.skills", icon: "skills" },
      { id: "mcp", label: "settings.page.mcp", icon: "mcp" },
      { id: "optimizers", label: "settings.page.optimizers", icon: "optimizers" },
      { id: "workflows", label: "settings.page.workflows", icon: "workflows" },
      { id: "rules", label: "settings.page.rules", icon: "rules" },
    ],
  },
  {
    title: "settings.group.workspace",
    pages: [
      { id: "environments", label: "settings.page.environments", icon: "environments" },
      { id: "schedules", label: "settings.page.schedules", icon: "schedules" },
    ],
  },
  {
    title: "settings.group.app",
    pages: [
      { id: "appearance", label: "settings.page.appearance", icon: "appearance" },
      { id: "keyboard", label: "settings.page.keyboard", icon: "keyboard" },
      { id: "sessions", label: "settings.page.sessions", icon: "sessions" },
      { id: "costs", label: "settings.page.costs", icon: "costs" },
      { id: "permissions", label: "settings.page.permissions", icon: "permissions" },
      { id: "network", label: "settings.page.network", icon: "network" },
      { id: "data", label: "settings.page.data", icon: "data" },
    ],
  },
];

// pageTitle is a page's name in the language in force.
export function pageTitle(page: string): string {
  const p = SETTINGS_GROUPS.flatMap((g) => g.pages).find((x) => x.id === page);
  return t(p ? p.label : "settings.title");
}

export const GENERAL_PAGES = SETTINGS_GROUPS.at(-1)!.pages.map((p) => p.id) as GeneralPage[];

export function isGeneral(kind: string): kind is GeneralPage {
  return (GENERAL_PAGES as string[]).includes(kind);
}

// The page the settings open on: the last one shown, or the first.
export function settingsStart(last: string | null): SettingsPage {
  return last && AREA.has(last) && last !== "import" ? (last as SettingsPage) : "appearance";
}

// The import page opens from Settings and MCP, so it belongs to the area
// without an entry of its own.
const AREA = new Set<string>([...SETTINGS_GROUPS.flatMap((g) => g.pages.map((p) => p.id)), "import"]);

export function inSettings(kind: string): boolean {
  return AREA.has(kind);
}

// ⌘, opens the settings, as in every Mac app; Ctrl+, elsewhere, as in VS Code.
export function isSettingsShortcut(k: Keys): boolean {
  return pressed(k, "settings");
}

// The Font size setting's range; config.go holds the same one.
export const MIN_FONT_SIZE = 11;
export const MAX_FONT_SIZE = 20;

// textSizeAfter is the Font size that ⌘+ or ⌘- (Ctrl outside a Mac) asks for, or
// null for any other key (NFR-A11Y-002). ⌘+ is ⌘= without Shift, on most keyboards.
export function textSizeAfter(k: Keys, current: number): number | null {
  if (pressed(k, "textLarger")) return Math.min(current + 1, MAX_FONT_SIZE);
  if (pressed(k, "textSmaller")) return Math.max(current - 1, MIN_FONT_SIZE);
  return null;
}

// settingsKey compares two configs as the General page edits them. Approvals are not
// edits: the engine keeps those itself.
export function settingsKey<T extends { commandApprovals?: unknown }>(c: T): string {
  const { commandApprovals: _approvals, ...rest } = c;
  return JSON.stringify(rest);
}

// autoSaved is what the General page saves by itself: everything but hooks, prices,
// budgets and the web search settings. Saving a hook approves a command to run, a
// half-typed address would take its key along, and a price or budget row is unfinished
// while being typed, so those wait for the Save button.
export function autoSaved<T extends { hooks?: unknown; prices?: unknown; budgets?: unknown; web?: unknown }>(form: T, saved: T): T {
  return { ...form, hooks: saved.hooks, prices: saved.prices, budgets: saved.budgets, web: saved.web };
}
