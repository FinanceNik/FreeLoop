import { useEffect, useRef, useState } from "react";
import { Browser, Events, Window } from "@wailsio/runtime";
import {
  CONFIG_EVENT_NAME,
  EngineService,
  EVENT_NAME,
  OPEN_SESSION_EVENT_NAME,
  errorText,
  type AttentionItem,
  type Config,
  type DayCost,
  type EnvironmentInfo,
  type HarnessInfo,
  type Issue,
  type Profile,
  type ProjectInfo,
  type ProviderInfo,
  type Session,
  type UpdateInfo,
  type SessionHits,
} from "./api.ts";
import { apply, emptyTranscript, type Transcript, type TranscriptEvent } from "./transcript.ts";
import { HIT_LABEL, snippetParts } from "./search.ts";
import { formatCost } from "./cost.ts";
import { windowSession } from "./windows.ts";
import * as ws from "./workspace.ts";
import { allTags, isEmpty, matches, parseQuery, sameFilter, saveFilter, type SavedFilter, type SessionFilter } from "./filters.ts";
import { Palette, PaletteContext, useRegistry } from "./Palette.tsx";
import type { PaletteItem } from "./palette.ts";
import SessionView, { OPEN_SESSION_DOM_EVENT } from "./SessionView.tsx";
import Board from "./Board.tsx";
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
import SettingsNav, { Icon } from "./SettingsNav.tsx";
import { inSettings, isSettingsShortcut } from "./settings.ts";
import ClaudeImport from "./ClaudeImport.tsx";

type Screen =
  | { kind: "session"; id: string; focus?: { seq: number; n: number } }
  | { kind: "new"; root?: string }
  | { kind: "onboarding" }
  | { kind: "providers" }
  | { kind: "rules" }
  | { kind: "environments" }
  | { kind: "settings" }
  | { kind: "attention" }
  | { kind: "board" }
  | { kind: "profiles" }
  | { kind: "skills" }
  | { kind: "mcp" }
  | { kind: "import" }
  | { kind: "none" };

const STATUS_LABEL: Record<string, string> = {
  running: "Running",
  waiting_permission: "Needs you",
  waiting_input: "Waiting",
  queued: "Queued",
  idle: "Idle",
  error: "Error",
};

// A session's own window (GUI-015) shows that session alone; every window
// receives every engine event.
const STANDALONE = windowSession(window.location.search);

export default function App() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [sessionsLoaded, setSessionsLoaded] = useState(false);
  // The main window's panes (GUI-017).
  const [work, setWork] = useState<ws.WorkspaceState>(ws.SINGLE);
  const workLoaded = useRef(false);
  const [providers, setProviders] = useState<ProviderInfo[]>([]);
  const [projects, setProjects] = useState<ProjectInfo[]>([]);
  const [environments, setEnvironments] = useState<EnvironmentInfo[]>([]);
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [harnesses, setHarnesses] = useState<HarnessInfo[]>([]);
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
  // Tags every listed session must have (SES-013).
  const [tagFilter, setTagFilter] = useState<string[]>([]);
  // The name being typed for a new saved filter, while the field is open.
  const [filterName, setFilterName] = useState<string | null>(null);
  const [showArchived, setShowArchived] = useState(false);
  // Full-text results for the sidebar query (SES-007), for that query.
  const [found, setFound] = useState<{ query: string; results: SessionHits[] } | null>(null);
  // Sessions that need the user (GUI-002).
  const [attention, setAttention] = useState<AttentionItem[]>([]);
  // Today's model calls and their estimated cost (PRV-008).
  const [today, setToday] = useState<DayCost | null>(null);
  // The command palette (GUI-014) and the actions views offer to it.
  const [palette, setPalette] = useState<PaletteItem[] | null>(null);
  const { registry, collect } = useRegistry();
  const searchInput = useRef<HTMLInputElement>(null);
  // The message a fork was made before, for its composer (SES-009).
  const [forkDraft, setForkDraft] = useState<{ sid: string; text: string } | null>(null);

  // The reducer state lives in a ref so event callbacks always see the latest
  // transcripts; React state mirrors it for rendering.
  const transcriptsRef = useRef<Record<string, Transcript>>({});
  // Per session: a catch-up fetch is in flight, and whether another gap was
  // seen meanwhile and needs one more fetch.
  const fetching = useRef<Map<string, boolean>>(new Map());

  const report = (err: unknown) => setError(errorText(err));

  // A list can arrive after events that changed a session since it was
  // read, or after a list asked for later; neither replaces newer state.
  const refreshCount = useRef(0);
  const refreshSessions = () => {
    const n = ++refreshCount.current;
    return EngineService.ListSessions()
      .then((list) => {
        if (n !== refreshCount.current) return;
        setSessions((old) => (list ?? []).map((s) => old.find((o) => o.id === s.id && o.lastSeq > s.lastSeq) ?? s));
        setSessionsLoaded(true);
      })
      .catch(report);
  };
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
  const refreshToday = () =>
    EngineService.Usage(1)
      .then((days) => setToday(days?.[0] ?? null))
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
  const refreshProfiles = () => {
    EngineService.ListHarnesses()
      .then((list) => setHarnesses(list ?? []))
      .catch(report);
    return EngineService.ListProfiles()
      .then((list) => setProfiles(list ?? []))
      .catch(report);
  };
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
    if (STANDALONE) openSession(STANDALONE);

    return Events.On(EVENT_NAME, ({ data: ev }) => {
      ingest(ev.sid, [ev]);
      if (ev.type === "turn_ended") refreshToday();
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
        if (ev.type === "model_changed" && d.model) {
          s.model = d.model;
          s.providerId = d.providerId || s.providerId;
        }
        if (ev.type === "session_info_update" && d.title) s.title = d.title;
        if (ev.type === "tags_changed") s.tags = (ev.data as { tags?: string[] }).tags ?? [];
        return [s, ...list.slice(0, i), ...list.slice(i + 1)];
      });
    });
  }, []);

  // A session opens in the active pane, or with where "new" in another one
  // (GUI-017); the screen follows the active pane.
  const openSession = (sid: string, seq?: number, where: "active" | "new" = "active") => {
    setWork((w) => ws.openIn(w, sid, where));
    setScreen({ kind: "session", id: sid, focus: seq ? { seq, n: Date.now() } : undefined });
    setLastSessionId(sid);
    if (!transcriptsRef.current[sid]) catchUp(sid);
  };
  // showWork shows the workspace as next, its active pane's session first.
  const showWork = (next: ws.WorkspaceState) => {
    setWork(next);
    const sid = next.panes[next.active];
    setScreen({ kind: "session", id: sid ?? "" });
    if (sid) {
      setLastSessionId(sid);
      if (!transcriptsRef.current[sid]) catchUp(sid);
    }
  };
  const workRef = useRef(work);
  workRef.current = work;
  const activatePane = (i: number) => {
    if (i !== workRef.current.active) showWork(ws.focusPane(workRef.current, i));
  };
  const activatePaneRef = useRef(activatePane);
  activatePaneRef.current = activatePane;
  const showWorkRef = useRef(showWork);
  showWorkRef.current = showWork;

  // The workspace comes back as it was left, without sessions deleted since,
  // and is saved whenever a pane changes (GUI-017).
  useEffect(() => {
    if (STANDALONE || !sessionsLoaded || workLoaded.current) return;
    workLoaded.current = true;
    EngineService.Workspace()
      .then((saved) => {
        const exists = (sid: string) => sessions.some((x) => x.id === sid);
        const w = ws.forget(ws.restore({ ...saved, panes: (saved.panes ?? []).map((x) => x || null) }), exists);
        setWork(w);
        for (const sid of w.panes) if (sid && !transcriptsRef.current[sid]) catchUp(sid);
        const sid = w.panes[w.active];
        if (sid) setScreen((sc) => (sc.kind === "none" ? { kind: "session", id: sid } : sc));
      })
      .catch(() => {});
  }, [sessionsLoaded]);
  useEffect(() => {
    if (!workLoaded.current) return;
    const t = setTimeout(() => {
      EngineService.SaveWorkspace({ layout: work.layout, panes: work.panes.map((x) => x ?? ""), active: work.active }).catch(() => {});
    }, 400);
    return () => clearTimeout(t);
  }, [work]);
  useEffect(() => {
    if (sessionsLoaded) setWork((w) => ws.forget(w, (sid) => sessions.some((x) => x.id === sid)));
  }, [sessions, sessionsLoaded]);

  // ⌥⌘1 to ⌥⌘4 make a pane active, ⌥⌘← and ⌥⌘→ move between panes.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (STANDALONE || !e.metaKey || !e.altKey || e.ctrlKey || e.shiftKey) return;
      const w = workRef.current;
      const digit = /^Digit([1-4])$/.exec(e.code);
      if (digit && Number(digit[1]) <= w.layout) {
        e.preventDefault();
        activatePaneRef.current(Number(digit[1]) - 1);
      } else if ((e.code === "ArrowRight" || e.code === "ArrowLeft") && w.layout > 1) {
        e.preventDefault();
        showWorkRef.current(ws.cycle(w, e.code === "ArrowRight" ? 1 : -1));
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // ⌘1 to ⌘9 open the sessions listed first, ⌘0 the attention view
  // (NFR-A11Y-001).
  const listedRef = useRef<Session[]>([]);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (STANDALONE || !e.metaKey || e.altKey || e.ctrlKey || e.shiftKey || !/^[0-9]$/.test(e.key)) return;
      e.preventDefault();
      if (e.key === "0") {
        setScreen({ kind: "attention" });
        return;
      }
      const s = listedRef.current[Number(e.key) - 1];
      if (s) openRef.current(s.id);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // ⌘, opens the settings area; its Back returns to the last screen
  // outside it.
  const outside = useRef<Screen>({ kind: "none" });
  if (!inSettings(screen.kind)) outside.current = screen;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (STANDALONE || !isSettingsShortcut(e)) return;
      e.preventDefault();
      setScreen((s) => (inSettings(s.kind) ? s : { kind: "settings" }));
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  const leaveSettings = () => {
    const s = outside.current;
    setScreen(s.kind === "session" && !sessions.some((x) => x.id === s.id) ? { kind: "none" } : s);
  };

  // ⌘K opens the command palette over everything, and closes it again.
  const paletteRef = useRef<() => PaletteItem[]>(() => []);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!e.metaKey || e.altKey || e.ctrlKey || e.shiftKey || e.key.toLowerCase() !== "k") return;
      e.preventDefault();
      setPalette((open) => (open ? null : paletteRef.current()));
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // The first-run guide (GUI-010) opens once, before the first session.
  useEffect(() => {
    if (!STANDALONE && config && sessionsLoaded && !config.ui.onboarded && sessions.length === 0 && screen.kind === "none") setScreen({ kind: "onboarding" });
  }, [config, sessionsLoaded]);

  // A clicked notification opens its session (GUI-008).
  const openRef = useRef(openSession);
  openRef.current = openSession;
  useEffect(() => (STANDALONE ? undefined : Events.On(OPEN_SESSION_EVENT_NAME, ({ data }) => openRef.current(String(data)))), []);
  // A link inside a session (a subagent's transcript, HRN-014).
  useEffect(() => {
    const open = (e: Event) => {
      const sid = String((e as CustomEvent).detail);
      // A session window keeps its session; another opens in its own.
      if (STANDALONE) {
        if (sid !== STANDALONE) openWindow(sid);
        return;
      }
      refreshSessions();
      openRef.current(sid);
    };
    window.addEventListener(OPEN_SESSION_DOM_EVENT, open);
    return () => window.removeEventListener(OPEN_SESSION_DOM_EVENT, open);
  }, []);

  const selected = screen.kind === "session" ? (sessions.find((s) => s.id === screen.id) ?? null) : null;

  const openWindow = (sid: string) => EngineService.OpenSessionWindow(sid).catch(report);
  // A newer release, when the user turned the update check on (OQ-015).
  const [update, setUpdate] = useState<UpdateInfo | null>(null);
  useEffect(() => {
    if (STANDALONE) return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const ask = (again: boolean) =>
      EngineService.UpdateStatus()
        .then((u) => {
          setUpdate(u);
          // A check due now runs in the background; look once more after it.
          const due = u.enabled && Date.now() - new Date(u.checked).getTime() > 24 * 3600 * 1000;
          if (due && again) timer = setTimeout(() => ask(false), 5000);
        })
        .catch(() => {});
    ask(true);
    return () => clearTimeout(timer);
  }, [config]);
  // The window's title follows its session's (a fork made here opens here).
  const standaloneTitle = STANDALONE ? selected?.title : undefined;
  useEffect(() => {
    if (standaloneTitle) {
      document.title = standaloneTitle;
      Window.SetTitle(standaloneTitle).catch(() => {});
    }
  }, [standaloneTitle]);

  const relink = (p: ProjectInfo) =>
    EngineService.ChooseFolder()
      .then((root) => (root ? EngineService.RelinkProject(p.id, root) : null))
      .then(() => {
        refreshProjects();
        refreshSessions();
      })
      .catch(report);

  // Saved prices change today's estimate.
  useEffect(() => {
    if (config) refreshToday();
  }, [config]);

  // The query also searches every transcript, once typing pauses.
  useEffect(() => {
    const q = parseQuery(query).text;
    if (q.length < 2) {
      setFound(null);
      return;
    }
    const timer = setTimeout(() => {
      EngineService.Search(q)
        .then((results) => setFound({ query: q, results: results ?? [] }))
        .catch(report);
    }, 200);
    return () => clearTimeout(timer);
  }, [query]);

  // Sessions grouped under their projects (GUI-001); sessions from before
  // projects existed are grouped by folder.
  const groups: { key: string; name: string; project?: ProjectInfo; sessions: Session[] }[] = [];
  const filter: SessionFilter = { query, status: statusFilter, tags: tagFilter };
  const applyFilter = (f: SessionFilter) => {
    setQuery(f.query);
    setStatusFilter(f.status);
    setTagFilter(f.tags);
  };
  const savedFilters: SavedFilter[] = (config?.filters ?? []).map((f) => ({ ...f, tags: f.tags ?? [] }));
  const saveFilters = (filters: SavedFilter[]) => {
    if (!config) return;
    EngineService.SaveSettings({ ...config, filters })
      .then((c) => c && setConfig(c))
      .catch(report);
  };
  const tagsInUse = allTags(sessions.filter((s) => s.status !== "archived"));
  // Subagents are listed under the session that started them (HRN-014).
  const subagents = (id: string) => sessions.filter((c) => c.parentId === id && (showArchived || c.status !== "archived"));
  const visible = sessions.filter(
    (s) =>
      !s.parentId &&
      (showArchived ? s.status === "archived" : s.status !== "archived") &&
      matches(s, filter),
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
  listedRef.current = groups.flatMap((g) => g.sessions);

  // Each waiting request counts, and each session in error or waiting for a
  // reply without a request.
  const attentionCount = attention.reduce((n, it) => n + Math.max(it.requests?.length ?? 0, 1), 0);

  // The palette lists, in this order: the views' own actions (the open
  // session's first), the app's pages and actions, then every session.
  paletteRef.current = () => {
    const page = (id: string, title: string, go: () => void, extra: Partial<PaletteItem> = {}): PaletteItem => ({ id: `page:${id}`, title, group: "Page", run: go, ...extra });
    const items: PaletteItem[] = [
      ...collect(),
      { id: "app:new", title: "New session", group: "Action", keywords: "create start", run: () => setScreen({ kind: "new" }) },
      page("attention", "Needs attention", () => (refreshAttention(), setScreen({ kind: "attention" })), { shortcut: "⌘0", detail: attentionCount ? `${attentionCount} waiting` : undefined }),
      page("board", "Board", () => setScreen({ kind: "board" }), { keywords: "cards overview columns kanban" }),
      page("providers", "Providers", () => setScreen({ kind: "providers" }), { keywords: "api keys models" }),
      page("profiles", "Profiles", () => setScreen({ kind: "profiles" }), { keywords: "harness agents" }),
      page("skills", "Skills", () => setScreen({ kind: "skills" })),
      page("mcp", "MCP servers", () => setScreen({ kind: "mcp" }), { keywords: "tools" }),
      page("environments", "Environments", () => setScreen({ kind: "environments" }), { keywords: "docker secrets variables" }),
      page("rules", "Permission rules", () => setScreen({ kind: "rules" }), { keywords: "allow deny" }),
      page("settings", "Settings", () => setScreen({ kind: "settings" }), { keywords: "preferences theme notifications prices" }),
      page("usage", "Usage and prices", () => setScreen({ kind: "settings" }), { keywords: "cost tokens", detail: today && (today.models ?? []).some((m) => m.priced) ? `today ≈ ${formatCost(today.cost)}` : undefined }),
      page("import", "Import from Claude Code", () => setScreen({ kind: "import" })),
      page("backup", "Back up or restore the configuration", () => setScreen({ kind: "settings" }), { keywords: "export import settings providers profiles" }),
      page("guide", "Setup guide", () => setScreen({ kind: "onboarding" }), { keywords: "onboarding first run" }),
      {
        id: "app:find",
        title: "Find a session",
        group: "Action",
        keywords: "search filter transcripts",
        run: () => searchInput.current?.focus(),
      },
      {
        id: "app:archived",
        title: showArchived ? "Show active sessions" : "Show archived sessions",
        group: "Filter",
        run: () => setShowArchived(!showArchived),
      },
      ...[
        ["", "Any status"],
        ["waiting_permission", "Needs you"],
        ["running", "Running"],
        ["idle", "Idle"],
        ["error", "Error"],
      ].map(([v, name]) => ({
        id: `filter:${v}`,
        title: `Show sessions: ${name}`,
        group: "Filter",
        detail: statusFilter === v ? "current" : undefined,
        run: () => setStatusFilter(v),
      })),
      ...tagsInUse.map((t) => ({
        id: `tag:${t}`,
        title: `Show sessions tagged #${t}`,
        group: "Filter",
        detail: tagFilter.includes(t) ? "current" : undefined,
        run: () => setTagFilter([t]),
      })),
      ...savedFilters.map((f) => ({
        id: `saved:${f.name}`,
        title: `Saved filter: ${f.name}`,
        group: "Filter",
        detail: sameFilter(f, filter) ? "current" : undefined,
        run: () => applyFilter(f),
      })),
      ...projects.map((p) => ({
        id: `new:${p.id}`,
        title: `New session in ${p.name}`,
        group: "Action",
        detail: p.root,
        run: () => setScreen({ kind: "new", root: p.root }),
      })),
      ...projects
        .filter((p) => p.missing)
        .map((p) => ({ id: `relink:${p.id}`, title: `Relink ${p.name}…`, group: "Action", detail: p.root, keywords: "missing moved", run: () => relink(p) })),
      ...sessions.map((s) => {
        const listed = listedRef.current.indexOf(s);
        return {
          id: `session:${s.id}`,
          title: s.title,
          group: "Session",
          detail: `${projects.find((p) => p.id === s.projectId)?.name ?? basename(s.projectRoot)} · ${STATUS_LABEL[s.status] ?? s.status}`,
          keywords: `${s.branch ?? ""} ${s.status === "archived" ? "archived" : ""}`,
          shortcut: listed >= 0 && listed < 9 ? `⌘${listed + 1}` : undefined,
          run: () => openSession(s.id),
        };
      }),
    ];
    return items;
  };

  return (
    <PaletteContext.Provider value={registry}>
    <div className={STANDALONE ? "app standalone" : "app"}>
      {!STANDALONE && (
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
            aria-keyshortcuts="Meta+0"
            title="⌘0"
            onClick={() => {
              refreshAttention();
              setScreen({ kind: "attention" });
            }}
          >
            Needs attention
            {attentionCount > 0 && <span className="badge badge-waiting_permission">{attentionCount}</span>}
          </button>
          <button className="attention-link" aria-current={screen.kind === "board" ? "page" : undefined} onClick={() => setScreen({ kind: "board" })}>
            Board
          </button>
          <div className="filters">
            <input ref={searchInput} type="search" placeholder="Find a session" aria-label="Find a session" value={query} onChange={(e) => setQuery(e.target.value)} />
            <div className="row">
              <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} aria-label="Status">
                <option value="">Any status</option>
                <option value="waiting_permission">Needs you</option>
                <option value="running">Running</option>
                <option value="idle">Idle</option>
                <option value="error">Error</option>
              </select>
              {tagsInUse.length > 0 && (
                <select value={tagFilter[0] ?? ""} onChange={(e) => setTagFilter(e.target.value ? [e.target.value] : [])} aria-label="Tag">
                  <option value="">Any tag</option>
                  {tagsInUse.map((t) => (
                    <option key={t} value={t}>
                      #{t}
                    </option>
                  ))}
                </select>
              )}
              <label className="row muted">
                <input type="checkbox" checked={showArchived} onChange={(e) => setShowArchived(e.target.checked)} /> Archived
              </label>
            </div>
            {(savedFilters.length > 0 || !isEmpty(filter)) && (
              <div className="saved-filters" role="group" aria-label="Saved filters">
                {savedFilters.map((f) => (
                  <span key={f.name} className={`saved-filter ${sameFilter(f, filter) ? "saved-filter-on" : ""}`}>
                    <button
                      className="saved-filter-name"
                      aria-pressed={sameFilter(f, filter)}
                      title={[f.query, f.status && (STATUS_LABEL[f.status] ?? f.status), ...(f.tags ?? []).map((t) => `#${t}`)].filter(Boolean).join(" · ")}
                      onClick={() => (sameFilter(f, filter) ? applyFilter({ query: "", status: "", tags: [] }) : applyFilter(f))}
                    >
                      {f.name}
                    </button>
                    <button className="saved-filter-remove" aria-label={`Remove the saved filter ${f.name}`} onClick={() => saveFilters(savedFilters.filter((x) => x !== f))}>
                      ×
                    </button>
                  </span>
                ))}
                {!isEmpty(filter) && !savedFilters.some((f) => sameFilter(f, filter)) && filterName === null && (
                  <button className="link" onClick={() => setFilterName("")}>
                    Save filter…
                  </button>
                )}
                {filterName !== null && (
                  <form
                    className="row"
                    onSubmit={(e) => {
                      e.preventDefault();
                      if (!filterName.trim()) return;
                      saveFilters(saveFilter(savedFilters, filterName, filter));
                      setFilterName(null);
                    }}
                  >
                    <input
                      autoFocus
                      aria-label="Filter name"
                      placeholder="Name"
                      value={filterName}
                      maxLength={40}
                      onChange={(e) => setFilterName(e.target.value)}
                      onKeyDown={(e) => e.key === "Escape" && setFilterName(null)}
                    />
                    <button type="submit" disabled={!filterName.trim()}>
                      Save
                    </button>
                  </form>
                )}
              </div>
            )}
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
                        aria-keyshortcuts={listedRef.current.indexOf(s) < 9 ? `Meta+${listedRef.current.indexOf(s) + 1}` : undefined}
                        onClick={() => openSession(s.id)}
                      >
                        <span className="session-title">{s.title}</span>
                        <span className="session-meta">
                          <span className={`badge badge-${s.status}`}>{STATUS_LABEL[s.status] ?? s.status}</span>
                          {s.branch && <span className="muted">{s.branch}</span>}
                          {(s.tags ?? []).map((t) => (
                            <span key={t} className="tag">
                              #{t}
                            </span>
                          ))}
                        </span>
                      </button>
                      {subagents(s.id).length > 0 && (
                        <ul className="subagents" aria-label={`Subagents of ${s.title}`}>
                          {subagents(s.id).map((c) => (
                            <li key={c.id}>
                              <button
                                className="session-row session-child"
                                aria-current={screen.kind === "session" && screen.id === c.id ? "true" : undefined}
                                onClick={() => openSession(c.id)}
                              >
                                <span className="session-title">{c.title.replace(/^Subagent: /, "")}</span>
                                <span className={`badge badge-${c.status}`}>{STATUS_LABEL[c.status] ?? c.status}</span>
                              </button>
                            </li>
                          ))}
                        </ul>
                      )}
                    </li>
                  ))}
                </ul>
              </section>
            ))}
            {sessions.length === 0 && <p className="muted">No sessions yet.</p>}
            {sessions.length > 0 && visible.length === 0 && !found?.results.length && <p className="muted">No sessions match.</p>}
            {found && found.query === query.trim() && found.results.length > 0 && (
              <section className="project-group search-results" aria-label="Found in transcripts">
                <h2 className="project-name">In transcripts</h2>
                <ul>
                  {found.results.map((r) => {
                    const s = sessions.find((x) => x.id === r.sessionId);
                    if (!s) return null;
                    return (
                      <li key={r.sessionId}>
                        <span className="session-title search-session" title={s.projectRoot}>
                          {s.title}
                          {s.status === "archived" && <span className="muted"> · archived</span>}
                        </span>
                        <ul>
                          {(r.hits ?? []).map((h, i) => (
                            <li key={i}>
                              <button className="search-hit" onClick={() => openSession(s.id, h.seq)}>
                                <span className="muted search-kind">{HIT_LABEL[h.kind] ?? h.kind}</span>
                                <span className="search-snippet">
                                  {snippetParts(h.snippet).map((p, j) => (p.mark ? <mark key={j}>{p.text}</mark> : <span key={j}>{p.text}</span>))}
                                </span>
                              </button>
                            </li>
                          ))}
                        </ul>
                      </li>
                    );
                  })}
                </ul>
              </section>
            )}
          </nav>
          {update?.newer && update.url && (
          <button className="link today" title={update.url} onClick={() => Browser.OpenURL(update.url!).catch(report)}>
            Version {update.latest} is available
          </button>
        )}
        {config?.network?.localOnly && (
            <button className="link today" title="The app connects to nothing but this Mac; change it in Settings" onClick={() => setScreen({ kind: "settings" })}>
              Local only
            </button>
          )}
          {today && (today.models ?? []).length > 0 && (
            <button
              className="link today"
              title="Estimated from the price table in Settings; calls to models without a price are left out"
              onClick={() => setScreen({ kind: "settings" })}
            >
              {(today.models ?? []).some((m) => m.priced)
                ? `Today ≈ ${formatCost(today.cost)}${(today.models ?? []).some((m) => !m.priced) ? "+" : ""}`
                : "Today: no prices set"}
            </button>
          )}
          <footer>
            <button
              className="settings-link"
              aria-current={inSettings(screen.kind) ? "page" : undefined}
              aria-keyshortcuts="Meta+,"
              onClick={() => (inSettings(screen.kind) ? leaveSettings() : setScreen({ kind: "settings" }))}
            >
              <Icon name="general" />
              Settings
              <kbd>⌘,</kbd>
            </button>
          </footer>
        </aside>
      )}

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
            focus={screen.focus}
            initialDraft={forkDraft?.sid === selected.id ? forkDraft.text : undefined}
            profiles={profiles}
            onForked={(sid, text) => {
              setForkDraft({ sid, text });
              refreshSessions();
              openSession(sid);
            }}
            prices={config?.prices ?? []}
            knownTags={tagsInUse}
            events={transcripts[selected.id]?.events ?? []}
            provider={providers.find((p) => p.id === selected.providerId)}
            providers={providers}
            environment={environments.find((x) => x.id === (selected.environmentId || "local"))}
            harness={harnesses.find((h) => h.id === selected.harnessId)}
            project={projects.find((p) => p.id === selected.projectId)}
            onProjectsChanged={refreshProjects}
            configVersion={configVersion}
            allowBypass={config?.permissions.allowBypass ?? false}
            onError={report}
            onOpenWindow={STANDALONE ? undefined : () => openWindow(selected.id)}
            onDeleted={() => {
              if (STANDALONE) {
                Window.Close().catch(() => {});
                return;
              }
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
            harnesses={harnesses}
            sessions={sessions}
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
        {screen.kind === "onboarding" && config && (
          <Onboarding
            config={config}
            providers={providers}
            harnesses={harnesses}
            onProvidersChanged={refreshProviders}
            onProjectsChanged={refreshProjects}
            onSaved={setConfig}
            onDone={(root) => setScreen({ kind: "new", root })}
            onError={report}
          />
        )}
        {inSettings(screen.kind) && (
          <div className="settings-area">
            <SettingsNav current={screen.kind} onOpen={(kind) => setScreen({ kind })} onBack={leaveSettings} />
            <div className="settings-content">
              {screen.kind === "providers" && (
                <Providers
                  providers={providers}
                  onChanged={refreshProviders}
                  onError={report}
                  agents={harnesses.filter((h) => h.kind === "acp")}
                  defaultProfileId={config?.defaults?.profileId}
                  onStartWith={(profileId) => {
                    // The agent becomes the default, so the form comes up with it.
                    if (!config) return setScreen({ kind: "new" });
                    EngineService.SaveSettings({ ...config, defaults: { ...config.defaults, profileId } })
                      .then((c) => {
                        setConfig(c);
                        setScreen({ kind: "new" });
                      })
                      .catch(report);
                  }}
                />
              )}
              {screen.kind === "settings" && config && (
                <Settings
                  config={config}
                  providers={providers}
                  environments={environments}
                  profiles={profiles}
                  projects={projects}
                  onProjectsChanged={refreshProjects}
                  onImport={() => setScreen({ kind: "import" })}
                  onProviders={() => setScreen({ kind: "providers" })}
                  onSaved={setConfig} onError={report} />
              )}
              {screen.kind === "environments" && <Environments environments={environments} onChanged={refreshEnvironments} onError={report} />}
              {screen.kind === "rules" && <Rules session={sessions.find((s) => s.id === lastSessionId) ?? null} version={configVersion} onError={report} />}
              {screen.kind === "mcp" && <MCP configVersion={configVersion} onImport={() => setScreen({ kind: "import" })} onError={report} />}
              {screen.kind === "import" && (
                <section className="page">
                  <h2>Import from Claude Code</h2>
                  <ClaudeImport onError={report} />
                </section>
              )}
              {screen.kind === "skills" && (
                <Skills projects={projects} providers={providers} configVersion={configVersion} onConfigChanged={refreshConfig} onProjectsChanged={refreshProjects} onError={report} />
              )}
              {screen.kind === "profiles" && (
                <Profiles
                  profiles={profiles}
                  harnesses={harnesses}
                  providers={providers}
                  onChanged={() => {
                    refreshProfiles();
                    refreshConfig();
                  }}
                  onError={report}
                />
              )}
            </div>
          </div>
        )}
        {screen.kind === "board" && (
          <Board
            sessions={visible.filter((s) => s.status !== "archived")}
            projectName={(s) => projects.find((p) => p.id === s.projectId)?.name ?? basename(s.projectRoot)}
            statusLabel={STATUS_LABEL}
            filtered={!isEmpty(filter)}
            onOpen={(id) => openSession(id)}
          />
        )}
        {screen.kind === "attention" && <Attention items={attention} onOpen={openSession} onChanged={refreshAttention} onError={report} />}
        {STANDALONE && screen.kind === "session" && !selected && sessionsLoaded && <div className="empty">This session no longer exists.</div>}
        {!STANDALONE && screen.kind === "none" && (
          <div className="empty">
            <p>Select a session or create a new one.</p>
            <button className="link" onClick={() => setScreen({ kind: "onboarding" })}>
              Open the setup guide
            </button>
          </div>
        )}
      </main>
      {palette && <Palette items={palette} onClose={() => setPalette(null)} />}
    </div>
    </PaletteContext.Provider>
  );
}

function basename(p: string): string {
  const parts = p.split("/").filter(Boolean);
  return parts[parts.length - 1] ?? p;
}
