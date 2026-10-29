import { useEffect, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";
import {
  CONFIG_EVENT_NAME,
  EngineService,
  EVENT_NAME,
  OPEN_SESSION_EVENT_NAME,
  errorText,
  type AttentionItem,
  type Config,
  type EnvironmentInfo,
  type Issue,
  type Profile,
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
import Settings from "./Settings.tsx";
import Attention from "./Attention.tsx";
import Profiles from "./Profiles.tsx";
import Onboarding from "./Onboarding.tsx";
import Skills from "./Skills.tsx";
import MCP from "./MCP.tsx";

type Screen =
  | { kind: "session"; id: string }
  | { kind: "new"; root?: string }
  | { kind: "onboarding" }
  | { kind: "providers" }
  | { kind: "rules" }
  | { kind: "environments" }
  | { kind: "settings" }
  | { kind: "attention" }
  | { kind: "profiles" }
  | { kind: "skills" }
  | { kind: "mcp" }
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
  const [sessionsLoaded, setSessionsLoaded] = useState(false);
  const [providers, setProviders] = useState<ProviderInfo[]>([]);
  const [projects, setProjects] = useState<ProjectInfo[]>([]);
  const [environments, setEnvironments] = useState<EnvironmentInfo[]>([]);
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [config, setConfig] = useState<Config | null>(null);
  const [screen, setScreen] = useState<Screen>({ kind: "none" });
  const [transcripts, setTranscripts] = useState<Record<string, Transcript>>({});
  const [issues, setIssues] = useState<Issue[]>([]);
  // The last session opened, for the rules page's session scope.
  const [lastSessionId, setLastSessionId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  // Bumped when config files change on disk, so pages refetch (SET-001).
  const [configVersion, setConfigVersion] = useState(0);
  // Sidebar filters (SES-004).
  const [query, setQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [showArchived, setShowArchived] = useState(false);
  // Sessions that need the user (GUI-002).
  const [attention, setAttention] = useState<AttentionItem[]>([]);

  // The reducer state lives in a ref so event callbacks always see the latest
  // transcripts; React state mirrors it for rendering.
  const transcriptsRef = useRef<Record<string, Transcript>>({});
  // Per session: a catch-up fetch is in flight, and whether another gap was
  // seen meanwhile and needs one more fetch.
  const fetching = useRef<Map<string, boolean>>(new Map());

  const report = (err: unknown) => setError(errorText(err));

  const refreshSessions = () =>
    EngineService.ListSessions()
      .then((list) => {
        setSessions(list ?? []);
        setSessionsLoaded(true);
      })
      .catch(report);
  const refreshProjects = () =>
    EngineService.ListProjects()
      .then((list) => setProjects(list ?? []))
      .catch(report);
  const refreshEnvironments = () =>
    EngineService.ListEnvironments()
      .then((list) => setEnvironments(list ?? []))
      .catch(report);
  const refreshAttention = () =>
    EngineService.Attention()
      .then((list) => setAttention(list ?? []))
      .catch(report);
  // Events that change what needs attention arrive in bursts; one refresh
  // per burst is enough.
  const attentionTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const scheduleAttention = () => {
    if (attentionTimer.current) return;
    attentionTimer.current = setTimeout(() => {
      attentionTimer.current = null;
      refreshAttention();
    }, 100);
  };
  const refreshProfiles = () =>
    EngineService.ListProfiles()
      .then((list) => setProfiles(list ?? []))
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

  // Appearance (GUI-011): theme and font size from config.yaml.
  useEffect(() => {
    if (!config) return;
    const root = document.documentElement;
    if (config.ui.theme === "system") delete root.dataset.theme;
    else root.dataset.theme = config.ui.theme;
    root.style.setProperty("--font-size", `${config.ui.fontSize || 14}px`);
  }, [config?.ui.theme, config?.ui.fontSize]);

  // A config object only replaces the current one when it differs, so an
  // unrelated reload does not reset a half-edited settings form.
  const refreshConfig = () => {
    EngineService.Config()
      .then((c) => setConfig((old) => (old && JSON.stringify(old) === JSON.stringify(c) ? old : c)))
      .catch(report);
    EngineService.ConfigIssues()
      .then((list) => setIssues(list ?? []))
      .catch(report);
  };

  // Files edited by hand are reloaded by the engine (SET-001).
  useEffect(
    () =>
      Events.On(CONFIG_EVENT_NAME, () => {
        refreshConfig();
        refreshProviders();
        refreshEnvironments();
        refreshProfiles();
        setConfigVersion((v) => v + 1);
      }),
    [],
  );

  useEffect(() => {
    refreshSessions();
    refreshProviders();
    refreshProjects();
    refreshEnvironments();
    refreshConfig();
    refreshAttention();
    refreshProfiles();

    return Events.On(EVENT_NAME, ({ data: ev }) => {
      ingest(ev.sid, [ev]);
      if (ev.type === "permission_requested" || ev.type === "permission_decided" || ev.type === "session_status") scheduleAttention();
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
        if (ev.type === "session_info_update" && d.title) s.title = d.title;
        return [s, ...list.slice(0, i), ...list.slice(i + 1)];
      });
    });
  }, []);

  const openSession = (sid: string) => {
    setScreen({ kind: "session", id: sid });
    setLastSessionId(sid);
    if (!transcriptsRef.current[sid]) catchUp(sid);
  };

  // The first-run guide (GUI-010) opens once, before the first session.
  useEffect(() => {
    if (config && sessionsLoaded && !config.ui.onboarded && sessions.length === 0 && screen.kind === "none") setScreen({ kind: "onboarding" });
  }, [config, sessionsLoaded]);

  // A clicked notification opens its session (GUI-008).
  const openRef = useRef(openSession);
  openRef.current = openSession;
  useEffect(() => Events.On(OPEN_SESSION_EVENT_NAME, ({ data }) => openRef.current(String(data))), []);

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
  const q = query.trim().toLowerCase();
  const visible = sessions.filter(
    (s) =>
      (showArchived ? s.status === "archived" : s.status !== "archived") &&
      (!statusFilter || s.status === statusFilter) &&
      (!q || s.title.toLowerCase().includes(q) || s.projectRoot.toLowerCase().includes(q) || (s.branch ?? "").toLowerCase().includes(q)),
  );
  for (const s of visible) {
    const project = projects.find((p) => p.id === s.projectId);
    const key = project?.id ?? s.projectRoot;
    let g = groups.find((x) => x.key === key);
    if (!g) {
      g = { key, name: project?.name ?? basename(s.projectRoot), project, sessions: [] };
      groups.push(g);
    }
    g.sessions.push(s);
  }

  // Each waiting request counts, and each session in error or waiting for a
  // reply without a request.
  const attentionCount = attention.reduce((n, it) => n + Math.max(it.requests?.length ?? 0, 1), 0);

  return (
    <div className="app">
      <aside className="sidebar">
        <header>
          <h1>Freeloop</h1>
          <button className="primary" onClick={() => setScreen({ kind: "new" })}>
            New session
          </button>
        </header>
        <button
          className={`attention-link ${attentionCount > 0 ? "attention-link-on" : ""}`}
          aria-current={screen.kind === "attention" ? "page" : undefined}
          onClick={() => {
            refreshAttention();
            setScreen({ kind: "attention" });
          }}
        >
          Needs attention
          {attentionCount > 0 && <span className="badge badge-waiting_permission">{attentionCount}</span>}
        </button>
        <div className="filters">
          <input type="search" placeholder="Find a session" aria-label="Find a session" value={query} onChange={(e) => setQuery(e.target.value)} />
          <div className="row">
            <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} aria-label="Status">
              <option value="">Any status</option>
              <option value="waiting_permission">Needs you</option>
              <option value="running">Running</option>
              <option value="idle">Idle</option>
              <option value="error">Error</option>
            </select>
            <label className="row muted">
              <input type="checkbox" checked={showArchived} onChange={(e) => setShowArchived(e.target.checked)} /> Archived
            </label>
          </div>
        </div>
        <nav aria-label="Projects and sessions">
          {groups.map((g) => (
            <section key={g.key} className="project-group">
              <h2 className="project-name" title={g.project?.root ?? g.key}>
                {g.name}
                {g.project && !g.project.trusted && !g.project.missing && (
                  <span className="muted untrusted" title="Untrusted: global settings only">
                    untrusted
                  </span>
                )}
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
          {sessions.length > 0 && visible.length === 0 && <p className="muted">No sessions match.</p>}
        </nav>
        <footer className="row">
          <button onClick={() => setScreen({ kind: "providers" })}>Providers</button>
          <button onClick={() => setScreen({ kind: "profiles" })}>Profiles</button>
          <button onClick={() => setScreen({ kind: "skills" })}>Skills</button>
          <button onClick={() => setScreen({ kind: "mcp" })}>MCP</button>
          <button onClick={() => setScreen({ kind: "environments" })}>Environments</button>
          <button onClick={() => setScreen({ kind: "rules" })}>Rules</button>
          <button onClick={() => setScreen({ kind: "settings" })}>Settings</button>
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
                  {i.line ? `:${i.line}` : ""}
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
            project={projects.find((p) => p.id === selected.projectId)}
            onProjectsChanged={refreshProjects}
            configVersion={configVersion}
            allowBypass={config?.permissions.allowBypass ?? false}
            onError={report}
            onDeleted={() => {
              setScreen({ kind: "none" });
              refreshSessions();
            }}
          />
        )}
        {screen.kind === "new" && (
          <NewSession
            key={screen.root ?? ""}
            providers={providers}
            projects={projects}
            environments={environments}
            profiles={profiles}
            defaults={config?.defaults}
            initialRoot={screen.root}
            onManageProviders={() => setScreen({ kind: "providers" })}
            onProvidersChanged={refreshProviders}
            onProjectsChanged={refreshProjects}
            onCreated={(s) => {
              setSessions((list) => (list.some((x) => x.id === s.id) ? list : [s, ...list]));
              openSession(s.id);
            }}
            onError={report}
          />
        )}
        {screen.kind === "providers" && <Providers providers={providers} onChanged={refreshProviders} onError={report} />}
        {screen.kind === "settings" && config && (
          <Settings
            config={config}
            providers={providers}
            environments={environments}
            profiles={profiles}
            projects={projects}
            onProjectsChanged={refreshProjects}
            onSaved={setConfig} onError={report} />
        )}
        {screen.kind === "environments" && <Environments environments={environments} onChanged={refreshEnvironments} onError={report} />}
        {screen.kind === "rules" && <Rules session={sessions.find((s) => s.id === lastSessionId) ?? null} version={configVersion} onError={report} />}
        {screen.kind === "onboarding" && config && (
          <Onboarding
            config={config}
            providers={providers}
            onProvidersChanged={refreshProviders}
            onProjectsChanged={refreshProjects}
            onSaved={setConfig}
            onDone={(root) => setScreen({ kind: "new", root })}
            onError={report}
          />
        )}
        {screen.kind === "mcp" && <MCP configVersion={configVersion} onError={report} />}
        {screen.kind === "skills" && (
          <Skills projects={projects} configVersion={configVersion} onConfigChanged={refreshConfig} onProjectsChanged={refreshProjects} onError={report} />
        )}
        {screen.kind === "profiles" && (
          <Profiles
            profiles={profiles}
            providers={providers}
            onChanged={() => {
              refreshProfiles();
              refreshConfig();
            }}
            onError={report}
          />
        )}
        {screen.kind === "attention" && <Attention items={attention} onOpen={openSession} onChanged={refreshAttention} onError={report} />}
        {screen.kind === "none" && (
          <div className="empty">
            <p>Select a session or create a new one.</p>
            <button className="link" onClick={() => setScreen({ kind: "onboarding" })}>
              Open the setup guide
            </button>
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
