import type { ReactNode } from "react";
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
  unread,
  activityOf,
  markOf,
  renderRequest,
  onOpen,
  onQueue,
}: {
  sessions: T[];
  projectName: (s: T) => string;
  statusLabel: Record<string, string>;
  filtered: boolean;
  // Sessions that finished work the user has not seen (SES-022).
  unread: readonly string[];
  // What the session is doing (GUI-030).
  activityOf: (s: T) => ReactNode;
  // What it runs on (GUI-034).
  markOf: (s: T) => ReactNode;
  // The approval a session waits on longest, answered on its card.
  renderRequest: (sid: string) => ReactNode;
  onOpen: (id: string) => void;
  // Opens the queue of tasks (SES-025), whose tasks start as the sessions here.
  onQueue?: () => void;
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
        {onQueue && (
          <button className="link board-queue" onClick={onQueue}>
            Queue of tasks
          </button>
        )}
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
                    <span className="board-card-title">
                      {markOf(s)}
                      {unread.includes(s.id) && <span className="unread-dot" role="img" aria-label="Finished, not read yet" />}
                      {s.title}
                    </span>
                    <span className="muted board-card-where">
                      {projectName(s)}
                      {s.branch ? ` · ${s.branch}` : ""}
                    </span>
                    {activityOf(s)}
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
                  {renderRequest(s.id)}
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
