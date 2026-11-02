import { useEffect, useState, type FormEvent } from "react";
import { EngineService, errorText, type HarnessInfo, type ModelInfo, type PresetInfo, type ProviderInfo } from "./api.ts";

interface Props {
  providers: ProviderInfo[];
  onChanged: () => void;
  onError: (err: unknown) => void;
  // Agents that sign in by themselves (such as Claude Code with a Claude
  // subscription), shown above the providers on the Providers page; they
  // need no key here. onStartWith starts a session with one.
  agents?: HarnessInfo[];
  defaultProfileId?: string;
  onStartWith?: (profileId: string) => void;
}

interface Form {
  id: string;
  preset: string;
  name: string;
  type: string;
  baseUrl: string;
  apiKey: string;
  defaultModel: string;
}

const EMPTY: Form = { id: "", preset: "", name: "", type: "", baseUrl: "", apiKey: "", defaultModel: "" };

// presetOf maps a saved provider back to the preset it came from.
function presetOf(p: ProviderInfo): string {
  return p.type === "openai_compat" ? p.preset || "custom" : p.type;
}

export default function Providers({ providers, onChanged, onError, agents, defaultProfileId, onStartWith }: Props) {
  const [presets, setPresets] = useState<PresetInfo[]>([]);
  const [form, setForm] = useState<Form | null>(null);
  const [tests, setTests] = useState<Record<string, { ok: boolean; text: string; models?: ModelInfo[] }>>({});
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    EngineService.ProviderPresets()
      .then((list) => setPresets(list ?? []))
      .catch(onError);
  }, []);

  const preset = presets.find((p) => p.id === form?.preset);

  const choosePreset = (id: string) => {
    const p = presets.find((x) => x.id === id);
    if (!p || !form) return;
    setForm({ ...form, preset: p.id, type: p.type, name: form.id ? form.name : p.name, baseUrl: p.baseUrl, defaultModel: p.defaultModel });
  };

  const startAdd = () => {
    const p = presets[0];
    setForm(p ? { ...EMPTY, preset: p.id, type: p.type, name: p.name, baseUrl: p.baseUrl, defaultModel: p.defaultModel } : EMPTY);
  };

  const set = (k: keyof Form) => (e: { target: { value: string } }) => form && setForm({ ...form, [k]: e.target.value });

  const save = (e: FormEvent) => {
    e.preventDefault();
    if (!form) return;
    setBusy(true);
    EngineService.SaveProvider({
      id: form.id,
      name: form.name,
      type: form.type,
      preset: form.type === "openai_compat" ? form.preset : "",
      baseUrl: form.baseUrl === preset?.baseUrl ? "" : form.baseUrl,
      apiKey: form.apiKey,
      defaultModel: form.defaultModel,
    })
      .then((p) => {
        setForm(null);
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
        setTests((t) => ({ ...t, [id]: { ok: true, text: `Connected: ${list.length} model${list.length === 1 ? "" : "s"}`, models: list } }));
      })
      .catch((err) => setTests((t) => ({ ...t, [id]: { ok: false, text: errorText(err) } })));
  };

  const remove = (p: ProviderInfo) => {
    if (!confirm(`Delete ${p.name}? Its API key is removed from the Keychain.`)) return;
    EngineService.DeleteProvider(p.id).then(onChanged).catch(onError);
  };

  const edit = (p: ProviderInfo) => {
    const pr = presets.find((x) => x.id === presetOf(p));
    setForm({ id: p.id, preset: presetOf(p), name: p.name, type: p.type, baseUrl: p.baseUrl || pr?.baseUrl || "", apiKey: "", defaultModel: p.defaultModel });
  };

  return (
    <section className="page">
      <h2>Providers</h2>
      {agents && agents.length > 0 && (
        <section className="choice" aria-labelledby="signed-in-agents">
          <h3 id="signed-in-agents">Agents you're signed in to</h3>
          <p className="muted">These need no key here: Freeloop starts the agent on your Mac and it uses its own account. There is nothing to add.</p>
          <ul className="agent-choices">
            {agents.map((h) => (
              <li key={h.id} className="agent-choice">
                <div>
                  <strong>{h.name}</strong>
                  {h.missing ? <span className="error-text"> · {h.missing}</span> : <span className="ok"> · ready</span>}
                  {defaultProfileId === h.id && <span className="muted"> · default for new sessions</span>}
                  {h.description && <p>{h.description}</p>}
                  {h.notes && <p className="muted">{h.notes}</p>}
                </div>
                {!h.missing && onStartWith && (
                  <button className="primary" onClick={() => onStartWith(h.id)}>
                    Start a session with {h.name}
                  </button>
                )}
              </li>
            ))}
          </ul>
        </section>
      )}
      {agents && agents.length > 0 && <h3>API keys and local models</h3>}
      <p className="muted">
        API keys are stored in the macOS Keychain. Freeloop never shows a saved key again and only connects to the endpoints you add here.
      </p>
      <ul className="provider-list">
        {providers.map((p) => (
          <li key={p.id} className="card">
            <div>
              <strong>{p.name}</strong> <span className="muted">{presets.find((x) => x.id === presetOf(p))?.name ?? p.type}</span>
              {p.builtin && <span className="muted"> · development only</span>}
              <div className="muted">
                {p.baseUrl || "default endpoint"} · default model {p.defaultModel || "none"}
                {p.hasKey ? " · key set" : ""}
              </div>
              {p.addressChanged && (
                <div className="banner warn row" role="alert">
                  <span>
                    This address was changed outside Freeloop, so the API key is not being sent. If you did not change it, something on this Mac may be trying to get your key.
                  </span>
                  <button
                    onClick={() => {
                      if (!confirm(`Send the API key of ${p.name} to ${p.baseUrl || "its default endpoint"}?`)) return;
                      EngineService.ConfirmProviderAddress(p.id).then(onChanged).catch(onError);
                    }}
                  >
                    Use this address
                  </button>
                </div>
              )}
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

      {!form && (
        <button className="primary" onClick={startAdd} disabled={presets.length === 0}>
          Add provider
        </button>
      )}
      {form && (
        <form className="card form" onSubmit={save}>
          <h3>{form.id ? `Edit ${form.name}` : "Add provider"}</h3>
          <label>
            Kind
            <select value={form.preset} onChange={(e) => choosePreset(e.target.value)} disabled={!!form.id}>
              {presets.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                  {p.local ? " (local)" : ""}
                </option>
              ))}
            </select>
          </label>
          {preset?.note && <p className="muted">{preset.note}</p>}
          <label>
            Name
            <input value={form.name} onChange={set("name")} required />
          </label>
          <label>
            Base URL
            <input value={form.baseUrl} onChange={set("baseUrl")} placeholder={preset?.baseUrl || "http://localhost:8080/v1"} required={preset?.id === "custom"} />
          </label>
          <label>
            API key {preset && !preset.needsKey && <span className="muted">(optional for local servers)</span>}
            <input
              type="password"
              autoComplete="off"
              value={form.apiKey}
              onChange={set("apiKey")}
              placeholder={form.id ? "Leave empty to keep the saved key" : ""}
              required={!form.id && !!preset?.needsKey}
            />
          </label>
          <label>
            Default model <span className="muted">(optional)</span>
            <input value={form.defaultModel} onChange={set("defaultModel")} placeholder={preset?.local ? "Pick one when starting a session" : ""} />
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
