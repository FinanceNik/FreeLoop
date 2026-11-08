import { useEffect, useState } from "react";
import { EngineService, MODES, type Config, type WorkflowSkill, type WorkflowStatus } from "./api.ts";
import { stageAction } from "./workflows.ts";

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

const SKILL_STATE: Record<string, string> = {
  installed: "installed",
  changed: "installed, and changed since",
  outdated: "installed; this version of Freeloop has a newer text",
  other: "a skill of this name from elsewhere is used",
  missing: "not installed",
};

function modeName(id: string | undefined): string {
  return MODES.find((m) => m.id === id)?.name ?? "the session's mode";
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
        setInstalled((old) => ({ ...old, [w.id]: names?.length ? `Installed ${names.join(", ")}.` : "Nothing to install." }));
        return refresh();
      })
      .catch(onError)
      .finally(() => setInstalling(""));
  };
  const setDefault = (id: string) => EngineService.SetDefaultWorkflow(id).then(onSaved).catch(onError);
  const needsInstall = (skills: WorkflowSkill[] | null) => (skills ?? []).some((s) => s.shipped && (s.state === "missing" || s.state === "outdated"));

  return (
    <section className="page">
      <h2>Workflows</h2>
      <p className="muted">
        A workflow is a way of working in stages. Each stage is a slash command in a session: it sends a prompt that names a skill, switches the mode, or starts an AFK run, a review or a clean-up. Every tool call still goes through your policy; an AFK run asks you up front for the commands it will need, and anything else it wants waits in Needs attention.
      </p>
      <div className="card form">
        <label>
          For new sessions
          <select value={config.defaults.workflow ?? ""} onChange={(e) => setDefault(e.target.value)}>
            <option value="">Off</option>
            {(list ?? []).map((w) => (
              <option key={w.id} value={w.id}>
                {w.needsApproval ? `${w.name} (not approved)` : w.name}
              </option>
            ))}
          </select>
          <span className="muted">A profile can choose its own, and each session can switch from beside its optimizer.</span>
        </label>
      </div>
      {list === null && <p className="muted">Loading…</p>}
      {(list ?? []).map((w) => (
        <div className="card workflow-card" key={w.id} aria-label={w.name}>
          <h3>{w.name}</h3>
          {w.description && <p>{w.description}</p>}
          <p className="muted">
            {w.source ? `From ${w.source} in the data folder.` : "Built into Freeloop."} Its backlog is the project's <code>{w.backlog || "backlog"}</code> folder: PRDs in <code>prds/</code>, issues in <code>issues/</code>.
          </p>
          {w.needsApproval && (
            <>
              <p>Not approved. Until you approve it as it is now, none of its stages run.</p>
              <p className="row">
                <button onClick={() => approve(w)}>Approve</button>
              </p>
            </>
          )}
          <h4>Stages</h4>
          <ol className="workflow-stages">
            {(w.stages ?? []).map((s) => (
              <li key={s.command}>
                <code>/{s.command}</code> {s.title}
                <span className="muted">
                  {" "}
                  · {stageAction(s.kind)}
                  {s.mode ? `, in ${modeName(s.mode)} mode` : ""}
                  {s.skill ? `, with the skill ${s.skill}` : ""}
                </span>
              </li>
            ))}
          </ol>
          {(w.skills ?? []).length > 0 && (
            <>
              <h4>Skills</h4>
              <ul className="workflow-skills">
                {(w.skills ?? []).map((s) => (
                  <li key={s.name}>
                    <code>{s.name}</code> <span className={s.state === "missing" ? "error-text" : "muted"}>{SKILL_STATE[s.state] ?? s.state}</span>
                    {s.off && <span className="muted"> · turned off in Skills</span>}
                  </li>
                ))}
              </ul>
              {needsInstall(w.skills) && (
                <p className="row">
                  <button className="primary" disabled={installing === w.id} onClick={() => install(w)}>
                    {installing === w.id ? "Installing…" : "Install skills"}
                  </button>
                  <span className="muted">Into Freeloop's own skill store, with their source recorded. Nothing is written to ~/.claude or a project.</span>
                </p>
              )}
              {installed[w.id] && <p className="muted">{installed[w.id]}</p>}
            </>
          )}
          {(w.stages ?? []).some((s) => s.kind === "afk") && (
            <>
              <h4>AFK run</h4>
              <p className="muted">
                One fresh session per unblocked AFK issue, one at a time, in a worktree of the run's own, in {modeName(w.afk.mode || "accept_edits")} mode
                {w.afk.sandbox ? ", sandboxed where your system offers it" : ""}, at most {w.afk.maxIssues || 10} issues a run. {w.review.off ? "Finished issues are not reviewed." : "Each finished issue is reviewed by a second agent before it waits for your QA."}
              </p>
              {(w.afk.commands ?? []).length > 0 && (
                <p className="muted">
                  It will want these commands, besides the project's tests and type checks, which you can allow for the run when you start it: {(w.afk.commands ?? []).map((c, i) => (
                    <span key={c}>
                      {i > 0 && ", "}
                      <code>{c}</code>
                    </span>
                  ))}
                  .
                </p>
              )}
            </>
          )}
        </div>
      ))}
      <p className="muted">
        To add another, put a file in the <code>workflows</code> folder of the data folder and approve it here. <code>docs/DEVELOPING.md</code> lists the fields.
      </p>
    </section>
  );
}
