import { useEffect, useEffectEvent, useState, type FormEvent } from "react";
import { EngineService, MODES, errorText, modeName, type PolicyCheck, type PolicyOverview, type ProjectInfo, type Rule, type Session } from "./api.ts";
import { CHECK_TOOLS, headline, ruleWhere, toQuery } from "./policy.ts";
import { t, type PlainKey } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";

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
  return t("rules.anyCall", { tool: r.tool });
}

// A rule's effect in words; one the engine names otherwise shows as given.
const EFFECT: Record<string, PlainKey> = { allow: "panel.rule.allow", deny: "panel.rule.deny" };
function effectName(effect: string): string {
  const key = EFFECT[effect];
  return key ? t(key) : effect;
}

export default function Rules({ session, projects, version, onError }: Props) {
  const [rules, setRules] = useState<Rule[]>([]);
  const [form, setForm] = useState({ effect: "allow", tool: "shell", pattern: "" });

  const load = () =>
    EngineService.ListRules(session?.id ?? "")
      .then((list) => setRules(list ?? []))
      .catch(onError);

  const loadNow = useEffectEvent(load);
  useEffect(() => {
    loadNow();
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
      <h2>{t("settings.page.rules")}</h2>
      <PolicyCheckForm session={session} projects={projects} version={version} />
      <p className="muted">{tNodes("rules.intro", { star: <code>*</code> })}</p>

      <h3>{t("rules.global.title")}</h3>
      <p className="muted">{t("rules.global.note")}</p>
      <RuleList rules={global} onDelete={remove} />

      <form className="card form" onSubmit={add}>
        <div className="row">
          <select value={form.effect} onChange={(e) => setForm({ ...form, effect: e.target.value })} aria-label={t("panel.rules.effect")}>
            <option value="allow">{t("panel.rules.allow")}</option>
            <option value="deny">{t("panel.rules.deny")}</option>
          </select>
          <select value={form.tool} onChange={(e) => setForm({ ...form, tool: e.target.value })} aria-label={t("rules.tool")}>
            {RULE_TOOLS.map((tool) => (
              <option key={tool} value={tool}>
                {tool === "*" ? t("panel.rules.anyTool") : tool}
              </option>
            ))}
          </select>
          <input
            aria-label={form.tool === "shell" ? t("rules.command") : t("panel.rules.path")}
            placeholder={form.tool === "shell" ? t("rules.command.placeholder") : t("rules.path.placeholder")}
            value={form.pattern}
            onChange={(e) => setForm({ ...form, pattern: e.target.value })}
          />
          <button type="submit" className="primary">
            {t("rules.add")}
          </button>
        </div>
      </form>

      {session && (
        <>
          <h3>{t("rules.thisSession", { title: session.title })}</h3>
          <p className="muted">{t("rules.session.note", { allowAlways: t("approval.allowAlways") })}</p>
          <RuleList rules={local} />
        </>
      )}

      <Overview session={session} projects={projects} version={version} />
    </section>
  );
}

function RuleList({ rules, onDelete }: { rules: Rule[]; onDelete?: (r: Rule) => void }) {
  if (rules.length === 0) return <p className="muted">{t("rules.none")}</p>;
  return (
    <ul className="provider-list">
      {rules.map((r, i) => (
        <li key={i} className="card">
          <div>
            <span className={r.effect === "deny" ? "error-text" : "ok"}>{effectName(r.effect)}</span> <code>{describeRule(r)}</code>
            {r.pending && <span className="muted"> {t("rules.pending")}</span>}
            <div className="muted">{r.source}</div>
          </div>
          {onDelete && (
            <button className="danger" onClick={() => onDelete(r)}>
              {t("rules.delete")}
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
  const out = session ? [{ id: `session:${session.id}`, label: t("rules.thisSession", { title: session.title }) }] : [];
  for (const p of projects) out.push({ id: `project:${p.root}`, label: t("rules.newSessionIn", { name: p.name }) });
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
  const tool = CHECK_TOOLS.find((c) => c.id === input.tool) ?? CHECK_TOOLS[0];

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
  const recheck = useEffectEvent(() => {
    if (verdict) check();
  });
  useEffect(() => {
    recheck();
  }, [version]);

  if (where.length === 0) return null;
  const head = verdict && headline(verdict);
  return (
    <form className="card form policy-check" onSubmit={check} aria-label={t("rules.check.title")}>
      <h3>{t("rules.check.title")}</h3>
      <div className="row">
        <select aria-label={t("rules.check.where")} value={at} onChange={(e) => setInput({ ...input, where: e.target.value })}>
          {where.map((w) => (
            <option key={w.id} value={w.id}>
              {w.label}
            </option>
          ))}
        </select>
        <select aria-label={t("rules.check.mode")} value={input.mode} onChange={(e) => setInput({ ...input, mode: e.target.value })}>
          <option value="">{at.startsWith("session:") ? t("rules.check.itsMode") : t("rules.check.defaultMode")}</option>
          {MODES.map((m) => (
            <option key={m.id} value={m.id}>
              {modeName(m.id)}
            </option>
          ))}
        </select>
      </div>
      <div className="row">
        <select aria-label={t("rules.check.action")} value={input.tool} onChange={(e) => setInput({ ...input, tool: e.target.value })}>
          {CHECK_TOOLS.map((c) => (
            <option key={c.id} value={c.id}>
              {t(c.label)}
            </option>
          ))}
        </select>
        <input
          aria-label={input.tool === "shell" ? t("rules.check.command") : t("rules.check.path")}
          placeholder={tool.placeholder}
          value={input.what}
          onChange={(e) => setInput({ ...input, what: e.target.value })}
        />
        <button type="submit" className="primary" disabled={!input.what.trim()}>
          {t("rules.check.submit")}
        </button>
      </div>
      {error && <p className="error-text">{error}</p>}
      {verdict && head && (
        <div className="policy-verdict" role="status">
          <p className={head.tone === "ok" ? "ok" : head.tone === "error" ? "error-text" : "warn-text"}>{head.text}</p>
          {verdict.rule && (
            <p className="muted">
              {tNodes("rules.verdict.rule", { rule: <code>{describeRule(verdict.rule)}</code>, where: ruleWhere(verdict.rule) })}
            </p>
          )}
          {(verdict.overruled ?? []).map((r, i) => (
            <p key={i} className="muted">
              {tNodes("rules.verdict.overruled", { effect: effectName(r.effect), rule: <code>{describeRule(r)}</code>, where: ruleWhere(r) })}
            </p>
          ))}
          {(verdict.pending ?? []).map((r, i) => (
            <p key={`p${i}`} className="muted">
              {tNodes("rules.verdict.pending", { effect: effectName(r.effect), rule: <code>{describeRule(r)}</code> })}
            </p>
          ))}
          {(verdict.hooks ?? []).length > 0 && (
            <p className="muted">
              {t("rules.verdict.hooks", { count: verdict.hooks!.length, sources: verdict.hooks!.map((h) => h.source).join(", ") })}
            </p>
          )}
          {(verdict.agent ?? []).map((a, i) => (
            <p key={`a${i}`} className="muted">
              {a}
            </p>
          ))}
          <p className="muted">
            {t("rules.verdict.where", { mode: modeName(verdict.mode), root: verdict.root })}
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
      <h3>{t("rules.overview.title")}</h3>
      {where.length > 1 && (
        <select aria-label={t("rules.overview.for")} value={shown} onChange={(e) => setAt(e.target.value)}>
          {where.map((w) => (
            <option key={w.id} value={w.id}>
              {w.label}
            </option>
          ))}
        </select>
      )}
      {o.rulesUnreadable && <p className="error-text">{t("rules.unreadable", { error: o.rulesUnreadable })}</p>}
      <p className="muted">
        {t(o.sandboxed ? "rules.overview.modeSandboxed" : "rules.overview.mode", { harness: o.harnessName, mode: modeName(o.mode) })}{" "}
        {t(o.trusted ? "rules.overview.trusted" : "rules.overview.untrusted")} {t("rules.overview.projectDeny")}
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
          <h4>{t("rules.overview.agent", { harness: o.harnessName })}</h4>
          {(o.agent ?? []).map((a, i) => (
            <p key={i} className="muted">
              {a}
            </p>
          ))}
        </>
      )}
      <h4>{t("rules.hooks.title")}</h4>
      {(o.hooks ?? []).length === 0 ? (
        <p className="muted">{t("rules.hooks.none")}</p>
      ) : (
        <ul className="provider-list">
          {(o.hooks ?? []).map((h, i) => (
            <li key={i} className="card">
              <div>
                <code>{h.event}</code> {h.match && <code>{h.match}</code>} <span className="muted">{h.source}</span>
                {!h.approved && <span className="muted"> {t("rules.hooks.pending")}</span>}
                <div className="muted">{h.command}</div>
              </div>
            </li>
          ))}
        </ul>
      )}
      <details>
        <summary>{t("rules.protected.title")}</summary>
        <p className="muted">{t("rules.protected.always")}</p>
        <ul className="policy-paths">
          {(o.protected ?? []).map((p) => (
            <li key={p}>
              <code>{p}</code>
            </li>
          ))}
        </ul>
        <p className="muted">
          {t("rules.protected.neverWritten", { paths: (o.neverWritten ?? []).join(", ") })}{" "}
          {t("rules.protected.actedOnLater", { paths: (o.actedOnLater ?? []).join(", ") })} {t("rules.protected.secrets")}
        </p>
      </details>
    </>
  );
}
