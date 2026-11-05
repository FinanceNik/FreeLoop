import { useEffect, useState, type FormEvent } from "react";
import WorktreeScripts from "./WorktreeScripts.tsx";
import IssuePicker from "./IssuePicker.tsx";
import {
  EngineService,
  type IssueBrief,
  MODES,
  STARTING_MODES,
  errorText,
  type EnvironmentInfo,
  type HarnessInfo,
  type ModelCaps,
  type ModelInfo,
  type Profile,
  type ProjectInfo,
  type ProjectSettings,
  type ProviderInfo,
  type Session,
  type OptimizerStatus,
  type StartWaits,
} from "./api.ts";
import { inheritedLabel, OPTIMIZER_OFF } from "./optimizers.ts";
import { orderModels } from "./models.ts";
import type { Defaults } from "../bindings/github.com/FinanceNik/Freeloop/internal/config/models";

interface Props {
  providers: ProviderInfo[];
  projects: ProjectInfo[];
  environments: EnvironmentInfo[];
  profiles: Profile[];
  harnesses: HarnessInfo[];
  sessions: Session[];
  defaults?: Defaults;
  // A folder chosen beforehand, by the first-run guide (GUI-010).
  initialRoot?: string;
  // draft is the first message to put in the session's box, from an
  // issue (SES-017).
  onCreated: (s: Session, draft?: string) => void;
  // A comparison of one task on several profiles started (HRN-016).
  onCompare: (tag: string) => void;
  onProjectsChanged: () => void;
  onImport?: () => void;
  onManageProviders: () => void;
  onProvidersChanged: () => void;
  onError: (err: unknown) => void;
}

const SOURCE: Record<string, string> = {
  static: "known for this provider",
  reported: "reported by the server",
  probe: "tested",
  override: "set by you",
};

export default function NewSession({ providers, projects, environments, profiles, harnesses, sessions, defaults, initialRoot, onCreated, onCompare, onProjectsChanged, onImport, onManageProviders, onProvidersChanged, onError }: Props) {
  // SET-004: the configured defaults, else the first provider.
  const initial = providers.find((p) => p.id === defaults?.providerId) ?? providers.find((p) => !p.builtin) ?? providers[0];
  // An agent that signs in by itself (Claude Code with a subscription)
  // needs no provider; without one, the form starts with such an agent.
  const readyAgent = harnesses.find((h) => h.kind === "acp" && !h.missing);
  const fallbackProfile = providers.length === 0 && readyAgent ? readyAgent.id : "default";
  const [root, setRoot] = useState(initialRoot ?? "");
  const [environmentId, setEnvironmentId] = useState(defaults?.environmentId ?? "");
  const [mode, setMode] = useState("");
  const [profileId, setProfileId] = useState(defaults?.profileId || fallbackProfile);
  const [makeDefault, setMakeDefault] = useState(false);
  // An own worktree (SES-010), and whether to remember the choice for the
  // project (OQ-008).
  const [worktree, setWorktree] = useState(false);
  const [rememberWorktree, setRememberWorktree] = useState(false);
  const [providerId, setProviderId] = useState(initial?.id ?? "");
  const [model, setModel] = useState((initial && initial.id === defaults?.providerId && defaults?.model) || initial?.defaultModel || "");
  const [title, setTitle] = useState("");
  // The GitHub issue the session starts from (SES-017).
  const [issue, setIssue] = useState<IssueBrief | null>(null);
  const [pickingIssue, setPickingIssue] = useState(false);
  // Other profiles the same task runs on, to compare (HRN-016), and the
  // task itself, which a comparison sends at once.
  const [extra, setExtra] = useState<string[]>([]);
  const [task, setTask] = useState("");
  const [busy, setBusy] = useState(false);
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [modelsError, setModelsError] = useState<string | null>(null);
  const [caps, setCaps] = useState<ModelCaps | null>(null);
  const [capsState, setCapsState] = useState<"idle" | "loading" | "probing">("idle");
  const [capsError, setCapsError] = useState<string | null>(null);
  // The token optimizer (HRN-019): empty takes the profile's or the default.
  const [optimizer, setOptimizer] = useState("");
  const [optimizers, setOptimizers] = useState<OptimizerStatus[]>([]);
  useEffect(() => {
    EngineService.ListOptimizers()
      .then((l) => setOptimizers(l ?? []))
      .catch(onError);
  }, []);

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

  // A known project preselects its default environment.
  const project = projects.find((p) => p.root === root);
  useEffect(() => {
    if (!project) return;
    setEnvironmentId(project.defaultEnvironmentId || defaults?.environmentId || "local");
    pickProfile(project.defaultProfileId || defaults?.profileId || fallbackProfile);
    setMakeDefault(false);
    setRememberWorktree(false);
    setWorktree(project.worktreeDefault === "always" || (project.worktreeDefault !== "never" && sessionsIn(project.id) > 0));
  }, [project?.id]);
  const sessionsIn = (projectId: string) => sessions.filter((s) => s.projectId === projectId && s.status !== "archived").length;
  const others = project ? sessionsIn(project.id) : 0;
  // Trust and .freeloop/ settings of the chosen folder (SEC-006, SET-002).
  const [settings, setSettings] = useState<ProjectSettings | null>(null);
  const loadSettings = () => {
    if (!root.trim()) return setSettings(null);
    EngineService.ProjectSettings(root.trim())
      .then(setSettings)
      .catch(() => setSettings(null));
  };
  useEffect(() => {
    setSettings(null);
    const t = setTimeout(loadSettings, 300);
    return () => clearTimeout(t);
  }, [root]);
  const trust = () =>
    EngineService.SetProjectTrust(root.trim(), true)
      .then(() => {
        loadSettings();
        onProjectsChanged();
      })
      .catch(onError);
  const projectProfiles = settings?.profiles ?? [];
  const allProfiles = [...profiles, ...projectProfiles];
  const profile = allProfiles.find((p) => p.id === profileId);
  // What a file chose and the user has not approved does not apply yet (SET-005).
  const [waits, setWaits] = useState<StartWaits | null>(null);
  useEffect(() => {
    EngineService.StartWaiting(profileId)
      .then(setWaits)
      .catch(() => setWaits(null));
  }, [profileId, JSON.stringify(defaults)]);
  const harness = harnesses.find((h) => h.id === (profile?.harness || "builtin"));
  const usesProvider = !harness || harness.usesProvider;
  // The first profile that runs on a provider, to offer from one that does not.
  const builtinProfile = [...profiles, ...projectProfiles].find((p) => harnesses.find((h) => h.id === (p.harness || "builtin"))?.usesProvider);
  const env = environments.find((e) => e.id === (environmentId || "local"));

  // Favorites and hidden models (PRV-003).
  const current = providers.find((p) => p.id === providerId);
  const favorites = current?.favorites ?? [];
  const hidden = current?.hidden ?? [];
  const [showHidden, setShowHidden] = useState(false);
  const listed = orderModels(
    models.map((m) => m.id),
    favorites,
    hidden,
    showHidden,
  );
  const names = new Map(models.map((m) => [m.id, m.displayName]));
  const chosen = model.trim();
  const flag = (favorite: boolean, hide: boolean) =>
    EngineService.SetModelFlags(providerId, chosen, favorite, hide).then(onProvidersChanged).catch(onError);

  const pickProvider = (id: string) => {
    setProviderId(id);
    setModel(providers.find((p) => p.id === id)?.defaultModel ?? "");
  };

  // A profile (HRN-003) brings its provider and model; the fields stay
  // editable.
  const pickProfile = (id: string) => {
    setProfileId(id);
    const p = [...profiles, ...(settings?.profiles ?? [])].find((x) => x.id === id);
    if (!p) return;
    const prov = providers.find((x) => x.id === p.provider);
    if (prov) {
      setProviderId(prov.id);
      setModel(p.model || prov.defaultModel);
    } else if (p.model) {
      setModel(p.model);
    }
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
    if (extra.length > 0) return compare();
    setBusy(true);
    const own = worktree && !!settings?.trusted;
    EngineService.CreateSession({ title, projectRoot: root, environmentId, profileId, providerId: usesProvider ? providerId : "", model: usesProvider ? model.trim() : "", mode, worktree: own, optimizer: usesProvider || harness?.takesOptimizer ? optimizer : "" })
      .then(async (s) => {
        if (rememberWorktree && s.projectId) await EngineService.SetProjectWorktreeDefault(s.projectId, own ? "always" : "never").catch(onError);
        if (makeDefault && s.projectId) await EngineService.SetProjectProfile(s.projectId, profileId).catch(onError);
        if (issue) await EngineService.SetTags(s.id, [issue.tag]).catch(onError);
        onCreated(s, issue?.text);
      })
      .catch(onError)
      .finally(() => setBusy(false));
  };

  // A built-in profile without its own provider and model runs on the
  // form's; an external agent brings its own.
  const runOf = (p: Profile) => {
    const h = harnesses.find((x) => x.id === (p.harness || "builtin"));
    const own = !h || h.usesProvider;
    return { profileId: p.id, label: p.name, providerId: own ? p.provider || providerId : "", model: own ? p.model || model.trim() : "" };
  };
  const compare = () => {
    const chosen = allProfiles.find((p) => p.id === profileId);
    if (!chosen) return;
    setBusy(true);
    EngineService.CreateCompare({
      title,
      projectRoot: root,
      environmentId,
      mode,
      prompt: task,
      profiles: [chosen, ...extra.map((id) => allProfiles.find((p) => p.id === id)).filter((p): p is Profile => !!p)].map(runOf),
    })
      .then(onCompare)
      .catch(onError)
      .finally(() => setBusy(false));
  };

  if (providers.length === 0 && !readyAgent) {
    return (
      <section className="page">
        <h2>New session</h2>
        <p>Add a model provider, or install an agent that signs in by itself, such as Claude Code.</p>
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
        {projects.length > 0 && (
          <label>
            Project
            <select value={project?.root ?? ""} onChange={(e) => setRoot(e.target.value)}>
              <option value="">Another folder…</option>
              {projects
                .filter((p) => !p.missing)
                .map((p) => (
                  <option key={p.id} value={p.root}>
                    {p.name}
                  </option>
                ))}
            </select>
          </label>
        )}
        <label>
          Project folder
          <div className="row">
            <input value={root} onChange={(e) => setRoot(e.target.value)} placeholder="/Users/you/code/project" required />
            <button type="button" onClick={choose}>
              Choose…
            </button>
          </div>
        </label>
        {root.trim() && settings && !settings.trusted && (
          <div className="banner warn trust-box" role="status">
            <strong>Do you trust this folder?</strong>
            <p>
              Until you do, sessions here use your global settings only: Freeloop does not read its <code>AGENTS.md</code> or{" "}
              <code>CLAUDE.md</code>, its <code>.freeloop/</code> settings, run its hooks, or run <code>git status</code> in it. Trust only folders whose
              contents you know.
            </p>
            {(settings.hooks ?? []).length > 0 && (
              <div>
                <strong>Trusting it lets it run these commands (hooks):</strong>
                <ul>
                  {(settings.hooks ?? []).map((h, n) => (
                    <li key={n}>
                      {h.event}
                      {h.match ? ` (${h.match})` : ""}: <code>{h.command}</code> <span className="muted">{h.source}</span>
                    </li>
                  ))}
                </ul>
              </div>
            )}
            {(settings.worktree?.setup || settings.worktree?.run || settings.worktree?.archive || (settings.worktree?.copy ?? []).length > 0) && (
              <div>
                <strong>Its worktree sessions would do this (.freeloop/worktree.yaml):</strong>
                <ul>
                  {(settings.worktree?.copy ?? []).length > 0 && (
                    <li>
                      copy into each worktree: {(settings.worktree?.copy ?? []).map((c) => <code key={c}>{c} </code>)}
                    </li>
                  )}
                  {(["setup", "run", "archive"] as const)
                    .filter((k) => settings.worktree?.[k])
                    .map((k) => (
                      <li key={k}>
                        {k}: <code>{settings.worktree![k]}</code>
                      </li>
                    ))}
                </ul>
              </div>
            )}
            <div className="row">
              <button type="button" className="primary" onClick={trust}>
                Trust this project
              </button>
              <span className="muted">or start without trusting it.</span>
            </div>
          </div>
        )}
        {settings?.trusted && (settings.hooks ?? []).length > 0 && (
          <div className="banner row" role="status">
            <span>
              This project runs hooks from <code>.freeloop/hooks.yaml</code>:
              <ul>
                {(settings.hooks ?? []).map((h, n) => (
                  <li key={n}>
                    {h.event}
                    {h.match ? ` (${h.match})` : ""}: <code>{h.command}</code>
                  </li>
                ))}
              </ul>
            </span>
          </div>
        )}
        {settings && (settings.issues ?? []).length > 0 && (
          <div className="banner warn" role="status">
            <strong>Problems in the project's .freeloop/ settings</strong>
            <ul>
              {(settings.issues ?? []).map((i, n) => (
                <li key={n}>
                  {i.file}
                  {i.line ? `:${i.line}` : ""}
                  {i.field ? ` (${i.field})` : ""}: {i.message}
                </li>
              ))}
            </ul>
          </div>
        )}
        <label>
          Profile
          <select value={profileId} onChange={(e) => pickProfile(e.target.value)}>
            {harnesses.flatMap((h) =>
              [false, true].map((fromProject) => {
                const list = (fromProject ? projectProfiles : profiles).filter((p) => (p.harness || "builtin") === h.id);
                if (list.length === 0) return null;
                return (
                  <optgroup key={h.id + fromProject} label={fromProject ? `${h.name} · this project's .freeloop/` : h.name}>
                    {list.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                        {project && project.defaultProfileId === p.id ? " (project default)" : ""}
                      </option>
                    ))}
                  </optgroup>
                );
              }),
            )}
          </select>
          {profile?.description && <span className="muted">{profile.description}</span>}
          {project && (project.defaultProfileId || "default") !== profileId && (
            <span className="row muted">
              <input type="checkbox" checked={makeDefault} onChange={(e) => setMakeDefault(e.target.checked)} /> Make it {project.name}'s default
            </span>
          )}
        </label>
        {harness?.missing && (
          <p className="error-text" role="alert">
            {harness.name} cannot start: {harness.missing}.
          </p>
        )}
        {harness && !harness.usesProvider && (
          <p className="muted harness-note">
            {harness.name} brings its own model access and sign-in; Freeloop passes it no credentials.{harness.notes ? ` ${harness.notes}` : ""}
          </p>
        )}
        {harness && !harness.usesProvider && builtinProfile && providers.length > 0 && (
          <p className="harness-note provider-offer">
            <span className="muted">
              {harness.name} cannot use {providers.length === 1 ? providers[0].name : "your providers"}. To choose a provider and model, run the session on {builtinProfile.name}.
            </span>{" "}
            <button type="button" onClick={() => pickProfile(builtinProfile.id)}>
              Use a provider
            </button>
          </p>
        )}
        {usesProvider && providers.length === 0 && (
          <p className="banner warn">
            This profile runs on a model provider, and none is set up.{" "}
            <button type="button" className="link" onClick={onManageProviders}>
              Add a provider
            </button>
            {readyAgent ? `, or choose the ${readyAgent.name} profile above.` : "."}
          </p>
        )}
        {(!harness || harness.usesProvider) && providers.length > 0 && (
          <>
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
            {listed.map((id) => {
              const name = names.get(id);
              const label = [favorites.includes(id) && "★", hidden.includes(id) && "hidden", name && name !== id && name].filter(Boolean).join(" ");
              return (
                <option key={id} value={id}>
                  {label || undefined}
                </option>
              );
            })}
          </datalist>
          {modelsError && <span className="error-text">Could not list models: {modelsError}</span>}
          {!modelsError && models.length > 0 && (
            <span className="muted">
              {models.length} models available
              {hidden.length > 0 && ` · ${hidden.length} hidden`}
            </span>
          )}
        </label>
        {!current?.builtin && (
          <div className="row wrap model-flags">
            {favorites.length > 0 && <span className="muted">Favorites:</span>}
            {favorites.map((f) => (
              <button type="button" key={f} className={f === chosen ? "chip chip-on" : "chip"} onClick={() => setModel(f)}>
                {f}
              </button>
            ))}
            {chosen && (
              <>
                <button
                  type="button"
                  aria-pressed={favorites.includes(chosen)}
                  onClick={() => flag(!favorites.includes(chosen), false)}
                  title="Favorites are listed first"
                >
                  {favorites.includes(chosen) ? "★ Unfavorite" : "☆ Favorite"}
                </button>
                <button type="button" onClick={() => flag(false, !hidden.includes(chosen))} title="Hidden models are left out of the list">
                  {hidden.includes(chosen) ? "Unhide" : "Hide"}
                </button>
              </>
            )}
            {hidden.length > 0 && (
              <label className="row">
                <input type="checkbox" checked={showHidden} onChange={(e) => setShowHidden(e.target.checked)} /> Show hidden
              </label>
            )}
          </div>
        )}

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
          </>
        )}

        {root.trim() && (
          <div className="worktree-choice">
            {others > 0 && project && !project.worktreeDefault && (
              <p className="muted">
                {others === 1 ? "Another session works" : `${others} other sessions work`} in {project.name}. Its own worktree keeps this one's edits apart.
              </p>
            )}
            <label className="row">
              <input type="checkbox" checked={worktree && !!settings?.trusted} disabled={!settings?.trusted} onChange={(e) => setWorktree(e.target.checked)} />
              Work in its own git worktree and branch
              {!settings?.trusted && <span className="muted"> (trust the project first)</span>}
            </label>
            {project && settings?.trusted && (
              <label className="row muted">
                <input type="checkbox" checked={rememberWorktree} onChange={(e) => setRememberWorktree(e.target.checked)} /> Remember for {project.name}
              </label>
            )}
            {project && settings?.trusted && worktree && <WorktreeScripts projectId={project.id} onError={onError} />}
          </div>
        )}
        <label>
          Environment
          <select value={environmentId || "local"} onChange={(e) => setEnvironmentId(e.target.value)}>
            {environments.map((e) => (
              <option key={e.id} value={e.id}>
                {e.name}
                {(e.secretNames ?? []).length > 0 ? ` (${(e.secretNames ?? []).length} secrets)` : ""}
              </option>
            ))}
          </select>
          {env?.sandbox?.enabled ? (
            <span className="muted">The built-in agent's commands run without asking, and can only write in the project. Leaving the sandbox always asks.</span>
          ) : (
            env?.kind === "local" && <span className="muted">Commands ask each time, and run with your full access. Choose a sandboxed environment to confine them to the project.</span>
          )}
        </label>
        {(usesProvider || harness?.takesOptimizer) && extra.length === 0 && (
          <label>
            Token optimizer
            <select value={optimizer} onChange={(e) => setOptimizer(e.target.value)}>
              <option value="">{waits?.optimizer ? "Off, until you approve the file's choice in Settings" : inheritedLabel(profile?.optimizer, defaults?.optimizer, optimizers)}</option>
              <option value={OPTIMIZER_OFF}>Off</option>
              {optimizers.map((o) => (
                <option key={o.id} value={o.id} disabled={o.needsApproval || (!o.path && env?.kind !== "docker")}>
                  {o.needsApproval ? `${o.name} (not approved)` : o.path || env?.kind === "docker" ? o.name : `${o.name} (not installed)`}
                </option>
              ))}
            </select>
          </label>
        )}
        <label>
          Permission mode
          <select value={mode} onChange={(e) => setMode(e.target.value)}>
            <option value="">
              {waits?.mode ? "Ask, until you approve the file's choice in Settings" : <>{profile?.mode ? "From the profile" : "From the environment"} ({MODES.find((m) => m.id === (profile?.mode || env?.defaultMode || "ask"))?.name})</>}
            </option>
            {STARTING_MODES.map((m) => (
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
        {settings?.trusted && allProfiles.length > 1 && (
          <details className="compare-choice" open={extra.length > 0}>
            <summary>Also run this task on other profiles, to compare</summary>
            <p className="muted">Each profile gets its own worktree; the task is sent to all of them at once, and a page shows the runs side by side.</p>
            <div className="row wrap">
              {allProfiles
                .filter((p) => p.id !== profileId)
                .map((p) => (
                  <label key={p.id} className="row">
                    <input
                      type="checkbox"
                      checked={extra.includes(p.id)}
                      disabled={!extra.includes(p.id) && extra.length >= 3}
                      onChange={(e) => setExtra(e.target.checked ? [...extra, p.id] : extra.filter((x) => x !== p.id))}
                    />
                    {p.name}
                  </label>
                ))}
            </div>
            {extra.length > 0 && (
              <label>
                Task <span className="muted">(sent to all {extra.length + 1} runs)</span>
                <textarea rows={4} value={task} onChange={(e) => setTask(e.target.value)} placeholder="Describe the change all runs should make." required />
              </label>
            )}
          </details>
        )}
        {settings?.trusted && !pickingIssue && extra.length === 0 && (
          <p className="muted">
            {issue ? (
              <>
                Starts from issue #{issue.number}; its text goes into the message box for you to edit and send.{" "}
                <button type="button" className="link" onClick={() => (setIssue(null), setTitle(""))}>
                  Remove
                </button>
              </>
            ) : (
              <button type="button" className="link" onClick={() => setPickingIssue(true)}>
                Start from a GitHub issue…
              </button>
            )}
          </p>
        )}
        {pickingIssue && (
          <IssuePicker
            root={root}
            onClose={() => setPickingIssue(false)}
            onPick={(b) => {
              setIssue(b);
              setTitle(b.sessionTitle);
              setPickingIssue(false);
            }}
          />
        )}
        <div className="row">
          <button type="submit" className="primary" disabled={busy || !root || !!harness?.missing || (usesProvider && (!model.trim() || (blocked && caps?.source !== "unknown")))}>
            {busy ? "Starting…" : extra.length > 0 ? `Start ${extra.length + 1} runs` : "Start session"}
          </button>
          {onImport && (
            <button type="button" className="link" onClick={onImport}>
              Import an existing session…
            </button>
          )}
        </div>
      </form>
    </section>
  );
}
