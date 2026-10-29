import { useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { EngineService, FILES_DROPPED_EVENT_NAME, type ProjectInfo, type SkillCandidate, type SkillList } from "./api.ts";

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

const STATUS: Record<string, string> = { new: "new", same: "up to date", changed: "changed", invalid: "cannot load" };

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

  // Import and install (SKL-003, SKL-005): preview what a source offers,
  // then copy the chosen skills into the store.
  const [preview, setPreview] = useState<{ src: string; cands: SkillCandidate[]; picked: string[] } | null>(null);
  const [busy, setBusy] = useState(false);
  const [installed, setInstalled] = useState("");
  const [path, setPath] = useState("");
  const open = (src: string) => {
    if (!src) return;
    setInstalled("");
    EngineService.PreviewSkills(src)
      .then((list) => {
        const cands = list ?? [];
        setPreview({ src, cands, picked: cands.filter((c) => c.status === "new" || c.status === "changed").map((c) => c.name) });
      })
      .catch(onError);
  };
  const importClaude = () =>
    EngineService.ClaudeSkillsDir()
      .then(open)
      .catch(onError);
  const chooseFolder = () =>
    EngineService.ChooseFolder()
      .then((p) => p && open(p))
      .catch(onError);
  // A folder or .zip dropped on the drop zone.
  useEffect(() => Events.On(FILES_DROPPED_EVENT_NAME, ({ data }) => open(((data as string[]) ?? [])[0] ?? "")), []);
  const install = () => {
    if (!preview) return;
    setBusy(true);
    EngineService.InstallSkills(preview.src, preview.picked)
      .then((done) => {
        setInstalled((done ?? []).length > 0 ? `Installed ${(done ?? []).join(", ")}.` : "Nothing to install; the chosen skills are up to date.");
        setPreview(null);
        load();
      })
      .catch(onError)
      .finally(() => setBusy(false));
  };
  const remove = (name: string) => {
    if (!confirm(`Remove ${name} from the skill store? Its folder is deleted.`)) return;
    EngineService.RemoveSkill(name).then(load).catch(onError);
  };

  const skills = list?.skills ?? [];
  return (
    <section className="page">
      <h2>Skills</h2>
      <p className="muted">
        Agent Skills from the app's skill store (<code>skills/</code> in the data folder) and, in trusted projects, from <code>.freeloop/skills</code>,{" "}
        <code>.agents/skills</code> and <code>.claude/skills</code>. A project skill hides a store skill of the same name. Profiles can limit a session to
        some skills.
      </p>
      <div className="card skill-install" data-file-drop-target="skills">
        <div className="row wrap">
          <button onClick={importClaude}>Import from Claude Code</button>
          <button onClick={chooseFolder}>Install from a folder…</button>
          <span className="muted">or drop a skill folder or .zip here. Skills are copied into the store; the source is not changed.</span>
        </div>
        <form
          className="row"
          onSubmit={(e) => {
            e.preventDefault();
            open(path.trim());
          }}
        >
          <input value={path} onChange={(e) => setPath(e.target.value)} placeholder="Or paste the path of a skill folder or .zip" aria-label="Skill source path" />
          <button type="submit" disabled={!path.trim()}>
            Preview
          </button>
        </form>
        {installed && <p className="ok">{installed}</p>}
        {preview && (
          <div className="skill-preview">
            <p className="muted">
              From <code>{preview.src}</code>
            </p>
            {preview.cands.map((c) => (
              <div key={c.name} className="skill-candidate">
                <label className="row">
                  <input
                    type="checkbox"
                    disabled={c.status === "invalid" || c.status === "same"}
                    checked={preview.picked.includes(c.name)}
                    onChange={(e) =>
                      setPreview({ ...preview, picked: e.target.checked ? [...preview.picked, c.name] : preview.picked.filter((n) => n !== c.name) })
                    }
                  />
                  <strong>{c.name}</strong> <span className={`badge skill-${c.status}`}>{STATUS[c.status] ?? c.status}</span>
                </label>
                {c.description && <div>{c.description}</div>}
                {c.issue && <div className="error-text">{c.issue}</div>}
                {(c.scripts ?? []).length > 0 && (
                  <div className="muted">
                    Scripts: <code>{(c.scripts ?? []).join(", ")}</code>. They run only through the shell tool, which asks before each command.
                  </div>
                )}
                {(c.changes ?? []).length > 0 && (
                  <pre className="skill-changes">{(c.changes ?? []).join("\n")}</pre>
                )}
                {c.installed && c.status !== "new" && (
                  <div className="muted">
                    Installed from <code>{c.installed.source}</code>
                    {c.installed.importedAt && !c.installed.importedAt.startsWith("0001") ? ` on ${new Date(c.installed.importedAt).toLocaleDateString()}` : ""}
                  </div>
                )}
              </div>
            ))}
            <div className="row">
              <button className="primary" disabled={busy || preview.picked.length === 0} onClick={install}>
                Install {preview.picked.length} {preview.picked.length === 1 ? "skill" : "skills"}
              </button>
              <button onClick={() => setPreview(null)}>Cancel</button>
            </div>
          </div>
        )}
      </div>
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
            {s.source === "app" && (
              <button className="link" onClick={() => remove(s.name)}>
                Remove
              </button>
            )}
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
