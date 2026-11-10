import { useEffect, useState } from "react";
import { EngineService, modeName, type Config, type WorkflowSkill, type WorkflowStatus } from "./api.ts";
import { stageAction } from "./workflows.ts";
import { t, type PlainKey } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";
import ModelSets from "./ModelSets.tsx";

// The Settings page for workflows (WFL-001, WFL-003): what each one's stages do,
// the skills it uses and what its AFK run will want allowed. Listing them runs
// nothing; a file's workflow is used only once approved as it is, and its skills
// install only when asked, into Freeloop's own skill store.

interface Props {
  config: Config;
  // Rises when a config file changes, such as a file in workflows/.
  version: number;
  onSaved: (c: Config) => void;
  onError: (err: unknown) => void;
}

const SKILL_STATE: Record<string, PlainKey> = {
  installed: "workflows.skill.installed",
  changed: "workflows.skill.changed",
  outdated: "workflows.skill.outdated",
  other: "workflows.skill.other",
  missing: "workflows.skill.missing",
};

// stageLine says what a stage does, in which mode and with which skill.
function stageLine(kind: string, mode?: string, skill?: string): string {
  const action = stageAction(kind);
  if (mode && skill) return t("workflows.stage.inModeWithSkill", { action, mode: modeName(mode), skill });
  if (mode) return t("workflows.stage.inMode", { action, mode: modeName(mode) });
  if (skill) return t("workflows.stage.withSkill", { action, skill });
  return action;
}

export default function Workflows({ config, version, onSaved, onError }: Props) {
  const [list, setList] = useState<WorkflowStatus[] | null>(null);
  const [installing, setInstalling] = useState("");
  const [installed, setInstalled] = useState<Record<string, string>>({});
  const refresh = () =>
    EngineService.ListWorkflows()
      .then((l) => setList(l ?? []))
      .catch(onError);
  useEffect(() => {
    refresh();
  }, [version]);

  const approve = (w: WorkflowStatus) =>
    EngineService.ApproveWorkflow(w.id, w.fingerprint)
      .then((c) => {
        onSaved(c);
        refresh();
      })
      .catch(onError);
  const install = (w: WorkflowStatus) => {
    setInstalling(w.id);
    EngineService.InstallWorkflowSkills(w.id)
      .then((names) => {
        setInstalled((old) => ({ ...old, [w.id]: names?.length ? t("workflows.installed", { names: names.join(", ") }) : t("workflows.nothingToInstall") }));
        return refresh();
      })
      .catch(onError)
      .finally(() => setInstalling(""));
  };
  const setDefault = (id: string) => EngineService.SetDefaultWorkflow(id).then(onSaved).catch(onError);
  // WFL-013: the model set a workflow uses, kept in config.yaml, not in its file.
  const setModelSet = (w: WorkflowStatus, id: string) =>
    EngineService.SetWorkflowModelSet(w.id, id)
      .then((c) => {
        onSaved(c);
        refresh();
      })
      .catch(onError);
  const needsInstall = (skills: WorkflowSkill[] | null) => (skills ?? []).some((s) => s.shipped && (s.state === "missing" || s.state === "outdated"));

  return (
    <section className="page">
      <h2>{t("settings.page.workflows")}</h2>
      <p className="muted">{t("workflows.intro")}</p>
      <div className="card form">
        <label>
          {t("workflows.forNewSessions")}
          <select value={config.defaults.workflow ?? ""} onChange={(e) => setDefault(e.target.value)}>
            <option value="">{t("workflows.off")}</option>
            {(list ?? []).map((w) => (
              <option key={w.id} value={w.id}>
                {w.needsApproval ? t("composer.workflow.notApproved", { name: w.name }) : w.name}
              </option>
            ))}
          </select>
          <span className="muted">{t("workflows.forNewSessionsHint")}</span>
        </label>
      </div>
      <ModelSets version={version} onSaved={(c) => {
        onSaved(c);
        refresh();
      }} onError={onError} />
      {list === null && <p className="muted">{t("workflows.loading")}</p>}
      {(list ?? []).map((w) => (
        <div className="card workflow-card" key={w.id} aria-label={w.name}>
          <h3>{w.name}</h3>
          {w.description && <p>{w.description}</p>}
          <p className="muted">
            {w.source ? t("workflows.fromFile", { source: w.source }) : t("workflows.builtIn")}{" "}
            {tNodes("workflows.backlog", { folder: <code>{w.backlog || "backlog"}</code>, prds: <code>{"prds/"}</code>, issues: <code>{"issues/"}</code> })}
          </p>
          {w.needsApproval && (
            <>
              <p>{t("workflows.notApproved")}</p>
              <p className="row">
                <button onClick={() => approve(w)}>{t("pending.approve")}</button>
              </p>
            </>
          )}
          <div className="form">
            <label>
              {t("workflows.modelSet")}
              <select value={w.modelSet ?? ""} onChange={(e) => setModelSet(w, e.target.value)}>
                <option value="">{t("workflows.modelSet.none")}</option>
                {(config.modelSets ?? []).map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.name}
                  </option>
                ))}
              </select>
              <span className="muted">{t("workflows.modelSet.hint")}</span>
            </label>
          </div>
          <h4>{t("workflows.stages")}</h4>
          <ol className="workflow-stages">
            {(w.stages ?? []).map((s) => (
              <li key={s.command}>
                <code>/{s.command}</code> {s.title}
                <span className="muted"> · {stageLine(s.kind, s.mode, s.skill)}</span>
              </li>
            ))}
          </ol>
          {(w.skills ?? []).length > 0 && (
            <>
              <h4>{t("workflows.skills")}</h4>
              <ul className="workflow-skills">
                {(w.skills ?? []).map((s) => (
                  <li key={s.name}>
                    <code>{s.name}</code> <span className={s.state === "missing" ? "error-text" : "muted"}>{SKILL_STATE[s.state] ? t(SKILL_STATE[s.state]) : s.state}</span>
                    {s.off && <span className="muted"> · {t("workflows.skill.off")}</span>}
                  </li>
                ))}
              </ul>
              {needsInstall(w.skills) && (
                <p className="row">
                  <button className="primary" disabled={installing === w.id} onClick={() => install(w)}>
                    {installing === w.id ? t("workflows.installing") : t("workflows.install")}
                  </button>
                  <span className="muted">{t("workflows.installNote")}</span>
                </p>
              )}
              {installed[w.id] && <p className="muted">{installed[w.id]}</p>}
            </>
          )}
          {(w.stages ?? []).some((s) => s.kind === "afk") && (
            <>
              <h4>{t("workflows.afkRun")}</h4>
              <p className="muted">
                {t(w.afk.sandbox ? "workflows.afk.planSandboxed" : "workflows.afk.plan", { mode: modeName(w.afk.mode || "accept_edits"), count: w.afk.maxIssues || 10 })}{" "}
                {w.review.off ? t("workflows.afk.noReview") : t("workflows.afk.review")}
              </p>
              {(w.afk.commands ?? []).length > 0 && (
                <p className="muted">
                  {tNodes("workflows.afk.commands", {
                    commands: (w.afk.commands ?? []).map((c, i) => (
                      <span key={c}>
                        {i > 0 && ", "}
                        <code>{c}</code>
                      </span>
                    )),
                  })}
                </p>
              )}
            </>
          )}
        </div>
      ))}
      <p className="muted">
        {tNodes("workflows.addAnother", { folder: <code>{"workflows"}</code> })} {tNodes("workflows.fields", { doc: <code>{"docs/DEVELOPING.md"}</code> })}
      </p>
    </section>
  );
}
