import { useEffect, useRef, useState } from "react";
import { EngineService, type IssueDetail } from "./api.ts";
import { issueCostLine, stepLabel } from "./afk.ts";
import { laneTitle, shortId, since, type Lane } from "./board.ts";
import { Markdown } from "./Markdown.tsx";
import { typeLabel } from "./backlog.ts";
import { t } from "./i18n.ts";

// The Board's panel for one issue (GUI-016): read-only. Its text as Markdown, the file, what
// it waits on and holds up, and the sessions that worked on it and reviewed it, each opening
// that session. Nothing in it changes the issue: the Board only indicates, so there is no
// Mark done and no Run this issue. The text is the agent's, untrusted, and goes through the
// Markdown renderer, never as HTML.
export default function IssuePanel({
  root,
  workflowId,
  issueId,
  column,
  onClose,
  onOpen,
  onError,
}: {
  root: string;
  workflowId: string;
  issueId: string;
  column: Lane;
  onClose: () => void;
  onOpen: (sessionId: string) => void;
  onError: (err: unknown) => void;
}) {
  const [detail, setDetail] = useState<IssueDetail | null>(null);
  const close = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    setDetail(null);
    let current = true;
    EngineService.IssueDetail(root, workflowId, issueId)
      .then((d) => current && setDetail(d))
      .catch((err) => {
        if (!current) return;
        onError(err);
        onClose();
      });
    return () => {
      current = false;
    };
  }, [root, workflowId, issueId]);
  // The keyboard reaches the panel first, and Escape closes it.
  useEffect(() => {
    close.current?.focus();
  }, [issueId]);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || e.defaultPrevented) return;
      e.preventDefault();
      onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <aside className="issue-panel" aria-label={t("board.detail.label", { id: shortId(issueId) })}>
      <header className="issue-panel-head">
        <h3>{detail?.title ?? shortId(issueId)}</h3>
        <button ref={close} className="icon-button" aria-label={t("board.detail.close")} title={t("board.detail.close")} onClick={onClose}>
          <span aria-hidden="true">×</span>
        </button>
      </header>
      {!detail && <p className="muted">{t("board.detail.loading")}</p>}
      {detail && (
        <>
          <p className="issue-meta">
            <span className="issue-id">{shortId(detail.id)}</span>
            <span className={`tag backlog-type-${detail.type}`}>{typeLabel({ type: detail.type } as never)}</span>
            <span className="muted">{laneTitle(column)}</span>
          </p>
          <p className="muted issue-panel-file">
            {t("board.detail.file")}: <code>{detail.path}</code>
          </p>
          {(detail.waitsOn ?? []).length > 0 && (
            <p className="muted">
              {t("board.detail.waitsOn")}: {(detail.waitsOn ?? []).map(shortId).join(", ")}
            </p>
          )}
          {(detail.blocks ?? []).length > 0 && (
            <p className="muted">
              {t("board.detail.blocks")}: {(detail.blocks ?? []).map(shortId).join(", ")}
            </p>
          )}
          <div className="issue-panel-text">
            <Markdown text={detail.text} />
          </div>
          <h4>{t("board.detail.sessions")}</h4>
          {detail.cost && <p className="muted issue-panel-cost">{issueCostLine(detail.cost)}</p>}
          {(detail.sessions ?? []).length === 0 && <p className="muted">{t("board.detail.noSessions")}</p>}
          <ul className="issue-panel-sessions">
            {(detail.sessions ?? []).map((s) => (
              <li key={s.sessionId}>
                <button className="link" onClick={() => onOpen(s.sessionId)}>
                  {s.title}
                </button>
                <span className="muted">
                  {" · "}
                  {t(s.kind === "review" ? "board.detail.kind.review" : "board.detail.kind.issue")}
                  {" · "}
                  {stepLabel({ issue: detail.id, title: "", state: s.state })}
                  {" · "}
                  {since(s.startedAt)}
                </span>
              </li>
            ))}
          </ul>
        </>
      )}
    </aside>
  );
}
