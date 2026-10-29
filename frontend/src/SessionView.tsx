import { memo, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { insertMention, mentionAt, rankFiles, type Mention } from "./mentions.ts";
import { matchCommands, MODE_COMMANDS, modeCommand, slashQuery, type SlashCommand } from "./commands.ts";
import { APPROVAL_GRACE_MS, APPROVAL_HINTS, approvalShortcut, isTextField, optionKind } from "./shortcuts.ts";
import { EngineService, MODES, type EnvironmentInfo, type HarnessInfo, type ProjectInfo, type ProviderInfo, type Session } from "./api.ts";
import { awaitingDecision, buildView, lineDiff, type Item, type ToolContent, type TranscriptEvent } from "./transcript.ts";
import { Markdown } from "./Markdown.tsx";
import SessionPanel from "./SessionPanel.tsx";

interface Props {
  session: Session;
  events: TranscriptEvent[];
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

export default function SessionView({ session, events, provider, providers, environment, harness, project, onProjectsChanged, allowBypass, configVersion, onError, onDeleted }: Props) {
  const [panelOpen, setPanelOpen] = useState(panelOpenInitially);
  const togglePanel = () =>
    setPanelOpen((open) => {
      try {
        localStorage.setItem(PANEL_KEY, open ? "0" : "1");
      } catch {
        // not remembered
      }
      return !open;
    });
  const [renaming, setRenaming] = useState<string | null>(null);
  // The model switch form (PRV-006).
  const [switching, setSwitching] = useState<{ providerId: string; model: string } | null>(null);
  const rename = () => {
    if (renaming === null) return;
    EngineService.RenameSession(session.id, renaming)
      .then(() => setRenaming(null))
      .catch(onError);
  };
  const archived = session.status === "archived";
  const archive = () => EngineService.ArchiveSession(session.id, !archived).catch(onError);
  const remove = () => {
    if (!confirm(`Delete "${session.title}"? Its transcript is removed from disk. This cannot be undone.`)) return;
    EngineService.DeleteSession(session.id).then(onDeleted).catch(onError);
  };
  const view = useMemo(() => buildView(events), [events]);

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
  const [draft, setDraft] = useState("");
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
  const commands = useMemo<SlashCommand[]>(
    () => [...MODE_COMMANDS.filter((c) => c.mode !== "bypass" || allowBypass), ...view.commands.map((c) => ({ name: c.name, description: c.description }))],
    [view.commands, allowBypass],
  );
  const slashMatches = useMemo(() => (slash === null ? [] : matchCommands(slash, commands)), [slash, commands]);
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

  // Follow the conversation as it grows.
  useEffect(() => {
    bottom.current?.scrollIntoView({ block: "end" });
  }, [view.items.length, view.items[view.items.length - 1]]);

  useEffect(() => {
    if (!view.running) setQueued(0);
  }, [view.running]);

  const send = () => {
    const text = draft.trim();
    if (!text) return;
    const m = modeCommand(text);
    if (m && (m !== "bypass" || allowBypass)) {
      setDraft("");
      setMode(m);
      return;
    }
    EngineService.Prompt(session.id, text)
      .then((r) => {
        setDraft("");
        if (r.queued) setQueued((n) => n + 1);
      })
      .catch(onError);
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
  const total = view.usage.inputTokens + view.usage.cacheReadTokens + view.usage.cacheWriteTokens;

  return (
    <div className="session-layout">
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
            <p className="muted">
              <code title={session.worktree ? `Worktree of ${session.projectRoot}` : undefined}>{session.worktree || session.projectRoot}</code>
              {view.git?.repo && (
                <span title={`HEAD ${view.git.head.slice(0, 10)}, started at ${view.git.baseCommit.slice(0, 10)}`}>
                  {" "}
                  · {view.git.branch || "detached"}
                  {session.worktree ? " (own worktree)" : ""}
                  {view.git.dirty > 0 ? ` · ${view.git.dirty} changed` : view.git.dirty < 0 ? " · changes not checked" : " · clean"}
                </span>
              )}
              {" "}· env {environment?.name ?? session.environmentId ?? "Local"}
            </p>
          </div>
          <div className="session-controls">
            <div className="row">
              <button onClick={() => setRenaming(session.title)}>Rename</button>
              <button onClick={archive}>{archived ? "Unarchive" : "Archive"}</button>
              <button className="danger" onClick={remove}>
                Delete
              </button>
              <button onClick={togglePanel} aria-pressed={panelOpen} title="Show or hide the right panel">
                Panel
              </button>
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
                {MODES.filter((m) => m.id !== "bypass" || allowBypass || mode === "bypass").map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.name}
                  </option>
                ))}
              </select>
            </label>
            <span className="muted" title="Input (including cache reads and writes) / output tokens">
              {total.toLocaleString()} in · {view.usage.outputTokens.toLocaleString()} out
            </span>
          </div>
        </header>
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
        {project && !project.trusted && !project.missing && (
          <div className="banner warn row" role="status">
            <span>
              Untrusted project: this session uses your global settings only, without the project's <code>AGENTS.md</code>,{" "}
              <code>.freeloop/</code> settings or git status.
            </span>
            <button
              onClick={() =>
                EngineService.SetProjectTrust(project.root, true)
                  .then(onProjectsChanged)
                  .catch(onError)
              }
            >
              Trust project
            </button>
          </div>
        )}
        {mode === "bypass" && (
          <div className="banner error" role="status">
            Bypass mode: tool calls run without asking. Protected paths are still refused.
          </div>
        )}

        <div className="sr-only" role="status" aria-live="polite">
          {announcement}
        </div>
        <div className="conversation" aria-label="Conversation">
          {view.items.length === 0 && <p className="muted empty">Describe a task to start.</p>}
          {view.items.map((item) => (
            <ItemView key={item.key} item={item} onError={onError} />
          ))}
          {view.running && (
            <p className="muted working">{view.status === "queued" ? "Waiting for a free slot on the provider…" : "Working…"}</p>
          )}
          <div ref={bottom} />
        </div>

        <div className="composer">
          {slash !== null && slashMatches.length > 0 && (
            <ul className="mention-picker" role="listbox" aria-label="Commands">
              {slashMatches.map((c, i) => (
                <li
                  key={c.name}
                  role="option"
                  aria-selected={i === pick}
                  className={i === pick ? "picked" : ""}
                  onMouseDown={(e) => {
                    e.preventDefault();
                    chooseCommand(c);
                  }}
                  onMouseEnter={() => setPick(i)}
                >
                  <span>/{c.name}</span> <span className="muted">{c.mode ? `Mode: ${c.description}` : c.description}</span>
                </li>
              ))}
            </ul>
          )}
          {mention && (
            <ul className="mention-picker" role="listbox" aria-label="Files">
              {files === null && <li className="muted">Listing files…</li>}
              {files !== null && matches.length === 0 && <li className="muted">No matching files</li>}
              {matches.map((f, i) => (
                <li
                  key={f}
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
            {queued > 0 && <span className="muted">{queued} queued</span>}
            {view.running && (
              <button onClick={() => EngineService.Cancel(session.id).catch(onError)} className="danger">
                Stop
              </button>
            )}
            <button className="primary" onClick={send} disabled={!draft.trim()}>
              {view.running ? "Queue" : "Send"}
            </button>
          </div>
        </div>
      </section>
      {panelOpen && (
        <SessionPanel
          session={session}
          view={view}
          provider={provider}
          environment={environment}
          configVersion={configVersion}
          onClose={togglePanel}
          onError={onError}
        />
      )}
    </div>
  );
}

// ItemView re-renders only when its item changed (see Item.rev).
const ItemView = memo(
  function ItemView({ item, onError }: { item: Item; onError: (err: unknown) => void }) {
    return <ItemBody item={item} onError={onError} />;
  },
  (a, b) => a.item.key === b.item.key && a.item.rev === b.item.rev,
);

function ItemBody({ item, onError }: { item: Item; onError: (err: unknown) => void }) {
  switch (item.kind) {
    case "user":
      return (
        <div className="msg msg-user">
          {item.text}
          {item.files.length > 0 && (
            <div className="attached" aria-label="Attached files">
              {item.files.map((f) => (
                <code key={f} title="Attached to this message">
                  {f}
                </code>
              ))}
            </div>
          )}
        </div>
      );
    case "agent":
      return (
        <div className="msg msg-agent">
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
        </div>
      );
    case "error":
      return <div className="banner error">{item.message}</div>;
    case "mode":
      return <div className="turn-end">Mode changed to {MODES.find((m) => m.id === item.mode)?.name ?? item.mode}</div>;
    case "model":
      return <div className="turn-end">Model changed to {item.model}</div>;
    case "notice":
      return <div className="turn-end turn-end-warn">{item.message}</div>;
  }
}

function ToolView({ item, onError }: { item: Extract<Item, { kind: "tool" }>; onError: (err: unknown) => void }) {
  const waiting = awaitingDecision(item);
  const [reason, setReason] = useState("");
  const [scope, setScope] = useState("session");
  const decided = item.permission?.decided;
  const shownDecision = decided && decided.actor !== "policy" && !(decided.actor === "mode" && decided.outcome === "allowed");

  const decide = (optionId: string) =>
    EngineService.Decide({ requestId: item.permission!.requestId, optionId, reason, scope }).catch(onError);
  const offersAlways = item.permission?.options.some((o) => o.kind === "allow_always");

  // Shortcuts (GUI-006) go to the first waiting card on screen only.
  const box = useRef<HTMLDivElement>(null);
  const shownAt = useRef(0);
  useEffect(() => {
    if (!waiting) return;
    shownAt.current = Date.now();
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (e.repeat || e.defaultPrevented || e.isComposing) return;
      if (document.querySelector(".approval") !== box.current) return;
      const kind = approvalShortcut(e, isTextField(e.target));
      const opt = kind && item.permission?.options.find((o) => optionKind(o.kind) === kind);
      if (!opt) return;
      e.preventDefault();
      if (Date.now() - shownAt.current < APPROVAL_GRACE_MS) return;
      decide(opt.optionId);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [waiting, item.permission, reason, scope]);

  const plan = item.tool === "present_plan" ? String((item.input as { plan?: string } | null)?.plan ?? "") : null;

  return (
    <details
      id={`tool-${item.toolCallId}`}
      className={`tool tool-${item.status} ${waiting ? "tool-waiting" : ""}`}
      open={waiting || plan !== null || undefined}
    >
      <summary>
        <span className={`dot dot-${item.status}`} aria-hidden="true" />
        <span className="tool-title">{item.title}</span>
        <span className="muted"> {item.tool}</span>
        {shownDecision && (
          <span className={`decision decision-${decided.outcome}`}>
            {decided.outcome} by {decided.actor}
          </span>
        )}
      </summary>
      {plan !== null ? (
        <div className="plan">{plan}</div>
      ) : (
        <pre className="tool-input">{JSON.stringify(item.input, null, 2)}</pre>
      )}
      {item.content.map((c, i) => (
        <ContentView key={i} c={c} />
      ))}
      {shownDecision && decided.reason && <p className="muted">{decided.reason}</p>}
      {decided?.persisted && (
        <p className="muted">
          Saved rule: <code>{decided.persisted.command || decided.persisted.path || decided.persisted.tool}</code> ({decided.persisted.scope})
        </p>
      )}
      {waiting && (
        <div className="approval" role="group" aria-label="Approve this tool call" ref={box}>
          {item.permission?.command && (
            <p>
              Run <code>{item.permission.command}</code>?
            </p>
          )}
          {item.permission?.paths?.map((p) => (
            <p key={p} className="muted">
              <code>{p}</code>
            </p>
          ))}
          <input placeholder="Optional note for the agent" value={reason} onChange={(e) => setReason(e.target.value)} />
          <div className="approval-buttons">
            {offersAlways && (
              <select value={scope} onChange={(e) => setScope(e.target.value)} aria-label="Allow always applies to">
                <option value="session">Always: this session</option>
                <option value="global">Always: everywhere</option>
              </select>
            )}
            {item.permission!.options.map((o) => {
              const kind = optionKind(o.kind);
              const hint = kind ? APPROVAL_HINTS[kind] : null;
              return (
                <button
                  key={o.optionId}
                  className={o.kind.startsWith("allow") ? "primary" : "danger"}
                  onClick={() => decide(o.optionId)}
                  title={hint ? `Shortcut: ${hint}` : undefined}
                >
                  {o.name}
                  {hint && <kbd>{hint.split(" ")[0]}</kbd>}
                </button>
              );
            })}
          </div>
        </div>
      )}
    </details>
  );
}

function ContentView({ c }: { c: ToolContent }) {
  if (c.type === "diff") {
    return (
      <div className="diff">
        <div className="muted">{c.path}</div>
        <pre>
          {lineDiff(c.oldText ?? "", c.newText ?? "").map((l, i) => (
            <div key={i} className={l.sign === "+" ? "add" : l.sign === "-" ? "del" : ""}>
              {l.sign} {l.text}
            </div>
          ))}
        </pre>
      </div>
    );
  }
  if (c.type === "terminal_output") {
    return (
      <pre className="tool-output">
        <span className="muted">exit {c.exitCode}</span>
        {"\n"}
        {c.output}
      </pre>
    );
  }
  return <pre className="tool-output">{c.content?.text}</pre>;
}
