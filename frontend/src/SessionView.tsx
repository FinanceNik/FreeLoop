import { Browser, Clipboard } from "@wailsio/runtime";
import { inFileManager, primaryKey, showInFiles } from "./platform.ts";
import { nameStart } from "./paths.ts";
import { ask as askUser } from "./ask.ts";
import { isDemo } from "./demo.ts";
import { memo, useEffect, useLayoutEffect, useMemo, useRef, useState, type DragEvent, type KeyboardEvent } from "react";
import { insertMention, mentionAt, rankFiles, type Mention } from "./mentions.ts";
import { matchCommands, MODE_COMMANDS, modeCommand, slashQuery, type SlashCommand } from "./commands.ts";
import { stageCall, stageCommands, withoutStageSkills } from "./workflows.ts";
import WorkflowStrip from "./WorkflowStrip.tsx";
import {
  EngineService,
  MODES,
  type EnvironmentInfo,
  type OptimizerInfo,
  type AFKRun,
  type WorkflowInfo,
  type HarnessInfo,
  type ProjectInfo,
  type ProviderInfo,
  type Profile,
  type ResourceRef,
  type Session,
  type SessionMCPContext,
} from "./api.ts";
import {
  awaitingDecision,
  buildView,
  toolRuns,
  type Item,
  type TranscriptEvent,
} from "./transcript.ts";
import { pickLabel } from "./a11y.ts";
import AttachedImages from "./AttachedImages.tsx";
import { useDismiss } from "./useDismiss.ts";
import { placeholder } from "./composer.ts";
import ToolView, { openSessionEvent } from "./ToolView.tsx";
import Tags from "./Tags.tsx";
import { prepareImage, type PreparedImage } from "./imageprep.ts";
import { addText, isPicture, type TextFile } from "./attach.ts";
import SessionControls from "./SessionControls.tsx";
import SourceMark from "./SourceMark.tsx";
import GitMark from "./GitMark.tsx";
import { contextHigh, contextLabel, contextTitle, shortTokens } from "./contextMeter.ts";
import type { Source } from "./source.ts";
import { draftOf, extrasOf, forgetDraft, rememberDraft, rememberExtras, storeDraft } from "./drafts.ts";
import { MAX_IMAGES, room } from "./images.ts";
import { CheckIcon, CloseIcon, CompactIcon, CopyIcon, ForkIcon, InfoIcon, MoreIcon, PanelRightIcon, PaperclipIcon, QueueIcon, RewindIcon, SendIcon, StopIcon } from "./icons.tsx";
import { Markdown } from "./Markdown.tsx";
import SessionPanel, { showTool, type Tab as PanelTab } from "./SessionPanel.tsx";
import { REVIEW_PLAN_EVENT } from "./PlanReview.tsx";
import { usePaletteActions } from "./Palette.tsx";
import type { PaletteItem } from "./palette.ts";
import { itemForSeq } from "./search.ts";
import { drawOnce, HOLD_PLACE, holdsPlace, READER_PAUSE, readerScrolling, scrollsByKey, scrollsUpByKey, settledPlace } from "./scrollPlace.ts";
import { costText, estimate, formatCost, type PriceEntry } from "./cost.ts";
import { trustLines, trustStep } from "./trust.ts";

interface Props {
  session: Session;
  events: TranscriptEvent[];
  // A search hit to scroll to (SES-007); n tells repeated jumps apart.
  focus?: { seq: number; n: number };
  // The price table for cost estimates (PRV-008).
  prices: PriceEntry[];
  // Opens the AFK run's dialog for a project's backlog (WFL-005), for /afk.
  onRunAFK?: (root: string, workflowId: string) => void;
  // Shows a project's AFK run in full (WFL-009).
  onShowRun?: (root: string) => void;
  // Text for the composer, as after a fork (SES-009).
  initialDraft?: string;
  // Profiles to continue the session with (HRN-012).
  profiles: Profile[];
  onForked: (sessionId: string, text: string) => void;
  provider?: ProviderInfo;
  providers: ProviderInfo[];
  environment?: EnvironmentInfo;
  harness?: HarnessInfo;
  project?: ProjectInfo;
  onProjectsChanged: () => void;
  allowBypass: boolean;
  configVersion: number;
  onError: (err: unknown) => void;
  onDeleted: () => void;
  // Opens the session in a window of its own (GUI-015); absent in that window.
  onOpenWindow?: () => void;
  // Tags on any session, offered when adding one (SES-013).
  knownTags?: string[];
  // Opens the comparison a session belongs to (HRN-016).
  onOpenCompare?: (tag: string) => void;
  // In a workspace of several panes (GUI-017): whether this pane is the
  // active one, which alone takes the keyboard, the palette and the right
  // panel and speaks to screen readers; and how to close the pane.
  active?: boolean;
  onClosePane?: () => void;
  // Starts dragging this session's pane by its header, to swap it with another.
  onPaneDrag?: (e: DragEvent) => void;
  // What the session runs on (GUI-034).
  source: Source;
}

const STOP_LABEL: Record<string, string> = {
  end_turn: "Turn complete",
  cancelled: "Cancelled",
  max_tokens: "Stopped: output limit reached",
  max_turns: "Stopped: turn limit reached",
  refusal: "The model declined this request",
  error: "Stopped by an error",
};

// The right panel's open state is a per-viewer convenience; storage may be
// unavailable, so every access is guarded. Without a stored choice the panel
// starts open only where it fits beside the conversation.
type KeptPicture = PreparedImage & { id: number };
type ComposerExtras = { attached: ResourceRef[]; pictures: KeptPicture[]; textFiles: TextFile[] };

const PANEL_KEY = "freeloop.panelOpen";
function panelOpenInitially(): boolean {
  let stored: string | null = null;
  try {
    stored = localStorage.getItem(PANEL_KEY);
  } catch {
    // no stored choice
  }
  return stored === null ? window.innerWidth > 1000 : stored === "1";
}

export default function SessionView({ session, events, focus, prices, initialDraft, onForked, profiles, provider, providers, environment, harness, project, onProjectsChanged, allowBypass, configVersion, onError, onDeleted, onOpenWindow, knownTags = [], onOpenCompare, active = true, onClosePane, onPaneDrag, source, onRunAFK, onShowRun }: Props) {
  // Beside other panes the panel starts closed and only the active pane shows
  // it; the remembered choice is for a session alone in the window.
  const split = !!onClosePane;
  const [panelOpen, setPanelOpen] = useState(() => !split && panelOpenInitially());
  const wasSplit = useRef(split);
  useEffect(() => {
    if (wasSplit.current !== split) setPanelOpen(!split && panelOpenInitially());
    wasSplit.current = split;
  }, [split]);
  const togglePanel = () =>
    setPanelOpen((open) => {
      if (split) return !open;
      try {
        localStorage.setItem(PANEL_KEY, open ? "0" : "1");
      } catch {
        // not remembered
      }
      return !open;
    });
  const layoutBox = useRef<HTMLDivElement>(null);
  const [panelTab, setPanelTab] = useState<PanelTab>("approvals");
  // The commands an untrusted project's hooks would run, shown before the
  // user trusts it (EXT-005).
  const [trustLookup, setTrustLookup] = useState<string[] | null>(null);
  const trustHooks = trustLookup ?? [];
  const untrusted = !!project && !project.trusted && !project.missing;
  const canBypass = allowBypass && !!project?.trusted;
  // Only an agent with such a mode of its own can approve by itself (SEC-001).
  const offersAuto = !!harness?.modes?.includes("auto");
  const canAuto = offersAuto && !!project?.trusted;
  const modeAllowed = (m: string) => (m === "bypass" ? canBypass : m === "auto" ? canAuto : true);
  useEffect(() => {
    setTrustLookup(null);
    if (!untrusted || !project) return;
    EngineService.ProjectSettings(project.root)
      .then((s) => setTrustLookup(trustLines(s)))
      .catch(() => setTrustLookup(null));
  }, [untrusted, project?.root]);
  const trustProject = async (ask: boolean) => {
    if (!project) return;
    const step = trustStep(trustLookup, ask);
    if (step === "lookup") {
      // Trusting would run hooks nobody has seen; look again, and let the user
      // read the answer before choosing.
      EngineService.ProjectSettings(project.root)
        .then((s) => {
          const hooks = trustLines(s);
          setTrustLookup(hooks);
          if (hooks.length > 0) return onError(`Check what ${project.name} would run, then trust it again.`);
          return EngineService.SetProjectTrust(project.root, true).then(onProjectsChanged);
        })
        .catch((err) => onError(`${project.name} was not trusted: its hooks could not be read. ${err}`));
      return;
    }
    const list = trustHooks.join("\n");
    if (step === "confirm" && !(await askUser(`Trusting ${project.name} lets it run or try these:\n\n${list}`))) return;
    EngineService.SetProjectTrust(project.root, true).then(onProjectsChanged).catch(onError);
  };
  // Trust is taken back as it is given, by the user: the project's instructions,
  // .freeloop/ settings, hooks and skills stop applying, and trusting again approves
  // its hooks as they are then (SEC-006).
  const distrust = async () => {
    if (!project?.trusted) return;
    if (!(await askUser(`Stop trusting ${project.name}? Its AGENTS.md, .freeloop/ settings, hooks and skills stop applying to its sessions until you trust it again.`))) return;
    EngineService.SetProjectTrust(project.root, false).then(onProjectsChanged).catch(onError);
  };
  const [renaming, setRenaming] = useState<string | null>(null);
  const [tagging, setTagging] = useState(false);
  const setTags = (tags: string[]) => EngineService.SetTags(session.id, tags).catch(onError);
  // The model switch form (PRV-006).
  const [switching, setSwitching] = useState<{ providerId: string; model: string } | null>(null);
  const rename = () => {
    if (renaming === null) return;
    EngineService.RenameSession(session.id, renaming)
      .then(() => setRenaming(null))
      .catch(onError);
  };
  // Export (SES-008): the path saved to, shown for a moment.
  const [exported, setExported] = useState("");
  const exportAs = (format: "markdown" | "json") =>
    EngineService.ExportSession(session.id, format)
      .then((path) => {
        if (!path) return;
        setExported(path);
        setTimeout(() => setExported(""), 4000);
      })
      .catch(onError);
  // Continue with another profile or harness (HRN-012): a new session that
  // gets a handoff summary.
  const [switchingTo, setSwitchingTo] = useState<string | null>(null);
  const continueWith = (p: Profile) => {
    setSwitchingTo(p.name);
    EngineService.SwitchProfile(session.id, p.id)
      .then((ses) => onForked(ses.id, ""))
      .catch(onError)
      .finally(() => setSwitchingTo(null));
  };
  const otherProfiles = profiles.filter((p) => p.id !== (session.profileId || "default"));
  // A stuck session continues on another profile in one step (HRN-020): the old turn
  // stops, what it waited on ends, and the new session's box holds the last request.
  const handOff = (profileId: string) => {
    setSwitchingTo(profiles.find((p) => p.id === profileId)?.name ?? profileId);
    EngineService.HandOff(session.id, profileId, view.stuck?.reason ?? "")
      .then((r) => onForked(r.session.id, r.continue))
      .catch(onError)
      .finally(() => setSwitchingTo(null));
  };
  const archived = session.status === "archived";
  const archive = () => EngineService.ArchiveSession(session.id, !archived).catch(onError);
  const remove = async () => {
    if (!(await askUser(`Delete "${session.title}"? Its transcript is removed from disk. This cannot be undone.`))) return;
    EngineService.DeleteSession(session.id).then(onDeleted).catch(onError);
  };
  const demo = isDemo(session);
  const removeDemo = async () => {
    if (!(await askUser("Remove the demo? Its session, its scratch project folder and its project entry are deleted. Nothing else is touched."))) return;
    EngineService.RemoveDemo(session.id)
      .then(() => {
        onProjectsChanged();
        onDeleted();
      })
      .catch(onError);
  };
  const view = useMemo(() => buildView(events), [events]);
  // Estimated cost of the session and of each turn, by turn_end key.
  const costs = useMemo(() => {
    const own = { providerId: session.providerId, model: session.model };
    const turns = new Map<string, string>();
    for (const it of view.items) {
      if (it.kind !== "turn_end" || it.calls.length === 0) continue;
      const e = estimate(prices, it.calls, own);
      if (e.unpriced < it.calls.length) turns.set(it.key, `≈ ${formatCost(e.cost)}${e.unpriced ? " + unpriced calls" : ""}`);
    }
    return { session: estimate(prices, view.calls, own), calls: view.calls.filter((c) => !c.adjustment).length, turns };
  }, [view, prices, session.providerId, session.model]);

  // Screen readers hear approvals and turn ends, not every streamed word
  // (NFR-A11Y-002).
  const waitingTools = view.items.filter((i) => i.kind === "tool" && awaitingDecision(i));
  const [announcement, setAnnouncement] = useState("");
  const heard = useRef({ waiting: waitingTools.length, running: view.running });
  useEffect(() => {
    const before = heard.current;
    const last = waitingTools[waitingTools.length - 1];
    if (waitingTools.length > before.waiting && last?.kind === "tool") setAnnouncement(`Approval needed: ${last.title}`);
    else if (before.running && !view.running) setAnnouncement(view.status === "error" ? "The turn stopped with an error" : "Turn finished");
    heard.current = { waiting: waitingTools.length, running: view.running };
  }, [waitingTools.length, view.running]);
  // Cleared after a moment: the same words again are only heard as a change.
  useEffect(() => {
    if (!announcement) return;
    const t = setTimeout(() => setAnnouncement(""), 4000);
    return () => clearTimeout(t);
  }, [announcement]);
  // Kept until sent or deleted, wherever the user goes meanwhile (SES-023).
  const [draft, setDraft] = useState(() => initialDraft ?? draftOf(session.id));
  const typed = useRef(draft);
  typed.current = draft;
  useEffect(() => {
    rememberDraft(session.id, draft);
    const later = setTimeout(() => storeDraft(session.id, draft, EngineService.RedactText), 400);
    return () => clearTimeout(later);
  }, [draft]);
  // Leaving before the pause is over still writes it.
  useEffect(() => () => void storeDraft(session.id, typed.current, EngineService.RedactText), []);
  const [queued, setQueued] = useState(0);
  const mode = view.mode ?? session.mode;
  const approvalWaiting = view.items.some((i) => i.kind === "tool" && awaitingDecision(i));

  // @ mentions (GUI-004): the project's files load when the picker opens and
  // reload when the list is older than ten seconds.
  const input = useRef<HTMLTextAreaElement>(null);
  const [mention, setMention] = useState<Mention | null>(null);
  const [dismissed, setDismissed] = useState<number | null>(null);
  const [pick, setPick] = useState(0);
  const [files, setFiles] = useState<string[] | null>(null);
  const filesAt = useRef(0);
  useEffect(() => {
    setFiles(null);
    filesAt.current = 0;
  }, [session.id]);
  const loadFiles = () => {
    if (Date.now() - filesAt.current < 10_000) return;
    filesAt.current = Date.now();
    EngineService.ProjectFiles(session.id)
      .then((list) => setFiles(list ?? []))
      .catch((err) => {
        setFiles([]);
        onError(err);
      });
  };
  const syncMention = (el: HTMLTextAreaElement) => {
    const m = el.selectionStart === el.selectionEnd ? mentionAt(el.value, el.selectionStart) : null;
    if (m && m.start === dismissed) return setMention(null);
    if (m && (m.start !== mention?.start || m.query !== mention?.query)) setPick(0);
    setMention(m);
    if (m) loadFiles();
  };
  const matches = useMemo(() => (mention && files ? rankFiles(mention.query, files) : []), [mention, files]);

  // Slash commands (GUI-004): the app's mode commands and the ones the
  // session's harness advertises.
  const [slash, setSlash] = useState<string | null>(null);
  const [slashDismissed, setSlashDismissed] = useState(false);
  // The resources and prompts of the session's MCP servers (MCP-007), read
  // when the session opens and again when a picker opens, since servers
  // start in the background.
  const [mcp, setMcp] = useState<SessionMCPContext | null>(null);
  const refreshMcp = () =>
    EngineService.SessionMCP(session.id)
      .then(setMcp)
      .catch(() => setMcp(null));
  useEffect(() => {
    refreshMcp();
  }, [session.id, view.running]);
  useEffect(() => {
    if (slash !== null) refreshMcp();
  }, [slash === null]);
  // Workflows (WFL-002): the session's workflow's stages are slash commands.
  const [workflows, setWorkflows] = useState<WorkflowInfo[]>([]);
  const loadWorkflows = () =>
    EngineService.Workflows()
      .then((list) => setWorkflows(list ?? []))
      .catch(onError);
  useEffect(() => {
    loadWorkflows();
  }, [session.id]);
  const workflow = workflows.find((w) => w.id === session.workflow);
  const setWorkflow = (id: string) => EngineService.SetWorkflow(session.id, id).catch(onError);
  // The project's AFK run, for the stage strip's AFK stage (WFL-008).
  const [afkRun, setAfkRun] = useState<AFKRun | null>(null);
  useEffect(() => {
    if (!workflow) return setAfkRun(null);
    const load = () =>
      EngineService.Backlog(session.projectRoot, workflow.id)
        .then((b) => setAfkRun(b.run ?? null))
        .catch(() => setAfkRun(null));
    load();
    const t = setInterval(load, 10000);
    return () => clearInterval(t);
  }, [workflow?.id, session.projectRoot, view.running]);
  const commands = useMemo<SlashCommand[]>(
    () => [
      ...MODE_COMMANDS.filter((c) => modeAllowed(c.mode ?? "")),
      ...stageCommands(workflow),
      // The agent's own command for a stage's skill would duplicate the stage (WFL-008).
      ...withoutStageSkills(view.commands, workflow).map((c) => ({ name: c.name, description: c.description })),
      ...(mcp?.prompts ?? []).map((p) => ({
        name: p.command,
        description: p.description || p.title || `Prompt from ${p.server}`,
        prompt: { server: p.server, name: p.name, arguments: p.arguments ?? [] },
      })),
    ],
    [view.commands, canBypass, canAuto, mcp, workflow],
  );
  // Resources attached to the next message, and the open prompt form.
  const keptExtras = extrasOf<ComposerExtras>(session.id);
  const [attached, setAttached] = useState<ResourceRef[]>(keptExtras?.attached ?? []);
  // Pictures for the next message (SES-015), already scaled down.
  const [pictures, setPictures] = useState<KeptPicture[]>(keptExtras?.pictures ?? []);
  const [dragging, setDragging] = useState(false);
  const picker = useRef<HTMLInputElement>(null);
  const addPictures = (files: File[]) => {
    const take = files.filter((f) => f.type.startsWith("image/")).slice(0, room(pictures.length, files.length));
    if (files.some((f) => f.type.startsWith("image/")) && take.length === 0) onError(new Error(`A message takes at most ${MAX_IMAGES} pictures.`));
    for (const f of take) {
      prepareImage(f)
        .then((p) => setPictures((list) => (list.length >= MAX_IMAGES ? (URL.revokeObjectURL(p.url), list) : [...list, { ...p, id: ++pictureCount }])))
        .catch(onError);
    }
  };
  const dropPicture = (id: number) =>
    setPictures((list) => {
      list.filter((p) => p.id === id).forEach((p) => URL.revokeObjectURL(p.url));
      return list.filter((p) => p.id !== id);
    });
  // Text files for the next message (SES-019).
  const [textFiles, setTextFiles] = useState<TextFile[]>(keptExtras?.textFiles ?? []);
  useEffect(() => rememberExtras(session.id, { attached, pictures, textFiles } satisfies ComposerExtras), [attached, pictures, textFiles]);
  const addFiles = (files: File[]) => {
    addPictures(files.filter((f) => isPicture(f.type)));
    for (const f of files.filter((f) => !isPicture(f.type))) {
      f.arrayBuffer()
        .then((b) => {
          const bytes = new Uint8Array(b);
          setTextFiles((list) => {
            const r = addText(list, f.name, bytes);
            if (r.error) onError(new Error(r.error));
            return r.list;
          });
        })
        .catch(onError);
    }
  };
  const [attaching, setAttaching] = useState<string | null>(null);
  const [promptForm, setPromptForm] = useState<{ command: SlashCommand; values: Record<string, string> } | null>(null);
  const runPrompt = (c: SlashCommand, values: Record<string, string>) => {
    if (!c.prompt) return;
    holdBottom.current = false;
    EngineService.RunMCPPrompt(session.id, c.prompt.server, c.prompt.name, values)
      .then((r) => {
        setPromptForm(null);
        forgetDraft(session.id);
        setDraft("");
        if (r.queued) setQueued((n) => n + 1);
      })
      .catch(onError);
  };
  const slashMatches = useMemo(() => (slash === null ? [] : matchCommands(slash, commands)), [slash, commands]);
  // The focus stays in the message box while a picker is open, so the entry
  // that lights up is announced.
  const pickerNames = slash !== null && slashMatches.length > 0 ? slashMatches.map((c) => c.name) : mention ? matches.map((f) => f.slice(nameStart(f))) : [];
  const pickerOptions = pickerNames.length;
  const pickedName = pickerNames[pick] ?? "";
  useEffect(() => {
    if (pickedName) setAnnouncement(pickLabel(pickedName, pick, pickerOptions));
  }, [pickedName, pick, pickerOptions]);
  const syncSlash = (el: HTMLTextAreaElement) => {
    const q = el.selectionStart === el.selectionEnd ? slashQuery(el.value, el.selectionStart) : null;
    if (q === null) setSlashDismissed(false);
    if (q !== slash) setPick(0);
    setSlash(slashDismissed && q !== null ? null : q);
  };
  const chooseCommand = (c: SlashCommand) => {
    setSlash(null);
    if (c.mode) {
      setDraft("");
      setMode(c.mode);
      return;
    }
    if (c.prompt) {
      if (c.prompt.arguments.length > 0) setPromptForm({ command: c, values: {} });
      else runPrompt(c, {});
      return;
    }
    const text = `/${c.name} `;
    setDraft(text);
    requestAnimationFrame(() => {
      input.current?.focus();
      input.current?.setSelectionRange(text.length, text.length);
    });
  };
  const choose = (path: string) => {
    const el = input.current;
    if (!el || !mention) return;
    const r = insertMention(draft, mention, el.selectionStart, path);
    setDraft(r.text);
    setMention(null);
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(r.caret, r.caret);
    });
  };

  // Follow the conversation as it grows, unless the reader scrolled up or a search
  // hit is shown.
  const holdBottom = useRef(false);
  const conversation = useRef<HTMLDivElement>(null);
  const lastTop = useRef(0);
  // When the reader last touched the conversation and last moved up, and whether they
  // still are: new content waits while they do.
  const lastInput = useRef(0);
  const lastUp = useRef(-Infinity);
  const pointerDown = useRef(false);
  const settle = useRef<ReturnType<typeof setTimeout> | null>(null);
  const toTheEnd = (el: HTMLElement) => el.scrollHeight - el.scrollTop - el.clientHeight;
  const followEnd = () => {
    const el = conversation.current;
    if (!el || holdBottom.current || pointerDown.current || performance.now() - lastUp.current < READER_PAUSE) return;
    el.scrollTop = el.scrollHeight;
  };
  const readerInput = () => {
    lastInput.current = performance.now();
  };
  // The reader moved up, or may have: their input can scroll something else or nothing,
  // so where the view ends up decides.
  const readerMovedUp = () => {
    readerInput();
    lastUp.current = performance.now();
    if (settle.current) clearTimeout(settle.current);
    settle.current = setTimeout(() => {
      const el = conversation.current;
      if (!el) return;
      holdBottom.current = settledPlace(holdBottom.current, toTheEnd(el));
      followEnd();
    }, READER_PAUSE);
  };
  // Before the frame is drawn, so the view never shows itself away from the end.
  useLayoutEffect(followEnd, [view]);
  // The conversation changes size without any event too: the composer grows, a pane
  // becomes active and shows its controls, a hidden tab comes back. So do its items: one
  // has a placeholder height until first drawn, and a picture or a card can grow later.
  useEffect(() => {
    const el = conversation.current;
    if (!el) return;
    const resized = new ResizeObserver(followEnd);
    resized.observe(el);
    for (const item of el.children) resized.observe(item, { box: "border-box" });
    const items = new MutationObserver((changes) => {
      for (const c of changes) {
        c.addedNodes.forEach((n) => n instanceof Element && resized.observe(n, { box: "border-box" }));
        c.removedNodes.forEach((n) => n instanceof Element && resized.unobserve(n));
      }
    });
    items.observe(el, { childList: true });
    const hold = () => {
      holdBottom.current = true;
    };
    el.addEventListener(HOLD_PLACE, hold);
    const released = () => {
      pointerDown.current = false;
    };
    // WebKit may not report the release of a scroll bar.
    const moved = (e: PointerEvent) => {
      if (e.buttons === 0) pointerDown.current = false;
    };
    window.addEventListener("pointerup", released);
    window.addEventListener("pointercancel", released);
    window.addEventListener("pointermove", moved);
    return () => {
      resized.disconnect();
      items.disconnect();
      el.removeEventListener(HOLD_PLACE, hold);
      if (settle.current) clearTimeout(settle.current);
      window.removeEventListener("pointerup", released);
      window.removeEventListener("pointercancel", released);
      window.removeEventListener("pointermove", moved);
    };
  }, []);
  useEffect(() => (conversation.current ? drawOnce(conversation.current) : undefined), []);

  // Scroll to a search hit once its item is there and mark it for a moment.
  const [focused, setFocused] = useState<string | null>(null);
  const shown = useRef(0);
  // A search hit from outside, or a link in the panel.
  const aims = useRef(0);
  const [aim, setAim] = useState<{ seq: number; n: number } | null>(null);
  useEffect(() => {
    if (focus) setAim({ seq: focus.seq, n: ++aims.current });
  }, [focus]);
  useEffect(() => {
    if (!aim || shown.current === aim.n) return;
    const key = itemForSeq(view.items, aim.seq);
    const el = key && layoutBox.current?.querySelector<HTMLElement>(`[data-item="${key}"]`);
    if (!el) return;
    shown.current = aim.n;
    holdBottom.current = true;
    // Items above have estimated heights until rendered; scroll again once
    // they settle.
    el.scrollIntoView({ block: "center" });
    setTimeout(() => el.scrollIntoView({ block: "center" }), 80);
    setFocused(key);
    const t = setTimeout(() => setFocused(null), 2500);
    return () => clearTimeout(t);
  }, [aim, view.items]);

  useEffect(() => {
    if (!view.running) setQueued(0);
  }, [view.running]);

  const [accepting, setAccepting] = useState(false);
  // runStage has the engine run a stage of the session's workflow; typed, it
  // clears the draft once accepted.
  const runStage = (command: string, text: string, typed: boolean) => {
    holdBottom.current = false;
    setAccepting(true);
    EngineService.RunStage(session.id, command, text)
      .then(() => {
        if (!typed) return;
        forgetDraft(session.id);
        setDraft("");
      })
      .catch(onError)
      .finally(() => setAccepting(false));
  };
  // The stage strip's Next button (WFL-008): a stage that needs to know what it is
  // about waits in the composer for it; an AFK run opens its dialog; any other runs.
  const nextStage = (stage: { command: string; kind?: string; needsInput?: boolean }) => {
    if (!workflow) return;
    if (stage.kind === "afk") return onRunAFK?.(session.projectRoot, workflow.id);
    if (stage.needsInput) {
      setDraft(`/${stage.command} `);
      requestAnimationFrame(() => {
        const el = input.current;
        if (!el) return;
        el.focus();
        el.setSelectionRange(el.value.length, el.value.length);
      });
      return;
    }
    runStage(stage.command, "", false);
  };
  const send = () => {
    const text = draft.trim();
    if ((!text && pictures.length === 0 && textFiles.length === 0) || accepting) return;
    const m = modeCommand(text);
    if (m && modeAllowed(m)) {
      setDraft("");
      setMode(m);
      return;
    }
    // A workflow's stage goes to the engine, with what follows its command.
    const call = stageCall(text, workflow);
    // An AFK run starts from its dialog, which shows what it will do first.
    if (call && workflow && (workflow.stages ?? []).find((s) => s.command === call.command)?.kind === "afk") {
      setDraft("");
      forgetDraft(session.id);
      onRunAFK?.(session.projectRoot, workflow.id);
      return;
    }
    if (call) {
      runStage(call.command, call.input, true);
      return;
    }
    // A message that is just an MCP prompt's command runs the prompt.
    const prompt = commands.find((c) => c.prompt && text === `/${c.name}`);
    if (prompt) {
      chooseCommand(prompt);
      return;
    }
    holdBottom.current = false;
    const sending = attached;
    const images = pictures.map((p) => ({ mediaType: p.mediaType, data: p.data }));
    // Until the engine takes the message, which waits for an agent to
    // start (the first start may download it), the composer says so and
    // takes no second one.
    setAccepting(true);
    (sending.length > 0 || images.length > 0 || textFiles.length > 0 ? EngineService.PromptWith(session.id, { text, resources: sending, images, files: textFiles }) : EngineService.Prompt(session.id, text))
      .then((r) => {
        // Here as well: the view may be gone by the time the engine has taken it.
        forgetDraft(session.id);
        setDraft("");
        setAttached([]);
        setTextFiles([]);
        pictures.forEach((p) => URL.revokeObjectURL(p.url));
        setPictures([]);
        if (r.queued) setQueued((n) => n + 1);
      })
      .catch(onError)
      .finally(() => setAccepting(false));
  };

  const compactNow = () => {
    holdBottom.current = false;
    setAccepting(true);
    EngineService.Prompt(session.id, "/compact")
      .catch(onError)
      .finally(() => setAccepting(false));
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.nativeEvent.isComposing) return;
    if (slash !== null && slashMatches.length > 0) {
      const n = slashMatches.length;
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        setPick((p) => (p + (e.key === "ArrowDown" ? 1 : n - 1)) % n);
        return;
      }
      if ((e.key === "Enter" && !primaryKey(e) && !e.shiftKey) || e.key === "Tab") {
        e.preventDefault();
        chooseCommand(slashMatches[Math.min(pick, n - 1)]);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        setSlashDismissed(true);
        setSlash(null);
        return;
      }
    }
    if (mention && matches.length > 0) {
      const n = matches.length;
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        setPick((p) => (p + (e.key === "ArrowDown" ? 1 : n - 1)) % n);
        return;
      }
      if ((e.key === "Enter" && !primaryKey(e)) || e.key === "Tab") {
        e.preventDefault();
        choose(matches[Math.min(pick, n - 1)]);
        return;
      }
    }
    if (mention && e.key === "Escape") {
      e.preventDefault();
      setDismissed(mention.start);
      setMention(null);
      return;
    }
    // ⌘↩ (Ctrl+Enter outside a Mac) answers a waiting approval (see shortcuts.ts)
    // instead of sending.
    if (primaryKey(e) && approvalWaiting) return;
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  };

  const setMode = (m: string) => EngineService.SetMode(session.id, m).catch(onError);
  // Token optimizers (HRN-019). They are looked for again whenever the list is
  // opened, so one installed a moment ago shows up.
  const [optimizers, setOptimizers] = useState<OptimizerInfo[]>([]);
  const loadOptimizers = () =>
    EngineService.Optimizers(session.id)
      .then((list) => setOptimizers(list ?? []))
      .catch(onError);
  useEffect(() => {
    loadOptimizers();
  }, [session.id]);
  // One that cannot be used yet says why here, with a way to install it.
  const [optimizerHelp, setOptimizerHelp] = useState<OptimizerInfo | null>(null);
  const optimizerWarning = optimizers.find((o) => o.warning)?.warning;
  const [warningDismissed, setWarningDismissed] = useState(false);
  const setOptimizer = (id: string) => {
    const chosen = optimizers.find((o) => o.id === id);
    if (chosen && !chosen.available) return setOptimizerHelp(chosen);
    setOptimizerHelp(null);
    EngineService.SetOptimizer(session.id, id).catch(onError);
  };
  // One click: fetch it, then turn it on for this session.
  const [installing, setInstalling] = useState(false);
  const install = (o: OptimizerInfo) => {
    setInstalling(true);
    EngineService.InstallOptimizer(o.id)
      .then(() => EngineService.SetOptimizer(session.id, o.id))
      .then(() => {
        setOptimizerHelp(null);
        return loadOptimizers();
      })
      .catch(onError)
      .finally(() => setInstalling(false));
  };
  const typeInstall = (o: OptimizerInfo) =>
    EngineService.TypeOptimizerInstall(session.id, o.id)
      .then(() => {
        setOptimizerHelp(null);
        setTerminalNonce((n) => n + 1);
        showPanel("terminal");
      })
      .catch(onError);
  // The rewound message comes back to the composer for editing (SES-014).
  const fork = (seq: number) =>
    EngineService.ForkSession(session.id, seq)
      .then((r) => onForked(r.session.id, r.text))
      .catch(onError);
  const rewind = (seq: number, text: string) =>
    EngineService.Rewind(session.id, seq)
      .then(() => {
        setDraft(text);
        input.current?.focus();
      })
      .catch(onError);
  // The box is one line until there is more to show; a hidden pane has no height to read.
  useLayoutEffect(() => {
    const el = input.current;
    if (!el) return;
    el.style.height = "auto";
    if (el.scrollHeight > 0) el.style.height = `${el.scrollHeight}px`;
  }, [draft, active, panelOpen]);

  // The folders a command's leading `cd` adds nothing for (quietCommand).
  const roots = useMemo(() => [session.projectRoot, session.worktree].filter(Boolean) as string[], [session.projectRoot, session.worktree]);
  const costLabel = costText(costs.session, costs.calls, view.measured);

  // Everything this view shows a control for, for the palette (GUI-014).
  const showPanel = (t: PanelTab) => {
    setPanelTab(t);
    if (!panelOpen) togglePanel();
  };
  // A plan's card opens the Plan tab (GUI-043).
  useEffect(() => {
    const el = layoutBox.current;
    if (!el) return;
    const review = () => showPanel("plan");
    el.addEventListener(REVIEW_PLAN_EVENT, review);
    return () => el.removeEventListener(REVIEW_PLAN_EVENT, review);
  });
  const showRules = () => {
    showPanel("approvals");
    requestAnimationFrame(() => requestAnimationFrame(() => document.querySelector(`[data-rules="${session.id}"]`)?.scrollIntoView({ block: "start" })));
  };
  const openFolder = () => EngineService.OpenProjectFolder(session.id).catch(onError);
  // The project's run command for a worktree session (SES-016).
  const [runCommand, setRunCommand] = useState("");
  const [terminalNonce, setTerminalNonce] = useState(0);
  useEffect(() => {
    setRunCommand("");
    if (!session.worktree || !session.projectId) return;
    EngineService.ProjectScripts(session.projectId)
      .then((s) => setRunCommand(s.effective.run ?? ""))
      .catch(() => {});
  }, [session.id, session.worktree, session.projectId, configVersion]);
  const runApp = () =>
    EngineService.RunWorktreeScript(session.id)
      .then(() => {
        setTerminalNonce((n) => n + 1);
        showPanel("terminal");
      })
      .catch(onError);
  const rewindable = view.items.filter((i): i is Extract<Item, { kind: "user" }> => i.kind === "user" && !!i.checkpointSeq && !i.rewound).reverse();
  const act = (id: string, title: string, run: () => void, extra: Partial<PaletteItem> = {}): PaletteItem => ({
    id: `session:${session.id}:${id}`,
    title,
    group: "This session",
    run,
    ...extra,
  });
  const decideFirst = (t: Item, kind: "allow_once" | "reject") => {
    // A form is sent from its card, with its answers.
    if (t.kind !== "tool" || !t.permission || (t.permission.form && kind !== "reject")) return;
    const opt = t.permission.options.find((o) => (kind === "reject" ? o.kind.startsWith("reject") : o.kind === kind));
    if (opt) EngineService.Decide({ requestId: t.permission.requestId, optionId: opt.optionId, reason: "", scope: "session" }).catch(onError);
  };
  usePaletteActions(!active ? [] : [
    ...waitingTools.flatMap((t) =>
      t.kind !== "tool"
        ? []
        : [
            act(`allow:${t.toolCallId}`, `Allow once: ${t.title}`, () => decideFirst(t, "allow_once"), { group: "Approval", keywords: "approve yes" }),
            act(`deny:${t.toolCallId}`, `Deny: ${t.title}`, () => decideFirst(t, "reject"), { group: "Approval", keywords: "reject no" }),
            act(`show:${t.toolCallId}`, `Show the request: ${t.title}`, () => showTool(t.toolCallId), { group: "Approval" }),
          ],
    ),
    ...(view.running ? [act("stop", "Stop the turn", () => EngineService.Cancel(session.id).catch(onError), { keywords: "cancel interrupt" })] : []),
    act("compose", "Write a message", () => input.current?.focus(), { keywords: "composer prompt focus" }),
    ...commands.map((c) =>
      act(`cmd:${c.name}`, `/${c.name}`, () => chooseCommand(c), { group: "Command", detail: c.description, keywords: c.prompt ? "slash mcp prompt" : "slash" }),
    ),
    ...(mcp?.resources ?? [])
      .filter((r) => !attached.some((a) => a.server === r.server && a.uri === r.uri))
      .map((r) =>
        act(`attach:${r.server}:${r.uri}`, `Attach ${r.title || r.name}`, () => setAttached((list) => [...list, { server: r.server, uri: r.uri, name: r.title || r.name }]), {
          group: "Resource",
          detail: `${r.server} · ${r.uri}`,
          keywords: `mcp ${r.description ?? ""}`,
        }),
      ),
    ...MODES.filter((m) => m.id !== mode && modeAllowed(m.id)).map((m) => act(`mode:${m.id}`, `Mode: ${m.name}`, () => setMode(m.id), { keywords: "permission" })),
    ...(session.providerId ? [act("model", "Switch model…", () => setSwitching({ providerId: session.providerId, model: session.model }), { keywords: "provider" })] : []),
    ...workflows
      .filter((w) => w.id !== session.workflow)
      .map((w) =>
        act(
          `workflow:${w.id}`,
          `Start a workflow: ${w.name}`,
          () => {
            setWorkflow(w.id);
            const first = (w.stages ?? [])[0];
            if (first) chooseCommand({ name: first.command, description: first.title, stage: true });
          },
          { keywords: "workflow plan prd slice afk issues backlog", detail: w.reason || (w.stages ?? []).map((s) => `/${s.command}`).join(" ") },
        ),
      ),
    ...(session.workflow ? [act("workflow:off", "Workflow: Off", () => setWorkflow(""), { keywords: "workflow" })] : []),
    ...[{ id: "", name: "Off" }, ...optimizers]
      .filter((o) => o.id !== session.optimizer)
      .map((o) => act(`optimizer:${o.id}`, `Token optimizer: ${o.name}`, () => setOptimizer(o.id), { keywords: "tokens rtk compress output" })),
    act(
      "report",
      "Copy a problem report",
      () =>
        EngineService.ProblemReport(session.id)
          .then((text) => Clipboard.SetText(text))
          .catch(onError),
      { keywords: "bug issue log crash", detail: "version, system, this session's agent, the log's end" },
    ),
    act("panel", panelOpen ? "Hide the panel" : "Show the panel", togglePanel, { keywords: "right sidebar" }),
    ...(onClosePane ? [act("close-pane", "Close this pane", onClosePane, { keywords: "workspace split" })] : []),
    act("approvals", "Show approvals", () => showPanel("approvals"), { keywords: "panel decided" }),
    act("diff", "Show changes", () => showPanel("diff"), { keywords: "diff git commit stage panel" }),
    act("files", "Show files", () => showPanel("files"), { keywords: "tree browse viewer open" }),
    act("terminal", "Show the terminal", () => showPanel("terminal"), { keywords: "shell console pty command line" }),
    act("preview", "Show the app preview", () => showPanel("preview"), { keywords: "browser dev server localhost port" }),
    act("info", "Show session details", () => showPanel("info"), { keywords: "info panel tokens" }),
    ...(project && !project.trusted && !project.missing
      ? [act("trust", "Trust this project", () => trustProject(true), { detail: trustHooks.length ? `${project.root} · runs ${trustHooks.length} hook${trustHooks.length > 1 ? "s" : ""}` : project.root })]
      : []),
    ...rewindable.map((u, n) =>
      act(`rewind:${u.key}`, n === 0 ? "Rewind to before the last message…" : `Rewind to before “${u.text.slice(0, 60)}”…`, async () => {
        if ((await askUser("Restore the files to before this message and continue from there?"))) rewind(u.checkpointSeq!, u.text);
      }, { keywords: `undo checkpoint restore ${n === 0 ? u.text.slice(0, 60) : ""}` }),
    ),
    ...(view.items.some((i) => i.kind === "user")
      ? [
          act(
            "fork",
            "Fork before the last message",
            () => {
              const last = [...view.items].reverse().find((i) => i.kind === "user");
              if (last) fork(Number(last.key));
            },
            { keywords: "branch copy new session" },
          ),
        ]
      : []),
    ...otherProfiles.map((p) =>
      act(`continue:${p.id}`, `Continue with ${p.name}`, () => continueWith(p), { keywords: "switch profile harness handoff", detail: p.description || undefined }),
    ),
    ...(onOpenWindow ? [act("window", "Open in a new window", onOpenWindow, { keywords: "window separate detach" })] : []),
    act("export-md", "Export as Markdown…", () => exportAs("markdown"), { keywords: "save share download" }),
    act("export-json", "Export as JSON…", () => exportAs("json"), { keywords: "save download events" }),
    act("rename", "Rename session", () => setRenaming(session.title), { keywords: "title" }),
    act("tag", "Add a tag to this session", () => setTagging(true), { keywords: "label tags" }),
    act("archive", archived ? "Unarchive session" : "Archive session", archive, { keywords: "hide" }),
    ...(project?.trusted ? [act("distrust", "Stop trusting this project…", distrust, { keywords: "untrust distrust trust revoke" })] : []),
    act("delete", "Delete session…", remove, { keywords: "remove" }),
  ]);

  // The header's More menu closes once an action is picked.
  const moreMenu = useRef<HTMLDetailsElement>(null);
  // It also closes on a click anywhere else, and on Escape.
  useDismiss(moreMenu);
  // What the header leaves to its tooltip and the Info tab.
  const headerDetails = [
    session.worktree ? `Worktree of ${session.projectRoot}: ${session.worktree}` : session.projectRoot,
    `Environment: ${environment?.name ?? session.environmentId ?? "Local"}`,
    session.providerId ? `${provider?.name ?? session.providerId} · ${session.model}` : (harness?.name ?? session.harnessId),
  ].join("\n");
  const fromMenu = (act: () => void) => () => {
    if (moreMenu.current) moreMenu.current.open = false;
    act();
  };

  return (
    <div ref={layoutBox} className="session-layout">
      <section className="session">
        <header className="session-header" draggable={!!onPaneDrag && renaming === null && !tagging} onDragStart={onPaneDrag}>
          <div className="session-heading">
            <div className="session-titleline">
              <SourceMark source={source} detail={session.model || undefined} />
              {view.context && contextLabel(view.context) && (
                <span className={contextHigh(view.context) ? "context-meter context-high" : "context-meter"} title={contextTitle(view.context)}>
                  {contextLabel(view.context)}
                </span>
              )}
              {renaming === null ? (
                <h2 onDoubleClick={() => setRenaming(session.title)} title={`${headerDetails}\nDouble-click to rename`}>
                  {session.title}
                </h2>
              ) : (
                <form
                  onSubmit={(e) => {
                    e.preventDefault();
                    rename();
                  }}
                >
                  <input autoFocus aria-label="Session title" value={renaming} onChange={(e) => setRenaming(e.target.value)} onBlur={rename} />
                </form>
              )}
              {(view.git?.repo || session.branch) && <GitMark branch={view.git?.repo ? view.git.branch : session.branch} />}
              <span
                className="muted session-sub"
                title={view.git?.repo ? `HEAD ${view.git.head.slice(0, 10)}, started at ${view.git.baseCommit.slice(0, 10)}` : undefined}
              >
                {source.name}
                {view.git?.repo && (
                  <>
                    {" · "}
                    {view.git.branch || "detached"}
                    {session.worktree ? " (own worktree)" : ""}
                    {view.git.dirty > 0 ? ` · ${view.git.dirty} changed` : ""}
                  </>
                )}
                {/* Last, so the narrowest pane loses it first. */}
                {session.model && ` · ${session.model}`}
                {switchingTo && ` · Handing over to ${switchingTo}…`}
              </span>
            </div>
            {session.parentId && (
              <p className="muted">
                A subagent.{" "}
                <button className="link" onClick={() => openSessionEvent(session.parentId)}>
                  Open the session that started it
                </button>
              </p>
            )}
            <Tags tags={session.tags ?? []} known={knownTags} adding={tagging} onAdding={setTagging} onChange={setTags} />
            {onOpenCompare &&
              (session.tags ?? [])
                .filter((t) => t.startsWith("compare-"))
                .map((t) => (
                  <button key={t} className="link compare-link" onClick={() => onOpenCompare(t)}>
                    Compare with the other runs
                  </button>
                ))}
          </div>
          <div className="session-actions">
            {contextHigh(view.context) && view.commands.some((c) => c.name === "compact") && (
              <button className="icon-button" aria-label="Compact context" disabled={view.running || accepting} onClick={compactNow} title="Compact context: summarise the conversation so far, so each message sends less">
                <CompactIcon />
              </button>
            )}
            {exported && (
              <span className="ok" role="status" title={exported}>
                Exported
              </span>
            )}
            <button className="icon-button" aria-label="Panel" onClick={togglePanel} aria-pressed={panelOpen && (active || !split)} title="Show or hide the right panel">
              <PanelRightIcon />
            </button>
            <details className="export-menu" ref={moreMenu}>
              <summary className="icon-button" aria-label="More actions for this session" title="More actions">
                <MoreIcon />
              </summary>
              <div className="export-options">
                {otherProfiles.length > 0 && (
                  <>
                    <span className="muted export-group" title="A new session with another profile or harness, told what happened here">
                      Continue with
                    </span>
                    {otherProfiles.map((p) => (
                      <button key={p.id} disabled={!!switchingTo || view.running} onClick={fromMenu(() => continueWith(p))}>
                        {p.name}
                      </button>
                    ))}
                    <hr />
                  </>
                )}
                {runCommand && (
                  <button title={`Runs ${runCommand} in a terminal tab`} disabled={archived} onClick={fromMenu(runApp)}>
                    Run
                  </button>
                )}
                <button onClick={fromMenu(openFolder)} title={`Show ${session.worktree || session.projectRoot} ${inFileManager()}`}>
                  {showInFiles()}
                </button>
                {onOpenWindow && <button onClick={fromMenu(onOpenWindow)}>Open in a new window</button>}
                <hr />
                <button onClick={fromMenu(() => setRenaming(session.title))}>Rename</button>
                <button onClick={fromMenu(() => setTagging(true))}>Add a tag</button>
                <button onClick={fromMenu(() => exportAs("markdown"))}>Export as Markdown…</button>
                <button onClick={fromMenu(() => exportAs("json"))}>Export as JSON…</button>
                <button onClick={fromMenu(archive)}>{archived ? "Unarchive" : "Archive"}</button>
                {project?.trusted && <button onClick={fromMenu(distrust)}>Stop trusting this project…</button>}
                <hr />
                <button className="danger" onClick={fromMenu(remove)}>
                  Delete…
                </button>
              </div>
            </details>
            {onClosePane && (
              <button className="icon-button pane-close" onClick={onClosePane} aria-label="Close this pane" title="Close this pane">
                <CloseIcon />
              </button>
            )}
          </div>
        </header>
        {optimizerWarning && !warningDismissed && (
          <div className="banner optimizer-help" role="status">
            <p>{optimizerWarning}</p>
            <button className="link" onClick={() => setWarningDismissed(true)}>
              Dismiss
            </button>
          </div>
        )}
        {optimizerHelp && (
          <div className="banner optimizer-help" role="status">
            <p>
              {optimizerHelp.reason}
              {optimizerHelp.canDownload && <span className="muted"> {optimizerHelp.download}</span>}
            </p>
            {optimizerHelp.canDownload && (
              <button className="primary" disabled={installing} onClick={() => install(optimizerHelp)}>
                {installing ? "Installing…" : `Install ${optimizerHelp.name} and turn it on`}
              </button>
            )}
            {optimizerHelp.install && <button onClick={() => typeInstall(optimizerHelp)}>Type it in the terminal</button>}
            <button className="link" onClick={() => setOptimizerHelp(null)}>
              Dismiss
            </button>
          </div>
        )}
        {switching && (
          <form
            className="banner row model-switch"
            onSubmit={(e) => {
              e.preventDefault();
              EngineService.SetModel(session.id, switching.providerId, switching.model)
                .then(() => setSwitching(null))
                .catch(onError);
            }}
          >
            <span>Next turn uses</span>
            <select value={switching.providerId} onChange={(e) => setSwitching({ providerId: e.target.value, model: providers.find((p) => p.id === e.target.value)?.defaultModel ?? "" })} aria-label="Provider">
              {providers.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
            <input value={switching.model} onChange={(e) => setSwitching({ ...switching, model: e.target.value })} aria-label="Model" placeholder="Model ID" />
            <button type="submit" className="primary" disabled={!switching.model.trim()}>
              Switch
            </button>
            <button type="button" onClick={() => setSwitching(null)}>
              Cancel
            </button>
          </form>
        )}
        {demo && (
          <div className="banner banner-slim demo-banner" role="status">
            <span>This is the demo: a scripted agent in a scratch project. Nothing outside it is touched.</span>
            <button onClick={removeDemo}>Remove demo</button>
          </div>
        )}
        {untrusted && (
          <div className="banner warn row" role="status">
            <span title="Without the project's AGENTS.md, .freeloop/ settings or git status.">
              Untrusted project: global settings only.
              {trustHooks.length > 0 && (
                <>
                  {" "}
                  Trusting it lets it run or try these:
                  <ul>
                    {trustHooks.map((line, n) => (
                      <li key={n}>
                        <code>{line}</code>
                      </li>
                    ))}
                  </ul>
                </>
              )}
            </span>
            <button onClick={() => trustProject(false)}>Trust project</button>
          </div>
        )}
        {mode === "auto" && (
          <div className="banner warn banner-slim" role="status" title="What the agent does not ask about never reaches Freeloop, and your deny rules do not apply to it.">
            <InfoIcon /> Auto mode: the agent approves its own tool calls.
          </div>
        )}
{view.stuck && !archived && (
          <div className="banner warn row stuck-banner" role="status">
            <span>
              Not getting on: {view.stuck.detail}.
              {view.stuck.auto && view.stuck.profileName ? ` It continues on ${view.stuck.profileName} when this turn ends.` : ""}
            </span>
            {view.stuck.profileId && !view.stuck.auto && (
              <button className="primary" disabled={!!switchingTo} onClick={() => handOff(view.stuck!.profileId!)}>
                Continue on {view.stuck.profileName}
              </button>
            )}
            {otherProfiles.length > 0 && (
              <select aria-label="Continue on another profile" value="" disabled={!!switchingTo} onChange={(e) => e.target.value && handOff(e.target.value)}>
                <option value="">{view.stuck.profileId ? "Another profile…" : "Continue on a profile…"}</option>
                {otherProfiles.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
            )}
          </div>
        )}
                {mode === "bypass" && (
          <div className="banner error banner-slim" role="status" title="Protected paths and deny rules still hold.">
            <InfoIcon /> Bypass: tool calls run without asking.
          </div>
        )}

        <div className="sr-only" role="status" aria-live="polite">
          {active && announcement}
        </div>
        <div
          ref={conversation}
          className="conversation"
          role="region"
          aria-label="Conversation"
          onScroll={(e) => {
            const el = e.currentTarget;
            const byReader = readerScrolling(performance.now(), lastInput.current, pointerDown.current);
            holdBottom.current = holdsPlace(holdBottom.current, lastTop.current, el.scrollTop, el.scrollHeight - el.scrollTop - el.clientHeight, byReader);
            lastTop.current = el.scrollTop;
          }}
          onWheel={(e) => {
            readerInput();
            if (e.deltaY < 0) readerMovedUp();
          }}
          onTouchMove={readerInput}
          onKeyDown={(e) => {
            if (scrollsUpByKey(e.key)) readerMovedUp();
            else if (scrollsByKey(e.key)) readerInput();
          }}
          onPointerDown={() => {
            pointerDown.current = true;
          }}
          onClick={(e) => {
            // A card opened to read stays where it is; closed again near the end, the
            // following picks up.
            const card = e.target instanceof Element ? e.target.closest("summary")?.parentElement : null;
            if (!(card instanceof HTMLDetailsElement)) return;
            const box = e.currentTarget;
            if (!card.open) holdBottom.current = true;
            else requestAnimationFrame(() => (holdBottom.current = settledPlace(holdBottom.current, toTheEnd(box))));
          }}
        >
          {view.items.length === 0 && <p className="muted empty">Describe a task to start.</p>}
          {toolRuns(view.items).map((run) => {
            const one = (item: Item) => (
              <ItemView key={item.key} sid={session.id} item={item} focused={item.key === focused} cost={costs.turns.get(item.key)} roots={roots} onError={onError} onRewind={rewind} onFork={fork} />
            );
            if (run.items.length === 1) return one(run.items[0]);
            return (
              // Opens for a search hit inside it; otherwise it is the user's.
              <details key={`run-${run.items[0].key}`} className="tool-run" open={run.items.some((i) => i.key === focused) || undefined}>
                <summary className="muted">{run.items.length} tool calls</summary>
                {run.items.map(one)}
              </details>
            );
          })}
          {accepting && !view.running && (
            <p className="muted working" role="status">
              {view.setup === "running"
                ? "Waiting for the worktree setup to finish…"
                : harness && !harness.usesProvider
                  ? `Starting ${harness.name}… The first start can take a minute while it downloads.`
                  : "Sending…"}
            </p>
          )}
          {view.running && (
            <p className="muted working">{view.status === "queued" ? "Waiting for a free slot on the provider…" : "Working…"}</p>
          )}
        </div>

        {promptForm && promptForm.command.prompt && (
          <form
            className="banner prompt-form"
            aria-label={`Arguments of /${promptForm.command.name}`}
            onSubmit={(e) => {
              e.preventDefault();
              runPrompt(promptForm.command, promptForm.values);
            }}
          >
            <strong>/{promptForm.command.name}</strong> <span className="muted">{promptForm.command.description}</span>
            {promptForm.command.prompt.arguments.map((a, i) => (
              <label key={a.name}>
                {a.title || a.name}
                {a.required ? " *" : ""} {a.description && <span className="muted">{a.description}</span>}
                <input
                  autoFocus={i === 0}
                  required={a.required}
                  value={promptForm.values[a.name] ?? ""}
                  onChange={(e) => setPromptForm({ ...promptForm, values: { ...promptForm.values, [a.name]: e.target.value } })}
                />
              </label>
            ))}
            <div className="row">
              <button type="submit" className="primary">
                Run prompt
              </button>
              <button type="button" onClick={() => setPromptForm(null)}>
                Cancel
              </button>
            </div>
          </form>
        )}
        {workflow && (
          <WorkflowStrip
            workflow={workflow}
            items={view.items}
            running={view.running}
            run={afkRun}
            roots={roots}
            onNext={nextStage}
            onShowRun={onShowRun ? () => onShowRun(session.projectRoot) : undefined}
          />
        )}
        <div
          className={dragging ? "composer composer-drop" : "composer"}
          onDragOver={(e) => {
            if (!e.dataTransfer.types.includes("Files")) return;
            e.preventDefault();
            setDragging(true);
          }}
          onDragLeave={(e) => {
            if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setDragging(false);
          }}
          onDrop={(e) => {
            if (!e.dataTransfer.files.length) return;
            e.preventDefault();
            setDragging(false);
            addFiles([...e.dataTransfer.files]);
          }}
        >
          {view.plan.length > 0 && <TodoList entries={view.plan} running={view.running} thisTurn={view.planThisTurn} />}
          {attaching !== null && (
            <div className="mention-picker attach-picker" role="dialog" aria-label="Attach a resource">
              <input
                autoFocus
                aria-label="Find a resource"
                placeholder="Find a resource"
                value={attaching}
                onChange={(e) => setAttaching(e.target.value)}
                onKeyDown={(e) => e.key === "Escape" && (e.preventDefault(), setAttaching(null))}
              />
              <ul role="listbox" aria-label="Resources">
                {mcp === null && <li className="muted">Asking the servers…</li>}
                {mcp !== null && (mcp.resources ?? []).length === 0 && (
                  <li className="muted">None of this session's MCP servers offers resources{(mcp.problems ?? []).length ? ` (${(mcp.problems ?? []).join("; ")})` : ""}.</li>
                )}
                {(mcp?.resources ?? [])
                  .filter((r) => `${r.server} ${r.name} ${r.title ?? ""} ${r.uri} ${r.description ?? ""}`.toLowerCase().includes(attaching.trim().toLowerCase()))
                  .map((r) => (
                    <li
                      key={`${r.server} ${r.uri}`}
                      role="option"
                      aria-selected={false}
                      onMouseDown={(e) => {
                        e.preventDefault();
                        if (!attached.some((a) => a.server === r.server && a.uri === r.uri)) setAttached([...attached, { server: r.server, uri: r.uri, name: r.title || r.name }]);
                        setAttaching(null);
                        input.current?.focus();
                      }}
                    >
                      <span>{r.title || r.name}</span> <span className="muted">{r.server} · {r.description || r.uri}</span>
                    </li>
                  ))}
              </ul>
            </div>
          )}
          {pictures.length > 0 && (
            <ul className="picture-strip" aria-label="Pictures for the next message">
              {pictures.map((p, i) => (
                <li key={p.id}>
                  <img src={p.url} alt={`Picture ${i + 1}`} />
                  <button className="picture-remove" aria-label={`Remove picture ${i + 1}`} title="Remove" onClick={() => dropPicture(p.id)}>
                    <CloseIcon />
                  </button>
                </li>
              ))}
            </ul>
          )}
          {textFiles.length > 0 && (
            <ul className="attachments" aria-label="Files for the next message">
              {textFiles.map((f) => (
                <li key={f.name} className="chip" title={`${f.text.length.toLocaleString()} characters`}>
                  {f.name}
                  <button className="link" aria-label={`Remove ${f.name}`} onClick={() => setTextFiles((list) => list.filter((x) => x !== f))} title="Remove">
                    <CloseIcon />
                  </button>
                </li>
              ))}
            </ul>
          )}
          {attached.length > 0 && (
            <ul className="attachments" aria-label="Attached resources">
              {attached.map((a) => (
                <li key={`${a.server} ${a.uri}`} className="chip" title={a.uri}>
                  {a.server}: {a.name || a.uri}
                  <button className="link" aria-label={`Remove ${a.name || a.uri}`} onClick={() => setAttached(attached.filter((x) => x !== a))} title="Remove">
                    <CloseIcon />
                  </button>
                </li>
              ))}
            </ul>
          )}
          {slash !== null && slashMatches.length > 0 && (
            <ul className="mention-picker" id={`composer-options-${session.id}`} role="listbox" aria-label="Commands">
              {slashMatches.map((c, i) => (
                <li
                  key={c.name}
                  id={`composer-option-${i}`}
                  role="option"
                  aria-selected={i === pick}
                  className={i === pick ? "picked" : ""}
                  onMouseDown={(e) => {
                    e.preventDefault();
                    chooseCommand(c);
                  }}
                  onMouseEnter={() => setPick(i)}
                >
                  <span>/{c.name}</span> <span className="muted">{c.mode ? `Mode: ${c.description}` : c.prompt ? `MCP prompt · ${c.description}` : c.description}</span>
                </li>
              ))}
            </ul>
          )}
          {mention && (
            <ul className="mention-picker" id={`composer-options-${session.id}`} role="listbox" aria-label="Files">
              {files === null && <li className="muted">Listing files…</li>}
              {files !== null && matches.length === 0 && <li className="muted">No matching files</li>}
              {matches.map((f, i) => (
                <li
                  key={f}
                  id={`composer-option-${i}`}
                  role="option"
                  aria-selected={i === pick}
                  className={i === pick ? "picked" : ""}
                  onMouseDown={(e) => {
                    e.preventDefault();
                    choose(f);
                  }}
                  onMouseEnter={() => setPick(i)}
                >
                  <span>{f.slice(nameStart(f))}</span> <span className="muted">{f}</span>
                </li>
              ))}
            </ul>
          )}
          <div className="composer-box">
            <textarea
              ref={input}
              aria-label="Message"
              aria-autocomplete="list"
              aria-controls={pickerOptions > 0 ? `composer-options-${session.id}` : undefined}
              aria-activedescendant={pickerOptions > 0 ? `composer-option-${pick}` : undefined}
              onPaste={(e) => {
                const files = [...e.clipboardData.files];
                if (files.length === 0) return;
                e.preventDefault();
                addFiles(files);
              }}
              placeholder={placeholder(view.running, active || !split)}
              value={draft}
              rows={1}
              onChange={(e) => {
                setDraft(e.target.value);
                if (dismissed !== null && !e.target.value.slice(dismissed).startsWith("@")) setDismissed(null);
                syncMention(e.target);
                syncSlash(e.target);
              }}
              onSelect={(e) => {
                syncMention(e.currentTarget);
                syncSlash(e.currentTarget);
              }}
              onBlur={() => {
                setMention(null);
                setSlash(null);
              }}
              onKeyDown={onKeyDown}
            />
            <div className="composer-actions">
              {/* Stop sits at the far end from Send, so a second click on Send
                  never stops the turn it started. */}
              {view.running && (
                <button onClick={() => EngineService.Cancel(session.id).catch(onError)} className="danger composer-stop icon-button" aria-label="Stop" title="Stop the turn">
                  <StopIcon />
                </button>
              )}
              <SessionControls
                workflow={
                  <label className="session-control" title="A workflow's stages are slash commands in this session: type / to see them.">
                    <span className="sr-only">Workflow</span>
                    <select value={session.workflow ?? ""} onChange={(e) => setWorkflow(e.target.value)} onFocus={loadWorkflows}>
                      <option value="">Workflow off</option>
                      {workflows.map((w) => (
                        <option key={w.id} value={w.id}>
                          {w.reason ? `${w.name} (not approved)` : w.name}
                        </option>
                      ))}
                      {session.workflow && !workflows.some((w) => w.id === session.workflow) && <option value={session.workflow}>{session.workflow} (no such workflow)</option>}
                    </select>
                  </label>
                }
                optimizer={
                  <label className="session-control" title="Runs this session's shell commands through a program that shortens their output, to spend fewer tokens. You still approve the command as the model wrote it.">
                    <span className="sr-only">Optimizer</span>
                    <select value={session.optimizer} onChange={(e) => setOptimizer(e.target.value)} onFocus={loadOptimizers}>
                      <option value="">Optimizer off</option>
                      {optimizers.map((o) => (
                        <option key={o.id} value={o.id}>
                          {o.available ? o.name : `${o.name} (not available)`}
                        </option>
                      ))}
                      {session.optimizer && !optimizers.some((o) => o.id === session.optimizer) && <option value={session.optimizer}>{session.optimizer} (no such optimizer)</option>}
                    </select>
                  </label>
                }
                sessionId={session.id}
                mode={mode}
                canBypass={canBypass}
                offersAuto={offersAuto}
                canAuto={canAuto}
                bypassOn={allowBypass}
                version={view.items.filter((i) => i.kind === "model").length + view.settings}
                onMode={setMode}
                onOtherModel={() => setSwitching({ providerId: session.providerId, model: session.model })}
                onRules={showRules}
                onError={onError}
              />
              <span className="composer-spacer" />
              {(mcp?.resources ?? []).length > 0 && (
                <button
                  title="Attach a resource of one of the session's MCP servers"
                  onClick={() => {
                    refreshMcp();
                    setAttaching(attaching === null ? "" : null);
                  }}
                >
                  MCP resource…
                </button>
              )}
              <input
                ref={picker}
                type="file"
                multiple
                hidden
                onChange={(e) => {
                  addFiles([...(e.target.files ?? [])]);
                  e.target.value = "";
                }}
              />
              <button
                className="ghost composer-attach"
                aria-label="Attach files"
                title="Attach files from anywhere on your computer: text goes in the message, pictures go to the model as pictures. You can also paste or drop them."
                onClick={() => picker.current?.click()}
              >
                <PaperclipIcon />
              </button>
              {/* The count sits on the button, so a queued message never makes the row wrap,
                  and outside it, so it stays clear while the button is dimmed. */}
              <span className="composer-send-wrap">
                <button
                  className="primary icon-button composer-send"
                  aria-label={`${view.running ? "Queue" : "Send"}${queued > 0 ? `, ${queued} queued` : ""}`}
                  title={`${view.running ? "Queue for when this turn ends" : "Send"}${queued > 0 ? ` (${queued} queued)` : ""}`}
                  onClick={send}
                  disabled={(!draft.trim() && pictures.length === 0 && textFiles.length === 0) || accepting}
                >
                  {view.running ? <QueueIcon /> : <SendIcon />}
                </button>
                {queued > 0 && (
                  <span className="composer-queued" aria-hidden="true">
                    {queued}
                  </span>
                )}
              </span>
            </div>
          </div>
        </div>
      </section>
      {panelOpen && (active || !split) && (
        <SessionPanel
          session={session}
          tab={panelTab}
          onTab={setPanelTab}
          view={view}
          provider={provider}
          environment={environment}
          configVersion={configVersion}
          costLabel={costLabel}
          terminalNonce={terminalNonce}
          onJump={(seq) => setAim({ seq, n: ++aims.current })}
          onReview={(text) => {
            // Added to what is already typed, for the user to edit and send.
            setDraft((d) => (d.trim() ? `${d.trimEnd()}\n\n${text}` : text));
            requestAnimationFrame(() => input.current?.focus());
          }}
          onClose={togglePanel}
          onError={onError}
        />
      )}
    </div>
  );
}

// ItemView re-renders only when its item changed (see Item.rev).
const ItemView = memo(
  function ItemView({
    sid,
    item,
    focused,
    cost,
    roots,
    onError,
    onRewind,
    onFork,
  }: {
    sid: string;
    item: Item;
    focused: boolean;
    cost?: string;
    roots: string[];
    onError: (err: unknown) => void;
    onRewind: (seq: number, text: string) => void;
    onFork: (seq: number) => void;
  }) {
    const cls = [item.rewound && "rewound", focused && "found"].filter(Boolean).join(" ");
    return (
      <div data-item={item.key} className={cls || undefined}>
        {item.rewound && <span className="sr-only">Rewound, no longer part of the conversation:</span>}
        <ItemBody sid={sid} item={item} cost={cost} roots={roots} onError={onError} onRewind={onRewind} onFork={onFork} />
      </div>
    );
  },
  (a, b) => a.item.key === b.item.key && a.item.rev === b.item.rev && a.focused === b.focused && a.cost === b.cost,
);

function ItemBody({
  sid,
  item,
  cost,
  roots,
  onError,
  onRewind,
  onFork,
}: {
  sid: string;
  item: Item;
  cost?: string;
  roots: string[];
  onError: (err: unknown) => void;
  onRewind: (seq: number, text: string) => void;
  onFork: (seq: number) => void;
}) {
  switch (item.kind) {
    case "user":
      return (
        <div className="user-turn">
          <div className="msg msg-user">
            <h3 className="sr-only">You</h3>
            {item.images && item.images.length > 0 && <AttachedImages sid={sid} images={item.images} />}
            {item.text}
            {item.files.length > 0 && (
              <div className="attached" role="group" aria-label="Attached files">
                {item.files.map((f) => (
                  <code key={f} title="Attached to this message">
                    {f}
                  </code>
                ))}
              </div>
            )}
          </div>
          <MessageActions
            text={item.text}
            onFork={item.rewound ? undefined : () => onFork(Number(item.key))}
            rewind={item.checkpointSeq && !item.rewound ? { filesOnly: !!item.filesOnly, run: () => onRewind(item.checkpointSeq!, item.text) } : undefined}
          />
        </div>
      );
    case "agent":
      return (
        <div className="msg msg-agent">
          <h3 className="sr-only">Agent</h3>
          <Markdown text={item.text} />
        </div>
      );
    case "thought":
      return (
        <details className="thought">
          <summary>Thinking</summary>
          <div className="msg-thought">{item.text}</div>
        </details>
      );
    case "tool":
      return <ToolView item={item} roots={roots} onError={onError} />;
    case "turn_end":
      return (
        <div className={`turn-end ${item.stopReason !== "end_turn" ? "turn-end-warn" : ""}`}>
          {/* A turn that ended as usual says only when and what it cost (GUI-042). */}
          {item.stopReason === "end_turn" ? <span className="sr-only">Turn complete · </span> : `${STOP_LABEL[item.stopReason] ?? item.stopReason} · `}
          {new Date(item.ts).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
          {cost && <span title="Estimated from the price table in Settings"> · {cost}</span>}
        </div>
      );
    case "error":
      return <div className="banner error">{item.message}</div>;
    case "stage":
      return <div className="workflow-stage-marker">Workflow stage: {item.title}</div>;
    case "mode":
      return <div className="turn-end">Mode changed to {MODES.find((m) => m.id === item.mode)?.name ?? item.mode}</div>;
    case "model":
      return <div className="turn-end">Model changed to {item.model}</div>;
    case "notice":
      return <div className={item.warn ? "turn-end turn-end-warn" : "turn-end"}>{item.message}</div>;
    case "setup":
      return <SetupCard sid={sid} item={item} onError={onError} />;
    case "pull_request":
      return (
        <div className="turn-end pr-opened">
          Opened {item.draft ? "draft " : ""}pull request{" "}
          {item.url.startsWith("https://") ? (
            <a
              href={item.url}
              title={item.url}
              onClick={(e) => {
                e.preventDefault();
                Browser.OpenURL(item.url).catch(() => {});
              }}
            >
              {item.title}
            </a>
          ) : (
            <strong>{item.title}</strong>
          )}{" "}
          ({item.head} into {item.base})
        </div>
      );
    case "handoff":
      return (
        <details className="handoff">
          <summary>
            Continues from <strong>{item.fromTitle}</strong>
            {item.generated ? "" : " (the end of its transcript; no model was available for a summary)"}
          </summary>
          {item.generated ? <Markdown text={item.summary} /> : <pre>{item.summary}</pre>}
        </details>
      );
    case "cache_resend":
      return (
        <p className="turn-end" title="Writing to the cache costs more than reading from it, and the whole context goes through it again">
          The model's cache no longer held this conversation, so {shortTokens(item.tokens)} tokens were written to it again. This usually follows a pause of a few minutes or a change of model.
        </p>
      );
    case "compaction":
      return (
        <details className="compaction">
          <summary className="turn-end">
            Context compacted{item.reason === "manual" ? "" : " automatically"}: {item.before.toLocaleString()} → about {item.after.toLocaleString()} tokens
          </summary>
          <div className="md">
            <Markdown text={item.summary} />
          </div>
        </details>
      );
  }
}

// MessageActions are the icon buttons under a message of the user's: copy
// it, rewind the files (and the conversation) to before it, which asks once
// more (SES-014), and fork a new session from it (SES-009). They show when
// the message is pointed at or focused.
function MessageActions({ text, onFork, rewind }: { text: string; onFork?: () => void; rewind?: { filesOnly: boolean; run: () => void } }) {
  const [copied, setCopied] = useState(false);
  const [sure, setSure] = useState(false);
  const copy = () =>
    Clipboard.SetText(text)
      .then(() => {
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
      })
      .catch(() => {});
  if (sure && rewind) {
    return (
      <div className="msg-actions msg-actions-open" role="group" aria-label="Rewind to before this message">
        <span className="muted">{rewind.filesOnly ? "Restore the files to before this message?" : "Restore the files and the conversation to before this message?"}</span>
        <button className="danger" onClick={rewind.run} title={rewind.filesOnly ? "The conversation was compacted or rewound since, so the agent keeps its context" : undefined}>
          Rewind
        </button>
        <button onClick={() => setSure(false)}>Keep</button>
      </div>
    );
  }
  return (
    <div className="msg-actions" role="group" aria-label="Message actions">
      <button className="icon-button" onClick={copy} title={copied ? "Copied" : "Copy"} aria-label={copied ? "Copied" : "Copy message"}>
        {copied ? <CheckIcon /> : <CopyIcon />}
      </button>
      {rewind && (
        <button className="icon-button" onClick={() => setSure(true)} title="Rewind to here: restore the files to before this message" aria-label="Rewind to here">
          <RewindIcon />
        </button>
      )}
      {onFork && (
        <button className="icon-button" onClick={onFork} title="Fork: a new session with the conversation up to this message" aria-label="Fork from here">
          <ForkIcon />
        </button>
      )}
    </div>
  );
}

let pictureCount = 0;

// SetupCard shows the setup of the session's own worktree (SES-016): what
// was copied and run, the end of the output, and after a failure the
// choice to retry or start without it.
function SetupCard({ sid, item, onError }: { sid: string; item: Extract<Item, { kind: "setup" }>; onError: (err: unknown) => void }) {
  const [busy, setBusy] = useState(false);
  const act = (p: Promise<unknown>) => {
    setBusy(true);
    p.catch(onError).finally(() => setBusy(false));
  };
  const parts = [
    item.copied.length > 0 && `copied ${item.copied.join(", ")}`,
    item.missing.length > 0 && `not in the main checkout: ${item.missing.join(", ")}`,
  ].filter(Boolean);
  const title =
    item.status === "running"
      ? "Setting up the worktree…"
      : item.status === "done"
        ? "Worktree set up"
        : item.status === "skipped"
          ? "Started without finishing the worktree setup"
          : "The worktree setup failed";
  return (
    <div className={`setup-card setup-${item.status}`} role={item.status === "failed" ? "alert" : undefined}>
      <div>
        <strong>{title}</strong>
        {parts.length > 0 && <span className="muted"> · {parts.join(" · ")}</span>}
      </div>
      {item.command && (
        <div className="muted">
          <code>{item.command}</code>
        </div>
      )}
      {item.error && <div className="error-text">{item.error}</div>}
      {item.output && (
        <details open={item.status === "failed"}>
          <summary className="muted">Output</summary>
          <pre className="setup-output">{item.output}</pre>
        </details>
      )}
      {item.status === "failed" && (
        <div className="row">
          <button className="primary" disabled={busy} onClick={() => act(EngineService.RetrySetup(sid))}>
            Retry
          </button>
          <button disabled={busy} onClick={() => act(EngineService.SkipSetup(sid))}>
            Start anyway
          </button>
        </div>
      )}
    </div>
  );
}

// TodoList is the agent's latest to-do list (HRN-017), from the built-in
// harness or any external agent's plan. It folds to one line once the turn
// is over. A finished list shows only during the turn that finished it.
function TodoList({ entries, running, thisTurn }: { entries: { content: string; status: string }[]; running: boolean; thisTurn: boolean }) {
  const done = entries.filter((e) => e.status === "completed").length;
  const current = entries.find((e) => e.status === "in_progress");
  if (done === entries.length && (!running || !thisTurn)) return null;
  return (
    <details className="todo-list" open={running}>
      <summary>
        To-do · {done} of {entries.length} done{current ? ` · ${current.content}` : ""}
      </summary>
      <ul>
        {entries.map((e, i) => (
          <li key={i} className={`todo-${e.status}`}>
            <span aria-hidden="true">{e.status === "completed" ? "✓" : e.status === "in_progress" ? "•" : "○"}</span> {e.content}
          </li>
        ))}
      </ul>
    </details>
  );
}
