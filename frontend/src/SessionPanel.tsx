import { lazy, Suspense, useEffect, useState, type ReactNode } from "react";
import { tabAfterKey } from "./a11y.ts";
import { EngineService, MODES, type EnvironmentInfo, type ProviderInfo, type Rule, type Session } from "./api.ts";
import { panelData, type SessionView, type ToolItem } from "./transcript.ts";
import DiffPanel from "./DiffPanel.tsx";
import FilesPanel from "./FilesPanel.tsx";
// xterm.js is large; it loads when a terminal is first shown.
const TerminalPanel = lazy(() => import("./TerminalPanel.tsx"));
import PreviewPanel from "./PreviewPanel.tsx";

// SessionPanel is the right panel of the main window (GUI-001): approvals,
// the diff against the session's base commit (GUI-005), and the session's
// details. It reads the same
// folded transcript as the conversation, so it never disagrees with it.

export type Tab = "approvals" | "diff" | "files" | "terminal" | "preview" | "info";

interface Props {
  session: Session;
  tab: Tab;
  onTab: (t: Tab) => void;
  view: SessionView;
  provider?: ProviderInfo;
  environment?: EnvironmentInfo;
  configVersion: number;
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
  const still = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
  el.scrollIntoView({ block: "center", behavior: still ? "auto" : "smooth" });
}

export default function SessionPanel({ session, tab, onTab: setTab, view, provider, environment, configVersion, onReview, terminalNonce, onClose, onError }: Props) {
  const data = panelData(view);
  // Uncommitted files, from the latest git state.
  const dirty = view.git?.repo && view.git.dirty > 0 ? view.git.dirty : 0;

  return (
    <aside className={tab === "diff" || tab === "files" || tab === "terminal" || tab === "preview" ? "panel panel-wide" : "panel"} aria-label="Session panel">
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
        {tab === "approvals" && <Approvals pending={data.pending} decided={data.decided} onError={onError} />}
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

function Approvals({ pending, decided, onError }: { pending: ToolItem[]; decided: ToolItem[]; onError: (err: unknown) => void }) {
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
              {t.title}
            </button>
            {t.permission?.command && <code>{t.permission.command}</code>}
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
  onError: (err: unknown) => void;
}

function Info({ session, view, turns, provider, environment, configVersion, onError }: InfoProps) {
  const [rules, setRules] = useState<Rule[]>([]);
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
    ["Mode", MODES.find((m) => m.id === mode)?.name ?? mode],
    ["Environment", environment?.name ?? (session.environmentId || "Local")],
    ["Project", <code>{session.projectRoot}</code>],
    ...(session.worktree ? ([["Worktree", <code>{session.worktree}</code>]] as [string, ReactNode][]) : []),
    ["Branch", view.git?.repo ? view.git.branch || "detached" : "not a git repository"],
    ["Base commit", session.baseCommit ? <code title={session.baseCommit}>{session.baseCommit.slice(0, 12)}</code> : "none"],
    ["Turns", turns],
    ["Input tokens", u.inputTokens.toLocaleString()],
    ["Cache reads / writes", `${u.cacheReadTokens.toLocaleString()} / ${u.cacheWriteTokens.toLocaleString()}`],
    ["Output tokens", u.outputTokens.toLocaleString()],
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
