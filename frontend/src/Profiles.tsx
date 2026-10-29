import { useState, type FormEvent } from "react";
import { EngineService, MODES, type Command, type Profile, type ProviderInfo } from "./api.ts";

// Profiles (HRN-003): one YAML file each in profiles/. The form covers the
// common fields and slash commands; rules are edited in the file.

interface Props {
  profiles: Profile[];
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
    source: "",
  };
}

export default function Profiles({ profiles, providers, onChanged, onError }: Props) {
  const [form, setForm] = useState<Profile | null>(null);
  const [isNew, setIsNew] = useState(false);
  const [busy, setBusy] = useState(false);

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

  const remove = (p: Profile) => {
    const what = p.id === "default" ? "Reset the default profile to the built-in one" : `Delete ${p.name}`;
    if (!confirm(`${what}? Sessions using it ${p.id === "default" ? "use the built-in default" : "cannot start until it exists again"}.`)) return;
    EngineService.DeleteProfile(p.id).then(onChanged).catch(onError);
  };

  const set = <K extends keyof Profile>(k: K, v: Profile[K]) => setForm((f) => (f ? { ...f, [k]: v } : f));
  const setCommand = (i: number, c: Partial<Command>) =>
    set(
      "commands",
      (form?.commands ?? []).map((x, n) => (n === i ? { ...x, ...c } : x)),
    );

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
          <div className="row wrap">
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
          </div>
          <div className="row wrap">
            <label>
              Permission mode
              <select value={form.mode} onChange={(e) => set("mode", e.target.value)}>
                <option value="">From the environment</option>
                {MODES.filter((m) => m.id !== "bypass").map((m) => (
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
