import { useEffect, useState, type ReactNode } from "react";
import { EngineService, MODES, modeHint, type SessionOption, type SessionOptionsReport } from "./api.ts";
import { t } from "./i18n.ts";

// What can be changed in a session, beside where the next message is written
// (PRV-014): the same controls for every harness (HRN-001). The engine says what
// each one offers.

interface Props {
  sessionId: string;
  mode: string;
  canBypass: boolean;
  // Whether the agent has an auto mode, and whether it can be used here.
  offersAuto: boolean;
  canAuto: boolean;
  // Whether Settings allows bypass mode.
  bypassOn: boolean;
  // Changes when a model or effort was chosen, here or anywhere else.
  version: number;
  onMode: (mode: string) => void;
  onOtherModel: () => void;
  onRules: () => void;
  // The session's token optimizer, shown with the rest.
  optimizer: ReactNode;
  // The session's workflow (WFL-002), shown before it.
  workflow?: ReactNode;
  onError: (err: unknown) => void;
}

// A value the engine reports that is not among its choices still shows.
function grouped(o: SessionOption) {
  const listed = o.choices ?? [];
  const choices = listed.some((c) => c.value === o.current) ? listed : [{ value: o.current, name: o.current.split("\n").pop() || t("controls.default"), description: "", group: "" }, ...listed];
  const groups: { group: string; choices: typeof choices }[] = [];
  for (const c of choices) {
    const g = groups.find((x) => x.group === c.group);
    if (g) g.choices.push(c);
    else groups.push({ group: c.group, choices: [c] });
  }
  return groups;
}

const OTHER = "\u0000other";
// The permissions menu's way to the session's rules.
const RULES = "\u0000rules";

export default function SessionControls({ sessionId, mode, canBypass, offersAuto, canAuto, bypassOn, version, onMode, onOtherModel, onRules, optimizer, workflow, onError }: Props) {
  const [report, setReport] = useState<SessionOptionsReport | null>(null);
  const [loading, setLoading] = useState(false);

  const refresh = () =>
    EngineService.SessionOptions(sessionId)
      .then(setReport)
      .catch(onError);
  useEffect(() => {
    refresh();
  }, [sessionId, version]);

  const choose = (o: SessionOption, value: string) => {
    if (value === OTHER) return onOtherModel();
    setReport((r) => r && { ...r, options: (r.options ?? []).map((x) => (x.id === o.id ? { ...x, current: value } : x)) });
    EngineService.SetSessionOption(sessionId, o.id, value)
      .catch(onError)
      .finally(refresh);
  };
  const load = () => {
    setLoading(true);
    EngineService.LoadSessionOptions(sessionId)
      .then(setReport)
      .catch(onError)
      .finally(() => setLoading(false));
  };
  const hint = modeHint(mode);

  return (
    <div className="session-controls">
      {(report?.options ?? []).map((o) => (
        <label key={o.id} className="session-control" title={o.choices?.find((c) => c.value === o.current)?.description || undefined}>
          <span className="sr-only">{o.name || o.id}</span>
          <select value={o.current} onChange={(e) => choose(o, e.target.value)}>
            {grouped(o).map((g) =>
              g.group ? (
                <optgroup key={g.group} label={g.group}>
                  {g.choices.map((c) => (
                    <option key={c.value} value={c.value}>
                      {c.name || c.value}
                    </option>
                  ))}
                </optgroup>
              ) : (
                g.choices.map((c) => (
                  <option key={c.value} value={c.value}>
                    {c.name || c.value}
                  </option>
                ))
              ),
            )}
            {o.custom && <option value={OTHER}>{t("controls.other")}</option>}
          </select>
        </label>
      ))}
      {report?.unknown && (
        <button onClick={load} disabled={loading} title={t("controls.loadHint")}>
          {loading ? t("controls.loading") : t("controls.load")}
        </button>
      )}
      {report?.note && <span className="muted session-control-note">{report.note}</span>}
      {workflow}
      {optimizer}
      <label className="session-control" data-mode={mode} title={hint}>
        <span className="sr-only">{t("settings.page.permissions")}</span>
        <select value={mode} onChange={(e) => (e.target.value === RULES ? onRules() : onMode(e.target.value))}>
          {MODES.filter((m) => m.id !== "auto" || offersAuto || mode === "auto").map((m) => {
            const blocked = m.id !== mode && ((m.id === "bypass" && !canBypass) || (m.id === "auto" && !canAuto));
            const why = m.id === "bypass" && !bypassOn ? "controls.mode.needsSettings" : "controls.mode.needsTrust";
            return (
              <option key={m.id} value={m.id} disabled={blocked} title={t(m.hint)}>
                {blocked ? t(why, { mode: t(m.name) }) : t(m.name)}
              </option>
            );
          })}
          <option value={RULES} title={t("controls.rulesHint")}>
            {t("controls.rules")}
          </option>
        </select>
      </label>
    </div>
  );
}
