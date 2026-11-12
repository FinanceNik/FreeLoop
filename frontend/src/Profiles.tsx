import { useEffect, useEffectEvent, useState, type FormEvent } from "react";
import { ask } from "./ask.ts";
import { t } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";
import { EngineService, modeName, STARTING_MODES, type Command, type HarnessInfo, type Issue, type OptimizerStatus, type Profile, type ProviderInfo, type WorkflowInfo } from "./api.ts";
import { OPTIMIZER_OFF } from "./optimizers.ts";

// Profiles (HRN-003): one YAML file each in profiles/. The form covers the
// common fields and slash commands; rules are edited in the file.

interface Props {
  profiles: Profile[];
  harnesses: HarnessInfo[];
  providers: ProviderInfo[];
  onChanged: () => void;
  onError: (err: unknown) => void;
}

const TOOLS = ["read_file", "list_dir", "glob", "grep", "write_file", "edit_file", "shell"];
const EFFORTS = ["low", "medium", "high", "xhigh", "max"];

// names is a comma-separated field's entries.
function names(text: string): string[] {
  return text
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
}

function blank(): Profile {
  return {
    id: "",
    name: "",
    description: "",
    onLimit: "",
    harness: "builtin",
    provider: "",
    model: "",
    effort: "",
    mode: "",
    systemPrompt: "",
    tools: [],
    rules: [],
    maxTurns: 0,
    instructions: "",
    commands: [],
    skills: [],
    mcpServers: [],
    hooks: [],
    fallback: null,
    optimizer: "",
    workflow: "",
    source: "",
  };
}

export default function Profiles({ profiles, harnesses, providers, onChanged, onError }: Props) {
  // Token optimizers to choose from (HRN-019).
  const [optimizers, setOptimizers] = useState<OptimizerStatus[]>([]);
  const failed = useEffectEvent((err: unknown) => onError(err));
  useEffect(() => {
    EngineService.ListOptimizers()
      .then((l) => setOptimizers(l ?? []))
      .catch(failed);
  }, []);
  // Workflows to choose from (WFL-002).
  const [workflows, setWorkflows] = useState<WorkflowInfo[]>([]);
  useEffect(() => {
    EngineService.Workflows()
      .then((l) => setWorkflows(l ?? []))
      .catch(failed);
  }, []);
  // A custom harness from a template (HRN-011).
  const [scaffold, setScaffold] = useState<{ name: string; language: string; folder: string; made?: string } | null>(null);
  const makeHarness = () => {
    if (!scaffold) return;
    EngineService.ScaffoldHarness({ name: scaffold.name, language: scaffold.language, folder: scaffold.folder })
      .then((r) => {
        setScaffold({ ...scaffold, made: r.dir });
        onChanged();
        return EngineService.OpenHarness(r.preset.id);
      })
      .catch(onError);
  };
  const [form, setForm] = useState<Profile | null>(null);
  const [isNew, setIsNew] = useState(false);
  const [busy, setBusy] = useState(false);
  // The YAML editor (GUI-009): the file as written, checked as you type.
  const [yamlEdit, setYamlEdit] = useState<{ id: string; text: string } | null>(null);
  const [yamlIssues, setYamlIssues] = useState<Issue[]>([]);
  const checkYAML = useEffectEvent((id: string, text: string) =>
    EngineService.CheckProfileYAML(id, text)
      .then((l) => setYamlIssues(l ?? []))
      .catch(onError),
  );
  const yamlId = yamlEdit?.id;
  const yamlText = yamlEdit?.text;
  useEffect(() => {
    if (yamlId === undefined || yamlText === undefined) return;
    const timer = setTimeout(() => checkYAML(yamlId, yamlText), 250);
    return () => clearTimeout(timer);
  }, [yamlId, yamlText]);
  const openYAML = (id: string) =>
    EngineService.ProfileYAML(id)
      .then((text) => setYamlEdit({ id, text }))
      .catch(onError);

  const save = (e: FormEvent) => {
    e.preventDefault();
    if (!form) return;
    setBusy(true);
    EngineService.SaveProfile({ ...form, commands: (form.commands ?? []).filter((c) => c.name.trim() || c.prompt.trim()) })
      .then(() => {
        setForm(null);
        onChanged();
      })
      .catch(onError)
      .finally(() => setBusy(false));
  };

  const remove = async (p: Profile) => {
    if (!(await ask(p.id === "default" ? t("profiles.resetDefaultAsk") : t("profiles.deleteAsk", { name: p.name })))) return;
    EngineService.DeleteProfile(p.id).then(onChanged).catch(onError);
  };

  const providerName = (id: string) => providers.find((x) => x.id === id)?.name ?? id;
  const set = <K extends keyof Profile>(k: K, v: Profile[K]) => setForm((f) => (f ? { ...f, [k]: v } : f));
  const setCommand = (i: number, c: Partial<Command>) =>
    set(
      "commands",
      (form?.commands ?? []).map((x, n) => (n === i ? { ...x, ...c } : x)),
    );

  if (yamlEdit) {
    return (
      <section className="page">
        <h2>
          <code>{`profiles/${yamlEdit.id}.yaml`}</code>
        </h2>
        <textarea
          className="yaml-editor"
          spellCheck={false}
          rows={24}
          value={yamlEdit.text}
          onChange={(e) => setYamlEdit({ ...yamlEdit, text: e.target.value })}
          aria-label={t("profiles.yamlLabel")}
        />
        {yamlIssues.length > 0 ? (
          <ul className="error-text">
            {yamlIssues.map((i, n) => (
              <li key={n}>
                {i.line ? t("profiles.yamlLine", { line: i.line, issue: i.field ? `${i.field}: ${i.message}` : i.message }) : i.field ? `${i.field}: ${i.message}` : i.message}
              </li>
            ))}
          </ul>
        ) : (
          <p className="ok">{t("profiles.yamlValid")}</p>
        )}
        <div className="row">
          <button
            className="primary"
            disabled={busy || yamlIssues.length > 0}
            onClick={() => {
              setBusy(true);
              EngineService.SaveProfileYAML(yamlEdit.id, yamlEdit.text)
                .then(() => {
                  setYamlEdit(null);
                  onChanged();
                })
                .catch(onError)
                .finally(() => setBusy(false));
            }}
          >
            {t("common.save")}
          </button>
          <button onClick={() => setYamlEdit(null)}>{t("common.cancel")}</button>
        </div>
      </section>
    );
  }

  if (form) {
    const tools = form.tools ?? [];
    const provider = providers.find((p) => p.id === form.provider);
    return (
      <section className="page">
        <h2>{isNew ? t("profiles.new") : t("profiles.editTitle", { name: form.name || form.id })}</h2>
        <form className="card form" onSubmit={save}>
          {isNew && (
            <label>
              {t("profiles.id")} <span className="muted">{t("profiles.idHint")}</span>
              <input value={form.id} onChange={(e) => set("id", e.target.value)} placeholder={"reviewer"} required pattern="[a-z0-9][a-z0-9-]*" />
            </label>
          )}
          <label>
            {t("profiles.name")}
            <input value={form.name} onChange={(e) => set("name", e.target.value)} required />
          </label>
          <label>
            {t("profiles.description")}
            <input value={form.description} onChange={(e) => set("description", e.target.value)} />
          </label>
          <label>
            {t("profiles.harness")}
            <select value={form.harness || "builtin"} onChange={(e) => set("harness", e.target.value)}>
              {harnesses.map((h) => (
                <option key={h.id} value={h.id}>
                  {h.name}
                </option>
              ))}
            </select>
          </label>
          {!(harnesses.find((h) => h.id === (form.harness || "builtin"))?.usesProvider ?? true) && (
            <label>
              {t("profiles.agentModel")}
              <input value={form.model} onChange={(e) => set("model", e.target.value)} placeholder={t("profiles.agentModelPlaceholder", { model: "gemini-2.5-pro" })} aria-describedby="agent-model-note" />
              <span id="agent-model-note" className="muted">
                {t("profiles.agentModelNote")}
              </span>
            </label>
          )}
          <div className="row wrap" hidden={!(harnesses.find((h) => h.id === (form.harness || "builtin"))?.usesProvider ?? true)}>
            <label>
              {t("settings.defaults.provider")}
              <select value={form.provider} onChange={(e) => set("provider", e.target.value)}>
                <option value="">{t("profiles.perSession")}</option>
                {providers.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
            </label>
            <label>
              {t("settings.defaults.model")}
              <input value={form.model} onChange={(e) => set("model", e.target.value)} placeholder={provider?.defaultModel || t("profiles.perSession")} />
            </label>
            <label title={t("profiles.fallbackTitle")}>
              {t("profiles.fallbackProvider")}
              <select
                value={form.fallback?.provider ?? ""}
                onChange={(e) => set("fallback", e.target.value ? { provider: e.target.value, model: form.fallback?.model ?? "" } : null)}
              >
                <option value="">{t("profiles.none")}</option>
                {providers.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
            </label>
            {form.fallback && (
              <label>
                {t("profiles.fallbackModel")}
                <input required value={form.fallback.model} onChange={(e) => set("fallback", { provider: form.fallback!.provider, model: e.target.value })} />
              </label>
            )}
          </div>
          <div className="row wrap">
            <label>
              {t("profiles.mode")}
              <select value={form.mode} onChange={(e) => set("mode", e.target.value)}>
                <option value="">{t("profiles.modeFromEnvironment")}</option>
                {STARTING_MODES.map((m) => (
                  <option key={m.id} value={m.id}>
                    {modeName(m.id)}
                  </option>
                ))}
              </select>
            </label>
            <label>
              {t("profiles.effort")}
              <select value={form.effort} onChange={(e) => set("effort", e.target.value)}>
                <option value="">{t("profiles.effortDefault", { effort: "high" })}</option>
                {EFFORTS.map((x) => (
                  <option key={x}>{x}</option>
                ))}
              </select>
            </label>
            <label hidden={!(harnesses.find((h) => h.id === (form.harness || "builtin"))?.usesProvider || harnesses.find((h) => h.id === (form.harness || "builtin"))?.takesOptimizer || !harnesses.length)}>
              {t("profiles.optimizer")}
              <select value={form.optimizer} onChange={(e) => set("optimizer", e.target.value)}>
                <option value="">{t("profiles.fromSettings")}</option>
                <option value={OPTIMIZER_OFF}>{t("profiles.off")}</option>
                {optimizers.map((o) => (
                  <option key={o.id} value={o.id}>
                    {o.needsApproval ? t("profiles.notApproved", { name: o.name }) : o.path ? o.name : t("profiles.notInstalled", { name: o.name })}
                  </option>
                ))}
                {form.optimizer && form.optimizer !== OPTIMIZER_OFF && !optimizers.some((o) => o.id === form.optimizer) && <option value={form.optimizer}>{form.optimizer}</option>}
              </select>
            </label>
            <label>
              {t("profiles.workflow")}
              <select value={form.workflow} onChange={(e) => set("workflow", e.target.value)}>
                <option value="">{t("profiles.fromSettings")}</option>
                <option value={OPTIMIZER_OFF}>{t("profiles.off")}</option>
                {workflows.map((w) => (
                  <option key={w.id} value={w.id}>
                    {w.name}
                  </option>
                ))}
                {form.workflow && form.workflow !== OPTIMIZER_OFF && !workflows.some((w) => w.id === form.workflow) && <option value={form.workflow}>{form.workflow}</option>}
              </select>
            </label>
            <label>
              {t("profiles.maxTurns")}
              <input type="number" min={0} max={1000} value={form.maxTurns || ""} placeholder="100" onChange={(e) => set("maxTurns", Number(e.target.value) || 0)} />
            </label>
          </div>
          <label>
            {t("profiles.systemPrompt")} <span className="muted">{t("profiles.systemPromptHint")}</span>
            <textarea rows={5} value={form.systemPrompt} onChange={(e) => set("systemPrompt", e.target.value)} />
          </label>
          <fieldset>
            <legend>{t("profiles.tools")} {tools.length === 0 && <span className="muted">{t("profiles.toolsAll")}</span>}</legend>
            <div className="row wrap">
              {TOOLS.map((tool) => (
                <label key={tool} className="row">
                  <input
                    type="checkbox"
                    checked={tools.length === 0 || tools.includes(tool)}
                    onChange={() => {
                      const on = tools.length === 0 ? TOOLS : tools;
                      const next = on.includes(tool) ? on.filter((x) => x !== tool) : [...on, tool];
                      set("tools", next.length === TOOLS.length ? [] : next);
                    }}
                  />
                  <code>{tool}</code>
                </label>
              ))}
            </div>
          </fieldset>
          <label>
            {t("profiles.skills")} <span className="muted">{t("profiles.skillsHint")}</span>
            <input
              key={form.id}
              defaultValue={(form.skills ?? []).join(", ")}
              onBlur={(e) =>
                set("skills", names(e.target.value))
              }
            />
          </label>
          <label>
            {t("profiles.mcpServers")} <span className="muted">{t("profiles.mcpServersHint")}</span>
            <input
              key={form.id + "-mcp"}
              defaultValue={(form.mcpServers ?? []).join(", ")}
              onBlur={(e) =>
                set("mcpServers", names(e.target.value))
              }
            />
          </label>
          <label className="row">
            <input type="checkbox" checked={form.instructions !== "none"} onChange={(e) => set("instructions", e.target.checked ? "" : "none")} /> {t("profiles.instructions")}
          </label>
          <fieldset>
            <legend>{t("profiles.commands")}</legend>
            {(form.commands ?? []).map((c, i) => (
              <div key={i} className="profile-command">
                <div className="row">
                  <span className="muted">/</span>
                  <input value={c.name} onChange={(e) => setCommand(i, { name: e.target.value })} placeholder={"review"} aria-label={t("profiles.commandName")} />
                  <input value={c.description} onChange={(e) => setCommand(i, { description: e.target.value })} placeholder={t("profiles.commandDescription")} aria-label={t("profiles.description")} />
                  <button type="button" onClick={() => set("commands", (form.commands ?? []).filter((_, n) => n !== i))}>
                    {t("common.remove")}
                  </button>
                </div>
                <textarea rows={2} value={c.prompt} onChange={(e) => setCommand(i, { prompt: e.target.value })} placeholder={t("profiles.commandPromptPlaceholder")} aria-label={t("profiles.commandPrompt")} />
              </div>
            ))}
            <div>
              <button type="button" onClick={() => set("commands", [...(form.commands ?? []), { name: "", description: "", prompt: "" }])}>
                {t("profiles.addCommand")}
              </button>
            </div>
          </fieldset>
          {(form.rules ?? []).length > 0 && (
            <div>
              <strong>{t("profiles.rules")}</strong> <span className="muted">{form.source ? t("profiles.rulesIn", { file: form.source }) : t("profiles.rulesInFile")}</span>
              <ul>
                {(form.rules ?? []).map((r, i) => (
                  <li key={i}>
                    <code>
                      {r.effect} {r.tool}
                      {r.command ? ` ${r.command}` : r.path ? ` ${r.path}` : ""}
                    </code>
                  </li>
                ))}
              </ul>
            </div>
          )}
          <div className="row">
            <button type="submit" className="primary" disabled={busy}>
              {t("common.save")}
            </button>
            <button type="button" onClick={() => setForm(null)}>
              {t("common.cancel")}
            </button>
          </div>
        </form>
      </section>
    );
  }

  return (
    <section className="page">
      <h2>{t("settings.page.profiles")}</h2>
      <p className="muted">
        {tNodes("profiles.intro", { folder: <code>{"profiles/"}</code> })}
      </p>
      <FallbackCard profiles={profiles} harnesses={harnesses} onError={onError} />
      <div className="card">
        <h3>{t("profiles.ownAgent.title")}</h3>
        <p className="muted">
          {t("profiles.ownAgent.intro")}
        </p>
        {!scaffold && (
          <button onClick={() => setScaffold({ name: "", language: "python", folder: "" })}>{t("profiles.ownAgent.new")}</button>
        )}
        {scaffold && !scaffold.made && (
          <form
            className="skill-create"
            onSubmit={(e) => {
              e.preventDefault();
              makeHarness();
            }}
          >
            <label>
              {t("profiles.name")}
              <input required autoFocus value={scaffold.name} onChange={(e) => setScaffold({ ...scaffold, name: e.target.value })} />
            </label>
            <label>
              {t("profiles.ownAgent.template")}
              <select value={scaffold.language} onChange={(e) => setScaffold({ ...scaffold, language: e.target.value })}>
                <option value="python">{t("profiles.ownAgent.python")}</option>
                <option value="go">{t("profiles.ownAgent.go", { module: "github.com/coder/acp-go-sdk" })}</option>
              </select>
            </label>
            <div className="row">
              <button
                type="button"
                onClick={() =>
                  EngineService.ChooseFolder()
                    .then((f) => f && setScaffold({ ...scaffold, folder: f }))
                    .catch(onError)
                }
              >
                {t("profiles.ownAgent.chooseFolder")}
              </button>
              {scaffold.folder ? <code>{scaffold.folder}</code> : <span className="muted">{t("profiles.ownAgent.noFolder")}</span>}
            </div>
            <div className="row">
              <button type="submit" className="primary" disabled={!scaffold.folder || !scaffold.name.trim()}>
                {t("profiles.ownAgent.create")}
              </button>
              <button type="button" onClick={() => setScaffold(null)}>
                {t("common.cancel")}
              </button>
            </div>
          </form>
        )}
        {scaffold?.made && (
          <p className="ok">
            {tNodes("profiles.ownAgent.created", { folder: <code>{scaffold.made}</code> })}{" "}
            <button className="link" onClick={() => setScaffold(null)}>
              {t("profiles.ownAgent.done")}
            </button>
          </p>
        )}
        {harnesses.filter((h) => h.custom).length > 0 && (
          <ul>
            {harnesses
              .filter((h) => h.custom)
              .map((h) => (
                <li key={h.id}>
                  {h.name}{" "}
                  <button className="link" onClick={() => EngineService.OpenHarness(h.id).catch(onError)}>
                    {t("profiles.edit")}
                  </button>
                  {h.missing && <span className="error-text"> {h.missing}</span>}
                </li>
              ))}
          </ul>
        )}
      </div>
      <ul className="provider-list">
        {profiles.map((p) => (
          <li key={p.id} className="card">
            <div>
              <strong>{p.name}</strong> <span className="muted">{p.id}</span>
              {p.description && <div className="muted">{p.description}</div>}
              <div className="muted">
                {[
                  p.provider && (p.model ? `${providerName(p.provider)} · ${p.model}` : providerName(p.provider)),
                  (p.tools ?? []).length > 0 && t("profiles.toolCount", { count: (p.tools ?? []).length }),
                  (p.rules ?? []).length > 0 && t("profiles.ruleCount", { count: (p.rules ?? []).length }),
                  (p.commands ?? []).length > 0 && (p.commands ?? []).map((c) => `/${c.name}`).join(" "),
                  !p.source && t("profiles.builtIn"),
                ]
                  .filter(Boolean)
                  .join(" · ")}
              </div>
            </div>
            <div className="row">
              <button
                onClick={() => {
                  setIsNew(false);
                  setForm({ ...p });
                }}
              >
                {t("profiles.edit")}
              </button>
              {!p.id.startsWith("project:") && <button onClick={() => openYAML(p.id)}>{t("profiles.yaml")}</button>}
              {p.source && <button onClick={() => remove(p)}>{p.id === "default" ? t("common.reset") : t("profiles.delete")}</button>}
            </div>
          </li>
        ))}
      </ul>
      <button
        className="primary"
        onClick={() => {
          setIsNew(true);
          setForm(blank());
        }}
      >
        {t("profiles.new")}
      </button>
    </section>
  );
}

// FallbackCard sets, per agent, the profile a stuck session is offered to continue on
// (HRN-020). A profile's own onLimit continues without asking; this only offers.
function FallbackCard({ profiles, harnesses, onError }: { profiles: Profile[]; harnesses: HarnessInfo[]; onError: (err: unknown) => void }) {
  const [fallbacks, setFallbacks] = useState<Record<string, string> | null>(null);
  const failed = useEffectEvent((err: unknown) => onError(err));
  useEffect(() => {
    EngineService.Config()
      .then((c) => setFallbacks(Object.fromEntries(Object.entries(c.fallbacks ?? {}).filter((e): e is [string, string] => !!e[1]))))
      .catch(failed);
  }, []);
  if (fallbacks === null) return null;
  const save = (agent: string, profileId: string) => {
    const next = { ...fallbacks, [agent]: profileId };
    if (!profileId) delete next[agent];
    setFallbacks(next);
    EngineService.Config()
      .then((c) => EngineService.SaveSettings({ ...c, fallbacks: next }))
      .catch(onError);
  };
  return (
    <div className="card">
      <h3>{t("profiles.stuck.title")}</h3>
      <p className="muted">
        {tNodes("profiles.stuck.intro", { key: <code>{"onLimit"}</code> })}
      </p>
      <dl className="panel-info">
        {harnesses.map((h) => (
          <div key={h.id}>
            <dt>{h.name}</dt>
            <dd>
              <select aria-label={t("profiles.stuck.label", { name: h.name })} value={fallbacks[h.id] ?? ""} onChange={(e) => save(h.id, e.target.value)}>
                <option value="">{t("profiles.stuck.ask")}</option>
                {profiles.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

