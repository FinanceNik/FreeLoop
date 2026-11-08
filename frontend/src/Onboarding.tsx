import { useState } from "react";
import { EngineService, type Config, type HarnessInfo, type ProviderInfo } from "./api.ts";
import Providers from "./Providers.tsx";
import ClaudeImport from "./ClaudeImport.tsx";

// Onboarding is the first-run guide (GUI-010): choose how agents run (an
// agent that signs in by itself, such as Claude Code with a subscription,
// or a provider with an API key or a local model), import from Claude Code
// if wanted, choose a first project and decide whether to trust it, then
// start a session. Agents are listed from their presets, never by name.

interface Props {
  config: Config;
  providers: ProviderInfo[];
  harnesses: HarnessInfo[];
  onProvidersChanged: () => void;
  onProjectsChanged: () => void;
  // Called with the chosen folder ("" when skipped); the app then opens the
  // new-session form.
  onDone: (root: string) => void;
  // Called with the demo session (GUI-045) once it exists and the guide is done.
  onDemo: (sessionId: string) => void;
  onSaved: (c: Config) => void;
  onError: (err: unknown) => void;
}

type Step = "provider" | "import" | "project";

export default function Onboarding({ config, providers, harnesses, onProvidersChanged, onProjectsChanged, onDone, onDemo, onSaved, onError }: Props) {
  const [step, setStep] = useState<Step>("provider");
  const [root, setRoot] = useState("");
  const [trust, setTrust] = useState(false);
  const [busy, setBusy] = useState(false);
  const agents = harnesses.filter((h) => h.kind === "acp");
  const hasProvider = providers.some((p) => !p.builtin);
  const ready = hasProvider || agents.some((h) => !h.missing);
  // Choosing an agent makes it the default for new sessions.
  const useAgent = (h: HarnessInfo) =>
    EngineService.SaveSettings({ ...config, defaults: { ...config.defaults, profileId: h.id } })
      .then((c) => {
        onSaved(c);
        setStep("import");
      })
      .catch(onError);

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

  // The demo needs no setup, so it ends the guide as finishing does.
  const tryDemo = () => {
    setBusy(true);
    EngineService.StartDemo()
      .then((s) => {
        onProjectsChanged();
        return EngineService.SaveSettings({ ...config, ui: { ...config.ui, onboarded: true } }).then((c) => {
          onSaved(c);
          onDemo(s.id);
        });
      })
      .catch(onError)
      .finally(() => setBusy(false));
  };

  const choose = () =>
    EngineService.ChooseFolder()
      .then((p) => p && setRoot(p))
      .catch(onError);

  return (
    <section className="page onboarding">
      <h2>Welcome to Freeloop</h2>
      <ol className="steps" aria-label="Setup steps">
        <li className={step === "provider" ? "step-on" : "step-done"}>Choose how agents run</li>
        <li className={step === "import" ? "step-on" : step === "project" ? "step-done" : ""}>Import from Claude Code (optional)</li>
        <li className={step === "project" ? "step-on" : ""}>Choose a project</li>
        <li>Start a session</li>
      </ol>

      {step === "provider" && (
        <>
          {agents.length > 0 && (
            <section className="choice" aria-labelledby="choice-agent">
              <h3 id="choice-agent">Use an agent you're signed in to</h3>
              <p className="muted">It runs with its own account. Freeloop starts it on your Mac and never sees the login.</p>
              <ul className="agent-choices">
                {agents.map((h) => (
                  <li key={h.id} className="agent-choice">
                    <div>
                      <strong>{h.name}</strong>
                      {h.description && <p>{h.description}</p>}
                      {h.notes && <p className="muted">{h.notes}</p>}
                    </div>
                    {h.missing ? (
                      <span className="error-text">{h.missing}</span>
                    ) : (
                      <button className="primary" onClick={() => useAgent(h)}>
                        Use {h.name}
                      </button>
                    )}
                  </li>
                ))}
              </ul>
            </section>
          )}
          <details className="choice" open={hasProvider || agents.length === 0}>
            <summary>
              <h3>Use an API key or a local model</h3>
              <span className="muted">Anthropic, OpenAI, Google Gemini, OpenRouter, or a server on this Mac such as Ollama or LM Studio. Keys go to the macOS Keychain.</span>
            </summary>
            <Providers providers={providers} onChanged={onProvidersChanged} onError={onError} />
          </details>
          <div className="row onboarding-nav">
            <button className="primary" disabled={!ready} onClick={() => setStep("import")}>
              Next
            </button>
            {!ready && <span className="muted">Choose an agent or add a provider to continue.</span>}
            <button className="link" onClick={() => finish("")} disabled={busy}>
              Skip the guide
            </button>
          </div>
          <div className="demo-offer">
            <button onClick={tryDemo} disabled={busy}>
              Try it with a demo
            </button>
            <p className="muted">A scripted agent shows what Freeloop allows, asks about and refuses, in a scratch project. No key needed.</p>
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
