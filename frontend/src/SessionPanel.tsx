import ModelSees from "./ModelSees.tsx";
import ContextLimit from "./ContextLimit.tsx";
import { lazy, Suspense, useEffect, useEffectEvent, useLayoutEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { visibleCommand } from "./a11y.ts";
import { tabAfterKey } from "./a11y.ts";
import { describeRule, RULE_TOOLS } from "./Rules.tsx";
import { EngineService, modeName, type EnvironmentInfo, type ProviderInfo, type Rule, type Session } from "./api.ts";
import { HELD_BY, optimizerCounts, panelData, type SessionView, type ToolItem } from "./transcript.ts";
import { HOLD_PLACE } from "./scrollPlace.ts";
import Splitter from "./Splitter.tsx";
import { clampPanel, PANEL_MIN, panelMax, setPanelWidth } from "./sizes.ts";
import DiffPanel from "./DiffPanel.tsx";
import { PlanPanel } from "./PlanReview.tsx";
import { planVersions } from "./planView.ts";
import FilesPanel from "./FilesPanel.tsx";
import { locale, t, type PlainKey } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";
// xterm.js is large; it loads when a terminal is first shown.
const TerminalPanel = lazy(() => import("./TerminalPanel.tsx"));
import PreviewPanel from "./PreviewPanel.tsx";

// SessionPanel is the right panel of the main window (GUI-001): approvals,
// the diff against the session's base commit (GUI-005), and the session's
// details. It reads the same
// folded transcript as the conversation, so it never disagrees with it.

export type Tab = "approvals" | "plan" | "diff" | "files" | "terminal" | "preview" | "info";

// A rule's effect, and where a rule applies, as the list in the panel says them.
const EFFECT: Record<string, PlainKey> = { allow: "panel.rule.allow", deny: "panel.rule.deny" };
const LISTED_SCOPE: Record<string, PlainKey> = { session: "panel.rules.listed.session", global: "panel.rules.listed.global" };

// How a decision went and who made it, as the engine names them.
const OUTCOME = {
  allowed: "panel.decided.allowed",
  denied: "panel.decided.denied",
  cancelled: "panel.decided.cancelled",
} as const;
const ACTOR: Record<string, PlainKey> = {
  user: "panel.actor.user",
  mode: "panel.actor.mode",
  protected_path: "panel.actor.protectedPath",
  policy: "panel.actor.policy",
  rule: "panel.actor.rule",
  hook: "panel.actor.hook",
};
const SAVED_SCOPE: Record<string, PlainKey> = { session: "panel.scope.session", global: "panel.scope.global", project: "panel.scope.project" };

// decisionText says how a tool call was decided and by whom: "denied by rule".
function decisionText(outcome: string, actor: string): string {
  const who = ACTOR[actor] ? t(ACTOR[actor]) : actor;
  return Object.hasOwn(OUTCOME, outcome) ? t(OUTCOME[outcome as keyof typeof OUTCOME], { actor: who }) : t("panel.decided.other", { outcome, actor: who });
}

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
    <aside ref={panelBox} className={tab === "plan" || tab === "diff" || tab === "files" || tab === "terminal" || tab === "preview" ? "panel panel-wide" : "panel"} aria-label={t("panel.label")}>
      <Splitter
        className="splitter-panel"
        label={t("panel.width")}
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
          aria-label={t("panel.tabs")}
          onKeyDown={(e) => {
            const tabs = [...e.currentTarget.querySelectorAll<HTMLElement>('[role="tab"]')];
            const next = tabAfterKey(e.key, tabs.indexOf(e.target as HTMLElement), tabs.length);
            if (next < 0) return;
            e.preventDefault();
            tabs[next].focus();
          }}
        >
          <TabButton id="approvals" tab={tab} setTab={setTab}>
            {t("panel.tab.approvals")}
            {data.pending.length > 0 && <span className="badge badge-waiting_permission">{data.pending.length}</span>}
          </TabButton>
          {plans.length > 0 && (
            <TabButton id="plan" tab={tab} setTab={setTab}>
              {t("panel.tab.plan")}
              {plans.at(-1)?.state === "waiting" && <span className="badge badge-waiting_permission">1</span>}
            </TabButton>
          )}
          <TabButton id="diff" tab={tab} setTab={setTab}>
            {t("panel.tab.diff")}
            {dirty > 0 && <span className="muted"> {dirty}</span>}
          </TabButton>
          <TabButton id="files" tab={tab} setTab={setTab}>
            {t("panel.tab.files")}
          </TabButton>
          <TabButton id="terminal" tab={tab} setTab={setTab}>
            {t("panel.tab.terminal")}
          </TabButton>
          <TabButton id="preview" tab={tab} setTab={setTab}>
            {t("panel.tab.preview")}
          </TabButton>
          <TabButton id="info" tab={tab} setTab={setTab}>
            {t("panel.tab.info")}
          </TabButton>
        </div>
        <button className="panel-close" onClick={onClose} aria-label={t("panel.hide")} title={t("panel.hide")}>
          ×
        </button>
      </div>
      <div className="panel-body" role="tabpanel" id="panel-body" aria-labelledby={`panel-tab-${tab}`}>
        {tab === "approvals" && <Approvals session={session} configVersion={configVersion} settings={view.settings} pending={data.pending} decided={data.decided} onError={onError} />}
        {tab === "plan" && <PlanPanel items={view.items} onError={onError} />}
        {tab === "diff" && <DiffPanel session={session} gitKey={JSON.stringify(view.git)} agentEdits={data.changes} onReview={onReview} onError={onError} />}
        {tab === "files" && <FilesPanel session={session} gitKey={JSON.stringify(view.git) + data.changes.length} agentEdits={data.changes} onError={onError} />}
        {tab === "terminal" && (
          <Suspense fallback={<p className="muted">{t("panel.loading")}</p>}>
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
  const loadNow = useEffectEvent(load);
  useEffect(() => {
    loadNow();
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
      <h3>{t("panel.rules.title")}</h3>
      <p className="muted">{t("panel.rules.intro")}</p>
      {rules.length === 0 && <p className="muted">{t("panel.rules.none")}</p>}
      <ul className="panel-list">
        {rules.map((r, i) => (
          <li key={i} className="row">
            <span className={`decision decision-${r.effect === "deny" ? "denied" : "allowed"}`}>{EFFECT[r.effect] ? t(EFFECT[r.effect]) : r.effect}</span>
            <code>{describeRule(r)}</code>
            <span className="muted">{LISTED_SCOPE[r.scope] ? t(LISTED_SCOPE[r.scope]) : String(r.scope)}</span>
            {r.pending && <span className="muted">{t("panel.rules.pending")}</span>}
            {(r.scope === "session" || r.scope === "global") && (
              <button className="link" onClick={() => remove(r)} aria-label={t("panel.rules.remove", { rule: describeRule(r) })}>
                {t("common.remove")}
              </button>
            )}
          </li>
        ))}
      </ul>
      <form className="session-rule-form" onSubmit={add}>
        <select value={form.effect} onChange={(e) => setForm({ ...form, effect: e.target.value })} aria-label={t("panel.rules.effect")}>
          <option value="allow">{t("panel.rules.allow")}</option>
          <option value="deny">{t("panel.rules.deny")}</option>
        </select>
        <select value={form.tool} onChange={(e) => setForm({ ...form, tool: e.target.value })} aria-label={t("hooks.tool")}>
          {RULE_TOOLS.map((tool) => (
            <option key={tool} value={tool}>
              {tool === "*" ? t("panel.rules.anyTool") : tool}
            </option>
          ))}
        </select>
        <input
          aria-label={form.tool === "shell" ? t("hooks.command") : t("panel.rules.path")}
          placeholder={form.tool === "shell" ? "git *" : "src/**"}
          value={form.pattern}
          onChange={(e) => setForm({ ...form, pattern: e.target.value })}
        />
        <select value={form.scope} onChange={(e) => setForm({ ...form, scope: e.target.value })} aria-label={t("panel.rules.appliesTo")}>
          <option value="session">{t("panel.rules.forSession")}</option>
          <option value="global">{t("panel.rules.forAll")}</option>
        </select>
        <button type="submit">{t("panel.rules.add")}</button>
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
  const saved = decided.filter((item) => item.permission?.decided?.persisted).length;
  // Quick answers only; the card in the conversation has the note, the
  // scope for "Allow always" and the keyboard shortcuts.
  const decide = (item: ToolItem, kind: string) => {
    const opt = item.permission?.options.find((o) => (kind === "reject" ? o.kind.startsWith("reject") : o.kind === kind));
    // A form is sent from its card, with its answers.
    if (!opt || (item.permission?.form && kind !== "reject")) return;
    EngineService.Decide({ requestId: item.permission!.requestId, optionId: opt.optionId, reason: "", scope: "session" }).catch(onError);
  };
  return (
    <>
      <h3>{t("panel.approvals.waiting")}</h3>
      {pending.length === 0 && <p className="muted">{t("panel.approvals.nothing")}</p>}
      <ul className="panel-list">
        {pending.map((item) => (
          <li key={item.toolCallId} className="panel-pending">
            <button className="link" onClick={() => showTool(item.toolCallId)}>
              {visibleCommand(item.title)}
            </button>
            {item.permission?.command && <code>{visibleCommand(item.permission.command)}</code>}
            {item.permission?.paths?.map((p) => (
              <code key={p}>{p}</code>
            ))}
            <div className="row">
              {item.permission?.form ? (
                <button className="primary" onClick={() => showTool(item.toolCallId)}>
                  {t("panel.approvals.answer")}
                </button>
              ) : (
                <button className="primary" onClick={() => decide(item, "allow_once")}>
                  {t("panel.approvals.allowOnce")}
                </button>
              )}
              <button className="danger" onClick={() => decide(item, "reject")}>
                {item.permission?.form ? t("panel.approvals.decline") : t("panel.approvals.deny")}
              </button>
              {!item.permission?.form && <button onClick={() => showTool(item.toolCallId)}>{t("panel.approvals.more")}</button>}
            </div>
          </li>
        ))}
      </ul>
      <SessionRules session={session} version={`${configVersion} ${settings} ${saved}`} onError={onError} />
      <h3>{t("panel.approvals.decided")}</h3>
      {decided.length === 0 && <p className="muted">{t("panel.approvals.noDecisions")}</p>}
      <ul className="panel-list">
        {decided.map((item) => {
          const d = item.permission!.decided!;
          return (
            <li key={item.toolCallId}>
              <button className="link" onClick={() => showTool(item.toolCallId)}>
                {item.title}
              </button>{" "}
              <span className={`decision decision-${d.outcome}`}>{decisionText(d.outcome, d.actor)}</span>
              {d.reason && <div className="muted">{d.reason}</div>}
              {d.persisted && (
                <div className="muted">
                  {tNodes("panel.approvals.savedRule", {
                    rule: <code>{d.persisted.command || d.persisted.path || d.persisted.tool}</code>,
                    scope: SAVED_SCOPE[d.persisted.scope] ? t(SAVED_SCOPE[d.persisted.scope]) : d.persisted.scope,
                  })}
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
  const failed = useEffectEvent((err: unknown) => onError(err));
  useEffect(() => {
    EngineService.ListRules(session.id)
      .then((list) => setRules((list ?? []).filter((r) => r.scope === "session")))
      .catch(failed);
  }, [session.id, saved, configVersion]);

  const mode = view.mode ?? session.mode;
  const u = view.usage;
  // Each row's label is a catalogue key, translated as it renders. The thousands of
  // tokens are written as they always were, 1000k rather than 1,000k.
  const rows: [PlainKey, ReactNode][] = [
    ["panel.info.status", view.status ?? session.status],
    ["panel.info.harness", session.harnessId],
    ["settings.defaults.profile", session.profileId || t("panel.info.defaultProfile")],
    ["settings.defaults.provider", provider?.name ?? session.providerId],
    ["settings.defaults.model", <code>{session.model}</code>],
    ["panel.info.permissions", modeName(mode)],
    ["settings.defaults.environment", environment?.name ?? (session.environmentId || t("settings.defaults.local"))],
    ["panel.info.project", <code>{session.projectRoot}</code>],
    ...(session.worktree ? ([["panel.info.worktree", <code>{session.worktree}</code>]] as [PlainKey, ReactNode][]) : []),
    ["panel.info.branch", view.git?.repo ? view.git.branch || t("panel.info.detached") : t("panel.info.noRepo")],
    ["panel.info.baseCommit", session.baseCommit ? <code title={session.baseCommit}>{session.baseCommit.slice(0, 12)}</code> : t("panel.info.none")],
    ["panel.info.turns", turns],
    ["panel.info.inputTokens", u.inputTokens.toLocaleString(locale())],
    ["panel.info.cacheTokens", `${u.cacheReadTokens.toLocaleString(locale())} / ${u.cacheWriteTokens.toLocaleString(locale())}`],
    ["panel.info.outputTokens", u.outputTokens.toLocaleString(locale())],
    ...(view.context
      ? ([
          [
            "panel.info.context",
            view.context.window > 0
              ? t("panel.info.contextShare", { share: Math.round((100 * view.context.tokens) / view.context.window), window: String(Math.round(view.context.window / 1000)) })
              : t("panel.info.contextTokens", { tokens: String(Math.round(view.context.tokens / 1000)) }),
          ],
        ] as [PlainKey, ReactNode][])
      : []),
    ...(session.providerId && session.model
      ? ([["panel.info.contextLimit", <ContextLimit key={`${session.providerId}/${session.model}`} providerId={session.providerId} model={session.model} onError={onError} />]] as [PlainKey, ReactNode][])
      : []),
    ["panel.info.cost", costLabel],
    ["panel.info.held", view.held.length === 0 ? t("panel.info.heldNone") : t("panel.info.heldCount", { count: view.held.length })],
    ...(session.optimizer
      ? ([
          [
            "pending.kind.optimizer",
            savings
              ? t("panel.info.optimizerSaved", { rewritten: optimized.rewritten, commands: optimized.commands, saved: savings.saved, input: savings.input })
              : t("panel.info.optimizer", { rewritten: optimized.rewritten, commands: optimized.commands }),
          ],
        ] as [PlainKey, ReactNode][])
      : []),
    ...(session.portBase > 0 ? ([["panel.info.ports", `${session.portBase}–${session.portBase + 9}`]] as [PlainKey, ReactNode][]) : []),
    ["panel.info.sessionId", <code>{session.id}</code>],
  ];
  return (
    <>
      <dl className="panel-info">
        {rows.map(([k, v]) => (
          <div key={k}>
            <dt>{t(k)}</dt>
            <dd>{v}</dd>
          </div>
        ))}
      </dl>
      {view.held.length > 0 && (
        <>
          <h3>{t("panel.info.held")}</h3>
          <ul className="panel-list held-list">
            {view.held
              .slice(-HELD_SHOWN)
              .reverse()
              .map((h, i) => (
                <li key={`${h.seq}-${i}`}>
                  <button className="link" title={t("panel.held.show")} onClick={() => onJump(h.seq)}>
                    <code>{h.what}</code>
                  </button>
                  <div className="muted">
                    {HELD_BY[h.kind] ?? h.kind.replace(/_/g, " ")}
                    {h.reason ? `: ${h.reason}` : ""}
                  </div>
                </li>
              ))}
          </ul>
          {view.held.length > HELD_SHOWN && <p className="muted">{t("panel.held.latest", { shown: HELD_SHOWN, total: view.held.length })}</p>}
        </>
      )}
      <h3>{t("panel.sessionRules.title")}</h3>
      {rules.length === 0 && <p className="muted">{t("panel.sessionRules.none")}</p>}
      <ul className="panel-list">
        {rules.map((r, i) => (
          <li key={i} className="row">
            <span className={`decision decision-${r.effect === "deny" ? "denied" : "allowed"}`}>{EFFECT[r.effect] ? t(EFFECT[r.effect]) : r.effect}</span>
            <code>
              {r.tool}
              {r.command ? `: ${r.command}` : r.path ? `: ${r.path}` : ""}
            </code>
          </li>
        ))}
      </ul>
      {rules.length > 0 && <p className="muted">{t("panel.sessionRules.note")}</p>}
      <ModelSees records={view.modelContext} />
    </>
  );
}
