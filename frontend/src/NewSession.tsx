import { useEffect, useState, type FormEvent } from "react";
import { EngineService, MODES, type ProviderInfo, type Session } from "./api.ts";

interface Props {
  providers: ProviderInfo[];
  onCreated: (s: Session) => void;
  onManageProviders: () => void;
  onError: (err: unknown) => void;
}

export default function NewSession({ providers, onCreated, onManageProviders, onError }: Props) {
  const [root, setRoot] = useState("");
  const [providerId, setProviderId] = useState(providers[0]?.id ?? "");
  const [model, setModel] = useState(providers[0]?.defaultModel ?? "");
  const [mode, setMode] = useState("ask");
  const [title, setTitle] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!providerId && providers[0]) {
      setProviderId(providers[0].id);
      setModel(providers[0].defaultModel);
    }
  }, [providers]);

  const pickProvider = (id: string) => {
    setProviderId(id);
    setModel(providers.find((p) => p.id === id)?.defaultModel ?? "");
  };

  const choose = () =>
    EngineService.ChooseFolder()
      .then((p) => p && setRoot(p))
      .catch(onError);

  const create = (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    EngineService.CreateSession({ title, projectRoot: root, providerId, model, mode })
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
          <input value={model} onChange={(e) => setModel(e.target.value)} required />
        </label>
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
          <button type="submit" className="primary" disabled={busy || !root}>
            Start session
          </button>
        </div>
      </form>
    </section>
  );
}
