import { useState } from "react";
import { EngineService, type Config, type HarnessInfo, type ProviderInfo } from "./api.ts";
import Providers from "./Providers.tsx";
import ClaudeImport from "./ClaudeImport.tsx";

// Onboarding is the first-run guide (GUI-010): add and test a provider,
// choose a first project and decide whether to trust it, then start a
// session. Importing Claude Code skills and MCP servers joins it with
// SKL-003 and MCP-002.

interface Props {
  config: Config;
  providers: ProviderInfo[];
  harnesses: HarnessInfo[];
  onProvidersChanged: () => void;
  onProjectsChanged: () => void;
  // Called with the chosen folder ("" when skipped); the app then opens the
  // new-session form.
  onDone: (root: string) => void;
  onSaved: (c: Config) => void;
  onError: (err: unknown) => void;
}

type Step = "provider" | "import" | "project";

export default function Onboarding({ config, providers, harnesses, onProvidersChanged, onProjectsChanged, onDone, onSaved, onError }: Props) {
  const [step, setStep] = useState<Step>("provider");
  const [root, setRoot] = useState("");
  const [trust, setTrust] = useState(false);
  const [busy, setBusy] = useState(false);
  const ready = providers.some((p) => !p.builtin) || harnesses.some((h) => h.kind === "acp" && !h.missing);

  // Finishing or skipping is remembered, so the guide shows once.
  const finish = (folder: string) => {
    setBusy(true);
    const done = () =>
      EngineService.SaveSettings({ ...config, ui: { ...config.ui, onboarded: true } })
        .then((c) => {
          onSaved(c);
          onDone(folder);
        })
        .catch(onError)
        .finally(() => setBusy(false));
    if (!folder) return done();
    const p = trust ? EngineService.SetProjectTrust(folder, true) : EngineService.AddProject(folder);
    p.then(onProjectsChanged)
      .then(done)
      .catch((err) => {
        setBusy(false);
        onError(err);
      });
  };

  const choose = () =>
    EngineService.ChooseFolder()
      .then((p) => p && setRoot(p))
      .catch(onError);

  return (
    <section className="page onboarding">
      <h2>Welcome to Freeloop</h2>
      <ol className="steps" aria-label="Setup steps">
        <li className={step === "provider" ? "step-on" : "step-done"}>Add a model provider</li>
        <li className={step === "import" ? "step-on" : step === "project" ? "step-done" : ""}>Import from Claude Code (optional)</li>
        <li className={step === "project" ? "step-on" : ""}>Choose a project</li>
        <li>Start a session</li>
      </ol>

      {step === "provider" && (
        <>
          <p>
            Freeloop runs agents with your own models: an API key for Anthropic, OpenAI or OpenRouter, or a local server such as Ollama or LM
            Studio. Keys go to the macOS Keychain. Add one and test it.
          </p>
          <Providers providers={providers} onChanged={onProvidersChanged} onError={onError} />
          {harnesses.some((h) => h.kind === "acp") && (
            <div className="card">
              <strong>Agents that bring their own model access</strong>
              <ul className="agent-status">
                {harnesses
                  .filter((h) => h.kind === "acp")
                  .map((h) => (
                    <li key={h.id}>
                      {h.name}: {h.missing ? <span className="error-text">{h.missing}</span> : <span className="ok">ready to start</span>}
                      {h.notes && <div className="muted">{h.notes}</div>}
                    </li>
                  ))}
              </ul>
              <p className="muted">They need no provider here; you can skip adding one and continue with the guide.</p>
            </div>
          )}
          <div className="row onboarding-nav">
            <button className="primary" disabled={!ready} onClick={() => setStep("import")}>
              Next
            </button>
            {!ready && <span className="muted">Add a provider to continue.</span>}
            <button className="link" onClick={() => finish("")} disabled={busy}>
              Skip the guide
            </button>
          </div>
        </>
      )}

      {step === "import" && (
        <>
          <p>If you use Claude Code, you can bring its MCP servers, permission rules and skills along. This only reads its files.</p>
          <ClaudeImport onError={onError} />
          <div className="row onboarding-nav">
            <button className="primary" onClick={() => setStep("project")}>
              Next: choose a project
            </button>
            <button onClick={() => setStep("provider")}>Back</button>
          </div>
        </>
      )}

      {step === "project" && (
        <div className="card form">
          <p>Pick the folder of a repository you want an agent to work in.</p>
          <label>
            Project folder
            <div className="row">
              <input value={root} onChange={(e) => setRoot(e.target.value)} placeholder="/Users/you/code/project" />
              <button type="button" onClick={choose}>
                Choose…
              </button>
            </div>
          </label>
          <label className="row">
            <input type="checkbox" checked={trust} onChange={(e) => setTrust(e.target.checked)} />
            Trust this project
          </label>
          <p className="muted">
            A trusted project's <code>AGENTS.md</code> or <code>CLAUDE.md</code>, its <code>.freeloop/</code> settings and <code>git status</code> are
            used. Trust only folders whose contents you know; you can trust it later from the session.
          </p>
          <div className="row onboarding-nav">
            <button className="primary" disabled={!root.trim() || busy} onClick={() => finish(root.trim())}>
              Next: start a session
            </button>
            <button onClick={() => setStep("import")}>Back</button>
            <button className="link" onClick={() => finish("")} disabled={busy}>
              Skip the guide
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
