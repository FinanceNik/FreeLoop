import { useEffect, useEffectEvent } from "react";
import { EngineService, type AttentionItem, type BacklogInfo } from "./api.ts";
import { runCostLine, runLine, stepLabel } from "./afk.ts";
import { t } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";

// AFKRunView is a project's AFK run in full (WFL-009): each issue it took on, with
// its state, why it waits, its session and review, the approvals its session waits
// for, and QA for what is finished; then what is still to come. The run's own
// controls are here too, so following a run needs no other page.

interface Props {
  backlog: BacklogInfo;
  attention: AttentionItem[];
  onOpen: (sessionId: string) => void;
  onClose: () => void;
  onChanged: () => void;
  onError: (err: unknown) => void;
}

export default function AFKRunView({ backlog, attention, onOpen, onClose, onChanged, onError }: Props) {
  const run = backlog.run;
  const escape = useEffectEvent(() => onClose());
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && escape();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  if (!run) return null;
  const going = run.state === "running" || run.state === "paused";
  const steps = run.steps ?? [];
  const taken = new Set(steps.map((s) => s.issue));
  const toCome = (backlog.issues ?? []).filter((it) => it.state !== "done" && it.type === "afk" && !taken.has(it.id));
  const asks = (sid?: string) => (sid ? attention.filter((a) => a.session.id === sid).reduce((n, a) => n + (a.requests?.length ?? 0), 0) : 0);
  const act = (p: Promise<unknown>) => p.then(onChanged).catch(onError);
  return (
    <div className="palette-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="afk-dialog afk-run-view" role="dialog" aria-modal="true" aria-label={t("afk.run.title", { project: backlog.projectName })}>
        <h2>{t("afk.run.title", { project: backlog.projectName })}</h2>
        <p className="muted" role="status">
          {runLine(run)}
          {run.branch && (
            <>
              {" · "}
              {tNodes("afk.run.onBranch", { branch: <code>{run.branch}</code> })}
            </>
          )}
        </p>
        {/* What it cost, by part and model (WFL-015). */}
        {backlog.runCost && runCostLine(backlog.runCost) && <p className="muted afk-run-cost">{runCostLine(backlog.runCost)}</p>}
        <ol className="afk-run-steps">
          {steps.map((s) => {
            const waiting = s.state === "working" && !!s.waiting;
            const n = asks(s.sessionId);
            return (
              <li key={s.issue} className={`afk-run-step afk-run-step-${s.state}`}>
                <div className="afk-run-step-head">
                  <strong>{s.title}</strong> <span className="muted">{s.issue}</span>
                </div>
                <div className={waiting ? "afk-run-step-state backlog-step-waiting" : "afk-run-step-state"}>{stepLabel(s)}</div>
                {s.note && <div className="muted">{s.note}</div>}
                <div className="row afk-run-step-actions">
                  {n > 0 && (
                    <button className="primary" onClick={() => onOpen(s.sessionId!)}>
                      {t("afk.run.approvals", { count: n })}
                    </button>
                  )}
                  {s.sessionId && (
                    <button className="link" onClick={() => onOpen(s.sessionId!)}>
                      {t("afk.run.openSession")}
                    </button>
                  )}
                  {s.reviewId && (
                    <button className="link" onClick={() => onOpen(s.reviewId!)}>
                      {t("afk.run.openReview")}
                    </button>
                  )}
                  {s.state === "qa" && <button onClick={() => act(EngineService.QADone(run.id, s.issue))}>{t("afk.run.qaDone")}</button>}
                </div>
              </li>
            );
          })}
        </ol>
        {toCome.length > 0 && (
          <>
            <h3>{t("afk.run.toCome")}</h3>
            <ul className="afk-run-to-come">
              {toCome.map((it) => (
                <li key={it.id}>
                  {it.title} <span className="muted">{it.state === "ready" ? t("afk.run.ready") : t("afk.run.blocked")}</span>
                </li>
              ))}
            </ul>
          </>
        )}
        <p className="row afk-actions">
          {going && <button onClick={() => act(EngineService.PauseAFK(run.id, run.state === "running"))}>{run.state === "running" ? t("app.afk.pause") : t("app.afk.resume")}</button>}
          {going && <button onClick={() => act(EngineService.StopAFK(run.id))}>{t("app.afk.stop")}</button>}
          <button onClick={onClose}>{t("afk.run.close")}</button>
        </p>
      </div>
    </div>
  );
}
