import { visibleCommand } from "./a11y.ts";
import { optionLabel, formLabel } from "./shortcuts.ts";
import { useEffect, useMemo, useRef, useState } from "react";
import { EngineService, type AttentionItem, type PermissionRequest } from "./api.ts";
import { currentOf, editPreview, moved, oldestFirst } from "./attentionview.ts";
import { since } from "./board.ts";
import { approvalShortcut, isTextField, optionKind } from "./shortcuts.ts";
import { hookFailedText, lineDiff } from "./transcript.ts";
import { McpFormView, useMcpForm } from "./McpForm.tsx";
import { basename } from "./paths.ts";
import { t } from "./i18n.ts";

// Attention is the list of every session that needs the user, across
// projects (GUI-002): pending approvals answered inline, failed turns with
// their error, and a jump to each session.

interface Props {
  items: AttentionItem[];
  onOpen: (sessionId: string) => void;
  // A session continued elsewhere (HRN-020), with the text for its box.
  onContinued?: (sessionId: string, text: string) => void;
  onChanged: () => void;
  onError: (err: unknown) => void;
}

export default function Attention({ items, onOpen, onContinued, onChanged, onError }: Props) {
  const waiting = items.reduce((n, it) => n + (it.requests?.length ?? 0), 0);
  const asking = items.filter((it) => (it.requests?.length ?? 0) > 0).length;
  const failed = items.filter((it) => it.session.status === "error" && !it.qa).length;
  // Issues an AFK run finished, waiting for the user's QA (WFL-006).
  const qa = items.filter((it) => it.qa).length;
  const parts = [
    waiting > 0 && t("attention.requests", { count: waiting, sessions: t("attention.sessions", { count: asking }) }),
    failed > 0 && t("attention.failed", { count: failed }),
    qa > 0 && t("attention.qa", { count: qa }),
  ].filter(Boolean);
  // J and K, or the arrows, choose the request the approval keys act on. The keys
  // listen on the window, so answering one, which removes its buttons, loses nothing.
  const queue = useMemo(() => oldestFirst(items.map((it) => it.requests)), [items]);
  const [chosen, setChosen] = useState<string | null>(null);
  const current = currentOf(queue, chosen);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || isTextField(e.target)) return;
      const by = e.key === "j" || e.key === "ArrowDown" ? 1 : e.key === "k" || e.key === "ArrowUp" ? -1 : 0;
      if (by === 0 || queue.length === 0) return;
      e.preventDefault();
      setChosen(moved(queue, current, by));
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [queue, current]);
  return (
    <div className="page attention">
      <h2>{t("app.needsAttention")}</h2>
      <p className="muted">
        {items.length === 0 ? t("attention.empty") : parts.length > 0 ? `${parts.join("; ")}.` : t("attention.reply")}
        {queue.length > 1 && ` ${t("attention.moveKeys")}`}
      </p>
      {items.map((it) =>
        it.qa ? (
          <section key={`${it.session.id}:qa:${it.qa.issue}`} className="card attention-item attention-qa" aria-label={t("attention.qaLabel", { title: it.qa.title })}>
            <header className="attention-head">
              <div>
                <span className="muted attention-project">{it.projectName || basename(it.session.projectRoot)}</span>
                <h3>
                  <span className="badge attention-qa-badge">{t("attention.qaBadge")}</span> {it.qa.title}
                </h3>
              </div>
              <button onClick={() => onOpen(it.session.id)}>{t("attention.openSession")}</button>
            </header>
            <p className="muted">
              {it.qa.branch ? t("attention.qaDoneOnBranch", { path: it.qa.path, branch: it.qa.branch }) : t("attention.qaDone", { path: it.qa.path })}
              {it.qa.note ? ` ${it.qa.note}` : ""}
            </p>
            {it.qa.reply && (
              <blockquote className="attention-qa-review">
                <span className="muted">{t("attention.review")}</span> {it.qa.reply}
              </blockquote>
            )}
            <p className="row">
              {it.qa.reviewId && <button onClick={() => onOpen(it.qa!.reviewId)}>{t("attention.openReview")}</button>}
              <button
                className="primary"
                onClick={() =>
                  EngineService.QADone(it.qa!.runId, it.qa!.issue)
                    .then(onChanged)
                    .catch(onError)
                }
              >
                {t("attention.qaFinished")}
              </button>
            </p>
          </section>
        ) : (
        <section key={it.session.id} className={`card attention-item attention-status-${it.session.status}`} aria-label={it.session.title}>
          <header className="attention-head">
            <div>
              <span className="muted attention-project">{it.projectName || basename(it.session.projectRoot)}</span>
              <h3>{it.session.title}</h3>
            </div>
            <button onClick={() => onOpen(it.session.id)}>{t("attention.openSession")}</button>
          </header>
          {(it.requests ?? []).map((r) => (
            <RequestView key={r.requestId} req={r} current={r.requestId === current} onError={onError} />
          ))}
          {it.stuck && (
            <div className="attention-error">
              <p className="error-text">{t("session.stuck.detail", { detail: it.stuck.detail })}</p>
              {it.stuck.profileId && !it.stuck.auto && (
                <button
                  onClick={() =>
                    EngineService.HandOff(it.session.id, it.stuck!.profileId!, it.stuck!.reason)
                      .then((r) => (onContinued ? onContinued(r.session.id, r.continue) : onOpen(r.session.id)))
                      .catch(onError)
                  }
                >
                  {t("session.stuck.continueOn", { profile: it.stuck.profileName ?? "" })}
                </button>
              )}
              {it.stuck.auto && <span className="muted">{t("attention.continuesOn", { profile: it.stuck.profileName ?? "" })}</span>}
            </div>
          )}
          {it.session.status === "error" && (
            <div className="attention-error">
              <p className="error-text">{it.error || t("attention.turnFailed")}</p>
              <button
                onClick={() =>
                  EngineService.DismissError(it.session.id)
                    .then(onChanged)
                    .catch(onError)
                }
              >
                {t("session.dismiss")}
              </button>
            </div>
          )}
          {it.session.status === "waiting_input" && (it.requests ?? []).length === 0 && <p className="muted">{t("attention.agentWaiting")}</p>}
        </section>
        ),
      )}
    </div>
  );
}

// RequestView answers one request. When current, the approval keys answer it.
export function RequestView({ req, current, onError }: { req: PermissionRequest; current?: boolean; onError: (err: unknown) => void }) {
  // Answered requests leave the list on the next refresh; until then the
  // buttons stay disabled so a double click cannot answer a later request.
  const [busy, setBusy] = useState(false);
  const [reason, setReason] = useState("");
  const [scope, setScope] = useState("session");
  const options = req.options ?? [];
  const mcpForm = useMcpForm(req.form, req.requestId);
  const decide = (optionId: string, answers?: Record<string, unknown>) => {
    setBusy(true);
    EngineService.Decide({ requestId: req.requestId, optionId, reason, scope, answers }).catch((err) => {
      setBusy(false);
      onError(err);
    });
  };
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (current) box.current?.scrollIntoView?.({ block: "nearest" });
  }, [current]);
  // A form is answered by filling it in, not by a key.
  useEffect(() => {
    if (!current || busy || req.form) return;
    const onKey = (e: KeyboardEvent) => {
      const kind = approvalShortcut(e, isTextField(e.target));
      const option = kind && options.find((o) => optionKind(o.kind) === kind);
      if (!option) return;
      e.preventDefault();
      mcpForm.choose(option.optionId, option.kind, decide);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });
  const edit = req.command ? null : editPreview(req.input);
  return (
    <div ref={box} className={current ? "attention-request attention-current" : "attention-request"} aria-current={current ? "true" : undefined}>
      <div>
        <strong>{visibleCommand(req.title)}</strong> <span className="muted">{req.tool}</span>
        {req.createdAt && <span className="muted"> · {t("attention.waitingSince", { ago: since(req.createdAt) })}</span>}
      </div>
      {req.command && <code className="attention-command">{visibleCommand(req.command)}</code>}
      {req.paths?.map((p) => (
        <code key={p} className="attention-command">
          {p}
        </code>
      ))}
      {req.hookFailed && <p className="warn-text">{hookFailedText(req.hookFailed)}</p>}
      {edit && (
        <div className="diff">
          <pre>
            {lineDiff(edit.oldText, edit.newText).map((l, i) => (
              <div key={i} className={l.sign === "+" ? "add" : l.sign === "-" ? "del" : ""}>
                {l.sign} {l.text}
              </div>
            ))}
          </pre>
        </div>
      )}
      {req.input && !req.command && !edit && (
        <details>
          <summary className="muted">{t("attention.input")}</summary>
          <pre className="tool-input">{JSON.stringify(req.input, null, 2)}</pre>
        </details>
      )}
      {req.form && <McpFormView form={req.form} state={mcpForm} idPrefix={`form-${req.requestId}`} />}
      {!req.form && <input aria-label={t("tool.note")} placeholder={t("tool.notePlaceholder")} value={reason} onChange={(e) => setReason(e.target.value)} />}
      <div className="approval-buttons">
        {options.some((o) => o.kind === "allow_always") && (
          <select value={scope} onChange={(e) => setScope(e.target.value)} aria-label={t("tool.alwaysScope")}>
            <option value="session">{t("tool.alwaysSession")}</option>
            <option value="global">{t("tool.alwaysGlobal")}</option>
          </select>
        )}
        {options.map((o) => (
          <button
            key={o.optionId}
            disabled={busy}
            className={o.kind.startsWith("allow") ? "primary" : "danger"}
            title={o.kind === "allow_always" ? t("attention.savesRule") : undefined}
            onClick={() => mcpForm.choose(o.optionId, o.kind, decide)}
          >
            {req.form ? formLabel(o.kind) : optionLabel(o.kind, o.name)}
          </button>
        ))}
      </div>
    </div>
  );
}
