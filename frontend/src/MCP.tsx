import { useEffect, useState, type FormEvent } from "react";
import { EngineService, type MCPServer, type MCPServerInfo } from "./api.ts";

// MCP servers (MCP-001, MCP-005): stdio commands and Streamable HTTP
// endpoints in mcp.yaml. A server runs only after the user approves exactly
// what it runs; any change asks again. Secrets go to the Keychain.

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
  headers: string; // Name: value per line
  enabled: boolean;
  secretNames: string[];
  remove: string[];
  added: { name: string; value: string }[];
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
    headers: Object.entries(s?.headers ?? {})
      .map(([k, v]) => `${k}: ${v}`)
      .join("\n"),
    enabled: s?.enabled ?? true,
    secretNames: s?.secretNames ?? [],
    remove: [],
    added: [],
  };
}

function pairs(text: string, sep: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const t = line.trim();
    if (!t) continue;
    const i = t.indexOf(sep);
    if (i <= 0) throw new Error(`"${t}" is not name${sep}value`);
    out[t.slice(0, i).trim()] = t.slice(i + sep.length).trim();
  }
  return out;
}

// commandLine shows what a stdio server runs, quoting arguments with spaces.
function commandLine(s: MCPServer): string {
  return [s.command, ...(s.args ?? [])].map((a) => (/\s/.test(a) ? JSON.stringify(a) : a)).join(" ");
}

export default function MCP({ configVersion, onImport, onError }: Props) {
  const [servers, setServers] = useState<MCPServerInfo[]>([]);
  const [form, setForm] = useState<Form | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
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
    const t = setTimeout(load, 1000);
    return () => clearTimeout(t);
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

  if (form) {
    const set = <K extends keyof Form>(k: K, v: Form[K]) => setForm({ ...form, [k]: v });
    return (
      <section className="page">
        <h2>{form.isNew ? "Add an MCP server" : `Edit ${form.name || form.id}`}</h2>
        <form className="card form" onSubmit={save}>
          {form.isNew && (
            <label>
              ID <span className="muted">(its tools are named mcp__ID__tool)</span>
              <input value={form.id} onChange={(e) => set("id", e.target.value)} placeholder="github" required pattern="[a-z0-9]+(-[a-z0-9]+)*" />
            </label>
          )}
          <label>
            Name
            <input value={form.name} onChange={(e) => set("name", e.target.value)} />
          </label>
          <label>
            Transport
            <select value={form.transport} onChange={(e) => set("transport", e.target.value)}>
              <option value="stdio">Command (stdio)</option>
              <option value="http">Remote (Streamable HTTP)</option>
            </select>
          </label>
          {form.transport === "stdio" ? (
            <>
              <label>
                Command
                <input value={form.command} onChange={(e) => set("command", e.target.value)} placeholder="npx" required />
              </label>
              <label>
                Arguments <span className="muted">(one per line)</span>
                <textarea rows={3} value={form.args} onChange={(e) => set("args", e.target.value)} placeholder={"-y\n@modelcontextprotocol/server-github@1.0.0"} />
              </label>
              <label>
                Variables <span className="muted">(KEY=value per line; use secrets for keys)</span>
                <textarea rows={2} value={form.env} onChange={(e) => set("env", e.target.value)} />
              </label>
            </>
          ) : (
            <>
              <label>
                URL
                <input value={form.url} onChange={(e) => set("url", e.target.value)} placeholder="https://example.com/mcp" required />
              </label>
              <label>
                Headers <span className="muted">(Name: value per line; use secrets for tokens)</span>
                <textarea rows={2} value={form.headers} onChange={(e) => set("headers", e.target.value)} />
              </label>
            </>
          )}
          <fieldset>
            <legend>Secrets {form.transport === "http" ? "(sent as headers)" : "(passed as variables)"}</legend>
            {form.secretNames
              .filter((n) => !form.remove.includes(n))
              .map((n) => (
                <div key={n} className="row">
                  <code>{n}</code> <span className="muted">stored in the Keychain</span>
                  <button type="button" onClick={() => set("remove", [...form.remove, n])}>
                    Remove
                  </button>
                </div>
              ))}
            {form.added.map((s, i) => (
              <div key={i} className="row">
                <input
                  placeholder={form.transport === "http" ? "Authorization" : "GITHUB_TOKEN"}
                  value={s.name}
                  onChange={(e) => set("added", form.added.map((x, n) => (n === i ? { ...x, name: e.target.value } : x)))}
                  aria-label="Secret name"
                />
                <input
                  type="password"
                  placeholder="value"
                  value={s.value}
                  onChange={(e) => set("added", form.added.map((x, n) => (n === i ? { ...x, value: e.target.value } : x)))}
                  aria-label="Secret value"
                />
              </div>
            ))}
            <div>
              <button type="button" onClick={() => set("added", [...form.added, { name: "", value: "" }])}>
                Add a secret
              </button>
            </div>
          </fieldset>
          <label className="row">
            <input type="checkbox" checked={form.enabled} onChange={(e) => set("enabled", e.target.checked)} /> Offer its tools in every session
            <span className="muted">(profiles can add servers that are off)</span>
          </label>
          <div className="row">
            <button type="submit" className="primary">
              Save
            </button>
            <button type="button" onClick={() => setForm(null)}>
              Cancel
            </button>
          </div>
        </form>
      </section>
    );
  }

  return (
    <section className="page">
      <h2>MCP servers</h2>
      <p className="muted">
        Tools from Model Context Protocol servers, offered to the built-in agent as <code>mcp__server__tool</code>. Every call asks unless a rule
        allows it, such as <code>mcp__github__*</code>. Servers run with your privileges; prefer ones you can read and pin their versions.
      </p>
      {servers.length === 0 && <p className="muted">No servers yet.</p>}
      <ul className="provider-list">
        {servers.map((s) => (
          <li key={s.id} className="card mcp-server">
            <div className="mcp-main">
              <div>
                <strong>{s.name || s.id}</strong> <span className="muted">{s.id}</span>{" "}
                <span className={`badge mcp-${s.needsApproval ? "approval" : s.status.state}`}>{s.needsApproval ? "needs approval" : s.status.state}</span>
                {!s.enabled && <span className="muted"> · off by default</span>}
              </div>
              <code className="mcp-cmd">{s.transport === "http" ? s.url : commandLine(s)}</code>
              {s.source && <div className="muted">Imported from {s.source}</div>}
              {s.status.state === "running" && (
                <details>
                  <summary className="muted">
                    {(s.status.tools ?? []).length} tools · protocol {s.status.protocol}
                  </summary>
                  <ul>
                    {(s.status.tools ?? []).map((t) => (
                      <li key={t.name}>
                        <code>{t.name}</code> <span className="muted">{t.description}</span>
                      </li>
                    ))}
                  </ul>
                </details>
              )}
              {s.status.error && <div className="error-text">{s.status.error}</div>}
              {s.needsApproval && (
                <div className="banner warn mcp-approve">
                  <strong>Approve before it runs.</strong> It will {s.transport === "http" ? "connect to" : "run"}:
                  <pre>{s.transport === "http" ? s.url : commandLine(s)}</pre>
                  {Object.keys(s.env ?? {}).length + (s.secretNames ?? []).length > 0 && (
                    <div className="muted">
                      With {s.transport === "http" ? "headers" : "variables"}: {[...Object.keys(s.transport === "http" ? (s.headers ?? {}) : (s.env ?? {})), ...(s.secretNames ?? []).map((n) => `${n} (secret)`)].join(", ")}
                    </div>
                  )}
                  {s.warning && <div className="error-text">{s.warning}</div>}
                  <button className="primary" disabled={busy === s.id} onClick={() => run(s.id, EngineService.ApproveMCPServer(s.id, s.fingerprint))}>
                    Approve and start
                  </button>
                </div>
              )}
            </div>
            <div className="mcp-actions">
              {!s.needsApproval && s.status.state !== "running" && (
                <button disabled={busy === s.id} onClick={() => run(s.id, EngineService.StartMCPServer(s.id))}>
                  {busy === s.id ? "Starting…" : "Start"}
                </button>
              )}
              {s.status.state === "running" && (
                <>
                  <button disabled={busy === s.id} onClick={() => run(s.id, EngineService.StartMCPServer(s.id).then(() => undefined))}>
                    Refresh
                  </button>
                  <button onClick={() => run(s.id, EngineService.StopMCPServer(s.id))}>Stop</button>
                </>
              )}
              <button onClick={() => setForm(toForm(s))}>Edit</button>
              <button
                className="danger"
                onClick={() => confirm(`Delete ${s.name || s.id}? Its secrets are removed from the Keychain.`) && run(s.id, EngineService.DeleteMCPServer(s.id))}
              >
                Delete
              </button>
            </div>
          </li>
        ))}
      </ul>
      <div className="row">
        <button className="primary" onClick={() => setForm(toForm())}>
          Add a server
        </button>
        <button onClick={onImport}>Import from Claude Code…</button>
      </div>
    </section>
  );
}
