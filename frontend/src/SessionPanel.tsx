import ContextLimit from "./ContextLimit.tsx";
import { lazy, Suspense, useEffect, useLayoutEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { visibleCommand } from "./a11y.ts";
import { tabAfterKey } from "./a11y.ts";
import { describeRule, RULE_TOOLS } from "./Rules.tsx";
import { EngineService, MODES, type EnvironmentInfo, type ProviderInfo, type Rule, type Session } from "./api.ts";
import { HELD_BY, optimizerCounts, panelData, type SessionView, type ToolItem } from "./transcript.ts";
import { HOLD_PLACE } from "./scrollPlace.ts";
import Splitter from "./Splitter.tsx";
import { clampPanel, PANEL_MIN, panelMax, setPanelWidth } from "./sizes.ts";
import DiffPanel from "./DiffPanel.tsx";
import { PlanPanel } from "./PlanReview.tsx";
import { planVersions } from "./planView.ts";
import FilesPanel from "./FilesPanel.tsx";
// xterm.js is large; it loads when a terminal is first shown.
const TerminalPanel = lazy(() => import("./TerminalPanel.tsx"));
import PreviewPanel from "./PreviewPanel.tsx";

// SessionPanel is the right panel of the main window (GUI-001): approvals,
// the diff against the session's base commit (GUI-005), and the session's
// details. It reads the same
// folded transcript as the conversation, so it never disagrees with it.

export type Tab = "approvals" | "plan" | "diff" | "files" | "terminal" | "preview" | "info";

interface Props {
  // Shows a moment of the session in the conversation.
  onJump: (seq: number) => void;
  session: Session;
  tab: Tab;
  onTab: (t: Tab) => void;
  view: SessionView;
  provider?: ProviderInfo;
  environment?: EnvironmentInfo;
  configVersion: number;
  // The session's estimated cost, as the Info tab shows it.
  costLabel: string;
  // Puts a review of the diff into the composer (GUI-019).
  onReview: (text: string) => void;
  // Changes when a terminal was opened elsewhere, to show it.
  terminalNonce: number;
  onClose: () => void;
  onError: (err: unknown) => void;
}

// showTool opens a tool card in the conversation and scrolls to it: in the
// active pane of the workspace (GUI-017), which is the one the panel,
// the palette and the keyboard act on.
export function showTool(toolCallId: string) {
  const sel = `[data-tool-call="${CSS.escape(toolCallId)}"]`;
  const el = document.querySelector(`.pane-active ${sel}`) ?? document.querySelector(sel);
  if (!el) return;
  if (el instanceof HTMLDetailsElement) el.open = true;
  // It may be inside a collapsed run of tool calls.
  for (let up = el.parentElement; up; up = up.parentElement) if (up instanceof HTMLDetailsElement) up.open = true;
  el.closest(".conversation")?.dispatchEvent(new Event(HOLD_PLACE));
  const still = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
  el.scrollIntoView({ block: "center", behavior: still ? "auto" : "smooth" });
}

export default function SessionPanel({ session, tab, onTab: setTab, view, provider, environment, configVersion, costLabel, onReview, terminalNonce, onClose, onError, onJump }: Props) {
  const data = panelData(view);
  const plans = planVersions(view.items);
  // Uncommitted files, from the latest git state.
  const dirty = view.git?.repo && view.git.dirty > 0 ? view.git.dirty : 0;
  // The panel's width, dragged by its left edge (GUI-032). Each tab has a usual width.
  const panelBox = useRef<HTMLElement>(null);
  const [width, setWidth] = useState(PANEL_MIN);
  useLayoutEffect(() => {
    if (panelBox.current) setWidth(panelBox.current.offsetWidth);
  }, [tab]);
  const available = () => panelBox.current?.parentElement?.clientWidth ?? window.innerWidth;
  const dragWidth = (px: number) => {
    const next = clampPanel(px, available());
    setPanelWidth(next);
    setWidth(next);
  };

  return (
    <aside ref={panelBox} className={tab === "plan" || tab === "diff" || tab === "files" || tab === "terminal" || tab === "preview" ? "panel panel-wide" : "panel"} aria-label="Session panel">
      <Splitter
        className="splitter-panel"
        label="Panel width"
        value={width}
        min={PANEL_MIN}
        max={panelMax(available())}
        // Dragged or pressed to the right, it gets narrower.
        step={-16}
        fromPointer={(x) => (panelBox.current?.getBoundingClientRect().right ?? x) - x}
        onChange={dragWidth}
        onReset={() => {
          setPanelWidth(null);
          setWidth(panelBox.current?.offsetWidth ?? PANEL_MIN);
        }}
      />
      <div className="panel-tabs">
        <div
          className="panel-tablist"
          role="tablist"
          aria-label="Panel views"
          onKeyDown={(e) => {
            const tabs = [...e.currentTarget.querySelectorAll<HTMLElement>('[role="tab"]')];
            const next = tabAfterKey(e.key, tabs.indexOf(e.target as HTMLElement), tabs.length);
            if (next < 0) return;
            e.preventDefault();
            tabs[next].focus();
          }}
        >
          <TabButton id="approvals" tab={tab} setTab={setTab}>
            Approvals{data.pending.length > 0 && <span className="badge badge-waiting_permission">{data.pending.length}</span>}
          </TabButton>
          {plans.length > 0 && (
            <TabButton id="plan" tab={tab} setTab={setTab}>
              Plan{plans.at(-1)?.state === "waiting" && <span className="badge badge-waiting_permission">1</span>}
            </TabButton>
          )}
          <TabButton id="diff" tab={tab} setTab={setTab}>
            Diff{dirty > 0 && <span className="muted"> {dirty}</span>}
          </TabButton>
          <TabButton id="files" tab={tab} setTab={setTab}>
            Files
          </TabButton>
          <TabButton id="terminal" tab={tab} setTab={setTab}>
            Terminal
          </TabButton>
          <TabButton id="preview" tab={tab} setTab={setTab}>
            Preview
          </TabButton>
          <TabButton id="info" tab={tab} setTab={setTab}>
            Info
          </TabButton>
        </div>
        <button className="panel-close" onClick={onClose} aria-label="Hide the panel" title="Hide the panel">
          ×
        </button>
      </div>
      <div className="panel-body" role="tabpanel" id="panel-body" aria-labelledby={`panel-tab-${tab}`}>
        {tab === "approvals" && <Approvals session={session} configVersion={configVersion} settings={view.settings} pending={data.pending} decided={data.decided} onError={onError} />}
        {tab === "plan" && <PlanPanel items={view.items} onError={onError} />}
        {tab === "diff" && <DiffPanel session={session} gitKey={JSON.stringify(view.git)} agentEdits={data.changes} onReview={onReview} onError={onError} />}
        {tab === "files" && <FilesPanel session={session} gitKey={JSON.stringify(view.git) + data.changes.length} agentEdits={data.changes} onError={onError} />}
        {tab === "terminal" && (
          <Suspense fallback={<p className="muted">Loading…</p>}>
            <TerminalPanel key={terminalNonce} session={session} onError={onError} />
          </Suspense>
        )}
        {tab === "preview" && <PreviewPanel session={session} onError={onError} />}
        {tab === "info" && (
          <Info
            session={session}
            view={view}
            turns={data.turns}
            provider={provider}
            environment={environment}
            configVersion={configVersion}
            costLabel={costLabel}
            onJump={onJump}
            onError={onError}
          />
        )}
      </div>
    </aside>
  );
}

function TabButton({ id, tab, setTab, children }: { id: Tab; tab: Tab; setTab: (t: Tab) => void; children: ReactNode }) {
  return (
    <button
      role="tab"
      id={`panel-tab-${id}`}
      aria-selected={tab === id}
      aria-controls={tab === id ? "panel-body" : undefined}
      tabIndex={tab === id ? 0 : -1}
      className={tab === id ? "tab tab-on" : "tab"}
      onClick={() => setTab(id)}
    >
      {children}
    </button>
  );
}

// What a session allows and denies without asking, and changing it (SEC-002).
function SessionRules({ session, version, onError }: { session: Session; version: string; onError: (err: unknown) => void }) {
  const [rules, setRules] = useState<Rule[]>([]);
  const [form, setForm] = useState({ effect: "allow", tool: "shell", pattern: "", scope: "session" });
  const load = () =>
    EngineService.ListRules(session.id)
      .then((list) => setRules(list ?? []))
      .catch(onError);
  useEffect(() => {
    load();
  }, [session.id, version]);
  const add = (e: FormEvent) => {
    e.preventDefault();
    const pattern = form.pattern.trim();
    const rule = { effect: form.effect, tool: form.tool, command: form.tool === "shell" ? pattern : "", path: form.tool !== "shell" ? pattern : "", scope: form.scope, source: "" } as unknown as Rule;
    (form.scope === "session" ? EngineService.AddSessionRule(session.id, rule) : EngineService.AddRule(rule))
      .then(() => {
        setForm({ ...form, pattern: "" });
        return load();
      })
      .catch(onError);
  };
  const remove = (r: Rule) => (r.scope === "session" ? EngineService.DeleteSessionRule(session.id, r) : EngineService.DeleteRule(r)).then(load).catch(onError);
  return (
    <div data-rules={session.id}>
      <h3>Rules</h3>
      <p className="muted">These decide before you are asked. Deny rules win, and an allow rule never lifts plan mode or protected paths.</p>
      {rules.length === 0 && <p className="muted">No rules yet.</p>}
      <ul className="panel-list">
        {rules.map((r, i) => (
          <li key={i} className="row">
            <span className={`decision decision-${r.effect === "deny" ? "denied" : "allowed"}`}>{r.effect}</span>
            <code>{describeRule(r)}</code>
            <span className="muted">{r.scope === "session" ? "this session" : r.scope === "global" ? "all sessions" : String(r.scope)}</span>
            {r.pending && <span className="muted">waits for approval in Settings</span>}
            {(r.scope === "session" || r.scope === "global") && (
              <button className="link" onClick={() => remove(r)} aria-label={`Remove the rule ${describeRule(r)}`}>
                Remove
              </button>
            )}
          </li>
        ))}
      </ul>
      <form className="session-rule-form" onSubmit={add}>
        <select value={form.effect} onChange={(e) => setForm({ ...form, effect: e.target.value })} aria-label="Effect">
          <option value="allow">Allow</option>
          <option value="deny">Deny</option>
        </select>
        <select value={form.tool} onChange={(e) => setForm({ ...form, tool: e.target.value })} aria-label="Tool">
          {RULE_TOOLS.map((t) => (
            <option key={t} value={t}>
              {t === "*" ? "any tool" : t}
            </option>
          ))}
        </select>
        <input
          aria-label={form.tool === "shell" ? "Command" : "Path pattern"}
          placeholder={form.tool === "shell" ? "git *" : "src/**"}
          value={form.pattern}
          onChange={(e) => setForm({ ...form, pattern: e.target.value })}
        />
        <select value={form.scope} onChange={(e) => setForm({ ...form, scope: e.target.value })} aria-label="Applies to">
          <option value="session">This session</option>
          <option value="global">All sessions</option>
        </select>
        <button type="submit">Add</button>
      </form>
    </div>
  );
}

function Approvals({
  session,
  configVersion,
  settings,
  pending,
  decided,
  onError,
}: {
  session: Session;
  configVersion: number;
  settings: number;
  pending: ToolItem[];
  decided: ToolItem[];
  onError: (err: unknown) => void;
}) {
  const saved = decided.filter((t) => t.permission?.decided?.persisted).length;
  // Quick answers only; the card in the conversation has the note, the
  // scope for "Allow always" and the keyboard shortcuts.
  const decide = (t: ToolItem, kind: string) => {
    const opt = t.permission?.options.find((o) => (kind === "reject" ? o.kind.startsWith("reject") : o.kind === kind));
    // A form is sent from its card, with its answers.
    if (!opt || (t.permission?.form && kind !== "reject")) return;
    EngineService.Decide({ requestId: t.permission!.requestId, optionId: opt.optionId, reason: "", scope: "session" }).catch(onError);
  };
  return (
    <>
      <h3>Waiting for you</h3>
      {pending.length === 0 && <p className="muted">Nothing is waiting.</p>}
      <ul className="panel-list">
        {pending.map((t) => (
          <li key={t.toolCallId} className="panel-pending">
            <button className="link" onClick={() => showTool(t.toolCallId)}>
              {visibleCommand(t.title)}
            </button>
            {t.permission?.command && <code>{visibleCommand(t.permission.command)}</code>}
            {t.permission?.paths?.map((p) => (
              <code key={p}>{p}</code>
            ))}
            <div className="row">
              {t.permission?.form ? (
                <button className="primary" onClick={() => showTool(t.toolCallId)}>
                  Answer…
                </button>
              ) : (
                <button className="primary" onClick={() => decide(t, "allow_once")}>
                  Allow once
                </button>
              )}
              <button className="danger" onClick={() => decide(t, "reject")}>
                {t.permission?.form ? "Decline" : "Deny"}
              </button>
              {!t.permission?.form && <button onClick={() => showTool(t.toolCallId)}>More…</button>}
            </div>
          </li>
        ))}
      </ul>
      <SessionRules session={session} version={`${configVersion} ${settings} ${saved}`} onError={onError} />
      <h3>Decided</h3>
      {decided.length === 0 && <p className="muted">No decisions yet. Calls allowed by the session's mode are not listed.</p>}
      <ul className="panel-list">
        {decided.map((t) => {
          const d = t.permission!.decided!;
          return (
            <li key={t.toolCallId}>
              <button className="link" onClick={() => showTool(t.toolCallId)}>
                {t.title}
              </button>{" "}
              <span className={`decision decision-${d.outcome}`}>
                {d.outcome} by {d.actor}
              </span>
              {d.reason && <div className="muted">{d.reason}</div>}
              {d.persisted && (
                <div className="muted">
                  Saved rule <code>{d.persisted.command || d.persisted.path || d.persisted.tool}</code> ({d.persisted.scope})
                </div>
              )}
            </li>
          );
        })}
      </ul>
    </>
  );
}

interface InfoProps {
  session: Session;
  view: SessionView;
  turns: number;
  provider?: ProviderInfo;
  environment?: EnvironmentInfo;
  configVersion: number;
  costLabel: string;
  onJump: (seq: number) => void;
  onError: (err: unknown) => void;
}

// How many of what the policy held are listed; the count covers them all.
const HELD_SHOWN = 50;

function Info({ session, view, turns, provider, environment, configVersion, costLabel, onJump, onError }: InfoProps) {
  const [rules, setRules] = useState<Rule[]>([]);
  // What the token optimizer did (HRN-019). Its savings are its own report, which
  // not every optimizer has, so no answer shows only the count.
  const optimized = optimizerCounts(view, !!session.optimizer);
  const [savings, setSavings] = useState<{ input: number; saved: number } | null>(null);
  useEffect(() => {
    if (!session.optimizer) return setSavings(null);
    EngineService.OptimizerSavings(session.id)
      .then((s) => setSavings(s && s.input > 0 ? s : null))
      .catch(() => setSavings(null));
  }, [session.id, session.optimizer, turns]);
  // Refetched when a decision may have saved a rule or the files changed.
  const saved = view.items.filter((i) => i.kind === "tool" && i.permission?.decided?.persisted).length;
  useEffect(() => {
    EngineService.ListRules(session.id)
      .then((list) => setRules((list ?? []).filter((r) => r.scope === "session")))
      .catch(onError);
  }, [session.id, saved, configVersion]);

  const mode = view.mode ?? session.mode;
  const u = view.usage;
  const rows: [string, ReactNode][] = [
    ["Status", view.status ?? session.status],
    ["Harness", session.harnessId],
    ["Profile", session.profileId || "default"],
    ["Provider", provider?.name ?? session.providerId],
    ["Model", <code>{session.model}</code>],
    ["Permissions", MODES.find((m) => m.id === mode)?.name ?? mode],
    ["Environment", environment?.name ?? (session.environmentId || "Local")],
    ["Project", <code>{session.projectRoot}</code>],
    ...(session.worktree ? ([["Worktree", <code>{session.worktree}</code>]] as [string, ReactNode][]) : []),
    ["Branch", view.git?.repo ? view.git.branch || "detached" : "not a git repository"],
    ["Base commit", session.baseCommit ? <code title={session.baseCommit}>{session.baseCommit.slice(0, 12)}</code> : "none"],
    ["Turns", turns],
    ["Input tokens", u.inputTokens.toLocaleString()],
    ["Cache reads / writes", `${u.cacheReadTokens.toLocaleString()} / ${u.cacheWriteTokens.toLocaleString()}`],
    ["Output tokens", u.outputTokens.toLocaleString()],
    ...(view.context
      ? ([
          [
            "Context",
            view.context.window > 0 ? `${Math.round((100 * view.context.tokens) / view.context.window)} % of ${Math.round(view.context.window / 1000)}k` : `${Math.round(view.context.tokens / 1000)}k tokens`,
          ],
        ] as [string, ReactNode][])
      : []),
    ...(session.providerId && session.model
      ? ([["Context limit", <ContextLimit key={`${session.providerId}/${session.model}`} providerId={session.providerId} model={session.model} onError={onError} />]] as [string, ReactNode][])
      : []),
    ["Estimated cost", costLabel],
    ["Held by your policy", view.held.length === 0 ? "Nothing so far" : `${view.held.length} ${view.held.length === 1 ? "thing" : "things"}`],
    ...(session.optimizer
      ? ([
          [
            "Token optimizer",
            `${optimized.rewritten} of ${optimized.commands} shell commands shortened` +
              (savings ? `; ${savings.saved.toLocaleString()} of ${savings.input.toLocaleString()} tokens saved` : ""),
          ],
        ] as [string, ReactNode][])
      : []),
    ...(session.portBase > 0 ? ([["Ports", `${session.portBase}–${session.portBase + 9}`]] as [string, ReactNode][]) : []),
    ["Session ID", <code>{session.id}</code>],
  ];
  return (
    <>
      <dl className="panel-info">
        {rows.map(([k, v]) => (
          <div key={k}>
            <dt>{k}</dt>
            <dd>{v}</dd>
          </div>
        ))}
      </dl>
      {view.held.length > 0 && (
        <>
          <h3>Held by your policy</h3>
          <ul className="panel-list held-list">
            {view.held
              .slice(-HELD_SHOWN)
              .reverse()
              .map((h, i) => (
                <li key={`${h.seq}-${i}`}>
                  <button className="link" title="Show this in the conversation" onClick={() => onJump(h.seq)}>
                    <code>{h.what}</code>
                  </button>
                  <div className="muted">
                    {HELD_BY[h.kind] ?? h.kind.replace(/_/g, " ")}
                    {h.reason ? `: ${h.reason}` : ""}
                  </div>
                </li>
              ))}
          </ul>
          {view.held.length > HELD_SHOWN && <p className="muted">The latest {HELD_SHOWN} of {view.held.length}.</p>}
        </>
      )}
      <h3>Rules for this session</h3>
      {rules.length === 0 && <p className="muted">None. "Allow always" with "this session" adds one.</p>}
      <ul className="panel-list">
        {rules.map((r, i) => (
          <li key={i} className="row">
            <span className={`decision decision-${r.effect === "deny" ? "denied" : "allowed"}`}>{r.effect}</span>
            <code>
              {r.tool}
              {r.command ? `: ${r.command}` : r.path ? `: ${r.path}` : ""}
            </code>
          </li>
        ))}
      </ul>
      {rules.length > 0 && <p className="muted">Session rules last as long as the session; global rules are on the Rules page.</p>}
    </>
  );
}
