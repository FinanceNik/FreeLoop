import { useEffect, useEffectEvent, useState } from "react";
import { ask } from "./ask.ts";
import { EngineService, type CompareRun } from "./api.ts";
import { compact } from "./analytics.ts";
import { formatCost } from "./cost.ts";
import { t, type PlainKey } from "./i18n.ts";

// Compare shows the runs of one task on several profiles side by side
// (HRN-016): what each changed, what it used and what it last said.
// Keeping one archives the others; their worktrees go, their branches
// stay.

const STATUS: Record<string, PlainKey> = {
  running: "status.running",
  waiting_permission: "status.waitingPermission",
  waiting_input: "status.waitingInput",
  queued: "status.queued",
  idle: "status.idle",
  error: "status.error",
  archived: "sidebar.filter.archived",
};
const WORKING = new Set(["running", "waiting_permission", "waiting_input", "queued"]);

export default function Compare({ tag, onOpen, onError }: { tag: string; onOpen: (id: string) => void; onError: (err: unknown) => void }) {
  const [runs, setRuns] = useState<CompareRun[] | null>(null);
  const [busy, setBusy] = useState(false);
  const load = () =>
    EngineService.Compare(tag)
      .then((r) => setRuns(r ?? []))
      .catch(onError);
  const loadNow = useEffectEvent(load);
  useEffect(() => {
    loadNow();
  }, [tag]);
  // The runs' numbers move while they work, and a run can start or stop
  // at any time, so the page reads them again every few seconds.
  const working = (runs ?? []).some((r) => WORKING.has(r.session.status));
  useEffect(() => {
    const t = setInterval(() => loadNow(), working ? 3000 : 8000);
    return () => clearInterval(t);
  }, [working, tag]);
  const keep = async (r: CompareRun) => {
    const others = (runs ?? []).filter((x) => x !== r && x.session.status !== "archived").length;
    if (!(await ask(t("compare.keepConfirm", { title: r.session.title, count: others })))) return;
    setBusy(true);
    EngineService.KeepCompare(tag, r.session.id)
      .then(load)
      .catch(onError)
      .finally(() => setBusy(false));
  };
  const title = runs?.[0]?.session.title.replace(/ · [^·]*$/, "") ?? "";

  return (
    <section className="page compare-page">
      <h2>{t("compare.title")}</h2>
      <p className="muted">
        {title ? <strong>{title}</strong> : null} {runs ? t("compare.intro", { count: runs.length }) : t("compare.introSeveral")}
      </p>
      {runs === null && <p className="muted">{t("usagePage.loading")}</p>}
      <div className="compare-grid">
        {(runs ?? []).map((r) => {
          const label = r.session.title.split(" · ").pop();
          const archived = r.session.status === "archived";
          return (
            <article key={r.session.id} className={archived ? "compare-run compare-archived" : "compare-run"}>
              <header>
                <strong>{label}</strong>
                <span className={`badge badge-${r.session.status}`}>{STATUS[r.session.status] ? t(STATUS[r.session.status]) : r.session.status}</span>
              </header>
              <div className="muted compare-model">{r.session.model || r.session.harnessId}</div>
              <dl className="compare-stats">
                <div>
                  <dt>{t("compare.files")}</dt>
                  <dd>{archived ? "–" : r.files}</dd>
                </div>
                <div>
                  <dt>{t("compare.lines")}</dt>
                  <dd>
                    {archived ? (
                      "–"
                    ) : (
                      <>
                        <span className="diff-plus">+{r.added}</span> <span className="diff-minus">−{r.deleted}</span>
                      </>
                    )}
                  </dd>
                </div>
                <div>
                  <dt>{t("tokenFigures.tokens")}</dt>
                  <dd>{compact(r.tokens)}</dd>
                </div>
                <div>
                  <dt>{t("usagePage.col.estCost")}</dt>
                  <dd>{r.priced ? formatCost(r.cost) : "–"}</dd>
                </div>
              </dl>
              <p className="compare-reply">{r.lastReply || <span className="muted">{t("compare.noReply")}</span>}</p>
              <div className="row">
                <button onClick={() => onOpen(r.session.id)}>{t("compare.open")}</button>
                {!archived && (runs ?? []).filter((x) => x.session.status !== "archived").length > 1 && (
                  <button className="primary" disabled={busy || working} title={working ? t("compare.keepWaiting") : undefined} onClick={() => keep(r)}>
                    {t("compare.keep")}
                  </button>
                )}
              </div>
            </article>
          );
        })}
      </div>
    </section>
  );
}
