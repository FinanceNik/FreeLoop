import { useEffect, useState } from "react";
import { ask } from "./ask.ts";
import { Events } from "@wailsio/runtime";
import { EngineService, FILES_DROPPED_EVENT_NAME, type ProjectInfo, type ProviderInfo, type SkillCandidate, type SkillList, type SkillTest } from "./api.ts";
import { list as joinNames, locale, t, type PlainKey } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";

// Skills (SKL-002): every skill the app found, where it came from, and
// switches globally and per project. The model sees only the names and
// descriptions of skills that are on, and loads a skill when it needs it.

interface Props {
  projects: ProjectInfo[];
  providers: ProviderInfo[];
  configVersion: number;
  onConfigChanged: () => void;
  onProjectsChanged: () => void;
  onError: (err: unknown) => void;
}

const STATUS: Record<string, PlainKey> = {
  new: "skills.status.new",
  same: "skills.status.same",
  changed: "skills.status.changed",
  invalid: "skills.status.invalid",
};

// sourceName says where a skill came from: the store, or a folder of a project; any
// other source shows as given.
function sourceName(source: string): string {
  if (source === "app") return t("skills.source.app");
  if (source === ".freeloop" || source === ".agents" || source === ".claude") return t("skills.source.project", { folder: `${source}/skills` });
  return source;
}


// Where an installed skill came from: its source and commit, and the ref when it has one.
function installedFrom(i: { source: string; commit: string; ref?: string }): string {
  const commit = i.commit.slice(0, 8);
  return i.ref ? t("skills.fromRef", { source: i.source, commit, ref: i.ref }) : t("skills.from", { source: i.source, commit });
}

export default function Skills({ projects, providers, configVersion, onConfigChanged, onProjectsChanged, onError }: Props) {
  const trusted = projects.filter((p) => p.trusted && !p.missing);
  const [root, setRoot] = useState(trusted[0]?.root ?? "");
  const [list, setList] = useState<SkillList | null>(null);
  const load = () =>
    EngineService.ListSkills(root)
      .then(setList)
      .catch(onError);
  useEffect(() => {
    load();
  }, [root, configVersion]);

  const toggle = (name: string, projectId: string, on: boolean) =>
    EngineService.SetSkillEnabled(name, projectId, on)
      .then(() => {
        if (projectId) onProjectsChanged();
        else onConfigChanged();
        load();
      })
      .catch(onError);

  // Import and install (SKL-003, SKL-005): preview what a source offers,
  // then copy the chosen skills into the store.
  const [preview, setPreview] = useState<{ src: string; cands: SkillCandidate[]; picked: string[] } | null>(null);
  const [busy, setBusy] = useState(false);
  const [installed, setInstalled] = useState("");
  const [path, setPath] = useState("");
  const open = (src: string) => {
    if (!src) return;
    setInstalled("");
    EngineService.PreviewSkills(src)
      .then((list) => {
        const cands = list ?? [];
        setPreview({ src, cands, picked: cands.filter((c) => c.status === "new" || c.status === "changed").map((c) => c.name) });
      })
      .catch(onError);
  };
  const importClaude = () =>
    EngineService.ClaudeSkillsDir()
      .then(open)
      .catch(onError);
  const chooseFolder = () =>
    EngineService.ChooseFolder()
      .then((p) => p && open(p))
      .catch(onError);
  // A folder or .zip dropped on the drop zone.
  useEffect(() => Events.On(FILES_DROPPED_EVENT_NAME, ({ data }) => open(((data as string[]) ?? [])[0] ?? "")), []);
  const install = () => {
    if (!preview) return;
    setBusy(true);
    EngineService.InstallSkills(preview.src, preview.picked)
      .then((done) => {
        setInstalled((done ?? []).length > 0 ? t("skills.installed", { names: joinNames(done ?? []) }) : t("skills.upToDate"));
        setPreview(null);
        load();
      })
      .catch(onError)
      .finally(() => setBusy(false));
  };
  const remove = async (name: string) => {
    if (!(await ask(t("skills.removeAsk", { name })))) return;
    EngineService.RemoveSkill(name).then(load).catch(onError);
  };

  // Authoring (SKL-007): a new skill from the template, opened in the
  // editor, and a test run against a model.
  const [creating, setCreating] = useState<{ name: string; description: string } | null>(null);
  const create = () => {
    if (!creating) return;
    EngineService.CreateSkill(creating.name.trim(), creating.description)
      .then(() => {
        const name = creating.name.trim();
        setCreating(null);
        load();
        return EngineService.OpenSkill(name);
      })
      .catch(onError);
  };
  const [testing, setTesting] = useState<{ name: string; providerId: string; model: string; prompt: string; result?: SkillTest; busy?: boolean } | null>(null);
  const runTest = () => {
    if (!testing) return;
    setTesting({ ...testing, busy: true, result: undefined });
    EngineService.TestSkill({ name: testing.name, providerId: testing.providerId, model: testing.model, prompt: testing.prompt, root })
      .then((result) => setTesting((cur) => (cur ? { ...cur, result, busy: false } : cur)))
      .catch((err) => {
        setTesting((cur) => (cur ? { ...cur, busy: false } : cur));
        onError(err);
      });
  };
  // Git-installed skills (SKL-006): check for updates, update, roll back.
  const [update, setUpdate] = useState<{ name: string; cand?: SkillCandidate; busy: boolean } | null>(null);
  const checkUpdate = (name: string) => {
    setUpdate({ name, busy: true });
    EngineService.CheckSkillUpdate(name)
      .then((cand) => setUpdate({ name, cand, busy: false }))
      .catch((err) => {
        setUpdate(null);
        onError(err);
      });
  };
  const applyUpdate = (name: string) =>
    EngineService.UpdateSkill(name)
      .then(() => {
        setUpdate(null);
        load();
      })
      .catch(onError);
  const rollBack = async (name: string) => {
    if (!(await ask(t("skills.rollBackAsk", { name })))) return;
    EngineService.RollbackSkill(name).then(load).catch(onError);
  };
  const testable = providers.filter((p) => p.hasKey || p.type === "ollama" || p.builtin || p.baseUrl.startsWith("http://localhost"));

  const skills = list?.skills ?? [];
  return (
    <section className="page">
      <h2>{t("settings.page.skills")}</h2>
      <p className="muted">
        {tNodes("skills.intro", {
          store: <code>skills/</code>,
          freeloop: <code>.freeloop/skills</code>,
          agents: <code>.agents/skills</code>,
          claude: <code>.claude/skills</code>,
        })}
      </p>
      <div className="card skill-install" data-file-drop-target="skills">
        <div className="row wrap">
          <button onClick={() => setCreating(creating ? null : { name: "", description: "" })}>{t("skills.new")}</button>
          <button onClick={importClaude}>{t("settings.import.title")}</button>
          <button onClick={chooseFolder}>{t("skills.installFolder")}</button>
          <span className="muted">{t("skills.drop")}</span>
        </div>
        <form
          className="row"
          onSubmit={(e) => {
            e.preventDefault();
            open(path.trim());
          }}
        >
          <input
            value={path}
            onChange={(e) => setPath(e.target.value)}
            placeholder={t("skills.pathPlaceholder")}
            aria-label={t("skills.pathLabel")}
          />
          <button type="submit" disabled={!path.trim()}>
            {t("skills.preview")}
          </button>
        </form>
        {creating && (
          <form
            className="skill-create"
            onSubmit={(e) => {
              e.preventDefault();
              create();
            }}
          >
            <label>
              {t("skills.create.name")} <span className="muted">{t("skills.create.nameHint")}</span>
              <input
                autoFocus
                required
                pattern="[a-z0-9]+(-[a-z0-9]+)*"
                maxLength={64}
                value={creating.name}
                onChange={(e) => setCreating({ ...creating, name: e.target.value })}
              />
            </label>
            <label>
              {t("skills.create.description")} <span className="muted">{t("skills.create.descriptionHint")}</span>
              <textarea required rows={2} maxLength={1024} value={creating.description} onChange={(e) => setCreating({ ...creating, description: e.target.value })} />
            </label>
            <div className="row">
              <button type="submit" className="primary">
                {t("skills.create.submit")}
              </button>
              <button type="button" onClick={() => setCreating(null)}>
                {t("common.cancel")}
              </button>
            </div>
          </form>
        )}
        {installed && <p className="ok">{installed}</p>}
        {preview && (
          <div className="skill-preview">
            <p className="muted">{tNodes("skills.preview.from", { source: <code>{preview.src}</code> })}</p>
            {preview.cands.map((c) => (
              <div key={c.name} className="skill-candidate">
                <label className="row">
                  <input
                    type="checkbox"
                    disabled={c.status === "invalid" || c.status === "same"}
                    checked={preview.picked.includes(c.name)}
                    onChange={(e) =>
                      setPreview({ ...preview, picked: e.target.checked ? [...preview.picked, c.name] : preview.picked.filter((n) => n !== c.name) })
                    }
                  />
                  <strong>{c.name}</strong> <span className={`badge skill-${c.status}`}>{STATUS[c.status] ? t(STATUS[c.status]) : c.status}</span>
                </label>
                {c.description && <div>{c.description}</div>}
                {c.issue && <div className="error-text">{c.issue}</div>}
                {(c.scripts ?? []).length > 0 && (
                  <div className="muted">
                    {tNodes("skills.preview.scripts", { scripts: <code>{(c.scripts ?? []).join(", ")}</code> })}
                  </div>
                )}
                {(c.changes ?? []).length > 0 && (
                  <pre className="skill-changes">{(c.changes ?? []).join("\n")}</pre>
                )}
                {c.diff && <pre className="skill-diff">{c.diff}</pre>}
                {c.installed && c.status !== "new" && (
                  <div className="muted">
                    {c.installed.importedAt && !c.installed.importedAt.startsWith("0001")
                      ? tNodes("skills.preview.installedFromOn", {
                          source: <code>{c.installed.source}</code>,
                          date: new Date(c.installed.importedAt).toLocaleDateString(locale()),
                        })
                      : tNodes("skills.preview.installedFrom", { source: <code>{c.installed.source}</code> })}
                  </div>
                )}
              </div>
            ))}
            <div className="row">
              <button className="primary" disabled={busy || preview.picked.length === 0} onClick={install}>
                {t("skills.install", { count: preview.picked.length })}
              </button>
              <button onClick={() => setPreview(null)}>{t("common.cancel")}</button>
            </div>
          </div>
        )}
      </div>
      <label className="row">
        {t("skills.project")}{" "}
        <select value={root} onChange={(e) => setRoot(e.target.value)}>
          <option value="">{t("skills.storeOnly")}</option>
          {trusted.map((p) => (
            <option key={p.id} value={p.root}>
              {p.name}
            </option>
          ))}
        </select>
      </label>
      {skills.length === 0 && <p className="muted">{t("skills.none")}</p>}
      <ul className="provider-list">
        {skills.map((s) => (
          <li key={s.name} className="card skill">
            <div>
              <strong>{s.name}</strong> <span className="muted">{sourceName(s.source)}</span>
              <div>{s.description}</div>
              <div className="muted">
                <code>{s.path}</code>
                {s.lines > 500 && ` · ${t("skills.lines", { count: s.lines })}`}
                {s.installed?.commit && ` · ${installedFrom(s.installed)}`}
                {(s.allowedTools ?? []).length > 0 && ` · ${t("skills.suggestsTools", { tools: joinNames(s.allowedTools ?? []) })}`}
              </div>
            </div>
            <div className="skill-switches">
              <label className="row">
                <input type="checkbox" checked={s.global} onChange={(e) => toggle(s.name, "", e.target.checked)} /> {t("skills.onEverywhere")}
              </label>
              {list?.projectId && (
                <label className="row">
                  <input type="checkbox" checked={s.project} disabled={!s.global} onChange={(e) => toggle(s.name, list.projectId, e.target.checked)} /> {t("skills.inProject")}
                </label>
              )}
            </div>
            <div className="row">
              {s.source === "app" && (
                <button className="link" onClick={() => EngineService.OpenSkill(s.name).catch(onError)}>
                  {t("skills.edit")}
                </button>
              )}
              <button
                className="link"
                onClick={() =>
                  setTesting(testing?.name === s.name ? null : { name: s.name, providerId: testable[0]?.id ?? "", model: testable[0]?.defaultModel ?? "", prompt: "" })
                }
              >
                {t("skills.test")}
              </button>
              {s.installed?.commit && (
                <button className="link" disabled={update?.busy} onClick={() => checkUpdate(s.name)}>
                  {update?.name === s.name && update.busy ? t("skills.checking") : t("skills.checkUpdate")}
                </button>
              )}
              {s.canRollBack && (
                <button className="link" onClick={() => rollBack(s.name)}>
                  {t("skills.rollBack")}
                </button>
              )}
              {s.source === "app" && (
                <button className="link" onClick={() => remove(s.name)}>
                  {t("common.remove")}
                </button>
              )}
            </div>
            {update?.name === s.name && update.cand && (
              <div className="skill-update">
                {update.cand.status === "same" ? (
                  <p className="ok">{t("skills.update.same", { source: update.cand.source })}</p>
                ) : (
                  <>
                    <p>{t("skills.update.changes", { source: update.cand.source, changes: (update.cand.changes ?? []).join(", ") })}</p>
                    {update.cand.diff && <pre className="skill-diff">{update.cand.diff}</pre>}
                    <div className="row">
                      <button className="primary" disabled={update.cand.status === "invalid"} onClick={() => applyUpdate(s.name)}>
                        {t("skills.update.apply")}
                      </button>
                      <button onClick={() => setUpdate(null)}>{t("skills.update.notNow")}</button>
                    </div>
                    {update.cand.issue && <p className="error-text">{update.cand.issue}</p>}
                  </>
                )}
              </div>
            )}
            {testing?.name === s.name && (
              <form
                className="skill-test"
                onSubmit={(e) => {
                  e.preventDefault();
                  runTest();
                }}
              >
                <label>
                  {t("skills.test.prompt")}
                  <input required value={testing.prompt} onChange={(e) => setTesting({ ...testing, prompt: e.target.value })} />
                </label>
                <div className="row wrap">
                  <select
                    aria-label={t("settings.defaults.provider")}
                    value={testing.providerId}
                    onChange={(e) => setTesting({ ...testing, providerId: e.target.value, model: providers.find((p) => p.id === e.target.value)?.defaultModel ?? "" })}
                  >
                    {providers.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </select>
                  <input aria-label={t("settings.defaults.model")} placeholder={t("settings.defaults.model")} value={testing.model} onChange={(e) => setTesting({ ...testing, model: e.target.value })} />
                  <button type="submit" disabled={testing.busy || !testing.providerId}>
                    {testing.busy ? t("skills.test.asking") : t("skills.test.run")}
                  </button>
                </div>
                {/* What this call sends (NFR-PRIV-003): it belongs to no session, so no session records it. */}
                <p className="muted">{t("skills.test.sends")}</p>
                {testing.result && (
                  <p className={testing.result.picked ? "ok" : "banner warn"}>
                    {testing.result.picked
                      ? t("skills.test.picked")
                      : testing.result.loaded
                        ? t("skills.test.other", { name: testing.result.loaded })
                        : t("skills.test.missed")}
                    {testing.result.reply && <span className="muted"> {t("skills.test.said", { reply: testing.result.reply.slice(0, 300) })}</span>}
                  </p>
                )}
                <p className="muted">{t("skills.test.cost")}</p>
              </form>
            )}
          </li>
        ))}
      </ul>
      {(list?.issues ?? []).length > 0 && (
        <div className="banner warn">
          <strong>{t("skills.issues")}</strong>
          <ul>
            {(list?.issues ?? []).map((i, n) => (
              <li key={n}>
                <code>{i.path}</code>: {i.message}
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}
