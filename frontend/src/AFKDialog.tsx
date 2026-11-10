import { useEffect, useState } from "react";
import { EngineService, modeName, type AFKPlan, type AFKRun, type ModelChoice, type Profile, type ProfileEfforts, type ProfileModels } from "./api.ts";
import { commandFrom, commandOutcome, defaultAllow } from "./afk.ts";
import { fromSetNote, keptEffort, modelOffered } from "./modelChoice.ts";
import ModelSelect from "./ModelSelect.tsx";
import { t } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";

// Starting an AFK run (WFL-005): what it would do first, where, on which profiles,
// and the commands it will want, which the user can allow for the run's sessions up
// front. Nothing else is approved for it: every other request waits in Needs
// attention, as for any session (SEC-003).

interface Props {
  root: string;
  workflowId?: string;
  profiles: Profile[];
  startedBy: string;
  // The session it is started from, whose cost so far is the run's planning (WFL-015).
  fromSession?: string;
  onClose: () => void;
  onStarted: (run: AFKRun) => void;
  onError: (err: unknown) => void;
}

export default function AFKDialog({ root, workflowId, profiles, startedBy, fromSession, onClose, onStarted, onError }: Props) {
  const [plan, setPlan] = useState<AFKPlan | null>(null);
  const [allow, setAllow] = useState<string[]>([]);
  const [profileId, setProfileId] = useState("");
  const [reviewId, setReviewId] = useState("");
  // The models the issues and the reviews switch to; empty keeps the profile's (WFL-010).
  const [model, setModel] = useState("");
  const [reviewModel, setReviewModel] = useState("");
  // Their efforts, when the model takes one; empty keeps the profile's (WFL-012).
  const [effort, setEffort] = useState("");
  const [reviewEffort, setReviewEffort] = useState("");
  const [busy, setBusy] = useState(false);
  // Commands the user types in to allow as well, such as a check the plan missed (WFL-009).
  const [extra, setExtra] = useState<string[]>([]);
  const [typed, setTyped] = useState("");
  // Pickers whose model the chosen profile cannot have, which hold Start (WFL-014).
  const [blocked, setBlocked] = useState<Record<string, boolean>>({});
  // The workflow's model set starts the pickers off (WFL-014).
  const set = plan?.modelSet ?? null;
  const setIssues = rowValue(set?.issues);
  const setReviews = rowValue(set?.reviews);
  useEffect(() => {
    EngineService.PrepareAFK(root, workflowId ?? "")
      .then((p) => {
        setPlan(p);
        setAllow(defaultAllow(p.commands ?? []));
        setProfileId(p.profileId);
        setReviewId(p.reviewProfileId);
        setModel(rowValue(p.modelSet?.issues));
        setEffort(p.modelSet?.issues?.effort ?? "");
        setReviewModel(rowValue(p.modelSet?.reviews));
        setReviewEffort(p.modelSet?.reviews?.effort ?? "");
      })
      .catch((err) => {
        onError(err);
        onClose();
      });
  }, [root, workflowId]);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  const start = () => {
    if (!plan) return;
    setBusy(true);
    EngineService.StartAFK({ projectRoot: plan.projectRoot, workflowId: plan.workflowId, profileId, reviewProfileId: reviewId, model, reviewModel, effort, reviewEffort, allow, extra, startedBy, fromSession: fromSession ?? "" })
      .then(onStarted)
      .catch(onError)
      .finally(() => setBusy(false));
  };
  const toggle = (c: string, on: boolean) => setAllow((list) => (on ? [...list, c] : list.filter((x) => x !== c)));
  const addTyped = () => {
    const c = typed.trim().replace(/\s+/g, " ");
    if (!c) return;
    if ((plan?.commands ?? []).some((x) => x.command === c)) toggle(c, true);
    else if (!extra.includes(c)) setExtra([...extra, c]);
    setTyped("");
  };
  return (
    <div className="palette-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="afk-dialog" role="dialog" aria-modal="true" aria-label={t("afk.dialog.title")}>
        <h2>{plan ? t("afk.dialog.titleOf", { project: plan.projectName }) : t("afk.dialog.title")}</h2>
        {!plan && <p className="muted">{t("afk.dialog.reading")}</p>}
        {plan && (
          <>
            <p>
              {plan.next ? (
                <>
                  {tNodes("afk.dialog.first", { title: <strong>{plan.next.title}</strong>, path: <span className="muted">({plan.next.path})</span> })}{" "}
                </>
              ) : null}
              <span className="muted">
                {t("afk.dialog.counts", { count: plan.ready, waiting: plan.waiting, human: plan.human })} {t("afk.dialog.oneAtATime", { max: plan.maxIssues })}
              </span>
            </p>
            {plan.problem && (
              <p className="error-text" role="alert">
                {plan.problem}
              </p>
            )}
            {plan.held && profileId === plan.profileId && (
              <p className="error-text" role="status">
                {t("afk.dialog.held", { reason: plan.held })}
              </p>
            )}
            {plan.queuePaused && (
              <p className="error-text" role="status">
                {t("afk.dialog.queuePaused")}
              </p>
            )}
            <p className="muted">
              {t("afk.dialog.worktree")} {t("afk.dialog.mode", { mode: modeName(plan.mode) })} {plan.environmentNote}
            </p>
            <div className="form afk-profiles">
              <label>
                {t("afk.dialog.profileIssues")}
                <select
                  value={profileId}
                  onChange={(e) => {
                    setProfileId(e.target.value);
                    setModel(setIssues);
                  }}
                >
                  {profiles.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>
              </label>
              <ModelPicker
                label={t("afk.dialog.modelIssues")}
                root={plan.projectRoot}
                profileId={profileId}
                value={model}
                onChange={setModel}
                fromSet={{ value: setIssues, name: set?.name ?? "", why: plan.modelSetUnavailable?.issues ?? "" }}
                onReset={() => {
                  setModel(setIssues);
                  setEffort(set?.issues?.effort ?? "");
                }}
                onBlocked={(b) => setBlocked((old) => ({ ...old, issues: b }))}
                onError={onError}
              />
              <EffortPicker label={t("afk.dialog.effortIssues")} root={plan.projectRoot} profileId={profileId} model={model} value={effort} onChange={setEffort} onError={onError} />
              {!plan.reviewOff && (
                <label>
                  {t("afk.dialog.profileReviews")}
                  <select
                    value={reviewId}
                    onChange={(e) => {
                      setReviewId(e.target.value);
                      setReviewModel(setReviews);
                    }}
                  >
                    {profiles.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </select>
                  <span className="muted">{t("afk.dialog.reviewHint")}</span>
                </label>
              )}
              {!plan.reviewOff && (
                <ModelPicker
                  label={t("afk.dialog.modelReviews")}
                  root={plan.projectRoot}
                  profileId={reviewId}
                  value={reviewModel}
                  onChange={setReviewModel}
                  fromSet={{ value: setReviews, name: set?.name ?? "", why: plan.modelSetUnavailable?.reviews ?? "" }}
                  onReset={() => {
                    setReviewModel(setReviews);
                    setReviewEffort(set?.reviews?.effort ?? "");
                  }}
                  onBlocked={(b) => setBlocked((old) => ({ ...old, reviews: b }))}
                  onError={onError}
                />
              )}
              {!plan.reviewOff && <EffortPicker label={t("afk.dialog.effortReviews")} root={plan.projectRoot} profileId={reviewId} model={reviewModel} value={reviewEffort} onChange={setReviewEffort} onError={onError} />}
            </div>
            {(plan.commands ?? []).length > 0 && (
              <fieldset className="afk-commands">
                <legend>{t("afk.dialog.allow")}</legend>
                <p className="muted">{t("afk.dialog.allowHint")}</p>
                {(plan.commands ?? []).map((c) => (
                  <label key={c.command} className="row">
                    <input type="checkbox" checked={allow.includes(c.command)} disabled={c.outcome === "denied"} onChange={(e) => toggle(c.command, e.target.checked)} />
                    <code>{c.command}</code>
                    <span className="muted">
                      {commandFrom(c.from)} · {commandOutcome(c)}
                    </span>
                  </label>
                ))}
                {extra.map((c) => (
                  <label key={c} className="row">
                    <input type="checkbox" checked onChange={() => setExtra(extra.filter((x) => x !== c))} />
                    <code>{c}</code>
                    <span className="muted">{t("afk.dialog.yours")}</span>
                  </label>
                ))}
                <div className="row afk-add-command">
                  <input
                    aria-label={t("afk.dialog.another")}
                    placeholder={t("afk.dialog.anotherPlaceholder", { example: "make check-app" })}
                    value={typed}
                    onChange={(e) => setTyped(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault();
                        addTyped();
                      }
                    }}
                  />
                  <button type="button" onClick={addTyped} disabled={!typed.trim()}>
                    {t("afk.dialog.add")}
                  </button>
                </div>
              </fieldset>
            )}
          </>
        )}
        <p className="row afk-actions">
          <button className="primary" disabled={!plan || !!plan.problem || busy || Object.values(blocked).some(Boolean)} onClick={start}>
            {busy ? t("afk.dialog.starting") : t("afk.dialog.start")}
          </button>
          <button onClick={onClose}>{t("common.cancel")}</button>
        </p>
      </div>
    </div>
  );
}

// ModelPicker chooses the model a profile's sessions switch to for the run: for the
// built-in agent every model of every provider that can be called unasked, the
// profile's own first, or one named by hand (WFL-011); for an external agent what it
// last said it offers.
function ModelPicker({ label, root, profileId, value, onChange, fromSet, onReset, onBlocked, onError }: { label: string; root: string; profileId: string; value: string; onChange: (v: string) => void; fromSet: { value: string; name: string; why: string }; onReset: () => void; onBlocked: (b: boolean) => void; onError: (err: unknown) => void }) {
  const [models, setModels] = useState<ProfileModels | null>(null);
  useEffect(() => {
    setModels(null);
    if (!profileId) return;
    EngineService.ProfileModels(root, profileId)
      .then(setModels)
      .catch(onError);
  }, [root, profileId]);
  // What stands in the way: the set's model gone or without its key, or one the
  // profile cannot have, as with an external agent.
  const problem = !models ? "" : fromSet.why && value === fromSet.value ? t("afk.model.unavailable", { why: fromSet.why }) : !modelOffered({ choices: models.choices ?? [], custom: !!models.custom }, value) ? t("afk.model.notOffered") : "";
  useEffect(() => onBlocked(!!problem), [problem]);
  const note = fromSetNote(fromSet.value, value, fromSet.name);
  return (
    <label>
      {label}
      <ModelSelect
        key={profileId}
        choices={models?.choices ?? []}
        custom={!!models?.custom}
        value={value}
        emptyLabel={models?.default ? t("afk.model.profileOwnNamed", { model: models.default }) : t("afk.model.profileOwn")}
        onChange={onChange}
      />
      {note && (
        <span className="muted afk-model-source">
          {note}
          {value !== fromSet.value && (
            <>
              {" · "}
              <button type="button" className="link" onClick={onReset}>
                {t("afk.model.reset")}
              </button>
            </>
          )}
        </span>
      )}
      {problem && <span className="error-text">{problem}</span>}
      {models?.note && <span className="muted">{models.note}</span>}
    </label>
  );
}

// rowValue is a model set's row as a model picker's value.
function rowValue(row?: ModelChoice | null): string {
  if (!row?.model) return "";
  return row.provider ? `${row.provider}\n${row.model}` : row.model;
}

// EffortPicker chooses the effort a run's sessions switch to on their model: the
// profile's, or a level the model offers; disabled for a model without effort.
function EffortPicker({ label, root, profileId, model, value, onChange, onError }: { label: string; root: string; profileId: string; model: string; value: string; onChange: (v: string) => void; onError: (err: unknown) => void }) {
  const [efforts, setEfforts] = useState<ProfileEfforts | null>(null);
  useEffect(() => {
    if (!profileId) return;
    let live = true;
    EngineService.ModelEfforts(root, profileId, model)
      .then((eff) => {
        if (!live) return;
        setEfforts(eff);
        onChange(keptEffort(eff.choices ?? [], value));
      })
      .catch(onError);
    return () => {
      live = false;
    };
  }, [root, profileId, model]);
  const choices = efforts?.choices ?? [];
  // An external agent that reported no effort has nothing to choose here.
  if (efforts?.external && choices.length === 0) return null;
  return (
    <label>
      {label}
      <select value={value} disabled={choices.length === 0} onChange={(e) => onChange(e.target.value)}>
        <option value="">{efforts?.default ? t("afk.effort.profileOwnNamed", { effort: efforts.default }) : t("afk.model.profileOwn")}</option>
        {choices.map((c) => (
          <option key={c.value} value={c.value}>
            {c.name || c.value}
          </option>
        ))}
      </select>
      {efforts && choices.length === 0 && <span className="muted">{t("afk.effort.none")}</span>}
    </label>
  );
}
