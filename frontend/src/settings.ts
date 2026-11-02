import { pressed, type Keys } from "./shortcuts.ts";

// The settings area: the configuration pages, grouped, behind one sidebar
// entry. The pages share a column listing them, like a macOS settings
// window; the sidebar keeps its room for sessions.

export type SettingsPage = "providers" | "profiles" | "skills" | "mcp" | "optimizers" | "rules" | "environments" | "schedules" | "settings";

export interface SettingsGroup {
  title: string;
  pages: { id: SettingsPage; label: string; icon: string }[];
}

export const SETTINGS_GROUPS: SettingsGroup[] = [
  {
    title: "Models",
    pages: [
      { id: "providers", label: "Providers", icon: "providers" },
      { id: "profiles", label: "Profiles", icon: "profiles" },
    ],
  },
  {
    title: "Agents",
    pages: [
      { id: "skills", label: "Skills", icon: "skills" },
      { id: "mcp", label: "MCP servers", icon: "mcp" },
      { id: "optimizers", label: "Token optimizers", icon: "optimizers" },
      { id: "rules", label: "Permission rules", icon: "rules" },
    ],
  },
  {
    title: "Workspace",
    pages: [
      { id: "environments", label: "Environments", icon: "environments" },
      { id: "schedules", label: "Schedules", icon: "schedules" },
    ],
  },
  {
    title: "App",
    pages: [{ id: "settings", label: "General", icon: "general" }],
  },
];

// The import page opens from Settings and MCP, so it belongs to the area
// without an entry of its own.
const AREA = new Set<string>([...SETTINGS_GROUPS.flatMap((g) => g.pages.map((p) => p.id)), "import"]);

export function inSettings(kind: string): boolean {
  return AREA.has(kind);
}

// ⌘, opens the settings, as in every Mac app.
export function isSettingsShortcut(k: Keys): boolean {
  return pressed(k, "settings");
}

// The Font size setting's range; config.go holds the same one.
export const MIN_FONT_SIZE = 11;
export const MAX_FONT_SIZE = 20;

// textSizeAfter is the Font size that ⌘+ or ⌘- asks for, or null for any other
// key (NFR-A11Y-002). ⌘+ is ⌘= without Shift, on most keyboards.
export function textSizeAfter(k: Keys, current: number): number | null {
  if (pressed(k, "textLarger")) return Math.min(current + 1, MAX_FONT_SIZE);
  if (pressed(k, "textSmaller")) return Math.max(current - 1, MIN_FONT_SIZE);
  return null;
}
