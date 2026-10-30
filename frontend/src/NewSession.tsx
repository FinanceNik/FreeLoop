import { useEffect, useState, type FormEvent } from "react";
import {
  EngineService,
  MODES,
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
} from "./api.ts";
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
  onCreated: (s: Session) => void;
  onProjectsChanged: () => void;
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

export default function NewSession({ providers, projects, environments, profiles, harnesses, sessions, defaults, initialRoot, onCreated, onProjectsChanged, onManageProviders, onProvidersChanged, onError }: Props) {
  // SET-004: the configured defaults, else the first provider.
  const initial = providers.find((p) => p.id === defaults?.providerId) ?? providers.find((p) => !p.builtin) ?? providers[0];
  const [root, setRoot] = useState(initialRoot ?? "");
  const [environmentId, setEnvironmentId] = useState(defaults?.environmentId ?? "");
  const [mode, setMode] = useState("");
  const [profileId, setProfileId] = useState(defaults?.profileId || "default");
  const [makeDefault, setMakeDefault] = useState(false);
  // An own worktree (SES-010), and whether to remember the choice for the
  // project (OQ-008).
  const [worktree, setWorktree] = useState(false);
  const [rememberWorktree, setRememberWorktree] = useState(false);
  const [providerId, setProviderId] = useState(initial?.id ?? "");
  const [model, setModel] = useState((initial && initial.id === defaults?.providerId && defaults?.model) || initial?.defaultModel || "");
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

  // A known project preselects its default environment.
  const project = projects.find((p) => p.root === root);
  useEffect(() => {
    if (!project) return;
    setEnvironmentId(project.defaultEnvironmentId || defaults?.environmentId || "local");
    pickProfile(project.defaultProfileId || defaults?.profileId || "default");
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
  const harness = harnesses.find((h) => h.id === (profile?.harness || "builtin"));
  const usesProvider = !harness || harness.usesProvider;
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
    setBusy(true);
    const own = worktree && !!settings?.trusted;
    EngineService.CreateSession({ title, projectRoot: root, environmentId, profileId, providerId: usesProvider ? providerId : "", model: usesProvider ? model.trim() : "", mode, worktree: own })
      .then(async (s) => {
        if (rememberWorktree && s.projectId) await EngineService.SetProjectWorktreeDefault(s.projectId, own ? "always" : "never").catch(onError);
        if (makeDefault && s.projectId) await EngineService.SetProjectProfile(s.projectId, profileId).catch(onError);
        onCreated(s);
      })
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
        {(!harness || harness.usesProvider) && (
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
        </label>
        <label>
          Permission mode
          <select value={mode} onChange={(e) => setMode(e.target.value)}>
            <option value="">
              {profile?.mode ? "From the profile" : "From the environment"} ({MODES.find((m) => m.id === (profile?.mode || env?.defaultMode || "ask"))?.name})
            </option>
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
          <button type="submit" className="primary" disabled={busy || !root || !!harness?.missing || (usesProvider && (!model.trim() || (blocked && caps?.source !== "unknown")))}>
            {busy ? "Starting…" : "Start session"}
          </button>
        </div>
      </form>
    </section>
  );
}
