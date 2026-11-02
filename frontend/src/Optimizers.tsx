import { Clipboard } from "@wailsio/runtime";
import { useEffect, useState } from "react";
import { EngineService, type Config, type OptimizerCheck, type OptimizerStatus } from "./api.ts";

// The Settings page for token optimizers (HRN-019): which are installed, and
// what new sessions use. Listing them runs nothing, since another program can
// have the same name; Check and choosing one do.

interface Props {
  config: Config;
  // Rises when a config file changes, such as a file in optimizers/.
  version: number;
  onSaved: (c: Config) => void;
  onError: (err: unknown) => void;
}

export default function Optimizers({ config, version, onSaved, onError }: Props) {
  const [list, setList] = useState<OptimizerStatus[] | null>(null);
  const [checks, setChecks] = useState<Record<string, OptimizerCheck>>({});
  const [copied, setCopied] = useState("");
  const refresh = () =>
    EngineService.ListOptimizers()
      .then((l) => setList(l ?? []))
      .catch(onError);
  useEffect(() => {
    refresh();
  }, [version]);

  const check = (id: string) =>
    EngineService.CheckOptimizer(id)
      .then((c) => {
        setChecks((old) => ({ ...old, [id]: c }));
        refresh();
      })
      .catch(onError);
  const setDefault = (id: string) => EngineService.SetDefaultOptimizer(id).then(onSaved).catch(onError);
  const copy = (id: string, text: string) =>
    Clipboard.SetText(text)
      .then(() => {
        setCopied(id);
        setTimeout(() => setCopied(""), 1500);
      })
      .catch(onError);

  const chosen = config.defaults.optimizer;
  return (
    <section className="page">
      <h2>Token optimizers</h2>
      <p className="muted">
        An optimizer runs an agent's shell commands through a program that shortens their output, so they cost fewer tokens. You still approve each command as the model wrote it. Shortened output can leave out something the agent needs, such as a commit message or the end of a large diff, so it is off unless you choose one. It works with the built-in agent; other agents run their commands themselves.
      </p>
      <div className="card form">
        <label>
          For new sessions
          <select value={chosen} onChange={(e) => setDefault(e.target.value)}>
            <option value="">Off</option>
            {(list ?? []).map((o) => (
              <option key={o.id} value={o.id}>
                {o.path ? o.name : `${o.name} (not installed)`}
              </option>
            ))}
            {chosen && list && !list.some((o) => o.id === chosen) && <option value={chosen}>{chosen} (no such optimizer)</option>}
          </select>
          <span className="muted">A profile can choose its own, and each session can switch at any time from its header.</span>
        </label>
      </div>
      {list === null && <p className="muted">Looking for them…</p>}
      {(list ?? []).map((o) => {
        const c = checks[o.id];
        return (
          <div className="card" key={o.id}>
            <h3>{o.name}</h3>
            <p className="muted">
              {o.source ? `From ${o.source} in the data folder. ` : ""}
              {o.path ? `Found at ${o.path}.` : "Not installed."}
            </p>
            {!o.path && o.install && (
              <p className="row">
                <code>{o.install}</code>
                <button onClick={() => copy(o.id, o.install)}>{copied === o.id ? "Copied" : "Copy"}</button>
              </p>
            )}
            {o.path && (
              <p className="row">
                <button onClick={() => check(o.id)}>Check it</button>
                {c && <span className={c.ok ? "muted" : "error-text"}>{c.ok ? `Works${c.version ? `: ${c.version}` : ""}.` : c.reason}</span>}
              </p>
            )}
          </div>
        );
      })}
      <p className="muted">
        To add another, put a file in the <code>optimizers</code> folder of the data folder. Freeloop only uses a program's way of rewriting a command, and runs the original command if the rewrite does anything more. <code>docs/DEVELOPING.md</code> lists the fields.
      </p>
    </section>
  );
}
