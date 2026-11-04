import { useEffect, useState } from "react";
import { ask } from "./ask.ts";
import { EngineService, type CompareRun } from "./api.ts";
import { compact } from "./analytics.ts";
import { formatCost } from "./cost.ts";

// Compare shows the runs of one task on several profiles side by side
// (HRN-016): what each changed, what it used and what it last said.
// Keeping one archives the others; their worktrees go, their branches
// stay.

const STATUS: Record<string, string> = { running: "Running", waiting_permission: "Needs you", waiting_input: "Waiting", queued: "Queued", idle: "Idle", error: "Error", archived: "Archived" };
const WORKING = new Set(["running", "waiting_permission", "waiting_input", "queued"]);

export default function Compare({ tag, onOpen, onError }: { tag: string; onOpen: (id: string) => void; onError: (err: unknown) => void }) {
  const [runs, setRuns] = useState<CompareRun[] | null>(null);
  const [busy, setBusy] = useState(false);
  const load = () =>
    EngineService.Compare(tag)
      .then((r) => setRuns(r ?? []))
      .catch(onError);
  useEffect(() => {
    load();
  }, [tag]);
  // The runs' numbers move while they work, and a run can start or stop
  // at any time, so the page reads them again every few seconds.
  const working = (runs ?? []).some((r) => WORKING.has(r.session.status));
  useEffect(() => {
    const t = setInterval(load, working ? 3000 : 8000);
    return () => clearInterval(t);
  }, [working, tag]);
  const keep = async (r: CompareRun) => {
    const others = (runs ?? []).filter((x) => x !== r && x.session.status !== "archived").length;
    if (!(await ask(`Keep "${r.session.title}" and archive the ${others === 1 ? "other run" : `${others} other runs`}? Their worktrees are removed; their branches stay.`))) return;
    setBusy(true);
    EngineService.KeepCompare(tag, r.session.id)
      .then(load)
      .catch(onError)
      .finally(() => setBusy(false));
  };
  const title = runs?.[0]?.session.title.replace(/ · [^·]*$/, "") ?? "";

  return (
    <section className="page compare-page">
      <h2>Compare runs</h2>
      <p className="muted">
        {title ? <strong>{title}</strong> : null} One task on {runs?.length ?? "several"} profiles, each in its own worktree. Open a run to read it or
        review its diff; keep the one you want.
      </p>
      {runs === null && <p className="muted">Loading…</p>}
      <div className="compare-grid">
        {(runs ?? []).map((r) => {
          const label = r.session.title.split(" · ").pop();
          const archived = r.session.status === "archived";
          return (
            <article key={r.session.id} className={archived ? "compare-run compare-archived" : "compare-run"}>
              <header>
                <strong>{label}</strong>
                <span className={`badge badge-${r.session.status}`}>{STATUS[r.session.status] ?? r.session.status}</span>
              </header>
              <div className="muted compare-model">{r.session.model || r.session.harnessId}</div>
              <dl className="compare-stats">
                <div>
                  <dt>Files</dt>
                  <dd>{archived ? "–" : r.files}</dd>
                </div>
                <div>
                  <dt>Lines</dt>
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
                  <dt>Tokens</dt>
                  <dd>{compact(r.tokens)}</dd>
                </div>
                <div>
                  <dt>Est. cost</dt>
                  <dd>{r.priced ? formatCost(r.cost) : "–"}</dd>
                </div>
              </dl>
              <p className="compare-reply">{r.lastReply || <span className="muted">No reply yet.</span>}</p>
              <div className="row">
                <button onClick={() => onOpen(r.session.id)}>Open</button>
                {!archived && (runs ?? []).filter((x) => x.session.status !== "archived").length > 1 && (
                  <button className="primary" disabled={busy || working} title={working ? "Wait until every run has stopped" : undefined} onClick={() => keep(r)}>
                    Keep this one
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
