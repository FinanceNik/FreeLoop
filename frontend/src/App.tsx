import { useEffect, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";
import {
  EngineService,
  EVENT_NAME,
  errorText,
  type Config,
  type EnvironmentInfo,
  type Issue,
  type ProjectInfo,
  type ProviderInfo,
  type Session,
} from "./api.ts";
import { apply, emptyTranscript, type Transcript, type TranscriptEvent } from "./transcript.ts";
import SessionView from "./SessionView.tsx";
import Providers from "./Providers.tsx";
import NewSession from "./NewSession.tsx";
import Rules from "./Rules.tsx";
import Environments from "./Environments.tsx";

type Screen =
  | { kind: "session"; id: string }
  | { kind: "new" }
  | { kind: "providers" }
  | { kind: "rules" }
  | { kind: "environments" }
  | { kind: "none" };

const STATUS_LABEL: Record<string, string> = {
  running: "Running",
  waiting_permission: "Needs you",
  waiting_input: "Waiting",
  idle: "Idle",
  error: "Error",
};

export default function App() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [providers, setProviders] = useState<ProviderInfo[]>([]);
  const [projects, setProjects] = useState<ProjectInfo[]>([]);
  const [environments, setEnvironments] = useState<EnvironmentInfo[]>([]);
  const [config, setConfig] = useState<Config | null>(null);
  const [screen, setScreen] = useState<Screen>({ kind: "none" });
  const [transcripts, setTranscripts] = useState<Record<string, Transcript>>({});
  const [issues, setIssues] = useState<Issue[]>([]);
  // The last session opened, for the rules page's session scope.
  const [lastSessionId, setLastSessionId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // The reducer state lives in a ref so event callbacks always see the latest
  // transcripts; React state mirrors it for rendering.
  const transcriptsRef = useRef<Record<string, Transcript>>({});
  // Per session: a catch-up fetch is in flight, and whether another gap was
  // seen meanwhile and needs one more fetch.
  const fetching = useRef<Map<string, boolean>>(new Map());

  const report = (err: unknown) => setError(errorText(err));

  const refreshSessions = () =>
    EngineService.ListSessions()
      .then((list) => setSessions(list ?? []))
      .catch(report);
  const refreshProjects = () =>
    EngineService.ListProjects()
      .then((list) => setProjects(list ?? []))
      .catch(report);
  const refreshEnvironments = () =>
    EngineService.ListEnvironments()
      .then((list) => setEnvironments(list ?? []))
      .catch(report);
  const refreshProviders = () =>
    EngineService.ListProviders()
      .then((list) => setProviders(list ?? []))
      .catch(report);

  const ingest = (sid: string, events: readonly TranscriptEvent[]) => {
    const current = transcriptsRef.current[sid] ?? emptyTranscript;
    const { transcript, missingAfter } = apply(current, events);
    if (transcript !== current) {
      transcriptsRef.current = { ...transcriptsRef.current, [sid]: transcript };
      setTranscripts(transcriptsRef.current);
    }
    if (missingAfter !== null) catchUp(sid);
  };

  const catchUp = (sid: string) => {
    if (fetching.current.has(sid)) {
      fetching.current.set(sid, true);
      return;
    }
    fetching.current.set(sid, false);
    const after = transcriptsRef.current[sid]?.lastSeq ?? 0;
    EngineService.Events(sid, after)
      .then((events) => {
        const again = fetching.current.get(sid);
        fetching.current.delete(sid);
        ingest(sid, events ?? []);
        if (again) catchUp(sid);
      })
      .catch((err) => {
        fetching.current.delete(sid);
        report(err);
      });
  };

  useEffect(() => {
    refreshSessions();
    refreshProviders();
    refreshProjects();
    refreshEnvironments();
    EngineService.Config().then(setConfig).catch(report);
    EngineService.ConfigIssues()
      .then((list) => setIssues(list ?? []))
      .catch(report);

    return Events.On(EVENT_NAME, ({ data: ev }) => {
      ingest(ev.sid, [ev]);
      setSessions((list) => {
        const i = list.findIndex((s) => s.id === ev.sid);
        if (i < 0) {
          refreshSessions();
          refreshProjects();
          return list;
        }
        const d = ev.data as Record<string, string>;
        const s = { ...list[i], lastSeq: Math.max(list[i].lastSeq, ev.seq), updatedAt: ev.ts };
        if (ev.type === "session_status" && d.status) s.status = d.status as Session["status"];
        if (ev.type === "current_mode_update" && d.currentModeId) s.mode = d.currentModeId;
        if (ev.type === "git_state") s.branch = d.branch ?? "";
        return [s, ...list.slice(0, i), ...list.slice(i + 1)];
      });
    });
  }, []);

  const openSession = (sid: string) => {
    setScreen({ kind: "session", id: sid });
    setLastSessionId(sid);
    if (!transcriptsRef.current[sid]) catchUp(sid);
  };

  const selected = screen.kind === "session" ? (sessions.find((s) => s.id === screen.id) ?? null) : null;

  const relink = (p: ProjectInfo) =>
    EngineService.ChooseFolder()
      .then((root) => (root ? EngineService.RelinkProject(p.id, root) : null))
      .then(() => {
        refreshProjects();
        refreshSessions();
      })
      .catch(report);

  // Sessions grouped under their projects (GUI-001); sessions from before
  // projects existed are grouped by folder.
  const groups: { key: string; name: string; project?: ProjectInfo; sessions: Session[] }[] = [];
  for (const s of sessions) {
    const project = projects.find((p) => p.id === s.projectId);
    const key = project?.id ?? s.projectRoot;
    let g = groups.find((x) => x.key === key);
    if (!g) {
      g = { key, name: project?.name ?? basename(s.projectRoot), project, sessions: [] };
      groups.push(g);
    }
    g.sessions.push(s);
  }

  return (
    <div className="app">
      <aside className="sidebar">
        <header>
          <h1>Freeloop</h1>
          <button className="primary" onClick={() => setScreen({ kind: "new" })}>
            New session
          </button>
        </header>
        <nav aria-label="Projects and sessions">
          {groups.map((g) => (
            <section key={g.key} className="project-group">
              <h2 className="project-name" title={g.project?.root ?? g.key}>
                {g.name}
                {g.project?.missing && (
                  <>
                    <span className="badge badge-error">missing</span>
                    <button className="link" onClick={() => relink(g.project!)}>
                      Relink…
                    </button>
                  </>
                )}
              </h2>
              <ul>
                {g.sessions.map((s) => (
                  <li key={s.id}>
                    <button
                      className="session-row"
                      aria-current={screen.kind === "session" && screen.id === s.id ? "true" : undefined}
                      onClick={() => openSession(s.id)}
                    >
                      <span className="session-title">{s.title}</span>
                      <span className="session-meta">
                        <span className={`badge badge-${s.status}`}>{STATUS_LABEL[s.status] ?? s.status}</span>
                        {s.branch && <span className="muted">{s.branch}</span>}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            </section>
          ))}
          {sessions.length === 0 && <p className="muted">No sessions yet.</p>}
        </nav>
        <footer className="row">
          <button onClick={() => setScreen({ kind: "providers" })}>Providers</button>
          <button onClick={() => setScreen({ kind: "environments" })}>Environments</button>
          <button onClick={() => setScreen({ kind: "rules" })}>Rules</button>
        </footer>
      </aside>

      <main className="main">
        {issues.length > 0 && (
          <div role="alert" className="banner warn">
            <strong>Configuration problems; using defaults where needed.</strong>
            <ul>
              {issues.map((i, n) => (
                <li key={n}>
                  {i.file}
                  {i.field ? ` (${i.field})` : ""}: {i.message}
                </li>
              ))}
            </ul>
          </div>
        )}
        {error && (
          <div role="alert" className="banner error">
            {error} <button onClick={() => setError(null)}>Dismiss</button>
          </div>
        )}

        {screen.kind === "session" && selected && (
          <SessionView
            key={selected.id}
            session={selected}
            events={transcripts[selected.id]?.events ?? []}
            provider={providers.find((p) => p.id === selected.providerId)}
            environment={environments.find((x) => x.id === (selected.environmentId || "local"))}
            allowBypass={config?.permissions.allowBypass ?? false}
            onError={report}
          />
        )}
        {screen.kind === "new" && (
          <NewSession
            providers={providers}
            projects={projects}
            environments={environments}
            onManageProviders={() => setScreen({ kind: "providers" })}
            onCreated={(s) => {
              setSessions((list) => (list.some((x) => x.id === s.id) ? list : [s, ...list]));
              openSession(s.id);
            }}
            onError={report}
          />
        )}
        {screen.kind === "providers" && <Providers providers={providers} onChanged={refreshProviders} onError={report} />}
        {screen.kind === "environments" && <Environments environments={environments} onChanged={refreshEnvironments} onError={report} />}
        {screen.kind === "rules" && <Rules session={sessions.find((s) => s.id === lastSessionId) ?? null} onError={report} />}
        {screen.kind === "none" && (
          <div className="empty">
            <p>Select a session or create a new one.</p>
          </div>
        )}
      </main>
    </div>
  );
}

function basename(p: string): string {
  const parts = p.split("/").filter(Boolean);
  return parts[parts.length - 1] ?? p;
}
