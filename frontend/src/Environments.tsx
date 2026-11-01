import { useState, type FormEvent } from "react";
import { EngineService, MODES, type EnvironmentInfo } from "./api.ts";

interface Props {
  environments: EnvironmentInfo[];
  onChanged: () => void;
  onError: (err: unknown) => void;
}

const TOOLS = ["read_file", "list_dir", "glob", "grep", "write_file", "edit_file", "shell", "web_fetch", "web_search"];

interface Form {
  id: string;
  name: string;
  vars: string; // KEY=value per line
  secretNames: string[]; // already stored
  remove: string[];
  added: { name: string; value: string }[];
  disabled: string[];
  defaultMode: string;
  // Docker (ENV-005).
  kind: string;
  image: string;
  devcontainer: boolean;
  setup: string;
  health: string;
}

function toForm(e?: EnvironmentInfo): Form {
  return {
    id: e?.id ?? "",
    name: e?.name ?? "",
    vars: Object.entries(e?.env ?? {})
      .map(([k, v]) => `${k}=${v ?? ""}`)
      .join("\n"),
    secretNames: e?.secretNames ?? [],
    remove: [],
    added: [],
    disabled: e?.disabledTools ?? [],
    defaultMode: e?.defaultMode ?? "",
    kind: e?.kind ?? "local",
    image: e?.image ?? "",
    devcontainer: e?.devcontainer ?? false,
    setup: e?.setup ?? "",
    health: e?.health ?? "",
  };
}

function parseVars(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const t = line.trim();
    if (!t || t.startsWith("#")) continue;
    const i = t.indexOf("=");
    if (i <= 0) throw new Error(`"${t}" is not KEY=value`);
    out[t.slice(0, i).trim()] = t.slice(i + 1);
  }
  return out;
}

export default function Environments({ environments, onChanged, onError }: Props) {
  const [form, setForm] = useState<Form | null>(null);
  const [busy, setBusy] = useState(false);

  const save = (e: FormEvent) => {
    e.preventDefault();
    if (!form) return;
    let env: Record<string, string>;
    try {
      env = parseVars(form.vars);
    } catch (err) {
      onError(err);
      return;
    }
    const secrets: Record<string, string> = {};
    for (const s of form.added) if (s.name.trim()) secrets[s.name.trim()] = s.value;
    setBusy(true);
    EngineService.SaveEnvironment({
      id: form.id,
      name: form.name,
      env,
      secrets,
      removeSecrets: form.remove,
      disabledTools: form.disabled,
      defaultMode: form.defaultMode,
      kind: form.kind,
      image: form.image,
      devcontainer: form.devcontainer,
      setup: form.setup,
      health: form.health,
    })
      .then(() => {
        setForm(null);
        onChanged();
      })
      .catch(onError)
      .finally(() => setBusy(false));
  };

  const remove = (env: EnvironmentInfo) => {
    const what = env.id === "local" ? "Reset the default environment" : `Delete ${env.name}`;
    if (!confirm(`${what}? Its secrets are removed from the Keychain.`)) return;
    EngineService.DeleteEnvironment(env.id).then(onChanged).catch(onError);
  };

  const toggle = (list: string[], v: string) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v]);

  return (
    <section className="page">
      <h2>Environments</h2>
      <p className="muted">
        An environment is what a session's commands run with: variables, secrets, the tools the agent may use and the mode new
        sessions start in. Commands run on this Mac in the session's project folder. Secrets are stored in the macOS Keychain,
        handed to commands as variables when they start, and redacted from transcripts.
      </p>
      <ul className="provider-list">
        {environments.map((env) => (
          <li key={env.id} className="card">
            <div>
              <strong>{env.name}</strong> <span className="muted">{env.id === "local" ? "default" : env.id}</span>
              <div className="muted">
                {Object.keys(env.env ?? {}).length} variables · {(env.secretNames ?? []).length} secrets
                {(env.secretNames ?? []).length > 0 && ` (${(env.secretNames ?? []).join(", ")})`}
                {env.kind === "docker" && ` · Docker: ${env.devcontainer ? "the project's devcontainer.json" : env.image}`}
                {(env.disabledTools ?? []).length > 0 && ` · tools off: ${(env.disabledTools ?? []).join(", ")}`}
                {(env.rules ?? []).length > 0 &&
                  ` · rules: ${(env.rules ?? []).map((r) => `${r.effect} ${r.tool}${r.command ? `(${r.command})` : r.path ? `(${r.path})` : ""}`).join(", ")} (edit environments/${env.id}.yaml)`}
                {env.defaultMode && ` · starts in ${MODES.find((m) => m.id === env.defaultMode)?.name ?? env.defaultMode}`}
              </div>
            </div>
            <div className="row">
              <button onClick={() => setForm(toForm(env))}>Edit</button>
              <button className="danger" onClick={() => remove(env)}>
                {env.id === "local" ? "Reset" : "Delete"}
              </button>
            </div>
          </li>
        ))}
      </ul>
      {!form && (
        <button className="primary" onClick={() => setForm(toForm())}>
          Add environment
        </button>
      )}
      {form && (
        <form className="card form" onSubmit={save}>
          <h3>{form.id ? `Edit ${form.name}` : "Add environment"}</h3>
          <label>
            Name
            <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          </label>
          <label>
            Variables <span className="muted">(one KEY=value per line)</span>
            <textarea rows={4} value={form.vars} onChange={(e) => setForm({ ...form, vars: e.target.value })} placeholder="NODE_ENV=development" />
          </label>
          <fieldset>
            <legend>Secrets</legend>
            {form.secretNames.map((n) => (
              <label key={n} className="row">
                <code>{n}</code>
                <span className="muted">stored</span>
                <input
                  type="checkbox"
                  checked={form.remove.includes(n)}
                  onChange={() => setForm({ ...form, remove: toggle(form.remove, n) })}
                />{" "}
                remove
              </label>
            ))}
            {form.added.map((s, i) => (
              <div key={i} className="row">
                <input
                  placeholder="NAME"
                  value={s.name}
                  onChange={(e) => setForm({ ...form, added: form.added.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)) })}
                />
                <input
                  type="password"
                  autoComplete="off"
                  placeholder="value"
                  value={s.value}
                  onChange={(e) => setForm({ ...form, added: form.added.map((x, j) => (j === i ? { ...x, value: e.target.value } : x)) })}
                />
              </div>
            ))}
            <button type="button" onClick={() => setForm({ ...form, added: [...form.added, { name: "", value: "" }] })}>
              Add secret
            </button>
          </fieldset>
          <fieldset>
            <legend>Tools the agent may use</legend>
            <div className="row wrap">
              {TOOLS.map((t) => (
                <label key={t} className="row">
                  <input type="checkbox" checked={!form.disabled.includes(t)} onChange={() => setForm({ ...form, disabled: toggle(form.disabled, t) })} />
                  <code>{t}</code>
                </label>
              ))}
            </div>
          </fieldset>
          <label>
            Runs
            <select value={form.kind} onChange={(e) => setForm({ ...form, kind: e.target.value })}>
              <option value="local">On this Mac, in the project folder</option>
              <option value="docker">In a Docker container per session</option>
            </select>
          </label>
          {form.kind === "docker" && (
            <fieldset>
              <legend>Container</legend>
              <p className="muted">
                Each session gets its own container with its folder shared at the same path. Commands and external agents run inside it, so they see the
                project and the image, not the rest of your files. Needs Docker Desktop, or Colima and the docker CLI.
              </p>
              <label className="row">
                <input type="checkbox" checked={form.devcontainer} onChange={(e) => setForm({ ...form, devcontainer: e.target.checked })} /> Use the
                project's .devcontainer/devcontainer.json
              </label>
              {!form.devcontainer && (
                <label>
                  Image
                  <input value={form.image} onChange={(e) => setForm({ ...form, image: e.target.value })} placeholder="node:22-bookworm" required />
                </label>
              )}
              <label>
                Setup command <span className="muted">(once, after the container is created{form.devcontainer ? "; defaults to postCreateCommand" : ""})</span>
                <input value={form.setup} onChange={(e) => setForm({ ...form, setup: e.target.value })} placeholder="npm ci" />
              </label>
              <label>
                Health check <span className="muted">(before every session start; a failure blocks the session)</span>
                <input value={form.health} onChange={(e) => setForm({ ...form, health: e.target.value })} placeholder="node --version" />
              </label>
            </fieldset>
          )}
          <label>
            New sessions start in
            <select value={form.defaultMode} onChange={(e) => setForm({ ...form, defaultMode: e.target.value })}>
              <option value="">Ask (default)</option>
              {MODES.filter((m) => m.id !== "bypass" && m.id !== "ask").map((m) => (
                <option key={m.id} value={m.id}>
                  {m.name}
                </option>
              ))}
            </select>
          </label>
          <div className="row">
            <button type="submit" className="primary" disabled={busy}>
              Save
            </button>
            <button type="button" onClick={() => setForm(null)}>
              Cancel
            </button>
          </div>
        </form>
      )}
    </section>
  );
}
