import { useEffect, useState, type FormEvent } from "react";
import { ask } from "./ask.ts";
import { basename } from "./paths.ts";
import { commandOutcome, defaultAllow } from "./afk.ts";
import { EngineService, modeName, STARTING_MODES, type AFKPlan, type Defaults, type HarnessInfo, type Profile, type ProjectInfo, type ProviderInfo, type Schedule, type ScheduleInfo } from "./api.ts";
import { describe, REPEATS, weekdays, when } from "./schedules.ts";
import { t } from "./i18n.ts";

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
    const every = setInterval(refresh, 30_000);
    return () => clearInterval(every);
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
    if (!(await ask(t("schedules.deleteAsk", { name: s.name })))) return;
    EngineService.DeleteSchedule(s.id).then(refresh).catch(onError);
  };
  const projectName = (root: string) => projects.find((p) => p.root === root)?.name ?? (basename(root) || root);
  // A schedule's line: what it starts, when, and when it runs next and last ran.
  const line = (s: ScheduleInfo) => {
    const next = when(s.next);
    const last = when(s.lastRun);
    return [
      s.afk ? t("schedules.afkRun") : "",
      describe(s),
      s.enabled ? (next ? t("schedules.next", { when: next }) : "") : t("schedules.off"),
      last ? t("schedules.last", { when: last }) : "",
    ]
      .filter(Boolean)
      .join(" · ");
  };

  return (
    <section className="page">
      <h2>{t("settings.page.schedules")}</h2>
      <p className="muted">{t("schedules.intro")}</p>
      {list === null && <p className="muted">{t("schedules.loading")}</p>}
      {list?.length === 0 && !form && <p className="muted">{t("schedules.empty")}</p>}
      <ul className="provider-list">
        {(list ?? []).map((s) => (
          <li key={s.id} className="card">
            <div>
              <strong>{s.name}</strong> <span className="muted">{projectName(s.projectRoot)}</span>
              <div className="muted">{line(s)}</div>
              {s.lastError && <div className="error-text">{t("schedules.lastFailed", { error: s.lastError })}</div>}
              {s.lastSession && (
                <button className="link" onClick={() => onOpenSession(s.lastSession!)}>
                  {t("schedules.openLast")}
                </button>
              )}
            </div>
            <div className="row">
              <label className="row muted">
                <input type="checkbox" checked={s.enabled} onChange={() => toggle(s)} /> {t("schedules.on")}
              </label>
              <button disabled={busy === s.id} onClick={() => runNow(s)}>
                {busy === s.id ? t("schedules.starting") : t("schedules.runNow")}
              </button>
              <button onClick={() => setForm({ ...s })}>{t("schedules.edit")}</button>
              <button className="danger" onClick={() => remove(s)}>
                {t("schedules.delete")}
              </button>
            </div>
          </li>
        ))}
      </ul>
      {!form && (
        <button className="primary" disabled={projects.length === 0} onClick={() => setForm(blank(defaults, providers))}>
          {t("schedules.add")}
        </button>
      )}
      {!form && projects.length === 0 && <p className="muted">{t("schedules.noProjects")}</p>}
      {form && (
        <form className="card form" onSubmit={save}>
          <h3>{form.id ? t("schedules.editNamed", { name: form.name }) : t("schedules.add")}</h3>
          <label>
            {t("schedules.form.name")}
            <input value={form.name} maxLength={80} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          </label>
          <label>
            {t("schedules.form.project")}
            <select value={form.projectRoot} onChange={(e) => setForm({ ...form, projectRoot: e.target.value })} required>
              <option value="">{t("schedules.form.chooseProject")}</option>
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
            {t("schedules.form.starts")}
            <select value={afk ? "afk" : "session"} onChange={(e) => setForm({ ...form, afk: e.target.value === "afk" ? { allow: [] } : undefined })}>
              <option value="session">{t("schedules.form.startsSession")}</option>
              <option value="afk">{t("schedules.form.startsAfk")}</option>
            </select>
          </label>
          {!afk && (
            <label>
              {t("schedules.form.prompt")}
              <textarea rows={4} value={form.prompt} onChange={(e) => setForm({ ...form, prompt: e.target.value })} placeholder={t("schedules.form.promptPlaceholder")} required />
            </label>
          )}
          {afk && (
            <fieldset className="afk-commands">
              <legend>{t("schedules.form.allow")}</legend>
              <p className="muted">{t("schedules.form.allowHint")}</p>
              {!form.projectRoot && <p className="muted">{t("schedules.form.chooseFirst")}</p>}
              {plan?.problem && <p className="muted">{t("schedules.form.problem", { problem: plan.problem })}</p>}
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
              {t("schedules.form.repeat")}
              <select value={form.repeat} onChange={(e) => setForm({ ...form, repeat: e.target.value })}>
                {REPEATS.map((r) => (
                  <option key={r.id} value={r.id}>
                    {t(r.name)}
                  </option>
                ))}
              </select>
            </label>
            {form.repeat === "weekly" && (
              <label>
                {t("schedules.form.weekday")}
                <select value={form.weekday ?? 1} onChange={(e) => setForm({ ...form, weekday: Number(e.target.value) })}>
                  {weekdays().map((d, i) => (
                    <option key={i} value={i}>
                      {d}
                    </option>
                  ))}
                </select>
              </label>
            )}
            <label>
              {form.repeat === "hourly" ? t("schedules.form.minute") : t("schedules.form.at")}
              <input type="time" value={form.at} onChange={(e) => setForm({ ...form, at: e.target.value })} required />
            </label>
          </div>
          <div className="row wrap">
            <label>
              {t("schedules.form.profile")}
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
                {t("schedules.form.provider")}
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
                {t("schedules.form.model")} <span className="muted">{t("schedules.form.modelHint")}</span>
                <input value={form.model} onChange={(e) => setForm({ ...form, model: e.target.value })} />
              </label>
            )}
            <label hidden={afk}>
              {t("schedules.form.mode")}
              <select value={form.mode} onChange={(e) => setForm({ ...form, mode: e.target.value })}>
                {STARTING_MODES.map((m) => (
                  <option key={m.id} value={m.id}>
                    {modeName(m.id)}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <label className="row">
            <input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} /> {t("schedules.on")}
          </label>
          <div className="row">
            <button type="submit" className="primary">
              {t("common.save")}
            </button>
            <button type="button" onClick={() => setForm(null)}>
              {t("common.cancel")}
            </button>
          </div>
        </form>
      )}
    </section>
  );
}
