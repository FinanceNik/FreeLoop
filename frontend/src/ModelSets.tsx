import { useEffect, useEffectEvent, useState } from "react";
import { EngineService, type Config, type ModelChoice, type ModelSet, type ModelSetInfo, type ProfileEfforts, type ProfileModels } from "./api.ts";
import { ask } from "./ask.ts";
import { keptEffort, modelLabel } from "./modelChoice.ts";
import ModelSelect from "./ModelSelect.tsx";
import { t, type PlainKey } from "./i18n.ts";

// Model sets (WFL-013), on the Workflows settings page: named choices of model and
// effort for a workflow's planning stages, issues and reviews, each row "not fixed"
// or a model of any provider that can be called unasked. A row whose provider is gone
// or has no key keeps its value and says so.

export const ROWS: { key: "planning" | "issues" | "reviews"; label: PlainKey }[] = [
  { key: "planning", label: "modelSets.row.planning" },
  { key: "issues", label: "modelSets.row.issues" },
  { key: "reviews", label: "modelSets.row.reviews" },
];

const value = (c: ModelChoice) => (!c.model ? "" : c.provider ? `${c.provider}\n${c.model}` : c.model);
const choice = (v: string, effort: string): ModelChoice => {
  if (!v) return { provider: "", model: "", effort: "" };
  const i = v.indexOf("\n");
  return i < 0 ? { provider: "", model: v, effort } : { provider: v.slice(0, i), model: v.slice(i + 1), effort };
};
const empty = (): ModelSet => ({ id: "", name: "", planning: choice("", ""), issues: choice("", ""), reviews: choice("", "") });

interface Props {
  version: number;
  onSaved: (c: Config) => void;
  onError: (err: unknown) => void;
}

export default function ModelSets({ version, onSaved, onError }: Props) {
  const [sets, setSets] = useState<ModelSetInfo[] | null>(null);
  const [models, setModels] = useState<ProfileModels | null>(null);
  // The set being edited; an empty ID is a new one.
  const [editing, setEditing] = useState<ModelSet | null>(null);
  const refresh = () =>
    EngineService.ListModelSets()
      .then((l) => setSets(l ?? []))
      .catch(onError);
  const refreshNow = useEffectEvent(refresh);
  const loadModels = useEffectEvent(() => {
    if (editing && !models) EngineService.AllModels().then(setModels).catch(onError);
  });
  useEffect(() => {
    refreshNow();
  }, [version]);
  useEffect(() => {
    loadModels();
  }, [editing]);
  const done = (c: Config) => {
    onSaved(c);
    setEditing(null);
    refresh();
  };
  const save = () => editing && EngineService.SaveModelSet(editing).then(done).catch(onError);
  const remove = async (s: ModelSetInfo) => {
    const question = (s.workflows ?? []).length ? t("modelSets.deleteInUse", { name: s.name, count: (s.workflows ?? []).length }) : t("modelSets.deleteAsk", { name: s.name });
    if (!(await ask(question))) return;
    EngineService.DeleteModelSet(s.id).then(done).catch(onError);
  };
  const line = (c: ModelChoice) => (c.model ? [modelLabel(models?.choices ?? [], value(c)) || c.model, c.effort].filter(Boolean).join(" · ") : t("modelSets.notFixed"));

  return (
    <div className="card model-sets" aria-label={t("modelSets.title")}>
      <h3>{t("modelSets.title")}</h3>
      <p className="muted">{t("modelSets.intro")}</p>
      {(sets ?? []).length === 0 && !editing && <p className="muted">{t("modelSets.none")}</p>}
      <ul className="model-set-list">
        {(sets ?? []).map((s) => (
          <li key={s.id} aria-label={s.name}>
            <strong>{s.name}</strong>
            <dl>
              {ROWS.map((r) => (
                <div key={r.key}>
                  <dt>{t(r.label)}</dt>
                  <dd>
                    {line(s[r.key])}
                    {s.unavailable?.[r.key] && <span className="error-text"> · {t("modelSets.unavailable", { why: s.unavailable[r.key] ?? "" })}</span>}
                  </dd>
                </div>
              ))}
            </dl>
            <p className="row">
              <button onClick={() => setEditing({ ...s })}>{t("modelSets.edit")}</button>
              <button onClick={() => setEditing({ ...s, id: "", name: t("modelSets.copyName", { name: s.name }) })}>{t("modelSets.duplicate")}</button>
              <button onClick={() => remove(s)}>{t("modelSets.delete")}</button>
            </p>
          </li>
        ))}
      </ul>
      {editing ? (
        <div className="form model-set-editor" role="group" aria-label={editing.id ? t("modelSets.editing", { name: editing.name }) : t("modelSets.new")}>
          <label>
            {t("modelSets.name")}
            <input value={editing.name} onChange={(e) => setEditing({ ...editing, name: e.target.value })} />
          </label>
          {ROWS.map((r) => (
            <RowEditor key={r.key} label={t(r.label)} models={models} row={editing[r.key]} onChange={(c) => setEditing({ ...editing, [r.key]: c })} onError={onError} />
          ))}
          <p className="row">
            <button className="primary" disabled={!editing.name.trim()} onClick={save}>
              {t("modelSets.save")}
            </button>
            <button onClick={() => setEditing(null)}>{t("common.cancel")}</button>
          </p>
        </div>
      ) : (
        <p className="row">
          <button onClick={() => setEditing(empty())}>{t("modelSets.new")}</button>
        </p>
      )}
    </div>
  );
}

// RowEditor chooses one row's model and its effort, offered as the model takes one.
function RowEditor({ label, models, row, onChange, onError }: { label: string; models: ProfileModels | null; row: ModelChoice; onChange: (c: ModelChoice) => void; onError: (err: unknown) => void }) {
  const [efforts, setEfforts] = useState<ProfileEfforts | null>(null);
  const v = value(row);
  const keepEffort = useEffectEvent((eff: ProfileEfforts) => {
    const kept = keptEffort(eff.choices ?? [], row.effort);
    if (kept !== row.effort) onChange({ ...row, effort: kept });
  });
  const failed = useEffectEvent((err: unknown) => onError(err));
  useEffect(() => {
    if (!v) {
      setEfforts(null);
      return;
    }
    let live = true;
    EngineService.EffortsOf(v)
      .then((eff) => {
        if (!live) return;
        setEfforts(eff);
        keepEffort(eff);
      })
      .catch(failed);
    return () => {
      live = false;
    };
  }, [v]);
  const levels = efforts?.choices ?? [];
  return (
    <fieldset className="model-set-row">
      <legend>{label}</legend>
      <label>
        {t("modelSets.model")}
        <ModelSelect choices={models?.choices ?? []} custom={!!models?.custom} value={v} emptyLabel={t("modelSets.notFixed")} onChange={(nv) => onChange(choice(nv, nv === v ? row.effort : ""))} />
      </label>
      <label>
        {t("modelSets.effort")}
        <select value={row.effort} disabled={!v || levels.length === 0} onChange={(e) => onChange({ ...row, effort: e.target.value })}>
          <option value="">{t("modelSets.effortDefault")}</option>
          {levels.map((c) => (
            <option key={c.value} value={c.value}>
              {c.name || c.value}
            </option>
          ))}
        </select>
        {v && efforts && levels.length === 0 && <span className="muted">{t("afk.effort.none")}</span>}
      </label>
    </fieldset>
  );
}
