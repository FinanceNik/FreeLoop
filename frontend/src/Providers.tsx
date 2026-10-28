import { useState, type FormEvent } from "react";
import { EngineService, errorText, type ModelInfo, type ProviderInfo } from "./api.ts";

interface Props {
  providers: ProviderInfo[];
  onChanged: () => void;
  onError: (err: unknown) => void;
}

const BLANK = { id: "", name: "Anthropic", type: "anthropic", baseUrl: "", apiKey: "", defaultModel: "claude-opus-5-5" };

export default function Providers({ providers, onChanged, onError }: Props) {
  const [form, setForm] = useState(BLANK);
  const [editing, setEditing] = useState(false);
  const [tests, setTests] = useState<Record<string, { ok: boolean; text: string; models?: ModelInfo[] }>>({});
  const [busy, setBusy] = useState(false);

  const set = (k: keyof typeof BLANK) => (e: { target: { value: string } }) => setForm({ ...form, [k]: e.target.value });

  const save = (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    EngineService.SaveProvider(form)
      .then((p) => {
        setForm(BLANK);
        setEditing(false);
        onChanged();
        test(p.id);
      })
      .catch(onError)
      .finally(() => setBusy(false));
  };

  const test = (id: string) => {
    setTests((t) => ({ ...t, [id]: { ok: true, text: "Testing…" } }));
    EngineService.TestProvider(id)
      .then((models) => {
        const list = models ?? [];
        setTests((t) => ({ ...t, [id]: { ok: true, text: `Connected: ${list.length} models`, models: list } }));
      })
      .catch((err) => setTests((t) => ({ ...t, [id]: { ok: false, text: errorText(err) } })));
  };

  const remove = (p: ProviderInfo) => {
    if (!confirm(`Delete ${p.name}? Its API key is removed from the Keychain.`)) return;
    EngineService.DeleteProvider(p.id).then(onChanged).catch(onError);
  };

  const edit = (p: ProviderInfo) => {
    setForm({ id: p.id, name: p.name, type: p.type, baseUrl: p.baseUrl, apiKey: "", defaultModel: p.defaultModel });
    setEditing(true);
  };

  return (
    <section className="page">
      <h2>Providers</h2>
      <p className="muted">
        API keys are stored in the macOS Keychain. Freeloop never shows a saved key again and only connects to the endpoints you add here.
      </p>
      <ul className="provider-list">
        {providers.map((p) => (
          <li key={p.id} className="card">
            <div>
              <strong>{p.name}</strong> <span className="muted">{p.type}</span>
              {p.builtin && <span className="muted"> · development only</span>}
              <div className="muted">
                {p.baseUrl || "default endpoint"} · default model {p.defaultModel || "none"} · {p.hasKey || p.builtin ? "key set" : "no key"}
              </div>
              {tests[p.id] && (
                <div className={tests[p.id].ok ? "ok" : "error-text"} role="status">
                  {tests[p.id].text}
                </div>
              )}
            </div>
            <div className="row">
              <button onClick={() => test(p.id)}>Test connection</button>
              {!p.builtin && <button onClick={() => edit(p)}>Edit</button>}
              {!p.builtin && (
                <button className="danger" onClick={() => remove(p)}>
                  Delete
                </button>
              )}
            </div>
          </li>
        ))}
      </ul>

      {!editing && (
        <button className="primary" onClick={() => setEditing(true)}>
          Add provider
        </button>
      )}
      {editing && (
        <form className="card form" onSubmit={save}>
          <h3>{form.id ? `Edit ${form.name}` : "Add provider"}</h3>
          <label>
            Type
            <select value={form.type} onChange={set("type")}>
              <option value="anthropic">Anthropic</option>
            </select>
          </label>
          <label>
            Name
            <input value={form.name} onChange={set("name")} required />
          </label>
          <label>
            Base URL
            <input value={form.baseUrl} onChange={set("baseUrl")} placeholder="https://api.anthropic.com" />
          </label>
          <label>
            API key
            <input
              type="password"
              autoComplete="off"
              value={form.apiKey}
              onChange={set("apiKey")}
              placeholder={form.id ? "Leave empty to keep the saved key" : "sk-ant-…"}
            />
          </label>
          <label>
            Default model
            <input value={form.defaultModel} onChange={set("defaultModel")} placeholder="claude-opus-5-5" />
          </label>
          <div className="row">
            <button type="submit" className="primary" disabled={busy}>
              Save
            </button>
            <button
              type="button"
              onClick={() => {
                setForm(BLANK);
                setEditing(false);
              }}
            >
              Cancel
            </button>
          </div>
        </form>
      )}
    </section>
  );
}
