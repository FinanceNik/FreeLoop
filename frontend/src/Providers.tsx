import { useEffect, useState, type FormEvent } from "react";
import { secretStore } from "./platform.ts";
import { ask } from "./ask.ts";
import { keyMoves, plainRemote } from "./addresses.ts";
import { EngineService, errorText, type HarnessInfo, type ModelInfo, type PresetInfo, type ProviderInfo } from "./api.ts";
import { t } from "./i18n.ts";

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

// A connection test, kept as what happened and written at render, so a change of
// language shows at once; an error is the engine's, shown as given.
type TestState = { state: "testing" } | { state: "ok"; models: ModelInfo[] } | { state: "failed"; error: string };

// presetOf maps a saved provider back to the preset it came from.
function presetOf(p: ProviderInfo): string {
  return p.type === "openai_compat" ? p.preset || "custom" : p.type;
}

export default function Providers({ providers, onChanged, onError, agents, defaultProfileId, onStartWith }: Props) {
  const [presets, setPresets] = useState<PresetInfo[]>([]);
  const [form, setForm] = useState<Form | null>(null);
  const [tests, setTests] = useState<Record<string, TestState>>({});
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

  const save = async (e: FormEvent) => {
    e.preventDefault();
    if (!form) return;
    // The saved key goes along to a new address unless a new one is entered.
    const before = providers.find((p) => p.id === form.id);
    if (before?.hasKey && !form.apiKey && keyMoves(before.baseUrl, form.baseUrl === preset?.baseUrl ? "" : form.baseUrl)) {
      const question = form.baseUrl
        ? t("providers.ask.moveKey", { name: before.name, address: form.baseUrl })
        : t("providers.ask.moveKeyDefault", { name: before.name });
      if (!(await ask(question))) return;
    }
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
    setTests((old) => ({ ...old, [id]: { state: "testing" } }));
    EngineService.TestProvider(id)
      .then((models) => setTests((old) => ({ ...old, [id]: { state: "ok", models: models ?? [] } })))
      .catch((err) => setTests((old) => ({ ...old, [id]: { state: "failed", error: errorText(err) } })));
  };

  const remove = async (p: ProviderInfo) => {
    if (!(await ask(t("providers.ask.delete", { name: p.name, store: secretStore() })))) return;
    EngineService.DeleteProvider(p.id).then(onChanged).catch(onError);
  };

  const edit = (p: ProviderInfo) => {
    const pr = presets.find((x) => x.id === presetOf(p));
    setForm({ id: p.id, preset: presetOf(p), name: p.name, type: p.type, baseUrl: p.baseUrl || pr?.baseUrl || "", apiKey: "", defaultModel: p.defaultModel });
  };

  return (
    <section className="page">
      <h2>{t("settings.page.providers")}</h2>
      {agents && agents.length > 0 && (
        <section className="choice" aria-labelledby="signed-in-agents">
          <h3 id="signed-in-agents">{t("providers.agents.title")}</h3>
          <p className="muted">{t("providers.agents.intro")}</p>
          <ul className="agent-choices">
            {agents.map((h) => (
              <li key={h.id} className="agent-choice">
                <div>
                  <strong>{h.name}</strong>
                  {h.missing ? <span className="error-text"> · {h.missing}</span> : <span className="ok"> · {t("providers.agents.ready")}</span>}
                  {defaultProfileId === h.id && <span className="muted"> · {t("providers.agents.default")}</span>}
                  {h.description && <p>{h.description}</p>}
                  {h.notes && <p className="muted">{h.notes}</p>}
                </div>
                {!h.missing && onStartWith && (
                  <button className="primary" onClick={() => onStartWith(h.id)}>
                    {t("providers.agents.start", { name: h.name })}
                  </button>
                )}
              </li>
            ))}
          </ul>
        </section>
      )}
      {agents && agents.length > 0 && <h3>{t("providers.keys.title")}</h3>}
      <p className="muted">{t("providers.keys.intro", { store: secretStore() })}</p>
      <ul className="provider-list">
        {providers.map((p) => (
          <li key={p.id} className="card">
            <div>
              <strong>{p.name}</strong> <span className="muted">{presets.find((x) => x.id === presetOf(p))?.name ?? p.type}</span>
              {p.builtin && <span className="muted"> · {t("providers.devOnly")}</span>}
              <div className="muted">
                {p.baseUrl || t("providers.defaultEndpoint")} · {p.defaultModel ? t("providers.defaultModel", { model: p.defaultModel }) : t("providers.noDefaultModel")}
                {p.hasKey ? ` · ${t("providers.keySet")}` : ""}
              </div>
              {p.addressChanged && (
                <div className="banner warn row" role="alert">
                  <span>{t("providers.addressChanged")}</span>
                  <button
                    onClick={async () => {
                      const question = p.baseUrl
                        ? t("providers.ask.confirmAddress", { name: p.name, address: p.baseUrl })
                        : t("providers.ask.confirmAddressDefault", { name: p.name });
                      if (!(await ask(question))) return;
                      EngineService.ConfirmProviderAddress(p.id).then(onChanged).catch(onError);
                    }}
                  >
                    {t("providers.useAddress")}
                  </button>
                </div>
              )}
              {tests[p.id] && <TestResult test={tests[p.id]} />}
            </div>
            <div className="row">
              <button onClick={() => test(p.id)}>{t("providers.test")}</button>
              {!p.builtin && <button onClick={() => edit(p)}>{t("providers.edit")}</button>}
              {!p.builtin && (
                <button className="danger" onClick={() => remove(p)}>
                  {t("providers.delete")}
                </button>
              )}
            </div>
          </li>
        ))}
      </ul>

      {!form && (
        <button className="primary" onClick={startAdd} disabled={presets.length === 0}>
          {t("providers.add")}
        </button>
      )}
      {form && (
        <form className="card form" onSubmit={save}>
          <h3>{form.id ? t("providers.editNamed", { name: form.name }) : t("providers.add")}</h3>
          <label>
            {t("providers.kind")}
            <select value={form.preset} onChange={(e) => choosePreset(e.target.value)} disabled={!!form.id}>
              {presets.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.local ? t("providers.localPreset", { name: p.name }) : p.name}
                </option>
              ))}
            </select>
          </label>
          {preset?.note && <p className="muted">{preset.note}</p>}
          <label>
            {t("providers.name")}
            <input value={form.name} onChange={set("name")} required />
          </label>
          <label>
            {t("providers.baseUrl")}
            <input value={form.baseUrl} onChange={set("baseUrl")} placeholder={preset?.baseUrl || "http://localhost:8080/v1"} required={preset?.id === "custom"} />
          </label>
          <label>
            {t("providers.apiKey")} {preset && !preset.needsKey && <span className="muted">{t("providers.optionalLocal")}</span>}
            {plainRemote(form.baseUrl) && <span className="error-text"> {t("providers.plainHttp")}</span>}
            <input
              type="password"
              autoComplete="off"
              value={form.apiKey}
              onChange={set("apiKey")}
              placeholder={form.id ? t("providers.keepKey") : ""}
              required={!form.id && !!preset?.needsKey}
            />
          </label>
          <label>
            {t("providers.defaultModelLabel")} <span className="muted">{t("providers.optional")}</span>
            <input value={form.defaultModel} onChange={set("defaultModel")} placeholder={preset?.local ? t("providers.pickWhenStarting") : ""} />
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

function TestResult({ test }: { test: TestState }) {
  return (
    <div className={test.state === "failed" ? "error-text" : "ok"} role="status">
      {test.state === "testing" ? t("providers.testing") : test.state === "ok" ? t("providers.connected", { count: test.models.length }) : test.error}
    </div>
  );
}
