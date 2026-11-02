import { SETTINGS_GROUPS, type SettingsPage } from "./settings.ts";

// The column of the settings area: its pages in groups, with a way back to
// where the user was.

interface Props {
  current: string;
  onOpen: (page: SettingsPage) => void;
  onBack: () => void;
}

export default function SettingsNav({ current, onOpen, onBack }: Props) {
  return (
    <nav className="settings-nav" aria-label="Settings">
      <button className="settings-back" onClick={onBack}>
        <Icon name="back" />
        Back
      </button>
      {SETTINGS_GROUPS.map((g) => (
        <section key={g.title}>
          <h2>{g.title}</h2>
          <ul>
            {g.pages.map((p) => (
              <li key={p.id}>
                <button aria-current={current === p.id ? "page" : undefined} onClick={() => onOpen(p.id)}>
                  <Icon name={p.icon} />
                  {p.label}
                </button>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </nav>
  );
}

// Line icons on a 16-point grid, drawn in the text colour.
const PATHS: Record<string, string> = {
  back: "M10 3.5 5.5 8l4.5 4.5",
  "panes-1": "M2.5 3.5h11v9h-11z",
  "panes-2": "M2.5 3.5h11v9h-11zM8 3.5v9",
  "panes-3": "M2.5 3.5h11v9h-11zM8 3.5v9M8 8h5.5",
  "panes-4": "M2.5 3.5h11v9h-11zM8 3.5v9M2.5 8h11",
  providers: "M8 2.5 13 5.25v5.5L8 13.5 3 10.75v-5.5L8 2.5ZM3 5.25 8 8l5-2.75M8 8v5.5",
  profiles: "M3 4h10M3 8h10M3 12h6",
  skills: "M8 2.5l1.6 3.6 3.9.4-2.9 2.6.8 3.9L8 11l-3.4 2 .8-3.9-2.9-2.6 3.9-.4L8 2.5Z",
  mcp: "M6 2.5v3M10 2.5v3M4.5 5.5h7v2.5a3.5 3.5 0 0 1-7 0V5.5ZM8 11.5v2",
  optimizers: "M2.5 4h11l-4.2 4.6v4.1l-2.6-1.3V8.6L2.5 4Z",
  rules: "M8 2.5 13 4.5v3.5c0 3-2.2 5-5 6-2.8-1-5-3-5-6V4.5l5-2Z",
  environments: "M2.5 4.5a1.5 1.5 0 0 1 1.5-1.5h8a1.5 1.5 0 0 1 1.5 1.5v7a1.5 1.5 0 0 1-1.5 1.5H4a1.5 1.5 0 0 1-1.5-1.5v-7ZM5 6.5 7 8l-2 1.5M8.5 10H11",
  schedules: "M8 2.5a5.5 5.5 0 1 0 0 11 5.5 5.5 0 0 0 0-11ZM8 5v3l2 1.5",
  general:
    "M6.9 2.5h2.2l.3 1.6 1.2.7 1.5-.6 1.1 1.9-1.2 1v1.4l1.2 1-1.1 1.9-1.5-.6-1.2.7-.3 1.6H6.9l-.3-1.6-1.2-.7-1.5.6-1.1-1.9 1.2-1V7.1l-1.2-1 1.1-1.9 1.5.6 1.2-.7.3-1.6ZM8 6.2a1.8 1.8 0 1 0 0 3.6 1.8 1.8 0 0 0 0-3.6Z",
};

export function Icon({ name }: { name: string }) {
  return (
    <svg className="icon" width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" focusable="false">
      <path d={PATHS[name]} fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
