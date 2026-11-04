import { useEffect, useState, type FormEvent } from "react";
import { ask } from "./ask.ts";
import { EngineService, STARTING_MODES, type Defaults, type HarnessInfo, type Profile, type ProjectInfo, type ProviderInfo, type Schedule, type ScheduleInfo } from "./api.ts";
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

  const profile = profiles.find((p) => p.id === form?.profileId);
  const harness = harnesses.find((h) => h.id === (profile?.harness || "builtin"));
  const usesProvider = !harness || harness.usesProvider;

  const save = (e: FormEvent) => {
    e.preventDefault();
    if (!form) return;
    const s = { ...form, providerId: usesProvider ? form.providerId : "", model: usesProvider ? form.model : "" };
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
            Prompt
            <textarea rows={4} value={form.prompt} onChange={(e) => setForm({ ...form, prompt: e.target.value })} placeholder="Run the tests and summarise any failures." required />
          </label>
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
            {usesProvider && (
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
            {usesProvider && (
              <label>
                Model <span className="muted">(empty for the default)</span>
                <input value={form.model} onChange={(e) => setForm({ ...form, model: e.target.value })} />
              </label>
            )}
            <label>
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
