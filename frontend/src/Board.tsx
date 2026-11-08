import type { ReactNode } from "react";
import { columns, since } from "./board.ts";
import { counts, relations, stateLabel, typeLabel } from "./backlog.ts";
import type { BacklogInfo } from "./api.ts";

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
  backlogs = [],
  onArchive,
  backlogActions,
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
  // Trusted projects' workflow backlogs (WFL-004), shown before the sessions.
  backlogs?: BacklogInfo[];
  onArchive?: (b: BacklogInfo) => void;
  // Extra controls for a project's backlog, such as its AFK run (WFL-005).
  backlogActions?: (b: BacklogInfo) => ReactNode;
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
        {backlogs.length > 0 && (
          <section className="board-column board-backlog" aria-label="Backlog">
            <h3>
              Backlog <span className="muted">{backlogs.reduce((n, b) => n + counts(b.issues).open, 0)}</span>
            </h3>
            {backlogs.map((b) => {
              const n = counts(b.issues);
              return (
                <div key={b.projectId} className="backlog-project" aria-label={`Backlog of ${b.projectName}`}>
                  <div className="backlog-project-head">
                    <strong>{b.projectName}</strong>
                    <span className="muted">
                      {" "}
                      · {n.open} open, {n.afkReady} AFK ready
                      {b.uncommitted ? " · not committed" : ""}
                    </span>
                  </div>
                  {backlogActions?.(b)}
                  <ul>
                    {(b.issues ?? []).map((it) => (
                      <li key={it.id}>
                        <div className={`board-card backlog-card backlog-${it.state}`} aria-label={`${it.id}: ${it.title}`}>
                          <span className="board-card-title">{it.title}</span>
                          <span className="muted board-card-where">{it.path}</span>
                          <span className="session-meta">
                            <span className={`badge backlog-state backlog-state-${it.state}`}>{stateLabel(it)}</span>
                            <span className={`tag backlog-type-${it.type}`}>{typeLabel(it)}</span>
                          </span>
                          {relations(it) && <span className="muted backlog-relations">{relations(it)}</span>}
                        </div>
                      </li>
                    ))}
                    {(b.issues ?? []).length === 0 && <li className="muted board-empty">No issues yet</li>}
                  </ul>
                  {b.finished > 0 && onArchive && (
                    <button className="link backlog-archive" onClick={() => onArchive(b)} title="Moves done issues, and PRDs whose issues are all done, out of the project into Freeloop's data folder, in a commit of their own.">
                      Archive finished ({b.finished})
                    </button>
                  )}
                </div>
              );
            })}
          </section>
        )}
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
