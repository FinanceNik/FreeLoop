import { optionLabel } from "./shortcuts.ts";
import { useState } from "react";
import { EngineService, type AttentionItem, type PermissionRequest } from "./api.ts";
import { McpFormView, useMcpForm } from "./McpForm.tsx";
import { basename } from "./paths.ts";

// Attention is the list of every session that needs the user, across
// projects (GUI-002): pending approvals answered inline, failed turns with
// their error, and a jump to each session.

interface Props {
  items: AttentionItem[];
  onOpen: (sessionId: string) => void;
  onChanged: () => void;
  onError: (err: unknown) => void;
}

export default function Attention({ items, onOpen, onChanged, onError }: Props) {
  const waiting = items.reduce((n, it) => n + (it.requests?.length ?? 0), 0);
  const asking = items.filter((it) => (it.requests?.length ?? 0) > 0).length;
  const failed = items.filter((it) => it.session.status === "error").length;
  const parts = [
    waiting > 0 && `${waiting} ${waiting === 1 ? "request" : "requests"} waiting in ${asking} ${asking === 1 ? "session" : "sessions"}`,
    failed > 0 && `${failed} ${failed === 1 ? "session" : "sessions"} in error`,
  ].filter(Boolean);
  return (
    <div className="page attention">
      <h2>Needs attention</h2>
      <p className="muted">
        {items.length === 0
          ? "Nothing needs you right now. Sessions waiting for approval or whose last turn failed appear here."
          : `${parts.join("; ") || "A session is waiting for your reply"}.`}
      </p>
      {items.map((it) => (
        <section key={it.session.id} className={`card attention-item attention-status-${it.session.status}`} aria-label={it.session.title}>
          <header className="attention-head">
            <div>
              <span className="muted attention-project">{it.projectName || basename(it.session.projectRoot)}</span>
              <h3>{it.session.title}</h3>
            </div>
            <button onClick={() => onOpen(it.session.id)}>Open session</button>
          </header>
          {(it.requests ?? []).map((r) => (
            <RequestView key={r.requestId} req={r} onError={onError} />
          ))}
          {it.session.status === "error" && (
            <div className="attention-error">
              <p className="error-text">{it.error || "The last turn failed."}</p>
              <button
                onClick={() =>
                  EngineService.DismissError(it.session.id)
                    .then(onChanged)
                    .catch(onError)
                }
              >
                Dismiss
              </button>
            </div>
          )}
          {it.session.status === "waiting_input" && (it.requests ?? []).length === 0 && <p className="muted">The agent is waiting for your reply.</p>}
        </section>
      ))}
    </div>
  );
}

function RequestView({ req, onError }: { req: PermissionRequest; onError: (err: unknown) => void }) {
  // Answered requests leave the list on the next refresh; until then the
  // buttons stay disabled so a double click cannot answer a later request.
  const [busy, setBusy] = useState(false);
  const options = req.options ?? [];
  const mcpForm = useMcpForm(req.form, req.requestId);
  const decide = (optionId: string, answers?: Record<string, unknown>) => {
    setBusy(true);
    EngineService.Decide({ requestId: req.requestId, optionId, reason: "", scope: "session", answers }).catch((err) => {
      setBusy(false);
      onError(err);
    });
  };
  return (
    <div className="attention-request">
      <div>
        <strong>{req.title}</strong> <span className="muted">{req.tool}</span>
      </div>
      {req.command && <code className="attention-command">{req.command}</code>}
      {req.paths?.map((p) => (
        <code key={p} className="attention-command">
          {p}
        </code>
      ))}
      {req.input && !req.command && (
        <details>
          <summary className="muted">Input</summary>
          <pre className="tool-input">{JSON.stringify(req.input, null, 2)}</pre>
        </details>
      )}
      {req.form && <McpFormView form={req.form} state={mcpForm} idPrefix={`form-${req.requestId}`} />}
      <div className="approval-buttons">
        {options.map((o) => (
          <button
            key={o.optionId}
            disabled={busy}
            className={o.kind.startsWith("allow") ? "primary" : "danger"}
            title={o.kind === "allow_always" ? "Saves a rule for this session" : undefined}
            onClick={() => mcpForm.choose(o.optionId, o.kind, decide)}
          >
            {o.kind === "allow_always" ? `${optionLabel(o.kind, o.name)} (this session)` : optionLabel(o.kind, o.name)}
          </button>
        ))}
      </div>
    </div>
  );
}
