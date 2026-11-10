import { useState, type FormEvent } from "react";
import { secretStore } from "./platform.ts";
import { ask } from "./ask.ts";
import { t } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";
import { EngineService, modeName, STARTING_MODES, type EnvironmentInfo } from "./api.ts";

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
  // Whether commands may use the user's SSH agent (SSH_AUTH_SOCK).
  sshAgent: boolean;
  // The native sandbox (ENV-009).
  sandbox: boolean;
  sandboxNetwork: string;
  sandboxRead: string; // one path per line
  sandboxWrite: string;
  sandboxHosts: string; // one per line (ENV-010)
  // Another machine (ENV-008).
  host: string;
  remoteDir: string;
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
    sshAgent: e?.sshAgent ?? true,
    sandbox: e?.sandbox?.enabled ?? false,
    sandboxNetwork: e?.sandbox?.network || "localhost",
    sandboxRead: (e?.sandbox?.read ?? []).join("\n"),
    sandboxWrite: (e?.sandbox?.write ?? []).join("\n"),
    sandboxHosts: (e?.sandbox?.hosts ?? []).join("\n"),
    host: e?.host ?? "",
    remoteDir: e?.remoteDir ?? "",
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
    const entry = line.trim();
    if (!entry || entry.startsWith("#")) continue;
    const i = entry.indexOf("=");
    if (i <= 0) throw new Error(t("environments.notKeyValue", { line: entry }));
    out[entry.slice(0, i).trim()] = entry.slice(i + 1);
  }
  return out;
}

// ruleText writes a rule as the engine does: its effect, its tool and what it matches.
function ruleText(r: { effect: string; tool: string; command?: string; path?: string }): string {
  const match = r.command || r.path;
  return match ? `${r.effect} ${r.tool}(${match})` : `${r.effect} ${r.tool}`;
}

function sandboxText(network: string | undefined, hosts: number): string {
  if (network === "none") return t("environments.sandboxedNoNetwork");
  return network === "hosts" ? t("environments.sandboxedHosts", { count: hosts }) : t("environments.sandboxed");
}

// summary is an environment's one-line description, its parts joined by " · ".
function summary(env: EnvironmentInfo): string {
  const secrets = env.secretNames ?? [];
  const hosts = (env.sandbox?.hosts ?? []).length;
  const disabled = env.disabledTools ?? [];
  const rules = env.rules ?? [];
  const sandboxed = env.sandbox?.enabled && sandboxText(env.sandbox.network, hosts);
  return [
    t("environments.variableCount", { count: Object.keys(env.env ?? {}).length }),
    secrets.length > 0 ? t("environments.secretCountNamed", { count: secrets.length, names: secrets.join(", ") }) : t("environments.secretCount", { count: 0 }),
    sandboxed,
    env.kind === "docker" && (env.devcontainer ? t("environments.dockerDevcontainer") : t("environments.docker", { image: env.image })),
    env.kind === "ssh" && t("environments.ssh", { host: env.host }),
    disabled.length > 0 && t("environments.toolsOff", { tools: disabled.join(", ") }),
    rules.length > 0 &&
      t("environments.rules", {
        rules: rules.map(ruleText).join(", "),
        file: `environments/${env.id}.yaml`,
      }),
    env.defaultMode && t("environments.startsIn", { mode: modeName(env.defaultMode) }),
  ]
    .filter(Boolean)
    .join(" · ");
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
      host: form.host,
      remoteDir: form.remoteDir,
      sshAgent: form.kind === "local" && !form.sshAgent ? false : null,
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
    if (!(await ask(t(env.builtin ? "environments.resetAsk" : "environments.deleteAsk", { name: env.name, store: secretStore() })))) return;
    EngineService.DeleteEnvironment(env.id).then(onChanged).catch(onError);
  };

  const toggle = (list: string[], v: string) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v]);

  return (
    <section className="page">
      <h2>{t("settings.page.environments")}</h2>
      <p className="muted">
        {t("environments.intro", { store: secretStore() })}
      </p>
      <ul className="provider-list">
        {environments.map((env) => (
          <li key={env.id} className="card">
            <div>
              <strong>{env.name}</strong> <span className="muted">{env.id === "local" ? t("environments.default") : env.builtin ? t("environments.builtIn") : env.id}</span>
              <div className="muted">
                {summary(env)}
              </div>
            </div>
            <div className="row">
              <button onClick={() => setForm(toForm(env))}>{t("environments.edit")}</button>
              <button className="danger" onClick={() => remove(env)}>
                {env.builtin ? t("common.reset") : t("environments.delete")}
              </button>
            </div>
          </li>
        ))}
      </ul>
      {!form && (
        <button className="primary" onClick={() => setForm(toForm())}>
          {t("environments.add")}
        </button>
      )}
      {form && (
        <form className="card form" onSubmit={save}>
          <h3>{form.id ? t("environments.editTitle", { name: form.name }) : t("environments.add")}</h3>
          <label>
            {t("environments.name")}
            <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          </label>
          <label>
            {t("environments.variables")} <span className="muted">{t("environments.variablesHint")}</span>
            <textarea rows={4} value={form.vars} onChange={(e) => setForm({ ...form, vars: e.target.value })} placeholder={"NODE_ENV=development"} />
          </label>
          <fieldset>
            <legend>{t("environments.secrets")}</legend>
            {form.secretNames.map((n) => (
              <label key={n} className="row">
                <code>{n}</code>
                <span className="muted">{t("environments.stored")}</span>
                <input
                  type="checkbox"
                  checked={form.remove.includes(n)}
                  onChange={() => setForm({ ...form, remove: toggle(form.remove, n) })}
                />{" "}
                {t("environments.removeSecret")}
              </label>
            ))}
            {form.added.map((s, i) => (
              <div key={i} className="row">
                <input
                  placeholder={t("environments.secretName")}
                  value={s.name}
                  onChange={(e) => setForm({ ...form, added: form.added.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)) })}
                />
                <input
                  type="password"
                  autoComplete="off"
                  placeholder={t("environments.secretValue")}
                  value={s.value}
                  onChange={(e) => setForm({ ...form, added: form.added.map((x, j) => (j === i ? { ...x, value: e.target.value } : x)) })}
                />
              </div>
            ))}
            <button type="button" onClick={() => setForm({ ...form, added: [...form.added, { name: "", value: "" }] })}>
              {t("environments.addSecret")}
            </button>
          </fieldset>
          <fieldset>
            <legend>{t("environments.tools")}</legend>
            <div className="row wrap">
              {TOOLS.map((tool) => (
                <label key={tool} className="row">
                  <input type="checkbox" checked={!form.disabled.includes(tool)} onChange={() => setForm({ ...form, disabled: toggle(form.disabled, tool) })} />
                  <code>{tool}</code>
                </label>
              ))}
            </div>
          </fieldset>
          <label>
            {t("environments.runs")}
            <select value={form.kind} onChange={(e) => setForm({ ...form, kind: e.target.value })}>
              <option value="local">{t("environments.kind.local")}</option>
              <option value="docker">{t("environments.kind.docker")}</option>
              <option value="ssh">{t("environments.kind.ssh")}</option>
            </select>
          </label>
          {form.kind === "local" && (
            <fieldset>
              <legend>{t("environments.sshKeys")}</legend>
              <label className="row">
                <input type="checkbox" checked={form.sshAgent} onChange={(e) => setForm({ ...form, sshAgent: e.target.checked })} /> {t("environments.sshAgent")}
              </label>
              <p className="muted">
                {tNodes("environments.sshAgentNote", { command: <code>{"git push"}</code> })}
              </p>
            </fieldset>
          )}
          {form.kind === "local" && (
            <fieldset>
              <legend>{t("environments.sandbox")}</legend>
              <label className="row">
                <input type="checkbox" checked={form.sandbox} onChange={(e) => setForm({ ...form, sandbox: e.target.checked })} /> {t("environments.sandboxOn")}
              </label>
              <p className="muted">
                {t("environments.sandboxNote")}
              </p>
              {form.sandbox && (
                <>
                  <label>
                    {t("environments.network")}
                    <select value={form.sandboxNetwork} onChange={(e) => setForm({ ...form, sandboxNetwork: e.target.value })}>
                      <option value="localhost">{t("environments.network.localhost")}</option>
                      <option value="hosts">{t("environments.network.hosts")}</option>
                      <option value="none">{t("environments.network.none")}</option>
                    </select>
                  </label>
                  {form.sandboxNetwork === "hosts" && (
                    <label>
                      {t("environments.hosts")} <span className="muted">{t("environments.hostsHint")}</span>
                      <textarea rows={4} value={form.sandboxHosts} onChange={(e) => setForm({ ...form, sandboxHosts: e.target.value })} required />
                      <span className="muted">
                        {t("environments.hostsNote")}{" "}
                        <button type="button" className="link" onClick={() => setForm({ ...form, sandboxHosts: mergeLines(form.sandboxHosts, PACKAGE_HOSTS) })}>
                          {t("environments.addRegistries")}
                        </button>
                      </span>
                    </label>
                  )}
                  <label>
                    {t("environments.readFolders")} <span className="muted">{t("environments.readFoldersHint")}</span>
                    <textarea rows={2} value={form.sandboxRead} onChange={(e) => setForm({ ...form, sandboxRead: e.target.value })} placeholder={"~/tools"} />
                  </label>
                  <label>
                    {t("environments.writeFolders")} <span className="muted">{t("environments.writeFoldersHint")}</span>
                    <textarea rows={2} value={form.sandboxWrite} onChange={(e) => setForm({ ...form, sandboxWrite: e.target.value })} />
                  </label>
                </>
              )}
            </fieldset>
          )}
          {form.kind === "docker" && (
            <fieldset>
              <legend>{t("environments.container")}</legend>
              <p className="muted">
                {t("environments.containerNote")}
              </p>
              <label className="row">
                <input type="checkbox" checked={form.devcontainer} onChange={(e) => setForm({ ...form, devcontainer: e.target.checked })} /> {t("environments.devcontainer")}
              </label>
              {!form.devcontainer && (
                <label>
                  {t("environments.image")}
                  <input value={form.image} onChange={(e) => setForm({ ...form, image: e.target.value })} placeholder={"node:22-bookworm"} required />
                </label>
              )}
              <label>
                {t("environments.setup")} <span className="muted">{t(form.devcontainer ? "environments.setupHintDevcontainer" : "environments.setupHintDocker")}</span>
                <input value={form.setup} onChange={(e) => setForm({ ...form, setup: e.target.value })} placeholder={"npm ci"} />
              </label>
              <label>
                {t("environments.health")} <span className="muted">{t("environments.healthHint")}</span>
                <input value={form.health} onChange={(e) => setForm({ ...form, health: e.target.value })} placeholder={"node --version"} />
              </label>
            </fieldset>
          )}
          {form.kind === "ssh" && (
            <fieldset>
              <legend>{t("environments.machine")}</legend>
              <p className="muted">
                {t("environments.machineNote")}
              </p>
              <p className="muted">
                {tNodes("environments.machineCopies", { git: <code>{".git"}</code>, claude: <code>{".claude"}</code>, agents: <code>{"AGENTS.md"}</code> })}
              </p>
              <label>
                {t("environments.host")} <span className="muted">{t("environments.hostHint")}</span>
                <input value={form.host} required onChange={(e) => setForm({ ...form, host: e.target.value })} placeholder={"my-box"} autoCapitalize="off" spellCheck={false} />
              </label>
              <p className="muted">
                {tNodes("environments.hostNote", { command: <code>ssh {form.host.trim() || "my-box"}</code> })}
              </p>
              <label>
                {t("environments.remoteDir")} <span className="muted">{t("environments.remoteDirHint")}</span>
                <input value={form.remoteDir} onChange={(e) => setForm({ ...form, remoteDir: e.target.value })} placeholder={"~/.freeloop-sessions"} autoCapitalize="off" spellCheck={false} />
              </label>
              <label>
                {t("environments.setup")} <span className="muted">{t("environments.setupHintSsh")}</span>
                <input value={form.setup} onChange={(e) => setForm({ ...form, setup: e.target.value })} placeholder={"npm ci"} />
              </label>
              <label>
                {t("environments.health")} <span className="muted">{t("environments.healthHint")}</span>
                <input value={form.health} onChange={(e) => setForm({ ...form, health: e.target.value })} placeholder={"node --version"} />
              </label>
            </fieldset>
          )}
          <label>
            {t("environments.defaultMode")}
            <select value={form.defaultMode} onChange={(e) => setForm({ ...form, defaultMode: e.target.value })}>
              <option value="">{t("environments.defaultModeAsk", { mode: modeName("ask") })}</option>
              {STARTING_MODES.filter((m) => m.id !== "ask").map((m) => (
                <option key={m.id} value={m.id}>
                  {modeName(m.id)}
                </option>
              ))}
            </select>
          </label>
          <div className="row">
            <button type="submit" className="primary" disabled={busy}>
              {t("common.save")}
            </button>
            <button type="button" onClick={() => setForm(null)}>
              {t("common.cancel")}
            </button>
          </div>
        </form>
      )}
    </section>
  );
}
