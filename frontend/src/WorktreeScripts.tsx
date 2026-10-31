import { useEffect, useState } from "react";
import { EngineService, type ProjectScriptsInfo } from "./api.ts";

// WorktreeScripts shows and edits a project's worktree scripts (SES-016)
// under the new-session form's worktree option: what applies, where it
// comes from, and the app's own values, which override the project's
// .freeloop/worktree.yaml field by field.

const FIELDS = [
  { key: "setup", label: "Setup", hint: "Runs once when the worktree is made, before the first message, such as npm install." },
  { key: "run", label: "Run", hint: "Starts from the session's Run button in a terminal tab, such as npm run dev -- --port $FREELOOP_PORT." },
  { key: "archive", label: "Archive", hint: "Runs before the worktree is removed, such as docker compose down." },
] as const;

export default function WorktreeScripts({ projectId, onError }: { projectId: string; onError: (err: unknown) => void }) {
  const [info, setInfo] = useState<ProjectScriptsInfo | null>(null);
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState({ copy: "", setup: "", run: "", archive: "" });
  const load = () =>
    EngineService.ProjectScripts(projectId)
      .then((i) => {
        setInfo(i);
        setForm({ copy: (i.app.copy ?? []).join("\n"), setup: i.app.setup, run: i.app.run, archive: i.app.archive });
      })
      .catch(onError);
  useEffect(() => {
    load();
  }, [projectId]);
  if (!info) return null;
  const eff = info.effective;
  const from = (k: "setup" | "run" | "archive" | "copy") => {
    const app = k === "copy" ? (info.app.copy ?? []).length > 0 : !!info.app[k];
    return app ? "set in Freeloop" : ".freeloop/worktree.yaml";
  };
  const save = () =>
    EngineService.SetProjectScripts(projectId, {
      copy: form.copy
        .split(/[\n,]/)
        .map((s) => s.trim())
        .filter(Boolean),
      setup: form.setup,
      run: form.run,
      archive: form.archive,
    })
      .then(() => {
        setEditing(false);
        load();
      })
      .catch(onError);
  const empty = !(eff.copy ?? []).length && !eff.setup && !eff.run && !eff.archive;

  return (
    <div className="worktree-scripts">
      {info.repoError && <p className="banner warn">{info.repoError}</p>}
      {empty && !editing && (
        <p className="muted">
          No worktree scripts: the worktree starts with the committed files only.{" "}
          <button type="button" className="link" onClick={() => setEditing(true)}>
            Add setup…
          </button>
        </p>
      )}
      {!empty && !editing && (
        <ul className="worktree-scripts-list">
          {(eff.copy ?? []).length > 0 && (
            <li>
              <span className="muted">Copies</span> {(eff.copy ?? []).map((c) => <code key={c}>{c}</code>)} <span className="muted">· {from("copy")}</span>
            </li>
          )}
          {FIELDS.filter((f) => eff[f.key]).map((f) => (
            <li key={f.key}>
              <span className="muted">{f.label}</span> <code>{eff[f.key]}</code> <span className="muted">· {from(f.key)}</span>
            </li>
          ))}
          <li>
            <button type="button" className="link" onClick={() => setEditing(true)}>
              Edit…
            </button>
          </li>
        </ul>
      )}
      {editing && (
        <div className="worktree-scripts-form">
          <p className="muted">
            Values here override the project's <code>.freeloop/worktree.yaml</code>; leave one empty to use the file's. Commands run on this Mac in the
            worktree with the environment's variables and <code>FREELOOP_PORT</code>, <code>FREELOOP_ROOT_PATH</code> and <code>FREELOOP_WORKTREE</code>.
          </p>
          <label>
            Copy from the main checkout <span className="muted">(one path a line, such as .env)</span>
            <textarea rows={2} value={form.copy} placeholder={(info.repo.copy ?? []).join("\n")} onChange={(e) => setForm({ ...form, copy: e.target.value })} />
          </label>
          {FIELDS.map((f) => (
            <label key={f.key}>
              {f.label} <span className="muted">{f.hint}</span>
              <input value={form[f.key]} placeholder={info.repo[f.key] || ""} onChange={(e) => setForm({ ...form, [f.key]: e.target.value })} />
            </label>
          ))}
          <div className="row">
            <button type="button" className="primary" onClick={save}>
              Save
            </button>
            <button type="button" onClick={() => (setEditing(false), load())}>
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
