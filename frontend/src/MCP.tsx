import { useEffect, useState, type FormEvent } from "react";
import { secretStore } from "./platform.ts";
import { ask } from "./ask.ts";
import { Browser } from "@wailsio/runtime";
import { EngineService, type MCPServer, type MCPServerInfo, type RegistryOption, type RegistryServer } from "./api.ts";
import McpRegistry from "./McpRegistry.tsx";
import { added as alreadyAdded, AUTH_HEADER, MCP_SUGGESTIONS, type McpSuggestion } from "./mcpSuggestions.ts";
import { list, t, type PlainKey } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";

// MCP servers (MCP-001, MCP-005): stdio commands and Streamable HTTP
// endpoints in mcp.yaml. A server runs only after the user approves exactly
// what it runs; any change asks again. Secrets go to the system's secret store. A remote
// server that wants OAuth waits for "Sign in", which opens the server's
// sign-in page in the browser (MCP-006).

interface Props {
  configVersion: number;
  onImport: () => void;
  onError: (err: unknown) => void;
}

interface Form {
  isNew: boolean;
  id: string;
  name: string;
  transport: string;
  command: string;
  args: string; // one per line
  env: string; // KEY=value per line
  url: string;
  oauthClientId: string;
  headers: string; // Name: value per line
  enabled: boolean;
  secretNames: string[];
  remove: string[];
  // hint is a catalogue key, translated where the field shows.
  added: { name: string; value: string; hint?: PlainKey }[];
}

function toForm(s?: MCPServerInfo): Form {
  return {
    isNew: !s,
    id: s?.id ?? "",
    name: s?.name ?? "",
    transport: s?.transport ?? "stdio",
    command: s?.command ?? "",
    args: (s?.args ?? []).join("\n"),
    env: Object.entries(s?.env ?? {})
      .map(([k, v]) => `${k}=${v}`)
      .join("\n"),
    url: s?.url ?? "",
    oauthClientId: s?.oauthClientId ?? "",
    headers: Object.entries(s?.headers ?? {})
      .map(([k, v]) => `${k}: ${v}`)
      .join("\n"),
    enabled: s?.enabled ?? true,
    secretNames: s?.secretNames ?? [],
    remove: [],
    added: [],
  };
}

// registryForm fills in the add-server form from a registry's listing. Values it
// marks secret become secrets to fill in, which go to the system's secret store.
export function registryForm(server: RegistryServer, option: RegistryOption): Form {
  const plain = (option.transport === "http" ? (option.headers ?? []) : (option.variables ?? [])).filter((v) => !v.secret && (v.required || v.default));
  const secret = (option.transport === "http" ? (option.headers ?? []) : (option.variables ?? [])).filter((v) => v.secret);
  const sep = option.transport === "http" ? ": " : "=";
  return {
    ...toForm(),
    id: registryId(server.name),
    name: server.title || server.name,
    transport: option.transport,
    command: option.command,
    args: (option.args ?? []).join("\n"),
    url: option.url,
    env: option.transport === "http" ? "" : plain.map((v) => `${v.name}${sep}${v.default}`).join("\n"),
    headers: option.transport === "http" ? plain.map((v) => `${v.name}${sep}${v.default}`).join("\n") : "",
    added: secret.map((v) => ({ name: v.name, value: "" })),
  };
}

// suggestionForm fills in the add-server form from a suggested server (MCP-010): the
// user sees exactly what it runs or reaches, and saves it as any other.
export function suggestionForm(s: McpSuggestion): Form {
  return {
    ...toForm(),
    id: s.id,
    name: s.name,
    transport: s.transport,
    command: s.command ?? "",
    args: (s.args ?? []).join("\n"),
    url: s.url ?? "",
    added: s.secretHeader ? [{ name: s.secretHeader.name, value: "", hint: s.secretHeader.hint }] : [],
  };
}

// registryId makes an ID from a registry name such as io.github.acme/files.
export function registryId(name: string): string {
  const last = name.slice(name.lastIndexOf("/") + 1);
  return (
    last
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "") || "server"
  );
}

function pairs(text: string, sep: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const i = trimmed.indexOf(sep);
    if (i <= 0) throw new Error(t(sep === "=" ? "mcp.notPair.env" : "mcp.notPair.header", { line: trimmed }));
    out[trimmed.slice(0, i).trim()] = trimmed.slice(i + sep.length).trim();
  }
  return out;
}

// commandLine shows what a stdio server runs, quoting arguments with spaces.
function commandLine(s: MCPServer): string {
  return [s.command, ...(s.args ?? [])].map((a) => (/\s/.test(a) ? JSON.stringify(a) : a)).join(" ");
}

// The states a server reports, as their words; others (sign_in has its own) show as given.
const STATE: Record<string, PlainKey> = {
  stopped: "mcp.state.stopped",
  starting: "mcp.state.starting",
  running: "mcp.state.running",
  error: "mcp.state.error",
};

export default function MCP({ configVersion, onImport, onError }: Props) {
  const [servers, setServers] = useState<MCPServerInfo[]>([]);
  const [form, setForm] = useState<Form | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [browsing, setBrowsing] = useState(false);
  const load = () =>
    EngineService.ListMCPServers()
      .then((l) => setServers(l ?? []))
      .catch(onError);
  useEffect(() => {
    load();
  }, [configVersion]);
  // Starting servers change state in the background.
  useEffect(() => {
    if (!servers.some((s) => s.status.state === "starting")) return;
    const timer = setTimeout(load, 1000);
    return () => clearTimeout(timer);
  }, [servers]);

  const run = (id: string, p: Promise<unknown>) => {
    setBusy(id);
    p.catch(onError).finally(() => {
      setBusy(null);
      load();
    });
  };

  const save = (e: FormEvent) => {
    e.preventDefault();
    if (!form) return;
    let env: Record<string, string>, headers: Record<string, string>;
    try {
      env = form.transport === "stdio" ? pairs(form.env, "=") : {};
      headers = form.transport === "http" ? pairs(form.headers, ":") : {};
    } catch (err) {
      onError(err);
      return;
    }
    const secrets: Record<string, string> = {};
    for (const s of form.added) if (s.name.trim()) secrets[s.name.trim()] = s.value;
    const server: MCPServer = {
      id: form.id.trim(),
      name: form.name,
      transport: form.transport,
      command: form.transport === "stdio" ? form.command.trim() : "",
      args: form.transport === "stdio" ? form.args.split("\n").filter((a) => a.trim() !== "") : [],
      env,
      url: form.transport === "http" ? form.url.trim() : "",
      oauthClientId: form.transport === "http" ? form.oauthClientId.trim() : "",
      headers,
      enabled: form.enabled,
      source: "",
    };
    EngineService.SaveMCPServer({ server, secrets, removeSecrets: form.remove })
      .then(() => {
        setForm(null);
        load();
      })
      .catch(onError);
  };

  if (browsing && !form) {
    return (
      <McpRegistry
        onClose={() => setBrowsing(false)}
        onError={onError}
        onPick={(server, option) => {
          setBrowsing(false);
          setForm(registryForm(server, option));
        }}
      />
    );
  }

  if (form) {
    const set = <K extends keyof Form>(k: K, v: Form[K]) => setForm({ ...form, [k]: v });
    return (
      <section className="page">
        <h2>{form.isNew ? t("mcp.form.addTitle") : t("mcp.form.editTitle", { name: form.name || form.id })}</h2>
        <form className="card form" onSubmit={save}>
          {form.isNew && (
            <label>
              {t("mcp.form.id")} <span className="muted">{t("mcp.form.idHint")}</span>
              <input value={form.id} onChange={(e) => set("id", e.target.value)} placeholder="github" required pattern="[a-z0-9]+(-[a-z0-9]+)*" />
            </label>
          )}
          <label>
            {t("mcp.form.name")}
            <input value={form.name} onChange={(e) => set("name", e.target.value)} />
          </label>
          <label>
            {t("mcp.form.transport")}
            <select value={form.transport} onChange={(e) => set("transport", e.target.value)}>
              <option value="stdio">{t("mcp.form.stdio")}</option>
              <option value="http">{t("mcp.form.http")}</option>
            </select>
          </label>
          {form.transport === "stdio" ? (
            <>
              <label>
                {t("mcp.form.command")}
                <input value={form.command} onChange={(e) => set("command", e.target.value)} placeholder="npx" required />
              </label>
              <label>
                {t("mcp.form.args")} <span className="muted">{t("mcp.form.argsHint")}</span>
                <textarea rows={3} value={form.args} onChange={(e) => set("args", e.target.value)} placeholder={"-y\n@modelcontextprotocol/server-github@1.0.0"} />
              </label>
              <label>
                {t("mcp.form.env")} <span className="muted">{t("mcp.form.envHint")}</span>
                <textarea rows={2} value={form.env} onChange={(e) => set("env", e.target.value)} />
              </label>
            </>
          ) : (
            <>
              <label>
                {t("mcp.form.url")}
                <input value={form.url} onChange={(e) => set("url", e.target.value)} placeholder="https://example.com/mcp" required />
              </label>
              <label>
                {t("mcp.form.oauthClientId")} <span className="muted">{t("mcp.form.oauthClientIdHint")}</span>
                <input value={form.oauthClientId} onChange={(e) => set("oauthClientId", e.target.value)} />
              </label>
              <label>
                {t("mcp.form.headers")} <span className="muted">{t("mcp.form.headersHint")}</span>
                <textarea rows={2} value={form.headers} onChange={(e) => set("headers", e.target.value)} />
              </label>
            </>
          )}
          <fieldset>
            <legend>{form.transport === "http" ? t("mcp.form.secretsHeaders") : t("mcp.form.secretsVariables")}</legend>
            {form.secretNames
              .filter((n) => !form.remove.includes(n))
              .map((n) => (
                <div key={n} className="row">
                  <code>{n}</code> <span className="muted">{t("mcp.form.storedIn", { store: secretStore() })}</span>
                  <button type="button" onClick={() => set("remove", [...form.remove, n])}>
                    {t("common.remove")}
                  </button>
                </div>
              ))}
            {form.added.map((s, i) => (
              <div key={i} className="row">
                <input
                  placeholder={form.transport === "http" ? AUTH_HEADER : "GITHUB_TOKEN"}
                  value={s.name}
                  onChange={(e) => set("added", form.added.map((x, n) => (n === i ? { ...x, name: e.target.value } : x)))}
                  aria-label={t("mcp.form.secretName")}
                />
                <input
                  type="password"
                  placeholder={s.hint ? t(s.hint) : t("mcp.form.valuePlaceholder")}
                  value={s.value}
                  onChange={(e) => set("added", form.added.map((x, n) => (n === i ? { ...x, value: e.target.value } : x)))}
                  aria-label={t("mcp.form.secretValue")}
                />
              </div>
            ))}
            <div>
              <button type="button" onClick={() => set("added", [...form.added, { name: "", value: "" }])}>
                {t("mcp.form.addSecret")}
              </button>
            </div>
          </fieldset>
          <label className="row">
            <input type="checkbox" checked={form.enabled} onChange={(e) => set("enabled", e.target.checked)} /> {t("mcp.form.enabled")}
            <span className="muted">{t("mcp.form.enabledHint")}</span>
          </label>
          <div className="row">
            <button type="submit" className="primary">
              {t("common.save")}
            </button>
            <button type="button" onClick={() => setForm(null)}>
              {t("common.cancel")}
            </button>
          </div>
        </form>
      </section>
    );
  }

  return (
    <section className="page">
      <h2>{t("settings.page.mcp")}</h2>
      <p className="muted">{tNodes("mcp.intro", { pattern: <code>mcp__server__tool</code>, example: <code>mcp__github__*</code> })}</p>
      {servers.length === 0 && <p className="muted">{t("mcp.none")}</p>}
      <ul className="provider-list">
        {servers.map((s) => (
          <li key={s.id} className="card mcp-server">
            <div className="mcp-main">
              <div>
                <strong>{s.name || s.id}</strong> <span className="muted">{s.id}</span>{" "}
                <span className={`badge mcp-${s.needsApproval ? "approval" : s.status.state}`}>
                  {s.needsApproval ? t("mcp.badge.approval") : s.status.state === "sign_in" ? t("mcp.badge.signIn") : STATE[s.status.state] ? t(STATE[s.status.state]) : s.status.state}
                </span>
                {s.signedIn && <span className="muted"> · {t("mcp.signedIn")}</span>}
                {!s.enabled && <span className="muted"> · {t("mcp.offByDefault")}</span>}
              </div>
              <code className="mcp-cmd">{s.transport === "http" ? s.url : commandLine(s)}</code>
              {s.source && <div className="muted">{t("mcp.importedFrom", { source: s.source })}</div>}
              {s.status.state === "running" && (
                <details>
                  <summary className="muted">
                    {t("mcp.tools", { count: (s.status.tools ?? []).length, protocol: s.status.protocol })}
                  </summary>
                  <ul>
                    {(s.status.tools ?? []).map((tool) => (
                      <li key={tool.name}>
                        <code>{tool.name}</code> <span className="muted">{tool.description}</span>
                      </li>
                    ))}
                  </ul>
                </details>
              )}
              {s.status.error && s.status.state !== "sign_in" && <div className="error-text">{s.status.error}</div>}
              {s.status.state === "sign_in" && busy !== s.id && <div className="muted">{t("mcp.signInNeeded")}</div>}
              {s.signInUrl && (
                <div className="muted">
                  {t("mcp.signInWaiting")}{" "}
                  <a
                    href={s.signInUrl}
                    onClick={(e) => {
                      e.preventDefault();
                      Browser.OpenURL(s.signInUrl!).catch(onError);
                    }}
                  >
                    {t("mcp.signInAgain")}
                  </a>
                </div>
              )}
              {s.needsApproval && (
                <div className="banner warn mcp-approve">
                  <strong>{t("mcp.approve.title")}</strong> {s.transport === "http" ? t("mcp.approve.connect") : t("mcp.approve.run")}
                  <pre>{s.transport === "http" ? s.url : commandLine(s)}</pre>
                  {Object.keys(s.env ?? {}).length + (s.secretNames ?? []).length > 0 && (
                    <div className="muted">
                      {t(s.transport === "http" ? "mcp.approve.withHeaders" : "mcp.approve.withVariables", {
                        names: list([
                          ...Object.keys(s.transport === "http" ? (s.headers ?? {}) : (s.env ?? {})),
                          ...(s.secretNames ?? []).map((n) => t("mcp.approve.secret", { name: n })),
                        ]),
                      })}
                    </div>
                  )}
                  {s.warning && <div className="error-text">{s.warning}</div>}
                  <button className="primary" disabled={busy === s.id} onClick={() => run(s.id, EngineService.ApproveMCPServer(s.id, s.fingerprint))}>
                    {t("mcp.approve.start")}
                  </button>
                </div>
              )}
            </div>
            <div className="mcp-actions">
              {!s.needsApproval && s.status.state === "sign_in" && (
                <button className="primary" disabled={busy === s.id} onClick={() => run(s.id, EngineService.SignInMCPServer(s.id))}>
                  {busy === s.id ? t("mcp.signingIn") : t("mcp.signIn")}
                </button>
              )}
              {!s.needsApproval && s.status.state !== "running" && s.status.state !== "sign_in" && (
                <button disabled={busy === s.id} onClick={() => run(s.id, EngineService.StartMCPServer(s.id))}>
                  {busy === s.id ? t("mcp.starting") : t("mcp.start")}
                </button>
              )}
              {s.signedIn && (
                <button
                  onClick={async () => (await ask(t("mcp.signOutAsk", { name: s.name || s.id, store: secretStore() }))) && run(s.id, EngineService.SignOutMCPServer(s.id))}
                >
                  {t("mcp.signOut")}
                </button>
              )}
              {s.status.state === "running" && (
                <>
                  <button disabled={busy === s.id} onClick={() => run(s.id, EngineService.StartMCPServer(s.id).then(() => undefined))}>
                    {t("mcp.refresh")}
                  </button>
                  <button onClick={() => run(s.id, EngineService.StopMCPServer(s.id))}>{t("mcp.stop")}</button>
                </>
              )}
              <button onClick={() => setForm(toForm(s))}>{t("mcp.edit")}</button>
              <button
                className="danger"
                onClick={async () => (await ask(t("mcp.deleteAsk", { name: s.name || s.id, store: secretStore() }))) && run(s.id, EngineService.DeleteMCPServer(s.id))}
              >
                {t("mcp.delete")}
              </button>
            </div>
          </li>
        ))}
      </ul>
      <div className="row">
        <button className="primary" onClick={() => setForm(toForm())}>
          {t("mcp.add")}
        </button>
        <button onClick={() => setBrowsing(true)}>{t("mcp.browse")}</button>
        <button onClick={onImport}>{t("settings.import.button")}</button>
      </div>

      <h3>{t("mcp.suggested")}</h3>
      <p className="muted">{t("mcp.suggestedIntro")}</p>
      <ul className="mcp-suggestions">
        {MCP_SUGGESTIONS.map((s) => {
          const there = alreadyAdded(s, servers);
          return (
            <li key={s.id} className="card mcp-suggestion">
              <div className="row mcp-suggestion-head">
                <strong>{s.name}</strong>
                <span className="badge">{s.transport === "http" ? t("mcp.suggest.remote") : t("mcp.suggest.local")}</span>
                <span className="muted">{s.signIn === "oauth" ? t("mcp.suggest.oauth") : s.signIn === "token" ? t("mcp.suggest.token") : t("mcp.suggest.none")}</span>
              </div>
              <p>{t(s.description)}</p>
              <p className="muted">{t(s.reach)}</p>
              <div className="row">
                <button onClick={() => setForm(suggestionForm(s))} disabled={there} aria-label={there ? t("mcp.suggest.isAdded", { name: s.name }) : t("mcp.suggest.addNamed", { name: s.name })}>
                  {there ? t("mcp.suggest.added") : t("mcp.suggest.add")}
                </button>
                <button className="link" onClick={() => Browser.OpenURL(s.docs)}>
                  {t("mcp.suggest.about", { name: s.name })}
                </button>
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
