import { useEffect, useState, type FormEvent } from "react";
import { EngineService, MODES, errorText, type ModelCaps, type ModelInfo, type ProviderInfo, type Session } from "./api.ts";

interface Props {
  providers: ProviderInfo[];
  onCreated: (s: Session) => void;
  onManageProviders: () => void;
  onError: (err: unknown) => void;
}

const SOURCE: Record<string, string> = {
  static: "known for this provider",
  reported: "reported by the server",
  probe: "tested",
  override: "set by you",
};

export default function NewSession({ providers, onCreated, onManageProviders, onError }: Props) {
  const [root, setRoot] = useState("");
  const [providerId, setProviderId] = useState(providers[0]?.id ?? "");
  const [model, setModel] = useState(providers[0]?.defaultModel ?? "");
  const [mode, setMode] = useState("ask");
  const [title, setTitle] = useState("");
  const [busy, setBusy] = useState(false);
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [modelsError, setModelsError] = useState<string | null>(null);
  const [caps, setCaps] = useState<ModelCaps | null>(null);
  const [capsState, setCapsState] = useState<"idle" | "loading" | "probing">("idle");
  const [capsError, setCapsError] = useState<string | null>(null);

  useEffect(() => {
    if (!providerId && providers[0]) {
      setProviderId(providers[0].id);
      setModel(providers[0].defaultModel);
    }
  }, [providers]);

  // Discover the provider's models (PRV-003).
  useEffect(() => {
    if (!providerId) return;
    setModels([]);
    setModelsError(null);
    EngineService.TestProvider(providerId)
      .then((list) => setModels(list ?? []))
      .catch((err) => setModelsError(errorText(err)));
  }, [providerId]);

  const loadCaps = (probe: boolean) => {
    if (!providerId || !model.trim()) {
      setCaps(null);
      return;
    }
    setCapsState(probe ? "probing" : "loading");
    setCapsError(null);
    EngineService.ModelCapabilities(providerId, model.trim(), probe)
      .then(setCaps)
      .catch((err) => {
        setCaps(null);
        setCapsError(errorText(err));
      })
      .finally(() => setCapsState("idle"));
  };

  // What the chosen model can do (PRV-005), after typing settles.
  useEffect(() => {
    const t = setTimeout(() => loadCaps(false), 400);
    return () => clearTimeout(t);
  }, [providerId, model]);

  const pickProvider = (id: string) => {
    setProviderId(id);
    setModel(providers.find((p) => p.id === id)?.defaultModel ?? "");
  };

  const override = (tools: boolean | null) =>
    EngineService.SetToolsOverride(providerId, model.trim(), tools)
      .then(() => loadCaps(false))
      .catch(onError);

  const choose = () =>
    EngineService.ChooseFolder()
      .then((p) => p && setRoot(p))
      .catch(onError);

  const create = (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    EngineService.CreateSession({ title, projectRoot: root, providerId, model: model.trim(), mode })
      .then(onCreated)
      .catch(onError)
      .finally(() => setBusy(false));
  };

  if (providers.length === 0) {
    return (
      <section className="page">
        <h2>New session</h2>
        <p>Add a model provider first.</p>
        <button className="primary" onClick={onManageProviders}>
          Add a provider
        </button>
      </section>
    );
  }

  const blocked = caps !== null && !caps.tools;

  return (
    <section className="page">
      <h2>New session</h2>
      <form className="card form" onSubmit={create}>
        <label>
          Project folder
          <div className="row">
            <input value={root} onChange={(e) => setRoot(e.target.value)} placeholder="/Users/you/code/project" required />
            <button type="button" onClick={choose}>
              Choose…
            </button>
          </div>
        </label>
        <label>
          Provider
          <select value={providerId} onChange={(e) => pickProvider(e.target.value)}>
            {providers.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Model
          <input list="models" value={model} onChange={(e) => setModel(e.target.value)} placeholder="Pick or type a model ID" required />
          <datalist id="models">
            {models.map((m) => (
              <option key={m.id} value={m.id}>
                {m.displayName && m.displayName !== m.id ? m.displayName : undefined}
              </option>
            ))}
          </datalist>
          {modelsError && <span className="error-text">Could not list models: {modelsError}</span>}
          {!modelsError && models.length > 0 && <span className="muted">{models.length} models available</span>}
        </label>

        <div className="caps" role="status">
          {capsState === "loading" && <span className="muted">Checking the model…</span>}
          {capsState === "probing" && <span className="muted">Testing tool calling; local models may need a minute to load…</span>}
          {capsState === "idle" && capsError && <span className="error-text">{capsError}</span>}
          {capsState === "idle" && caps && caps.source === "unknown" && (
            <span>
              Tool support is not known yet; it is tested when the session starts.{" "}
              <button type="button" onClick={() => loadCaps(true)}>
                Test now
              </button>
            </span>
          )}
          {capsState === "idle" && caps && caps.source !== "unknown" && caps.tools && (
            <span className="ok">
              Tool calling supported ({SOURCE[caps.source] ?? caps.source})
              {caps.contextWindow > 0 && ` · ${caps.contextWindow.toLocaleString()} token context`}
              {caps.source === "override" && (
                <>
                  {" "}
                  <button type="button" onClick={() => override(null)}>
                    Undo override
                  </button>
                </>
              )}
            </span>
          )}
          {capsState === "idle" && blocked && caps.source !== "unknown" && (
            <span className="error-text">
              This model cannot be used by the built-in harness: no tool calling ({SOURCE[caps.source] ?? caps.source}).{" "}
              <button type="button" onClick={() => override(caps.source === "override" ? null : true)}>
                {caps.source === "override" ? "Undo override" : "Use anyway"}
              </button>
            </span>
          )}
        </div>

        <label>
          Permission mode
          <select value={mode} onChange={(e) => setMode(e.target.value)}>
            {MODES.filter((m) => m.id !== "bypass").map((m) => (
              <option key={m.id} value={m.id}>
                {m.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Title <span className="muted">(optional)</span>
          <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="New session" />
        </label>
        <div className="row">
          <button type="submit" className="primary" disabled={busy || !root || !model.trim() || (blocked && caps?.source !== "unknown")}>
            {busy ? "Starting…" : "Start session"}
          </button>
        </div>
      </form>
    </section>
  );
}
