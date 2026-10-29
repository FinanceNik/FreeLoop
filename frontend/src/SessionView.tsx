import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { EngineService, MODES, type EnvironmentInfo, type ProviderInfo, type Session } from "./api.ts";
import { awaitingDecision, buildView, lineDiff, type Item, type ToolContent, type TranscriptEvent } from "./transcript.ts";

interface Props {
  session: Session;
  events: TranscriptEvent[];
  provider?: ProviderInfo;
  environment?: EnvironmentInfo;
  allowBypass: boolean;
  onError: (err: unknown) => void;
}

const STOP_LABEL: Record<string, string> = {
  end_turn: "Turn complete",
  cancelled: "Cancelled",
  max_tokens: "Stopped: output limit reached",
  max_turns: "Stopped: turn limit reached",
  refusal: "The model declined this request",
  error: "Stopped by an error",
};

export default function SessionView({ session, events, provider, environment, allowBypass, onError }: Props) {
  const view = useMemo(() => buildView(events), [events]);
  const [draft, setDraft] = useState("");
  const [queued, setQueued] = useState(0);
  const bottom = useRef<HTMLDivElement>(null);
  const mode = view.mode ?? session.mode;

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
    EngineService.Prompt(session.id, text)
      .then((r) => {
        setDraft("");
        if (r.queued) setQueued((n) => n + 1);
      })
      .catch(onError);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      send();
    }
  };

  const setMode = (m: string) => EngineService.SetMode(session.id, m).catch(onError);
  const total = view.usage.inputTokens + view.usage.cacheReadTokens + view.usage.cacheWriteTokens;

  return (
    <section className="session">
      <header className="session-header">
        <div>
          <h2>{session.title}</h2>
          <p className="muted">
            <code>{session.projectRoot}</code>
            {view.git?.repo && (
              <span title={`HEAD ${view.git.head.slice(0, 10)}, started at ${view.git.baseCommit.slice(0, 10)}`}>
                {" "}
                · {view.git.branch || "detached"}
                {view.git.dirty > 0 ? ` · ${view.git.dirty} changed` : " · clean"}
              </span>
            )}
            {" "}· env {environment?.name ?? session.environmentId ?? "Local"}
          </p>
        </div>
        <div className="session-controls">
          <span className="muted" title="Provider and model">
            {provider?.name ?? session.providerId} · {session.model}
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
      {mode === "bypass" && (
        <div className="banner error" role="status">
          Bypass mode: tool calls run without asking. Protected paths are still refused.
        </div>
      )}

      <div className="conversation" aria-live="polite">
        {view.items.length === 0 && <p className="muted empty">Describe a task to start.</p>}
        {view.items.map((item) => (
          <ItemView key={item.key} item={item} onError={onError} />
        ))}
        {view.running && <p className="muted working">Working…</p>}
        <div ref={bottom} />
      </div>

      <div className="composer">
        <textarea
          aria-label="Message"
          placeholder={view.running ? "Queue a message for when this turn ends" : "Ask for a change, a fix, an explanation…"}
          value={draft}
          rows={3}
          onChange={(e) => setDraft(e.target.value)}
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
  );
}

function ItemView({ item, onError }: { item: Item; onError: (err: unknown) => void }) {
  switch (item.kind) {
    case "user":
      return <div className="msg msg-user">{item.text}</div>;
    case "agent":
      return <div className="msg msg-agent">{item.text}</div>;
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

  const plan = item.tool === "present_plan" ? String((item.input as { plan?: string } | null)?.plan ?? "") : null;

  return (
    <details className={`tool tool-${item.status} ${waiting ? "tool-waiting" : ""}`} open={waiting || plan !== null || undefined}>
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
        <div className="approval" role="group" aria-label="Approve this tool call">
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
            {item.permission!.options.map((o) => (
              <button key={o.optionId} className={o.kind.startsWith("allow") ? "primary" : "danger"} onClick={() => decide(o.optionId)}>
                {o.name}
              </button>
            ))}
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
