import { useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { EngineService, FILES_DROPPED_EVENT_NAME, type ProjectInfo, type ProviderInfo, type SkillCandidate, type SkillList, type SkillTest } from "./api.ts";

// Skills (SKL-002): every skill the app found, where it came from, and
// switches globally and per project. The model sees only the names and
// descriptions of skills that are on, and loads a skill when it needs it.

interface Props {
  projects: ProjectInfo[];
  providers: ProviderInfo[];
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

export default function Skills({ projects, providers, configVersion, onConfigChanged, onProjectsChanged, onError }: Props) {
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

  // Authoring (SKL-007): a new skill from the template, opened in the
  // editor, and a test run against a model.
  const [creating, setCreating] = useState<{ name: string; description: string } | null>(null);
  const create = () => {
    if (!creating) return;
    EngineService.CreateSkill(creating.name.trim(), creating.description)
      .then(() => {
        const name = creating.name.trim();
        setCreating(null);
        load();
        return EngineService.OpenSkill(name);
      })
      .catch(onError);
  };
  const [testing, setTesting] = useState<{ name: string; providerId: string; model: string; prompt: string; result?: SkillTest; busy?: boolean } | null>(null);
  const runTest = () => {
    if (!testing) return;
    setTesting({ ...testing, busy: true, result: undefined });
    EngineService.TestSkill({ name: testing.name, providerId: testing.providerId, model: testing.model, prompt: testing.prompt, root })
      .then((result) => setTesting((t) => (t ? { ...t, result, busy: false } : t)))
      .catch((err) => {
        setTesting((t) => (t ? { ...t, busy: false } : t));
        onError(err);
      });
  };
  // Git-installed skills (SKL-006): check for updates, update, roll back.
  const [update, setUpdate] = useState<{ name: string; cand?: SkillCandidate; busy: boolean } | null>(null);
  const checkUpdate = (name: string) => {
    setUpdate({ name, busy: true });
    EngineService.CheckSkillUpdate(name)
      .then((cand) => setUpdate({ name, cand, busy: false }))
      .catch((err) => {
        setUpdate(null);
        onError(err);
      });
  };
  const applyUpdate = (name: string) =>
    EngineService.UpdateSkill(name)
      .then(() => {
        setUpdate(null);
        load();
      })
      .catch(onError);
  const rollBack = (name: string) => {
    if (!confirm(`Put back the version of ${name} its last install replaced? You can undo this the same way.`)) return;
    EngineService.RollbackSkill(name).then(load).catch(onError);
  };
  const testable = providers.filter((p) => p.hasKey || p.type === "ollama" || p.builtin || p.baseUrl.startsWith("http://localhost"));

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
          <button onClick={() => setCreating(creating ? null : { name: "", description: "" })}>New skill…</button>
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
          <input
            value={path}
            onChange={(e) => setPath(e.target.value)}
            placeholder="Or paste a skill folder or .zip path, or a git URL (add #branch, #tag or #commit to pin it)"
            aria-label="Skill source path or git URL"
          />
          <button type="submit" disabled={!path.trim()}>
            Preview
          </button>
        </form>
        {creating && (
          <form
            className="skill-create"
            onSubmit={(e) => {
              e.preventDefault();
              create();
            }}
          >
            <label>
              Name <span className="muted">(lowercase letters, digits and hyphens; the folder's name)</span>
              <input
                autoFocus
                required
                pattern="[a-z0-9]+(-[a-z0-9]+)*"
                maxLength={64}
                value={creating.name}
                onChange={(e) => setCreating({ ...creating, name: e.target.value })}
              />
            </label>
            <label>
              Description <span className="muted">(what it does and when to use it; models pick skills by this)</span>
              <textarea required rows={2} maxLength={1024} value={creating.description} onChange={(e) => setCreating({ ...creating, description: e.target.value })} />
            </label>
            <div className="row">
              <button type="submit" className="primary">
                Create and open
              </button>
              <button type="button" onClick={() => setCreating(null)}>
                Cancel
              </button>
            </div>
          </form>
        )}
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
                {c.diff && <pre className="skill-diff">{c.diff}</pre>}
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
                {s.installed?.commit && ` · from ${s.installed.source} at ${s.installed.commit.slice(0, 8)}${s.installed.ref ? ` (${s.installed.ref})` : ""}`}
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
            <div className="row">
              {s.source === "app" && (
                <button className="link" onClick={() => EngineService.OpenSkill(s.name).catch(onError)}>
                  Edit
                </button>
              )}
              <button
                className="link"
                onClick={() =>
                  setTesting(testing?.name === s.name ? null : { name: s.name, providerId: testable[0]?.id ?? "", model: testable[0]?.defaultModel ?? "", prompt: "" })
                }
              >
                Test…
              </button>
              {s.installed?.commit && (
                <button className="link" disabled={update?.busy} onClick={() => checkUpdate(s.name)}>
                  {update?.name === s.name && update.busy ? "Checking…" : "Check for updates"}
                </button>
              )}
              {s.canRollBack && (
                <button className="link" onClick={() => rollBack(s.name)}>
                  Roll back
                </button>
              )}
              {s.source === "app" && (
                <button className="link" onClick={() => remove(s.name)}>
                  Remove
                </button>
              )}
            </div>
            {update?.name === s.name && update.cand && (
              <div className="skill-update">
                {update.cand.status === "same" ? (
                  <p className="ok">Up to date with {update.cand.source}.</p>
                ) : (
                  <>
                    <p>
                      {update.cand.source} has changes: {(update.cand.changes ?? []).join(", ")}
                    </p>
                    {update.cand.diff && <pre className="skill-diff">{update.cand.diff}</pre>}
                    <div className="row">
                      <button className="primary" disabled={update.cand.status === "invalid"} onClick={() => applyUpdate(s.name)}>
                        Update
                      </button>
                      <button onClick={() => setUpdate(null)}>Not now</button>
                    </div>
                    {update.cand.issue && <p className="error-text">{update.cand.issue}</p>}
                  </>
                )}
              </div>
            )}
            {testing?.name === s.name && (
              <form
                className="skill-test"
                onSubmit={(e) => {
                  e.preventDefault();
                  runTest();
                }}
              >
                <label>
                  A prompt that should use this skill
                  <input required value={testing.prompt} onChange={(e) => setTesting({ ...testing, prompt: e.target.value })} />
                </label>
                <div className="row wrap">
                  <select
                    aria-label="Provider"
                    value={testing.providerId}
                    onChange={(e) => setTesting({ ...testing, providerId: e.target.value, model: providers.find((p) => p.id === e.target.value)?.defaultModel ?? "" })}
                  >
                    {providers.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </select>
                  <input aria-label="Model" placeholder="Model" value={testing.model} onChange={(e) => setTesting({ ...testing, model: e.target.value })} />
                  <button type="submit" disabled={testing.busy || !testing.providerId}>
                    {testing.busy ? "Asking the model…" : "Run the test"}
                  </button>
                </div>
                {testing.result && (
                  <p className={testing.result.picked ? "ok" : "banner warn"}>
                    {testing.result.picked
                      ? "The model loaded the skill."
                      : testing.result.loaded
                        ? `The model loaded ${testing.result.loaded} instead.`
                        : "The model did not load the skill. Try a description that names the task and when to use it."}
                    {testing.result.reply && <span className="muted"> It said: {testing.result.reply.slice(0, 300)}</span>}
                  </p>
                )}
                <p className="muted">One model call with the skills a session lists; it uses tokens.</p>
              </form>
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
