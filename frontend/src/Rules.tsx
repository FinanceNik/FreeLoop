import { useEffect, useState, type FormEvent } from "react";
import { EngineService, type Rule, type Session } from "./api.ts";

interface Props {
  // The session whose session-scoped rules to show, if any.
  session: Session | null;
  // Changes when config files were reloaded from disk.
  version: number;
  onError: (err: unknown) => void;
}

export const RULE_TOOLS = ["shell", "write_file", "edit_file", "read_file", "list_dir", "glob", "grep", "*"];

export function describeRule(r: Rule): string {
  if (r.command) return `${r.tool}: ${r.command}`;
  if (r.path) return `${r.tool}: ${r.path}`;
  return `${r.tool}: any call`;
}

export default function Rules({ session, version, onError }: Props) {
  const [rules, setRules] = useState<Rule[]>([]);
  const [form, setForm] = useState({ effect: "allow", tool: "shell", pattern: "" });

  const load = () =>
    EngineService.ListRules(session?.id ?? "")
      .then((list) => setRules(list ?? []))
      .catch(onError);

  useEffect(() => {
    load();
  }, [session?.id, version]);

  const add = (e: FormEvent) => {
    e.preventDefault();
    const pattern = form.pattern.trim();
    const rule = {
      effect: form.effect,
      tool: form.tool,
      command: form.tool === "shell" ? pattern : "",
      path: form.tool !== "shell" ? pattern : "",
      scope: "global",
      source: "",
    } as unknown as Rule;
    EngineService.AddRule(rule)
      .then(() => {
        setForm({ ...form, pattern: "" });
        load();
      })
      .catch(onError);
  };

  const remove = (r: Rule) => EngineService.DeleteRule(r).then(load).catch(onError);

  const global = rules.filter((r) => r.scope === "global");
  const local = rules.filter((r) => r.scope === "session");

  return (
    <section className="page">
      <h2>Permission rules</h2>
      <p className="muted">
        Rules decide tool calls before you are asked. Deny rules win over allow rules in every scope and in every mode, bypass
        included. Allow rules never lift plan mode or protected paths. A shell rule ending in <code>*</code> matches a prefix and
        only ever approves a single plain command, never a chain, pipe or redirection.
      </p>

      <h3>Global</h3>
      <p className="muted">Stored in rules.yaml in the app data folder; hand edits are picked up on the next tool call, and an allow rule added there waits for your approval in Settings.</p>
      <RuleList rules={global} onDelete={remove} />

      <form className="card form" onSubmit={add}>
        <div className="row">
          <select value={form.effect} onChange={(e) => setForm({ ...form, effect: e.target.value })} aria-label="Effect">
            <option value="allow">Allow</option>
            <option value="deny">Deny</option>
          </select>
          <select value={form.tool} onChange={(e) => setForm({ ...form, tool: e.target.value })} aria-label="Tool">
            {RULE_TOOLS.map((t) => (
              <option key={t} value={t}>
                {t === "*" ? "any tool" : t}
              </option>
            ))}
          </select>
          <input
            aria-label={form.tool === "shell" ? "Command" : "Path pattern"}
            placeholder={form.tool === "shell" ? "git status, or git * for a prefix" : "src/**, or empty for any path"}
            value={form.pattern}
            onChange={(e) => setForm({ ...form, pattern: e.target.value })}
          />
          <button type="submit" className="primary">
            Add rule
          </button>
        </div>
      </form>

      {session && (
        <>
          <h3>This session: {session.title}</h3>
          <p className="muted">Created with "Allow always" for this session; they end with it.</p>
          <RuleList rules={local} />
        </>
      )}
    </section>
  );
}

function RuleList({ rules, onDelete }: { rules: Rule[]; onDelete?: (r: Rule) => void }) {
  if (rules.length === 0) return <p className="muted">No rules.</p>;
  return (
    <ul className="provider-list">
      {rules.map((r, i) => (
        <li key={i} className="card">
          <div>
            <span className={r.effect === "deny" ? "error-text" : "ok"}>{r.effect}</span> <code>{describeRule(r)}</code>
            {r.pending && <span className="muted"> does not apply until you approve it in Settings</span>}
            <div className="muted">{r.source}</div>
          </div>
          {onDelete && (
            <button className="danger" onClick={() => onDelete(r)}>
              Delete
            </button>
          )}
        </li>
      ))}
    </ul>
  );
}
