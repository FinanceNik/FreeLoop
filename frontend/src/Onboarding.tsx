import { useState } from "react";
import { secretStore } from "./platform.ts";
import { EngineService, type Config, type HarnessInfo, type ProviderInfo } from "./api.ts";
import Providers from "./Providers.tsx";
import ClaudeImport from "./ClaudeImport.tsx";
import { t } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";

// The files and the command a trusted project's sessions use, named as they are.
const AGENTS_MD = "AGENTS.md";
const CLAUDE_MD = "CLAUDE.md";
const SETTINGS_DIR = ".freeloop/";
const GIT_STATUS = "git status";

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
  const chooseAgent = (h: HarnessInfo) =>
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
      <h2>{t("onboarding.welcome")}</h2>
      <ol className="steps" aria-label={t("onboarding.steps")}>
        <li className={step === "provider" ? "step-on" : "step-done"}>{t("onboarding.step.agents")}</li>
        <li className={step === "import" ? "step-on" : step === "project" ? "step-done" : ""}>{t("onboarding.step.import")}</li>
        <li className={step === "project" ? "step-on" : ""}>{t("onboarding.step.project")}</li>
        <li>{t("onboarding.step.start")}</li>
      </ol>

      {step === "provider" && (
        <>
          {agents.length > 0 && (
            <section className="choice" aria-labelledby="choice-agent">
              <h3 id="choice-agent">{t("onboarding.agent.title")}</h3>
              <p className="muted">{t("onboarding.agent.intro")}</p>
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
                      <button className="primary" onClick={() => chooseAgent(h)}>
                        {t("onboarding.agent.use", { name: h.name })}
                      </button>
                    )}
                  </li>
                ))}
              </ul>
            </section>
          )}
          <details className="choice" open={hasProvider || agents.length === 0}>
            <summary>
              <h3>{t("onboarding.provider.title")}</h3>
              <span className="muted">{t("onboarding.provider.intro", { store: secretStore() })}</span>
            </summary>
            <Providers providers={providers} onChanged={onProvidersChanged} onError={onError} />
          </details>
          <div className="row onboarding-nav">
            <button className="primary" disabled={!ready} onClick={() => setStep("import")}>
              {t("onboarding.next")}
            </button>
            {!ready && <span className="muted">{t("onboarding.notReady")}</span>}
            <button className="link" onClick={() => finish("")} disabled={busy}>
              {t("onboarding.skip")}
            </button>
          </div>
          <div className="demo-offer">
            <button onClick={tryDemo} disabled={busy}>
              {t("onboarding.demo")}
            </button>
            <p className="muted">{t("onboarding.demoNote")}</p>
          </div>
        </>
      )}

      {step === "import" && (
        <>
          <p>{t("onboarding.import.intro")}</p>
          <ClaudeImport onError={onError} />
          <div className="row onboarding-nav">
            <button className="primary" onClick={() => setStep("project")}>
              {t("onboarding.nextProject")}
            </button>
            <button onClick={() => setStep("provider")}>{t("common.back")}</button>
          </div>
        </>
      )}

      {step === "project" && (
        <div className="card form">
          <p>{t("onboarding.project.intro")}</p>
          <label>
            {t("newSession.folder")}
            <div className="row">
              <input value={root} onChange={(e) => setRoot(e.target.value)} placeholder={t("newSession.folderPlaceholder")} />
              <button type="button" onClick={choose}>
                {t("newSession.choose")}
              </button>
            </div>
          </label>
          <label className="row">
            <input type="checkbox" checked={trust} onChange={(e) => setTrust(e.target.checked)} />
            {t("trust.palette")}
          </label>
          <p className="muted">
            {tNodes("onboarding.project.trustNote", {
              agents: <code>{AGENTS_MD}</code>,
              claude: <code>{CLAUDE_MD}</code>,
              settings: <code>{SETTINGS_DIR}</code>,
              git: <code>{GIT_STATUS}</code>,
            })}
          </p>
          <div className="row onboarding-nav">
            <button className="primary" disabled={!root.trim() || busy} onClick={() => finish(root.trim())}>
              {t("onboarding.nextStart")}
            </button>
            <button onClick={() => setStep("import")}>{t("common.back")}</button>
            <button className="link" onClick={() => finish("")} disabled={busy}>
              {t("onboarding.skip")}
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
