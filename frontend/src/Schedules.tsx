import { useEffect, useState, type FormEvent } from "react";
import { ask } from "./ask.ts";
import { commandOutcome, defaultAllow } from "./afk.ts";
import { EngineService, STARTING_MODES, type AFKPlan, type Defaults, type HarnessInfo, type Profile, type ProjectInfo, type ProviderInfo, type Schedule, type ScheduleInfo } from "./api.ts";
import { describe, REPEATS, WEEKDAYS, when } from "./schedules.ts";

// Schedules lists scheduled and recurring sessions (SES-012) and edits
// them. Each run is a new session tagged "scheduled"; approvals it needs
// wait in Needs attention, since nothing approves itself.

interface Props {
  projects: ProjectInfo[];
  profiles: Profile[];
  providers: ProviderInfo[];
  harnesses: HarnessInfo[];
  defaults?: Defaults;
  onOpenSession: (id: string) => void;
  onError: (err: unknown) => void;
}

const blank = (d?: Defaults, providers: ProviderInfo[] = []): Schedule => ({
  id: "",
  name: "",
  prompt: "",
  projectRoot: "",
  profileId: d?.profileId || "default",
  providerId: d?.providerId || providers.find((p) => !p.builtin)?.id || providers[0]?.id || "",
  model: "",
  mode: "ask",
  repeat: "daily",
  at: "09:00",
  weekday: 1,
  enabled: true,
  // The engine sets it; Go reads no empty string as a time.
  createdAt: undefined as unknown as string,
});

export default function Schedules({ projects, profiles, providers, harnesses, defaults, onOpenSession, onError }: Props) {
  const [list, setList] = useState<ScheduleInfo[] | null>(null);
  const [form, setForm] = useState<Schedule | null>(null);
  const [busy, setBusy] = useState("");
  const refresh = () =>
    EngineService.Schedules()
      .then((l) => setList(l ?? []))
      .catch(onError);
  useEffect(() => {
    refresh();
    // The next and last runs move while the page is open.
    const t = setInterval(refresh, 30_000);
    return () => clearInterval(t);
  }, []);

  // A schedule can start an AFK run instead (WFL-005): its commands are allowed
  // here, once, as in the run's own dialog.
  const [plan, setPlan] = useState<AFKPlan | null>(null);
  const afk = !!form?.afk;
  useEffect(() => {
    setPlan(null);
    if (!afk || !form?.projectRoot) return;
    EngineService.PrepareAFK(form.projectRoot, form.afk?.workflowId ?? "")
      .then((p) => {
        setPlan(p);
        if (!form.id && (form.afk?.allow ?? []).length === 0) setForm((f) => f && { ...f, afk: { ...f.afk, allow: defaultAllow(p.commands ?? []) } });
      })
      .catch(() => setPlan(null));
  }, [afk, form?.projectRoot]);
  const setAllow = (c: string, on: boolean) =>
    form && setForm({ ...form, afk: { ...form.afk, allow: on ? [...(form.afk?.allow ?? []), c] : (form.afk?.allow ?? []).filter((x) => x !== c) } });
  const profile = profiles.find((p) => p.id === form?.profileId);
  const harness = harnesses.find((h) => h.id === (profile?.harness || "builtin"));
  const usesProvider = !harness || harness.usesProvider;

  const save = (e: FormEvent) => {
    e.preventDefault();
    if (!form) return;
    const s = { ...form, providerId: usesProvider && !afk ? form.providerId : "", model: usesProvider && !afk ? form.model : "" };
    EngineService.SaveSchedule(s)
      .then(() => {
        setForm(null);
        refresh();
      })
      .catch(onError);
  };
  const toggle = (s: ScheduleInfo) =>
    EngineService.SaveSchedule({ ...s, enabled: !s.enabled })
      .then(refresh)
      .catch(onError);
  const runNow = (s: ScheduleInfo) => {
    setBusy(s.id);
    EngineService.RunSchedule(s.id)
      .then((sid) => sid && onOpenSession(sid))
      .catch(onError)
      .finally(() => {
        setBusy("");
        refresh();
      });
  };
  const remove = async (s: ScheduleInfo) => {
    if (!(await ask(`Delete the schedule "${s.name}"? The sessions it started stay.`))) return;
    EngineService.DeleteSchedule(s.id).then(refresh).catch(onError);
  };
  const projectName = (root: string) => projects.find((p) => p.root === root)?.name ?? root.split("/").pop() ?? root;

  return (
    <section className="page">
      <h2>Schedules</h2>
      <p className="muted">
        A schedule starts a new session in a project with the same prompt at set times, while Freeloop is open. Each run is
        tagged #scheduled. It asks for approvals like any session and waits in Needs attention until you answer; runs missed
        while the app was closed make one run when it opens.
      </p>
      {list === null && <p className="muted">Loading…</p>}
      {list?.length === 0 && !form && <p className="muted">No schedules yet.</p>}
      <ul className="provider-list">
        {(list ?? []).map((s) => (
          <li key={s.id} className="card">
            <div>
              <strong>{s.name}</strong> <span className="muted">{projectName(s.projectRoot)}</span>
              <div className="muted">
                {s.afk ? "AFK run · " : ""}
                {describe(s)}
                {s.enabled ? (s.next ? ` · next ${when(s.next)}` : "") : " · off"}
                {when(s.lastRun) && ` · last ${when(s.lastRun)}`}
              </div>
              {s.lastError && <div className="error-text">Last run failed: {s.lastError}</div>}
              {s.lastSession && (
                <button className="link" onClick={() => onOpenSession(s.lastSession!)}>
                  Open the last run
                </button>
              )}
            </div>
            <div className="row">
              <label className="row muted">
                <input type="checkbox" checked={s.enabled} onChange={() => toggle(s)} /> On
              </label>
              <button disabled={busy === s.id} onClick={() => runNow(s)}>
                {busy === s.id ? "Starting…" : "Run now"}
              </button>
              <button onClick={() => setForm({ ...s })}>Edit</button>
              <button className="danger" onClick={() => remove(s)}>
                Delete
              </button>
            </div>
          </li>
        ))}
      </ul>
      {!form && (
        <button className="primary" disabled={projects.length === 0} onClick={() => setForm(blank(defaults, providers))}>
          Add schedule
        </button>
      )}
      {!form && projects.length === 0 && <p className="muted">Start a session in a project first; schedules run in your projects.</p>}
      {form && (
        <form className="card form" onSubmit={save}>
          <h3>{form.id ? `Edit ${form.name}` : "Add schedule"}</h3>
          <label>
            Name
            <input value={form.name} maxLength={80} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          </label>
          <label>
            Project
            <select value={form.projectRoot} onChange={(e) => setForm({ ...form, projectRoot: e.target.value })} required>
              <option value="">Choose a project</option>
              {projects
                .filter((p) => !p.missing)
                .map((p) => (
                  <option key={p.id} value={p.root}>
                    {p.name}
                  </option>
                ))}
            </select>
          </label>
          <label>
            It starts
            <select value={afk ? "afk" : "session"} onChange={(e) => setForm({ ...form, afk: e.target.value === "afk" ? { allow: [] } : undefined })}>
              <option value="session">A session with a prompt</option>
              <option value="afk">An AFK run of the backlog</option>
            </select>
          </label>
          {!afk && (
            <label>
              Prompt
              <textarea rows={4} value={form.prompt} onChange={(e) => setForm({ ...form, prompt: e.target.value })} placeholder="Run the tests and summarise any failures." required />
            </label>
          )}
          {afk && (
            <fieldset className="afk-commands">
              <legend>Allow for the run's sessions</legend>
              <p className="muted">The run does the backlog's ready AFK issues, one fresh session each, in a worktree of its own, on the profile below. Commands you allow here hold for its sessions only; anything else waits in Needs attention.</p>
              {!form.projectRoot && <p className="muted">Choose a project to see what it will want.</p>}
              {plan?.problem && <p className="muted">Now: {plan.problem}</p>}
              {(plan?.commands ?? []).map((c) => (
                <label key={c.command} className="row">
                  <input type="checkbox" checked={(form.afk?.allow ?? []).includes(c.command)} disabled={c.outcome === "denied"} onChange={(e) => setAllow(c.command, e.target.checked)} />
                  <code>{c.command}</code>
                  <span className="muted">{commandOutcome(c)}</span>
                </label>
              ))}
            </fieldset>
          )}
          <div className="row wrap">
            <label>
              Repeat
              <select value={form.repeat} onChange={(e) => setForm({ ...form, repeat: e.target.value })}>
                {REPEATS.map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.name}
                  </option>
                ))}
              </select>
            </label>
            {form.repeat === "weekly" && (
              <label>
                On
                <select value={form.weekday ?? 1} onChange={(e) => setForm({ ...form, weekday: Number(e.target.value) })}>
                  {WEEKDAYS.map((d, i) => (
                    <option key={d} value={i}>
                      {d}
                    </option>
                  ))}
                </select>
              </label>
            )}
            <label>
              {form.repeat === "hourly" ? "Minute (the hour is ignored)" : "At"}
              <input type="time" value={form.at} onChange={(e) => setForm({ ...form, at: e.target.value })} required />
            </label>
          </div>
          <div className="row wrap">
            <label>
              Profile
              <select value={form.profileId} onChange={(e) => setForm({ ...form, profileId: e.target.value })}>
                {profiles.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
            </label>
            {usesProvider && !afk && (
              <label>
                Provider
                <select value={form.providerId} onChange={(e) => setForm({ ...form, providerId: e.target.value, model: "" })}>
                  {providers.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>
              </label>
            )}
            {usesProvider && !afk && (
              <label>
                Model <span className="muted">(empty for the default)</span>
                <input value={form.model} onChange={(e) => setForm({ ...form, model: e.target.value })} />
              </label>
            )}
            <label hidden={afk}>
              Mode
              <select value={form.mode} onChange={(e) => setForm({ ...form, mode: e.target.value })}>
                {STARTING_MODES.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.name}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <label className="row">
            <input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} /> On
          </label>
          <div className="row">
            <button type="submit" className="primary">
              Save
            </button>
            <button type="button" onClick={() => setForm(null)}>
              Cancel
            </button>
          </div>
        </form>
      )}
    </section>
  );
}
