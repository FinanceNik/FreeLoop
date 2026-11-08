import { AllowedIcon, PenIcon, RuleIcon } from "./icons.tsx";
import { useEffect, useRef, useState } from "react";
import { editMark, quietCommand, toolStatusLabel, visibleCommand } from "./a11y.ts";
import { EngineService } from "./api.ts";
import { McpFormView, useMcpForm } from "./McpForm.tsx";
import { APPROVAL_GRACE_MS, approvalHint, approvalShortcut, formLabel, isTextField, optionKind, optionLabel } from "./shortcuts.ts";
import { awaitingDecision, hookFailedText, lineDiff, type Item, type PermissionOption, type ToolContent } from "./transcript.ts";
import { planText } from "./planView.ts";
import { agentQuestions } from "./forms.ts";
import { PlanCard, PlanDecision } from "./PlanReview.tsx";

// ToolView is one tool call in the conversation: what it asked and did and,
// while it waits, the approval card (GUI-006).

export default function ToolView({ item, roots = [], onError }: { item: Extract<Item, { kind: "tool" }>; roots?: string[]; onError: (err: unknown) => void }) {
  const waiting = awaitingDecision(item);
  const [reason, setReason] = useState("");
  const [scope, setScope] = useState("session");
  const decided = item.permission?.decided;
  const shownDecision = decided && decided.actor !== "policy" && !(decided.actor === "mode" && decided.outcome === "allowed");
  const edited = editMark(item.toolKind, item.status);

  const form = item.permission?.form;
  const decide = (optionId: string, answers?: Record<string, unknown>) =>
    EngineService.Decide({ requestId: item.permission!.requestId, optionId, reason, scope, answers }).catch(onError);
  const mcpForm = useMcpForm(form, item.permission?.requestId);
  const choose = (o: PermissionOption) => mcpForm.choose(o.optionId, o.kind, decide);
  const offersAlways = item.permission?.options.some((o) => o.kind === "allow_always");

  // Shortcuts (GUI-006) go to the first waiting card on screen only.
  const box = useRef<HTMLDivElement>(null);
  const shownAt = useRef(0);
  useEffect(() => {
    // A keypress never sends a form.
    if (!waiting || form) return;
    shownAt.current = Date.now();
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (e.repeat || e.defaultPrevented || e.isComposing) return;
      // Only the first waiting card of the active pane (GUI-017).
      const pane = box.current?.closest(".pane");
      if (pane && !pane.classList.contains("pane-active")) return;
      if ((pane ?? document).querySelector(".approval") !== box.current) return;
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

  // A token optimizer ran this command in a shorter form (HRN-019).
  const via = item.via ?? item.content.find((c) => c.via)?.via;
  // A plan reads as a document, from either agent (GUI-043).
  const plan = planText(item);
  // An agent's questions read as questions (HRN-021).
  const questions = plan === null ? agentQuestions(item.input) : null;

  return (
    <details
      data-tool-call={item.toolCallId}
      className={`tool tool-${item.status} ${waiting ? "tool-waiting" : ""}`}
      open={waiting || plan !== null || undefined}
    >
      <summary>
        <span className={`dot dot-${item.status}`} aria-hidden="true" />
        <span className="sr-only">{toolStatusLabel(item.status, waiting)} </span>
        {/* What waits for a decision shows all of the command. */}
        <span className="tool-title">{visibleCommand(waiting ? item.title : quietCommand(item.title, roots))}</span>
        {!item.tool.startsWith("acp:") && <span className="muted"> {item.tool}</span>}
        {via && <span className="muted"> · via {via}</span>}
        {shownDecision &&
          (decided.outcome === "allowed" ? (
            // What went ahead is a small mark; what the policy held keeps its words (GUI-042).
            <span className={`decision decision-allowed decision-icon`} role="img" aria-label={decided.actor === "sandbox" ? "sandboxed" : `allowed by ${decided.actor}`} title={decided.actor === "sandbox" ? "Ran in the sandbox" : `Allowed by ${decided.actor}`}>
              {decided.actor === "rule" ? <RuleIcon /> : <AllowedIcon />}
            </span>
          ) : (
            <span className={`decision decision-${decided.outcome}`}>
              {decided.actor === "sandbox" ? "sandboxed" : `${decided.outcome} by ${decided.actor}`}
            </span>
          ))}
        {edited && (
          // A file edit that went through gets a pen, whatever allowed it.
          <span className="decision decision-allowed decision-icon edit-mark" role="img" aria-label={edited} title={edited}>
            <PenIcon />
          </span>
        )}
      </summary>
      {plan !== null ? (
        <PlanCard text={plan} />
      ) : questions ? (
        // The form shows them while it waits; after, the questions stay readable.
        !waiting && (
          <ul className="agent-questions">
            {questions.map((q, i) => (
              <li key={i}>
                {q.question} <span className="muted">{q.options.join(" · ")}</span>
              </li>
            ))}
          </ul>
        )
      ) : (
        <pre className="tool-input">{JSON.stringify(item.input, null, 2)}</pre>
      )}
      {item.content
        // An external agent sends the plan again as text: once is enough.
        .filter((c) => plan === null || (c.content?.text ?? "").trim() !== plan.trim())
        .map((c, i) => (
          <ContentView key={i} c={c} />
        ))}
      {shownDecision && decided.reason && <p className="muted">{decided.reason}</p>}
      {decided?.persisted && (
        <p className="muted">
          Saved rule: <code>{decided.persisted.command || decided.persisted.path || decided.persisted.tool}</code> ({decided.persisted.scope})
        </p>
      )}
      {waiting && plan !== null && (
        <div className="approval plan-approval" role="group" aria-label="Approve this plan" ref={box}>
          <PlanDecision item={item} onError={onError} />
        </div>
      )}
      {waiting && plan === null && (
        <div className="approval" role="group" aria-label={form ? `${form.server} asks for input` : "Approve this tool call"} ref={box}>
          {form && <McpFormView form={form} state={mcpForm} />}
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
          {item.permission?.hookFailed && <p className="warn-text">{hookFailedText(item.permission.hookFailed)}</p>}
          {!form && <input aria-label="Note for the agent" placeholder="Optional note for the agent" value={reason} onChange={(e) => setReason(e.target.value)} />}
          <div className="approval-buttons">
            {offersAlways && (
              <select value={scope} onChange={(e) => setScope(e.target.value)} aria-label="Allow always applies to">
                <option value="session">Always: this session</option>
                <option value="global">Always: everywhere</option>
              </select>
            )}
            {item.permission!.options.map((o) => {
              const kind = optionKind(o.kind);
              const hint = kind ? approvalHint(kind) : null;
              return (
                <button
                  key={o.optionId}
                  className={o.kind.startsWith("allow") ? "primary" : "danger"}
                  onClick={() => choose(o)}
                  title={hint && !form ? `Shortcut: ${hint}` : undefined}
                >
                  {form ? formLabel(o.kind) : optionLabel(o.kind, o.name)}
                  {hint && !form && <kbd aria-hidden="true">{hint.split(" ")[0]}</kbd>}
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
        <span className="muted">
          exit {c.exitCode}
          {c.via && ` · via ${c.via}: ${c.command}`}
        </span>
        {"\n"}
        {c.output}
      </pre>
    );
  }
  // A link to another session, such as a subagent's (HRN-014).
  if (c.content?.type === "resource_link" && c.content.uri?.startsWith(SESSION_URI)) {
    const sid = c.content.uri.slice(SESSION_URI.length);
    return (
      <button className="link" onClick={() => openSessionEvent(sid)}>
        {c.content.name || "Open the session"}
      </button>
    );
  }
  return <pre className="tool-output">{c.content?.text}</pre>;
}

const SESSION_URI = "freeloop://session/";

// openSessionEvent asks the app to open a session, from anywhere in the
// view.
export const OPEN_SESSION_DOM_EVENT = "freeloop-open-session";
export function openSessionEvent(sid: string) {
  window.dispatchEvent(new CustomEvent(OPEN_SESSION_DOM_EVENT, { detail: sid }));
}
