import { useEffect, useState, type FormEvent } from "react";
import { ask } from "./ask.ts";
import { EngineService, STARTING_MODES, type Command, type HarnessInfo, type Issue, type OptimizerStatus, type Profile, type ProviderInfo } from "./api.ts";
import { OPTIMIZER_OFF } from "./optimizers.ts";

// Profiles (HRN-003): one YAML file each in profiles/. The form covers the
// common fields and slash commands; rules are edited in the file.

interface Props {
  profiles: Profile[];
  harnesses: HarnessInfo[];
  providers: ProviderInfo[];
  onChanged: () => void;
  onError: (err: unknown) => void;
}

const TOOLS = ["read_file", "list_dir", "glob", "grep", "write_file", "edit_file", "shell"];
const EFFORTS = ["low", "medium", "high", "xhigh", "max"];

function blank(): Profile {
  return {
    id: "",
    name: "",
    description: "",
    harness: "builtin",
    provider: "",
    model: "",
    effort: "",
    mode: "",
    systemPrompt: "",
    tools: [],
    rules: [],
    maxTurns: 0,
    instructions: "",
    commands: [],
    skills: [],
    mcpServers: [],
    hooks: [],
    fallback: null,
    optimizer: "",
    source: "",
  };
}

export default function Profiles({ profiles, harnesses, providers, onChanged, onError }: Props) {
  // Token optimizers to choose from (HRN-019).
  const [optimizers, setOptimizers] = useState<OptimizerStatus[]>([]);
  useEffect(() => {
    EngineService.ListOptimizers()
      .then((l) => setOptimizers(l ?? []))
      .catch(onError);
  }, []);
  // A custom harness from a template (HRN-011).
  const [scaffold, setScaffold] = useState<{ name: string; language: string; folder: string; made?: string } | null>(null);
  const makeHarness = () => {
    if (!scaffold) return;
    EngineService.ScaffoldHarness({ name: scaffold.name, language: scaffold.language, folder: scaffold.folder })
      .then((r) => {
        setScaffold({ ...scaffold, made: r.dir });
        onChanged();
        return EngineService.OpenHarness(r.preset.id);
      })
      .catch(onError);
  };
  const [form, setForm] = useState<Profile | null>(null);
  const [isNew, setIsNew] = useState(false);
  const [busy, setBusy] = useState(false);
  // The YAML editor (GUI-009): the file as written, checked as you type.
  const [yamlEdit, setYamlEdit] = useState<{ id: string; text: string } | null>(null);
  const [yamlIssues, setYamlIssues] = useState<Issue[]>([]);
  useEffect(() => {
    if (!yamlEdit) return;
    const t = setTimeout(
      () =>
        EngineService.CheckProfileYAML(yamlEdit.id, yamlEdit.text)
          .then((l) => setYamlIssues(l ?? []))
          .catch(onError),
      250,
    );
    return () => clearTimeout(t);
  }, [yamlEdit?.text]);
  const openYAML = (id: string) =>
    EngineService.ProfileYAML(id)
      .then((text) => setYamlEdit({ id, text }))
      .catch(onError);

  const save = (e: FormEvent) => {
    e.preventDefault();
    if (!form) return;
    setBusy(true);
    EngineService.SaveProfile({ ...form, commands: (form.commands ?? []).filter((c) => c.name.trim() || c.prompt.trim()) })
      .then(() => {
        setForm(null);
        onChanged();
      })
      .catch(onError)
      .finally(() => setBusy(false));
  };

  const remove = async (p: Profile) => {
    const what = p.id === "default" ? "Reset the default profile to the built-in one" : `Delete ${p.name}`;
    if (!(await ask(`${what}? Sessions using it ${p.id === "default" ? "use the built-in default" : "cannot start until it exists again"}.`))) return;
    EngineService.DeleteProfile(p.id).then(onChanged).catch(onError);
  };

  const set = <K extends keyof Profile>(k: K, v: Profile[K]) => setForm((f) => (f ? { ...f, [k]: v } : f));
  const setCommand = (i: number, c: Partial<Command>) =>
    set(
      "commands",
      (form?.commands ?? []).map((x, n) => (n === i ? { ...x, ...c } : x)),
    );

  if (yamlEdit) {
    return (
      <section className="page">
        <h2>
          <code>profiles/{yamlEdit.id}.yaml</code>
        </h2>
        <textarea
          className="yaml-editor"
          spellCheck={false}
          rows={24}
          value={yamlEdit.text}
          onChange={(e) => setYamlEdit({ ...yamlEdit, text: e.target.value })}
          aria-label="Profile YAML"
        />
        {yamlIssues.length > 0 ? (
          <ul className="error-text">
            {yamlIssues.map((i, n) => (
              <li key={n}>
                {i.line ? `Line ${i.line}: ` : ""}
                {i.field ? `${i.field}: ` : ""}
                {i.message}
              </li>
            ))}
          </ul>
        ) : (
          <p className="ok">Valid.</p>
        )}
        <div className="row">
          <button
            className="primary"
            disabled={busy || yamlIssues.length > 0}
            onClick={() => {
              setBusy(true);
              EngineService.SaveProfileYAML(yamlEdit.id, yamlEdit.text)
                .then(() => {
                  setYamlEdit(null);
                  onChanged();
                })
                .catch(onError)
                .finally(() => setBusy(false));
            }}
          >
            Save
          </button>
          <button onClick={() => setYamlEdit(null)}>Cancel</button>
        </div>
      </section>
    );
  }

  if (form) {
    const tools = form.tools ?? [];
    const provider = providers.find((p) => p.id === form.provider);
    return (
      <section className="page">
        <h2>{isNew ? "New profile" : `Edit ${form.name || form.id}`}</h2>
        <form className="card form" onSubmit={save}>
          {isNew && (
            <label>
              ID <span className="muted">(the file name: lowercase letters, digits and dashes)</span>
              <input value={form.id} onChange={(e) => set("id", e.target.value)} placeholder="reviewer" required pattern="[a-z0-9][a-z0-9-]*" />
            </label>
          )}
          <label>
            Name
            <input value={form.name} onChange={(e) => set("name", e.target.value)} required />
          </label>
          <label>
            Description
            <input value={form.description} onChange={(e) => set("description", e.target.value)} />
          </label>
          <label>
            Harness
            <select value={form.harness || "builtin"} onChange={(e) => set("harness", e.target.value)}>
              {harnesses.map((h) => (
                <option key={h.id} value={h.id}>
                  {h.name}
                </option>
              ))}
            </select>
          </label>
          <div className="row wrap" hidden={!(harnesses.find((h) => h.id === (form.harness || "builtin"))?.usesProvider ?? true)}>
            <label>
              Provider
              <select value={form.provider} onChange={(e) => set("provider", e.target.value)}>
                <option value="">Chosen per session</option>
                {providers.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Model
              <input value={form.model} onChange={(e) => set("model", e.target.value)} placeholder={provider?.defaultModel || "Chosen per session"} />
            </label>
            <label title="Takes a call the model keeps failing with rate limits or server errors, after four tries">
              Fallback provider
              <select
                value={form.fallback?.provider ?? ""}
                onChange={(e) => set("fallback", e.target.value ? { provider: e.target.value, model: form.fallback?.model ?? "" } : null)}
              >
                <option value="">None</option>
                {providers.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
            </label>
            {form.fallback && (
              <label>
                Fallback model
                <input required value={form.fallback.model} onChange={(e) => set("fallback", { provider: form.fallback!.provider, model: e.target.value })} />
              </label>
            )}
          </div>
          <div className="row wrap">
            <label>
              Permission mode
              <select value={form.mode} onChange={(e) => set("mode", e.target.value)}>
                <option value="">From the environment</option>
                {STARTING_MODES.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.name}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Effort
              <select value={form.effort} onChange={(e) => set("effort", e.target.value)}>
                <option value="">Default (high)</option>
                {EFFORTS.map((x) => (
                  <option key={x}>{x}</option>
                ))}
              </select>
            </label>
            <label hidden={!(harnesses.find((h) => h.id === (form.harness || "builtin"))?.usesProvider || harnesses.find((h) => h.id === (form.harness || "builtin"))?.takesOptimizer || !harnesses.length)}>
              Token optimizer
              <select value={form.optimizer} onChange={(e) => set("optimizer", e.target.value)}>
                <option value="">From Settings</option>
                <option value={OPTIMIZER_OFF}>Off</option>
                {optimizers.map((o) => (
                  <option key={o.id} value={o.id}>
                    {o.needsApproval ? `${o.name} (not approved)` : o.path ? o.name : `${o.name} (not installed)`}
                  </option>
                ))}
                {form.optimizer && form.optimizer !== OPTIMIZER_OFF && !optimizers.some((o) => o.id === form.optimizer) && <option value={form.optimizer}>{form.optimizer}</option>}
              </select>
            </label>
            <label>
              Max model calls per message
              <input type="number" min={0} max={1000} value={form.maxTurns || ""} placeholder="100" onChange={(e) => set("maxTurns", Number(e.target.value) || 0)} />
            </label>
          </div>
          <label>
            System prompt <span className="muted">(added to the built-in agent's own)</span>
            <textarea rows={5} value={form.systemPrompt} onChange={(e) => set("systemPrompt", e.target.value)} />
          </label>
          <fieldset>
            <legend>Tools {tools.length === 0 && <span className="muted">(all)</span>}</legend>
            <div className="row wrap">
              {TOOLS.map((t) => (
                <label key={t} className="row">
                  <input
                    type="checkbox"
                    checked={tools.length === 0 || tools.includes(t)}
                    onChange={() => {
                      const on = tools.length === 0 ? TOOLS : tools;
                      const next = on.includes(t) ? on.filter((x) => x !== t) : [...on, t];
                      set("tools", next.length === TOOLS.length ? [] : next);
                    }}
                  />
                  <code>{t}</code>
                </label>
              ))}
            </div>
          </fieldset>
          <label>
            Skills <span className="muted">(names separated by commas; empty means every skill that is on)</span>
            <input
              key={form.id}
              defaultValue={(form.skills ?? []).join(", ")}
              onBlur={(e) =>
                set(
                  "skills",
                  e.target.value
                    .split(",")
                    .map((x) => x.trim())
                    .filter(Boolean),
                )
              }
            />
          </label>
          <label>
            MCP servers <span className="muted">(IDs separated by commas, added to the servers that are on for everyone)</span>
            <input
              key={form.id + "-mcp"}
              defaultValue={(form.mcpServers ?? []).join(", ")}
              onBlur={(e) =>
                set(
                  "mcpServers",
                  e.target.value
                    .split(",")
                    .map((x) => x.trim())
                    .filter(Boolean),
                )
              }
            />
          </label>
          <label className="row">
            <input type="checkbox" checked={form.instructions !== "none"} onChange={(e) => set("instructions", e.target.checked ? "" : "none")} /> Read
            the project's AGENTS.md or CLAUDE.md
          </label>
          <fieldset>
            <legend>Slash commands</legend>
            {(form.commands ?? []).map((c, i) => (
              <div key={i} className="profile-command">
                <div className="row">
                  <span className="muted">/</span>
                  <input value={c.name} onChange={(e) => setCommand(i, { name: e.target.value })} placeholder="review" aria-label="Command name" />
                  <input value={c.description} onChange={(e) => setCommand(i, { description: e.target.value })} placeholder="What it does" aria-label="Description" />
                  <button type="button" onClick={() => set("commands", (form.commands ?? []).filter((_, n) => n !== i))}>
                    Remove
                  </button>
                </div>
                <textarea rows={2} value={c.prompt} onChange={(e) => setCommand(i, { prompt: e.target.value })} placeholder="The prompt it sends" aria-label="Prompt" />
              </div>
            ))}
            <div>
              <button type="button" onClick={() => set("commands", [...(form.commands ?? []), { name: "", description: "", prompt: "" }])}>
                Add a command
              </button>
            </div>
          </fieldset>
          {(form.rules ?? []).length > 0 && (
            <div>
              <strong>Rules</strong> <span className="muted">(edit them in {form.source || "the file"})</span>
              <ul>
                {(form.rules ?? []).map((r, i) => (
                  <li key={i}>
                    <code>
                      {r.effect} {r.tool}
                      {r.command ? ` ${r.command}` : r.path ? ` ${r.path}` : ""}
                    </code>
                  </li>
                ))}
              </ul>
            </div>
          )}
          <div className="row">
            <button type="submit" className="primary" disabled={busy}>
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
      <h2>Profiles</h2>
      <p className="muted">
        A profile sets the model, tools, prompt, rules and slash commands a session runs with. Each is a YAML file in <code>profiles/</code> of the app's
        data folder; edits to the files apply to new sessions, and their rules at once.
      </p>
      <div className="card">
        <h3>Your own agent</h3>
        <p className="muted">
          Start an agent of your own from a Python or Go template that already speaks the Agent Client Protocol. Freeloop registers it as a harness;
          when you change its files, sessions start it again with your code. See docs/DEVELOPING.md.
        </p>
        {!scaffold && (
          <button onClick={() => setScaffold({ name: "", language: "python", folder: "" })}>New harness…</button>
        )}
        {scaffold && !scaffold.made && (
          <form
            className="skill-create"
            onSubmit={(e) => {
              e.preventDefault();
              makeHarness();
            }}
          >
            <label>
              Name
              <input required autoFocus value={scaffold.name} onChange={(e) => setScaffold({ ...scaffold, name: e.target.value })} />
            </label>
            <label>
              Template
              <select value={scaffold.language} onChange={(e) => setScaffold({ ...scaffold, language: e.target.value })}>
                <option value="python">Python (standard library only)</option>
                <option value="go">Go (github.com/coder/acp-go-sdk)</option>
              </select>
            </label>
            <div className="row">
              <button
                type="button"
                onClick={() =>
                  EngineService.ChooseFolder()
                    .then((f) => f && setScaffold({ ...scaffold, folder: f }))
                    .catch(onError)
                }
              >
                Choose a folder…
              </button>
              <code>{scaffold.folder || "No folder chosen"}</code>
            </div>
            <div className="row">
              <button type="submit" className="primary" disabled={!scaffold.folder || !scaffold.name.trim()}>
                Create and open
              </button>
              <button type="button" onClick={() => setScaffold(null)}>
                Cancel
              </button>
            </div>
          </form>
        )}
        {scaffold?.made && (
          <p className="ok">
            Created in <code>{scaffold.made}</code>. Pick it in the new-session form; it answers with what you said until you change it.{" "}
            <button className="link" onClick={() => setScaffold(null)}>
              Done
            </button>
          </p>
        )}
        {harnesses.filter((h) => h.custom).length > 0 && (
          <ul>
            {harnesses
              .filter((h) => h.custom)
              .map((h) => (
                <li key={h.id}>
                  {h.name}{" "}
                  <button className="link" onClick={() => EngineService.OpenHarness(h.id).catch(onError)}>
                    Edit
                  </button>
                  {h.missing && <span className="error-text"> {h.missing}</span>}
                </li>
              ))}
          </ul>
        )}
      </div>
      <ul className="provider-list">
        {profiles.map((p) => (
          <li key={p.id} className="card">
            <div>
              <strong>{p.name}</strong> <span className="muted">{p.id}</span>
              {p.description && <div className="muted">{p.description}</div>}
              <div className="muted">
                {[
                  p.provider && `${providers.find((x) => x.id === p.provider)?.name ?? p.provider}${p.model ? ` · ${p.model}` : ""}`,
                  (p.tools ?? []).length > 0 && `${(p.tools ?? []).length} tools`,
                  (p.rules ?? []).length > 0 && `${(p.rules ?? []).length} rules`,
                  (p.commands ?? []).length > 0 && (p.commands ?? []).map((c) => `/${c.name}`).join(" "),
                  !p.source && "built in",
                ]
                  .filter(Boolean)
                  .join(" · ")}
              </div>
            </div>
            <div className="row">
              <button
                onClick={() => {
                  setIsNew(false);
                  setForm({ ...p });
                }}
              >
                Edit
              </button>
              {!p.id.startsWith("project:") && <button onClick={() => openYAML(p.id)}>YAML</button>}
              {p.source && <button onClick={() => remove(p)}>{p.id === "default" ? "Reset" : "Delete"}</button>}
            </div>
          </li>
        ))}
      </ul>
      <button
        className="primary"
        onClick={() => {
          setIsNew(true);
          setForm(blank());
        }}
      >
        New profile
      </button>
    </section>
  );
}
