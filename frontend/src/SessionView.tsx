import { Browser, Clipboard } from "@wailsio/runtime";
import { memo, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { insertMention, mentionAt, rankFiles, type Mention } from "./mentions.ts";
import { matchCommands, MODE_COMMANDS, modeCommand, slashQuery, type SlashCommand } from "./commands.ts";
import {
  EngineService,
  MODES,
  type EnvironmentInfo,
  type OptimizerInfo,
  type HarnessInfo,
  type ProjectInfo,
  type ProviderInfo,
  type Hook,
  type Profile,
  type ResourceRef,
  type Session,
  type SessionMCPContext,
} from "./api.ts";
import {
  awaitingDecision,
  buildView,
  type Item,
  type TranscriptEvent,
} from "./transcript.ts";
import { pickLabel } from "./a11y.ts";
import AttachedImages from "./AttachedImages.tsx";
import ToolView, { openSessionEvent } from "./ToolView.tsx";
import Tags from "./Tags.tsx";
import { prepareImage, type PreparedImage } from "./imageprep.ts";
import { MAX_IMAGES, room } from "./images.ts";
import { CheckIcon, CopyIcon, ForkIcon, RewindIcon, StopIcon } from "./icons.tsx";
import { Markdown } from "./Markdown.tsx";
import SessionPanel, { showTool, type Tab as PanelTab } from "./SessionPanel.tsx";
import { usePaletteActions } from "./Palette.tsx";
import type { PaletteItem } from "./palette.ts";
import { itemForSeq } from "./search.ts";
import { estimate, formatCost, type PriceEntry } from "./cost.ts";

interface Props {
  session: Session;
  events: TranscriptEvent[];
  // A search hit to scroll to (SES-007); n tells repeated jumps apart.
  focus?: { seq: number; n: number };
  // The price table for cost estimates (PRV-008).
  prices: PriceEntry[];
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

export default function SessionView({ session, events, focus, prices, initialDraft, onForked, profiles, provider, providers, environment, harness, project, onProjectsChanged, allowBypass, configVersion, onError, onDeleted, onOpenWindow, knownTags = [], onOpenCompare, active = true, onClosePane }: Props) {
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
  const [trustHooks, setTrustHooks] = useState<Hook[]>([]);
  const untrusted = !!project && !project.trusted && !project.missing;
  const canBypass = allowBypass && !!project?.trusted;
  useEffect(() => {
    if (!untrusted || !project) return setTrustHooks([]);
    EngineService.ProjectSettings(project.root)
      .then((s) => setTrustHooks(s.hooks ?? []))
      .catch(() => setTrustHooks([]));
  }, [untrusted, project?.root]);
  const trustProject = (ask: boolean) => {
    if (!project) return;
    const list = trustHooks.map((h) => `${h.event}: ${h.command}`).join("\n");
    if (ask && trustHooks.length > 0 && !confirm(`Trusting ${project.name} lets it run these commands (hooks):\n\n${list}`)) return;
    EngineService.SetProjectTrust(project.root, true).then(onProjectsChanged).catch(onError);
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
  const archived = session.status === "archived";
  const archive = () => EngineService.ArchiveSession(session.id, !archived).catch(onError);
  const remove = () => {
    if (!confirm(`Delete "${session.title}"? Its transcript is removed from disk. This cannot be undone.`)) return;
    EngineService.DeleteSession(session.id).then(onDeleted).catch(onError);
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
    return { session: estimate(prices, view.calls, own), calls: view.calls.length, turns };
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
  const [draft, setDraft] = useState(initialDraft ?? "");
  const [queued, setQueued] = useState(0);
  const bottom = useRef<HTMLDivElement>(null);
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
  const commands = useMemo<SlashCommand[]>(
    () => [
      ...MODE_COMMANDS.filter((c) => c.mode !== "bypass" || canBypass),
      ...view.commands.map((c) => ({ name: c.name, description: c.description })),
      ...(mcp?.prompts ?? []).map((p) => ({
        name: p.command,
        description: p.description || p.title || `Prompt from ${p.server}`,
        prompt: { server: p.server, name: p.name, arguments: p.arguments ?? [] },
      })),
    ],
    [view.commands, allowBypass, mcp],
  );
  // Resources attached to the next message, and the open prompt form.
  const [attached, setAttached] = useState<ResourceRef[]>([]);
  // Pictures for the next message (SES-015), already scaled down.
  const [pictures, setPictures] = useState<(PreparedImage & { id: number })[]>([]);
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
  const [attaching, setAttaching] = useState<string | null>(null);
  const [promptForm, setPromptForm] = useState<{ command: SlashCommand; values: Record<string, string> } | null>(null);
  const runPrompt = (c: SlashCommand, values: Record<string, string>) => {
    if (!c.prompt) return;
    holdBottom.current = false;
    EngineService.RunMCPPrompt(session.id, c.prompt.server, c.prompt.name, values)
      .then((r) => {
        setPromptForm(null);
        setDraft("");
        if (r.queued) setQueued((n) => n + 1);
      })
      .catch(onError);
  };
  const slashMatches = useMemo(() => (slash === null ? [] : matchCommands(slash, commands)), [slash, commands]);
  // The focus stays in the message box while a picker is open, so the entry
  // that lights up is announced.
  const pickerNames = slash !== null && slashMatches.length > 0 ? slashMatches.map((c) => c.name) : mention ? matches.map((f) => f.slice(f.lastIndexOf("/") + 1)) : [];
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

  // Follow the conversation as it grows, unless a search hit is shown.
  const holdBottom = useRef(false);
  useEffect(() => {
    if (!holdBottom.current) bottom.current?.scrollIntoView({ block: "end" });
  }, [view.items.length, view.items[view.items.length - 1]]);

  // Scroll to a search hit once its item is there and mark it for a moment.
  const [focused, setFocused] = useState<string | null>(null);
  const shown = useRef(0);
  useEffect(() => {
    if (!focus || shown.current === focus.n) return;
    const key = itemForSeq(view.items, focus.seq);
    const el = key && layoutBox.current?.querySelector<HTMLElement>(`[data-item="${key}"]`);
    if (!el) return;
    shown.current = focus.n;
    holdBottom.current = true;
    // Items above have estimated heights until rendered; scroll again once
    // they settle.
    el.scrollIntoView({ block: "center" });
    setTimeout(() => el.scrollIntoView({ block: "center" }), 80);
    setFocused(key);
    const t = setTimeout(() => setFocused(null), 2500);
    return () => clearTimeout(t);
  }, [focus, view.items]);

  useEffect(() => {
    if (!view.running) setQueued(0);
  }, [view.running]);

  const [accepting, setAccepting] = useState(false);
  const send = () => {
    const text = draft.trim();
    if ((!text && pictures.length === 0) || accepting) return;
    const m = modeCommand(text);
    if (m && (m !== "bypass" || canBypass)) {
      setDraft("");
      setMode(m);
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
    (sending.length > 0 || images.length > 0 ? EngineService.PromptWith(session.id, { text, resources: sending, images }) : EngineService.Prompt(session.id, text))
      .then((r) => {
        setDraft("");
        setAttached([]);
        pictures.forEach((p) => URL.revokeObjectURL(p.url));
        setPictures([]);
        if (r.queued) setQueued((n) => n + 1);
      })
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
      if ((e.key === "Enter" && !e.metaKey && !e.shiftKey) || e.key === "Tab") {
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
      if ((e.key === "Enter" && !e.metaKey) || e.key === "Tab") {
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
    // ⌘↩ answers a waiting approval (see shortcuts.ts) instead of sending.
    if (e.metaKey && approvalWaiting) return;
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
  const setOptimizer = (id: string) => {
    const chosen = optimizers.find((o) => o.id === id);
    if (chosen && !chosen.available) return setOptimizerHelp(chosen);
    setOptimizerHelp(null);
    EngineService.SetOptimizer(session.id, id).catch(onError);
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
  const total = view.usage.inputTokens + view.usage.cacheReadTokens + view.usage.cacheWriteTokens;

  // Everything this view shows a control for, for the palette (GUI-014).
  const showPanel = (t: PanelTab) => {
    setPanelTab(t);
    if (!panelOpen) togglePanel();
  };
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
    group: "Session",
    detail: session.title,
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
    ...MODES.filter((m) => m.id !== mode && (m.id !== "bypass" || canBypass)).map((m) => act(`mode:${m.id}`, `Mode: ${m.name}`, () => setMode(m.id), { keywords: "permission" })),
    ...(session.providerId ? [act("model", "Switch model…", () => setSwitching({ providerId: session.providerId, model: session.model }), { keywords: "provider" })] : []),
    ...[{ id: "", name: "Off" }, ...optimizers]
      .filter((o) => o.id !== session.optimizer)
      .map((o) => act(`optimizer:${o.id}`, `Token optimizer: ${o.name}`, () => setOptimizer(o.id), { keywords: "tokens rtk compress output" })),
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
      act(`rewind:${u.key}`, n === 0 ? "Rewind to before the last message…" : `Rewind to before “${u.text.slice(0, 60)}”…`, () => {
        if (confirm("Restore the files to before this message and continue from there?")) rewind(u.checkpointSeq!, u.text);
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
    act("delete", "Delete session…", remove, { keywords: "remove" }),
  ]);

  // The header's More menu closes once an action is picked.
  const moreMenu = useRef<HTMLDetailsElement>(null);
  const fromMenu = (act: () => void) => () => {
    if (moreMenu.current) moreMenu.current.open = false;
    act();
  };

  return (
    <div ref={layoutBox} className="session-layout">
      <section className="session">
        <header className="session-header">
          <div>
            {renaming === null ? (
              <h2 onDoubleClick={() => setRenaming(session.title)} title="Double-click to rename">
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
            {session.parentId && (
              <p className="muted">
                A subagent.{" "}
                <button className="link" onClick={() => openSessionEvent(session.parentId)}>
                  Open the session that started it
                </button>
              </p>
            )}
            <p className="muted">
              <code className="session-path" title={session.worktree ? `Worktree of ${session.projectRoot}: ${session.worktree}` : session.projectRoot}>
                {session.worktree || session.projectRoot}
              </code>
              {view.git?.repo && (
                <span title={`HEAD ${view.git.head.slice(0, 10)}, started at ${view.git.baseCommit.slice(0, 10)}`}>
                  {" "}
                  · {view.git.branch || "detached"}
                  {session.worktree ? " (own worktree)" : ""}
                  {view.git.dirty > 0 ? ` · ${view.git.dirty} changed` : view.git.dirty < 0 ? " · changes not checked" : " · clean"}
                </span>
              )}
              {" "}· env {environment?.name ?? session.environmentId ?? "Local"}
              {session.portBase > 0 && (
                <span title="This session's commands get FREELOOP_PORT, the first of its ten ports">
                  {" "}
                  · ports {session.portBase}–{session.portBase + 9}
                </span>
              )}
            </p>
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
          <div className="session-controls">
            <div className="row">
              {otherProfiles.length > 0 && (
                <details className="export-menu">
                  <summary title="A new session with another profile or harness, told what happened here">{switchingTo ? `Handing over to ${switchingTo}…` : "Continue with…"}</summary>
                  <div className="export-options">
                    {otherProfiles.map((p) => (
                      <button key={p.id} disabled={!!switchingTo || view.running} onClick={() => continueWith(p)}>
                        {p.name}
                      </button>
                    ))}
                  </div>
                </details>
              )}
              {runCommand && (
                <button title={`Runs ${runCommand} in a terminal tab`} disabled={archived} onClick={runApp}>
                  Run
                </button>
              )}
              <details className="export-menu" ref={moreMenu}>
                <summary aria-label="More actions for this session">More</summary>
                <div className="export-options">
                  <button onClick={fromMenu(() => setRenaming(session.title))}>Rename</button>
                  <button onClick={fromMenu(() => setTagging(true))}>Add a tag</button>
                  {onOpenWindow && <button onClick={fromMenu(onOpenWindow)}>Open in a new window</button>}
                  <button onClick={fromMenu(() => exportAs("markdown"))}>Export as Markdown…</button>
                  <button onClick={fromMenu(() => exportAs("json"))}>Export as JSON…</button>
                  <button onClick={fromMenu(archive)}>{archived ? "Unarchive" : "Archive"}</button>
                  <hr />
                  <button className="danger" onClick={fromMenu(remove)}>
                    Delete…
                  </button>
                </div>
              </details>
              <button className="ghost" onClick={togglePanel} aria-pressed={panelOpen && (active || !split)} title="Show or hide the right panel">
                Panel
              </button>
              {onClosePane && (
                <button className="ghost pane-close" onClick={onClosePane} aria-label="Close this pane" title="Close this pane">
                  ×
                </button>
              )}
              {exported && (
                <span className="ok" role="status" title={exported}>
                  Exported
                </span>
              )}
            </div>
            <span className="muted" title="Provider and model">
              {session.providerId ? `${provider?.name ?? session.providerId} · ${session.model}` : (harness?.name ?? session.harnessId)}
              {session.providerId && (
                <button className="link" onClick={() => setSwitching(switching ? null : { providerId: session.providerId, model: session.model })}>
                  {" "}
                  Switch…
                </button>
              )}
            </span>
            <label>
              Mode{" "}
              <select value={mode} onChange={(e) => setMode(e.target.value)}>
                {MODES.filter((m) => m.id !== "bypass" || canBypass || mode === "bypass").map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.name}
                  </option>
                ))}
              </select>
            </label>
            <label title="Runs this session's shell commands through a program that shortens their output, to spend fewer tokens. You still approve the command as the model wrote it.">
              Optimizer{" "}
              <select value={session.optimizer} onChange={(e) => setOptimizer(e.target.value)} onFocus={loadOptimizers}>
                <option value="">Off</option>
                {optimizers.map((o) => (
                  <option key={o.id} value={o.id}>
                    {o.available ? o.name : `${o.name} (not available)`}
                  </option>
                ))}
                {session.optimizer && !optimizers.some((o) => o.id === session.optimizer) && <option value={session.optimizer}>{session.optimizer} (no such optimizer)</option>}
              </select>
            </label>
            {view.context && (
              <span
                className={view.context.window > 0 && view.context.tokens >= 0.8 * view.context.window ? "context-meter context-high" : "context-meter muted"}
                title="Context of the latest model call; the session compacts at 80 % of the model's window, or on /compact"
              >
                context {view.context.window > 0 ? `${Math.round((100 * view.context.tokens) / view.context.window)} % of ${Math.round(view.context.window / 1000)}k` : `${Math.round(view.context.tokens / 1000)}k tokens`}
              </span>
            )}
            <span className="muted" title="Input (including cache reads and writes) / output tokens">
              {total.toLocaleString()} in · {view.usage.outputTokens.toLocaleString()} out
            </span>
            {costs.calls > 0 && (
              <span
                className="muted"
                title={
                  costs.session.unpriced === costs.calls
                    ? "No price for this model; add one in Settings to estimate the cost"
                    : `Estimated from the price table in Settings${costs.session.unpriced ? `; ${costs.session.unpriced} calls to models without a price are left out` : ""}`
                }
              >
                {costs.session.unpriced === costs.calls ? "cost: no price" : `≈ ${formatCost(costs.session.cost)}${costs.session.unpriced ? "+" : ""}`}
              </span>
            )}
          </div>
        </header>
        {optimizerHelp && (
          <div className="banner row" role="status">
            <span>{optimizerHelp.reason}</span>
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
        {untrusted && (
          <div className="banner warn row" role="status">
            <span>
              Untrusted project: this session uses your global settings only, without the project's <code>AGENTS.md</code>,{" "}
              <code>.freeloop/</code> settings or git status.
              {trustHooks.length > 0 && (
                <>
                  {" "}
                  Trusting it lets it run these commands (hooks):
                  <ul>
                    {trustHooks.map((h, n) => (
                      <li key={n}>
                        {h.event}: <code>{h.command}</code>
                      </li>
                    ))}
                  </ul>
                </>
              )}
            </span>
            <button onClick={() => trustProject(false)}>Trust project</button>
          </div>
        )}
        {mode === "bypass" && (
          <div className="banner error" role="status">
            Bypass mode: tool calls run without asking. Protected paths are still refused.
          </div>
        )}

        <div className="sr-only" role="status" aria-live="polite">
          {active && announcement}
        </div>
        <div className="conversation" role="region" aria-label="Conversation">
          {view.items.length === 0 && <p className="muted empty">Describe a task to start.</p>}
          {view.items.map((item) => (
            <ItemView key={item.key} sid={session.id} item={item} focused={item.key === focused} cost={costs.turns.get(item.key)} onError={onError} onRewind={rewind} onFork={fork} />
          ))}
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
          <div ref={bottom} />
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
            addPictures([...e.dataTransfer.files]);
          }}
        >
          {view.plan.length > 0 && <TodoList entries={view.plan} running={view.running} />}
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
                    ×
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
                  <button className="link" aria-label={`Remove ${a.name || a.uri}`} onClick={() => setAttached(attached.filter((x) => x !== a))}>
                    ×
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
                  <span>{f.slice(f.lastIndexOf("/") + 1)}</span> <span className="muted">{f}</span>
                </li>
              ))}
            </ul>
          )}
          <textarea
            ref={input}
            aria-label="Message"
            aria-autocomplete="list"
            aria-controls={pickerOptions > 0 ? `composer-options-${session.id}` : undefined}
            aria-activedescendant={pickerOptions > 0 ? `composer-option-${pick}` : undefined}
            onPaste={(e) => {
              const files = [...e.clipboardData.files].filter((f) => f.type.startsWith("image/"));
              if (files.length === 0) return;
              e.preventDefault();
              addPictures(files);
            }}
            placeholder={view.running ? "Queue a message for when this turn ends" : "Ask for a change, a fix, an explanation… Type @ to attach a file, / for commands."}
            value={draft}
            rows={3}
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
              <button onClick={() => EngineService.Cancel(session.id).catch(onError)} className="danger composer-stop" title="Stop the turn">
                <StopIcon /> Stop
              </button>
            )}
            <span className="composer-spacer" />
            {queued > 0 && <span className="muted">{queued} queued</span>}
            {(mcp?.resources ?? []).length > 0 && (
              <button
                title="Attach a resource of one of the session's MCP servers"
                onClick={() => {
                  refreshMcp();
                  setAttaching(attaching === null ? "" : null);
                }}
              >
                Attach…
              </button>
            )}
            <input
              ref={picker}
              type="file"
              accept="image/png,image/jpeg,image/gif,image/webp"
              multiple
              hidden
              onChange={(e) => {
                addPictures([...(e.target.files ?? [])]);
                e.target.value = "";
              }}
            />
            <button title="Attach pictures; you can also paste or drop them" disabled={pictures.length >= MAX_IMAGES} onClick={() => picker.current?.click()}>
              Picture…
            </button>
            <button className="primary" onClick={send} disabled={(!draft.trim() && pictures.length === 0) || accepting}>
              {view.running ? "Queue" : "Send"}
            </button>
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
          terminalNonce={terminalNonce}
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
    onError,
    onRewind,
    onFork,
  }: {
    sid: string;
    item: Item;
    focused: boolean;
    cost?: string;
    onError: (err: unknown) => void;
    onRewind: (seq: number, text: string) => void;
    onFork: (seq: number) => void;
  }) {
    const cls = [item.rewound && "rewound", focused && "found"].filter(Boolean).join(" ");
    return (
      <div data-item={item.key} className={cls || undefined}>
        {item.rewound && <span className="sr-only">Rewound, no longer part of the conversation:</span>}
        <ItemBody sid={sid} item={item} cost={cost} onError={onError} onRewind={onRewind} onFork={onFork} />
      </div>
    );
  },
  (a, b) => a.item.key === b.item.key && a.item.rev === b.item.rev && a.focused === b.focused && a.cost === b.cost,
);

function ItemBody({
  sid,
  item,
  cost,
  onError,
  onRewind,
  onFork,
}: {
  sid: string;
  item: Item;
  cost?: string;
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
      return <ToolView item={item} onError={onError} />;
    case "turn_end":
      return (
        <div className={`turn-end ${item.stopReason !== "end_turn" ? "turn-end-warn" : ""}`}>
          {STOP_LABEL[item.stopReason] ?? item.stopReason} · {new Date(item.ts).toLocaleTimeString()}
          {cost && <span title="Estimated from the price table in Settings"> · {cost}</span>}
        </div>
      );
    case "error":
      return <div className="banner error">{item.message}</div>;
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
// is over.
function TodoList({ entries, running }: { entries: { content: string; status: string }[]; running: boolean }) {
  const done = entries.filter((e) => e.status === "completed").length;
  const current = entries.find((e) => e.status === "in_progress");
  if (!running && done === entries.length) return null;
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
