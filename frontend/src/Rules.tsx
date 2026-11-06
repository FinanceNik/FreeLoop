import { useEffect, useState, type FormEvent } from "react";
import { EngineService, MODES, errorText, type PolicyCheck, type PolicyOverview, type ProjectInfo, type Rule, type Session } from "./api.ts";
import { CHECK_TOOLS, headline, ruleWhere, toQuery } from "./policy.ts";

interface Props {
  // The session whose session-scoped rules to show, if any.
  session: Session | null;
  // Projects a call can be checked in without a session.
  projects: ProjectInfo[];
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

export default function Rules({ session, projects, version, onError }: Props) {
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
      <h2>Policy</h2>
      <PolicyCheckForm session={session} projects={projects} version={version} />
      <p className="muted">
        Rules decide tool calls before you are asked. Deny rules win over allow rules in every scope and in every mode, bypass
        included, except an agent's own auto mode, where the agent approves for itself. Allow rules never lift plan mode or protected paths. A shell rule ending in <code>*</code> matches a prefix and
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

      <Overview session={session} projects={projects} version={version} />
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

// Where a call is judged: the session last open, or a project as a new session there
// would be.
function places(session: Session | null, projects: ProjectInfo[]) {
  const out = session ? [{ id: `session:${session.id}`, label: `This session: ${session.title}` }] : [];
  for (const p of projects) out.push({ id: `project:${p.root}`, label: `New session in ${p.name}` });
  return out;
}

// "Would this be allowed?" (SEC-018): the broker's own judgment of a call that is not
// made, with the rule and the scope that decided it.
function PolicyCheckForm({ session, projects, version }: { session: Session | null; projects: ProjectInfo[]; version: number }) {
  const where = places(session, projects);
  const [input, setInput] = useState({ where: where[0]?.id ?? "", mode: "", tool: "shell", what: "" });
  const [verdict, setVerdict] = useState<PolicyCheck | null>(null);
  const [error, setError] = useState("");
  const at = where.some((w) => w.id === input.where) ? input.where : (where[0]?.id ?? "");
  const tool = CHECK_TOOLS.find((t) => t.id === input.tool) ?? CHECK_TOOLS[0];

  const check = (e?: FormEvent) => {
    e?.preventDefault();
    if (!input.what.trim() || !at) return;
    EngineService.CheckPolicy(toQuery({ ...input, where: at }) as never)
      .then((v) => {
        setVerdict(v);
        setError("");
      })
      .catch((err) => {
        setVerdict(null);
        setError(errorText(err));
      });
  };
  // A rule added or a file edited changes the answer.
  useEffect(() => {
    if (verdict) check();
  }, [version]);

  if (where.length === 0) return null;
  const head = verdict && headline(verdict);
  return (
    <form className="card form policy-check" onSubmit={check} aria-label="Would this be allowed?">
      <h3>Would this be allowed?</h3>
      <div className="row">
        <select aria-label="Where" value={at} onChange={(e) => setInput({ ...input, where: e.target.value })}>
          {where.map((w) => (
            <option key={w.id} value={w.id}>
              {w.label}
            </option>
          ))}
        </select>
        <select aria-label="Mode" value={input.mode} onChange={(e) => setInput({ ...input, mode: e.target.value })}>
          <option value="">{at.startsWith("session:") ? "Its mode" : "The default mode"}</option>
          {MODES.map((m) => (
            <option key={m.id} value={m.id}>
              {m.name}
            </option>
          ))}
        </select>
      </div>
      <div className="row">
        <select aria-label="Action" value={input.tool} onChange={(e) => setInput({ ...input, tool: e.target.value })}>
          {CHECK_TOOLS.map((t) => (
            <option key={t.id} value={t.id}>
              {t.label}
            </option>
          ))}
        </select>
        <input
          aria-label={input.tool === "shell" ? "Command to check" : "Path to check"}
          placeholder={tool.placeholder}
          value={input.what}
          onChange={(e) => setInput({ ...input, what: e.target.value })}
        />
        <button type="submit" className="primary" disabled={!input.what.trim()}>
          Check
        </button>
      </div>
      {error && <p className="error-text">{error}</p>}
      {verdict && head && (
        <div className="policy-verdict" role="status">
          <p className={head.tone === "ok" ? "ok" : head.tone === "error" ? "error-text" : "warn-text"}>{head.text}</p>
          {verdict.rule && (
            <p className="muted">
              Rule <code>{describeRule(verdict.rule)}</code>, {ruleWhere(verdict.rule)}
            </p>
          )}
          {(verdict.overruled ?? []).map((r, i) => (
            <p key={i} className="muted">
              Overruled: {r.effect} <code>{describeRule(r)}</code>, {ruleWhere(r)}
            </p>
          ))}
          {(verdict.pending ?? []).map((r, i) => (
            <p key={`p${i}`} className="muted">
              Waits for your approval in Settings: {r.effect} <code>{describeRule(r)}</code>
            </p>
          ))}
          {(verdict.hooks ?? []).length > 0 && (
            <p className="muted">
              {verdict.hooks!.length === 1 ? "A hook" : `${verdict.hooks!.length} hooks`} would still run and could deny it:{" "}
              {verdict.hooks!.map((h) => h.source).join(", ")}.
            </p>
          )}
          {(verdict.agent ?? []).map((a, i) => (
            <p key={`a${i}`} className="muted">
              {a}
            </p>
          ))}
          <p className="muted">
            In {MODES.find((m) => m.id === verdict.mode)?.name ?? verdict.mode} mode, in {verdict.root}.
          </p>
        </div>
      )}
    </form>
  );
}

// Everything else that decides a call, beside the rules above.
function Overview({ session, projects, version }: { session: Session | null; projects: ProjectInfo[]; version: number }) {
  const where = places(session, projects);
  const [at, setAt] = useState(where[0]?.id ?? "");
  const [o, setO] = useState<PolicyOverview | null>(null);
  const shown = where.some((w) => w.id === at) ? at : (where[0]?.id ?? "");
  useEffect(() => {
    if (!shown) return;
    EngineService.PolicyOverview(toQuery({ where: shown, mode: "", tool: "", what: "" }) as never)
      .then(setO)
      .catch(() => setO(null));
  }, [shown, version]);
  if (!o) return null;
  const scoped = (o.scopes ?? []).filter((s) => s.scope !== "global" && s.scope !== "session");
  return (
    <>
      <h3>What else decides</h3>
      {where.length > 1 && (
        <select aria-label="Show the policy for" value={shown} onChange={(e) => setAt(e.target.value)}>
          {where.map((w) => (
            <option key={w.id} value={w.id}>
              {w.label}
            </option>
          ))}
        </select>
      )}
      {o.rulesUnreadable && <p className="error-text">Some rules cannot be read, so everything but reads in the project asks: {o.rulesUnreadable}</p>}
      <p className="muted">
        {o.harnessName} in {MODES.find((m) => m.id === o.mode)?.name ?? o.mode} mode{o.sandboxed ? ", commands in the sandbox" : ""}. The project is{" "}
        {o.trusted ? "trusted, so its own rules and hooks apply" : "not trusted, so its own rules and hooks do not apply"}. A project's rules can only deny.
      </p>
      {scoped.map((s) => (
        <div key={s.scope}>
          <h4>{s.name}</h4>
          <p className="muted">{s.note}</p>
          <RuleList rules={s.rules ?? []} />
        </div>
      ))}
      {(o.agent ?? []).length > 0 && (
        <>
          <h4>What {o.harnessName} decides itself</h4>
          {(o.agent ?? []).map((a, i) => (
            <p key={i} className="muted">
              {a}
            </p>
          ))}
        </>
      )}
      <h4>Hooks</h4>
      {(o.hooks ?? []).length === 0 ? (
        <p className="muted">No hooks. A hook can deny a call, never allow one.</p>
      ) : (
        <ul className="provider-list">
          {(o.hooks ?? []).map((h, i) => (
            <li key={i} className="card">
              <div>
                <code>{h.event}</code> {h.match && <code>{h.match}</code>} <span className="muted">{h.source}</span>
                {!h.approved && <span className="muted"> does not run until you approve it in Settings</span>}
                <div className="muted">{h.command}</div>
              </div>
            </li>
          ))}
        </ul>
      )}
      <details>
        <summary>Protected paths</summary>
        <p className="muted">Never read or written, in every mode, whatever a rule says:</p>
        <ul className="policy-paths">
          {(o.protected ?? []).map((p) => (
            <li key={p}>
              <code>{p}</code>
            </li>
          ))}
        </ul>
        <p className="muted">
          Never written in a project: {(o.neverWritten ?? []).map((p) => p).join(", ")}. Other programs act on these later, so accept edits asks before changing them:{" "}
          {(o.actedOnLater ?? []).join(", ")}. Reading a file whose name says it holds secrets, such as .env, asks.
        </p>
      </details>
    </>
  );
}
