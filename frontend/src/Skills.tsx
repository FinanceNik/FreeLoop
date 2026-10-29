import { useEffect, useState } from "react";
import { EngineService, type ProjectInfo, type SkillList } from "./api.ts";

// Skills (SKL-002): every skill the app found, where it came from, and
// switches globally and per project. The model sees only the names and
// descriptions of skills that are on, and loads a skill when it needs it.

interface Props {
  projects: ProjectInfo[];
  configVersion: number;
  onConfigChanged: () => void;
  onProjectsChanged: () => void;
  onError: (err: unknown) => void;
}

const SOURCE: Record<string, string> = {
  app: "Skill store",
  ".freeloop": "Project .freeloop/skills",
  ".agents": "Project .agents/skills",
  ".claude": "Project .claude/skills",
};

export default function Skills({ projects, configVersion, onConfigChanged, onProjectsChanged, onError }: Props) {
  const trusted = projects.filter((p) => p.trusted && !p.missing);
  const [root, setRoot] = useState(trusted[0]?.root ?? "");
  const [list, setList] = useState<SkillList | null>(null);
  const load = () =>
    EngineService.ListSkills(root)
      .then(setList)
      .catch(onError);
  useEffect(() => {
    load();
  }, [root, configVersion]);

  const toggle = (name: string, projectId: string, on: boolean) =>
    EngineService.SetSkillEnabled(name, projectId, on)
      .then(() => {
        if (projectId) onProjectsChanged();
        else onConfigChanged();
        load();
      })
      .catch(onError);

  const skills = list?.skills ?? [];
  return (
    <section className="page">
      <h2>Skills</h2>
      <p className="muted">
        Agent Skills from the app's skill store (<code>skills/</code> in the data folder) and, in trusted projects, from <code>.freeloop/skills</code>,{" "}
        <code>.agents/skills</code> and <code>.claude/skills</code>. A project skill hides a store skill of the same name. Profiles can limit a session to
        some skills.
      </p>
      <label className="row">
        Project{" "}
        <select value={root} onChange={(e) => setRoot(e.target.value)}>
          <option value="">None (skill store only)</option>
          {trusted.map((p) => (
            <option key={p.id} value={p.root}>
              {p.name}
            </option>
          ))}
        </select>
      </label>
      {skills.length === 0 && <p className="muted">No skills found.</p>}
      <ul className="provider-list">
        {skills.map((s) => (
          <li key={s.name} className="card skill">
            <div>
              <strong>{s.name}</strong> <span className="muted">{SOURCE[s.source] ?? s.source}</span>
              <div>{s.description}</div>
              <div className="muted">
                <code>{s.path}</code>
                {s.lines > 500 && ` · ${s.lines} lines (the spec recommends under 500)`}
                {(s.allowedTools ?? []).length > 0 && ` · suggests tools: ${(s.allowedTools ?? []).join(", ")}`}
              </div>
            </div>
            <div className="skill-switches">
              <label className="row">
                <input type="checkbox" checked={s.global} onChange={(e) => toggle(s.name, "", e.target.checked)} /> On everywhere
              </label>
              {list?.projectId && (
                <label className="row">
                  <input type="checkbox" checked={s.project} disabled={!s.global} onChange={(e) => toggle(s.name, list.projectId, e.target.checked)} /> In this
                  project
                </label>
              )}
            </div>
          </li>
        ))}
      </ul>
      {(list?.issues ?? []).length > 0 && (
        <div className="banner warn">
          <strong>Skills that could not be loaded</strong>
          <ul>
            {(list?.issues ?? []).map((i, n) => (
              <li key={n}>
                <code>{i.path}</code>: {i.message}
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}
