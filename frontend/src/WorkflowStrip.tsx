import { stageOutputs, stageProgress, type ItemLike, type RunLike, type Stage, type WorkflowLike } from "./workflows.ts";

// WorkflowStrip shows, above the composer, where the session is in its workflow
// (WFL-008): each stage on the path, done, running, next or to do, with what the
// last stage wrote and a button for the next one. Choosing a workflow is enough to
// see it, so whether one is on and what comes next never has to be remembered.

interface Props {
  workflow: WorkflowLike;
  items: readonly ItemLike[];
  running: boolean;
  run?: RunLike | null;
  roots: string[];
  onNext: (stage: Stage) => void;
  onShowRun?: () => void;
}

const MARK: Record<string, string> = { done: "✓", running: "●", next: "→", todo: "" };
const SAID: Record<string, string> = { done: "done", running: "running", next: "next", todo: "to do" };

export default function WorkflowStrip({ workflow, items, running, run, roots, onNext, onShowRun }: Props) {
  const steps = stageProgress(workflow, items, running, run);
  const next = steps.find((s) => s.state === "next");
  const lastStage = items.filter((it) => it.kind === "stage").pop();
  const wrote = !running && lastStage ? stageOutputs(items) : [];
  const afk = steps.find((s) => s.stage.kind === "afk");
  return (
    <div className="workflow-strip" role="group" aria-label={`Workflow: ${workflow.name}`}>
      <ol className="workflow-steps">
        {steps.map((s) => (
          <li key={s.stage.command} className={`workflow-step workflow-step-${s.state}`} title={`/${s.stage.command}: ${s.stage.description || s.stage.title}`}>
            {MARK[s.state] && (
              <span className="workflow-step-mark" aria-hidden="true">
                {MARK[s.state]}
              </span>
            )}
            {s.stage.title}
            {s.detail && <span className="workflow-step-detail"> · {s.detail}</span>}
            <span className="sr-only">, {SAID[s.state]}</span>
          </li>
        ))}
      </ol>
      {workflow.reason ? (
        <span className="workflow-strip-note error-text">{workflow.reason}</span>
      ) : (
        <span className="workflow-strip-actions">
          {wrote.length > 0 && (
            <span className="workflow-wrote muted" title={wrote.join("\n")}>
              Wrote {wrote.slice(0, 2).map((p) => relative(p, roots)).join(", ")}
              {wrote.length > 2 ? ` and ${wrote.length - 2} more` : ""}
            </span>
          )}
          {afk && afk.state !== "todo" && afk.state !== "next" && onShowRun && (
            <button className="link" onClick={onShowRun}>
              Show the run
            </button>
          )}
          {next && !running && (
            <button className="workflow-next" onClick={() => onNext(next.stage)} title={next.stage.needsInput ? `Type what /${next.stage.command} is about, then send` : `Runs /${next.stage.command}`}>
              Next: {next.stage.title}
            </button>
          )}
        </span>
      )}
    </div>
  );
}

// relative shows a path inside the session's folders without the folder.
function relative(p: string, roots: string[]): string {
  for (const root of roots) {
    if (root && p.startsWith(root.endsWith("/") ? root : `${root}/`)) return p.slice(root.length).replace(/^\/+/, "");
  }
  return p;
}
