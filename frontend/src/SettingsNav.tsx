import { NavIcon as Icon } from "./icons.tsx";
import { SETTINGS_GROUPS, type SettingsPage } from "./settings.ts";

// The column of the settings area: its pages in groups, with a way back to
// where the user was.

interface Props {
  current: string;
  onOpen: (page: SettingsPage) => void;
  onBack: () => void;
  // A number to show beside a page, such as approvals waiting on Permissions.
  counts?: Partial<Record<SettingsPage, number>>;
}

export default function SettingsNav({ current, onOpen, onBack, counts = {} }: Props) {
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
                  {(counts[p.id] ?? 0) > 0 && (
                    <span className="badge settings-count" aria-label={`, ${counts[p.id]} waiting`}>
                      {counts[p.id]}
                    </span>
                  )}
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
