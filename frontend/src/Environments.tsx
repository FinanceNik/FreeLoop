import { useState, type FormEvent } from "react";
import { ask } from "./ask.ts";
import { EngineService, MODES, STARTING_MODES, type EnvironmentInfo } from "./api.ts";

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
  // The native sandbox (ENV-009).
  sandbox: boolean;
  sandboxNetwork: string;
  sandboxRead: string; // one path per line
  sandboxWrite: string;
  sandboxHosts: string; // one per line (ENV-010)
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
    sandbox: e?.sandbox?.enabled ?? false,
    sandboxNetwork: e?.sandbox?.network || "localhost",
    sandboxRead: (e?.sandbox?.read ?? []).join("\n"),
    sandboxWrite: (e?.sandbox?.write ?? []).join("\n"),
    sandboxHosts: (e?.sandbox?.hosts ?? []).join("\n"),
  };
}

// The registries npm, pip, Go and cargo fetch from, and GitHub for what they
// fetch there. Offered, never assumed: the list is the user's.
const PACKAGE_HOSTS = [
  "registry.npmjs.org",
  "registry.yarnpkg.com",
  "pypi.org",
  "files.pythonhosted.org",
  "proxy.golang.org",
  "sum.golang.org",
  "index.crates.io",
  "static.crates.io",
  "github.com",
  "codeload.github.com",
  "objects.githubusercontent.com",
];

function mergeLines(text: string, more: string[]): string {
  return [...new Set([...lines(text), ...more])].join("\n");
}

function lines(text: string): string[] {
  return text
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
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
      sandbox: form.kind === "local" && form.sandbox ? { enabled: true, network: form.sandboxNetwork, read: lines(form.sandboxRead), write: lines(form.sandboxWrite), hosts: form.sandboxNetwork === "hosts" ? lines(form.sandboxHosts) : [] } : null,
    })
      .then(() => {
        setForm(null);
        onChanged();
      })
      .catch(onError)
      .finally(() => setBusy(false));
  };

  const remove = async (env: EnvironmentInfo) => {
    const what = env.builtin ? `Reset ${env.name}` : `Delete ${env.name}`;
    if (!(await ask(`${what}? Its secrets are removed from the Keychain.`))) return;
    EngineService.DeleteEnvironment(env.id).then(onChanged).catch(onError);
  };

  const toggle = (list: string[], v: string) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v]);

  return (
    <section className="page">
      <h2>Environments</h2>
      <p className="muted">
        An environment is what a session's commands run with: variables, secrets, the tools the agent may use and the mode new
        sessions start in. Commands run on this Mac in the session's project folder, in a sandbox if the environment has one. Secrets are stored in the macOS Keychain,
        handed to commands as variables when they start, and redacted from transcripts.
      </p>
      <ul className="provider-list">
        {environments.map((env) => (
          <li key={env.id} className="card">
            <div>
              <strong>{env.name}</strong> <span className="muted">{env.id === "local" ? "default" : env.builtin ? "built in" : env.id}</span>
              <div className="muted">
                {Object.keys(env.env ?? {}).length} variables · {(env.secretNames ?? []).length} secrets
                {(env.secretNames ?? []).length > 0 && ` (${(env.secretNames ?? []).join(", ")})`}
                {env.sandbox?.enabled && ` · sandboxed${env.sandbox.network === "none" ? ", no network" : env.sandbox.network === "hosts" ? `, ${(env.sandbox.hosts ?? []).length} hosts` : ""}`}
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
                {env.builtin ? "Reset" : "Delete"}
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
          {form.kind === "local" && (
            <fieldset>
              <legend>Sandbox</legend>
              <label className="row">
                <input type="checkbox" checked={form.sandbox} onChange={(e) => setForm({ ...form, sandbox: e.target.checked })} /> Run commands in a sandbox
              </label>
              <p className="muted">
                The built-in agent's commands then run without asking, because they can only write in the project and read little outside it. Leaving the
                sandbox always asks. An external agent's own tools are not covered; for those, use a Docker environment.
              </p>
              {form.sandbox && (
                <>
                  <label>
                    Network
                    <select value={form.sandboxNetwork} onChange={(e) => setForm({ ...form, sandboxNetwork: e.target.value })}>
                      <option value="localhost">This Mac only: tests and dev servers work, and so does anything else listening here</option>
                      <option value="hosts">This Mac and the hosts listed here</option>
                      <option value="none">None</option>
                    </select>
                  </label>
                  {form.sandboxNetwork === "hosts" && (
                    <label>
                      Hosts <span className="muted">(one per line, such as registry.npmjs.org or *.github.com; add :port for a port other than 80 and 443)</span>
                      <textarea rows={4} value={form.sandboxHosts} onChange={(e) => setForm({ ...form, sandboxHosts: e.target.value })} required />
                      <span className="muted">
                        A tool has to use the proxy the sandbox sets up, as npm, pip, Go, git over https and curl do. Anything else gets no connection.{" "}
                        <button type="button" className="link" onClick={() => setForm({ ...form, sandboxHosts: mergeLines(form.sandboxHosts, PACKAGE_HOSTS) })}>
                          Add the usual package registries
                        </button>
                      </span>
                    </label>
                  )}
                  <label>
                    More folders it can read <span className="muted">(one per line, absolute or under ~/)</span>
                    <textarea rows={2} value={form.sandboxRead} onChange={(e) => setForm({ ...form, sandboxRead: e.target.value })} placeholder="~/tools" />
                  </label>
                  <label>
                    More folders it can write <span className="muted">(one per line)</span>
                    <textarea rows={2} value={form.sandboxWrite} onChange={(e) => setForm({ ...form, sandboxWrite: e.target.value })} />
                  </label>
                </>
              )}
            </fieldset>
          )}
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
              {STARTING_MODES.filter((m) => m.id !== "ask").map((m) => (
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
