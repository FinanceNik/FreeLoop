import type { ReactNode } from "react";
import { useRef } from "react";
import { cardMeta, LANES, laneTitle, placeIssues, waitingIssues, type IssueCard } from "./board.ts";
import { stepLabel } from "./afk.ts";
import { counts } from "./backlog.ts";
import type { BacklogInfo } from "./api.ts";
import { t } from "./i18n.ts";

// The Board is a scrum board of workflow issues (GUI-016): one row per project with a
// backlog, and in it one card per issue, in Backlog, Working, In review or Done. It only
// shows where each issue stands, from its file and the AFK run's step for it, so a card
// is never dragged; the sessions behind it are reached from the card, and the sidebar and
// Needs attention keep the ordinary sessions.
export default function Board({
  backlogs,
  waiting = {},
  renderRequest,
  qa = {},
  onQADone,
  onOpen,
  onArchive,
  backlogActions,
}: {
  // Trusted projects' workflow backlogs (WFL-004).
  backlogs: BacklogInfo[];
  // How many requests each session has waiting for the user (GUI-002), by session.
  waiting?: Record<string, number>;
  // The request a session waits on longest, answered on its card.
  renderRequest?: (sessionId: string) => ReactNode;
  // What the review of an issue ready for QA answered, by run and issue ("run:issue").
  qa?: Record<string, { reply: string }>;
  // The user checked an issue a run finished (WFL-006): it moves to Done.
  onQADone?: (runId: string, issue: string) => void;
  onOpen: (sessionId: string) => void;
  onArchive?: (b: BacklogInfo) => void;
  // Controls for a project's row, such as its AFK run (WFL-005).
  backlogActions?: (b: BacklogInfo) => ReactNode;
}) {
  const total = backlogs.reduce((n, b) => n + (b.issues ?? []).length, 0);
  const placed = backlogs.map((b) => placeIssues(b.issues, b.run, waiting));
  const waitingNow = placed.reduce((n, cols) => n + waitingIssues(cols), 0);
  const page = useRef<HTMLDivElement>(null);
  return (
    <div className="board-page" ref={page}>
      <header className="board-header">
        <h2>{t("app.board")}</h2>
        <span className="muted">{t("board.issueCount", { count: total })}</span>
        {waitingNow > 0 && (
          // What matters most, first: the agents that wait for an answer, and a way to the first.
          <button className="link board-waiting" onClick={() => page.current?.querySelector(".issue-needs")?.scrollIntoView({ block: "center", behavior: "smooth" })}>
            {t("board.waitingCount", { count: waitingNow })}
          </button>
        )}
      </header>
      {backlogs.length === 0 && <p className="muted board-empty">{t("board.noBacklogs")}</p>}
      <div className="board-lanes">
        {backlogs.map((b, i) => {
          const cols = placed[i];
          const n = counts(b.issues);
          return (
            <section key={b.projectId} className="board-lane" aria-label={t("board.backlogOf", { project: b.projectName })}>
              <header className="board-lane-head">
                <div className="board-lane-title">
                  <strong>{b.projectName}</strong>
                  <span className="muted">
                    {" · "}
                    {t("board.backlogCounts", { open: n.open, ready: n.afkReady })}
                    {b.uncommitted ? ` · ${t("board.notCommitted")}` : ""}
                  </span>
                </div>
                {backlogActions?.(b)}
                {b.finished > 0 && onArchive && (
                  <button className="link board-lane-archive" onClick={() => onArchive(b)} title={t("board.archiveTitle")}>
                    {t("board.archive", { count: b.finished })}
                  </button>
                )}
              </header>
              <div className="board-lane-columns">
                {LANES.map((lane) => (
                  <div key={lane} className={`board-col board-col-${lane}`} role="group" aria-label={laneTitle(lane)}>
                    <h3>
                      {laneTitle(lane)} <span className="muted">{cols[lane].length}</span>
                    </h3>
                    <ul>
                      {cols[lane].map((card) => (
                        <li key={card.id}>
                          <Issue card={card} onOpen={onOpen} reply={b.run ? qa[`${b.run.id}:${card.id}`]?.reply : undefined} onQADone={b.run && onQADone ? () => onQADone(b.run!.id, card.id) : undefined} request={card.waiting > 0 && card.step?.sessionId ? renderRequest?.(card.step.sessionId) : null} />
                        </li>
                      ))}
                    </ul>
                  </div>
                ))}
              </div>
            </section>
          );
        })}
      </div>
    </div>
  );
}

// Issue is one card: the title in at most two lines, one line saying which issue it is,
// who does it and what it waits on or how long it has worked, then a mark or an action
// only where one applies. The whole title and the file are in the hover text.
function Issue({ card, onOpen, request, reply, onQADone }: { card: IssueCard; onOpen: (sessionId: string) => void; request?: ReactNode; reply?: string; onQADone?: () => void }) {
  const [id, type, ...more] = cardMeta(card);
  const step = card.step;
  const waiting = !!step?.waiting && step.state === "working";
  return (
    <div className={`board-card issue-card backlog-card issue-${card.column} backlog-${card.state}${card.waiting > 0 ? " issue-needs" : ""}${card.review ? ` issue-review-${card.review}` : ""}`} role="group" aria-label={`${id}: ${card.title}`} title={card.hover}>
      <span className="issue-title">{card.title}</span>
      <span className="issue-meta">
        <span className="issue-id">{id}</span>
        <span className={`tag backlog-type-${card.type}`}>{type}</span>
        {more.map((m) => (
          <span key={m} className="muted">
            {m}
          </span>
        ))}
      </span>
      {card.waiting > 0 && <span className="issue-mark issue-mark-needs">{t("board.waiting")}</span>}
      {card.review === "reviewing" && <span className="issue-mark muted">{t("board.review.agent")}</span>}
      {card.review === "qa" && <span className="issue-mark issue-mark-qa">{t("board.review.qa")}</span>}
      {card.review === "qa" && reply && <span className="issue-reply muted">{reply}</span>}
      {card.stopped !== undefined && <span className="issue-mark issue-mark-stopped">{card.stopped ? t("board.stopped", { reason: card.stopped }) : t("board.stoppedHere")}</span>}
      {request}
      {card.note && <span className="issue-mark muted">{card.note}</span>}
      {waiting && <span className="issue-mark backlog-step-waiting">{t("board.afkStep", { step: stepLabel(step!) })}</span>}
      {(step?.sessionId || (card.review === "qa" && onQADone)) && (
        <span className="issue-actions">
          {card.review === "qa" && onQADone && (
            <button className="primary" onClick={onQADone}>
              {t("attention.qaFinished")}
            </button>
          )}
          {step?.sessionId && (
            <button className="link" onClick={() => onOpen(step.sessionId!)}>
              {t("board.openSession")}
            </button>
          )}
          {card.review && step?.reviewId && (
            <button className="link" onClick={() => onOpen(step.reviewId!)}>
              {t("attention.openReview")}
            </button>
          )}
        </span>
      )}
    </div>
  );
}
