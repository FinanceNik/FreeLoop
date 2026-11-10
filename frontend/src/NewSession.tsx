import ContextLimit from "./ContextLimit.tsx";
import { useEffect, useState, type FormEvent } from "react";
import WorktreeScripts from "./WorktreeScripts.tsx";
import IssuePicker from "./IssuePicker.tsx";
import {
  EngineService,
  type IssueBrief,
  modeName,
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
  type WorkflowInfo,
  type StartWaits,
} from "./api.ts";
import { inheritedLabel, OPTIMIZER_OFF } from "./optimizers.ts";
import { orderModels } from "./models.ts";
import { trustLines } from "./trust.ts";
import type { Defaults } from "../bindings/github.com/FinanceNik/Freeloop/internal/config/models";
import { t, type PlainKey } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";

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
  // A workflow chosen beforehand, from "Start a workflow…" (WFL-002): the new
  // session's box then holds its first stage's command.
  initialWorkflow?: string;
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

// Where what a model can do is known from; a source the app does not know shows as given.
const SOURCE: Record<string, PlainKey> = {
  static: "newSession.caps.source.static",
  reported: "newSession.caps.source.reported",
  probe: "newSession.caps.source.probe",
  override: "newSession.caps.source.override",
};

const sourceLabel = (source: string) => (SOURCE[source] ? t(SOURCE[source]) : source);

// The files and the command an untrusted folder's sessions go without, named as
// they are, and the project files that say what a trusted one runs.
const AGENTS_MD = "AGENTS.md";
const CLAUDE_MD = "CLAUDE.md";
const SETTINGS_DIR = ".freeloop/";
const GIT_STATUS = "git status";
const WORKTREE_YAML = ".freeloop/worktree.yaml";
const HOOKS_YAML = ".freeloop/hooks.yaml";

export default function NewSession({ providers, projects, environments, profiles, harnesses, sessions, defaults, initialRoot, initialWorkflow, onCreated, onCompare, onProjectsChanged, onImport, onManageProviders, onProvidersChanged, onError }: Props) {
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
  // The workflow (WFL-002): empty takes the profile's or the default.
  const [workflow, setWorkflow] = useState(initialWorkflow ?? "");
  const [workflows, setWorkflows] = useState<WorkflowInfo[]>([]);
  // Started as a workflow: what it is about, sent to its first stage at once (WFL-008).
  const [brief, setBrief] = useState("");
  const chosenWorkflow = initialWorkflow ? workflows.find((w) => w.id === (workflow || initialWorkflow)) : undefined;
  const firstStage = (chosenWorkflow?.stages ?? [])[0];
  useEffect(() => {
    EngineService.Workflows()
      .then((l) => setWorkflows(l ?? []))
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
    const timer = setTimeout(() => loadCaps(false), 400);
    return () => clearTimeout(timer);
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
    const timer = setTimeout(loadSettings, 300);
    return () => clearTimeout(timer);
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
    EngineService.CreateSession({ title, projectRoot: root, environmentId, profileId, providerId: usesProvider ? providerId : "", model: usesProvider ? model.trim() : "", mode, worktree: own, optimizer: usesProvider || harness?.takesOptimizer ? optimizer : "", workflow })
      .then(async (s) => {
        if (rememberWorktree && s.projectId) await EngineService.SetProjectWorktreeDefault(s.projectId, own ? "always" : "never").catch(onError);
        if (makeDefault && s.projectId) await EngineService.SetProjectProfile(s.projectId, profileId).catch(onError);
        if (issue) await EngineService.SetTags(s.id, [issue.tag]).catch(onError);
        const first = (workflows.find((w) => w.id === s.workflow)?.stages ?? [])[0];
        // The brief goes to the first stage straight away; if the stage cannot run
        // yet, it waits in the box with its command, and the reason shows.
        if (initialWorkflow && first && brief.trim() && (!first.kind || first.kind === "prompt")) {
          const ran = await EngineService.RunStage(s.id, first.command, brief.trim()).then(
            () => true,
            (err) => {
              onError(err);
              return false;
            },
          );
          onCreated(s, ran ? undefined : `/${first.command} ${brief.trim()}`);
          return;
        }
        onCreated(s, issue?.text ?? (initialWorkflow && first ? `/${first.command} ` : undefined));
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
        <h2>{t("app.newSession")}</h2>
        <p>{t("newSession.noProvider")}</p>
        <button className="primary" onClick={onManageProviders}>
          {t("newSession.addProvider")}
        </button>
      </section>
    );
  }

  const blocked = caps !== null && !caps.tools;
  const addProvider = (
    <button type="button" className="link" onClick={onManageProviders}>
      {t("newSession.addProvider")}
    </button>
  );

  return (
    <section className="page">
      <h2>{initialWorkflow ? t("newSession.startWorkflow") : t("app.newSession")}</h2>
      <form className="card form" onSubmit={create}>
        {initialWorkflow && firstStage && (
          <label className="workflow-brief">
            {t("newSession.brief")}
            <textarea
              value={brief}
              onChange={(e) => setBrief(e.target.value)}
              rows={3}
              required={!!firstStage.needsInput}
              placeholder={t("newSession.briefPlaceholder")}
              autoFocus
            />
            <span className="muted">
              {chosenWorkflow?.reason
                ? t("newSession.workflowCannotStart", { name: chosenWorkflow.name, reason: chosenWorkflow.reason })
                : t("newSession.workflowStartsAt", { name: chosenWorkflow?.name ?? "", stage: firstStage.title })}
            </span>
          </label>
        )}
        {projects.length > 0 && (
          <label>
            {t("panel.info.project")}
            <select value={project?.root ?? ""} onChange={(e) => setRoot(e.target.value)}>
              <option value="">{t("newSession.anotherFolder")}</option>
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
          {t("newSession.folder")}
          <div className="row">
            <input value={root} onChange={(e) => setRoot(e.target.value)} placeholder={t("newSession.folderPlaceholder")} required />
            <button type="button" onClick={choose}>
              {t("newSession.choose")}
            </button>
          </div>
        </label>
        {root.trim() && settings && !settings.trusted && (
          <div className="banner warn trust-box" role="status">
            <strong>{t("newSession.trust.ask")}</strong>
            <p>
              {tNodes("newSession.trust.until", {
                agents: <code>{AGENTS_MD}</code>,
                claude: <code>{CLAUDE_MD}</code>,
                settings: <code>{SETTINGS_DIR}</code>,
                git: <code>{GIT_STATUS}</code>,
              })}
            </p>
            {trustLines(settings).length > 0 && (
              <div>
                <strong>{t("trust.banner.lets")}</strong>
                <ul>
                  {trustLines(settings).map((line, n) => (
                    <li key={n}>
                      <code>{line}</code>
                    </li>
                  ))}
                </ul>
              </div>
            )}
            {(settings.worktree?.setup || settings.worktree?.run || settings.worktree?.archive || (settings.worktree?.copy ?? []).length > 0) && (
              <div>
                <strong>{t("newSession.trust.worktree", { file: WORKTREE_YAML })}</strong>
                <ul>
                  {(settings.worktree?.copy ?? []).length > 0 && (
                    <li>
                      {tNodes("newSession.trust.copy", {
                        files: (settings.worktree?.copy ?? []).map((c) => <code key={c}>{c} </code>),
                      })}
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
                {t("trust.palette")}
              </button>
              <span className="muted">{t("newSession.trust.or")}</span>
            </div>
          </div>
        )}
        {settings?.trusted && (settings.hooks ?? []).length > 0 && (
          <div className="banner row" role="status">
            <span>
              {tNodes("newSession.hooks", { file: <code>{HOOKS_YAML}</code> })}
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
            <strong>{t("newSession.problems")}</strong>
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
          {t("settings.defaults.profile")}
          <select value={profileId} onChange={(e) => pickProfile(e.target.value)}>
            {harnesses.flatMap((h) =>
              [false, true].map((fromProject) => {
                const list = (fromProject ? projectProfiles : profiles).filter((p) => (p.harness || "builtin") === h.id);
                if (list.length === 0) return null;
                return (
                  <optgroup key={h.id + fromProject} label={fromProject ? t("newSession.profile.fromProject", { harness: h.name }) : h.name}>
                    {list.map((p) => (
                      <option key={p.id} value={p.id}>
                        {project && project.defaultProfileId === p.id ? t("newSession.profile.projectDefault", { name: p.name }) : p.name}
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
              <input type="checkbox" checked={makeDefault} onChange={(e) => setMakeDefault(e.target.checked)} /> {t("newSession.profile.makeDefault", { project: project.name })}
            </span>
          )}
        </label>
        {harness?.missing && (
          <p className="error-text" role="alert">
            {t("newSession.harness.missing", { name: harness.name, reason: harness.missing })}
          </p>
        )}
        {harness && !harness.usesProvider && (
          <p className="muted harness-note">
            {t("newSession.harness.ownAccess", { name: harness.name })}
            {harness.notes ? ` ${harness.notes}` : ""}
          </p>
        )}
        {harness && !harness.usesProvider && builtinProfile && providers.length > 0 && (
          <p className="harness-note provider-offer">
            <span className="muted">
              {providers.length === 1
                ? t("newSession.harness.cannotUse", { name: harness.name, provider: providers[0].name, profile: builtinProfile.name })
                : t("newSession.harness.cannotUseAny", { name: harness.name, profile: builtinProfile.name })}
            </span>{" "}
            <button type="button" onClick={() => pickProfile(builtinProfile.id)}>
              {t("newSession.useProvider")}
            </button>
          </p>
        )}
        {usesProvider && providers.length === 0 && (
          <p className="banner warn">
            {readyAgent
              ? tNodes("newSession.noProviderForProfileOrAgent", { add: addProvider, agent: readyAgent.name })
              : tNodes("newSession.noProviderForProfile", { add: addProvider })}
          </p>
        )}
        {(!harness || harness.usesProvider) && providers.length > 0 && (
          <>
        <label>
          {t("settings.defaults.provider")}
          <select value={providerId} onChange={(e) => pickProvider(e.target.value)}>
            {providers.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          {t("settings.defaults.model")}
          <input list="models" value={model} onChange={(e) => setModel(e.target.value)} placeholder={t("newSession.modelPlaceholder")} required />
          <datalist id="models">
            {listed.map((id) => {
              const name = names.get(id);
              const label = [favorites.includes(id) && "★", hidden.includes(id) && t("newSession.model.hidden"), name && name !== id && name].filter(Boolean).join(" ");
              return (
                <option key={id} value={id}>
                  {label || undefined}
                </option>
              );
            })}
          </datalist>
          {modelsError && <span className="error-text">{t("newSession.modelsFailed", { error: modelsError })}</span>}
          {!modelsError && models.length > 0 && (
            <span className="muted">
              {t("newSession.modelsAvailable", { count: models.length })}
              {hidden.length > 0 && ` · ${t("newSession.modelsHidden", { count: hidden.length })}`}
            </span>
          )}
        </label>
        {!current?.builtin && (
          <div className="row wrap model-flags">
            {favorites.length > 0 && <span className="muted">{t("newSession.favorites")}</span>}
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
                  title={t("newSession.favoritesTip")}
                >
                  {favorites.includes(chosen) ? `★ ${t("newSession.unfavorite")}` : `☆ ${t("newSession.favorite")}`}
                </button>
                <button type="button" onClick={() => flag(false, !hidden.includes(chosen))} title={t("newSession.hideTip")}>
                  {hidden.includes(chosen) ? t("newSession.unhide") : t("newSession.hide")}
                </button>
              </>
            )}
            {hidden.length > 0 && (
              <label className="row">
                <input type="checkbox" checked={showHidden} onChange={(e) => setShowHidden(e.target.checked)} /> {t("newSession.showHidden")}
              </label>
            )}
          </div>
        )}

        <div className="caps" role="status">
          {capsState === "loading" && <span className="muted">{t("newSession.caps.checking")}</span>}
          {capsState === "probing" && <span className="muted">{t("newSession.caps.probing")}</span>}
          {capsState === "idle" && capsError && <span className="error-text">{capsError}</span>}
          {capsState === "idle" && caps && caps.source === "unknown" && (
            <span>
              {t("newSession.caps.unknown")}{" "}
              <button type="button" onClick={() => loadCaps(true)}>
                {t("newSession.caps.testNow")}
              </button>
            </span>
          )}
          {capsState === "idle" && caps && caps.source !== "unknown" && caps.tools && (
            <span className="ok">
              {t("newSession.caps.supported", { source: sourceLabel(caps.source) })}
              {caps.contextWindow > 0 && ` · ${t("newSession.caps.context", { tokens: caps.contextWindow })}`}
              {caps.source === "override" && (
                <>
                  {" "}
                  <button type="button" onClick={() => override(null)}>
                    {t("newSession.caps.undoOverride")}
                  </button>
                </>
              )}
            </span>
          )}
          {capsState === "idle" && blocked && caps.source !== "unknown" && (
            <span className="error-text">
              {t("newSession.caps.blocked", { source: sourceLabel(caps.source) })}{" "}
              <button type="button" onClick={() => override(caps.source === "override" ? null : true)}>
                {caps.source === "override" ? t("newSession.caps.undoOverride") : t("newSession.caps.useAnyway")}
              </button>
            </span>
          )}
        </div>
        {providerId && chosen && capsState === "idle" && caps && (
          <label className="context-limit-row">
            {t("panel.info.contextLimit")} <span className="muted">{t("newSession.contextLimitHint")}</span>
            <ContextLimit providerId={providerId} model={chosen} onChanged={() => loadCaps(false)} onError={onError} />
          </label>
        )}
          </>
        )}

        {root.trim() && (
          <div className="worktree-choice">
            {others > 0 && project && !project.worktreeDefault && (
              <p className="muted">
                {t("newSession.otherSessions", { count: others, project: project.name })}
              </p>
            )}
            <label className="row">
              <input type="checkbox" checked={worktree && !!settings?.trusted} disabled={!settings?.trusted} onChange={(e) => setWorktree(e.target.checked)} />
              {t("newSession.worktree")}
              {!settings?.trusted && <span className="muted"> {t("newSession.worktreeTrustFirst")}</span>}
            </label>
            {project && settings?.trusted && (
              <label className="row muted">
                <input type="checkbox" checked={rememberWorktree} onChange={(e) => setRememberWorktree(e.target.checked)} /> {t("newSession.rememberWorktree", { project: project.name })}
              </label>
            )}
            {project && settings?.trusted && worktree && <WorktreeScripts projectId={project.id} onError={onError} />}
          </div>
        )}
        <label>
          {t("settings.defaults.environment")}
          <select value={environmentId || "local"} onChange={(e) => setEnvironmentId(e.target.value)}>
            {environments.map((e) => (
              <option key={e.id} value={e.id}>
                {(e.secretNames ?? []).length > 0 ? t("newSession.environmentSecrets", { name: e.name, count: (e.secretNames ?? []).length }) : e.name}
              </option>
            ))}
          </select>
          {env?.sandbox?.enabled ? (
            <span className="muted">{t("newSession.sandboxed")}</span>
          ) : (
            env?.kind === "local" && <span className="muted">{t("newSession.unsandboxed")}</span>
          )}
        </label>
        {(usesProvider || harness?.takesOptimizer) && extra.length === 0 && (
          <label>
            {t("newSession.optimizer")}
            <select value={optimizer} onChange={(e) => setOptimizer(e.target.value)}>
              <option value="">{waits?.optimizer ? t("newSession.optimizerWaits") : inheritedLabel(profile?.optimizer, defaults?.optimizer, optimizers)}</option>
              <option value={OPTIMIZER_OFF}>{t("newSession.off")}</option>
              {optimizers.map((o) => (
                <option key={o.id} value={o.id} disabled={o.needsApproval || (!o.path && env?.kind !== "docker")}>
                  {o.needsApproval ? t("newSession.notApproved", { name: o.name }) : o.path || env?.kind === "docker" ? o.name : t("newSession.notInstalled", { name: o.name })}
                </option>
              ))}
            </select>
          </label>
        )}
        {workflows.length > 0 && (
          <label>
            {t("composer.workflow")}
            <select value={workflow} onChange={(e) => setWorkflow(e.target.value)}>
              <option value="">{inheritedLabel(profile?.workflow, defaults?.workflow, workflows)}</option>
              <option value={OPTIMIZER_OFF}>{t("newSession.off")}</option>
              {workflows.map((w) => (
                <option key={w.id} value={w.id}>
                  {w.reason ? t("newSession.notApproved", { name: w.name }) : w.name}
                </option>
              ))}
            </select>
            <span className="muted">{t("newSession.workflowTip")}</span>
          </label>
        )}
        <label>
          {t("newSession.mode")}
          <select value={mode} onChange={(e) => setMode(e.target.value)}>
            <option value="">
              {waits?.mode
                ? t("newSession.modeWaits", { mode: modeName("ask") })
                : t(profile?.mode ? "newSession.modeFromProfile" : "newSession.modeFromEnvironment", { mode: modeName(profile?.mode || env?.defaultMode || "ask") })}
            </option>
            {STARTING_MODES.map((m) => (
              <option key={m.id} value={m.id}>
                {modeName(m.id)}
              </option>
            ))}
          </select>
        </label>
        <label>
          {t("newSession.title")} <span className="muted">{t("newSession.optional")}</span>
          <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder={t("app.newSession")} />
        </label>
        {settings?.trusted && allProfiles.length > 1 && (
          <details className="compare-choice" open={extra.length > 0}>
            <summary>{t("newSession.compare")}</summary>
            <p className="muted">{t("newSession.compareNote")}</p>
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
                {t("newSession.task")} <span className="muted">{t("newSession.taskHint", { count: extra.length + 1 })}</span>
                <textarea rows={4} value={task} onChange={(e) => setTask(e.target.value)} placeholder={t("newSession.taskPlaceholder")} required />
              </label>
            )}
          </details>
        )}
        {settings?.trusted && !pickingIssue && extra.length === 0 && (
          <p className="muted">
            {issue ? (
              <>
                {t("newSession.fromIssue", { number: String(issue.number) })}{" "}
                <button type="button" className="link" onClick={() => (setIssue(null), setTitle(""))}>
                  {t("common.remove")}
                </button>
              </>
            ) : (
              <button type="button" className="link" onClick={() => setPickingIssue(true)}>
                {t("newSession.pickIssue")}
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
            {busy ? t("newSession.starting") : extra.length > 0 ? t("newSession.startRuns", { count: extra.length + 1 }) : t("newSession.start")}
          </button>
          {onImport && (
            <button type="button" className="link" onClick={onImport}>
              {t("newSession.import")}
            </button>
          )}
        </div>
      </form>
    </section>
  );
}
