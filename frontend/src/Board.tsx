import { columns, since } from "./board.ts";

type Card = { id: string; title: string; status: string; updatedAt: string | Date; projectRoot: string; projectId: string; branch?: string; tags?: string[] | null };

// Board shows one card per session in columns by status (GUI-016), an
// alternative to the sidebar's list. It shows the sessions the sidebar's
// filters let through.
export default function Board<T extends Card>({
  sessions,
  projectName,
  statusLabel,
  filtered,
  onOpen,
}: {
  sessions: T[];
  projectName: (s: T) => string;
  statusLabel: Record<string, string>;
  filtered: boolean;
  onOpen: (id: string) => void;
}) {
  const cols = columns(sessions);
  return (
    <div className="board-page">
      <header className="board-header">
        <h2>Board</h2>
        <span className="muted">
          {sessions.length} {sessions.length === 1 ? "session" : "sessions"}
          {filtered ? ", filtered in the sidebar" : ""}
        </span>
      </header>
      <div className="board">
        {cols.map((c) => (
          <section key={c.key} className={`board-column board-${c.key}`} aria-label={c.title}>
            <h3>
              {c.title} <span className="muted">{c.sessions.length}</span>
            </h3>
            <ul>
              {c.sessions.map((s) => (
                <li key={s.id}>
                  <button className="board-card" onClick={() => onOpen(s.id)}>
                    <span className="board-card-title">{s.title}</span>
                    <span className="muted board-card-where">
                      {projectName(s)}
                      {s.branch ? ` · ${s.branch}` : ""}
                    </span>
                    <span className="session-meta">
                      <span className={`badge badge-${s.status}`}>{statusLabel[s.status] ?? s.status}</span>
                      <span className="muted">{since(s.updatedAt)}</span>
                      {(s.tags ?? []).map((t) => (
                        <span key={t} className="tag">
                          #{t}
                        </span>
                      ))}
                    </span>
                  </button>
                </li>
              ))}
              {c.sessions.length === 0 && <li className="muted board-empty">None</li>}
            </ul>
          </section>
        ))}
      </div>
    </div>
  );
}
