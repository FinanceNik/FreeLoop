import { currentStep, stageOutputs, stageProgress, type ItemLike, type RunLike, type Stage, type StageState, type WorkflowLike } from "./workflows.ts";
import { t, type PlainKey } from "./i18n.ts";

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

const MARK: Record<StageState, string> = { done: "✓", running: "●", next: "→", todo: "" };
const SAID: Record<StageState, PlainKey> = { done: "workflow.state.done", running: "workflow.state.running", next: "workflow.state.next", todo: "workflow.state.todo" };

export default function WorkflowStrip({ workflow, items, running, run, roots, onNext, onShowRun }: Props) {
  const steps = stageProgress(workflow, items, running, run);
  const next = steps.find((s) => s.state === "next");
  // In a narrow strip only this stage keeps its label; the others shrink to dots.
  const current = currentStep(steps);
  const lastStage = items.filter((it) => it.kind === "stage").pop();
  const wrote = !running && lastStage ? stageOutputs(items) : [];
  const afk = steps.find((s) => s.stage.kind === "afk");
  const files = wrote
    .slice(0, 2)
    .map((p) => relative(p, roots))
    .join(", ");
  return (
    <div className="workflow-strip" role="group" aria-label={t("workflow.label", { name: workflow.name })}>
      <ol className="workflow-steps">
        {steps.map((s, i) => (
          <li key={s.stage.command} className={`workflow-step workflow-step-${s.state}${i === current ? " workflow-step-current" : ""}`} title={`/${s.stage.command}: ${s.stage.description || s.stage.title}`}>
            {MARK[s.state] && (
              <span className="workflow-step-mark" aria-hidden="true">
                {MARK[s.state]}
              </span>
            )}
            <span className="workflow-step-text">
              {s.stage.title}
              {s.detail && <span className="workflow-step-detail"> · {s.detail}</span>}
            </span>
            <span className="sr-only">, {t(SAID[s.state])}</span>
          </li>
        ))}
      </ol>
      {workflow.reason ? (
        <span className="workflow-strip-note error-text" title={workflow.reason}>
          {workflow.reason}
        </span>
      ) : (
        <span className="workflow-strip-actions">
          {wrote.length > 0 && (
            <span className="workflow-wrote muted" title={wrote.join("\n")}>
              {wrote.length > 2 ? t("workflow.wroteMore", { files, count: wrote.length - 2 }) : t("workflow.wrote", { files })}
            </span>
          )}
          {afk && afk.state !== "todo" && afk.state !== "next" && onShowRun && (
            <button className="link" onClick={onShowRun}>
              {t("workflow.showRun")}
            </button>
          )}
          {next && !running && (
            <button
              className="workflow-next"
              onClick={() => onNext(next.stage)}
              title={next.stage.needsInput ? t("workflow.nextNeedsInput", { command: next.stage.command }) : t("workflow.nextRuns", { command: next.stage.command })}
            >
              {t("workflow.next", { title: next.stage.title })}
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
