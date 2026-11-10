import { useEffect, useMemo, useRef, useState, type CSSProperties, type DragEvent } from "react";
import { screenName } from "./a11y.ts";
import { ariaKeys, bindingOf, keyLabel, pressed, setBindings } from "./shortcuts.ts";
import { isMac, otherKey, primaryKey, primaryOnly } from "./platform.ts";
import { Browser, Events, Window } from "@wailsio/runtime";
import {
  CONFIG_EVENT_NAME,
  EngineService,
  EVENT_NAME,
  OPEN_SESSION_EVENT_NAME,
  errorText,
  type AttentionItem,
  type BacklogInfo,
  type Config,
  type BudgetState, type DayCost,
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
import { hitLabel, snippetParts } from "./search.ts";
import { formatCost, type PriceEntry } from "./cost.ts";
import { windowSession } from "./windows.ts";
import * as ws from "./workspace.ts";
import { allTags, dateGroups, isEmpty, matches, parseQuery, sameFilter, saveFilter, statusSummary, type SavedFilter, type SessionFilter } from "./filters.ts";
import { Palette, PaletteContext, useRegistry } from "./Palette.tsx";
import type { PaletteItem } from "./palette.ts";
import SessionView from "./SessionView.tsx";
import { OPEN_SESSION_DOM_EVENT } from "./ToolView.tsx";
import Board from "./Board.tsx";
import Schedules from "./Schedules.tsx";
import Optimizers from "./Optimizers.tsx";
import Workflows from "./Workflows.tsx";
import AFKDialog from "./AFKDialog.tsx";
import AFKRunView from "./AFKRunView.tsx";
import { runLine, waitingStep } from "./afk.ts";
import Usage from "./Usage.tsx";
import Compare from "./Compare.tsx";
import Providers from "./Providers.tsx";
import NewSession from "./NewSession.tsx";
import Rules from "./Rules.tsx";
import Environments from "./Environments.tsx";
import Settings from "./Settings.tsx";
import Attention, { RequestView } from "./Attention.tsx";
import { addError, errorHeading, type ShownError } from "./errorlist.ts";
import { finishedTurn, markFinished, markRead, onScreen, parseUnread, UNREAD_KEY } from "./unread.ts";
import Profiles from "./Profiles.tsx";
import Onboarding from "./Onboarding.tsx";
import Workspace, { SESSION_DRAG_TYPE } from "./Workspace.tsx";
import Skills from "./Skills.tsx";
import MCP from "./MCP.tsx";
import SettingsNav from "./SettingsNav.tsx";
import { budgetLine, budgetName, worst } from "./budgets.ts";
import { BellIcon, BoardIcon, BookIcon, ChevronIcon, FilterIcon, Logo, NavIcon as Icon, NewSessionIcon, QueueIcon, UsageIcon } from "./icons.tsx";
import Help, { SHOWN_TOPICS } from "./Help.tsx";
import type { HelpPlace } from "./help.ts";
import { clearSelectionOnClick } from "./selection.ts";
import Splitter from "./Splitter.tsx";
import { clampSidebar, rememberSidebar, setPanelWidth, SIDEBAR_MIN, sidebarMax, storedPanel, storedSidebar } from "./sizes.ts";
import { forgetDraft, pruneDrafts } from "./drafts.ts";
import SourceMark from "./SourceMark.tsx";
import GitMark from "./GitMark.tsx";
import { contextHigh, contextLabel, contextTitle } from "./contextMeter.ts";
import { sourceId, sourceIndex, type SourceChoices } from "./source.ts";
import ContextMenu, { type MenuItem } from "./ContextMenu.tsx";
import { ask } from "./ask.ts";
import { trustLines } from "./trust.ts";
import { describeActivity, foldActivity, NO_ACTIVITY, type Activity } from "./sessionActivity.ts";
import { applyColors, type ColorSet } from "./colors.ts";
import { applyFonts, type FontChoice } from "./fonts.ts";
import { preferredLanguages, resolveLanguage, setLanguage, t, type PlainKey } from "./i18n.ts";
import { inSettings, isGeneral, isSettingsShortcut, MAX_FONT_SIZE, settingsStart, textSizeAfter, type GeneralPage } from "./settings.ts";
import ClaudeImport from "./ClaudeImport.tsx";
import SessionImport from "./SessionImport.tsx";
import { basename } from "./paths.ts";
import Queue from "./Queue.tsx";

type Screen =
  | { kind: "session"; id: string; focus?: { seq: number; n: number } }
  | { kind: "new"; root?: string; workflow?: string }
  | { kind: "onboarding" }
  | { kind: "providers" }
  | { kind: "rules" }
  | { kind: "environments" }
  | { kind: GeneralPage }
  | { kind: "attention" }
  | { kind: "board" }
  | { kind: "queue" }
  | { kind: "schedules" }
  | { kind: "optimizers" }
  | { kind: "workflows" }
  | { kind: "usage" }
  | { kind: "compare"; tag: string }
  | { kind: "profiles" }
  | { kind: "skills" }
  | { kind: "mcp" }
  | { kind: "import" }
  | { kind: "help"; topic?: string }
  | { kind: "none" };

// How a session's status reads, translated where it shows (GUI-048).
const STATUS_LABEL: Record<string, PlainKey> = {
  running: "status.running",
  waiting_permission: "status.waitingPermission",
  waiting_input: "status.waitingInput",
  queued: "status.queued",
  idle: "status.idle",
  error: "status.error",
  archived: "status.archived",
};

// statusText is a status in words, or as the engine gave it when the app has none.
const statusText = (status: string) => (Object.hasOwn(STATUS_LABEL, status) ? t(STATUS_LABEL[status]) : status);

// How many sessions of a collapsed project have a status, on its heading.
const STATUS_COUNT = {
  running: "status.count.running",
  waiting_permission: "status.count.waitingPermission",
  waiting_input: "status.count.waitingInput",
  queued: "status.count.queued",
  error: "status.count.error",
} as const;

const statusCount = (status: string, count: number) =>
  Object.hasOwn(STATUS_COUNT, status) ? t(STATUS_COUNT[status as keyof typeof STATUS_COUNT], { count }) : `${count} ${status}`;

// The statuses the sidebar's filter offers, the first showing every one.
const STATUS_CHOICES: [status: string, label: PlainKey][] = [
  ["", "sidebar.filter.anyStatus"],
  ["waiting_permission", "status.waitingPermission"],
  ["running", "status.running"],
  ["idle", "status.idle"],
  ["error", "status.error"],
];

const PANE_SHORTCUT = { left: "paneLeft", right: "paneRight", up: "paneUp", down: "paneDown" };

const COLLAPSED_KEY = "freeloop.collapsedProjects";
const GROUP_BY_KEY = "freeloop.groupBy";

type GroupBy = "project" | "date";

function readGroupBy(): GroupBy {
  try {
    return localStorage.getItem(GROUP_BY_KEY) === "date" ? "date" : "project";
  } catch {
    return "project";
  }
}

function readCollapsedProjects(): string[] {
  try {
    const list = JSON.parse(localStorage.getItem(COLLAPSED_KEY) ?? "[]");
    return Array.isArray(list) ? list.filter((k) => typeof k === "string") : [];
  } catch {
    return [];
  }
}

// A session's own window (GUI-015) shows that session alone; every window
// receives every engine event.
const STANDALONE = windowSession(window.location.search);

// Without project headings, a row says which project it belongs to, and a
// project that needs something still shows it: an untrusted one by the same
// ring as its heading, named in the tooltip.
function RowProject({ session, project }: { session: Session; project?: ProjectInfo }) {
  if (project?.missing) return <span className="badge badge-error">{t("sidebar.project.missing")}</span>;
  const name = project?.name ?? basename(session.projectRoot);
  const untrusted = project && !project.trusted;
  const root = project?.root ?? session.projectRoot;
  return (
    <span className="muted session-project" title={untrusted ? t("sidebar.project.untrustedRoot", { root }) : root}>
      {untrusted && <span className="untrusted" role="img" aria-label={t("sidebar.project.untrusted")} />}
      <span className="session-project-name">{name}</span>
    </span>
  );
}

// StatusDot is a session's status as a small dot before its title, named for the
// tooltip and screen readers; an idle session has none.
function StatusDot({ status }: { status: string }) {
  const label = statusText(status);
  if (status === "idle") return <span className="sr-only">{label}</span>;
  return <span className={`status-dot status-${status}`} role="img" aria-label={label} title={label} />;
}

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
  setBindings(config?.ui.shortcuts);
  const fontPending = useRef<number | null>(null);
  const fontsPending = useRef<FontChoice | null>(null);
  const [screen, setScreen] = useState<Screen>({ kind: "none" });
  const mainBox = useRef<HTMLElement>(null);
  const [heardScreen, setHeardScreen] = useState("");
  // Moving to another screen says so (NFR-A11Y-003): the window's title, an
  // announcement, and the focus in the new screen. A session opened from its
  // row keeps the focus there, so the next one is an arrow key or Tab away;
  // anything else would leave a screen reader where it no longer is.
  const screenId = screen.kind === "session" ? screen.id : "";
  const screenTitle = screenName(screen.kind, sessions.find((s) => s.id === screenId)?.title);
  useEffect(() => {
    document.title = screenTitle ? `${screenTitle} · Freeloop` : "Freeloop";
  }, [screenTitle]);
  const firstScreen = useRef(true);
  useEffect(() => {
    if (screen.kind === "none") return;
    if (firstScreen.current) {
      firstScreen.current = false;
      return;
    }
    setHeardScreen(screenTitle);
    const at = document.activeElement;
    const kept = screen.kind === "session" && at && at !== document.body;
    if (!kept && !mainBox.current?.contains(at)) mainBox.current?.focus({ preventScroll: true });
  }, [screen.kind, screenId]);
  const [transcripts, setTranscripts] = useState<Record<string, Transcript>>({});
  const [issues, setIssues] = useState<Issue[]>([]);
  // What waits for approval is not a problem, and may be long: Settings lists it.
  const problems = issues.filter((i) => !i.approval);
  const waitingApproval = issues.length - problems.length;
  // The last session opened, for the rules page's session scope.
  const [lastSessionId, setLastSessionId] = useState<string | null>(null);
  const [errors, setErrors] = useState<ShownError[]>([]);
  // Sessions that finished out of sight (SES-022); see the effects below.
  const [unread, setUnread] = useState<readonly string[]>(() => {
    try {
      return parseUnread(localStorage.getItem(UNREAD_KEY));
    } catch {
      return [];
    }
  });
  const [tabbedPanes, setTabbedPanes] = useState(false);
  const [windowFocused, setWindowFocused] = useState(() => document.hasFocus());
  const seenNow = useRef<string[]>([]);
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
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [collapsedProjects, setCollapsedProjects] = useState<string[]>(readCollapsedProjects);
  const [groupBy, setGroupBy] = useState<GroupBy>(readGroupBy);
  // Full-text results for the sidebar query (SES-007), for that query.
  const [found, setFound] = useState<{ query: string; results: SessionHits[] } | null>(null);
  // Sessions that need the user (GUI-002).
  const [attention, setAttention] = useState<AttentionItem[]>([]);
  // Trusted projects' workflow backlogs, for the Board (WFL-004), read while it shows.
  const [backlogs, setBacklogs] = useState<BacklogInfo[]>([]);
  const refreshBacklogs = () =>
    EngineService.Backlogs()
      .then((l) => setBacklogs(l ?? []))
      .catch(() => setBacklogs([]));
  // A project's AFK run in full (WFL-009), by its root, from the Board or a session.
  const [runFor, setRunFor] = useState<string | null>(null);
  useEffect(() => {
    if (screen.kind !== "board" && runFor === null) return;
    refreshBacklogs();
    const timer = setInterval(refreshBacklogs, 5000);
    return () => clearInterval(timer);
  }, [screen.kind, runFor]);
  // How many tasks wait in the queue, for the sidebar's icon (SES-025). The queue starts
  // its tasks every 30 seconds, so this looks as often as the Queue page does, slower.
  const [queueWaiting, setQueueWaiting] = useState(0);
  useEffect(() => {
    if (STANDALONE) return;
    const load = () =>
      EngineService.Queue()
        .then((q) => setQueueWaiting(((q as { tasks?: { state: string }[] }).tasks ?? []).filter((task) => task.state === "waiting").length))
        .catch(() => {});
    load();
    const every = setInterval(load, 5000);
    return () => clearInterval(every);
  }, []);
  // The AFK run's dialog (WFL-005), for a project's backlog.
  const [afkFor, setAfkFor] = useState<{ root: string; workflowId?: string; startedBy: string } | null>(null);
  const backlogActions = (b: BacklogInfo) => {
    const r = b.run;
    const going = r && (r.state === "running" || r.state === "paused");
    return (
      <div className="backlog-run">
        {r && (
          // A run whose issue waits says so plainly: until it starts, nothing else moves.
          <p className={waitingStep(r) ? "backlog-run-line backlog-run-waiting" : "muted backlog-run-line"} role="status">
            {runLine(r)}
          </p>
        )}
        <p className="row">
          {!going && (
            <button className="primary" onClick={() => setAfkFor({ root: b.projectRoot, workflowId: b.workflowId, startedBy: "the Board" })}>
              {t("app.afk.run")}
            </button>
          )}
          {going && (
            <button onClick={() => EngineService.PauseAFK(r.id, r.state === "running").then(refreshBacklogs).catch(report)}>{r.state === "running" ? t("app.afk.pause") : t("app.afk.resume")}</button>
          )}
          {going && (
            <button onClick={() => EngineService.StopAFK(r.id).then(refreshBacklogs).catch(report)}>
              {t("app.afk.stop")}
            </button>
          )}
          {r && (
            <button className="link backlog-run-details" onClick={() => setRunFor(b.projectRoot)}>
              {t("app.afk.showRun")}
            </button>
          )}
        </p>
      </div>
    );
  };
  // Trusting a project from its row on the Board, as from its session: the hooks its
  // settings would run are shown first, and trusting waits for the user's yes (SEC-006).
  const trustBacklog = async (b: BacklogInfo) => {
    try {
      const hooks = trustLines(await EngineService.ProjectSettings(b.projectRoot));
      if (hooks.length > 0 && !(await ask(t("trust.confirm", { name: b.projectName, list: hooks.join("\n") })))) return;
      await EngineService.SetProjectTrust(b.projectRoot, true);
      refreshProjects();
      refreshBacklogs();
    } catch (err) {
      report(t("trust.unreadable", { name: b.projectName, error: String(err) }));
    }
  };
  const archiveBacklog = async (b: BacklogInfo) => {
    if (!(await ask(t("app.afk.archiveAsk", { count: b.finished, project: b.projectName })))) return;
    EngineService.ArchiveBacklog(b.projectRoot, b.workflowId)
      .then(refreshBacklogs)
      .catch(report);
  };
  // Today's model calls and their estimated cost (PRV-008).
  const [today, setToday] = useState<DayCost | null>(null);
  // The budgets, for the sidebar's cost (PRV-017).
  const [budgets, setBudgets] = useState<BudgetState[]>([]);
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

  const report = (err: unknown, sessionTitle?: string) => setErrors((list) => addError(list, { detail: errorText(err), session: sessionTitle }));

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
  const refreshToday = () => {
    EngineService.Usage(1)
      .then((days) => setToday(days?.[0] ?? null))
      .catch(report);
    EngineService.Budgets()
      .then((list) => setBudgets(list ?? []))
      .catch(() => setBudgets([]));
  };
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

  // What each session is doing (GUI-030).
  const [activities, setActivities] = useState<Record<string, Activity>>({});
  const rootsOf = (s: Session) => [s.projectRoot, s.worktree].filter(Boolean) as string[];
  const ingest = (sid: string, events: readonly TranscriptEvent[]) => {
    setActivities((all) => {
      const before = all[sid] ?? NO_ACTIVITY;
      const after = events.reduce(foldActivity, before);
      return after === before ? all : { ...all, [sid]: after };
    });
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

  // The interface's language (GUI-048), from config.yaml or the system's. It is set
  // while this renders, before anything below it writes a word or a number, so a change
  // shows at once; the system's list can change while the app runs, and gives the
  // region numbers and dates are written for.
  const [systemLanguages, setSystemLanguages] = useState(preferredLanguages);
  useEffect(() => {
    const changed = () => setSystemLanguages(preferredLanguages());
    window.addEventListener("languagechange", changed);
    return () => window.removeEventListener("languagechange", changed);
  }, []);
  const language = resolveLanguage(config?.ui.language, systemLanguages);
  setLanguage(language, systemLanguages);
  useEffect(() => {
    document.documentElement.lang = language;
  }, [language]);

  // Appearance (GUI-011, GUI-025): theme, font size and fonts from config.yaml.
  useEffect(() => {
    if (!config) return;
    const root = document.documentElement;
    if (config.ui.theme === "system") delete root.dataset.theme;
    else root.dataset.theme = config.ui.theme;
    root.style.setProperty("--font-size", `${config.ui.fontSize || 14}px`);
    applyFonts(root.style, config.ui);
  }, [config?.ui.theme, config?.ui.fontSize, config?.ui.textFont, config?.ui.codeFont]);

  // The user's colors (GUI-022). A change shows at once and is saved a moment
  // later; until then it is newer than the file, so a reload leaves it alone.
  const [colors, setColors] = useState<ColorSet>({});
  const colorsPending = useRef(false);
  const colorSave = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => applyColors(colors), [colors]);
  const changeColors = (next: ColorSet) => {
    colorsPending.current = true;
    setColors(next);
    clearTimeout(colorSave.current);
    colorSave.current = setTimeout(() => {
      EngineService.SaveColors({ version: 1, light: next.light ?? {}, dark: next.dark ?? {} })
        .catch(report)
        .finally(() => {
          colorsPending.current = false;
        });
    }, 300);
  };

  // A config object only replaces the current one when it differs, so an
  // unrelated reload does not reset a half-edited settings form.
  const refreshConfig = () => {
    EngineService.Config()
      .then((disk) => {
        // A size chosen with ⌘+ or ⌘- and not saved yet is newer than the file.
        const c = fontPending.current === null ? disk : { ...disk, ui: { ...disk.ui, fontSize: fontPending.current } };
        if (fontsPending.current) c.ui = { ...c.ui, ...fontsPending.current };
        setConfig((old) => (old && JSON.stringify(old) === JSON.stringify(c) ? old : c));
      })
      .catch(report);
    EngineService.Colors()
      .then((disk) => {
        if (colorsPending.current) return;
        setColors((old) => (JSON.stringify([old.light, old.dark]) === JSON.stringify([disk.light, disk.dark]) ? old : (disk as ColorSet)));
      })
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
      if (finishedTurn(ev)) setUnread((list) => markFinished(list, ev.sid, seenNow.current));
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
        if (ev.type === "optimizer_changed") s.optimizer = d.optimizer ?? "";
        if (ev.type === "workflow_changed") s.workflow = (ev.data as { workflow?: string }).workflow ?? "";
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
    const pane = next.panes[next.active];
    const sid = ws.isSession(pane) ? pane : null;
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
        for (const sid of w.panes) if (ws.isSession(sid) && !transcriptsRef.current[sid]) catchUp(sid);
        const active = w.panes[w.active];
        const sid = ws.isSession(active) ? active : null;
        if (active || (w.layout > 1 && sessions.length > 0)) setScreen((sc) => (sc.kind === "none" ? { kind: "session", id: sid ?? "" } : sc));
      })
      .catch(() => {});
  }, [sessionsLoaded]);
  useEffect(() => {
    if (!workLoaded.current) return;
    const timer = setTimeout(() => {
      EngineService.SaveWorkspace({ layout: work.layout, panes: work.panes.map((x) => x ?? ""), active: work.active }).catch(() => {});
    }, 400);
    return () => clearTimeout(timer);
  }, [work]);
  useEffect(() => {
    if (sessionsLoaded) setWork((w) => ws.forget(w, (sid) => sessions.some((x) => x.id === sid)));
    if (sessionsLoaded) pruneDrafts((sid) => sessions.some((x) => x.id === sid));
  }, [sessions, sessionsLoaded]);

  // A pane chosen with the keyboard takes the keyboard: its composer.
  const focusActivePane = () => requestAnimationFrame(() => document.querySelector<HTMLElement>(".pane-active textarea")?.focus());

  // ⌥⌘1 to ⌥⌘4 make a pane active, ⌥⌘← and ⌥⌘→ move between panes. Outside a Mac
  // it is Alt and the digit or arrow: Ctrl+Alt is AltGr, which types characters on
  // many keyboards.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (STANDALONE || !e.altKey || e.shiftKey || otherKey(e) || (isMac ? !e.metaKey : e.ctrlKey)) return;
      const w = workRef.current;
      const digit = /^Digit([1-4])$/.exec(e.code);
      if (digit && Number(digit[1]) <= w.layout) {
        e.preventDefault();
        activatePaneRef.current(Number(digit[1]) - 1);
        focusActivePane();
      } else if ((e.code === "ArrowRight" || e.code === "ArrowLeft") && w.layout > 1) {
        e.preventDefault();
        showWorkRef.current(ws.cycle(w, e.code === "ArrowRight" ? 1 : -1));
        focusActivePane();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(clearSelectionOnClick, []);
  useEffect(() => {
    const panel = storedPanel();
    if (panel !== null) setPanelWidth(panel);
  }, []);

  // ⌃⇧ and the arrows (Alt+Shift outside a Mac) move between panes, as in tmux
  // (SET-006). Caught on the way down, since a terminal in focus keeps its keys, and
  // not while Settings records a key.
  const tabbedRef = useRef(false);
  tabbedRef.current = tabbedPanes;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (STANDALONE || workRef.current.layout === 1) return;
      if (e.target instanceof Element && e.target.closest(".shortcut-listening")) return;
      const direction = (["left", "right", "up", "down"] as const).find((d) => pressed(e, PANE_SHORTCUT[d]));
      if (!direction) return;
      e.preventDefault();
      e.stopPropagation();
      showWorkRef.current(ws.movePane(workRef.current, direction, tabbedRef.current));
      focusActivePane();
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, []);

  // ⌘1 to ⌘9 (Ctrl outside a Mac) open the sessions listed first, ⌘0 the attention
  // view (NFR-A11Y-001).
  const listedRef = useRef<Session[]>([]);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (STANDALONE) return;
      if (pressed(e, "attention")) {
        e.preventDefault();
        setScreen({ kind: "attention" });
        return;
      }
      if (!primaryOnly(e) || !/^[1-9]$/.test(e.key)) return;
      e.preventDefault();
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
  // The settings open where they were left (the page, not what was typed on it).
  const lastSettings = useRef<string | null>(null);
  if (inSettings(screen.kind) && screen.kind !== "import") lastSettings.current = screen.kind;
  const settingsScreen = () => ({ kind: settingsStart(lastSettings.current) }) as Screen;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (STANDALONE || !isSettingsShortcut(e)) return;
      e.preventDefault();
      setScreen((s) => (inSettings(s.kind) ? s : settingsScreen()));
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
      if (!pressed(e, "palette")) return;
      e.preventDefault();
      setPalette((open) => (open ? null : paletteRef.current()));
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // ⌘+ and ⌘- change the Font size setting from anywhere (NFR-A11Y-002). The
  // text changes at once; a burst of presses is saved as one change.
  const configNow = useRef(config);
  configNow.current = config;
  const fontSave = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const now = configNow.current;
      if (!now) return;
      const current = now.ui.fontSize || 14;
      const next = textSizeAfter(e, current);
      if (next === null) return;
      // Ahead of the View menu's page zoom, which would scale the layout too.
      e.preventDefault();
      if (next === current) {
        setHeardScreen(next === MAX_FONT_SIZE ? t("app.textSize.largest") : t("app.textSize.smallest"));
        return;
      }
      setHeardScreen(t("app.textSize.now", { size: next }));
      fontPending.current = next;
      setConfig({ ...now, ui: { ...now.ui, fontSize: next } });
      clearTimeout(fontSave.current);
      fontSave.current = setTimeout(() => {
        if (!configNow.current) return;
        EngineService.SaveSettings(configNow.current)
          .catch(report)
          .finally(() => {
            if (fontPending.current === next) fontPending.current = null;
          });
      }, 400);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // The fonts of Settings → Appearance (GUI-025) show at once and are saved a
  // moment later, as one change for a name being typed.
  const fontsSave = useRef<ReturnType<typeof setTimeout>>(undefined);
  const changeFonts = (next: FontChoice) => {
    const now = configNow.current;
    if (!now) return;
    fontsPending.current = next;
    setConfig({ ...now, ui: { ...now.ui, ...next } });
    clearTimeout(fontsSave.current);
    fontsSave.current = setTimeout(() => {
      if (!configNow.current) return;
      EngineService.SaveSettings(configNow.current)
        .catch(report)
        .finally(() => {
          if (fontsPending.current === next) fontsPending.current = null;
        });
    }, 300);
  };

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

  // The list prices come after the user's own (PRV-008).
  const [listPrices, setListPrices] = useState<PriceEntry[]>([]);
  useEffect(() => {
    EngineService.BuiltinPrices()
      .then((l) => setListPrices(l.prices ?? []))
      .catch(report);
  }, []);
  const prices = useMemo(() => [...(config?.prices ?? []), ...listPrices], [config?.prices, listPrices]);

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
  if (groupBy === "date") {
    for (const d of dateGroups(visible, new Date())) groups.push({ key: `date:${d.id}`, name: d.label, sessions: d.sessions });
  } else {
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
  }
  const chooseGroupBy = (next: GroupBy) => {
    setGroupBy(next);
    try {
      localStorage.setItem(GROUP_BY_KEY, next);
    } catch {
      // Only a convenience: it is back to projects next time.
    }
  };
  // A search or filter shows what it found, in collapsed projects too.
  const isCollapsed = (key: string) => isEmpty(filter) && !showArchived && collapsedProjects.includes(key);
  const toggleProject = (key: string) => {
    const next = collapsedProjects.includes(key) ? collapsedProjects.filter((k) => k !== key) : [...collapsedProjects, key];
    setCollapsedProjects(next);
    try {
      localStorage.setItem(COLLAPSED_KEY, JSON.stringify(next));
    } catch {
      // Only a convenience: it is back to open next time.
    }
  };
  const isIdleOrDone = (s: Session) => s.status === "idle" || s.status === "archived";
  // ⌘1 to ⌘9 go down the sessions that can be seen.
  listedRef.current = groups.filter((g) => !isCollapsed(g.key)).flatMap((g) => g.sessions);
  const filterChips = [
    ...(statusFilter ? [{ label: statusText(statusFilter), clear: () => setStatusFilter("") }] : []),
    ...tagFilter.map((tag) => ({ label: `#${tag}`, clear: () => setTagFilter(tagFilter.filter((x) => x !== tag)) })),
    ...(showArchived ? [{ label: t("sidebar.filter.archived"), clear: () => setShowArchived(false) }] : []),
  ];
  const openAttention = () => {
    refreshAttention();
    setScreen({ kind: "attention" });
  };

  // Each waiting request counts, and each session in error or waiting for a
  // reply without a request.
  const attentionCount = attention.reduce((n, it) => n + Math.max(it.requests?.length ?? 0, 1), 0);

  // The palette lists, in this order: the views' own actions (the open
  // session's first), the app's pages and actions, then every session.
  paletteRef.current = () => {
    const page = (id: string, title: string, go: () => void, extra: Partial<PaletteItem> = {}): PaletteItem => ({ id: `page:${id}`, title, group: "Page", run: go, ...extra });
    const items: PaletteItem[] = [
      ...collect(),
      { id: "app:new", title: t("app.newSession"), group: "Action", keywords: "create start", run: () => setScreen({ kind: "new" }) },
      {
        id: "app:workflow",
        title: t("app.palette.workflow"),
        group: "Action",
        keywords: "workflow plan prd slice afk issues backlog grill",
        detail: t("app.palette.workflowDetail"),
        run: () => setScreen({ kind: "new", workflow: config?.defaults.workflow || "plan-slice-afk" }),
      },
      ...projects
        .filter((p) => p.trusted && !p.missing)
        .map((p) => ({
          id: `app:afk:${p.id}`,
          title: t("app.palette.afk", { project: p.name }),
          group: "Action",
          keywords: "afk workflow backlog issues run unattended",
          detail: t("app.palette.afkDetail"),
          run: () => setAfkFor({ root: p.root, startedBy: "the palette" }),
        })),
      { id: "app:demo", title: t("app.palette.demo"), group: "Action", keywords: "tour example scripted no key policy", run: () => startDemo() },
      page("attention", t("app.needsAttention"), () => (refreshAttention(), setScreen({ kind: "attention" })), { shortcut: keyLabel(bindingOf("attention")), detail: attentionCount ? t("app.palette.waiting", { count: attentionCount }) : undefined }),
      ...(STANDALONE
        ? []
        : ([1, 2, 3, 4] as const).map((n) =>
            page(`panes-${n}`, n === 1 ? t("app.layout.onePane") : t("app.layout.panesSideBySide", { count: n }), () => showWork(ws.chooseLayout(workRef.current, n)), { keywords: "workspace layout split columns grid" }),
          )),
      page("board", t("app.board"), () => setScreen({ kind: "board" }), { keywords: "cards overview columns kanban" }),
      page("queue", t("app.queue"), () => setScreen({ kind: "queue" }), { keywords: "tasks later batch" }),
      page("providers", t("settings.page.providers"), () => setScreen({ kind: "providers" }), { keywords: "api keys models" }),
      page("profiles", t("settings.page.profiles"), () => setScreen({ kind: "profiles" }), { keywords: "harness agents" }),
      page("skills", t("settings.page.skills"), () => setScreen({ kind: "skills" })),
      page("mcp", t("settings.page.mcp"), () => setScreen({ kind: "mcp" }), { keywords: "tools" }),
      page("optimizers", t("settings.page.optimizers"), () => setScreen({ kind: "optimizers" }), { keywords: "rtk tokens compress output default" }),
      page("workflows", t("settings.page.workflows"), () => setScreen({ kind: "workflows" }), { keywords: "workflow prd slice afk backlog skills" }),
      page("schedules", t("settings.page.schedules"), () => setScreen({ kind: "schedules" }), { keywords: "scheduled recurring cron timer nightly" }),
      page("environments", t("settings.page.environments"), () => setScreen({ kind: "environments" }), { keywords: "docker secrets variables" }),
      page("rules", t("settings.page.rules"), () => setScreen({ kind: "rules" }), { keywords: "permission rules allow deny would this be allowed check" }),
      page("settings", t("settings.title"), () => setScreen(settingsScreen()), { keywords: "preferences" }),
      page("appearance", t("settings.page.appearance"), () => setScreen({ kind: "appearance" }), { keywords: "theme dark light font size colors provider marks" }),
      page("keyboard", t("app.palette.keyboard"), () => setScreen({ kind: "keyboard" }), { keywords: "keys bindings" }),
      page("sessions", t("app.palette.sessions"), () => setScreen({ kind: "sessions" }), { keywords: "default provider model environment profile notify mute" }),
      page("costs", t("app.palette.costs"), () => setScreen({ kind: "costs" }), { keywords: "costs price budget spending limit" }),
      page("permissions", t("settings.page.permissions"), () => setScreen({ kind: "permissions" }), { keywords: "bypass hooks approve approvals waiting" }),
      page("network", t("settings.page.network"), () => setScreen({ kind: "network" }), { keywords: "local only offline web search updates" }),
      page("usage-pane", t("app.palette.usagePane"), () => showWork(ws.openIn(workRef.current, ws.USAGE_PANE, "new")), { keywords: "live tokens cost graph 12 24 hours side by side" }),
      page("usage", t("app.usage"), () => setScreen({ kind: "usage" }), { keywords: "cost tokens analytics dashboard spend", detail: today && (today.models ?? []).some((m) => m.priced) ? t("app.palette.usageToday", { cost: formatCost(today.cost) }) : undefined }),
      page("import", t("settings.import.title"), () => setScreen({ kind: "import" })),
      page("import-sessions", t("app.palette.importSessions"), () => setScreen({ kind: "import" }), { keywords: "claude code agent transcript history resume" }),
      page("backup", t("app.palette.backup"), () => setScreen({ kind: "data" }), { keywords: "export import settings providers profiles logs data" }),
      page("guide", t("app.palette.guide"), () => setScreen({ kind: "onboarding" }), { keywords: "onboarding first run" }),
      page("help", t("app.help"), () => setScreen({ kind: "help" }), { keywords: "docs documentation guide manual how" }),
      ...SHOWN_TOPICS.map((topic) => ({ id: `help:${topic.slug}`, title: topic.title, group: "Help", keywords: "help docs guide how", run: () => setScreen({ kind: "help", topic: topic.slug }) })),
      page("report", t("settings.report.title"), () => setScreen({ kind: "data" }), { keywords: "bug issue copy report logs crash help" }),
      {
        id: "app:find",
        title: t("sidebar.find"),
        group: "Action",
        keywords: "search filter transcripts",
        run: () => searchInput.current?.focus(),
      },
      {
        id: "app:group-by",
        title: groupBy === "date" ? t("app.palette.groupByProject") : t("app.palette.groupByDate"),
        group: "Filter",
        run: () => chooseGroupBy(groupBy === "date" ? "project" : "date"),
      },
      {
        id: "app:archived",
        title: showArchived ? t("app.palette.showActive") : t("app.palette.showArchived"),
        group: "Filter",
        run: () => setShowArchived(!showArchived),
      },
      ...STATUS_CHOICES.map(([v, name]) => ({
        id: `filter:${v}`,
        title: t("app.palette.showStatus", { status: t(name) }),
        group: "Filter",
        detail: statusFilter === v ? t("app.palette.current") : undefined,
        run: () => setStatusFilter(v),
      })),
      ...tagsInUse.map((tag) => ({
        id: `tag:${tag}`,
        title: t("app.palette.showTag", { tag }),
        group: "Filter",
        detail: tagFilter.includes(tag) ? t("app.palette.current") : undefined,
        run: () => setTagFilter([tag]),
      })),
      ...[...new Set(sessions.flatMap((s) => (s.tags ?? []).filter((tag) => tag.startsWith("compare-"))))].map((tag) => ({
        id: `compare:${tag}`,
        title: t("app.palette.compare", { name: sessions.find((s) => (s.tags ?? []).includes(tag))?.title.replace(/ · [^·]*$/, "") ?? tag }),
        group: "Page",
        keywords: "compare profiles runs",
        run: () => setScreen({ kind: "compare", tag }),
      })),
      ...savedFilters.map((f) => ({
        id: `saved:${f.name}`,
        title: t("app.palette.savedFilter", { name: f.name }),
        group: "Filter",
        detail: sameFilter(f, filter) ? t("app.palette.current") : undefined,
        run: () => applyFilter(f),
      })),
      ...projects.map((p) => ({
        id: `new:${p.id}`,
        title: p.name,
        group: "New session",
        keywords: "new session start create",
        detail: p.root,
        run: () => setScreen({ kind: "new", root: p.root }),
      })),
      ...projects
        .filter((p) => p.missing)
        .map((p) => ({ id: `relink:${p.id}`, title: t("app.palette.relink", { project: p.name }), group: "Action", detail: p.root, keywords: "missing moved", run: () => relink(p) })),
      ...sessions.map((s) => {
        const listed = listedRef.current.indexOf(s);
        return {
          id: `session:${s.id}`,
          title: s.title,
          group: "Session",
          detail: `${projects.find((p) => p.id === s.projectId)?.name ?? basename(s.projectRoot)} · ${statusText(s.status)}`,
          keywords: `${s.branch ?? ""} ${s.status === "archived" ? "archived" : ""}`,
          shortcut: listed >= 0 && listed < 9 ? keyLabel(`Meta+${listed + 1}`) : undefined,
          run: () => openSession(s.id),
        };
      }),
    ];
    return items;
  };

  // What the user can see decides what is news, and seeing it ends the news.
  seenNow.current = STANDALONE
    ? selected && windowFocused ? [selected.id] : []
    : onScreen({ panes: work.panes, active: work.active, tabbed: tabbedPanes, showingPanes: screen.kind === "session", focused: windowFocused });
  const seenKey = seenNow.current.join(" ");
  useEffect(() => {
    setUnread((list) => markRead(list, seenNow.current));
  }, [seenKey]);
  useEffect(() => {
    try {
      localStorage.setItem(UNREAD_KEY, JSON.stringify(unread));
    } catch {
      // A convenience: without storage the marks last until the app quits.
    }
  }, [unread]);
  useEffect(() => {
    const focus = () => setWindowFocused(true);
    const blur = () => setWindowFocused(false);
    window.addEventListener("focus", focus);
    window.addEventListener("blur", blur);
    return () => {
      window.removeEventListener("focus", focus);
      window.removeEventListener("blur", blur);
    };
  }, []);

  // The demo (GUI-045) opens its session, which is new on the first call.
  const startDemo = () =>
    EngineService.StartDemo()
      .then((s) => {
        setSessions((list) => (list.some((x) => x.id === s.id) ? list : [s, ...list]));
        refreshProjects();
        openSession(s.id);
      })
      .catch(report);

  // Where a help page's buttons lead.
  const openPlace = (place: HelpPlace) => (place === "demo" ? startDemo() : setScreen({ kind: place }));

  const sessionDeleted = (sid: string) => {
    forgetDraft(sid);
    if (STANDALONE) {
      Window.Close().catch(() => {});
      return;
    }
    // Only a session on screen changes what is shown.
    if (workRef.current.panes.includes(sid)) {
      if (workRef.current.layout > 1) showWork(ws.forget(workRef.current, (x) => x !== sid));
      else setScreen({ kind: "none" });
    }
    refreshSessions();
  };
  // A session row's right-click menu, and renaming in the row.
  const [sidebarWidth, setSidebarWidth] = useState<number | null>(storedSidebar);
  const dragSidebar = (px: number) => {
    const next = clampSidebar(px, window.innerWidth);
    setSidebarWidth(next);
    rememberSidebar(next);
  };
  const [rowMenu, setRowMenu] = useState<{ session: Session; x: number; y: number } | null>(null);
  const [renamingRow, setRenamingRow] = useState<{ id: string; title: string } | null>(null);
  // Escape takes the field away, and losing focus must not then save what was typed.
  const renameCancelled = useRef(false);
  const finishRowRename = () => {
    if (!renamingRow || renameCancelled.current) return;
    const { id, title } = renamingRow;
    setRenamingRow(null);
    const was = sessions.find((x) => x.id === id)?.title;
    if (title.trim() && title.trim() !== was) EngineService.RenameSession(id, title.trim()).catch(report);
  };
  const rowMenuItems = (s: Session): MenuItem[] => {
    const archived = s.status === "archived";
    return [
      { label: t("sidebar.menu.rename"), run: () => setRenamingRow({ id: s.id, title: s.title }) },
      { label: t("sidebar.menu.openBeside"), run: () => openSession(s.id, undefined, "new") },
      { label: t("sidebar.menu.openWindow"), run: () => openWindow(s.id) },
      { label: archived ? t("sidebar.menu.unarchive") : t("sidebar.menu.archive"), run: () => EngineService.ArchiveSession(s.id, !archived).catch(report), apart: true },
      {
        label: t("sidebar.menu.delete"),
        danger: true,
        apart: true,
        run: async () => {
          if (!(await ask(t("sidebar.menu.deleteAsk", { title: s.title })))) return;
          EngineService.DeleteSession(s.id)
            .then(() => sessionDeleted(s.id))
            .catch(report);
        },
      },
    ];
  };

  // The statuses in words, for the views that show them (the panes, the Board).
  const statusLabels: Record<string, string> = Object.fromEntries(Object.keys(STATUS_LABEL).map((status) => [status, statusText(status)]));

  // What each session runs on, shown as a mark wherever it is (GUI-034).
  const sourceOf = useMemo(
    () => sourceIndex(providers, harnesses, sessions, config?.ui.sourceMarks as SourceChoices),
    [providers, harnesses, config?.ui.sourceMarks, sessions.map((s) => sourceId(s)).join()],
  );
  const sessionView = (s: Session, active = true, onClosePane?: () => void, onPaneDrag?: (e: DragEvent) => void) => (
    <SessionView
      key={s.id}
      session={s}
      focus={screen.kind === "session" && screen.id === s.id ? screen.focus : undefined}
      initialDraft={forkDraft?.sid === s.id ? forkDraft.text : undefined}
      profiles={profiles}
      onForked={(sid, text) => {
        setForkDraft({ sid, text });
        refreshSessions();
        openSession(sid);
      }}
      prices={prices}
      onRunAFK={(root, workflowId) => setAfkFor({ root, workflowId, startedBy: "/afk" })}
      onShowRun={(root) => setRunFor(root)}
      knownTags={tagsInUse}
      onOpenCompare={(tag) => setScreen({ kind: "compare", tag })}
      events={transcripts[s.id]?.events ?? []}
      provider={providers.find((p) => p.id === s.providerId)}
      providers={providers}
      environment={environments.find((x) => x.id === (s.environmentId || "local"))}
      harness={harnesses.find((h) => h.id === s.harnessId)}
      project={projects.find((p) => p.id === s.projectId)}
      onProjectsChanged={refreshProjects}
      configVersion={configVersion}
      allowBypass={config?.permissions.allowBypass ?? false}
      onError={(err) => report(err, s.title)}
      onOpenWindow={STANDALONE ? undefined : () => openWindow(s.id)}
      active={active}
      onClosePane={onClosePane}
      onPaneDrag={onPaneDrag}
      source={sourceOf(s)}
      onDeleted={() => sessionDeleted(s.id)}
    />
  );

  return (
    <PaletteContext.Provider value={registry}>
    <div className={STANDALONE ? "app standalone" : "app"} style={sidebarWidth ? ({ "--sidebar-width": `${sidebarWidth}px` } as CSSProperties) : undefined}>
      <div className="sr-only" role="status" aria-live="polite">
        {heardScreen}
      </div>
      {!STANDALONE && (
        <button className="skip-link" onClick={() => mainBox.current?.focus()}>
          {t("app.skip")}
        </button>
      )}
      {!STANDALONE && (
        <aside className="sidebar">
          <header>
            <h1>
              <Logo />
              Freeloop
            </h1>
            <button className="icon-button new-session" aria-label={t("app.newSession")} title={t("app.newSession")} onClick={() => setScreen({ kind: "new" })}>
              <NewSessionIcon />
            </button>
          </header>
          {attentionCount > 0 && (
            <button
              className="attention-link attention-link-on"
              aria-current={screen.kind === "attention" ? "page" : undefined}
              aria-keyshortcuts={ariaKeys(bindingOf("attention"))}
              title={keyLabel(bindingOf("attention"))}
              onClick={openAttention}
            >
              {t("app.needsAttention")}
              <span className="badge badge-waiting_permission">{attentionCount}</span>
            </button>
          )}
          <div className="filters">
            <div className="filter-search">
              <input ref={searchInput} type="search" placeholder={t("sidebar.find")} aria-label={t("sidebar.find")} value={query} onChange={(e) => setQuery(e.target.value)} />
              <button
                className="ghost filter-toggle"
                aria-expanded={filtersOpen}
                aria-label={filterChips.length > 0 ? t("sidebar.filters.count", { count: filterChips.length }) : t("sidebar.filters")}
                title={t("sidebar.filters.title")}
                onClick={() => setFiltersOpen(!filtersOpen)}
              >
                <FilterIcon />
                {filterChips.length > 0 && <span aria-hidden="true">{filterChips.length}</span>}
              </button>
            </div>
            {!filtersOpen && filterChips.length > 0 && (
              <div className="saved-filters" role="group" aria-label={t("sidebar.filters.on")}>
                {filterChips.map((c) => (
                  <span key={c.label} className="saved-filter saved-filter-on">
                    <span className="saved-filter-name">{c.label}</span>
                    <button className="saved-filter-remove" aria-label={t("sidebar.filter.remove", { label: c.label })} onClick={c.clear}>
                      ×
                    </button>
                  </span>
                ))}
              </div>
            )}
            {filtersOpen && (
              <label className="row muted group-by">
                {t("sidebar.groupBy")}
                <select value={groupBy} onChange={(e) => chooseGroupBy(e.target.value as GroupBy)}>
                  <option value="project">{t("sidebar.groupBy.project")}</option>
                  <option value="date">{t("sidebar.groupBy.date")}</option>
                </select>
              </label>
            )}
            {filtersOpen && (
              <div className="row">
                <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} aria-label={t("sidebar.filter.status")}>
                  {STATUS_CHOICES.map(([v, name]) => (
                    <option key={v} value={v}>
                      {t(name)}
                    </option>
                  ))}
                </select>
                {tagsInUse.length > 0 && (
                  <select value={tagFilter[0] ?? ""} onChange={(e) => setTagFilter(e.target.value ? [e.target.value] : [])} aria-label={t("sidebar.filter.tag")}>
                    <option value="">{t("sidebar.filter.anyTag")}</option>
                    {tagsInUse.map((tag) => (
                      <option key={tag} value={tag}>
                        #{tag}
                      </option>
                    ))}
                  </select>
                )}
                <label className="row muted">
                  <input type="checkbox" checked={showArchived} onChange={(e) => setShowArchived(e.target.checked)} /> {t("sidebar.filter.archived")}
                </label>
              </div>
            )}
            {filtersOpen && (savedFilters.length > 0 || !isEmpty(filter)) && (
              <div className="saved-filters" role="group" aria-label={t("sidebar.savedFilters")}>
                {savedFilters.map((f) => (
                  <span key={f.name} className={`saved-filter ${sameFilter(f, filter) ? "saved-filter-on" : ""}`}>
                    <button
                      className="saved-filter-name"
                      aria-pressed={sameFilter(f, filter)}
                      title={[f.query, f.status && statusText(f.status), ...(f.tags ?? []).map((tag) => `#${tag}`)].filter(Boolean).join(" · ")}
                      onClick={() => (sameFilter(f, filter) ? applyFilter({ query: "", status: "", tags: [] }) : applyFilter(f))}
                    >
                      {f.name}
                    </button>
                    <button className="saved-filter-remove" aria-label={t("sidebar.savedFilter.remove", { name: f.name })} onClick={() => saveFilters(savedFilters.filter((x) => x !== f))}>
                      ×
                    </button>
                  </span>
                ))}
                {!isEmpty(filter) && !savedFilters.some((f) => sameFilter(f, filter)) && filterName === null && (
                  <button className="link" onClick={() => setFilterName("")}>
                    {t("sidebar.savedFilter.save")}
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
                      aria-label={t("sidebar.savedFilter.name")}
                      placeholder={t("sidebar.savedFilter.namePlaceholder")}
                      value={filterName}
                      maxLength={40}
                      onChange={(e) => setFilterName(e.target.value)}
                      onKeyDown={(e) => e.key === "Escape" && setFilterName(null)}
                    />
                    <button type="submit" disabled={!filterName.trim()}>
                      {t("common.save")}
                    </button>
                  </form>
                )}
              </div>
            )}
          </div>
          <nav aria-label={t("sidebar.nav")}>
            {groups.map((g) => (
              <section key={g.key} className="project-group">
                <h2 className="project-name" title={g.project?.root ?? (groupBy === "date" ? undefined : g.key)}>
                  <button className="project-toggle" aria-expanded={!isCollapsed(g.key)} onClick={() => toggleProject(g.key)}>
                    <ChevronIcon />
                    <span>{g.name}</span>
                  </button>
                  {isCollapsed(g.key) &&
                    statusSummary(sessions.filter((s) => !isIdleOrDone(s) && g.sessions.some((x) => x.id === s.id || x.id === s.parentId))).map((c) => (
                      <span key={c.status} className={`badge badge-${c.status}`}>
                        {statusCount(c.status, c.count)}
                      </span>
                    ))}
                  {g.project && !g.project.trusted && !g.project.missing && (
                    <span className="untrusted" role="img" aria-label={t("sidebar.project.untrustedLong")} title={t("sidebar.project.untrustedLong")} />
                  )}
                  {g.project?.missing && (
                    <>
                      <span className="badge badge-error">{t("sidebar.project.missing")}</span>
                      <button className="link" onClick={() => relink(g.project!)}>
                        {t("sidebar.project.relink")}
                      </button>
                    </>
                  )}
                </h2>
                <ul hidden={isCollapsed(g.key)}>
                  {g.sessions.map((s) => (
                    <li key={s.id}>
                      {renamingRow?.id === s.id ? (
                        <form
                          className="session-rename"
                          onSubmit={(e) => {
                            e.preventDefault();
                            finishRowRename();
                          }}
                        >
                          <input
                            autoFocus
                            aria-label={t("sidebar.row.rename", { title: s.title })}
                            value={renamingRow.title}
                            onFocus={(e) => {
                              renameCancelled.current = false;
                              e.currentTarget.select();
                            }}
                            onChange={(e) => setRenamingRow({ id: s.id, title: e.target.value })}
                            onBlur={finishRowRename}
                            onKeyDown={(e) => {
                              if (e.key !== "Escape") return;
                              renameCancelled.current = true;
                              setRenamingRow(null);
                            }}
                          />
                        </form>
                      ) : (
                      <button
                        className="session-row"
                        aria-haspopup="menu"
                        onContextMenu={(e) => {
                          e.preventDefault();
                          // From the keyboard there is no pointer to open at.
                          const at = e.currentTarget.getBoundingClientRect();
                          const keyboard = e.clientX === 0 && e.clientY === 0;
                          setRowMenu({ session: s, x: keyboard ? at.left + 12 : e.clientX, y: keyboard ? at.bottom : e.clientY });
                        }}
                        aria-current={screen.kind === "session" && screen.id === s.id ? "true" : undefined}
                        aria-keyshortcuts={listedRef.current.indexOf(s) < 9 ? `Meta+${listedRef.current.indexOf(s) + 1}` : undefined}
                        data-shown={!STANDALONE && screen.kind === "session" && work.panes.includes(s.id) ? "" : undefined}
                        title={[describeActivity(s.status, activities[s.id], rootsOf(s), new Date()), s.branch, (s.tags ?? []).map((tag) => `#${tag}`).join(" "), t("sidebar.row.openBeside", { click: isMac ? t("sidebar.row.click.mac") : t("sidebar.row.click.other") })].filter(Boolean).join(" · ")}
                        draggable
                        onDragStart={(e) => {
                          e.dataTransfer.setData(SESSION_DRAG_TYPE, s.id);
                          e.dataTransfer.effectAllowed = "move";
                        }}
                        onClick={(e) => openSession(s.id, undefined, primaryKey(e) ? "new" : "active")}
                      >
                        <span className="session-line">
                          <SourceMark source={sourceOf(s)} detail={s.model || undefined} />
                          <StatusDot status={s.status} />
                          {unread.includes(s.id) && <span className="unread-dot" role="img" aria-label={t("status.unread")} title={t("sidebar.row.unreadTitle")} />}
                          <span className="session-title">{s.title}</span>
                          {s.branch && <GitMark branch={s.branch} />}
                          {contextHigh(activities[s.id]?.context) && (
                            <span className="context-meter context-high" title={contextTitle(activities[s.id].context!)}>
                              {contextLabel(activities[s.id].context)}
                            </span>
                          )}
                          {groupBy === "date" && <RowProject session={s} project={projects.find((p) => p.id === s.projectId)} />}
                        </span>
                        {/* Tags are in the tooltip, the filter menu and the palette, not on a line of their own. */}
                        {(s.tags ?? []).length > 0 && <span className="sr-only">{(s.tags ?? []).map((tag) => `#${tag}`).join(" ")}</span>}
                      </button>
                      )}
                      {subagents(s.id).length > 0 && (
                        <ul className="subagents" aria-label={t("sidebar.row.subagents", { title: s.title })}>
                          {subagents(s.id).map((c) => (
                            <li key={c.id}>
                              <button
                                className="session-row session-child"
                                aria-current={screen.kind === "session" && screen.id === c.id ? "true" : undefined}
                                onClick={() => openSession(c.id)}
                              >
                                <SourceMark source={sourceOf(c)} detail={c.model || undefined} />
                                <StatusDot status={c.status} />
                                <span className="session-title">{c.title.replace(/^Subagent: /, "")}</span>
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
            {sessions.length === 0 && <p className="muted">{t("sidebar.empty.none")}</p>}
            {sessions.length > 0 && visible.length === 0 && !found?.results.length && <p className="muted">{t("sidebar.empty.noMatch")}</p>}
            {found && found.query === query.trim() && found.results.length > 0 && (
              <section className="project-group search-results" aria-label={t("search.found")}>
                <h2 className="project-name">{t("search.heading")}</h2>
                <ul>
                  {found.results.map((r) => {
                    const s = sessions.find((x) => x.id === r.sessionId);
                    if (!s) return null;
                    return (
                      <li key={r.sessionId}>
                        <span className="session-title search-session" title={s.projectRoot}>
                          {s.title}
                          {s.status === "archived" && <span className="muted"> · {t("status.archived")}</span>}
                        </span>
                        <ul>
                          {(r.hits ?? []).map((h, i) => (
                            <li key={i}>
                              <button className="search-hit" onClick={() => openSession(s.id, h.seq)}>
                                <span className="muted search-kind">{hitLabel(h.kind)}</span>
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
            {t("sidebar.update", { version: update.latest ?? "" })}
          </button>
        )}
        {config?.network?.localOnly && (
            <button className="link today" title={t("sidebar.localOnly.title")} onClick={() => setScreen({ kind: "network" })}>
              {t("settings.localOnly.title")}
            </button>
          )}
          <footer>
            <div className="footer-links">
              {/* With something waiting, the attention link above takes the bell's place; its
                  cell stays, so the icons below remain in columns under these. */}
              {attentionCount > 0 && <span className="footer-gap" aria-hidden="true" />}
              {attentionCount === 0 && (
                <button
                  aria-current={screen.kind === "attention" ? "page" : undefined}
                  aria-keyshortcuts={ariaKeys(bindingOf("attention"))}
                  title={bindingOf("attention") ? t("sidebar.attention.key", { key: keyLabel(bindingOf("attention")) }) : t("sidebar.attention")}
                  onClick={openAttention}
                  className="icon-button"
                  aria-label={t("sidebar.attention")}
                >
                  <BellIcon />
                </button>
              )}
              <button className="icon-button" aria-label={t("app.board")} title={t("app.board")} aria-current={screen.kind === "board" ? "page" : undefined} onClick={() => setScreen({ kind: "board" })}>
                <BoardIcon />
              </button>
              <button
                className="icon-button footer-queue"
                aria-label={queueWaiting > 0 ? t("sidebar.queue.waiting", { count: queueWaiting }) : t("app.queue")}
                title={t("sidebar.queue.title")}
                aria-current={screen.kind === "queue" ? "page" : undefined}
                onClick={() => setScreen({ kind: "queue" })}
              >
                <QueueIcon />
                {queueWaiting > 0 && (
                  <span className="footer-badge" aria-hidden="true">
                    {queueWaiting}
                  </span>
                )}
              </button>
              <button
                aria-current={screen.kind === "usage" ? "page" : undefined}
                className="icon-button"
                aria-label={t("app.usage")}
                title={t("sidebar.usage.title")}
                draggable={!STANDALONE}
                onDragStart={(e) => {
                  e.dataTransfer.setData(SESSION_DRAG_TYPE, ws.USAGE_PANE);
                  e.dataTransfer.effectAllowed = "move";
                }}
                onClick={() => setScreen({ kind: "usage" })}
              >
                <UsageIcon />
              </button>
              <button className="icon-button" aria-label={t("app.help")} title={t("sidebar.help.title")} aria-current={screen.kind === "help" ? "page" : undefined} onClick={() => setScreen({ kind: "help" })}>
                <BookIcon />
              </button>
              {/* Shown once anything is priced today, and whenever a budget is near or reached (PRV-017). */}
              {((today && (today.models ?? []).some((m) => m.priced)) || worst(budgets) !== "") && (
                <button
                  className={`footer-cost${worst(budgets) ? ` footer-cost-${worst(budgets)}` : ""}`}
                  title={[
                    t("sidebar.cost.title"),
                    ...budgets.filter((b) => b.level).map((b) => `${budgetName(b)}: ${budgetLine(b)}`),
                  ].join("\n")}
                  onClick={() => setScreen({ kind: "usage" })}
                >
                  {`≈ ${formatCost(today?.cost ?? 0)}${today && ((today.models ?? []).some((m) => !m.priced) || (today.unreported ?? 0) > 0) ? "+" : ""}`}
                </button>
              )}
            </div>
            <div className="footer-row">
              <div className="layout-switch" role="group" aria-label={t("sidebar.layout")}>
                {([1, 2, 3, 4] as const).map((n) => (
                  <button
                    key={n}
                    className="ghost"
                    aria-pressed={work.layout === n}
                    aria-label={n === 1 ? t("app.layout.onePane") : t("sidebar.layout.panes", { count: n })}
                    title={n === 1 ? t("sidebar.layout.oneSession") : t("sidebar.layout.sessions", { count: n })}
                    onClick={() => showWork(ws.chooseLayout(work, n))}
                  >
                    <Icon name={`panes-${n}`} />
                  </button>
                ))}
              </div>
              {/* Alone at the lower right, under the cost, so it is always where the eye
                  looks for it, and the four layout icons stay under the four above. */}
              <button
                className="settings-link icon-button"
                aria-label={t("settings.title")}
                aria-current={inSettings(screen.kind) ? "page" : undefined}
                aria-keyshortcuts={ariaKeys(bindingOf("settings"))}
                title={bindingOf("settings") ? t("sidebar.settings.key", { key: keyLabel(bindingOf("settings")) }) : t("settings.title")}
                onClick={() => (inSettings(screen.kind) ? leaveSettings() : setScreen(settingsScreen()))}
              >
                <Icon name="general" />
              </button>
            </div>
          </footer>
          <Splitter
            className="splitter-sidebar"
            label={t("sidebar.width")}
            value={sidebarWidth ?? 264}
            min={SIDEBAR_MIN}
            max={sidebarMax(window.innerWidth)}
            step={16}
            fromPointer={(x) => x}
            onChange={dragSidebar}
            onReset={() => {
              setSidebarWidth(null);
              rememberSidebar(null);
            }}
          />
        </aside>
      )}
      {rowMenu && <ContextMenu x={rowMenu.x} y={rowMenu.y} label={t("sidebar.menu.label", { title: rowMenu.session.title })} items={rowMenuItems(rowMenu.session)} onClose={() => setRowMenu(null)} />}

      <main className="main" ref={mainBox} tabIndex={-1}>
        {waitingApproval > 0 && (
          <div role="status" className="banner warn row">
            <span>{t("app.approvals.waiting", { count: waitingApproval })}</span>
            <button
              type="button"
              onClick={() => {
                setScreen({ kind: "permissions" });
                requestAnimationFrame(() => document.getElementById("pending-approvals")?.scrollIntoView({ block: "start" }));
              }}
            >
              {t("app.approvals.review")}
            </button>
          </div>
        )}
        {problems.length > 0 && (
          <div role="alert" className="banner warn">
            <strong>{t("app.problems")}</strong>
            <ul>
              {problems.map((i, n) => (
                <li key={n}>
                  {i.file}
                  {i.line ? `:${i.line}` : ""}
                  {i.field ? ` (${i.field})` : ""}: {i.message}
                </li>
              ))}
            </ul>
          </div>
        )}
        {errors.map((e) => (
          <div key={`${e.session ?? ""}\n${e.detail}`} role="alert" className="banner error">
            <strong>{errorHeading(e)}</strong> {e.detail} <button onClick={() => setErrors((list) => list.filter((l) => l !== e))}>{t("app.dismiss")}</button>
          </div>
        ))}

        {STANDALONE && selected && screen.kind === "session" && sessionView(selected)}
        {!STANDALONE && screen.kind === "session" && (
          <Workspace
            work={work}
            sessions={sessions}
            renderSession={sessionView}
            sourceOf={sourceOf}
            activityOf={(s) => describeActivity(s.status, activities[s.id], rootsOf(s), new Date())}
            unread={unread}
            statusLabel={statusLabels}
            onTabbed={setTabbedPanes}
            projects={projects}
            providers={providers}
            harnesses={harnesses}
            onActivate={activatePane}
            onPlace={(sid, i) => showWork(ws.placeIn(workRef.current, sid, i))}
            onSwap={(from, to) => showWork(ws.swapPanes(workRef.current, from, to))}
            onClose={(i) => showWork(ws.closePane(workRef.current, i))}
            onNew={() => setScreen({ kind: "new" })}
            onUsagePage={() => setScreen({ kind: "usage" })}
            onError={report}
          />
        )}
        {screen.kind === "new" && (
          <NewSession
            key={`${screen.root ?? ""}|${screen.workflow ?? ""}`}
            providers={providers}
            projects={projects}
            environments={environments}
            profiles={profiles}
            harnesses={harnesses}
            sessions={sessions}
            defaults={config?.defaults}
            initialRoot={screen.root}
            initialWorkflow={screen.workflow}
            onManageProviders={() => setScreen({ kind: "providers" })}
            onProvidersChanged={refreshProviders}
            onProjectsChanged={refreshProjects}
            onImport={() => setScreen({ kind: "import" })}
            onCompare={(tag) => {
              refreshSessions();
              setScreen({ kind: "compare", tag });
            }}
            onCreated={(s, draft) => {
              setSessions((list) => (list.some((x) => x.id === s.id) ? list : [s, ...list]));
              if (draft) setForkDraft({ sid: s.id, text: draft });
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
            onDemo={(sid) => {
              refreshSessions();
              openSession(sid);
            }}
            onDone={(root) => setScreen({ kind: "new", root })}
            onError={report}
          />
        )}
        {screen.kind === "help" && <Help topic={screen.topic ?? ""} onTopic={(topic) => setScreen({ kind: "help", topic })} onOpen={openPlace} />}
        {inSettings(screen.kind) && (
          <div className="settings-area">
            <SettingsNav current={screen.kind} onOpen={(kind) => setScreen({ kind } as Screen)} onBack={leaveSettings} counts={{ permissions: waitingApproval }} />
            <div className="settings-content">
              {screen.kind === "providers" && (
                <Providers
                  providers={providers}
                  onChanged={() => (refreshProviders(), refreshConfig())}
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
              {isGeneral(screen.kind) && config && (
                <Settings
                  page={screen.kind}
                  config={config}
                  providers={providers}
                  harnesses={harnesses}
                  environments={environments}
                  profiles={profiles}
                  projects={projects}
                  onProjectsChanged={refreshProjects}
                  onImport={() => setScreen({ kind: "import" })}
                  onProviders={() => setScreen({ kind: "providers" })}
                  colors={colors}
                  onColors={changeColors}
                  onFonts={changeFonts}
                  reportSession={sessions.find((s) => s.id === lastSessionId) ?? null}
                onSaved={setConfig} onError={report} />
              )}
              {screen.kind === "environments" && <Environments environments={environments} onChanged={refreshEnvironments} onError={report} />}
              {screen.kind === "optimizers" && config && <Optimizers config={config} version={configVersion} onSaved={setConfig} onError={report} />}
              {screen.kind === "workflows" && config && <Workflows config={config} version={configVersion} onSaved={setConfig} onError={report} />}
              {screen.kind === "schedules" && (
                <Schedules projects={projects} profiles={profiles} providers={providers} harnesses={harnesses} defaults={config?.defaults} onOpenSession={(id) => openSession(id)} onError={report} />
              )}
              {screen.kind === "rules" && <Rules session={sessions.find((s) => s.id === lastSessionId) ?? null} projects={projects} version={configVersion} onError={report} />}
              {screen.kind === "mcp" && <MCP configVersion={configVersion} onImport={() => setScreen({ kind: "import" })} onError={report} />}
              {screen.kind === "import" && (
                <section className="page">
                  <h2>{t("app.import.title")}</h2>
                  <section>
                    <h3>{t("app.import.sessions")}</h3>
                    <SessionImport profiles={profiles} projects={projects} onOpen={(sid) => openSession(sid)} onImported={() => { refreshSessions(); refreshProjects(); }} onError={report} />
                  </section>
                  <h3>{t("app.import.settings")}</h3>
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
        {screen.kind === "compare" && <Compare key={screen.tag} tag={screen.tag} onOpen={(id) => openSession(id)} onError={report} />}
        {screen.kind === "usage" && (
          <Usage onShowInPane={STANDALONE ? undefined : () => showWork(ws.openIn(workRef.current, ws.USAGE_PANE, "new"))} projects={projects} providers={providers} harnesses={harnesses} onOpenSession={(id) => openSession(id)} onOpenPrices={() => {
            setScreen({ kind: "costs" });
            requestAnimationFrame(() => document.getElementById("prices")?.scrollIntoView({ block: "start" }));
          }} onError={report} />
        )}
        {screen.kind === "queue" && <Queue projects={projects} profiles={profiles} onOpen={openSession} onError={report} />}
        {screen.kind === "board" && (
          <Board
            backlogs={backlogs}
            waiting={Object.fromEntries(attention.map((it) => [it.session.id, it.requests?.length ?? 0]))}
            renderRequest={(sid) => {
              const req = attention.find((it) => it.session.id === sid)?.requests?.[0];
              return req ? <RequestView key={req.requestId} req={req} onError={report} /> : null;
            }}
            qa={Object.fromEntries(attention.filter((it) => it.qa).map((it) => [`${it.qa!.runId}:${it.qa!.issue}`, { reply: it.qa!.reply }]))}
            onQADone={(runId, issue) =>
              EngineService.QADone(runId, issue)
                .then(() => {
                  refreshAttention();
                  refreshBacklogs();
                })
                .catch(report)
            }
            onError={report}
            onTrust={trustBacklog}
            onStartWorkflow={() => setScreen({ kind: "new", workflow: config?.defaults.workflow || "plan-slice-afk" })}
            onHelp={() => setScreen({ kind: "help", topic: "workflows" })}
            onArchive={archiveBacklog}
            backlogActions={backlogActions}
            onOpen={(id) => openSession(id)}
          />
        )}
        {screen.kind === "attention" && (
          <Attention
            items={attention}
            onOpen={openSession}
            onContinued={(sid, text) => {
              setForkDraft({ sid, text });
              refreshSessions();
              openSession(sid);
            }}
            onChanged={refreshAttention}
            onError={report}
          />
        )}
        {STANDALONE && screen.kind === "session" && !selected && sessionsLoaded && <div className="empty">{t("app.empty.gone")}</div>}
        {!STANDALONE && screen.kind === "none" && (
          <div className="empty">
            <p>{t("app.empty.select")}</p>
            <button className="link" onClick={() => setScreen({ kind: "onboarding" })}>
              {t("app.empty.guide")}
            </button>
          </div>
        )}
      </main>
      {palette && <Palette items={palette} onClose={() => setPalette(null)} />}
      {runFor !== null &&
        (() => {
          const b = backlogs.find((x) => x.projectRoot === runFor && x.run);
          return b ? (
            <AFKRunView
              backlog={b}
              attention={attention}
              onOpen={(sid) => {
                setRunFor(null);
                openSession(sid);
              }}
              onClose={() => setRunFor(null)}
              onChanged={() => {
                refreshBacklogs();
                refreshAttention();
              }}
              onError={report}
            />
          ) : null;
        })()}
      {afkFor && (
        <AFKDialog
          root={afkFor.root}
          workflowId={afkFor.workflowId}
          profiles={profiles}
          startedBy={afkFor.startedBy}
          onClose={() => setAfkFor(null)}
          onStarted={() => {
            setAfkFor(null);
            refreshBacklogs();
            refreshSessions();
            // The run is followed on the Board, so show it there, whatever started it.
            setScreen({ kind: "board" });
          }}
          onError={report}
        />
      )}
    </div>
    </PaletteContext.Provider>
  );
}
