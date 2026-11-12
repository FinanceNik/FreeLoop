import { useEffect, useEffectEvent, useState } from "react";
import { t } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";
import { ask } from "./ask.ts";
import { EngineService, type Config, type Hook, type PendingCommand } from "./api.ts";
import { approveAllQuestion, EVENTS, groupPending, pendingLabel } from "./pending.ts";

// The global hooks in Settings (EXT-005). Profiles carry their own in their
// YAML; a trusted project's are in .freeloop/hooks.yaml.
const PROJECT_HOOKS = ".freeloop/hooks.yaml";

// What waits for approval in the app's files (SET-005): a backup or another
// program can put it there, so it does nothing until approved.
export function PendingCommands({ version, onSaved, onError }: { version: unknown; onSaved: (c: Config) => void; onError: (err: unknown) => void }) {
  const [list, setList] = useState<PendingCommand[]>([]);
  const refresh = () =>
    EngineService.PendingCommands()
      .then((l) => setList(l ?? []))
      .catch(onError);
  const refreshNow = useEffectEvent(refresh);
  const versionKey = JSON.stringify(version);
  useEffect(() => {
    refreshNow();
  }, [versionKey]);
  const approve = (c: PendingCommand) =>
    EngineService.ApproveCommand(c.fingerprint)
      .then((saved) => {
        onSaved(saved);
        refresh();
      })
      .catch(onError);
  // Names what the card shows, so what arrived since keeps waiting. One that
  // changed refuses the lot, and the card then shows what is there now.
  const approveAll = async () => {
    if (!(await ask(approveAllQuestion(list)))) return;
    EngineService.ApproveCommands(list.map((c) => c.fingerprint))
      .then((saved) => {
        onSaved(saved);
        refresh();
      })
      .catch((err) => {
        onError(err);
        refresh();
      });
  };
  if (list.length === 0) return null;
  return (
    <div className="card" id="pending-approvals">
      <h3>{t("pending.title", { count: list.length })}</h3>
      <p className="muted">{t("pending.intro")}</p>
      <div className="pending-list" tabIndex={0} role="group" aria-label={t("pending.label", { count: list.length })}>
        {groupPending(list).map((g) => (
          <div key={g.title}>
            <h4>{g.title}</h4>
            {g.items.map((c) => (
              <div className="row pending-row" key={c.fingerprint + c.source}>
                <span>{tNodes("pending.item", { what: pendingLabel(c), source: c.source, runs: <code>{c.runs}</code> })}</span>
                <button type="button" onClick={() => approve(c)}>
                  {t("pending.approve")}
                </button>
              </div>
            ))}
          </div>
        ))}
      </div>
      {list.length > 1 && (
        <div className="row pending-all">
          <button type="button" onClick={approveAll}>
            {t("pending.approveAll", { count: list.length })}
          </button>
        </div>
      )}
    </div>
  );
}

export function HookTable({ hooks, onChange }: { hooks: Hook[]; onChange: (h: Hook[]) => void }) {
  const set = (i: number, patch: Partial<Hook>) => onChange(hooks.map((h, j) => (j === i ? { ...h, ...patch } : h)));
  return (
    <>
      <p className="muted">{tNodes("hooks.intro", { file: <code>{PROJECT_HOOKS}</code> })}</p>
      {hooks.length > 0 && (
        <table className="prices hooks">
          <thead>
            <tr>
              <th>{t("hooks.when")}</th>
              <th>{t("hooks.tool")}</th>
              <th>{t("hooks.command")}</th>
              <th>{t("hooks.timeout")}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {hooks.map((h, i) => (
              <tr key={i}>
                <td>
                  <select aria-label={t("hooks.when")} value={h.event} onChange={(e) => set(i, { event: e.target.value, match: e.target.value === "session_start" ? "" : h.match })}>
                    {EVENTS.map((ev) => (
                      <option key={ev.id} value={ev.id}>
                        {t(ev.label)}
                      </option>
                    ))}
                  </select>
                </td>
                <td>
                  <input
                    aria-label={t("hooks.tool")}
                    placeholder={h.event === "session_start" ? "" : t("hooks.anyTool")}
                    disabled={h.event === "session_start"}
                    value={h.match}
                    onChange={(e) => set(i, { match: e.target.value })}
                  />
                </td>
                <td>
                  <input aria-label={t("hooks.command")} className="hook-command" required value={h.command} onChange={(e) => set(i, { command: e.target.value })} />
                </td>
                <td>
                  <input
                    aria-label={t("hooks.timeoutLabel")}
                    type="number"
                    min={1}
                    max={600}
                    placeholder="30"
                    value={h.timeout || ""}
                    onChange={(e) => set(i, { timeout: e.target.value === "" ? 0 : Math.max(0, Math.round(Number(e.target.value))) })}
                  />
                </td>
                <td>
                  <button type="button" className="link" onClick={() => onChange(hooks.filter((_, j) => j !== i))}>
                    {t("common.remove")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <div className="row">
        <button type="button" onClick={() => onChange([...hooks, { event: "pre_tool_use", match: "", command: "", timeout: 0, source: "" }])}>
          {t("hooks.add")}
        </button>
      </div>
    </>
  );
}
