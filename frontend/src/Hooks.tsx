import { useEffect, useState } from "react";
import { EngineService, type Config, type Hook, type PendingCommand } from "./api.ts";
import { approveAllQuestion, EVENTS, groupPending, pendingLabel } from "./pending.ts";

// The global hooks in Settings (EXT-005). Profiles carry their own in their
// YAML; a trusted project's are in .freeloop/hooks.yaml.

// What waits for approval in the app's files (SET-005): a backup or another
// program can put it there, so it does nothing until approved.
export function PendingCommands({ version, onSaved, onError }: { version: unknown; onSaved: (c: Config) => void; onError: (err: unknown) => void }) {
  const [list, setList] = useState<PendingCommand[]>([]);
  const refresh = () =>
    EngineService.PendingCommands()
      .then((l) => setList(l ?? []))
      .catch(onError);
  useEffect(() => {
    refresh();
  }, [JSON.stringify(version)]);
  const approve = (c: PendingCommand) =>
    EngineService.ApproveCommand(c.fingerprint)
      .then((saved) => {
        onSaved(saved);
        refresh();
      })
      .catch(onError);
  // Names what the card shows, so what arrived since keeps waiting. One that
  // changed refuses the lot, and the card then shows what is there now.
  const approveAll = () => {
    if (!confirm(approveAllQuestion(list))) return;
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
      <h3>Waiting for your approval ({list.length})</h3>
      <p className="muted">
        These are in the app's config files, but were not written or approved in the app: they may come from a restored backup, or from a file edited outside the app. Until you approve them as they are now, hooks and agents run nothing, allow rules and settings do not apply, no session can use an environment, and nothing is sent to an address. Approve only what you recognize.
      </p>
      <div className="pending-list" tabIndex={0} role="group" aria-label={`${list.length} waiting for approval`}>
        {groupPending(list).map((g) => (
          <div key={g.title}>
            <h4>{g.title}</h4>
            {g.items.map((c) => (
              <div className="row pending-row" key={c.fingerprint + c.source}>
                <span>
                  {pendingLabel(c)} from {c.source}: <code>{c.runs}</code>
                </span>
                <button type="button" onClick={() => approve(c)}>
                  Approve
                </button>
              </div>
            ))}
          </div>
        ))}
      </div>
      {list.length > 1 && (
        <div className="row pending-all">
          <button type="button" onClick={approveAll}>
            Approve all {list.length}…
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
      <p className="muted">
        Shell commands run in the session's environment with the event as JSON on standard input. Exit 0 goes on; exit 2 blocks the call (or
        the session's start) with standard error as the reason; any other exit is reported and the session goes on. Hooks can block, never
        approve. Profiles can add hooks in their YAML, and a trusted project in <code>.freeloop/hooks.yaml</code>.
      </p>
      {hooks.length > 0 && (
        <table className="prices hooks">
          <thead>
            <tr>
              <th>When</th>
              <th>Tool</th>
              <th>Command</th>
              <th>Timeout (s)</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {hooks.map((h, i) => (
              <tr key={i}>
                <td>
                  <select aria-label="When" value={h.event} onChange={(e) => set(i, { event: e.target.value, match: e.target.value === "session_start" ? "" : h.match })}>
                    {EVENTS.map((ev) => (
                      <option key={ev.id} value={ev.id}>
                        {ev.name}
                      </option>
                    ))}
                  </select>
                </td>
                <td>
                  <input
                    aria-label="Tool"
                    placeholder={h.event === "session_start" ? "" : "any"}
                    disabled={h.event === "session_start"}
                    value={h.match}
                    onChange={(e) => set(i, { match: e.target.value })}
                  />
                </td>
                <td>
                  <input aria-label="Command" className="hook-command" required value={h.command} onChange={(e) => set(i, { command: e.target.value })} />
                </td>
                <td>
                  <input
                    aria-label="Timeout in seconds"
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
                    Remove
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <div className="row">
        <button type="button" onClick={() => onChange([...hooks, { event: "pre_tool_use", match: "", command: "", timeout: 0, source: "" }])}>
          Add a hook
        </button>
      </div>
    </>
  );
}
