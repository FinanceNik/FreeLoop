// The settings area: the configuration pages, grouped, behind one sidebar
// entry. The pages share a column listing them, like a macOS settings
// window; the sidebar keeps its room for sessions.

export type SettingsPage = "providers" | "profiles" | "skills" | "mcp" | "rules" | "environments" | "settings";

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
      { id: "rules", label: "Permission rules", icon: "rules" },
    ],
  },
  {
    title: "Workspace",
    pages: [{ id: "environments", label: "Environments", icon: "environments" }],
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
export function isSettingsShortcut(k: { key: string; metaKey: boolean; ctrlKey: boolean; altKey: boolean; shiftKey: boolean }): boolean {
  return k.metaKey && !k.ctrlKey && !k.altKey && !k.shiftKey && k.key === ",";
}
