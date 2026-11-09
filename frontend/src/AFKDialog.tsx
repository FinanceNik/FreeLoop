import { useEffect, useState } from "react";
import { EngineService, modeName, type AFKPlan, type AFKRun, type Profile, type ProfileModels } from "./api.ts";
import { commandFrom, commandOutcome, defaultAllow } from "./afk.ts";

// Starting an AFK run (WFL-005): what it would do first, where, on which profiles,
// and the commands it will want, which the user can allow for the run's sessions up
// front. Nothing else is approved for it: every other request waits in Needs
// attention, as for any session (SEC-003).

interface Props {
  root: string;
  workflowId?: string;
  profiles: Profile[];
  startedBy: string;
  onClose: () => void;
  onStarted: (run: AFKRun) => void;
  onError: (err: unknown) => void;
}

export default function AFKDialog({ root, workflowId, profiles, startedBy, onClose, onStarted, onError }: Props) {
  const [plan, setPlan] = useState<AFKPlan | null>(null);
  const [allow, setAllow] = useState<string[]>([]);
  const [profileId, setProfileId] = useState("");
  const [reviewId, setReviewId] = useState("");
  // The models the issues and the reviews switch to; empty keeps the profile's (WFL-010).
  const [model, setModel] = useState("");
  const [reviewModel, setReviewModel] = useState("");
  const [busy, setBusy] = useState(false);
  // Commands the user types in to allow as well, such as a check the plan missed (WFL-009).
  const [extra, setExtra] = useState<string[]>([]);
  const [typed, setTyped] = useState("");
  useEffect(() => {
    EngineService.PrepareAFK(root, workflowId ?? "")
      .then((p) => {
        setPlan(p);
        setAllow(defaultAllow(p.commands ?? []));
        setProfileId(p.profileId);
        setReviewId(p.reviewProfileId);
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
    EngineService.StartAFK({ projectRoot: plan.projectRoot, workflowId: plan.workflowId, profileId, reviewProfileId: reviewId, model, reviewModel, allow, extra, startedBy })
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
  const mode = plan?.mode && modeName(plan.mode);
  return (
    <div className="palette-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="afk-dialog" role="dialog" aria-modal="true" aria-label="Run the AFK issues">
        <h2>Run the AFK issues{plan ? ` of ${plan.projectName}` : ""}</h2>
        {!plan && <p className="muted">Reading the backlog…</p>}
        {plan && (
          <>
            <p>
              {plan.next ? (
                <>
                  First: <strong>{plan.next.title}</strong> <span className="muted">({plan.next.path})</span>.{" "}
                </>
              ) : null}
              <span className="muted">
                {plan.ready} AFK {plan.ready === 1 ? "issue" : "issues"} ready, {plan.waiting} waiting on others, {plan.human} for a person. One fresh session per issue, one at a time, at most {plan.maxIssues}.
              </span>
            </p>
            {plan.problem && (
              <p className="error-text" role="alert">
                {plan.problem}
              </p>
            )}
            {plan.held && profileId === plan.profileId && (
              <p className="error-text" role="status">
                Its first issue would wait: {plan.held}. Pick a profile on another agent to start now.
              </p>
            )}
            {plan.queuePaused && (
              <p className="error-text" role="status">
                The queue is paused: the run's issues wait until you resume it on the Queue page.
              </p>
            )}
            <p className="muted">
              The run works in a git worktree and branch of its own; your checkout stays as it is until you merge the branch from the Diff tab. Its sessions start in {mode} mode. {plan.environmentNote}
            </p>
            <div className="form afk-profiles">
              <label>
                Profile for the issues
                <select
                  value={profileId}
                  onChange={(e) => {
                    setProfileId(e.target.value);
                    setModel("");
                  }}
                >
                  {profiles.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>
              </label>
              <ModelPicker label="Model for the issues" root={plan.projectRoot} profileId={profileId} value={model} onChange={setModel} onError={onError} />
              {!plan.reviewOff && (
                <label>
                  Profile for the reviews
                  <select
                    value={reviewId}
                    onChange={(e) => {
                      setReviewId(e.target.value);
                      setReviewModel("");
                    }}
                  >
                    {profiles.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </select>
                  <span className="muted">A stronger model than the issues' catches more. Each finished issue is reviewed, read-only, before it waits for your QA.</span>
                </label>
              )}
              {!plan.reviewOff && <ModelPicker label="Model for the reviews" root={plan.projectRoot} profileId={reviewId} value={reviewModel} onChange={setReviewModel} onError={onError} />}
            </div>
            {(plan.commands ?? []).length > 0 && (
              <fieldset className="afk-commands">
                <legend>Allow for this run's sessions</legend>
                <p className="muted">Unattended, a command that asks waits until you answer. Allow the ones it needs now; they hold for the run's sessions only. Anything else still asks.</p>
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
                    <span className="muted">yours</span>
                  </label>
                ))}
                <div className="row afk-add-command">
                  <input
                    aria-label="Another command to allow"
                    placeholder="Another command, such as make check-app"
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
                    Add
                  </button>
                </div>
              </fieldset>
            )}
          </>
        )}
        <p className="row afk-actions">
          <button className="primary" disabled={!plan || !!plan.problem || busy} onClick={start}>
            {busy ? "Starting…" : "Start the run"}
          </button>
          <button onClick={onClose}>Cancel</button>
        </p>
      </div>
    </div>
  );
}

// ModelPicker chooses the model a profile's sessions switch to for the run: the
// built-in agent's provider models, or what an external agent last said it offers.
function ModelPicker({ label, root, profileId, value, onChange, onError }: { label: string; root: string; profileId: string; value: string; onChange: (v: string) => void; onError: (err: unknown) => void }) {
  const [models, setModels] = useState<ProfileModels | null>(null);
  useEffect(() => {
    setModels(null);
    if (!profileId) return;
    EngineService.ProfileModels(root, profileId)
      .then(setModels)
      .catch(onError);
  }, [root, profileId]);
  const choices = models?.choices ?? [];
  const groups = [...new Set(choices.map((c) => c.group))];
  const name = (v: string) => v.split("\n").pop() ?? v;
  return (
    <label>
      {label}
      <select value={value} disabled={choices.length === 0} onChange={(e) => onChange(e.target.value)}>
        <option value="">{models?.default ? `The profile's own (${models.default})` : "The profile's own"}</option>
        {groups.map((g) => {
          const items = choices.filter((c) => c.group === g).map((c) => (
            <option key={c.value} value={c.value}>
              {c.name || name(c.value)}
            </option>
          ));
          return g ? (
            <optgroup key={g} label={g}>
              {items}
            </optgroup>
          ) : (
            items
          );
        })}
      </select>
      {models?.note && <span className="muted">{models.note}</span>}
    </label>
  );
}
